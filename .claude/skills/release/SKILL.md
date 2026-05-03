---
name: release
description: Cut a new release of buienradarcli. Computes the next SemVer version from changes since the last tag, drafts release notes, and pushes an annotated tag that triggers the Release workflow. Use when the user asks to publish, ship, cut, or release a new version.
---

# Release skill

Drive a release of `buienradarcli` end to end. The workflow at
`.github/workflows/release.yml` listens for `v*.*.*` tag pushes and runs
GoReleaser; this skill's job is everything that happens before that push.

The annotated tag message is used verbatim as the GitHub Release body — the
workflow extracts it with `git tag -l --format='%(contents)'` and passes it
to GoReleaser via `--release-notes`. So the notes you draft here are what
users will read on the release page.

## Steps

### 1. Preflight

Run these checks. Abort with a clear message on any failure — never try to
auto-fix dirty state.

- `git rev-parse --abbrev-ref HEAD` must equal `main`.
- `git status --porcelain` must be empty.
- `git fetch origin main --tags`
- `git rev-list --left-right --count origin/main...HEAD` must report `0 0`.

### 2. Find the previous tag

```
LAST=$(git describe --tags --abbrev=0 2>/dev/null || echo v0.0.0)
```

If `LAST=v0.0.0` and `git tag --list` is empty, this is the first release.

### 3. Inspect changes since LAST

```
git log "$LAST"..HEAD --pretty=format:'%h %s%n%b%n---'
git diff --stat "$LAST"..HEAD
```

### 4. Classify the bump

Read the commits and the diff and pick one:

- **major** — a public CLI command or flag was removed or renamed (look for
  deletions/renames in `internal/cli/`), or any commit body contains
  `BREAKING CHANGE`.
- **minor** — a new command or flag was added (new files in `internal/cli/`,
  new flag registrations, additive surface), or commits prefixed `feat:`.
- **patch** — bug fixes, refactors, doc-only, CI-only, dependency bumps,
  `fix:` / `chore:` prefixes.

Compute `NEXT` by applying the bump to `LAST`. Special case: if `LAST` is
`v0.0.0` (no prior releases), propose `v0.1.0` regardless of classification.

### 5. Draft release notes

Group commits under headings; omit empty sections. Each entry is one line:

```
### Features
- <subject> (<short-sha>)

### Fixes
- <subject> (<short-sha>)

### Other
- <subject> (<short-sha>)
```

### 6. Confirm with the user

Use `AskUserQuestion` to show:

- the proposed `NEXT` version,
- a one-sentence reason for the bump classification,
- the drafted notes.

Offer options: accept / override to patch / override to minor / override to
major / cancel. If the user wants to tweak the notes, edit them in
conversation before tagging — do not commit a notes file to the repo.

### 7. Tag and push

```
git tag -a "$NEXT" -m "$NOTES"
git push origin "$NEXT"
```

Never use `--force`, `--no-verify`, or rewrite history in this skill.

### 8. Report back

Print:

- The tag that was pushed.
- The Actions run URL: `https://github.com/benoitdion/buienradarcli/actions`
- The release page URL (populates once the workflow succeeds):
  `https://github.com/benoitdion/buienradarcli/releases/tag/<NEXT>`

## Bailout cases

- Working tree dirty → tell the user to commit or stash, then re-invoke.
- Not on `main` → tell the user to switch branches, then re-invoke.
- Behind origin → tell the user to `git pull`, then re-invoke.
- No commits since `LAST` → tell the user there's nothing to release and
  exit; do not create an empty tag.
