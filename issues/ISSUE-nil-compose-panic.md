# ISSUE-nil-compose-panic

## Context
When executing `halo check` on malformed or unparseable Docker Compose files, `config.ParseCompose` may return a `nil` `*ComposeConfig`.

## Broken Behavior
Calling `Engine.Run()` with `e.Compose == nil` triggers runtime nil pointer panics in `getSensitiveValues()`, `extractReferencedEnvVars()`, `checkNetworkAndPort()`, and `checkVolumeAndPermissions()`.

## Impact & Workaround
The CLI crashes ungracefully with a panic stack trace instead of outputting a clean `system_failure` report. Workaround is fixing compose file syntax before running diagnostics.
