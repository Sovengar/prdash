---
feature: 0001-fix-delete-branch-toggle
freshness: 4e3fb8f0bd1c5478f10fb8d7e87e7b65a1be4b9a
codegraph: ready
generated_by: codebase-researcher
---

# Context: fix/delete-branch-toggle — stop the delete-branch toggle lying on auto-delete repos

## Scope
- In: validate the prototype (commit `4e3fb8f`) against `behavior.feature`; add the missing boundary/regression/geometry tests; close the `README.md` + `CHANGELOG.md` gaps; keep `docs/FEATURES.md` and ADR 0013 coherent; pass the gates and prepare the PR.
- Out: preventing the forge's delete, mutating the repo setting, hiding/repurposing the toggle, any GitLab behaviour change, any new ADR (ADR 0013 already covers this decision).

## Files to Touch
| Symbol / Area | File | Lines | Why |
|---------------|------|-------|-----|
| `deleteLabel(it model.Item)` | `/home/buble/dev/projects/prdash/.worktrees/prdash.fix-delete-branch-toggle/internal/tui/sections.go` | 178-190 | The caveat conditional `it.AutoDeleteKnown && it.AutoDeleteBranch` (L186) is the fix and the prime mutation target (`&&`→`||` mutant must be killed by a policy-known-off test). Pure label helper — validate, don't reshape. |
| `mergeConfirmText()` | `/home/buble/dev/projects/prdash/.worktrees/prdash.fix-delete-branch-toggle/internal/tui/sections.go` | 161-176 | Passes `it` into `deleteLabel`; the head line `merge <ref>? <label>` is where the caveat surfaces. |
| `hintLines` / `wrapHint` / `keybindsSection` | `/home/buble/dev/projects/prdash/.worktrees/prdash.fix-delete-branch-toggle/internal/tui/sections.go` | 132-159 | The confirmation is the bottom Keybinds bar, clipped all-or-nothing at `maxHintLines = 3` (`/home/buble/dev/projects/prdash/.worktrees/prdash.fix-delete-branch-toggle/internal/tui/layout.go:8`). The caveat lengthens the head line — narrow-terminal geometry must be pinned by a test. |
| `TestConfirmSaysWhenTheRepoAutoDeletes` + neighbours | `/home/buble/dev/projects/prdash/.worktrees/prdash.fix-delete-branch-toggle/internal/tui/merge_delete_test.go` | 10-189 | Existing coverage: request-on ⇒ no caveat (L73-76), auto-delete-on + request-off ⇒ caveat (L78-85). MISSING: policy-known-off boundary (kills `||` mutant), GitLab regression, narrow-width geometry. |
| `autoDeleteFromGHRepo` | `/home/buble/dev/projects/prdash/.worktrees/prdash.fix-delete-branch-toggle/internal/forge/parse/parse.go` | 204-211 | Nil-pointer ⇒ `(false, false)`; the nil check (L207) and the value return (L210) are mutation targets. |
| `ghPRNodeRepository.DeleteBranchOnMerge` | `/home/buble/dev/projects/prdash/.worktrees/prdash.fix-delete-branch-toggle/internal/forge/parse/parse.go` | 60-70 | `*bool` at L69 — the unknown-vs-off distinction lives here. |
| `itemFromGHNode` | `/home/buble/dev/projects/prdash/.worktrees/prdash.fix-delete-branch-toggle/internal/forge/parse/parse.go` | 165-190 | Wiring at L186: `it.AutoDeleteBranch, it.AutoDeleteKnown = autoDeleteFromGHRepo(n.Repository)`. |
| `TestGHNodeCarriesTheAutoDeletePolicy` / `TestGHNodeWithoutAutoDeletePolicyStaysUnknown` | `/home/buble/dev/projects/prdash/.worktrees/prdash.fix-delete-branch-toggle/internal/forge/parse/merge_fields_test.go` | 65-102 | Pattern for parse field tests (raw JSON → `ParseGHGraphQLSearch` → assert `items[0]`). MISSING: `deleteBranchOnMerge: false` ⇒ known=true, value=false (kills value-flip mutants). |
| `Item.AutoDeleteBranch` / `Item.AutoDeleteKnown` | `/home/buble/dev/projects/prdash/.worktrees/prdash.fix-delete-branch-toggle/internal/forge/model/model.go` | 106-134 | The two flags at L129-133 with the WHY comment; the contract every path must respect. |
| `Adapter.Merge` (GitHub) | `/home/buble/dev/projects/prdash/.worktrees/prdash.fix-delete-branch-toggle/internal/forge/github/github.go` | 132-146 | `--delete-branch` only when `req.DeleteBranch` (L142-144) — the flag stays honest; must NOT change. |
| `ghPRFields` | `/home/buble/dev/projects/prdash/.worktrees/prdash.fix-delete-branch-toggle/internal/forge/github/github.go` | 274 | `deleteBranchOnMerge` inside the existing `repository { … }` block — no extra round-trip. Also used by `prQuery` (L289-294), so the single-item path carries the policy too. |
| `Adapter.Merge` (GitLab) | `/home/buble/dev/projects/prdash/.worktrees/prdash.fix-delete-branch-toggle/internal/forge/gitlab/gitlab.go` | 143-158 | `--remove-source-branch` only when `req.DeleteBranch` (L154-156) — must NOT change. GitLab items never set the AutoDelete flags (`itemFromGLMR`, `/home/buble/dev/projects/prdash/.worktrees/prdash.fix-delete-branch-toggle/internal/forge/parse/parse.go:497+`). |
| `handleMergeArmed` tab case | `/home/buble/dev/projects/prdash/.worktrees/prdash.fix-delete-branch-toggle/internal/tui/update.go` | 314-337 | Tab toggle at L323-325 flips `m.deleteBranch` and returns without disarming/firing — semantics untouched. |
| `deleteBranch` field + default | `/home/buble/dev/projects/prdash/.worktrees/prdash.fix-delete-branch-toggle/internal/tui/app.go` | 204, 249 | Field at L204; default `deleteBranch: true` at L249 in `New`. |
| "The merge also names whether the branch is deleted" | `/home/buble/dev/projects/prdash/.worktrees/prdash.fix-delete-branch-toggle/README.md` | 263-303 | Still describes the toggle as an unconditional promise; add the GitHub auto-delete caveat WITHOUT contradicting the "three endings" table (L281-285). Also L79-80 (`tab` toggles whether the branch is deleted). |
| `[Unreleased] / ### Fixed` | `/home/buble/dev/projects/prdash/.worktrees/prdash.fix-delete-branch-toggle/CHANGELOG.md` | 8, 28 | Record the fix entry here. |
| Branch-deletion entry | `/home/buble/dev/projects/prdash/.worktrees/prdash.fix-delete-branch-toggle/docs/FEATURES.md` | 56 | Prototype already updated it — coherence check only. |
| ADR 0013 | `/home/buble/dev/projects/prdash/.worktrees/prdash.fix-delete-branch-toggle/docs/adr/0013-delete-branch-toggle-is-a-request.md` | 1-57 | Already exists and matches the code (Decision L24-35, Consequences L37-46) — coherence check only; do NOT author a new ADR. |

