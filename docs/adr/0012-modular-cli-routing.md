# 12. Modular CLI Routing

## Status
Accepted

## Context
The root entrypoint [main.go](/main.go) compiles command routing, flag registration, rendering, filesystem monitoring, and implementation logic for 6 subcommands in a single monolithic 800+ lines file. This creates massive token overhead when AI agents inspect command behaviors or add new CLI flags.

## Decision
1. **Routing and Execution Segregation:**
   - Keep [main.go](/main.go) strictly for CLI definition, parsing root command flags, global initialization (e.g., config parsing, Docker client building), and printing the application version.
   - Separate execution logic for each subcommand (`check`, `doctor`, `init`, `snapshot`, `diff`) into independent files in the root folder under the same package `main`.
2. **Implementation Selection:**
   - Adopt Option A: Move subcommand execution handlers to distinct files in the root folder (`main_check.go`, `main_doctor.go`, `main_init.go`, `main_snapshot.go`). This retains package boundaries without requiring circular imports or global state refactoring.

## Consequences
- **Context Isolation:** AI agents modifying the behavior of a specific command (e.g., `doctor`) only need to load that specific command file instead of the 800+ line `main.go`.
- **Improved Code Maintainability:** Easier testing of individual command handlers without dealing with root Cobra commands and global streams.
