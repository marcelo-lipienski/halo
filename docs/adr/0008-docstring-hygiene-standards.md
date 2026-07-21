# 8. Docstring Hygiene & Comment Standards

## Status
Accepted

## Context
AI coding assistants read the codebase source files (`*.go`) to perform refactoring, debugging, and code generation. Verbose, conversational comments, redundant documentation, or detailed essays inside code files consume excessive context tokens. We need a code comment style standard that maintains developer clarity while minimizing context token footprint.

## Decision
1. **Conciseness over Prose:** Use short, declarative, imperative sentences. Avoid conversational filler or explaining standard Go behaviors.
2. **Avoid Redundancy:** Do not write comments that restate what the code clearly expresses (e.g. `// set count to 0` above `count := 0`).
3. **Reference ADRs:** If a function or struct relies on complex architectural details, reference the ADR ID (e.g. `// See ADR-0004 for lifecycle rules`) instead of writing long inline explanations.
4. **Structured Annotations:** Use compact, structured keys for operational details when applicable (e.g. `// Timeout: 2s` or `// Thread-Safe: yes`).

## Consequences
- Reduces code file token overhead by 10-20% on read/write actions.
- Speeds up agent reasoning and file-scanning operations.
- Promotes clean, self-documenting code style for both human developers and agents.
