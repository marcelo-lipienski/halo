# 19. Test Suite Expansion for System Resource and Snapshot Edge Cases

## Status
Proposed

## Context
Unit test coverage in `doctor` (70.4%) and `snapshot` (77.2%) leaves several critical boundary conditions untested, including cross-platform memory/disk parsing edge cases, port range diff matching (`8000-8005`), and container state transition diff rendering.

## Decision
1. Introduce table-driven unit test cases in [doctor_test.go](/doctor/doctor_test.go) for byte parsing and memory limit calculations.
2. Expand [snapshot_test.go](/snapshot/snapshot_test.go) to validate port range expansion diffs and container state diff transitions.

## Consequences
- **Coverage Goal:** Elevates unit test coverage in `doctor` and `snapshot` packages above 85%.
- **Regression Prevention:** Ensures boundary logic in resource calculation and diff engines remains stable under refactoring.
