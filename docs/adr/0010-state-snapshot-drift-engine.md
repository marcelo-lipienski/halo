# 10. State Snapshot & Drift Engine

## Status
Accepted

## Context
When diagnosing environments, developers frequently encounter "it worked yesterday" scenarios. Investigating these requires comparing the current state of files, ports, container images, and variables against a known good state. Committing full binary states to Git is impractical, and checking file details on every check is slow. We need a standardized snapshot capture and lightweight diff format to enable environmental state verification.

## Decision
We adopt the snapshot-drift architecture for historical analysis:
1. **Serialization Format:**
   - Snapshots are serialized as JSON documents.
   - Files are snapshotted using SHA256 hashes, file size, and modification timestamps.
   - Variables are grouped by source file path.
   - Ports record the service name, host/container port, protocol, occupancy status, and binding process details (PID, name).
   - Containers record ID, name, running state, status, image tag, and Image ID.
2. **Deterministic Diff Engine:**
   - Diffs must be computed by sorting all resource keys alphabetically (to prevent unstable JSON comparisons).
   - Diffs group changes into categorized slices: `Files`, `Variables`, `Ports`, and `Containers`.
   - Each diff entity uses standard transition labels: `added`, `removed`, `modified`, `status_changed`, `image_changed`, etc.
3. **Execution Boundary:**
   - The snapshot engine relies on files, active networking ports, and Docker API queries.
   - Snapshot tasks are run on-demand via `halo snapshot [file]` and diffs via `halo diff [file]`.

## Consequences
- **Quick Environmental Audit:** Allows developers to capture snapshots before/after running updates to pinpoint changes.
- **Traceability:** Prevents AI agents from reloading configuration state recursively; the JSON snapshot serves as a standard state dump.
- **Stable Comparison Output:** Deterministic key sorting guarantees identical outputs for identical diff runs.
