# 17. Diagnostics Interactive Stdin Guard and Prompt Safety

## Status
Accepted

## Context
In `diagnostics/volume_check.go`, `promptConfirm` reads user confirmation from `os.Stdin`. When diagnostic checks are executed concurrently across multiple goroutines, non-interactive execution contexts (e.g. CI/CD pipelines or piped input) can cause `fmt.Scanln` to block indefinitely or race across concurrent check routines.

## Decision
1. Guard `promptConfirm` in [volume_check.go](/diagnostics/volume_check.go#L21-L38) by checking character device status (`os.ModeCharDevice`).
2. If `os.Stdin` is not a character device TTY or if context cancellation occurs, return `false` immediately without blocking on user input.

## Consequences
- **Concurrency Safety:** Prevents race conditions and stdin read contention across concurrent diagnostic workers.
- **Automation:** Ensures automated non-interactive CLI runs default safely without hanging.
