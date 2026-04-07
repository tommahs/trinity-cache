# Trinity-cache
[![CI](https://img.shields.io/github/actions/workflow/status/tommahs/trinity-cache/ci.yml?branch=main&label=CI&style=flat-square)](https://github.com/tommahs/trinity-cache/actions)
[![Release](https://img.shields.io/github/v/release/tommahs/trinity-cache?style=flat-square)](https://github.com/tommahs/trinity-cache/releases)
[![Go](https://img.shields.io/badge/go-1.25-blue?style=flat-square)](https://golang.org)
[![License: MIT](https://img.shields.io/badge/license-MIT-green?style=flat-square)](LICENSE)

**Trinity-cache** is a high-performance Go service for fetching, caching, and serving Arch Linux packages from upstream mirrors.

It combines intelligent mirror scheduling, concurrent downloads, and controlled retention into a lightweight package distribution layer designed for real-world infrastructure.

## Why Trinity-cache?

If you run multiple Arch Linux systems — whether in CI, labs, clusters, or enterprise environments — you’ve likely encountered:

- Repeated downloads of identical packages
- Uneven upstream mirror performance
- Rate limited or slow mirror responses
- Lack of visibility into package distribution
- Uncontrolled disk growth from naive caching

Trinity-cache sits between your infrastructure and upstream mirrors, acting as:

- A performance accelerator  
- A fair mirror load distributor  
- A controlled local package cache  
- An observable package gateway 

It is not just a downloader — it is a smart edge for package distribution.

## Core Capabilities

### Concurrent Download Engine
High-performance worker pool for parallel HTTP downloads.

### Intelligent Mirror Scheduling
- Mirrors have configurable base weights  
- Effective weights dynamically adjust  
- Recently used mirrors are temporarily penalized  
- Weights recover over time  

This prevents repeatedly hammering a single mirror and distributes load fairly.

### Smart Retention
- Default: keep **2 most recent versions**
- Configurable retention policy
- Automatic cleanup after successful updates

Predictable disk usage without sacrificing rollback safety.

### On-Demand Fetching
If a requested package version is missing, Trinity-cache fetches it automatically.

### Observability Built In
- `/health` — service health
- `/healthz` — service health
- `/api/v1/stats` — operational metrics
- `/api/v1/metrics` — Prometheus-compatible metrics
- `/metrics` — Prometheus-compatible metrics

## Architecture Overview

Trinity-cache:

1. Selects a mirror using weighted scheduling
2. Downloads packages concurrently  
3. Stores artifacts in a local filesystem cache
4. Applies retention policies  
5. Serves packages via HTTP (pacman-compatible)  

Designed for deployment behind:

- Any reverse proxy or load balancer like nxing, Caddy, Traefik or HaProxy

## Quickstart
Choice between building the arch-independent binary and docker image

### Binary
Raw binary without configuration
```bash
make build
./bin/trinity-cache --config ./bin/trinity-cache.yaml
```

### Docker
```bash
make docker
docker run --rm \
  -v /var/lib/trinity-cache:/var/lib/trinity-cache \
  -p 8080:8080 \
  trinity-cache:latest \
  --config /etc/trinity-cache..yaml
```

Systemd example available in `CONFIG.md`.

## Configuration

YAML-based configuration (see `CONFIG.md`).

Key options:
- `concurrency` — parallel downloads (default: 8)
- `storage_path` — cache directory for storing packages
- `mirrors` — list of mirrors with base weights
- `retention.keep_versions` — versions to retain (default: 2)

## Production Deployment Guidance

- Use persistent storage for `storage_path`
- Run behind TLS reverse proxy
- Restrict filesystem permissions
- Limit network egress to trusted mirrors
- Apply CPU/memory limits
- Enable monitoring and structured logging

## Where Trinity-cache Fits

- Enterprise Arch Linux fleets
- CI/CD build pipelines
- University or lab environments
- Container clusters
- Edge compute nodes
- Self-hosted Arch mirrors

## Status

Stable prototype — ready for production evaluation.

Conservative defaults are recommended for concurrency and storage in initial deployments.

## Contributing

Issues and pull requests are welcome.

For behavioral changes (mirror selection, retention, download logic), please include tests.

## License
MIT License
