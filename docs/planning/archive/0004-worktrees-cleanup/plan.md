# 0004 — Worktree cleanup: batch `--orphans` and self-deletion on merge — Plan

adr_required: true
adr_reason: B introduces the **first implicit deletion** of prdash that is not born from a path written by the user. It narrowly supersedes the documented stance "on closing the app the worktrees are kept: there is no implicit deletion" (`cmd/prdash/worktrees.go:3-5`, README §Worktree management) with an exception justified by an observable fact ("the PR was merged from prdash") **plus** a lasting policy for the dirty case (delete only if clean; keep and warn if there are changes). It is a decision with real alternatives (delete with `--force` vs only if clean vs report the path) and with risk of data loss, not a refactor.
adr_title: adr-0007-worktree-cleanup-on-merge
adr_path: docs/adr/0007-worktree-cleanup-on-merge.md
adr_note: ADR 0007 bounds the implicit exception (trigger `ActionMerge` + `OK`, only from prdash), fixes the "only if clean" lock and the separation `Remove` (explicit paths, `--force`, no lock) vs `RemoveIfClean` (B). It does not rewrite ADR 0001 (provisioning): it complements it with the deletion policy. It is written in the plan and the executor keeps it coherent.

## Expected outcome

Two cleanup paths, and only two, for review worktrees that are already certain
trash:

1. **`prdash worktrees remove --orphans`** deletes **in batch** all the
   worktrees that `Audit` itself marks as orphans, and **only** those.
   `--orphans` is **exclusive** with explicit paths. With zero orphans it
   reports and exits with **0**. An optional `--dry-run` prints the exact batch
   that would be deleted and deletes nothing.
2. **On merge from prdash**: when a `merge` action launched by prdash finishes
   well, the worktree of that item is deleted **only if it is clean**. If
   there are uncommitted changes (including untracked files) or the state
   **cannot be checked**, it is **kept** and the notice says so:
   `merged, but the worktree has uncommitted changes — kept`.

Nothing else changes: **closing the app deletes nothing**, there is still no
close action, and the ownership (`worktree.Owned`) and managed-root
(`removablePath`) guards stay untouchable for every cleanup path.

## Scope

- **In**: `--orphans` and `--dry-run` flags in `remove`; "only if clean" lock
  and its fail-safe; review deletion port for the TUI; cleanup of the active
  review record when deleting; ADR 0007; README and CHANGELOG.
- **Out**: deletion on closing the app (hard restriction); a new "close"
  action; changes to the orphan definition of `Audit` and to the semantics of
  `Remove` (explicit paths); changes to the ownership guards; new state in the
  TOML schema (the cache `DeleteReview` is **not** config schema); the
  orchestrator's script and the untracked artifact `.codegraph/` (outside
  prdash).
- **Out (explicit)**: the worktree of a PR merged **outside** prdash is not
  deleted (the refresh that sees it merged does **not** trigger cleanup).

## Approach (high level)

### A — the flag lives in the subcommand, not in the global parser

`main.go` intercepts `worktrees` **before** the global `flag.Parse`, so
`--orphans`/`--dry-run` are parsed **inside** `remove`, with a pure helper
`parseRemoveArgs(args)` that does not touch `os.Args` and is testable in
isolation. The batch comes from the **same** source of truth that already
feeds `list`: `pr.Audit(ctx)` filtered by `Entry.Orphan`. `Remove` already
knows how to delete an orphan (source repo gone → it deletes the checkout), so
A **does not open a new deletion path**: it reuses the existing one, it only
automates the selection.

**Mitigation of the false orphan (without touching `Audit`)**: `Audit` marks
an orphan on **any** `os.Stat` error on the gitdir, not only "does not exist";
a temporarily inaccessible gitdir (network mount, permissions, another user)
would mark a healthy one as orphan. That semantics is not changed. The valve
is an optional **`--dry-run`** that prints the batch **from the same code
path** and deletes nothing, so the irreversible operation can be seen before
running it. The failure class is documented in the ADR/README.

