# 11. Security Redaction & Gitignore Boundaries

## Status
Accepted

## Context
When running local diagnostics, halo processes sensitive information, including environment variables, database passwords, and private API keys. Logging or outputting these values in diagnostic results or reports poses a severe security risk if the output is pasted into public chats, issues, or agent logs. At the same time, we must verify that environment files containing credentials are never committed to Git or left untracked.

## Decision
We implement a unified security mitigation and redaction boundary:
1. **Sensitive Key Classification:**
   - Any key containing substrings like `secret`, `password`, `token`, `key`, `auth`, `pass`, `pwd`, or `cred` (case-insensitive) is marked as sensitive.
2. **Post-Processing Redaction:**
   - The diagnostic engine automatically extracts all values associated with sensitive keys from active environment maps.
   - Values with length <= 2 are excluded from redaction to prevent false positives on short placeholders.
   - The engine sorts all extracted sensitive values by length in descending order (to redact longer secrets first and prevent partial replacement issues).
   - In the final `DiagnosticsReport`, all matching occurrences of these values in Check `Name`, `Error`, or `Mitigation` strings are replaced with `[REDACTED]`.
3. **Gitignore Auditing:**
   - The security pipeline checks all discovered environment files (e.g. `.env`, `.env.*`, excluding `.env.example`).
   - It runs `git ls-files --error-unmatch` to ensure they are not tracked.
   - It runs `git check-ignore` (with a custom regex fallback parser if Git is unavailable) to verify they are correctly ignored in `.gitignore`.

## Consequences
- **Zero Leakage:** Prevents accidental leakage of local credentials in diagnostic reports.
- **Developer Safety:** Raises immediate alerts for untracked or unignored secret files.
- **Rule Clarity:** AI agents adding new diagnostics must pass raw output through the redaction layer before rendering.
