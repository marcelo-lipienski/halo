# Changelog

All notable changes to **halo** are documented in this file.

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Versions follow [Semantic Versioning](https://semver.org/).

---

## [Unreleased]

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

[Unreleased]: https://github.com/marcelo-lipienski/halo/compare/v0.1.0-beta.1...HEAD
[0.1.0-beta.1]: https://github.com/marcelo-lipienski/halo/releases/tag/v0.1.0-beta.1
