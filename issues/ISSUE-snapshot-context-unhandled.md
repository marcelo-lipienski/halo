# ISSUE-snapshot-context-unhandled

## Context
`snapshot.CreateSnapshot` accepts a `context.Context` parameter to support timeouts and cancellations during environment state capture.

## Broken Behavior
The function ignores `ctx.Done()` during file scanning, SHA256 hashing, and service port inspection loops.

## Impact & Workaround
If context times out or is cancelled, long-running I/O operations continue executing until completion, causing CLI unresponsiveness. Workaround is increasing global CLI timeouts.
