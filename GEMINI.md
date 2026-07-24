# halo AI Guidelines (GEMINI.md)

## 1. Stack & Standards
- Go ^1.26, Docker (daemon check), prioritize standard library.
- Idiomatic Go: propagate `context.Context`, wrap errors (`errors.Join`), AST-based expansion (`mvdan.cc/sh/v3`).
- Defensive Engine execution: Always check for nil `*config.ComposeConfig` in diagnostic handlers before accessing services, secrets, or configs.
- Context Cancellation: Check `ctx.Err()` at loop boundaries during file scanning, hashing, and service port inspection to ensure prompt cancellation in long-running I/O operations.
- Multi-stage Dockerfiles: Pre-scan stage aliases (`AS <alias>`) in a first pass before auditing base image mutability to handle forward and out-of-order stage references.
- OS Test Hooks: Wrap OS-dependent command calls (`icacls`, `chmod`, `lsof`, `ss`, `netstat`) in package-level function variables (`fixPermissionsFunc`, `getOccupyingProcessFunc`) to enable deterministic unit testing.
- Auto-Mitigation Engine: Map dedicated remediation commands (`halo fix`) directly to `Engine.AutoFix` with persistent `--dry-run` (`-d`) preview capabilities.
- Standard Library Helpers: Prefer standard library functions (`strings.Contains`, `slices.Contains`) over custom slice/string iteration helpers in test code.

## 2. Testing & Quality
- Test-First (TDD): Table-driven tests, benchmark tests (`func Benchmark...`).
- Quality: `go fmt`, `golangci-lint` compliance, single responsibility.

## 3. Workflow & Documentation
- git/PR: Follow dedicated feature branching and remote PR merge workflow (see [ADR-0007](docs/adr/0007-git-pr-workflow.md)). No commit co-author tags.
- Docs & Code Analysis: Use the [ADR Index](docs/adr/README.md) for dynamic context loading; read only relevant ADRs. Avoid reading user-facing guides like `USAGE.md` as their rules are duplicated in the ADRs.
- Analysis & Reports: NEVER create long summary files or reports in `~/.gemini` or external folders. Whenever asked to analyze/summarize the codebase, document bugs, or suggest improvements, create clear, concise ADR-like markdown documents in `docs/adr/` following the [ADR-0001](docs/adr/0001-record-architecture-decisions.md) format, ensuring each distinct bug or improvement is stored in its own dedicated, atomic ADR.

## 4. Active Known Issues & Debt
- **Scope & Location:** Use `/issues/` ONLY for *unresolved, active bugs* or *in-progress debt*. Never retain files for resolved items.
- **Lifecycle (Create -> Resolve -> Delete):**
  - **Create:** On identifying an unfixed bug/debt, write `/issues/ISSUE-<short-name>.md`. Check active issue files when working on adjacent features.
  - **Resolve:** Upon fixing, write a covering regression test, add any 1-line architectural lesson learned to `GEMINI.md` or `CONVENTIONS.md`, and DELETE the issue file immediately.
- **Format Constraint:** Max 100–150 words per issue file: Context, Broken Behavior, and Impact/Workaround.

## 5. Communication & Response Style
- Response Style: Always adhere to being strictly minimal. Skip preambles, conversational filler, and verbose step-by-step summaries. Focus purely on required code changes, tool calls, and terse execution notes.
- Planning: Skip heavy planning artifacts unless explicitly instructed otherwise. For large tasks, provide a high-level task outline first and then prompt for user input.

