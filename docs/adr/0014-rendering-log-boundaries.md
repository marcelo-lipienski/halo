# 14. UI & Logging Standard

## Status
Accepted

## Context
When running local checks and diagnostic commands, keeping output streams clean and predictable is crucial for CI pipelines, Git hook integrations, and automated tools. Standardizing stdout vs stderr routing prevents console pollution.

## Decision
1. **Stream Segregation:**
   - **Stdout:** Reserved exclusively for final, structured results (checklists or JSON format payloads).
   - **Stderr:** Used for all diagnostic progress indicators, execution status updates, warnings, and fatal exit failures.
2. **Interactive Prompts:**
   - Interactive prompt questions must be rendered to stderr; inputs are read from stdin.
3. **ANSI Styling / Colors:**
   - Enable ANSI colored checklists by default in text format mode.
   - Automatically suppress ANSI escape codes when:
     - Output is redirected/piped (checked via `isatty` or similar).
     - The `NO_COLOR` environment variable is set.
4. **Verbosity Controls:**
   - When `--quiet` is enabled, suppress all stdout; output only critical failure descriptions to stderr.
   - When `--verbose` is enabled, print trace/debug logs to stderr.

## Consequences
- **Predictable Scripting:** External wrappers (e.g. pre-commit hooks) can reliably pipe stdout to JSON/text parsers without filtering execution warnings.
- **Clean Console Output:** Ensures errors and warnings do not contaminate checklist results.
