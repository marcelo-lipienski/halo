# Architectural Decision Records (ADRs)

This directory contains records of architectural decisions for **halo**.

Before starting a task, search this index or check the corresponding ADR to load only the relevant context into your execution window.

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
