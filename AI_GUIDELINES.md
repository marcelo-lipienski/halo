# AI Development Guidelines for halo

This document defines the strict engineering standards, architectural boundaries, and testing patterns for **halo**. Any AI agent modifying or expanding this codebase must adhere completely to these rules.

---

## 1. Project Stack & Environment
* **Language:** Go (Golang)
* **Minimum Version:** ^1.26
* **Domain:** High-performance, low-overhead CLI tooling for local Docker and development environment diagnostics.
* **Dependencies:** Prioritize the Go standard library. Avoid external dependencies unless strictly necessary for core functionality (e.g., Docker API client).

---

## 2. Core Engineering Principles
* **Performance-First:** Code must be highly optimized for fast CLI execution. Minimize allocations and avoid unnecessary disk I/O.
* **Graceful Degradation:** The tool must never panic when encountering unreadable configurations or broken local permissions. Handle all failures defensively and bubble up context-rich errors.
* **Actionable Outputs:** Error messages and diagnostic failures must be explicitly formatted to give the end user immediate, actionable resolution steps.

---

## 3. Go 1.26 Idiomatic Standards
* **Concurrency Management:** Always propagate and respect `context.Context` for all diagnostic checks, network pings, and file system scans. Enforce hard timeouts to prevent execution hangs.
* **Error Wrapping:** Utilize native Go error handling. Leverage `errors.Join` when compiling multi-container reports to aggregate errors cleanly without losing type data.
* **Memory & Types:** Use structured slice and map initializations. Maintain strict type safety across all configuration parsing models (`.env`, `docker-compose.yml`).

---

## 4. Test-Driven Development (TDD) Mandate
* **Test-First Workflow:** Tests must be written before or alongside any new diagnostic feature or code modification. A feature is incomplete without matching tests.
* **Patterns:** Use standard Go `testing` package primitives. Prioritize **table-driven tests** to cleanly validate varied inputs, edge cases, and failure states.
* **Performance Validation:** Include benchmark tests (`func Benchmark...`) for core parsing engines and heavy file system traversals to guarantee execution remains in the millisecond range.

---

## 5. Code Style & Quality Guards
* All code must format perfectly with native `go fmt`.
* Adhere to strict linting rules defined by `golangci-lint` configurations.
* Keep functions tightly focused on a single responsibility. Return errors as the final value from functions and handle them immediately at the call site.
* **Git Commits:**
  * AI agents must follow clean git branching strategies: develop in dedicated feature branches, and merge them back to the main branch (`master`) only after tests and validations pass.
  * Group modifications into logical, atomic commits with descriptive commit messages.
  * AI agents must never add themselves as a co-author (e.g., using `Co-authored-by` metadata trailers) for git commits.

