# 9. Testing Standards & Mocking

## Status
Accepted

## Context
The diagnostic engine integrates with the host file system and the Docker daemon API. Real system calls in unit tests make tests slow, flaky, and dependent on environment-specific state (e.g. Docker being installed/running or permissions of the test runner). Furthermore, test files like `engine_test.go` are extremely large (over 70KB), consuming large amount of tokens if read entirely. We need clear, concise testing guidelines so agents can write test code without parsing all test files.

## Decision
We adopt the following testing guidelines for **halo**:
1. **Isolated Filesystem Tests:**
   - Always use Go's `t.TempDir()` to construct sandboxed test directories.
   - Dynamically create test assets (e.g. mock `.env`, `docker-compose.yml`, bind-mount directories) rather than committing test fixtures to git.
2. **Mocking Interfaces:**
   - Define minimal mock implementations (e.g. `mockDockerClient` wrapping `client.ContainerAPIClient`) to simulate Docker daemon states.
   - Use mock functions or struct fields (e.g. `listFunc`, `inspectFunc`) to control return values and simulate error scenarios.
3. **Table-Driven Tests:**
   - Use table-driven test configurations (`struct{ name string; ... }`) for complex check scenarios (e.g. shell expansion logic, version comparisons).
4. **Benchmark Testing:**
   - Implement benchmark tests using Go's `testing.B` for latency-critical components (such as config parsers and diagnostic executions) to guarantee the 100ms budget is maintained.
5. **Standard Output Capture:**
   - Capture `stdout` and `stderr` redirects by patching internal variables (e.g. `osExit`, `stdout`, `stderr`) rather than reading/writing to global descriptors.

## Consequences
- **High Test Reliability:** Zero network/socket dependencies in tests ensure stable, environment-agnostic CI runs.
- **Improved Token Conservation:** AI agents can follow these design rules and mock templates directly, skipping reading massive test source files.
- **Consistent Code Quality:** Enforces uniform table-driven and benchmark testing practices.
