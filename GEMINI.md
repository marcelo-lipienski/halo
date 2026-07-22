# halo AI Guidelines (GEMINI.md)

## 1. Stack & Standards
- Go ^1.26, Docker (daemon check), prioritize standard library.
- Idiomatic Go: propagate `context.Context`, wrap errors (`errors.Join`), AST-based expansion (`mvdan.cc/sh/v3`).

## 2. Testing & Quality
- Test-First (TDD): Table-driven tests, benchmark tests (`func Benchmark...`).
- Quality: `go fmt`, `golangci-lint` compliance, single responsibility.

## 3. Workflow & Documentation
- git/PR: Follow dedicated feature branching and remote PR merge workflow (see [ADR-0007](docs/adr/0007-git-pr-workflow.md)). No commit co-author tags.
- Docs & Code Analysis: Use the [ADR Index](docs/adr/README.md) for dynamic context loading; read only relevant ADRs. Avoid reading user-facing guides like `USAGE.md` as their rules are duplicated in the ADRs.
- Analysis & Reports: NEVER create long summary files or reports in `~/.gemini` or external folders. Whenever asked to analyze/summarize the codebase, document a bug, or suggest improvements, create clear, concise ADR-like markdown documents in `docs/adr/` following the [ADR-0001](docs/adr/0001-record-architecture-decisions.md) format.

## 4. Communication & Response Style
- Response Style: Always adhere to being strictly minimal. Skip preambles, conversational filler, and verbose step-by-step summaries. Focus purely on required code changes, tool calls, and terse execution notes.
- Planning: Skip heavy planning artifacts unless explicitly instructed otherwise. For large tasks, provide a high-level task outline first and then prompt for user input.
