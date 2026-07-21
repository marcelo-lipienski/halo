# Architectural Decision Records (ADRs)

This directory contains records of architectural decisions for **halo**.

Before starting a task, search this index or check the corresponding ADR to load only the relevant context into your execution window.

## Package-to-ADR Mapping

Before modifying any package code, load the corresponding ADR context:

| Package / Component | Related ADRs | Key Design Directives |
| :--- | :--- | :--- |
| `config` | [ADR-0003](0003-ast-config-parsing.md), [ADR-0013](0013-compose-merge-rules.md) | AST parsing via `mvdan.cc/sh/v3/expand`; deterministic override & merge precedence. |
| `diagnostics` | [ADR-0004](0004-concurrent-diagnostic-lifecycle.md), [ADR-0005](0005-volume-permission-mitigation.md), [ADR-0006](0006-docker-api-graceful-degradation.md), [ADR-0011](0011-security-redaction-boundaries.md) | Concurrently execution in 3 groups; fix permissions; warning fallbacks for Docker socket offline; redact credentials. |
| `doctor` | [ADR-0002](0002-cli-exit-boundaries.md) | Diagnose system constraints: resources, space, and CLI dependencies. |
| `snapshot` | [ADR-0010](0010-state-snapshot-drift-engine.md) | Collect system states into sorted JSON snapshots and compute differences. |
| `output` | [ADR-0014](0014-rendering-log-boundaries.md) | Restrict stdout to checklist/JSON outputs; pipe diagnostics, warnings, and errors to stderr. |
| `main.go` / Routing | [ADR-0002](0002-cli-exit-boundaries.md), [ADR-0012](0012-modular-cli-routing.md) | Subcommand exit boundaries; isolate execution handlers from `main.go`. |

## Active ADRs

| ID | Title | Scope / Topics | File Link |
| :--- | :--- | :--- | :--- |
| `0001` | [Record Architecture Decisions](0001-record-architecture-decisions.md) | Standard for writing and using ADRs | [0001-record-architecture-decisions.md](0001-record-architecture-decisions.md) |
| `0002` | [CLI Interface & Exit Boundaries](0002-cli-exit-boundaries.md) | CLI commands, flags, global settings, exit codes | [0002-cli-exit-boundaries.md](0002-cli-exit-boundaries.md) |
| `0003` | [AST & Config Parsing](0003-ast-config-parsing.md) | `.env` parsing, YAML unmarshaling, AST expansion | [0003-ast-config-parsing.md](0003-ast-config-parsing.md) |
| `0004` | [Concurrent Diagnostic Engine Lifecycle](0004-concurrent-diagnostic-lifecycle.md) | Concurrency, Context timeouts, scheduling | [0004-concurrent-diagnostic-lifecycle.md](0004-concurrent-diagnostic-lifecycle.md) |
| `0005` | [Volume & Permission Mitigation](0005-volume-permission-mitigation.md) | File permissions, secrets, configs, auto-fix, dry-run | [0005-volume-permission-mitigation.md](0005-volume-permission-mitigation.md) |
| `0006` | [Docker API & Graceful Degradation](0006-docker-api-graceful-degradation.md) | Docker daemon detection, socket connection, ports check | [0006-docker-api-graceful-degradation.md](0006-docker-api-graceful-degradation.md) |
| `0007` | [Git & Pull Request Workflow](0007-git-pr-workflow.md) | Feature branching, commit formatting, wait-for-actions, PR merging | [0007-git-pr-workflow.md](0007-git-pr-workflow.md) |
| `0008` | [Docstring Hygiene & Comment Standards](0008-docstring-hygiene-standards.md) | Code commenting, comment conciseness, ADR referencing | [0008-docstring-hygiene-standards.md](0008-docstring-hygiene-standards.md) |
| `0009` | [Testing Standards & Mocking](0009-testing-standards-mocking.md) | isolated testing, mock docker client, table-driven testing, benchmarks | [0009-testing-standards-mocking.md](0009-testing-standards-mocking.md) |
| `0010` | [State Snapshot & Drift Engine](0010-state-snapshot-drift-engine.md) | snapshot serialization, environment drift, port bindings, container state diffs | [0010-state-snapshot-drift-engine.md](0010-state-snapshot-drift-engine.md) |
| `0011` | [Security Redaction & Gitignore Boundaries](0011-security-redaction-boundaries.md) | key matching, credential redaction, untracked env checks, gitignore checks | [0011-security-redaction-boundaries.md](0011-security-redaction-boundaries.md) |
| `0012` | [Modular CLI Routing](0012-modular-cli-routing.md) | Subcommand separation, command handlers, code isolation | [0012-modular-cli-routing.md](0012-modular-cli-routing.md) |
| `0013` | [Docker Compose Merge Semantics](0013-compose-merge-rules.md) | Merging precedence, environment merge, port combine, volume overrides | [0013-compose-merge-rules.md](0013-compose-merge-rules.md) |
| `0014` | [UI & Logging Standard](0014-rendering-log-boundaries.md) | Stdout/stderr separation, ANSI color suppression, verbosity routing | [0014-rendering-log-boundaries.md](0014-rendering-log-boundaries.md) |

