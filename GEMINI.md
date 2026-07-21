# halo AI Guidelines (GEMINI.md)

## 1. Stack & Standards
- Go ^1.26, Docker (daemon check), prioritize standard library.
- Idiomatic Go: propagate `context.Context`, wrap errors (`errors.Join`), AST-based expansion (`mvdan.cc/sh/v3`).

## 2. Testing & Quality
- Test-First (TDD): Table-driven tests, benchmark tests (`func Benchmark...`).
- Quality: `go fmt`, `golangci-lint` compliance, single responsibility.

## 3. Workflow & Documentation
- git/PR: Follow dedicated feature branching and remote PR merge workflow (see [ADR-0007](file:///home/catz/dev/halo/docs/adr/0007-git-pr-workflow.md)). No commit co-author tags.
- Docs: Use the [ADR Index](file:///home/catz/dev/halo/docs/adr/README.md) for dynamic context loading; read only relevant ADRs.
