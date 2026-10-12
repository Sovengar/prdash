# Prototype brief — fix/merge-delete-branch-everywhere

Branch: `fix/merge-delete-branch-everywhere`
Source: task #15 (prdash) — "Merge no borra la rama (ni remota ni local)".

## Goal (from the user, verbatim)

> "la rama debe eliminarse tanto en local como el bare clone como el review-worktree como el upstream/origin"

After a successful merge, prdash must delete the PR's branch in **all four** places:
the **local** clone, the prdash **bare clone**, the **review-worktree**, and the
**upstream/origin** (remote).

## Current behaviour (verified in code — build on this, don't re-derive)

- **upstream/origin**: already deleted. `internal/forge/github/github.go` passes
  `--delete-branch`, `internal/forge/gitlab/gitlab.go` passes
  `--remove-source-branch`. gh's `deleteRemoteBranch()` deletes
  `refs/heads/<headRefName>` via API whenever `--delete-branch` is set (and the PR
  is not a fork and not auto-merge). `--repo` does **not** disable it.
- **review-worktree**: already removed by `Executor.RemoveReview` →
  `worktree.RemoveIfClean` (`internal/review/executor/executor.go:112`,
  `internal/worktree/worktree.go:141`) when the worktree is clean, right after a
  successful merge (`internal/tui/update.go:167 triggersReviewCleanup`).
- **bare clone**: NOT cleaned. The PR head ref is fetched into
  `refs/prdash/<forge>/<n>` and a local branch `prdash/pr-N`
  (`internal/reporesolver/reporesolver.go:138 FetchReviewRef`, `:237
  ReviewBranch`). Removing the worktree does **not** delete `prdash/pr-N`; it
  lingers in the bare clone forever.
- **local clone**: never touched. prdash does not own the user's clone, and gh
  deliberately skips local deletion when `--repo` is present
  (`CanDeleteLocalBranch = !Changed("repo")`). `Resolver.ResolveLocal(ref)` maps
  a repo to a local path when the user configured roots.

## What the prototype must build

A runnable prdash that, **after a successful merge**, removes the branch also
from the places the CLI leaves behind — small and honest, no half-states:

1. **Review-worktree**: already removed; make sure the branch deletion runs
   **after** the worktree is gone (git refuses `branch -D` on a branch checked
   out in a worktree — verified: `error: cannot delete branch ... used by
   worktree`, exit 1).
2. **Bare clone**: delete `prdash/pr-N` (resolver `ReviewBranch(it.Number)`) and
   the tracking ref `refs/prdash/<forge>/<n>` from the bare clone. Best-effort
   and non-fatal: a failure is a warning in the notice, never a failed merge.
3. **Local clone**: delete the PR head branch (`model.Item.SourceBranch`) in the
   local repo prdash can resolve for that item (`ResolveLocal`), only when it
   exists and is not currently checked out. If prdash can't resolve a local repo,
   it is a no-op with a notice — never an error.
4. **upstream/origin**: already handled by the CLI; keep it, don't duplicate.

The merge notice (`internal/tui/update.go` `actionDoneNotice` /
`reviewCleanupNotice`) must stay truthful: say what was actually deleted, warn
when something could not be. Unknown/fork/other-forge cases must degrade, not
panic.

## Constraints

- Go 1.26, bubbletea **v2** (`charm.land/*`, never `github.com/charmbracelet`),
  `internal/` only, English everywhere, comments only when they explain WHY
  (`docs/adr/0009`, `docs/adr/0010`).
- `make test` (build + vet + gofmt + `go test -race`) must stay green; new
  behaviour needs tests. `make check` before you call it done.
- Never hardcode forges/hosts/branches; derive from the item/config.
- Degradation preserved: a missing `gh`/`glab`/Herdr never breaks the TUI.
- If it changes observable behaviour, the decision + alternatives go in a new ADR
  in `docs/adr/`.
- This change is user-visible → update `docs/FEATURES.md` in the same change.
  The global skill `~/.agents/skills/prdash/SKILL.md` is OUTSIDE the working
  environment; the orchestrator handles it (note it, don't try to edit it).
- Nothing is published from this lane: commits stay on
  `fix/merge-delete-branch-everywhere`. The user runs `~/.local/bin/prdash`;
  `make install` is the orchestrator's job at the end.

## How to verify

- Unit tests with the existing fakes (`internal/testutil`) for the bare-clone and
  local deletion paths, and for the "worktree must be removed first" ordering.
- A real git fixture: create a bare clone + a worktree on `prdash/pr-N`, merge,
  and assert the branch is gone from all four places.

## Sibling note

A separate run `fix/delete-branch-toggle` (task #16) is in flight on the same
area (making the `tab` toggle honest on auto-delete repos). Do not let this
prototype collide with it; scope here is deletion, not the toggle.

## Iteration 1 — added scope: pre-emptive "not mergeable" indication

User feedback: merging a conflicting PR (`#7`, `CONFLICTING`/`DIRTY`) gave only
the post-hoc refusal `merge refused: the forge will not merge it as it is: rebase
the branch onto the target and push`. The TUI never warned **before** arming —
"la TUI no me indica que no pueda mergearse por conflictos". This is tsk task #17
plus part of #15's usability.

Mechanism (verified): GitHub's `mergeable` is lazy — list/detail return
`UNKNOWN` until computed, and `state.MergeBlock` (`internal/state/state.go`)
only returns the conflict reason when `Mergeable.Known && Conflicted`. A
stale/`UNKNOWN` item therefore arms with **no** warning.

Add to this prototype: a clear, pre-emptive **not-mergeable** signal so the user
never gets to a doomed merge.

- Resolve the lazy value: when arming (or refreshing the selected item), make
  sure the item's mergeability is actually known — re-read it (`ItemState`) or
  fetch `mergeable` specifically — instead of trusting a stale `UNKNOWN` from
  the inbox list.
- Surface it clearly in the TUI, not only as a soft suffix in the armed box:
  e.g. a visible marker on the item/row and an explicit block/warning at arm
  time when the item conflicts (or is otherwise not mergeable). Honest
  degradation: `UNKNOWN` must not be painted as mergeable.
- Keep the existing correct refusal path; this is about warning earlier, not
  about skipping the forge's verdict.

Still: Go 1.26, charm.land v2, `make test` green, tests for the new behaviour,
update `docs/FEATURES.md`; ADR if the behaviour decision warrants it (it likely
does). Nothing published; commit on `fix/merge-delete-branch-everywhere`.

## Iteration 2 — feedback on where the signal lives

The conflict marker must not ride inside the title text. Two changes:

1. **Row:** the `✗` must live **on the item**, not prefixed to the title. Give
   the row a dedicated item-level indicator (its own small marker column/gutter,
   or the existing status area) so the title stays the title. `internal/tui/table.go`
   currently renders it as a span inside the title cell (`titleCell`) — move it
   out of there. Keep the rule: only a forge-reported conflict is marked;
   `UNKNOWN` and `MERGEABLE` stay unmarked.
2. **Detail:** add a field in the detail pane that states mergeability — e.g.
   `Mergeable` with `yes` / `no (conflicts with <target>)` / `unknown`. It goes
   in the `status` block next to State/Checks (`internal/tui/detail.go`).

Everything else from Iteration 1 stands (hard block at arm time, lazy resolution,
UNKNOWN warning).
