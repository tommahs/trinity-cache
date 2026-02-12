# Trinity-cache — Configuration Reference

This document describes all configuration options for Trinity-cache. Configuration is loaded from a YAML file (e.g., `/etc/trinity-cache/config.yaml`). All values listed include their actual defaults from the code.

## Complete configuration schema

```yaml
# Application concurrency: number of parallel download workers
# Type: integer, Range: 1–10000, Default: 8
concurrency: 8

# Path to local filesystem storage for cached packages
# Type: string, Required: Yes
storage_path: "/var/lib/trinity-cache"

# Logging verbosity
# Type: string, Values: debug, info, warn, error, Default: info
log_level: "info"

# HTTP server settings
server:
  # Listen port (include colon prefix)
  # Type: string, Default: :8080
  port: "8080"
  
  # Server read timeout (seconds)
  # Type: integer, Range: 1+, Default: 30
  read_timeout: 30
  
  # Server write timeout (seconds)
  # Type: integer, Range: 1+, Default: 30
  write_timeout: 30

# Upstream mirrors (at least one required)
# Type: array, Min items: 1
mirrors:
  # Each mirror entry:
  - url: "https://mirror1.archlinux.org"
    # Base weight for this mirror (affects selection priority)
    # Type: float, Range: >0, Default: 1.0
    weight: 1.0
    
    # Download timeout for this mirror (seconds)
    # Type: integer, Range: 1+, Default: 30
    timeout: 30

    # Additional mirrors...
  - url: "https://mirror2.archlinux.org"
    weight: 1.0
    timeout: 30

# Package retention settings
retention:
  # Number of recent versions to keep per package
  # Type: integer, Range: 1+, Default: 2
  keep_versions: 2
  
  # How often retention runs (hours)
  # Type: float, Range: 0.1+, Default: 1.0
  enforcement_interval: 1.0

# Mirror weight recovery (affects mirror selection load balancing)
mirror_recovery:
  # How often to recover mirror weights toward base (minutes)
  # Type: integer, Range: 1+, Default: 5
  interval: 5
  
  # Fractional recovery rate per interval (0.0–1.0)
  # Type: float, Range: 0.01–1.0, Default: 0.05 (5% per interval)
  rate: 0.05

# Download behavior
downloads:
  # Maximum retries per failed download attempt
  # Type: integer, Range: 1+, Default: 3
  max_retries: 3
  
  # Download timeout (seconds)
  # Type: integer, Range: 1+, Default: 30
  timeout: 30
  
  # Temporary directory for downloads (empty = system temp)
  # Type: string, Default: ""
  temp_dir: "/var/lib/trinity-cache"
```

## Field reference

| YAML Key | Type | Default | Required | Notes |
|----------|------|---------|----------|-------|
| `concurrency` | int | 8 | — | Must be ≥1 and ≤10000 |
| `storage_path` | string | — | **Yes** | Must be a valid directory path |
| `log_level` | string | "info" | — | One of: `debug`, `info`, `warn`, `error` |
| `server.port` | string | "8080" | — | Port clients can connect to |
| `server.read_timeout` | int | 30 | — | Seconds; must be ≥1 |
| `server.write_timeout` | int | 30 | — | Seconds; must be ≥1 |
| `mirrors` | array | — | **Yes** | Minimum 1 mirror required |
| `mirrors[].url` | string | — | **Yes** | Upstream mirror base URL |
| `mirrors[].weight` | float | 1.0 | — | Must be >0; affects selection priority |
| `mirrors[].timeout` | int | 30 | — | Seconds; must be ≥1 |
| `retention.keep_versions` | int | 2 | — | Must be ≥1 |
| `retention.enforcement_interval` | float | 1.0 | — | Hours; must be ≥0.1 |
| `mirror_recovery.interval` | int | 5 | — | Minutes; must be ≥1 |
| `mirror_recovery.rate` | float | 0.05 | — | Fractional; must be 0.01–1.0 |
| `downloads.max_retries` | int | 3 | — | Must be ≥1 |
| `downloads.timeout` | int | 30 | — | Seconds; must be ≥1 |
| `downloads.temp_dir` | string | "" | — | Empty string uses OS temp directory |

