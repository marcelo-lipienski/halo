# 17. Deterministic Snapshot Port Ordering

## Status
Accepted

## Context
When generating environment state snapshots, `CreateSnapshot` gathers ports while iterating over compose services stored in a Go map. Map iteration in Go is non-deterministic by design, causing the serialized `ports` JSON array in `.halo-snapshot.json` to shuffle between executions even when no environment changes have occurred.

## Decision
1. Explicitly sort `snap.Ports` by service name, host port, and protocol in `snapshot.CreateSnapshot` before returning the `EnvironmentSnapshot`.
2. Add deterministic regression test `TestCreateSnapshotPortsDeterministicOrder` in `snapshot/snapshot_test.go`.

## Consequences
- Guarantees byte-for-byte deterministic JSON serialization for `.halo-snapshot.json` files on disk.
- Prevents spurious diff artifacts when snapshot files are committed to Git or inspected with external tools.
