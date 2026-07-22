# 1. Record Architecture Decisions

## Status
Accepted

## Context
As the codebase evolves and AI agents assist in implementing tasks, there is a need to maintain clear, modular architectural documentation. Monolithic files consume excessive tokens, causing long reasoning paths and high API usage. We need a way to document system designs and historical decisions such that an agent only loads relevant sections for a specific task.

## Decision
We adopt the Architectural Decision Record (ADR) pattern.
1. All architectural decisions, codebase analyses, bug documentations, and suggested improvements must be recorded in `docs/adr/` (avoiding external summary files in `~/.gemini`). Each distinct bug, feature, or improvement MUST be stored in its own dedicated, atomic ADR document (never bundle multiple issues into a single monolithic ADR).
2. Each file must be named as `<four-digit-id>-<short-description>.md`.
3. Every ADR must follow a standard structure:
   - **Title/ID**
   - **Status** (Draft, Proposed, Accepted, Superceded, Rejected)
   - **Context** (Why this decision is being made, problem statement, bug details, or proposed improvement scope)
   - **Decision** (The specific engineering choices, fix implementation details, APIs, command flags, schemas)
   - **Consequences** (Tradeoffs, impacts on performance, testing, and other components)

## Consequences
- **Token Efficiency:** Agents can read individual ADRs (typically ~250 tokens each) instead of monolithic specification files.
- **Traceability:** Developers and AI agents can understand the history of decision-making over time.
- **Maintainability:** Refactoring a specific subsystem only requires updating or adding a single ADR.
