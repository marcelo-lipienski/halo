# Changelog

All notable changes to **halo** are documented in this file.

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Versions follow [Semantic Versioning](https://semver.org/).

---

## [Unreleased]

### Added
- Image Tag Security Audit: scans compose service image tags and warns on mutable/unlocked tags (like `:latest`, `:dev`, `:staging`, or implicitly using `:latest` by omitting tags) to ensure reproducible and safe environments.

## [1.3.0] — 2026-07-20

### Added
- `halo doctor` command: system-level prerequisites scanner that evaluates Docker CLI/Compose v2 installation, Docker Engine version, required CLI tools in `$PATH` (`git`, `make`, `docker`), system memory availability vs limits declared in compose files, and free disk space.

## [1.2.0] — 2026-07-20

### Added
- `.env.example` drift detection: new Environmental Alignment check that compares `.env` against `.env.example`. Reports missing keys as failures (with a hint to run `halo init`) and undeclared keys as warnings. Skipped automatically when `.env.example` does not exist.

## [1.1.0] — 2026-07-20

### Added
- `halo init`: smart `.env` merge command. Copies `.env.example` to `.env` if it does not exist, or merges missing keys into an existing `.env` without overwriting current values. Flags placeholder values that still need to be filled. Supports `--dry-run`.

## [1.0.0] — 2026-07-20

### Added
- Native Windows permission fixes using `icacls` (Modify permission grants) and Windows-specific diagnostic mitigations.
- Service-level `secrets` and `configs` declarations mapping validation check.
- High-coverage, in-process CLI test harness (substantially reducing test runtime and boosting coverage to >80%).

### Fixed
- Graceful degradation when the Docker daemon is offline/unreachable: network reachability checks are downgraded to warnings instead of hard system failures, allowing offline diagnostics to complete.

---

*For older releases (v0.3.0 and prior), see the [Changelog Archive](docs/CHANGELOG_ARCHIVE.md).*

[Unreleased]: https://github.com/marcelo-lipienski/halo/compare/v1.3.0...HEAD
[1.3.0]: https://github.com/marcelo-lipienski/halo/compare/v1.2.0...v1.3.0
[1.2.0]: https://github.com/marcelo-lipienski/halo/compare/v1.1.0...v1.2.0
[1.1.0]: https://github.com/marcelo-lipienski/halo/compare/v1.0.0...v1.1.0
[1.0.0]: https://github.com/marcelo-lipienski/halo/compare/v0.3.0...v1.0.0
