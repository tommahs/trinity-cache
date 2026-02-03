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

Example:

```yaml
concurrency: 8
storage_path: "/var/lib/Trinity-cache"

mirrors:
  - url: "https://mirror1.archlinux.org"
    weight: 1.0

  - url: "https://mirror2.archlinux.org"
    weight: 1.0

  - url: "https://mirror3.archlinux.org"
    weight: 1.0
```

## Configuration Options
* concurrency
  Maximum number of concurrent downloads.
* mirrors[].url
  Base URL of an Arch Linux mirror.
* mirrors[].weight
  Initial base weight for the mirror.
  This value is dynamically adjusted at runtime based on mirror usage.

# Serving Packages
Trinity-cache is intended to expose a local package-serving interface (e.g. HTTP).
Clients can request packages normally. Trinity-cache will:
1. Serve the package from cache if available.
2. Fetch the package from upstream mirrors if a newer version is required.
3. Update the local cache and enforce version retention rules.

## Usage

🚧 Trinity-cache is under active development.

Planned usage:
```
Trinity-cache --config config.yaml
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
