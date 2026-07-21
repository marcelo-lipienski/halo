# 13. Docker Compose Merge Semantics

## Status
Accepted

## Context
When loading multiple configurations using the `--compose-file` flag (defined in [ADR-0002](0002-cli-exit-boundaries.md)), `halo` merges them left-to-right. A clear, token-efficient specification of this merging behavior is required so that both developers and AI agents can predict configuration parsing logic without loading external documentation.

## Decision
We enforce the following deterministic merge boundaries when combining multiple Docker Compose YAML files:

| Field / Key | Merge Rule | Description |
| :--- | :--- | :--- |
| `image`, `container_name`, `entrypoint`, `command` | **Override** | The value in the rightmost file replaces previous values if non-empty. |
| `environment` | **Merge** | Latter files' environment keys override former files' keys; new keys are appended. |
| `ports` | **Append** | All ports from all compose files are union-combined. |
| `volumes` | **Override by target path** | If a latter file defines a volume mounting to the same container target path, it completely replaces the former volume entry. |
| `secrets`, `configs`, root-level `volumes` | **Override** | The latter definition completely replaces the former definition matching by name. |

## Consequences
- **Deterministic Configurations:** Prevents structural configuration parse mismatches during complex local setups.
- **Agent Focus:** AI agents can read this lightweight specification file directly instead of reading user-facing guides like [USAGE.md](/USAGE.md).
