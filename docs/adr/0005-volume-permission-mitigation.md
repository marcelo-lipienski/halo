# 5. Volume & Permission Mitigation

## Status
Accepted

## Context
Incorrect file permissions or missing bind-mount host directories are a frequent source of local container startup failures. Developers need a way to automatically discover and rectify these issues safely.

## Decision
1. **Validation Scope:** Identify and audit all local bind-mount folders, Docker Secrets, and Docker Config files defined in Compose.
2. **Access Verification:** Assert that the executing host user has appropriate read/write privileges on host directories/files before Docker starts.
3. **Auto-Fix (`--fix`):**
   - If `--fix` is active, create missing directories.
   - Adjust permission bits on Unix using standard permissions (`chmod`) and on Windows using `icacls`.
   - Re-verify files/directories readability immediately after application.
4. **Interactive Mode (`--interactive`):** Prompt the developer before applying write changes or permission adjustments to the host system.
5. **Dry Run (`--dry-run`):** Emulate file creation or modification actions without altering the filesystem.

## Consequences
- Reduces onboarding friction by fixing permissions directly.
- `--dry-run` and `--interactive` prevent unintended modifications to sensitive directories.
- Auto-fix only applies to volume/permissions scope; it does not change `.env` keys or code files.
