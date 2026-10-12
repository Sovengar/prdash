# 0001 — fix/merge-delete-branch-everywhere — Plan

Branch: `fix/merge-delete-branch-everywhere` (change 30, promoted prototype).
Spec: `behavior.feature`. Brief: `prototype-brief.md`.
ADRs: `docs/adr/0013-*`, `docs/adr/0014-*`.

## Scenarios

| # | Scenario | Commits |
|---|----------|---------|
| 1 | A successful merge deletes the branch in every clone | `c64bfcc`, `9ef6ed2` |
| 2 | A dirty worktree keeps its branch | `c64bfcc`, `f32f938` |
| 3 | A conflicting PR is refused before arming | `2809503`, `45c137c`, `9ef6ed2` |
| 4 | An unknown mergeability is never painted as mergeable | `2809503`, `9ef6ed2` |
| 5 | Deleting a branch fails without failing the merge | `c64bfcc`, `9ef6ed2` |

Refactor commits (no scenario of their own): `b00dc5b`, `1e9a169`.

## Progress

Phase: close

Close: started 2026-10-12

Review: 2026-10-12 — round 1, verdict **fix** — 0 critical, 3 high, 6 medium, 8 low (17 findings from the five-way review: behaviour + code/security/performance/docs lenses on `main...HEAD`). Verification read at synthesis time: `pending` (Lint ✓, Mutation skipped, Test/CI fast queued) — not awaited, not a gate.

Review: 2026-10-12 — round 2 (delta `029b868..HEAD`), verdict **fix** — 0 critical, 0 high, 1 medium, 7 low. Round-1 MEDIUM/HIGH findings confirmed resolved (`50fbab4`, `5f294f7`, `f02e937`, `24046df`, `30e8348`); accepted LOWs (tab toggle, attempt cap, 2 gh spawns) recorded as product decisions. Remaining MEDIUM: `isDefaultBranch` (`internal/reporesolver/reporesolver.go`) fails open when `refs/remotes/origin/HEAD` is absent (bare cache clones, `git remote add` clones), contradicting the unconditional "never deletes the local default branch" claim in f02e937/ADR 0013/README/FEATURES. Verification read: `passing` (Lint, Test, CI fast ✓; Mutation skipped).

Review: 2026-10-12 — round 3 (delta `6a552bb..HEAD`, `2f65564`), verdict **fix** — 1 critical, 0 high, 1 medium, 5 low. Round-2 MEDIUM (`isDefaultBranch` fail-open) confirmed fixed: `(bool, error)`, fail closed, bare-clone HEAD fallback, existence-before-guard ordering, tests for failing `symbolic-ref` and bare clone; docs claims now unconditionally accurate. NEW CRITICAL: `reporesolver.go:319-321` (bare clone whose own HEAD is unreadable) is uncovered — the 100% diff-coverage gate measures red (behaviour lens ran `scripts/diff-coverage.sh` → 99.21%, exit 1), so a required CI check blocks the merge. MEDIUM: ADR 0013 does not record the new fail-closed warning or the bare-HEAD fallback (Decision 4 / Consequences / Verification). LOWs: ADR bare-fallback clause, CHANGELOG Fixed entry for the new warning, positive-path pinning test, `~/.agents/skills/prdash/SKILL.md:19` (outside repo), `behavior.feature` + `prototype-brief.md` still untracked in the planning dir. REFUTED: code lens's claim that `defaultName` strips `"origin"` without the slash — read at HEAD, `reporesolver.go:335` is `TrimPrefix(name, "origin/")`, and the behaviour lens's full suite run is green. Accepted LOWs stand (tab toggle, attempt cap, 2 gh spawns). Verification read: `pending` (Lint/Test/CI fast queued; Mutation skipped) — not awaited, not failing.

Review: 2026-10-12 — round 4 (delta `21778bc..HEAD`: `d7f1060`, `9096136`, `ff84ad0`), verdict **PASS** — 0 critical, 0 high, 0 medium, 4 low. All round-3 findings resolved: CRITICAL coverage (`d7f1060` test executes `reporesolver.go:319-321`, coverprofile block `319.17,321.4 1 1`; `make coverage-check` diff 100.00% / total 100.00% vs merge-base c69beeb, `make test` green), ADR 0013 Decision 4 / Consequences / Verification (`9096136`), CHANGELOG `Fixed` entry + positive-path test + planning artifacts committed (`ff84ad0`). Delta contains zero production code (tests + docs only) — the fail-closed guard is byte-identical to round 3, where the security lens verdict was "no fail-open path". LOWs (4, recorded): `behavior.feature` has no explicit unreadable-default scenario; Spanish verbatim quote in `prototype-brief.md:8` (labelled quotation); broken line ref `prototype-brief.md:31` (`:237 ReviewBranch` → `:377`); ADR 0013 Decision 4 enumeration order swapped vs code. Security lens round 4 returned `blocked` (mis-transcribed working path + denied shell); its question — does the delta weaken the guard — is answered by the zero-production-code diff and the round-3 security verdict. SKILL.md handled by the orchestrator. **Change opened for integration** (`publish-change --open` → `open_for_integration: true`); verification `passing` (CI fast ✓).
