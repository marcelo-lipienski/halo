# 18. Standardized Docker Daemon Ping Timeouts

## Status
Proposed

## Context
Docker daemon health checks across `doctor/doctor.go`, `diagnostics/engine.go`, and `snapshot/snapshot.go` use inconsistent context timeouts (ranging from unbudgeted calls to 5s socket timeouts) and duplicate warning string formats when the Docker socket is offline or unresponsive.

## Decision
1. Standardize all Docker daemon ping context timeouts to 2 seconds across `doctor.RunDoctor`, `Engine.Run`, and `snapshot.CreateSnapshot`.
2. Ensure graceful degradation to warning state without blocking command execution when the daemon is unreachable.

## Consequences
- **Predictability:** Guarantees uniform 2-second fallback latency when Docker daemon is offline across all subcommands.
- **Consistency:** Unifies offline warning output across `doctor`, `check`, and `snapshot` subcommands.
