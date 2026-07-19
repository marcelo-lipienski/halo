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

## Development Workflow

### Branching Strategy

- **Always** branch off `master`.
- Name feature branches descriptively: `feat/volume-health-check`, `fix/env-regex-default`, etc.
- Open a pull request back to `master` once all tests pass.

### Commit Style

- Write atomic commits with clear, imperative subject lines (≤ 72 chars):
  ```
  fix: respect io.EOF sentinel in isReadable
  feat: add summary line to text renderer output
  ```
- Group logically related changes into a single commit. Avoid `WIP` commits in PRs.
- **Do not** include `Co-authored-by` AI metadata trailers.

---

## Code Standards

This project enforces the standards defined in [`AI_GUIDELINES.md`](./AI_GUIDELINES.md):

- Run `make fmt` to check formatting (or `make fmt-fix` to apply it).
- Run `make vet` before every commit.
- Run `make lint` (requires `golangci-lint`) for deeper static analysis.

### Test-Driven Development

A new feature or bug fix is **not complete** without matching tests:

1. Write the test first (or alongside the code).
2. Use table-driven tests in `_test.go` files co-located with the package.
3. Add benchmark tests (`func Benchmark...`) for any new parsing or filesystem traversal logic.

```bash
# Run all tests
make test

# Run benchmarks
make bench
```

---

## Building a Release Binary

```bash
# With version tag (set VERSION explicitly or let git describe resolve it)
VERSION=v0.2.3 make build

# Verify version is baked in
./halo version
```

---

## Reporting Issues

Please include:

1. The output of `halo version`.
2. A minimal `docker-compose.yml` and `.env` that reproduces the issue.
3. The full `halo check --verbose` output.
