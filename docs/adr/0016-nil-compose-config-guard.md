# 16. Nil Compose Config Guard for Diagnostics Engine

## Status
Accepted

## Context
When `Engine.Run` is invoked on unparseable or missing Docker Compose files, `Engine.Compose` may be `nil`. Dereferencing `e.Compose.Services`, `Secrets`, or `Configs` leads to unhandled runtime panics.

## Decision
1. Add defensive `e.Compose != nil` guards across `diagnostics` package check methods (`getSensitiveValues`, `extractReferencedEnvVars`, `checkEnvironmentalAlignment`, `checkNetworkAndPort`, and `checkVolumeAndPermissions`).
2. Add regression test `TestEngineRunWithNilCompose` in `diagnostics/engine_test.go`.

## Consequences
- Prevents runtime panics when diagnostic engine runs without a valid Docker Compose configuration.
- Ensures graceful fallback execution for environmental and security checks.
