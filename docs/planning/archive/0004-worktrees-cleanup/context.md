---
feature: 0004-feature-worktrees-cleanup
freshness: 925270001710278918c82eac8443603e4aca9b85
codegraph: ready
generated_by: codebase-researcher
---

# Context: Worktree cleanup — batch `--orphans` and self-deletion on merge

**Worktree root (the single prefix of all the paths below — do not assume the shell starts here):**
`/home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup`

All the paths of this document are absolute. `codegraph sync` ran OK
(`Already up to date`, `.codegraph/codegraph.db` present) ⇒ `codegraph: ready`.

## Scope
- In: `--orphans` flag (+ `--dry-run`) in `prdash worktrees remove`; `parseRemoveArgs`;
  `Provisioner.RemoveIfClean` with a "only if clean" lock; `RemoveReview` in the
  `Executor` + `ReviewRemover`/`SetReviewRemover` port; `Resolver.ForgetReview` +
  `cache.Store.DeleteReview`; trigger B in `applyAction` + `reviewCleanupMsg`
  message that composes the notice; ADR 0007 / README / CHANGELOG.
- Out: deletion on closing the app; a new "close" action; semantics of `Audit` or of
  `Remove` (explicit paths); TOML schema; `list`; `forge`.

## Files to Touch
| Symbol / Area | File | Lines | Why |
|---------------|------|-------|-----|
| `worktreeTimeout` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/cmd/prdash/worktrees.go | 23 | Timeout shared by `list` and each `remove`; also reuse it for the `--orphans` batch. |
| `runWorktrees` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/cmd/prdash/worktrees.go | 26-40 | `list`/`remove` dispatch; `remove` moves to `parseRemoveArgs(args[1:])` before deciding the mode. Returns `int` (not `os.Exit`): the tests run it in process. |
| `removeWorktrees` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/cmd/prdash/worktrees.go | 74-99 | **It already iterates over all the paths.** Extend with `--orphans`/`--dry-run`; keep the per-path rejection block (86-90, `Owned`+`Exists`) and exit 1. Exit 2 on invalid usage, 0 on success (including zero orphans). |
| `listWorktrees` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/cmd/prdash/worktrees.go | 42-70 | Model of how `Audit` is filtered by `Entry.Orphan` (57-67) and of the expected stdout/stderr pair. Not changed; it is the source of truth of "orphan". |
| `main` intercept | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/cmd/prdash/main.go | 28-34 | Intercepts `worktrees` **before** `flag.Parse` and passes `worktree.Select(..., cfg.WorktreeDir)`. **No changes** for A; B adds `model.SetReviewRemover(ex)` next to `SetReviewLookup` (line 64). |
| `worktreeRepoFixture` / `worktreeFixture` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/cmd/prdash/worktrees_test.go | 16-37 | Real git fixtures (`testutil.InitRepo`/`CommitFile`/`RunGit`); today they create 1 own (`prdash-pr-1`) + 1 foreign. Extend to two orphans + healthy + foreign. |
| `captureStdout` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/cmd/prdash/print_test.go | 141-156 | Exact template for the `captureStderr` that the exit 2 scenarios lack (swap of `os.Stderr` + pipe). |
| `Provisioner` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/worktree/worktree.go | 43-53 | Add `RemoveIfClean(ctx, id) (removed bool, reason string, err error)`. `Remove` (48) stays **exactly** as is. |
| `GitDirect.Remove` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/worktree/worktree.go | 108-128 | **No changes.** Reference for `RemoveIfClean`: same `removablePath` (109) and same `os.RemoveAll`/prune. An absent path errors today ⇒ `RemoveIfClean` checks before. |
| `removablePath` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/worktree/worktree.go | 132-143 | Sole ownership+root guard; `RemoveIfClean` uses it **first**. It is not relaxed. |
| `git.Run(...)` usage | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/worktree/worktree.go | 88, 116, 122, 169 | Git invocation pattern from the package; the `dirty` helper uses `g.git.Run(ctx, id, "status", "--porcelain")`. |
| `gitcmd.Runner.Run` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/gitcmd/gitcmd.go | 60-93 | Single git executor (non-interactive env). Returns trimmed stdout — `TrimSpace!="" ⇒ dirty`. |
| `Owned` / `LabelPrefix` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/worktree/audit.go | 14, 19-21 | Ownership guard; not touched. |
| `Entry` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/worktree/audit.go | 24-31 | `Orphan bool` + `Reason string`: the `--orphans` filter is `e.Orphan`. |
| `Audit` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/worktree/audit.go | 36-71 | Single source of "orphan" (`sourceReachable`, 62-65). **It is not reimplemented nor its semantics changed.** |
| `sourceReachable` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/worktree/audit.go | 96-103 | Marks an orphan on **any** `os.Stat` error of the gitdir (possible false-orphan) ⇒ the reason for `--dry-run`. |
| `HerdrNative.Remove` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/worktree/herdr.go | 174-188 | Model for `HerdrNative.RemoveIfClean`: resolve the workspace with `WorktreeList` and delegate to the native `Remove`, falling back to `h.scan` if there is no workspace. |
| `HerdrNative.Audit` / `Select` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/worktree/herdr.go | 197 / 39-44 | `Audit` delegates to `GitDirect`; `Select` picks the impl by environment. The native/direct symmetry is kept. |
| `Resolver` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/review/executor/executor.go | 23-33 | Add `ForgetReview(it model.Item) error` to the port. |
| `Executor` struct + `Worktrees` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/review/executor/executor.go | 48-57 | The `Executor` already has the `Provisioner`: `RemoveReview` reuses this field, with no new deletion path. |
| `ActiveReview` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/review/executor/executor.go | 128-140 | Item→worktree mapping used by `RemoveReview`. Careful: it uses the record's `rec.Worktree`. |
| `resolveRepo` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/review/executor/executor.go | 145-156 | Pattern of using the `Resolver` from the `Executor` (not modified). |
| `RemoveReview` (new) | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/review/executor/executor.go | new | `ActiveReview` → absent ⇒ `(false,"",nil)`; present ⇒ `Worktrees.RemoveIfClean`; only if `removed==true` ⇒ `Resolver.ForgetReview`. |
| `fakeProvisioner` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/review/executor/executor_test.go | 354-361 | Double of the `Provisioner`; add `RemoveIfClean`. It is built in `TestMountPassesNativeContainerToLayout` (340). The package's `Resolver` fakes need `ForgetReview`. |
| `Remember` / `RecordReview` / `ActiveReview` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/reporesolver/reporesolver.go | 199-204 / 207-210 / 213-215 | Add `ForgetReview` next to them; it uses `itemKey(it.ID())` (372-374). |
| `ReviewRecord` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/cache/memo.go | 17-23 | Persisted record that has to be forgotten. |
| `Store.Review` / `SetReview` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/cache/memo.go | 107-120 | Add `DeleteReview(key)` with `delete(s.memo.Reviews, key)` + `s.saveLocked()` (122-127) to persist. |
| `ActionKind` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/forge/forge.go | 266-276 | Confirms B is `ActionMerge`+`OK`; there is no close action. **No changes.** |
| `Outcome` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/forge/forge.go | 379-418 | `Kind`, `OK`, `DeleteMsg`, `Item`, `HasItem`: **everything B needs already travels here**. |
| `RunAction` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/forge/forge.go | 466-501 | Merge funnel; not touched. |
| `Derive` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/state/state.go | 86-111 | `StateMerged`/`StateClosed`; not used for the trigger (which is `out.Kind`+`out.OK`). No changes. |
| `Update` switch + `case actionMsg` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/tui/update.go | 20-130 (89-91) | Add `case reviewCleanupMsg:` that composes the notice. `actionMsg` is the template of "background work result". |
| `applyAction` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/tui/update.go | 154-199 | **Single point of B.** Hooks on the `out.OK` branch (177). Watch the early `return`s: `DeleteMsg` (182-186) and stale retarget (190-194). |
| `actionDoneNotice` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/tui/update.go | 480-496 | Base merge notice; B composes over it, it does not overwrite it. |
| `startAction` / `launchAction` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/tui/update.go | 501-510 / 519-530 | Template of the background deletion: goroutine + `sendEvent(actionMsg{...})`. B uses the same pattern with `reviewCleanupMsg`. |
| `Model` fields + `reviewLookup` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/tui/app.go | 190-318 (286-289) | Add field `reviewRemover ReviewRemover`. |
| `Mounter` / `SetMounter` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/tui/app.go | 99-101 / 377-379 | **Exact template** of optional port + injector ("nil disables it"). `ReviewRemover`/`SetReviewRemover` copy it. |
| `mountMsg` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/tui/app.go | 77-81 | Template of the typed result message ⇒ `reviewCleanupMsg`. |
| `SetReviewLookup` call | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/cmd/prdash/main.go | 64 | Place where to add `model.SetReviewRemover(ex)` right after. |
| `ReviewLookup` / `SetReviewLookup` / `staleReviewNotice` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/tui/retarget.go | 133-135 / 149 / 682-691 | A **read-only** port today; it is **not** extended. `staleReviewNotice` keeps its meaning (the user's worktree). |
| `fakeLookup` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/tui/retarget_test.go | 523-532 | Lookup double; B's tests also need a fake `ReviewRemover`. |
| `WorktreeDir` config | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/config/config.go | 132, 147, 235-236, 318 | Key/toml/default already exist. **Zero schema changes** (the `DeleteReview` belongs to the cache, not to config). |
| README §worktrees | `README.md` | 502-545 | Fix `remove <path>` → `remove <path>…`, document `--orphans`/`--dry-run` and when B deletes. Lines 504-506 fix the keep rule that is **not** contradicted. |
| CHANGELOG `[Unreleased]` | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/CHANGELOG.md | 8-11 | Add entry in `### Added`. |
| ADR 0007 | /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/docs/adr/0007-worktree-cleanup-on-merge.md | 1-106 | Already drafted in planning; the executor keeps it coherent (it does not rewrite accepted ADRs). |

## Contracts
- `Provisioner.RemoveIfClean(ctx context.Context, id string) (removed bool, reason string, err error)` — new in `internal/worktree/worktree.go:43`. `Remove(ctx, id) error` (`:48`) does **not** change signature nor semantics. The "only if clean" lock lives **only** in `RemoveIfClean`: not inside `Remove`, not in a boolean.
- `Executor.RemoveReview(ctx context.Context, it model.Item) (removed bool, reason string, err error)` — new in `internal/review/executor/executor.go`. It implements the TUI port; it maps with `ActiveReview` (`:128`) and delegates to `Worktrees.RemoveIfClean`. It only calls `Resolver.ForgetReview` if `removed == true`.
- `Resolver.ForgetReview(it model.Item) error` — new in the interface `internal/review/executor/executor.go:23` and in `internal/reporesolver/reporesolver.go` (next to `ActiveReview`, `:213`).
- `cache.Store.DeleteReview(key string)` — new in `internal/cache/memo.go` (next to `SetReview`, `:115`); it must call `saveLocked()` (`:122`) to persist.
- New TUI port `ReviewRemover interface { RemoveReview(ctx context.Context, it model.Item) (bool, string, error) }` + `(*Model).SetReviewRemover(ReviewRemover)`; `ReviewLookup` is **not** extended (read-only contract, `retarget.go:133`).
- `parseRemoveArgs(args []string) (orphans, dryRun bool, paths []string, err error)` — a **pure** helper (it does not touch `os.Args`); `--orphans`+paths ⇒ error, `--dry-run` without `--orphans` ⇒ error, unknown `-…` token ⇒ error, nothing ⇒ error. Invalid usage: stderr + exit 2 + zero deletions. Success: stdout + exit 0; deletion failure: exit 1.
- `reviewCleanupMsg` — new message type in `internal/tui` that carries the `base` (merge notice captured on trigger) + `(removed, reason, err)` and composes the final notice.
- `forge.Outcome` (`Kind`/`OK`/`DeleteMsg`/`Item`/`HasItem`) is **respected** as B's input contract: no plumbing is added to the forge.

## Pattern to Follow
- **Optional port + injector with degradation** → `Mounter`/`SetMounter` (`internal/tui/app.go:99-101`, `:377-379`); `ReviewRemover`/`SetReviewRemover` copy it literally ("nil disables it"). Injection from `main.go:64`.
- **Background work + typed message** → `launchAction` (`internal/tui/update.go:519-530`) and `mountMsg` (`internal/tui/app.go:77-81`): goroutine with timeout, `sendEvent`, `case ...Msg` in `Update` (89, 103). B uses this same skeleton.
- **Guard + worktree deletion** → `GitDirect.Remove` (`internal/worktree/worktree.go:108-128`): `removablePath` first; `RemoveIfClean` replicates the guard and decides before deleting.
- **Orphan filter** → `listWorktrees` (`cmd/prdash/worktrees.go:55-68`) and `Audit` (`internal/worktree/audit.go:36-71`): `--orphans` filters `e.Orphan`, without reimplementing the detection.
- **Testable in-process CLI** → `runWorktrees`/`removeWorktrees` return `int`; the tests use `captureStdout` (`cmd/prdash/print_test.go:141-156`) and real git fixtures (`cmd/prdash/worktrees_test.go:16-37`).
- **TUI tests by messages** → `merge_test.go` (`newMergeFixture` `:38`, `waitOutcome` `:57`, `send(t, m, actionMsg{...})` `:311`) and helpers `newTestModel`/`send`/`press`/`lastToast` (`internal/tui/app_test.go:23,66,76,117`).
- **Worktree tests with real repos** → `internal/worktree/worktree_test.go` (`TestCreateListRemove` `:21`) and `audit_test.go` (`TestRemoveOrphanDeletesCheckout` `:105`, `TestRemoveRefusesForeignWorktree` `:129`).

## Tests
- Existing affected:
  - /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/cmd/prdash/worktrees_test.go — `TestRunWorktreesListsOnlyOwned` `:41`, `TestRunWorktreesRemoveRefusesForeign` `:72`, `TestRunWorktreesRemoveOwned` `:89`, `TestRunWorktreesRemoveOrphan` `:108`, `TestRunWorktreesUsageErrors` `:124` (they are kept; the `--orphans`/`--dry-run` ones are added).
  - /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/review/executor/executor_test.go — `fakeProvisioner` `:354` (add `RemoveIfClean`); package `Resolver` fakes (add `ForgetReview`).
  - /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/tui/retarget_test.go — `fakeLookup` `:523-532` (the lookup double stays the same; B needs a separate `ReviewRemover`).
  - /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/tui/merge_test.go — `TestMergeNoticeNamesTheMode` `:306` and the `send(..., actionMsg{...})` `:311` pattern are the base of B's tests.
  - /home/buble/dev/projects/prdash/.worktrees/prdash.feat-worktrees-cleanup/internal/worktree/audit_test.go — guards `TestRemoveRefusesForeignWorktree` `:129`, `TestRemoveRefusesPathOutsideBase` `:148` must stay green.
- Framework / runner: Go testing. `make test` = `go build ./... && go vet ./... && gofmt -l (git ls-files '*.go') && go test -race ./...` (`Makefile:35-41`).
- Integration infra: **unit only**, no network. Fixtures with real git via `internal/testutil/git.go` (`InitRepo` `:28`, `CommitFile` `:40`, `RunGit` `:13`; also `InitBare`, `SetRemote`, `Push`). Fake adapters: `testutil.FakeAdapter` (`internal/testutil/testutil.go`). TUI cache isolated with `XDG_CACHE_HOME` in `newTestModel`.
- Suggested targets:
  - `--orphans` (behavior §A) → `cmd/prdash/worktrees_test.go` with the extended `worktreeRepoFixture` (two orphans removing the repo, one healthy, one foreign) + a new `captureStderr`.
  - Exclusive / unknown flag / no paths → exit 2 via stderr (`captureStderr`).
  - `RemoveIfClean` (clean/dirty/untracked/status-error/absent/guards) → `internal/worktree/worktree_test.go` + `audit_test.go`.
  - `RemoveReview` (active⇒remove+forget; kept⇒no forget; no review⇒`(false,"",nil)`) → `executor_test.go`.
  - B (OK merge deletes and composes; dirty keeps; does not trigger with approve/retarget/failure/conflict/permission/no-mergeable; no review ⇒ no error; `DeleteMsg` coexists; quit does not delete) → `internal/tui/merge_test.go` / `section_test.go`.

## Conventions & Boundaries
- Comments in **Spanish**, identifiers in **English**; the code is the source of truth (no references to specs/IDs).
- `internal/worktree` **cannot** import `internal/tui` nor `internal/review/executor` (it does not today: it only imports `gitcmd`, and `herdr` in `herdr.go`).
- `internal/tui` **already** imports `internal/config` and `internal/review/executor` (`app.go:17-23`) and `internal/worktree` (`retarget.go:25`) ⇒ the `ReviewRemover` port can talk about `model.Item` with no new cycle.
- `internal/forge` **must not** know worktrees: B hooks into the TUI over the existing `Outcome`; zero changes in `forge`.
- `--print` does not change (it uses neither `newRefLayout` nor the mode; guard in `print_test.go:168`).
- The TUI touches neither network nor disk in the update loop: B runs in a goroutine (the `launchAction` pattern).
- No state is added to the TOML schema; `cache.ReviewRecord` is persisted memory, not config.

## Integration Points (non-obvious)
- **`removeWorktrees` already walks all the paths** (`cmd/prdash/worktrees.go:84`): A's only real gap is the flag. A `--orphans` token today is treated as a path and rejected (`:86`) — hence the need to parse before. 
- **`applyAction` has early `return`s** on `DeleteMsg` (`update.go:182-186`) and on stale retarget (`:190-194`): a B `setNotice` placed after would be skipped or would overwrite those facts. B must start the deletion **before/independently** of those returns and come back with `reviewCleanupMsg` that **composes** over the base notice captured on trigger.
- **"Merge OK but the branch was not deleted" (`out.DeleteMsg`) must coexist** with "worktree removed/kept": they are two different truths of the same merge (scenario `the merge notice keeps all the truths at once`).
- **`Audit` marks an orphan on any `os.Stat` error** (`audit.go:62-65`, `sourceReachable:96-103`), not only "does not exist" (false-orphan due to an inaccessible gitdir). That is why `--dry-run` is mandatory in the plan even though `behavior.feature` does not list it as a scenario: it mitigates the irreversible without touching `Audit`.
- **`main.go` intercepts `worktrees` before `flag.Parse`** (`main.go:28`): the parsing of `--orphans`/`--dry-run` must be local to `remove` (`parseRemoveArgs`), never global flags.
- **`mainRepoOf`/`linkedGitDir`** (`worktree.go:201-208` / `audit.go:75-92`): `Remove` resolves the repo from the worktree itself (not from memory); an orphan has no repo, and `Remove` already handles it (`worktree.go:119-127`). `RemoveIfClean` must not assume the repo is present.
- **`worktree_test.go`/`audit_test.go` create real git repos** with `testutil`: a "clean" `RemoveIfClean` can create a real worktree and delete it; "dirty" write a file without committing; "untracked" a new file (which `git diff --quiet` does **not** detect ⇒ use `status --porcelain`).
- **`removeWorktrees` does not call `os.Exit`**: the tests run it in process (`runWorktrees` returns `int`). Putting an `os.Exit` there would kill the suite.
- **`HerdrNative.Remove` delegates to `h.scan.Remove`** when it does not find a workspace (`herdr.go:187`): the native `RemoveIfClean` must ask the state through `h.scan` (real git) and delete through the native one, without skipping the lock.

## Risks / Assumptions
- **`--dry-run` is already covered by `behavior.feature`** (three scenarios: it shows the exact batch without deleting, zero orphans ⇒ exit 0, and `--dry-run` without `--orphans` ⇒ usage error) besides `plan.md` (decision 8). There is no scope gap: the executor derives it from the scenarios.
- **Coupled fakes**: adding a method to `Provisioner` and another to `Resolver` forces touching `fakeProvisioner` (`executor_test.go:354`) and the `Resolver` fakes; mechanical work, not risk. Any test `Provisioner` in `internal/worktree/herdr_test.go` (`fakeRunner` `:19` belongs to the Herdr port, not to the `Provisioner`) is not affected.
- **TOCTOU window** between `status --porcelain` and `Remove`: the lock is not atomic at git level. Accepted (fail-safe keeps when in doubt).
- **Residual record in A**: `--orphans` (CLI) does not have the `Store` at hand and leaves whatever `ReviewRecord` there is; it is documented, the CLI is not widened. Only B forgets the record (and only if `removed==true`).
- **Composed notice**: if the cleanup result arrives after another notice, it must re-compose over the **merge's base** (captured on trigger), not over the current notice.
- **`staleReviewNotice` vs B**: after really deleting, `ForgetReview` clears the record so `ActiveReview` stops lying; without it, the "stale base" notice would keep pointing at a nonexistent checkout.
- **Freshness**: context generated against `925270001710278918c82eac8443603e4aca9b85`; if the executor detects that the symbols/lines no longer match, it must emit `CONTEXT_STALE` instead of re-discovering.
