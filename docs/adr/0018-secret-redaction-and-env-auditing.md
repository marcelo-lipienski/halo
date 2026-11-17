# 18. Secret Redaction and Custom Env Auditing

## Status
Accepted

## Context
Sensitive environment keys containing `PWD` (e.g. `DB_PWD`) and `CRED` (e.g. `USER_CRED`) were unredacted because the sensitive key regex omitted these tokens. Conversely, matching bare `PWD` on Unix systems matches the host environment variable `$PWD`, inadvertently redacting the current working directory path across reports. Additionally, non-standard env file names specified in Compose service `env_file` definitions (e.g. `secrets.env`) escaped Gitignore security audits.

## Decision
1. Update `sensitiveKeyRegex` in `diagnostics` and `snapshot` to capture `PASSWD`, `_PWD`, `^PWD_`, and `CRED`, while explicitly exempting bare `PWD` and `OLDPWD` host environment variables.
2. Expand `CheckGitignoreSecurity` to include all env files declared in Compose service `env_file` mappings and explicit `--env-file` flags.
3. Normalize path separators to forward slashes (`/`) via `filepath.ToSlash` when appending unignored entries to `.gitignore` on Windows.

## Consequences
- Prevents credential leakage in diagnostic outputs for common password/credential naming conventions.
- Ensures all declared environment files containing secrets are tracked and validated by Git security checks.
