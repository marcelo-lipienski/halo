# Architectural Decision Records (ADRs)

This directory contains records of architectural decisions for **halo**.

Before starting a task, search this index or check the corresponding ADR to load only the relevant context into your execution window.

## Active ADRs

| ID | Title | Scope / Topics | File Link |
| :--- | :--- | :--- | :--- |
| `0001` | [Record Architecture Decisions](/docs/adr/0001-record-architecture-decisions.md) | Standard for writing and using ADRs | [0001-record-architecture-decisions.md](/docs/adr/0001-record-architecture-decisions.md) |
| `0002` | [CLI Interface & Exit Boundaries](/docs/adr/0002-cli-exit-boundaries.md) | CLI commands, flags, global settings, exit codes | [0002-cli-exit-boundaries.md](/docs/adr/0002-cli-exit-boundaries.md) |
| `0003` | [AST & Config Parsing](/docs/adr/0003-ast-config-parsing.md) | `.env` parsing, YAML unmarshaling, AST expansion | [0003-ast-config-parsing.md](/docs/adr/0003-ast-config-parsing.md) |
| `0004` | [Concurrent Diagnostic Engine Lifecycle](/docs/adr/0004-concurrent-diagnostic-lifecycle.md) | Concurrency, Context timeouts, scheduling | [0004-concurrent-diagnostic-lifecycle.md](/docs/adr/0004-concurrent-diagnostic-lifecycle.md) |
| `0005` | [Volume & Permission Mitigation](/docs/adr/0005-volume-permission-mitigation.md) | File permissions, secrets, configs, auto-fix, dry-run | [0005-volume-permission-mitigation.md](/docs/adr/0005-volume-permission-mitigation.md) |
| `0006` | [Docker API & Graceful Degradation](/docs/adr/0006-docker-api-graceful-degradation.md) | Docker daemon detection, socket connection, ports check | [0006-docker-api-graceful-degradation.md](/docs/adr/0006-docker-api-graceful-degradation.md) |
| `0007` | [Git & Pull Request Workflow](/docs/adr/0007-git-pr-workflow.md) | Feature branching, commit formatting, wait-for-actions, PR merging | [0007-git-pr-workflow.md](/docs/adr/0007-git-pr-workflow.md) |
| `0008` | [Docstring Hygiene & Comment Standards](/docs/adr/0008-docstring-hygiene-standards.md) | Code commenting, comment conciseness, ADR referencing | [0008-docstring-hygiene-standards.md](/docs/adr/0008-docstring-hygiene-standards.md) |


