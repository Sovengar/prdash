# Prototype brief — fix/delete-branch-toggle

Branch: `fix/delete-branch-toggle`
Source: task #16 (prdash) — "El toggle 'no borrar rama' (tab) no tiene efecto".

## Problem

In the merge confirmation, `tab` toggles `delete branch: yes|no`
(`internal/tui/update.go:323`, default `true` in `internal/tui/app.go:249`,
label in `internal/tui/sections.go:178`). Setting it to `no` is **not honoured**
on GitHub: the branch is deleted anyway.

## Mechanism (verified in code; confirm against the real forge)

- `internal/forge/github/github.go:142`: `--delete-branch` is appended only when
  the toggle is `yes`. Omitting it does not delete — so far the toggle looks honest.
- The lie is at the forge level: when the GitHub repo has the setting
  **"Automatically delete head branches"** enabled, GitHub deletes the head
  branch on merge **regardless** of the flag. There the `no` state promises
  something GitHub will not keep.
- GitLab (`internal/forge/gitlab/gitlab.go`, `--remove-source-branch`) sets
  `should_remove_source_branch` per MR; GitLab's project setting only changes the
  UI default, so the toggle likely works there. Confirm.

## What the prototype must build

A runnable prdash that **stops the toggle from lying**, along the preferred
direction (the other two options in the task are recorded below for context):

- **Preferred**: rename/adjust the option so it no longer promises "no
  deletion" it cannot keep — e.g. reframe it as a *request* to delete the branch
  (`delete branch: request yes/no`) and/or reflect the repo's real auto-delete
  policy in the confirm text. Behavioural choice: if the repo will auto-delete
  regardless, say so instead of offering a switch that cannot be kept.
- Alternative A: document the limitation (docs only).
- Alternative B: disable the repo setting before merging (needs admin + a
  GraphQL mutation, then restoring it) — heavier, likely out of scope.

The user has not chosen yet: the prototype is what the user tries before
deciding. Keep the diff small and the TUI honest.

## Constraints

- Go 1.26, bubbletea **v2** (`charm.land/*`, never `github.com/charmbracelet`),
  `internal/` only, English everywhere, comments only when they explain WHY
  (`docs/adr/0009`, `docs/adr/0010`).
- `make test` (build + vet + gofmt + `go test -race`) must stay green.
- TUI layouts degrade honestly — never paint a box halfway.
- Nothing is published from this lane: commits stay on `fix/delete-branch-toggle`.
- Verify the GitHub/GitLab behaviour against the real tools (`gh api`,
  `glab api`) or the vendor docs before asserting it in code/comments.