## Configuration loading behavior

- When `trinity-cache --config /path/to/config.yaml` is called, the YAML file is parsed and defaults are applied to any missing fields.
- When `trinity-cache` is called with no `--config` argument, all defaults are used.
- Missing numeric fields (0 or unset) receive their defaults automatically.
- CLI flag `--port` overrides `server.port` from config.

## Mirror behavior and weight recovery

Trinity-cache uses **effective weight** per mirror, derived from `weight` in the config. After a mirror is selected for download, its effective weight recovers gradually based on `mirror_recovery.interval` and `mirror_recovery.rate`. Higher `rate` (closer to 1.0) means faster recovery toward the base weight.

## Retention policy

The retention manager keeps only `retention.keep_versions` recent versions per package and removes older ones. The enforcement runs every `retention.enforcement_interval` hours. Versions are ordered lexicographically; newer versions (lexicographically greater) are kept.

## Logging

Set `log_level` to:
- `debug` — detailed internal state (development/troubleshooting)
- `info` — significant events and startup info (recommended for production)
- `warn` — recoverable issues
- `error` — critical failures

Logs are structured and printed to stdout via Go's `log/slog`.

## Production deployment example

### Systemd unit

Create `/etc/systemd/system/trinity-cache.service`:

```ini
[Unit]
Description=Trinity-cache package cache service
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=trinity
Group=trinity
WorkingDirectory=/var/lib/trinity-cache
ExecStart=/usr/local/bin/trinity-cache --config /etc/trinity-cache/config.yaml
Restart=on-failure
RestartSec=10s

# Resource limits
LimitNOFILE=65536
LimitNPROC=65536

# Graceful shutdown
KillMode=mixed
KillSignal=SIGTERM
TimeoutStopSec=30s

[Install]
WantedBy=multi-user.target
```

Then:

```bash
sudo systemctl daemon-reload
sudo systemctl enable trinity-cache
sudo systemctl start trinity-cache
sudo systemctl status trinity-cache
```

### Docker

```bash
docker run -d --name trinity-cache \
  -v /var/lib/trinity-cache:/var/lib/trinity-cache \
  -p 8080:8080 \
  trinity-cache:latest \
  --config /etc/trinity-cache/config.yaml
```

## Validation

The service validates configuration on startup:

- `storage_path` must be non-empty
- `concurrency` must be 1–10000
- At least one mirror must be configured
- All mirror URLs must be non-empty
- `log_level` (if set) must be one of the four allowed values

If validation fails, the service exits with error and will not start.

## Example production config

```yaml
concurrency: 16
storage_path: "/mnt/cache/trinity"
log_level: "info"

server:
  port: "8080"
  read_timeout: 60
  write_timeout: 60

mirrors:
  - url: "https://mirror.example.com/archlinux"
    weight: 2.0
    timeout: 60
  - url: "https://mirror-backup.example.com/archlinux"
    weight: 1.0
    timeout: 60

retention:
  keep_versions: 3
  enforcement_interval: 0.5

mirror_recovery:
  interval: 10
  rate: 0.1

downloads:
  max_retries: 5
  timeout: 60
  temp_dir: "/tmp/trinity-downloads"
```

## Health & monitoring endpoints

- `GET /health` — service health check
- `GET /api/v1/stats` — operational statistics
- `GET /metrics` — Prometheus metrics

Monitor for:
- High download failure rate
- Low cache hit rate
- Rapid disk usage growth
- Mirror connectivity issues

## Troubleshooting

**Service fails to start**
- Check `storage_path` is writable and has sufficient disk space
- Verify at least one mirror is configured and reachable
- Check `concurrency` is between 1 and 10000

**High cache miss rate**
- Increase `retention.keep_versions` to retain more versions
- Verify mirror connectivity and upstream availability

**Slow downloads**
- Increase `concurrency` (if system resources allow)
- Adjust mirror `weight` to prefer faster mirrors
- Increase `downloads.timeout` for slow/remote mirrors