### B — the trigger already exists; the port and the lock are missing

The prdash merge goes through a single funnel: `applyAction(out forge.Outcome,
cycle)` (`internal/tui/update.go`), called from `case actionMsg:`. The
`Outcome` **already** brings everything needed: `Kind == forge.ActionMerge`
and `OK` (plus the re-read post-action `Item`). **B needs no new plumbing in
the forge**: it hooks that funnel. What is missing is:

- a **deletion capability** that the TUI does not have today (`ReviewLookup`
  is read-only): an optional **`ReviewRemover`** port implemented by the
  `Executor`, which already has the `Provisioner` injected;
- an **"only if clean" lock** at the level that knows git (the provisioner),
  because `Remove` is `--force` on purpose for A's explicit paths and **must
  not** change semantics;
- clearing the **active review record** when it really is deleted, so that
  `ActiveReview` stops reporting a mounted review.

B's deletion runs **in the background** (git subprocess; the `Update`
handler cannot block) and its result comes back as its own message, which
composes the final notice without overwriting the facts the merge already
brought.

## Key decisions

1. **Two deletion modes, two methods, a single set of guards.**
   `Provisioner` gains `RemoveIfClean(ctx, id) (removed bool, reason string, err error)`.
   - `Remove` (explicit paths, A) stays **exactly** as is: it deletes even if
     there are changes; an orphan does not even have a repo to ask.
   - `RemoveIfClean` (B) applies the lock: first `removablePath` (the same
     guards), then it decides. The lock is **not** put inside `Remove` nor a
     boolean flag that changes its meaning.
   - **Where it does NOT go**: not in `executor` (its contract is to orchestrate
     without knowing git) nor in the TUI. The git fact lives in the `worktree`
     package.
2. **The lock asks the tree, not the index.**
   `git status --porcelain` (via `internal/gitcmd`), not `git diff --quiet`: a
   **new untracked file** is uncommitted work and `diff` ignores it.
   `strings.TrimSpace(output) != ""` ⇒ dirty ⇒ keep.
3. **Fail-safe in B: doubt keeps.** If the state check **fails** (index.lock,
   EACCES, gitdir that moved between `stat` and `status`), the result is
   `(removed=false, reason="could not read the worktree status")` and **not** a
   deletion. `err` is reserved for what really is a reportable infrastructure
   failure; "could not be checked" is a keep reason, not an action error.
4. **B is idempotent and invents no errors.** If the item has no mounted
   review, or the record's path no longer exists on disk, the result is
   `(false, "", nil)`: the merge finishes the same way and **no** cleanup
   error is reported. `Remove` is not idempotent today (an absent path
   errors), so `RemoveIfClean` checks before trying.
