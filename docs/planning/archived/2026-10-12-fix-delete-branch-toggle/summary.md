# Summary: delete-branch-toggle

## Metadata
- **Completed:** 2026-10-12 17:41
- **Duration:** ~140 minutes
- **Plan Number:** 0001

## Scenarios
| Scenario (behavior.feature) | Status |
|-----------------------------|--------|
| GitHub repo auto-deletes and the request is no — the confirmation says so | ✅ Passed |
| GitHub repo auto-deletes but the request is yes — no caveat | ✅ Passed |
| GitHub repo policy explicitly off and the request is no — no caveat | ✅ Passed |
| GitHub repo policy unknown and the request is no — no caveat | ✅ Passed |
| GitLab is unchanged | ✅ Passed |
| The request still drives the client-side delete flag | ✅ Passed |
| The toggle still turns off without disarming or firing | ✅ Passed |
| A narrow terminal degrades honestly | ✅ Passed |
| The policy travels with the item data | ✅ Passed |

## Commits
- fix(merge): stop the delete-branch toggle from lying on auto-delete repos
- chore: add the delete-branch-toggle plan
- test(parse): pin the auto-delete value when the repo reports it off
- test(tui): pin the auto-delete caveat boundaries, GitLab and geometry
- docs(planning): update progress and record the review-fix round
- test(github): assert the PR query asks for the auto-delete policy

## Files
- Created: docs/adr/0013-delete-branch-toggle-is-a-request.md, docs/planning/0001-fix-delete-branch-toggle/ (9 planning files), .ignore
- Modified: CHANGELOG.md, README.md, docs/FEATURES.md, internal/forge/github/github.go, internal/forge/github/merge_pin_test.go, internal/forge/model/model.go, internal/forge/parse/merge_fields_test.go, internal/forge/parse/parse.go, internal/tui/merge_delete_test.go, internal/tui/sections.go

## Tests
- Added: 9
- System Tests: ✅ Passed

## Documentation
- Changelog: ✅ Updated
- Docs: README.md, docs/FEATURES.md, ADR 0013
- ADR: No required (ADR 0013 already covers the decision)

## Code Review Issues
- Critical Found: 0
- High Found: 0
- User Decision: Approved to continue

## Next Step
Merge approval: integrate `fix/delete-branch-toggle` into `main` as change #29.
The sync rebase onto `main` restores the CI Mutation measurement — the last run
failed with "no merge base", a CI infrastructure artifact of `main` advancing
past the branch's merge-base — and integration waits for the fresh CI to go
green before merging.
