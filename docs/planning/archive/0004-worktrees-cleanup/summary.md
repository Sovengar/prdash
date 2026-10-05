# Summary: worktrees-cleanup

## Metadata
- **Completed:** 2026-09-27 20:30
- **Duration:** ~15 minutes (closing)
- **Plan Number:** 0004
- **PR:** [#21](https://github.com/Sovengar/prdash/pull/21) — `feat/worktrees-cleanup` → `main`

## Scenarios
| Scenario (behavior.feature) | Status |
|-----------------------------|--------|
| `--orphans` deletes all the orphans and only the orphans | ✅ Passed |
| `--orphans` deletes exactly what "worktrees list" marks as orphaned | ✅ Passed |
| `--orphans` never touches a foreign worktree | ✅ Passed |
| a healthy own worktree is never deleted with `--orphans` | ✅ Passed |
| `--orphans` with zero orphans is the happy path, not an error | ✅ Passed |
| `--dry-run` shows the exact batch it would delete and deletes nothing | ✅ Passed |
| `--dry-run` with zero orphans is still the happy path | ✅ Passed |
| `--dry-run` without `--orphans` is a usage error | ✅ Passed |
| `--orphans` is exclusive with explicit paths | ✅ Passed |
| an unknown flag in `remove` is a usage error | ✅ Passed |
| a dash-leading token is a flag, never a path | ✅ Passed |
| `remove` with explicit paths keeps behaving as before | ✅ Passed |
| an orphan with an unresolvable `.git` link is deleted by explicit path | ✅ Passed |
| an orphan with an unresolvable `.git` does not take down the `--orphans` batch | ✅ Passed |
| `remove` without paths nor `--orphans` is still a usage error | ✅ Passed |
| an OK merge from prdash deletes the item's clean worktree | ✅ Passed |
| an OK merge with a dirty worktree keeps it and says so | ✅ Passed |
| a new untracked file also counts as dirty | ✅ Passed |
| on an unreadable git state the worktree is kept | ✅ Passed |
| an OK merge without a mounted worktree reports no error | ✅ Passed |
| if the worktree is no longer on disk, it is a no-op without error | ✅ Passed |
| the merge notice keeps all the truths at once | ✅ Passed |
| approve deletes no worktree | ✅ Passed |
| retarget deletes no worktree | ✅ Passed |
| a merge that does not go well deletes no worktree | ✅ Passed |
| closing the app deletes no worktree | ✅ Passed |
| a PR merged directly on the forge does not trigger the cleanup | ✅ Passed |
| no cleanup path touches a foreign worktree or one outside the root | ✅ Passed |
| the managed-root guard applies on both provisionings | ✅ Passed |
| a nonexistent path is rejected without touching anything | ✅ Passed |

30/30 scenarios of `behavior.feature` (single source of behavior; it is not
Cucumber). The real verification is the Go suite: the scenarios are translated
into unit/integration tests of `cmd/prdash`, `internal/worktree`,
`internal/tui` and `internal/review/executor`.

## Commits
History rewritten (History Finalization): 21 commits `merge-base..HEAD`
reduced to **7 groups** by behavior unit. Deterministic grouping — primary
(`feat`/`fix`/`refactor`/`perf`) grouped in contiguous runs by `scope`,
secondary (`docs`/`test`/`chore`/`ci`/`build`/`style`) folded into the
previous primary group (or into the next one if there is none yet), and a
secondary with a different `scope` closes the run. The shape was taken to the
fixed point (3 passes) and emitted in a single rebase.

Tree invariant: `58876b12cd13cd34c721c29a78c53534699fd462` identical before
(`3fd78e5`) and after (`b5de918`) the rewrite — the code is preserved byte by
byte; only messages and grouping change.

- `feat(worktree): remove a worktree only when it is clean`
  (folds `docs: add worktrees-cleanup plan (0004)`)
- `feat(worktrees): batch-remove orphans with --orphans and --dry-run`
- `feat(review): remove the active review worktree only when clean`
- `feat(tui): auto-remove a merged item's clean worktree`
  (folds `docs: document orphan cleanup and merge-time worktree removal`,
  `refactor(tui): clean up the worktree cleanup tests`,
  `test(worktree): cover HerdrNative.RemoveIfClean`)
- `fix(worktrees): remove orphans lacking a gitdir; time each batch`
  (folds `test(tui)`, `docs(worktrees)` and `docs:` of that stretch)
- `fix(worktree): apply the managed-root guard in the native removal path`
- `fix(tui): fire the cleanup notice once and update it in place`
  (folds the final `test(worktrees)` and `docs:`)

## Files
- Created: `docs/adr/0007-worktree-cleanup-on-merge.md`,
  `docs/planning/0004-feature-worktrees-cleanup/{behavior.feature,context.md,issue.md,plan.md}`,
  `docs/planning/0004-feature-worktrees-cleanup/diagrams/{feature-flow.md,process-flow.md}`
- Modified: `CHANGELOG.md`, `README.md`,
  `cmd/prdash/{main.go,worktrees.go,worktrees_test.go}`,
  `internal/cache/memo.go`, `internal/reporesolver/{reporesolver.go,reporesolver_test.go}`,
  `internal/review/executor/{executor.go,executor_test.go}`,
  `internal/tui/{app.go,app_test.go,merge_test.go,retarget.go,toast.go,toast_test.go,update.go}`,
  `internal/worktree/{audit_test.go,herdr.go,herdr_test.go,worktree.go,worktree_test.go}`

## Tests
- Added: 48 test functions (no new test file; the ones that grow are
  `cmd/prdash/worktrees_test.go` +393, `internal/tui/merge_test.go` +365,
  `internal/worktree/worktree_test.go` +145, `internal/worktree/herdr_test.go` +131)
- System Tests: ✅ Passed — `make test` (build + vet + gofmt + `go test -race ./...`)

## Documentation
- Changelog: ✅ Updated — two new entries in `### Added` of `[Unreleased]`
  (`prdash worktrees remove --orphans` and self-deletion of a merged PR's
  worktree only if it is clean)
- Docs: `README.md` (section `worktrees remove --orphans` and cleanup notices)
- ADR: ✅ Created — `docs/adr/0007-worktree-cleanup-on-merge.md`

## Code Review Issues
- Critical Found: 0
- High Found: 0
- User Decision: Approved to continue

## Next Step
Linear merge (`gh pr merge 21 --rebase`) onto `main` and post-merge cleanup
(`fetch --prune`, `branch -D`, worktree destruction via workspace-manager).
