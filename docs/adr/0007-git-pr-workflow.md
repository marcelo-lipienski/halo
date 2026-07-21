# 7. Git & Pull Request Workflow

## Status
Accepted

## Context
AI agents need explicit, structured instructions for committing, pushing, and merging code changes remotely without direct pushes to `master`. Storing these instructions in a separate document prevents them from wasting token space during core code implementation tasks.

## Decision
1. **Branching:** Work must occur on feature branches named `feat/<name>` or `fix/<name>`. Local commits to `master` are forbidden.
2. **PR Process:**
   - Commit changes atomically with descriptive commit messages.
   - Do not include `Co-authored-by` metadata trailers.
   - Push feature branch to GitHub.
   - Monitor GitHub Actions runs via the `gh` CLI.
   - Open a Pull Request via `gh pr create` with a descriptive title and body.
   - Merge and close the PR via `gh pr merge --merge --delete-branch` once CI checks turn green.
   - Check out local `master` branch and pull updates: `git checkout master && git pull origin master`.
   - Tagging/Release: Suggest a new tag version based on the latest git tag (e.g., query latest tag using `git describe --tags --abbrev=0`), and prompt the user for confirmation. Upon confirmation, create the tag locally and push it to GitHub: `git tag vX.Y.Z && git push origin vX.Y.Z`.

## Consequences
- Protects `master` branch stability via automated CI checks.
- Establishes a clean git history.
- Isolates git workflow instructions so they are only loaded when committing changes.
