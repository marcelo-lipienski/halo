# halo

**halo** is a lightweight, blazing-fast CLI tool built to eliminate developer onboarding friction. It analyzes local `.env` and `docker-compose.yml` configurations to instantly verify container health, network connectivity, and file permissions—surfacing actionable fixes for broken local development environments in milliseconds.

---

## Key Features
* **Zero Guesswork Diagnostics:** Validates local system state against declared Docker topologies.
* **Environment Drift Detection:** Compares `.env` against `.env.example` to detect missing or undeclared keys.
* **Permission & Volume Auditing:** Inspects host-mounted paths, service secrets, and config declarations.
* **Conflicting Process Identification:** Identifies host applications causing port collisions.
* **Sensitive Data Redaction:** Automatically filters credentials and keys containing keywords like `SECRET`, `PASSWORD`, or `TOKEN`.
* **High-Performance Engine:** Built in Go using concurrent diagnostics, running full checks in milliseconds.

---

## Installation

Download and install the latest release via Go:

```bash
go install github.com/marcelo-lipienski/halo@latest
```

> **Note:** halo requires Go 1.26 or later and a running Docker daemon.

---

## Quick Start

### 1. Initialize Your Environment
If you are setting up a project for the first time, copy or merge keys from `.env.example` into `.env`:
```bash
halo init
```

### 2. Run Diagnostics
Validate your workspace configuration, port availability, and volume permissions:
```bash
halo check
```
*(Running `halo` with no subcommands defaults to `halo check`)*

### 3. Automatically Mitigate Issues
Repair file permissions and missing directory structures:
```bash
halo fix
```

---

## CLI Overview

| Command | Description |
| --- | --- |
| `halo check` | Run the full diagnostic suite (default command). |
| `halo fix` | Automatically mitigate configuration, file permission, and directory issues. |
| `halo init` | Initialize or update `.env` from `.env.example`. |
| `halo doctor` | Inspect host system prerequisites (Docker, memory, disk, PATH binaries). |
| `halo snapshot [file]` | Capture a baseline state snapshot of the local environment. |
| `halo diff [file]` | Compare current environment state against a saved snapshot. |
| `halo version` | Display binary version, build commit SHA, and Go runtime details. |

---

## Advanced Usage
For detailed instructions on advanced flags (such as `--watch`, `--fix`, `--dry-run`, `--interactive`, or `--format json`), multi-file compose merging rules, and system-level diagnostics using `halo doctor`, please refer to:
* [USAGE.md](USAGE.md)

---

## Development & Guidelines
* Automated agent workspace limits and style configurations are detailed in [GEMINI.md](GEMINI.md).
* Detailed architectural definitions are structured under the [ADR Index](docs/adr/README.md).
* Contribution guidelines are in [CONTRIBUTING.md](CONTRIBUTING.md).
