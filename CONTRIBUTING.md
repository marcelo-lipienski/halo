# Contributing to halo

Thank you for contributing to **halo**! Please read this guide before opening a pull request.

---

## Prerequisites

| Requirement | Version |
|---|---|
| Go | ≥ 1.26 (pre-release — see [go.dev/dl](https://go.dev/dl/)) |
| Docker | Engine or Docker Desktop (daemon must be running for integration scenarios) |
| golangci-lint | Latest stable (optional, for `make lint`) |

---

## Getting Started

```bash
# Clone the repository
git clone https://github.com/marcelo-lipienski/halo.git
cd halo

# Download dependencies
go mod download

# Build the binary (with version injection via ldflags)
make build

# Run the full test suite
make test
```

---

## Code Quality & Git Workflow

To maintain a clean and standard codebase, contributors must follow the core styling and quality rules defined in:
* [GEMINI.md](file:///home/catz/dev/halo/GEMINI.md) — Coding styles, linting commands, and Test-Driven Development (TDD) table-test constraints.
* [Git & Pull Request Workflow ADR](file:///home/catz/dev/halo/docs/adr/0007-git-pr-workflow.md) — Detailed instructions on branch names, commit formats, CI builds, merging, and release tagging.

---

## Building a Release Binary

```bash
# With version tag (set VERSION explicitly or let git describe resolve it)
VERSION=v0.2.4 make build

# Verify version is baked in
./halo version
```

---

## Reporting Issues

Please include:
1. The output of `halo version`.
2. A minimal `docker-compose.yml` and `.env` that reproduces the issue.
3. The full `halo check --verbose` output.
