# ADR 0007 — worktree cleanup: orphans in batch and self-deletion on merge

- **Status**: Accepted
- **Date**: 2026-09-27
- **Decider**: user (buble)
- **Scope**: `prdash worktrees` subcommand, `internal/worktree/`,
  `internal/review/executor/`, `internal/tui/`
- **Depends on**: ADR 0001 (review worktree provisioning).
- **Supersedes**: the total-preservation clause of `cmd/prdash/worktrees.go`
  ("on closing the app the worktrees are preserved: there is no implicit
  deletion"), which becomes "there is no deletion because the app is closed;
  there is deletion because of an observable fact" (see Decision 2).

## Context

Review worktrees **belong to the user**: they exist to edit the PR inside
them, so prdash preserves them on purpose and deletes nothing when the app
closes. That decision is still correct and **is not touched**. But it leaves
two hygiene gaps, both bounded to worktrees that are already trash with
certainty:

1. **Cleaning orphans requires enumerating paths by hand.** When the source
   repo disappears, the worktree is orphaned and `worktrees list` already
   reports it, but deleting several requires remembering every path.
2. **The worktree of a PR merged from prdash stays on disk.** If the PR is
   merged **through prdash**, its worktree is already trash: the review is
   over.

Two verified facts condition the decision:

- **prdash has no close action.** Only `approve`, `merge` and `retarget`
  exist. "Closed from prdash" is unreachable today; the only observable fact
  of "the PR no longer exists" that prdash can cause is a merge that goes
  well.
- **`git worktree remove --force` destroys uncommitted changes.** A successful
  merge says the PR is done, **not** that the checkout is clean.

## Decision

1. **A — `prdash worktrees remove --orphans`** deletes in batch **only** the
   entries that `Audit` itself marks as orphaned (same source of truth as
   `worktrees list`), and it is **mutually exclusive** with explicit paths.
   Zero orphans is the happy case (exit 0). The definition of orphan is **not**
   touched.
2. **B — self-deletion on merge from prdash.** When a `merge` action launched
   by prdash finishes well (`ActionMerge` + `OK`), the worktree of that item is
   deleted **only if it is clean**. It is the first implicit deletion that is
   not born from a path written by the user; it is justified on an observable
   fact (prdash did the merge) and not on closing the app.
3. **"Only if clean" padlock**, with fail-safe: dirty (untracked files
   included) ⇒ **preserved** and warned
   (`merged, but the worktree has uncommitted changes — kept`). If the state
   **cannot be checked**, it is also preserved. When in doubt, never delete.
4. **Two deletion modes, not a flag.** `Remove` (explicit paths, `--force`,
   no padlock) and `RemoveIfClean` (B, with padlock) are different methods.
   The padlock is not put inside `Remove` nor in a boolean that changes its
   meaning: A and B have different justifications and must be readable
   separately.
5. **The guards are not relaxed.** Every cleanup path goes through
   `worktree.Owned` (label or name with the `prdash-` prefix) **and** through
   living under the managed root. A foreign worktree is untouchable by explicit
   path, by `--orphans` and by B. That guard applies equally to plain git and
   to Herdr's native provisioning **before** delegating its deletion: the
   native path cannot skip ownership nor the root.
6. **`--dry-run` for `--orphans`.** `Audit` marks orphan on **any** `os.Stat`
   error, not only "does not exist", so a temporarily inaccessible gitdir
   could mark a healthy one as orphaned. `Audit` is not changed: the exact
   batch is offered for printing, from the same code path, without deleting
   anything.
7. **Record of the active review.** When B really deletes, it forgets that
   item's `cache.ReviewRecord`; otherwise `ActiveReview` would lie forever. It
   is a record deletion, not new configuration state.

## Rejected alternatives

- **Always delete with `--force` on merge (no padlock).** Simple and
  deterministic, but it destroys uncommitted changes without warning: it is
  exactly the scenario the preservation rule tries to avoid. Rejected.
- **Always delete and report the path to "recover".** It keeps the automation,
  but `--force` deletes the checkout: reporting the path does not recover the
  uncommitted work and the message promises something that may not hold.
  Rejected as dishonest.
- **B as a confirmation (press to delete after the merge).** It removes the
  risk of loss, but reintroduces the manual step B exists to remove and adds
  state to the TUI. Rejected.
- **`remove` without arguments = orphans.** Convenient, but it breaks the
  explicit-path contract and is dangerous by default. Rejected; the flag names
  the intent.

## Consequences

**Positive**
- Orphan cleanup goes from "one path at a time, by hand" to a single gesture,
  with a preview (`--dry-run`) before the irreversible step.
- The worktree of a PR merged from prdash does not stay there dirtying the
  listing, and the case with uncommitted work is preserved with an explicit
  warning.
- The `Remove` / `RemoveIfClean` split leaves each deletion justification in
  its own method, without changing the semantics the explicit paths already
  used.

**Negative / costs**
- An implicit deletion appears. It is bounded to an observable fact (merge via
  prdash) and to a clean checkout; it is recorded here so that it does not
  generalize.
- The dirty case leaves a worktree on disk that the user cleans by hand (with
  a warning).
- Orphan detection does not distinguish "absent" from "inaccessible":
  `--dry-run` mitigates it, it does not eliminate it.
- In A, the persisted record of the review can stay residual (the CLI does not
  have the store at hand); it is documented and the wiring is not widened.

**Verification (to close with `make test`)**
- `RemoveIfClean` deletes when clean, preserves dirty/untracked/unreadable and
  it is not an error if the path no longer exists; the guards keep rejecting
  foreign and outside-the-root.
- `remove --orphans` deletes all and only orphans; `--dry-run` deletes
  nothing; mutually exclusive with paths; zero orphans ⇒ exit 0.
- Merge OK from prdash deletes the clean worktree and composes the warning; it
  does not trigger with approve/retarget/failure/conflict/permission nor when
  closing the app; it does not trigger with a PR merged outside prdash.