## Contracts
- `model.Item` field shape — `AutoDeleteBranch bool` + `AutoDeleteKnown bool` (`/home/buble/dev/projects/prdash/.worktrees/prdash.fix-delete-branch-toggle/internal/forge/model/model.go:129-133`). `AutoDeleteKnown == false` is a third state (unknown) distinct from "off"; no path may read it as a guarantee.
- GitHub GraphQL query — `ghPRFields` (`/home/buble/dev/projects/prdash/.worktrees/prdash.fix-delete-branch-toggle/internal/forge/github/github.go:274`): `deleteBranchOnMerge` rides inside the existing `repository { … }` block; no extra round-trip, no new query.
- `forge.MergeRequest{Mode, HeadSHA, DeleteBranch}` (`/home/buble/dev/projects/prdash/.worktrees/prdash.fix-delete-branch-toggle/internal/forge/forge.go:60-64`) and the `Adapter.Merge(ctx, ref, number, req) []model.Warning` signature (`/home/buble/dev/projects/prdash/.worktrees/prdash.fix-delete-branch-toggle/internal/forge/forge.go:55`) — implemented by github (`github.go:132`), gitlab (`gitlab.go:143`), bitbucket (`/home/buble/dev/projects/prdash/.worktrees/prdash.fix-delete-branch-toggle/internal/forge/bitbucket/bitbucket.go:49`). Signature must not change.
- GitLab must not change: no AutoDelete fields set in `itemFromGLMR`, no caveat for GitLab items, merge args untouched.

