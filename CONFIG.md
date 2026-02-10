# Trinity-Cache Configuration Guide

## Example Configuration File

Create a `trinity-cache.yaml` configuration file with the following structure:

```yaml
# Log level: debug, info, warn, error
log_level: info

# Local storage path for cached packages
storage_path: /var/cache/trinity

# Number of concurrent download workers
concurrency: 4

# HTTP server configuration
server:
  # Port to listen on
  port: :8080
  # Read timeout in seconds
  read_timeout: 30
  # Write timeout in seconds
  write_timeout: 30

# Mirror configuration
mirrors:
  # Primary mirror
  - url: https://mirror1.example.com/arch/packages
    weight: 1.0
    timeout: 30
  
  # Secondary mirror with lower priority
  - url: https://mirror2.example.com/arch/packages
    weight: 0.8
    timeout: 30
  
  # Tertiary mirror, lightweight
  - url: https://mirror3.example.com/arch/packages
    weight: 0.5
    timeout: 30

# Cache retention settings
retention:
  # Number of package versions to keep (minimum 1)
  keep_versions: 2
  # How often to enforce retention (in hours)
  enforcement_interval: 1

# Mirror weight recovery settings
mirror_recovery:
  # How often to attempt weight recovery (in minutes)
  interval: 5
  # Recovery rate: 0.05 = 5% increase per interval
  rate: 0.05

# Download settings
downloads:
  # Maximum number of retries per download
  max_retries: 3
  # Download timeout in seconds
  timeout: 30
  # Temporary directory for downloads (default: system temp)
  temp_dir: /tmp/trinity-cache
```

## Running Trinity-Cache

### Basic Usage

```bash
# Run with default configuration
trinity-cache

# Run with custom configuration file
trinity-cache --config /etc/trinity-cache/config.yaml

# Show version
trinity-cache --version

# Run with custom server port
trinity-cache --config config.yaml --port :9000
```

### Systemd Service Example

Create `/etc/systemd/system/trinity-cache.service`:

```ini
[Unit]
Description=Trinity Package Cache Service
After=network.target

[Service]
Type=simple
User=trinity
Group=trinity
WorkingDirectory=/var/lib/trinity
ExecStart=/usr/bin/trinity-cache --config /etc/trinity-cache/config.yaml
Restart=on-failure
RestartSec=10
StandardOutput=journal
StandardError=journal

# Graceful shutdown timeout
TimeoutStopSec=30

[Install]
WantedBy=multi-user.target
```

Then enable and start:

```bash
sudo systemctl enable trinity-cache
sudo systemctl start trinity-cache
sudo systemctl status trinity-cache
```

## API Endpoints

### GET /api/v1/packages/{name}/{version}
Serves a cached package file

**Response:**
- `200`: Package file (binary/octet-stream)
- `404`: Package not found in cache

### GET /api/v1/stats
Returns cache and download statistics

**Response:**
```json
{
  "timestamp": "2026-02-10T15:30:00Z",
  "downloads": {
    "total": 1024,
    "successful": 1000,
    "failed": 24,
    "bytes_total": 5242880,
    "avg_duration_ms": 250
  },
  "cache": {
    "hits": 750,
    "misses": 274,
    "hit_rate": 0.732,
    "packages": 150,
    "versions": 450
  },
  "mirrors": {
    "selections": 1024,
    "penalties": 48,
    "recoveries": 12
  },
  "retention": {
    "packages_removed": 25,
    "versions_removed": 75,
    "last_run": "2026-02-10T14:30:00Z"
  }
}
```

### GET /health
Server health check

**Response:**
```json
{
  "status": "ok",
  "running": true,
  "active_requests": 2,
  "timestamp": "2026-02-10T15:30:00Z"
}
```

### GET /metrics
Detailed metrics snapshot

**Response:** Same as `/api/v1/stats` with additional timing information

### POST /api/v1/fetch/{name}/{version}
On-demand fetch and cache of a package

**Parameters:**
- `{name}`: Package name (e.g., `linux`)
- `{version}`: Package version (e.g., `6.7.1-1`)

**Description:** 
Triggers an immediate download of a specific package version from configured mirrors if not already cached. This is useful for ensuring newer package versions are available before serving. The fetch respects mirror selection and weight penalties.

**Response (200 OK):**
```json
{
  "name": "linux",
  "version": "6.7.1-1",
  "size": 67108864,
  "checksum": "abcd1234...",
  "path": "/var/cache/trinity/linux/linux-6.7.1-1.pkg",
  "timestamp": "2026-02-10T15:30:00Z"
}
```

**Response (503 Service Unavailable):**
```json
{
  "error": "fetch manager not available"
}
```

**Response (500 Internal Server Error):**
```json
{
  "error": "fetch failed: network error"
}
```

**Example Usage:**
```bash
# Fetch a specific package version
curl -X POST http://localhost:8080/api/v1/fetch/linux/6.7.1-1

# Check cache before fetching (optional)
curl http://localhost:8080/api/v1/packages/linux/6.7.1-1
```

## Mirror Management

### How Mirror Selection Works

1. **Initial Selection**: Mirrors are selected based on effective weight
2. **Usage Penalty**: After use, a mirror's weight is reduced by ~50%
3. **Weight Recovery**: Weights gradually recover toward base values (~5% per 5 minutes)
4. **Load Balancing**: Lower weight = lower selection priority (avoids overloading)

