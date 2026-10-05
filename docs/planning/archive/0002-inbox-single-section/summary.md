# Summary: inbox-single-section

## Metadata
- **Completed:** 2026-09-26 22:01
- **Duration:** ~35 minutes (closing)
- **Plan Number:** 0002
- **PR:** [#17](https://github.com/Sovengar/prdash/pull/17) — `feat/inbox-single-section` → `main`

## Scenarios
| Scenario (behavior.feature) | Status |
|-----------------------------|--------|
| On opening, the active section is Assigned | ✅ Passed |
| Only one section is painted at a time | ✅ Passed |
| tab cycles Assigned → Mentioned → Mine → Assigned | ✅ Passed |
| tab also cycles when a section is empty | ✅ Passed |
| The cycle key comes from the config, not from a hardcoded "tab" | ✅ Passed |
| The top-left border legend replaces the "Inbox" title | ✅ Passed |
| The active section is highlighted in the legend | ✅ Passed |
| The legend counts reflect the deduped section | ✅ Passed |
| The common prefix of the active section is still shown | ✅ Passed |
| The prefix shown is the active section's one | ✅ Passed |
| A section without common prefix does not invent one | ✅ Passed |
| The prefix does not break the table width | ✅ Passed |
| Each section remembers its cursor and its scroll | ✅ Passed |
| A refresh keeps the position of each section | ✅ Passed |
| The empty active section is marked as empty | ✅ Passed |
| The "loading more…" belongs to the active section | ✅ Passed |
| Query warnings are shown for the active section | ✅ Passed |
| The --print mode does not change | ✅ Passed |
| On a narrow terminal the legend is truncated without breaking the box | ✅ Passed |

19/19 scenarios of `behavior.feature` (single source of behavior; it is not
Cucumber). The real verification is the Go suite: the scenarios are translated
into unit/integration tests of `internal/tui` and `internal/forge/model`.

## Commits
History rewritten (History Finalization, deterministic grouping by
type/scope with a tree-hash `092e103` invariant identical before/after):

- `feat(model): add short section labels for the inbox`
  (folds `docs(planning): add 0002 inbox single-section plan` +
  `feat(model): add Section.Legend() for short section labels`)
- `feat(tui): show only the active inbox section`
  (folds `test(tui)`, `refactor(tui)` and the `docs(adr)` ×2,
  `docs(changelog)`, `docs(readme)` of the branch)

## Files
- Created: `docs/adr/0004-inbox-single-section.md`,
  `docs/planning/0002-inbox-single-section/{issue.md,behavior.feature,plan.md,context.md}`,
  `docs/planning/0002-inbox-single-section/diagrams/{feature-flow.md,process-flow.md}`,
  `internal/tui/section_test.go`
- Modified: `internal/tui/{app.go,list.go,refcol.go,sections.go,styles.go,update.go}`,
  `internal/forge/model/model.go`, `README.md`, `CHANGELOG.md`,
  and their tests (`app_test.go`, `list_test.go`, `refcol_test.go`, `table_test.go`,
  `comments_test.go`, `mount_test.go`, `auth_reason_test.go`,
  `sim_test.go`, `sim_integration_test.go`, `model_test.go`)

## Tests
- Added: 17 test functions (1 new file: `internal/tui/section_test.go`,
  353 lines)
- System Tests: ✅ Passed — `make test` → `go build` + `go vet` + `gofmt` +
  `go test -race ./...`, exit 0 (run before and after the history rewrite;
  byte-identical tree)

## Documentation
- Changelog: ✅ Updated — entry in `## [Unreleased]` → `### Changed`
  (commit `docs(changelog)`, no reconciliation needed at closing)
- Docs: `README.md` (default section and count legend),
  `docs/planning/0002-inbox-single-section/*`
- ADR: ✅ Created — `docs/adr/0004-inbox-single-section.md` (Accepted),
  supersedes two clauses of ADR 0002 (prefix in the section header and
  the «all sections» scope of the ITEM width)

## Code Review Issues
- Critical Found: 0
- High Found: 0
- User Decision: Approved to continue

## Next Step
Merge of PR #17 with `gh pr merge --rebase` (linear history, no squash), then
branch/worktree cleanup. Post-merge: the user runs `make install` (expressly
out of scope of this closing). Declared pending in `plan.md`: the
selectable/toggleable path prefix is a later feature (here only the seam was
left).
