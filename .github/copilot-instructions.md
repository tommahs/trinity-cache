# GitHub Copilot / AI Agent Instructions for Trinity-cache ⚡

**Purpose (quick):** Help agents understand this small, Go-based package caching project so they can make safe, high-value changes quickly.

## Where to start 🔍
- Read `README.md` — it contains the project architecture, goals, and a YAML config example.
- Note: the repository currently contains documentation; the Go sources and tests are expected but not present here. Search the repo root for `go.mod` or `main` if code is added later.

## Big-picture architecture (from repo docs) 🏗️
- Trinity-cache is a Go service that:
  - Concurrently downloads Arch Linux packages from configured mirrors.
  - Maintains a local cache and serves packages to clients (intended via HTTP).
  - Keeps the **two most recent versions** of each package and removes older ones.
  - Uses **dynamic mirror weighting**: mirror weights are adjusted at runtime to avoid overloading any single mirror.
- Key configuration keys (from `README.md`): `concurrency`, `storage_path`, and `mirrors[].{url,weight}`.

## Project-specific conventions & patterns ✅
- Config is YAML-based; treat `mirrors` as a prioritized list with floating-point `weight` values which are dynamically adjusted at runtime.
- Version retention rule is explicit: keep two most recent package versions — implement/remove in garbage collection or retention code paths.
- Mirror usage should *temporarily penalize* a mirror's effective weight after use; look for code that performs a penalization/decay operation when implementing related changes.

## Typical tasks & examples for agents 🛠️
- Adding features related to mirror scheduling:
  - Update the scheduler and include unit tests that assert weight changes after a download.
  - Reference the `mirrors` config keys when adding validation or schema checks.
- Implementing package retention/cleanup:
  - Ensure retention keeps 2 most recent versions; add a test with a mock package set and assert deletion of older versions.
- Configuration changes:
  - Update the YAML example in `README.md` and include a migration note in docs if config keys change.

## Developer workflows (what to try) ▶️
- Build (expected): `go build ./...` (project is described as Go-based in `README.md`).
- Run (expected CLI pattern): `Trinity-cache --config config.yaml` (documented in `README.md`).
- Tests: not present in repo; when adding tests, prefer simple unit tests and table-driven tests common in Go.

## Integration & external dependencies 🔗
- Upstream mirrors (Arch Linux mirrors) are external HTTP endpoints — treat network interactions as flaky: use retries and backoff in downloader codepaths.
- Storage is filesystem-based (`storage_path`) — be conservative about concurrency on the same paths and ensure consistent locking.

## Safety & review guidance ⚠️
- Preserve the retention rule (2 versions) unless the change explicitly updates README and adds tests documenting and exercising the new retention behavior.
- Any change to mirror selection must include tests that simulate multiple mirrors and assert distribution/penalization behavior.

## Files to reference when present 📁
- `README.md` — architecture, config keys, and behavior expectations (already present).
- `go.mod` and Go sources (when added) — expect primary service entrypoint (package `main`) and packages for scheduler, downloader, storage/cache, and HTTP serving.

---

If anything above is unclear or you want the instructions to include additional specifics (for example, preferred test frameworks, CI commands, or code layout conventions), tell me which parts to expand and I will iterate. ✅