### Weight Adjustment Formula

```
Score = EffectiveWeight × (1 + timeSinceLastUseBoost) / (1 + inFlightPenalty)
```

Where:
- `EffectiveWeight`: Current weight (reduced by penalties, increased by recovery)
- `timeSinceLastUseBoost`: Boost for unused mirrors (encourages rotation)
- `inFlightPenalty`: Penalty for mirrors with many concurrent downloads
## Cache Layout

Packages are stored with the following directory structure:

```
/var/cache/trinity/
├── package-a/
│   ├── package-a-1.0.pkg
│   ├── package-a-1.1.pkg
│   └── package-a-2.0.pkg
├── package-b/
│   ├── package-b-0.5.pkg
│   └── package-b-1.0.pkg
└── ...
```

### Retention Policy

The retention manager automatically:
- Keeps the 2 most recent versions of each package
- Removes older versions based on lexicographic ordering
- Runs periodically (default: every hour)
- Logs removal operations for audit trail

## Performance Tuning

### Download Performance

```yaml
# Increase concurrency for better throughput on fast networks
concurrency: 8

# Reduce timeout for flaky mirrors
mirrors:
  - url: https://slow-mirror.example.com
    timeout: 60  # Longer timeout for slow/remote mirrors
```

### Cache Hit Rate

```yaml
# Keep more versions for frequently accessed packages
retention:
  keep_versions: 3  # Instead of 2

# Adjust mirror recovery for stable mirrors
mirror_recovery:
  interval: 10  # More frequent recovery
  rate: 0.1     # Faster recovery
```

## Monitoring

### Via Metrics Endpoint

```bash
# Get current metrics
curl http://localhost:8080/metrics | jq

# Get just cache stats
curl http://localhost:8080/api/v1/stats | jq '.cache'

# Monitor in real-time
watch -n 1 'curl -s http://localhost:8080/health | jq'
```

### Via Systemd Logs

```bash
# Follow service logs
journalctl -u trinity-cache -f

# Get logs since service started
journalctl -u trinity-cache -S today

# Get only errors
journalctl -u trinity-cache -p err
```

## Troubleshooting

### High Cache Miss Rate

```yaml
# Symptoms: cache.hit_rate < 0.5

# Solution 1: Increase retention
retention:
  keep_versions: 5

# Solution 2: Check mirror connectivity
# - Verify mirrors are accessible
# - Check network connectivity
```

### Download Failures

```yaml
# Symptoms: downloads.failed > downloads.successful

# Solution 1: Increase retry count
downloads:
  max_retries: 5

# Solution 2: Increase timeout
downloads:
  timeout: 60

# Solution 3: Add more mirrors
mirrors:
  - url: https://backup-mirror.example.com
    weight: 0.5
```

### High CPU Usage

```yaml
# Symptoms: High CPU utilization

# Solution: Reduce concurrency
concurrency: 2  # Instead of 4 or 8
```

### Disk Space Issues

```yaml
# Symptoms: Disk full or running out of space

# Solution 1: Increase cleanup frequency
retention:
  enforcement_interval: 0.5  # Every 30 minutes instead of hourly

# Solution 2: Reduce retention count
retention:
  keep_versions: 1  # Keep only latest version
```

## Security Considerations

1. **File Permissions**: Cache directory should be readable by the trinity-cache user
   ```bash
   chmod 755 /var/cache/trinity
   chown trinity:trinity /var/cache/trinity
   ```

2. **Network Security**: Use HTTPS mirrors when possible
   ```yaml
   mirrors:
     - url: https://secure-mirror.example.com  # Preferred
     - url: http://mirror.example.com          # Fallback only
   ```

3. **Access Control**: Restrict HTTP server access
   ```dockerfile
   # Example with firewall
   ufw allow from 192.168.1.0/24 to any port 8080
   ```

## Integration with CI/CD

### GitLab CI Example

```yaml
.cache_template:
  before_script:
    - curl -o /tmp/app.pkg http://trinity-cache:8080/api/v1/packages/app/latest
    - tar xf /tmp/app.pkg
```

### GitHub Actions Example

```yaml
- name: Download from cache
  run: |
    curl -o app.pkg \
      http://trinity-cache:8080/api/v1/packages/app/${{ matrix.version }}
```

## Maintenance

### Regular Tasks

- Monitor metrics endpoint weekly
- Review logs for errors
- Verify mirror connectivity
- Check disk usage

### Cleanup

```bash
# Manual cleanup (usually automatic)
curl -X POST http://localhost:8080/api/v1/admin/cleanup

# Verify cache integrity
curl http://localhost:8080/api/v1/admin/verify
```

## Advanced Configuration

### Custom Mirror Weights Based on Geography

```yaml
mirrors:
  # EU Mirror (closest to us)
  - url: https://eu-mirror.example.com
    weight: 1.0
  
  # US Mirror (fallback)
  - url: https://us-mirror.example.com
    weight: 0.7
  
  # Asia Mirror (slow for us)
  - url: https://asia-mirror.example.com
    weight: 0.3
```

### Different Retention for Different Package Types

```yaml
# Note: Current implementation uses single retention policy
# This is a planned feature for future versions
retention:
  policies:
    - pattern: "critical-*"
      keep_versions: 5  # Keep more versions of critical packages
    - pattern: "*"
      keep_versions: 2  # Default policy
```

---

For more information, visit: https://github.com/tommahs/trinity-cache
