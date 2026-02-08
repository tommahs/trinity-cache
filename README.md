# Trinity-cache

Trinity-cache is a Go-based system for fetching, caching, and serving Arch Linux packages.

At its core, it downloads packages from Arch Linux mirrors using **concurrent HTTP downloads** and **dynamic mirror weighting** to distribute load fairly across mirrors. Beyond downloading, Trinity-cache maintains a local package cache, keeps recent versions, and serves packages on demand.

The project is designed to grow into a lightweight, intelligent package distribution layer rather than a single-purpose downloader.

---

## Project Scope

Trinity-cache aims to:

- Download the latest Arch Linux packages from upstream mirrors
- Cache packages locally
- Retain the **two most recent versions** of each package
- Serve cached packages to clients
- Fetch newer versions automatically when requested
- Distribute mirror usage dynamically to avoid overloading any single mirror

---

## Features

- ⚡ Concurrent package downloads over HTTP
- 🪞 Mirror-aware fetching with dynamic weight adjustment
- 🔄 Actively rotates mirrors by penalizing recently used ones
- 📦 Local package cache with version tracking
- 🕒 Keeps the two most recent versions of each package
- 📡 On-demand fetching when a newer version is requested
- 📄 YAML-based configuration
- 🚀 Written in Go for performance and simplicity

---

## Design Philosophy

Trinity-cache is built around three core principles:

1. **Efficiency**  
   Use concurrency and intelligent scheduling to maximize throughput.

2. **Fairness**  
   Prevent hammering individual mirrors by dynamically adjusting mirror priority after each use.

3. **Self-sufficiency**  
   Serve packages locally whenever possible and fetch upstream only when needed.

---

## How It Works (High-Level)

- Mirrors are defined with initial base weights.
- A scheduler selects mirrors for downloads based on their current effective weight.
- When a mirror is used, its weight is temporarily reduced to promote selection of other mirrors.
- Package metadata and versions are tracked locally.
- For each package:
  - The latest available version is downloaded if not present.
  - The two most recent versions are retained.
  - Older versions are removed.
- When a client requests a package:
  - If the requested version is cached, it is served locally.
  - If the requested version is newer than the cached version, Trinity-cache downloads it and updates the cache.

---

## Configuration

Trinity-cache uses a YAML configuration file.

## Configuration Schema

Trinity-cache uses a formal configuration schema with validation for all parameters.

### Configuration Options

| Key | Type | Required | Default | Description |
|-----|------|----------|---------|-------------|
| `concurrency` | integer | Optional | 8 | Maximum number of concurrent downloads. Must be between 1 and 10000. |
| `storage_path` | string | Required | - | File system path where cached packages are stored. |
| `log_level` | string | Optional | info | Logging verbosity level. One of: `debug`, `info`, `warn`, `error`. |
| `mirrors` | array | Required | - | List of mirror definitions (at least one required). |
| `mirrors[].url` | string | Required | - | Base URL of an Arch Linux mirror. |
| `mirrors[].weight` | float | Optional | 1.0 | Initial base weight for the mirror. Must be positive. This value is dynamically adjusted at runtime based on mirror usage. |

### Validation Rules

All configuration values are validated on load:

- **concurrency**: Must be a positive integer ≤ 10000
- **storage_path**: Cannot be empty; must be provided or uses default
- **log_level**: Must be one of `debug`, `info`, `warn`, or `error`; defaults to `info` if not specified
- **mirrors**: At least one mirror is required
- **mirrors[].url**: Cannot be empty for any mirror
- **mirrors[].weight**: Must be positive (> 0); defaults to 1.0 if not specified

### Example Configuration

```yaml
concurrency: 8                           # Use up to 8 concurrent connections
storage_path: "/var/lib/trinity-cache"   # Store packages here
log_level: "info"                        # Log level: debug, info, warn, error

mirrors:
  - url: "https://mirror1.archlinux.org"
    weight: 1.0

  - url: "https://mirror2.archlinux.org"
    weight: 1.0

  - url: "https://mirror3.archlinux.org"
    weight: 1.5                         # Higher initial weight
```

## Logging

Trinity-cache provides **structured logging** based on Go's standard `log/slog` library. The philosophy follows Go's pragmatic approach: simple, explicit, and focused on what matters.

### Log Levels

The `log_level` configuration option controls verbosity (default: "info"):

- **debug**: Detailed diagnostic information for troubleshooting.
- **info**: Application startup, significant operations, and notable events.
- **warn**: Recoverable issues and temporary failures.
- **error**: Operation failures and serious problems.

### Log Output

Trinity-cache logs events as structured text. Each log entry includes a timestamp, level, message, and relevant attributes:

```
time=2026-02-08T10:15:22Z level=INFO msg="Trinity-cache started" version=0.1.0 concurrency=8 storage=/var/lib/trinity-cache mirrors=3
time=2026-02-08T10:15:23Z level=ERROR msg="no mirrors configured"
```

### Logging Philosophy

Following Go's ethos:
- **Explicit errors**: Operations return errors; failures are not silently logged and swallowed.
- **Minimal logging**: Log significant events and failures, not every internal detail.
- **Standard library**: Uses Go's standard `log/slog` without external dependencies.
- **Pragmatic**: Logging serves debugging and operations; unnecessary verbosity is avoided.

### Logger API

The logger package provides simple functions for structured logging:

```go
// Configure log level during startup
logger.SetLevel(logger.ParseLevel(cfg.LogLevel))

// Log messages at different levels
logger.Debug("detailed diagnostic info", "key", value)
logger.Info("significant event", "operation", "cache_loaded")
logger.Warn("recoverable issue", "retry", 3)
logger.Error("operation failed", "error", err)

// Create a contextual logger with pre-set fields
ctxLogger := logger.With("component", "downloader", "mirror", mirrorURL)
ctxLogger.Info("download started")
```

**Functions:**
- `SetLevel(level)` — Configure the log level (call during startup after config loads)
- `ParseLevel(s string)` — Parse "debug", "info", "warn", "error" from config string
- `Debug(msg, args...)` — Log at debug level
- `Info(msg, args...)` — Log at info level
- `Warn(msg, args...)` — Log at warn level
- `Error(msg, args...)` — Log at error level
- `With(args...)` — Return a contextual logger with pre-set key-value pairs
Trinity-cache is intended to expose a local package-serving interface (e.g. HTTP).
Clients can request packages normally. Trinity-cache will:
1. Serve the package from cache if available.
2. Fetch the package from upstream mirrors if a newer version is required.
3. Update the local cache and enforce version retention rules.

## Usage

🚧 Trinity-cache is under active development.

Planned usage:
```
trinity-cache --config config.yaml
```

Local build and run (developer):

```bash
# build
go build ./...

# build and run docker image
make docker
docker run --rm trinity-cache:dev --version
```

# Why “Trinity-cache”?
The name reflects the three pillars of the project:
- Concurrent fetching
- Dynamic mirror scheduling
- Local package serving
Together, they form a balanced and adaptive downloader.

# Status
This project is experimental and under active development.
Interfaces, configuration, and behavior may change.

# License

MIT License
