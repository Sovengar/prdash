# 0004 — Worktree cleanup: batch `--orphans` and self-deletion on merge from prdash (feature)

## Problem

The review worktrees **belong to the user**: they exist to edit the PR inside
them, so prdash keeps them on purpose and **deletes nothing implicitly on
closing the app** (`cmd/prdash/worktrees.go:3-5`). That decision is correct and
is not touched. But it leaves two **hygiene** gaps, both bounded to worktrees
that are already **certain trash** (orphans, or whose PR no longer exists):

1. **Cleaning orphans one by one.** When the source repo disappears, the
   worktree becomes an orphan and `prdash worktrees list` already reports it
   (`prdash: <path> is orphaned: …`, `audit.go:62-65`). Deleting it today
   requires **enumerating paths by hand**, one at a time. Cleaning several
   requires a script or remembering every path. A gesture is missing:
   **"delete all the orphans"**.
2. **The worktree of a PR merged from prdash.** If the PR is merged **through
   prdash**, its worktree is **certainly** trash: the review is over. Today it
   stays on disk, taking space and dirtying the listing, until someone deletes
   it by hand. Since the trigger is "the PR no longer exists", deleting it at
   that moment **does not violate** the keep-on-close rule: it is not an
   implicit deletion by exiting, it is a deletion justified by an observable
   fact.

The risk of A is **nil** by construction: the orphans cannot come back. The
risk of B is **low** but not nil, and its concrete shape (uncommitted work) is
the open decision of this issue.

## Scope (in)

### A — `prdash worktrees remove --orphans`

New flag in the `remove` subcommand (`cmd/prdash/worktrees.go`). When present,
it deletes **only** the entries that `Audit` itself marks as orphaned:

- Source of truth: `pr.Audit(ctx)` filtering `Entry.Orphan` (`audit.go:23-31`,
  `audit.go:62-65`). The detection is not reimplemented: it is the same one
  that already feeds the `list` report.
- An orphan is deleted through the already existing `pr.Remove(ctx, path)`,
  which for the orphan case already knows how to delete the checkout directly
  (`worktree.go:105-127`).
- **Orphans only.** An owned and healthy worktree is **never** touched with
  `--orphans`.
- With **zero orphans** it finishes with **exit 0** and a clear message (it is
  not an error: it is the happy path).
- Deletion by **explicit paths** (`remove <ruta1> <ruta2> …`) keeps working
  exactly as today.

### B — self-deletion on merge from prdash

When a merge action launched **from prdash** finishes well, the worktree of
that item is deleted. The single observation point is `applyAction` in
`internal/tui/update.go:154`, which already receives the post-action
`forge.Outcome`:

- Signal: `out.Kind == forge.ActionMerge && out.OK` (`forge.go:271-275`). Any
  other combination (approve, retarget, failure, conflict, denied permission,
  `Unmergeable`) **triggers** nothing.
- Item → worktree mapping: `Executor.ActiveReview(it)` already exists
  (`executor.go:128`) and is exposed to the TUI through the `ReviewLookup`
  port (`retarget.go:133`, injected with `SetReviewLookup`). The `Executor`
  already has `Worktrees worktree.Provisioner` (`executor.go:50`), so the
  deletion uses the same path as the CLI command.
- If the item **has no** mounted worktree, there is nothing to delete and no
  error is reported: it merges and that's it.
- The action notice (`actionDoneNotice`) keeps coming out; if the worktree is
  also deleted, it is reported in the same notice.

## Out of scope (out)

- **Do NOT delete on closing the app.** It is the hard restriction
  (`worktrees.go:3-5`, README:504-506). No path of this feature touches
  `quit`/teardown. The only implicit deletion allowed is that of a worktree
  **provably dead** (orphan, or PR merged **from prdash**).
- **Do NOT create a "close" action.** `forge.go:270-275` only defines
  `ActionApprove`, `ActionMerge` and `ActionRetarget`: prdash **cannot close**
  a PR. "Closed from prdash" is today **unreachable** (see Correction to the
  statement); B reduces to "merged from prdash".
