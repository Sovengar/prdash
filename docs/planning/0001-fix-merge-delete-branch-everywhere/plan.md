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

Phase: review

Review: 2026-10-12 — verdict **fix** — 0 critical, 3 high, 6 medium, 8 low (17 findings from the five-way review: behaviour + code/security/performance/docs lenses on `main...HEAD`). Verification read at synthesis time: `pending` (Lint ✓, Mutation skipped, Test/CI fast queued) — not awaited, not a gate.
