# 2. CLI Interface & Exit Boundaries

## Status
Accepted

## Context
A predictable and automated command-line interface (CLI) is critical for local development setup validation and script integration (e.g. Git hooks, CI/CD).

## Decision
The command structure and flags for **halo** are defined as follows:

### Commands
- `halo check`: Executes the diagnostic suite once and exits. (This is the default command).
- `halo doctor`: Scans system-level host prerequisites (Docker, path, memory, disk).
- `halo init`: Merges `.env.example` keys into `.env` (or copies it verbatim).
- `halo version`: Outputs the CLI build version, runtime details, and git commit hash.

### Global Flags
- `--config-dir, -c <path>`: Directory with config files (Default: `.`).
- `--env-file, -e <path>`: Path to `.env` file (Default: `.env` in config directory).
- `--compose-file <path>`: Explicit docker-compose.yml files (can specify multiple times).
- `--fix`: Create missing directories/files and adjust permissions.
- `--dry-run`: Simulation mode for `--fix` validation.
- `--quiet, -q`: Suppress stdout, route system discovery/fatal issues to stderr.
- `--format, -f <text|json>`: Output format (Default: `text`).
- `--verbose, -v`: Enable debug/raw system logs in text output.
- `--interactive, -i`: Ask for confirmation before applying mitigations.
- `--watch, -w`: Live-monitor configurations and re-run check dynamically on updates.

### Exit Codes
The binary must return the following status codes:
- `0` (Healthy): Diagnostics parsed and all checks passed.
- `1` (System Failure): Missing core configurations, docker daemon down, or invalid flags.
- `2` (Environment Broken): Configurations parsed successfully but check validations failed.

## Consequences
- Enables deterministic embedding of `halo` in wrapper shell scripts and pre-commit hooks.
- Streamlines CI testing by suppressing text artifacts when `--quiet` is enabled.
