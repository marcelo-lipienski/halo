# 3. AST & Config Parsing

## Status
Accepted

## Context
Docker Compose configurations often reference environment variables with complex shell expansion rules (e.g. `${VAR:-default}` or `${VAR:?error}`). Using raw regex patterns to parse and expand these expressions is fragile, error-prone, and fails to handle nested or shell-standard expansion boundaries.

## Decision
1. **Parser Engine:** Utilize Go AST-based parsing libraries (specifically `mvdan.cc/sh/v3/expand`) rather than standard regex packages to expand and substitute environment variables.
2. **File Discovery:**
   - Locate `.env` and `docker-compose.yml` (or `docker-compose.yaml`).
   - If multiple `--compose-file` flags are supplied, merge them left-to-right following Docker Compose specification rules.
3. **Parameter Expansion:**
   - Map out variables declared in `.env` or host env.
   - Resolve dynamic settings using AST-compliant rules, throwing specific errors if a required variable (`:?error`) is missing or empty.

## Consequences
- Complies with standard POSIX shell variable expansion specs.
- Prevents structural configuration parse failures due to regex edge cases.
- Minor overhead during parsing phase (negligible in standard development workflows).