## Pattern to Follow
- Pure TUI label/copy helper → `deleteLabel(it model.Item)` at `/home/buble/dev/projects/prdash/.worktrees/prdash.fix-delete-branch-toggle/internal/tui/sections.go:178-190`: a pure read of the request + the two policy flags returning a string; no I/O, no gate logic, no control flow. Any new copy follows this shape.
- Parse field tests → `/home/buble/dev/projects/prdash/.worktrees/prdash.fix-delete-branch-toggle/internal/forge/parse/merge_fields_test.go:65-102`: raw JSON literal → `ParseGHGraphQLSearch(raw)` → assert on `items[0]` fields, with a one-line WHY comment per test.
- TUI tests → `/home/buble/dev/projects/prdash/.worktrees/prdash.fix-delete-branch-toggle/internal/tui/merge_delete_test.go`: `newMergeFixture(t, items...)` (`/home/buble/dev/projects/prdash/.worktrees/prdash.fix-delete-branch-toggle/internal/tui/merge_test.go:31-42`), `mkItem` (`/home/buble/dev/projects/prdash/.worktrees/prdash.fix-delete-branch-toggle/internal/tui/app_test.go:30-44`), `press` (`app_test.go:67-71`), `send` (`app_test.go:61-65`), `stripANSI` (`/home/buble/dev/projects/prdash/.worktrees/prdash.fix-delete-branch-toggle/internal/tui/table.go:279`), `waitOutcome` (`merge_test.go:48-56`). Fixture terminal is 160×40 (`app_test.go:24`); `newMergeFixture` wires a github `FakeAdapter` only — a GitLab TUI test needs a gitlab `FakeAdapter` via `newTestModel` + `page(...)` as in `app_test.go:140-148`.
- Hint wrapping / narrow-terminal geometry → `wrapHint` (`sections.go:151-159`) → `wrapText` (`/home/buble/dev/projects/prdash/.worktrees/prdash.fix-delete-branch-toggle/internal/tui/toast.go:184-202`), capped by `maxHintLines = 3` (`/home/buble/dev/projects/prdash/.worktrees/prdash.fix-delete-branch-toggle/internal/tui/layout.go:8`), width floor `max(38, outerWidth-2)` (`/home/buble/dev/projects/prdash/.worktrees/prdash.fix-delete-branch-toggle/internal/tui/update.go:563-565`), sliced in `keybindsSection` (`sections.go:132-136`). Clipping is all-or-nothing by design; pin it with a test.

## Tests
- Existing affected: `/home/buble/dev/projects/prdash/.worktrees/prdash.fix-delete-branch-toggle/internal/tui/merge_delete_test.go` (all 9 tests), `/home/buble/dev/projects/prdash/.worktrees/prdash.fix-delete-branch-toggle/internal/forge/parse/merge_fields_test.go` (L65-102), `/home/buble/dev/projects/prdash/.worktrees/prdash.fix-delete-branch-toggle/internal/tui/merge_test.go`, `/home/buble/dev/projects/prdash/.worktrees/prdash.fix-delete-branch-toggle/internal/tui/layout_cascade_test.go`, `/home/buble/dev/projects/prdash/.worktrees/prdash.fix-delete-branch-toggle/internal/tui/layout_invariants_test.go`.
- Framework / runner: `make test` (build + vet + gofmt gate + `go test -race -count=1`). Baseline verified GREEN at `4e3fb8f` (all 20 packages ok). `make check` adds golangci-lint v2.13.2.
- Coverage gate: `scripts/diff-coverage.sh` — the PR's diff at 100% AND the total ≥ `scripts/coverage-floor` (100.00, measured 3231/3231). Local equivalent: `make coverage-check`.
- Mutation gate: `make mutate-diff` (advisory locally, BLOCKING in CI on non-draft PRs). `.mutation-allowlist` is EMPTY ON PURPOSE — a survivor must be killed by a test or refactored away, not allowlisted. `.mutation-timeouts` records expired mutants.
- Integration infra: unit only — git fixtures in `t.TempDir()` via `internal/testutil`, fake adapters; no `gh`/`glab`/`herdr` needed.
- Suggested targets (scenario → test area):
  - "policy explicitly off + request no ⇒ no caveat" → NEW TUI test in `merge_delete_test.go`: GitHub item with `AutoDeleteKnown=true, AutoDeleteBranch=false`, `tab`, assert `delete branch: no` present and `auto-deletes` absent. **This is the test that kills the `AutoDeleteKnown && AutoDeleteBranch` → `||` mutant in `deleteLabel` (sections.go:186) — mandatory.**
  - "policy unknown + request no ⇒ no caveat" → NEW TUI test: GitHub item with both flags false (default `mkItem`), `tab`, assert no caveat.
  - "GitLab is unchanged" → NEW TUI test: GitLab item (`mkItem("gitlab", ...)`), `tab`, assert `delete branch: no` and no `auto-deletes`; plus the existing `TestTabToggleReachesTheAdapter` pattern for `--remove-source-branch` (GitLab merge args already covered by `gitlab.go:154-156` — a parse-level assertion that `itemFromGLMR` leaves `AutoDeleteKnown` false also fits `merge_fields_test.go`).
  - "policy present with value false ⇒ known=true, value=false" → NEW parse test in `merge_fields_test.go` with `"deleteBranchOnMerge":false` (kills value-flip mutants in `autoDeleteFromGHRepo`).
  - "narrow terminal degrades honestly" → NEW geometry test: arm the merge on an auto-delete item with the request off, shrink `m.width`, assert the Keybinds box is clipped at ≤ `maxHintLines` with no corrupted/overwritten text (pattern: `layout_cascade_test.go` / `sim_test.go:233-261` width assertions).
  - "request still drives the client-side delete flag" → already covered by `TestTabToggleReachesTheAdapter` / `TestDeleteBranchIsOnByDefault` (`merge_delete_test.go:28-104`).
  - "toggle turns off without disarming or firing" → already covered by `TestTabTogglesTheDeleteWithoutDisarming` (L46-63).
  - "policy travels with the item data" → already covered by `TestGHNodeCarriesTheAutoDeletePolicy` (parse level) + `ghPRFields` sharing (both `searchQuery` and `prQuery` embed it).