- **Do NOT touch the ownership/security guards.** They stay as they are:
  `worktree.Owned` (`audit.go:19-21`) and the requirement that the path lives
  under the managed root `Base` (`removablePath`, `worktree.go:130-143`). No
  foreign worktree is listed nor deleted.
- **Do NOT delete a dirty worktree with `--force` as an unquestioned
  default.** The `git worktree remove --force` (`worktree.go:116`) destroys
  uncommitted changes; whether that applies to B is decided by the **OPEN
  decision**, not by this section.
- **Do NOT change the semantics of `remove <path>`** nor the contract of
  `Provisioner.Audit`/`Remove`.
- **Do NOT persist** any new state nor touch the TOML schema.
- **Do NOT touch `list`** beyond coexisting with the new flag.

## Acceptance criteria

- [ ] `prdash worktrees remove --orphans` deletes **all** the orphans that
      `Audit` reports and **only** orphans.
- [ ] With `--orphans`, every **healthy** owned worktree (not orphan) stays on
      disk and `Audit` keeps listing it after the command.
- [ ] With `--orphans`, every **foreign** worktree (label/path not starting
      with `prdash-`, or outside `Base`) stays intact.
- [ ] `--orphans` with **zero orphans** finishes with **exit 0** and a clear
      message; it does not print an error nor a code other than 0.
- [ ] `prdash worktrees remove <ruta1> <ruta2>` (explicit paths) keeps working
      as today, including the current rejections (foreign/nonexistent → not
      touched, exit 1).
- [ ] After a **successful merge from prdash**, the worktree of the item
      disappears and the TUI reflects it (the item no longer has a mounted
      review).
- [ ] B's self-deletion **does not trigger** with: `approve`, `retarget`,
      **failed** merge, merge **blocked by permission**, merge with
      **conflict**, nor `Unmergeable`.
- [ ] B's self-deletion **does not trigger** on **closing the app** (it can be
      verified that removing all teardown paths deletes nothing).
- [ ] If the merged item **had no** mounted worktree, the merge finishes the
      same way and **without error**.
- [ ] The guards are preserved: a `Remove` on a **foreign** worktree or one
      **outside `Base`** keeps being impossible (the `removeRefusesForeign`
      tests still hold).
- [ ] `make test` green (build + vet + gofmt + `go test -race`).
- [ ] README (`502-545`) and CHANGELOG without contradictions:
      `remove --orphans` is documented, `remove <path>` → `remove <path>…` is
      corrected, and when B deletes and when it does not is explained.

## OPEN decision (blocks the plan) — B's deletion vs uncommitted work

`GitDirect.Remove` runs `git worktree remove --force` (`worktree.go:116`): if
the worktree has **uncommitted changes**, deleting it **destroys** them. The
code's own stance is that the worktree **belongs to the user** and can host
uncommitted work (`staleReviewNotice` warning in `update.go:187-194` and
README:129). A successful merge from prdash says that **the PR is over**, not
that the checkout is clean. What B does when the worktree is dirty has to be
chosen:

- **A — delete anyway with `--force`.** Simple and deterministic: "merged from
  prdash ⇒ worktree gone". Cost: **it can destroy uncommitted changes without
  warning**; it is exactly the scenario that the keep-on-close rule tries to
  avoid.
