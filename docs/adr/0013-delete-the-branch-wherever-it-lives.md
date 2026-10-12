# ADR 0013 — after a merge, delete the branch wherever prdash knows it lives

- **Status**: Accepted
- **Date**: 2026-10-12
- **Decider**: user (buble)
- **Scope**: `internal/reporesolver/`, `internal/review/executor/`,
  `internal/tui/`
- **Depends on**: ADR 0007 (worktree cleanup on merge), ADR 0001 (review worktree
  provisioning).

## Context

The merge notice says "branch deleted", and it is true **only for the remote
branch**: the forge deletes `refs/heads/<head>` through `--delete-branch` (gh) or
`--remove-source-branch` (glab). Two copies survive:

1. **The review branch in the clone that served the worktree.** `FetchReviewRef`
   fetches the PR head into `refs/prdash/<forge>/<n>` and creates the local
   branch `prdash/pr-N`. Removing the worktree (ADR 0007) does not touch them, so
   `prdash/pr-N` lingers in the bare clone (or in the local clone) forever.
2. **The user's own clone** keeps the PR head branch. prdash never touched it,
   partly because gh deliberately skips local deletion when `--repo` is present
   (`CanDeleteLocalBranch = !Changed("repo")`).

The forge owns neither of those copies, and `branch -D` refuses when a worktree
still holds the branch — verified: `error: cannot delete branch ... used by
worktree`, exit 1. So any deletion has to run **after** the worktree is gone.

## Decision

1. **On a successful merge, clean the three local copies too**, in this order:
   worktree (only if clean, per ADR 0007) → review branch and tracking ref in the
   review's clone → PR head branch in the local clone the user configured
   (`ResolveLocal`).
2. **The worktree gate is also the branch gate.** If the worktree is kept
   (dirty or unreadable), the review branch is not deleted: it is checked out,
   git would refuse, and pretending otherwise would report a deletion that did
   not happen. No worktree, or a removed one, frees the branch.
3. **Deletion is target-scoped and best-effort.** Each target is attempted on
   its own; a failure becomes a warning in the notice, never a failed merge. The
   notice says exactly what was deleted (`review branch`, `local branch`) and
   what could not be.
4. **The local clone is deletable only when prdash can identify it**:
   `ResolveLocal` maps the item's repo to a path, and the branch exists there and
   is checked out **nowhere** (`worktree list` sees every checkout). An unknown
   repo, a missing branch or a checked-out branch is a silent no-op: prdash does
   not own the user's clone and never guesses.
5. **The tracking ref is bookkeeping, not a deletion claim.** Removing
   `refs/prdash/<forge>/<n>` does not mean the branch is gone, so only the branch
   deletion produces "review branch deleted".
6. **The remote branch is left where it is.** The forge already deletes it; this
   change does not duplicate that call nor add an API round trip.

## Rejected alternatives

- **Delete the branch before removing the worktree.** Simpler ordering, but git
  refuses on a held branch and the branch would silently survive. Rejected.
- **`git update-ref -d` without `branch -D`.** Deletes the ref even when checked
  out, which leaves the worktree with a dangling HEAD and is exactly the
  half-state the clean-worktree gate avoids. Rejected.
- **Delete the local branch even when checked out.** Would either fail (warning
  noise) or require switching the user's checkout. prdash does not move the
  user's HEAD for hygiene. Rejected.
- **Ask the forge to delete the local branch** (drop `--repo`). Out of prdash's
  control, breaks the multi-forge case, and ties local cleanup to gh's
  `Changed("repo")` heuristic. Rejected.

## Consequences

**Positive**
- `prdash/pr-N` stops accumulating in the bare clone and the local clone.
- The merge notice is honest: it lists what went and warns on what did not.
- A dirty worktree keeps its branch, consistently with ADR 0007's padlock.

**Negative / costs**
- An implicit deletion now reaches the user's clone. It is bounded to a
  configured root, an existing branch and no checkout, and it is recorded here so
  that it does not generalize.
- A branch checked out somewhere is reported as kept only by omission (no
  warning), because it is a legitimate state, not a failure.

**Verification (closed with `make test`)**
- Review branch and tracking ref disappear from the bare clone and from the local
  clone; the local PR branch disappears from the resolved clone.
- A branch held by a worktree is refused and reported; after the worktree is
  removed the same call succeeds.
- A checked-out local branch and an unresolvable local repo are no-ops.
- A dirty worktree leaves the review branch in place and reports nothing deleted.
