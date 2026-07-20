# Changelog

All notable changes to **halo** are documented in this file.

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Versions follow [Semantic Versioning](https://semver.org/).

---

## [Unreleased]

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

## [0.3.0] — 2026-07-20

### Added
- `--watch` / `-w` flag: enables real-time configuration monitoring (including service-level env files, override files, and deletions) with clear-screen re-runs.
- `--interactive` / `-i` flag: thread-safe interactive mitigation prompts for confirming auto-fixes before execution.
- Host port collision process identification: resolves and prints PID and command name of processes occupying blocked ports.
- Sensitive data redaction: post-processes check results to filter and redact credentials/keys containing sensitive keywords.
- Service-level `.env` validation: supports parsing and validating env files defined per service.

## [0.2.4] — 2026-07-20

### Fixed
- Resolve linter warning QF1001 (staticcheck) by applying De Morgan's Law to the hex-character validator in `main.go`.

## [0.2.3] — 2026-07-19

### Fixed
- Clean version command: omit the `(unknown)` commit suffix if it cannot be resolved at runtime (e.g., when compiling via remote `go install`), and support parsing commit hashes from Go pseudo-versions.

## [0.2.2] — 2026-07-19

### Fixed
- Version fallback: dynamically resolve build version and commit SHA at runtime using Go build info metadata (`debug.ReadBuildInfo`) when compiled without explicit `-ldflags` (e.g. via `go install`).

## [0.2.0] — 2026-07-19

### Added
- `--quiet` / `-q` flag: suppresses standard output, routing system/discovery failures to `stderr` for CI scripting.
- `--dry-run` flag: simulates autofix mitigations (`--fix`) without performing write changes on the host filesystem.
- Docker Secrets and Docker Configs validation (existence, readability, auto-fixing missing files).
- Port scale protection: emits a warning if a mapped port range exceeds 64 ports.
- Mermaid-based Execution Pipeline Flow diagram in `README.md`.

### Fixed
- AST-compliant shell-expression expansion via `mvdan.cc/sh/v3` instead of standard regex patterns.
- Re-verify readability after applying permissions in `--fix` and log original file/directory permissions.
- Downgrade "No container found for service" check to a warning instead of a critical failure.
- Make service reachability check granular and symmetric per service (handling starting, unhealthy, and running states correctly).
- Self-exclusion for network port collisions: if a port is bound by a running container belonging to the same project and service, it is not flagged as a collision.
- Upgraded Go compiler/lint target in `golangci-lint` configuration to Go 1.26.

## [0.1.0] — 2026-07-18

### Fixed
- Skip POSIX directory permission assertions on Windows in diagnostics test.
- Resolve API deprecation, staticcheck, govet, and errcheck lint warnings.
- Resolve golangci-lint schema compliance warning.

## [0.1.0-beta.1] — 2026-07-18

### Added
- Initial public beta release of **halo**.
- `halo check` — full diagnostic suite: environmental alignment, port collision, and volume/permission checks.
- `halo version` — prints version, commit SHA, and Go runtime details.
- `--compose-file` flag (repeatable) for explicit compose file paths.
- `--env-file` / `-e` flag for explicit `.env` file path.
- `--config-dir` / `-c` flag to set the root directory for auto-discovery.
- `--format text|json` / `-f` for text (ANSI) or structured JSON output.
- `--verbose` / `-v` for additional debug context in text output.
- `--fix` for automatic mitigation of missing directories and permission issues.
- Auto-discovery of `docker-compose.yml`, `docker-compose.yaml`, and `docker-compose.override.*` files.
- Support for long-form and short-form Docker Compose port syntax (including port ranges).
- Support for bind-mount, named-volume, and anonymous-volume parsing.
- Windows drive-path volume handling.
- Secret and config file existence and readability checks.
- `docker compose` project name resolution via `COMPOSE_PROJECT_NAME` env var or directory name.
- Semantic exit codes: `0` healthy, `1` system failure, `2` environment broken.
- Summary line in text output (`N of M checks passed`).
- Makefile with `build`, `install`, `test`, `bench`, `vet`, `fmt`, `lint`, `clean` targets.
- GitHub Actions CI workflow.

### Fixed
- `docker-compose.yml` is now correctly preferred over `docker-compose.yaml` (matches Docker Compose's own precedence).
- Anonymous volumes are no longer incorrectly de-duplicated during compose file merging.
- `io.EOF` is now compared using `errors.Is` instead of a string literal.
- `--compose-file` now correctly supports multiple independent `--compose-file` invocations.
- Mitigation strings use `docker compose` (modern CLI) instead of the deprecated `docker-compose`.
- `exitWithSystemFailure` now respects the `--verbose` flag consistently.

[Unreleased]: https://github.com/marcelo-lipienski/halo/compare/v1.2.0...HEAD
[1.2.0]: https://github.com/marcelo-lipienski/halo/compare/v1.1.0...v1.2.0
[1.1.0]: https://github.com/marcelo-lipienski/halo/compare/v1.0.0...v1.1.0
[1.0.0]: https://github.com/marcelo-lipienski/halo/compare/v0.3.0...v1.0.0
[0.3.0]: https://github.com/marcelo-lipienski/halo/compare/v0.2.4...v0.3.0
[0.2.4]: https://github.com/marcelo-lipienski/halo/compare/v0.2.3...v0.2.4
[0.2.3]: https://github.com/marcelo-lipienski/halo/compare/v0.2.2...v0.2.3
[0.2.2]: https://github.com/marcelo-lipienski/halo/compare/v0.2.0...v0.2.2
[0.2.0]: https://github.com/marcelo-lipienski/halo/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/marcelo-lipienski/halo/compare/v0.1.0-beta.1...v0.1.0
[0.1.0-beta.1]: https://github.com/marcelo-lipienski/halo/releases/tag/v0.1.0-beta.1