- **B (recommended) — delete only if clean; if there are changes, warn and
  keep.** B respects the keep-when-in-doubt rule and only automates the
  no-loss case. Cost: the dirty state has to be **detected** (e.g.
  `git status --porcelain`/`git diff --quiet`) before deleting, and the dirty
  worktree stays on disk with a warning ("merged, but the worktree has
  uncommitted changes — kept"), which the user has to clean up by hand.
- **C — delete anyway, but report the path so it can be recovered.** It keeps
  the automation and gives a clue. Cost: `--force` deletes the **checkout**;
  reporting the path **does not recover** the uncommitted work (the message
  promises a recovery that may not exist), so it is the worst of the three in
  terms of trust.

**Recommendation: B.** It is the only one that fulfils the documented
invariant ("the worktree belongs to the user") without giving up automation in
the frequent case (PR merged, clean checkout). The tradeoff is a residual dirty
case that stays on disk with a warning; one extra worktree is preferable to
lost work.

**Question for the user**: when you merge a PR from prdash and its worktree
has **uncommitted changes**, what do we do with the worktree? **(A)** delete it
anyway even if those changes are lost, **(B)** delete it only if it is clean
and, if there are changes, warn and keep it (recommended), or **(C)** delete it
anyway and show the path so you can try to recover whatever you want.

## Correction to the statement (non-blocking, but falsehoods are not documented)

1. **A does not fix "accepts a single path".** `removeWorktrees(pr, args[1:])`
   **already iterates over all the paths** (`worktrees.go:74-98`):
   `prdash worktrees remove <ruta1> <ruta2> …` **works today**. The only thing
   lying is the README (`511: remove <path>`, singular). What is genuinely
   missing is the `--orphans` flag: as is, a `--orphans` token would be
   treated as a path and rejected (`worktrees.go:85-90`). And the coverage
   already exists: `TestRunWorktreesRemoveOwned`,
   `TestRunWorktreesRemoveOrphan` and `TestRunWorktreesRemoveRefusesForeign`
   (`cmd/prdash/worktrees_test.go`).
2. **B is not "closed or merged from prdash": it is only "merged".** prdash
   **has no close action** (`forge.go:270-275`: `ActionApprove`,
   `ActionMerge`, `ActionRetarget`), so "closed from prdash" is
   **unreachable** today. `state.Derive` does map to
   `StateMerged`/`StateClosed` (`state.go:86-91`), but `StateClosed` describes
   a PR closed **on the forge**, not a prdash action. The observable signal of
   B is `out.Kind == forge.ActionMerge && out.OK`. Adding a close action stays
   **out of scope**.

## Risks / pending verifications

- **Loss of uncommitted work (B).** It is the central risk; the OPEN decision
  resolves it. Whatever option is chosen must be **documented** in the README
  next to the keep warning.
- **False positive of "merged"**: B's notice relies on `out.OK` +
  `ActionMerge`. Verify that a merge that **returns OK but leaves the branch
  undeleted** (`out.DeleteMsg`, `update.go:182-186`) is not confused: deleting
  the worktree is still correct, but the notice must say both things.
- **`out.Item` re-read**: `applyAction` receives the post-action item
  (`update.go:156-164`); the worktree must be mapped with the same `Item`
  that `ActiveReview` expects so the wrong worktree is not deleted.
- **The `ReviewLookup` port is read-only today** (`retarget.go:133`): exposing
  deletion to the TUI requires adding a method (or a new port) on the
  `Executor`, which already has `Worktrees` (`executor.go:50`). No new
  deletion path is invented: `Provisioner.Remove` is reused.
- **Orphan vs healthy in `--orphans`**: the detection depends on
  `sourceReachable` (`audit.go:94-103`). A temporarily inaccessible gitdir
  would mark a healthy worktree as orphan. `--orphans` would delete that
  checkout. Verify that the definition of orphan (nonexistent gitdir after
  `os.Stat`) is stable enough, or bound the risk in the plan (e.g.
  `--dry-run`/confirmation), without changing the semantics of `Audit`.
- **`HerdrNative`**: its `Audit` delegates to the `GitDirect` scan, so an
  orphan is detected the same inside and outside Herdr; confirm that `Remove`
  behaves the same in both environments.
- **Flag exclusivity (micro-decision, non-blocking)**: whether `--orphans`
  coexists with explicit paths or is exclusive. Proposal: `--orphans` is
  **exclusive** (or ignores paths) so that "orphans only" is a readable
  guarantee; it is settled in the plan.
- **Tests to add**: `--orphans` deletes all and only orphans; zero orphans →
  exit 0 with message; `--orphans` touches neither healthy nor foreign ones; B
  triggers with a OK merge and not with approve/retarget/failure/conflict; B
  does not trigger on teardown; B with a dirty worktree according to the
  chosen decision.
