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
# Trinity-cache

Trinity-cache is a focused, production-oriented service that fetches, caches, and serves Arch Linux packages from upstream mirrors with concurrency, mirror-aware scheduling, and simple retention rules.

This repository contains a Go implementation intended for deployment behind a reverse proxy or load balancer. It is optimized for reliability and observability in production environments.

Highlights:
- Concurrent HTTP downloads with a worker pool
- Dynamic mirror weighting and temporary penalization after selection
- Local filesystem cache with configurable retention (defaults to keeping 2 most recent versions)
- HTTP endpoints compatible with pacman and operational APIs for metrics and health

---

**Quick Links**

- Configuration: [CONFIG.md](CONFIG.md)
- Service entrypoint: [cmd/trinity-cache/main.go](cmd/trinity-cache/main.go)
- Cache implementation: [internal/cache](internal/cache)

---

**Status**: Stable prototype — ready for production evaluation. Use conservative defaults for `concurrency` and `storage_path` in production.

## Production Checklist

- Set `storage_path` to a persistent volume with sufficient disk space
- Run behind a TLS reverse proxy (Nginx, Caddy, Traefik) for public access
- Configure systemd for automatic restart and graceful shutdown
- Enable monitoring (`/metrics`, Prometheus) and structured logging
- Ensure regular backups of important package artifacts if needed

## Quickstart

Build locally:

```bash
go build ./...
./trinity-cache --config /etc/trinity-cache/config.yaml
```

Run via Docker (example):

```bash
make docker
docker run --rm -v /var/lib/trinity-cache:/var/lib/trinity-cache -p 8080:8080 trinity-cache:latest --config /etc/trinity-cache/config.yaml
```

Systemd unit example: see [CONFIG.md](CONFIG.md#systemd-service-example)

## Configuration

Trinity-cache loads a YAML file described in `CONFIG.md`. Key operational knobs include:

- `concurrency` — number of parallel downloads (default: 8)
- `storage_path` — path to the filesystem cache (required in production)
- `mirrors` — list of mirror URLs and base `weight` values
- `retention.keep_versions` — how many versions to keep (default: 2)

Always validate your configuration with `trinity-cache --config /path/to/config.yaml --validate` (CLI flag planned).

## Observability

- Health: `/health`
- Operational stats: `/api/v1/stats`
- Prometheus metrics: `/metrics` (if enabled)

Instrument monitoring for cache hit-rate, download success/failure, mirror selection metrics, and retention operations.

## Security and Deployment Guidance

- Run behind TLS; terminate TLS at a reverse proxy
- Run the service as an unprivileged user and limit access to `storage_path`
- Restrict network egress to known mirror endpoints when possible
- Use systemd resource limits (CPU, memory) to protect host

## Retention & Mirror Behavior

- Default retention: keep two most recent versions per package
- Mirror selection uses effective weights and penalizes a mirror after it is used; weights recover over time

## Contributing

Contributions are welcome. Please open issues for bugs or feature requests. For code changes, open a PR with tests for behavioral changes (mirror selection, retention, download logic).

## License

MIT License

