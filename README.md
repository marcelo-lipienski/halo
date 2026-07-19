# halo

**halo** is a lightweight, blazing-fast CLI tool built to eliminate developer onboarding friction. It analyzes local `.env` and `docker-compose.yml` configurations to instantly verify container health, network connectivity, and file permissions—surfacing actionable fixes for broken local development environments in milliseconds.

Instead of wasting engineering hours debugging mismatched environment configurations, locked storage volumes, or port collisions, running `halo` gives developers a definitive, production-grade diagnostic health check of their workspace.

---

## Key Features

* **Zero Guestwork Diagnostics:** Validates local system state against declared Docker topologies.
* **Environment Drift Detection:** Cross-references `.env` variables directly with active compose declarations to flag missing keys.
* **Permission & Volume Auditing:** Inspects host-mounted paths to catch write-permission lockouts before containers fail.
* **High-Performance Engine:** Built in Go ^1.26 using native tokenization, executing full suite checks concurrently in milliseconds.
* **Automation Ready:** Supports semantic exit codes and structured JSON output to easily integrate into local git hooks or setup scripts.

---

## Architectural Lifecycle

`halo` operates as a single, decoupled binary that safely evaluates your system state without side effects:

```
[1. Discovery] ──> [2. Configuration Parsing] ──> [3. Concurrent Checks] ──> [4. Actionable UI]
Locate local        Extract .env & Compose        Run Port, Volume, &      Render ANSI status
config files        AST structures natively       Env checks in parallel   or stream JSON
```

---

## Quick Start

### Installation
Download and install the latest release via Go:

```bash
go install github.com/marcelo-lipienski/halo@latest
```

> **Note:** halo requires Go 1.26 or later.

## Usage

Run the diagnostic check inside any project root containing a docker-compose.yml file:

```bash
halo check
```

For automation pipelines or custom initialization scripts, request structured JSON output:

```bash
halo check --format json
```

### Auto-Mitigation (Fixing Issues)

To automatically attempt creating missing directories or fixing incorrect file permissions, use the `--fix` flag:

```bash
halo check --fix
```

### Custom Configurations

To run diagnostics against specific configurations outside the current directory:

```bash
halo check --env-file /path/to/.env --compose-file /path/to/docker-compose.yml
```

### Exit Codes

`halo` returns standard semantic exit codes to safely embed into local developer automation workflows:

* `0`: **Healthy** — All configurations parsed and all diagnostic checks passed.
* `1`: **System Failure** — Configuration files are missing, flags are invalid, or the local Docker daemon is unreachable.
* `2`: **Environment Broken** — Configurations parsed successfully, but one or more localized development infrastructure checks failed.

---

## Development & Guidelines

This project enforces strict **Test-Driven Development (TDD)** and leverages modern Go ^1.26 concurrency primitives and error handling patterns. 

* Detailed architectural definitions can be found in `SPECIFICATION.md`.
* Automated agent workspace limits and style configurations are detailed in `AI_GUIDELINES.md`.