5. **New `ReviewRemover` port, not extending `ReviewLookup`.**
   `ReviewLookup` is documented as **read-only and degradable** ("without it…
   the only thing lost is that notice", `internal/tui/retarget.go`). A
   capability that **deletes** cannot inherit that contract: it needs an
   explicit absence ("without a remover there is no self-deletion, and the
   merge stays the same"). Signature:
   `RemoveReview(ctx, it) (removed bool, reason string, err error)`, implemented
   by `*Executor`: it maps item→worktree with `ActiveReview`, calls
   `Worktrees.RemoveIfClean`, and **only if it deleted** forgets the record.
   It is injected with `SetReviewRemover(ex)` next to the existing
   `SetReviewLookup(ex)` in `main.go`. With no remover injected, B degrades to
   "it does not self-delete".
6. **The active review record is cleared when deleting for real.**
   `cache.ReviewRecord` is persistent; leaving it pointing at a checkout that no
   longer exists would make `ActiveReview` lie forever ("stale base" notices,
   `--print`, remounts). `Resolver.ForgetReview(it)` +
   `cache.Store.DeleteReview(key)` are added, called **only** when
   `removed==true`. It is a record deletion, **not** new config state (nothing
   touches the TOML schema). Accepted residual: path A (`--orphans`) does not
   have the store at hand and leaves whatever record there is; it is
   documented, the CLI wiring is not widened.
7. **Composed notice, never overwritten.** `applyAction` already does an early
   `return` on the `DeleteMsg` case (branch not deleted) and on the stale
   retarget case. B cannot be a separate `setNotice` that overwrites those
   facts: the deletion starts as a background `tea.Cmd` and its result arrives
   as its own message (`reviewCleanupMsg`) that **composes** the merge's base
   notice with the outcome of the cleanup. Rules:
   - deleted → `… ok · worktree removed` (OK level);
   - kept due to dirty/unreadable → `… ok · worktree kept: <motivo>` (warn
     level); the dirty text is literally `merged, but the worktree has
     uncommitted changes — kept`;
   - infrastructure failure → `… ok · could not remove the worktree: <err>`
     (warn, **not** error: the merge did go through).
   With `DeleteMsg` present, the three facts coexist in the same notice.
8. **`--orphans` exclusive and `--dry-run` only with `--orphans`.**
   `parseRemoveArgs(args) (orphans, dryRun bool, paths []string, err error)`:
   `--orphans` + paths ⇒ usage error; `--dry-run` without `--orphans` ⇒ usage
   error; a token starting with `-` that is not recognized ⇒ usage error;
   neither `--orphans` nor paths ⇒ usage error (current behavior). Every usage
   error: message on **stderr**, **exit 2**, and **zero** deletions. Success
   (including "zero orphans") on **stdout**, **exit 0**. Deletion failure:
   **exit 1**.
9. **No `os.Exit` inside `runWorktrees`/`removeWorktrees`.** They return `int`
   and `main.go` exits; the tests run them in process. Putting an exit there
   would kill the suite. That seam is kept.
10. **ADR 0007, no editing accepted ADRs.** The 0007 bounds the implicit
    exception, declares which keep decision it supersedes and the policy for
    the dirty case. It chains with the 0001 (provisioning) without rewriting
    it. It is written in the plan.

## Affected files (high level)

**Production**

- `internal/worktree/worktree.go`: `RemoveIfClean` in the `Provisioner`
  interface and in `GitDirect`; `dirty` helper (`status --porcelain`);
  `Remove` **unchanged**.
- `internal/worktree/herdr.go`: `HerdrNative.RemoveIfClean` (same lock over
  `h.scan` and delegating to its native `Remove`, so the workspace is not left
  orphan).
- `internal/review/executor/executor.go`: `RemoveReview` in the `Executor`;
  `ForgetReview` in the `Resolver` interface.
- `internal/reporesolver/reporesolver.go` + `internal/cache/memo.go`:
  `ForgetReview` / `DeleteReview` (deletion of the persisted record).
- `internal/tui/retarget.go` (or `review_remover.go`): `ReviewRemover` port +
  `SetReviewRemover`; field on the `Model` (`app.go`).
- `internal/tui/app.go`: `reviewRemover` field; `reviewCleanupMsg` type.
- `internal/tui/update.go`: in `applyAction`, if `merge` + `OK` launch the
  deletion in the background; new `case reviewCleanupMsg` that composes the
  notice.
- `cmd/prdash/worktrees.go`: `parseRemoveArgs`; `removeWorktrees` with
  `--orphans` (and `--dry-run`) besides the explicit paths.
- `cmd/prdash/main.go`: `model.SetReviewRemover(ex)`.
- `docs/adr/0007-worktree-cleanup-on-merge.md`: new (written in the plan).
- `README.md`: `remove <path>` → `remove <path>…`, document `--orphans`,
  `--dry-run`, and **when** B deletes (merge from prdash, only if clean, with
  a warning) without contradicting the keep-on-close rule.
- `CHANGELOG.md`: `## [Unreleased] → ### Added`.

**Tests**

- `internal/worktree/worktree_test.go`: `RemoveIfClean` clean deletes; dirty
  keeps (with the reason); **untracked** keeps; `status` error keeps
  (fail-safe); absent path ⇒ no-op without error; guards (foreign / outside
  `Base`) keep rejecting.
- `internal/review/executor/executor_test.go`: `RemoveReview` with an active
  review → `RemoveIfClean` + `ForgetReview`; kept ⇒ does **not** forget; no
  review ⇒ `(false,"",nil)`; update `fakeProvisioner`/`Resolver` fakes.
- `internal/tui/section_test.go` / `app_test.go`: OK merge deletes (fake
  remover invoked with the right item); dirty keeps and warns with the exact
  text; does not trigger with approve/retarget/failure/conflict/permission/
  no-mergeable; no review ⇒ no error; composed notice with `DeleteMsg`;
  **closing the app does not invoke the remover**.
- `cmd/prdash/worktrees_test.go`: `--orphans` deletes all and only orphans;
  healthy and foreign intact; zero orphans ⇒ exit 0 and stdout with no stderr;
  **`--dry-run` prints the batch and does not delete**; `--orphans <path>` and
  `--dry-run` without `--orphans` and an unknown flag ⇒ exit 2 without touching
  anything; explicit paths keep behaving the same (current tests are kept). It
  may require a `captureStderr` helper.

## Work order (TDD, rough)

1. **Tests first, in blocks**: `worktree` (`RemoveIfClean`) → `cmd/prdash`
   (`parseRemoveArgs` + `removeWorktrees`) → `executor` (`RemoveReview`) →
   `tui` (trigger and notice).
2. **Wiring**: `Provisioner` interface (+`HerdrNative`, +fakes);
   `Resolver.ForgetReview` (+cache, +fakes); `Executor.RemoveReview`;
   `ReviewRemover` port + `SetReviewRemover` + `main.go`; `applyAction` +
   `reviewCleanupMsg`.
3. **Docs**: ADR 0007, README, CHANGELOG.
4. `make test` and manual smoke.

## Verifications

- `make test` (build + vet + gofmt + `go test -race ./...`) **green**. It is
  the only guarantee: the repo has no CI.
  _Correction (2026-10-04): CI does exist now — `.github/workflows/ci.yml`
  and `mutation.yml`; `make check` is its local equivalent._
- CLI smoke: `worktrees list` marks orphans; `remove --orphans --dry-run`
  prints the batch and does not delete; `remove --orphans` deletes only those;
  a healthy and a foreign one survive; `remove --orphans <path>` and `--bogus`
  exit with 2.
- TUI smoke: mount a clean review and merge → worktree gone and "removed"
  notice; mount with an uncommitted file and merge → worktree kept and
  "merged, but the worktree has uncommitted changes — kept" notice; `q`
  deletes nothing.
- ADR coherence: the 0007 declares which clause it supersedes; 0001 intact.

## Risks

- **False orphan** (mitigated, not eliminated): the definition of `Audit` does
  not distinguish "missing gitdir" from "inaccessible". `--dry-run` gives the
  batch before deleting and documents the class; changing `Audit` stays out of
  scope.
- **TOCTOU window** between `status` and `remove` inside the same goroutine:
  the lock is not atomic at git level. Acceptable: it reduces deletion to
  "clean at the moment of checking" and the fail-safe keeps when in doubt.
- **Residual record in A**: `--orphans` does not clear `cache.ReviewRecord`;
  it is documented and the CLI is not widened.
- **Composed notice**: if the cleanup result arrives after another notice, it
  must re-compose over the merge's **base**, not overwrite the current notice;
  the own message with the `base` captured on trigger fixes that.
- **Coupled fakes**: adding a method to `Provisioner` and another to
  `Resolver` forces updating the `executor`/`tui` fakes; it is mechanical work,
  not risk.