## Conventions & Boundaries
- `internal/` exclusively — no exported packages.
- English everywhere; comments only when they explain the WHY, compressed to one line (`/home/buble/dev/projects/prdash/.worktrees/prdash.fix-delete-branch-toggle/docs/adr/0009-comments-in-english.md`, `/home/buble/dev/projects/prdash/.worktrees/prdash.fix-delete-branch-toggle/docs/adr/0010-everything-in-english.md`).
- No business logic in the render: decisions go in `internal/<subsystem>/`, the TUI paints. `deleteLabel` is a pure string read, not a decision — keep it that way.
- Honest TUI degradation: a box that does not fit is not painted halfway; the floor arithmetic lives next to the guard.
- `make test` must remain the gate: gofmt-clean, `go vet`, suite with `-race`. No new dependencies (`charm.land/*` v2 import paths, never `github.com/charmbracelet`).
- Docs rule: any user-visible change updates `docs/FEATURES.md` AND the global skill `~/.agents/skills/prdash/SKILL.md` in the same change. **The skill is OUT-OF-ENV (orchestrator-owned): `/home/buble/.agents/skills/prdash/SKILL.md:17` says "`tab` toggles delete-branch" with no caveat — note it in the PR/summary; do NOT plan a subagent for it.**
- ADR 0013 already exists for exactly this decision — keep it coherent; a second ADR would duplicate it.
- After code changes: `make install` (the user runs `~/.local/bin/prdash`, not the repo).

## Integration Points (non-obvious)
- The merge confirmation is NOT a popup: it is the bottom Keybinds bar (`keybindsSection` → `hintLines` → `wrapHint(mergeConfirmText(), …)`), clipped at `maxHintLines = 3`. The caveat lengthens the head line, so on a narrow terminal the `press the mode: … · esc cancel` tail can be clipped — honest by design, but the geometry must be pinned by a test so the box never corrupts.
- The policy reaches the UI purely via `model.Item`: `pageMsg` → inbox → list/detail all carry it; no adapter call, no second source of truth. Both GitHub query paths embed `ghPRFields` (`searchQuery` L276-287 and `prQuery` L289-294), so search and single-item refreshes both report it.
- Mutation-gate reality: the `&&` at `sections.go:186` and the nil check at `parse.go:207` are exactly the short-circuit/boundary mutants gremlins produces. Without the policy-known-off TUI test and the `deleteBranchOnMerge:false` parse test, those mutants survive and the CI `Mutation` job kills the PR. `.mutation-allowlist` being empty on purpose means "test it or refactor it", not "file it".
- `newMergeFixture` is GitHub-only (`ghAdapter()`); GitLab TUI coverage needs the `newTestModel` + `page(...)` wiring shown in `app_test.go:140-148`.

## Risks / Assumptions
- The plan is FEASIBLE against the current code: the prototype at `4e3fb8f` matches the spec (caveat only when request off AND policy known-on; GitLab untouched; flag still drives `--delete-branch`), and `make test` is green at the branch tip.
- Mutation gate is the main CI risk — the two mandatory boundary tests above are what make it pass; treat them as part of the fix, not optional extras.
- Narrow-terminal clipping: if the caveat makes the confirmation tail disappear below the width floor (`contentWidth` = `max(38, outerWidth-2)`), decide with the user whether a shorter caveat is warranted — the current wording is the approved one.
- README wording trap: keep the "three endings" table (L281-285) intact; the change is about the toggle's *promise*, not the post-merge notice.
- Stale global skill: if the orchestrator does not update `~/.agents/skills/prdash/SKILL.md`, the runtime contract agents load stays wrong — surface it in the PR/summary rather than silently skipping it.
