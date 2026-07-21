# 15. Volume Write Permission Auto-Fix & Diagnostic Verification

## Status
Accepted

## Context
In the volume diagnostics check, write permissions on bind-mount volumes are verified. However, two logic bugs exist:
1. If write permission checks fail but are successfully corrected by the auto-fix (`--fix`) routine, a success result is added, but the overall diagnostic check is still marked as failed because it only checks if the result slice grew (`len(results) > before`).
2. When a missing volume source is automatically created, the write check is invoked inside that block. However, if the write check fails or dry-runs, the overall suite execution is not marked as failed because `volumeCheckPassed` is never set to false in that scenario.

We need to fix the engine execution logic to properly report verification outcomes.

## Decision
1. **Refactor Write Verification Method Signature:**
   - Change `checkWritePermission` signature to return both the updated check results and a boolean representing whether the path is writable (either initially writable or successfully auto-fixed).
   - `func (e *Engine) checkWritePermission(...) ([]output.CheckResult, bool)`
2. **Propagate Writability Status:**
   - If `checkWritePermission` returns `false` for the writability status, mark the overall volume diagnostic run as failed (`volumeCheckPassed = false`).
   - Use this status in both the general volume checking iteration and the auto-create volume block to ensure consistent logic reporting.

## Consequences
- **Accurate Diagnostic Reporting:** Auto-fixed write lockouts no longer trigger false negative overall test suite failures.
- **Fail-Safe Missing Directory Write Audits:** If an auto-created directory fails write verification or is running under a dry run, the suite correctly reports check failures.
- **Cleaner Verification Flow:** Matches the readability checks structure and adheres to the code comment rules specified in ADR-0008.
