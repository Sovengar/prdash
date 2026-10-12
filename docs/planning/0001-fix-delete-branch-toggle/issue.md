Status: approved

# 0001 — The delete-branch toggle stops lying on auto-delete repos (fix)

Source: task #16 (prdash) — "El toggle 'no borrar rama' (tab) no tiene efecto".
This is a **promoted** run: the prototype is already implemented on
`fix/delete-branch-toggle` (commit `4e3fb8f`) and the user approved its
direction. The work below is **validate the prototype against this spec, close
the gaps, meet the gates and prepare the PR** — not build from scratch.

## Problem

In the merge confirmation, `tab` toggles `delete branch: yes|no`
(`internal/tui/update.go:323`, default `yes` in `internal/tui/app.go:249`, shown
in `internal/tui/sections.go:178`). On a GitHub repository with
**"Automatically delete head branches"** enabled, setting it to `no` is **not
honoured**: GitHub deletes the head branch on merge regardless of the flag.
There the toggle promised something the forge will not keep — the bug.

The mechanism, **verified this session** (not assumed):

- GitHub GraphQL exposes `deleteBranchOnMerge` as a real repository field:
  `gh api graphql '{repository(owner:"cli",name:"cli"){deleteBranchOnMerge}}'`
  → `true`.
- Upstream [`cli/cli#11508`](https://github.com/cli/cli/issues/11508) (closed)
  confirms it: `--delete-branch=false` does **not** prevent branch deletion on
  merge; no client flag bypasses the repository setting.
- The GitHub adapter only appends `--delete-branch` when the toggle is `yes`
  (`internal/forge/github/github.go:142`), so the flag itself is honest — the
  lie is at the forge level.
- **GitLab is different and is left untouched**: the adapter sends
  `--remove-source-branch` only when the toggle is `yes`; GitLab's project
  setting only sets the UI default, while the per-MR flag controls behaviour, so
  the toggle is honoured there.

**The fix is about honest signalling, not enforcement.** On a GitHub repo that
auto-deletes *and* the toggle is off, the branch is deleted no matter what —
prdash **cannot** "keep the no". Turning the repo setting off (or restoring it)
passes the explicit API call and needs repository admin: out of scope (see ADR
0013, rejected alternative). What prdash can do is stop claiming a "no" it
cannot deliver.

## What already landed (prototype, the starting point)

Commit `4e3fb8f` on the branch tip already implements the chosen direction:

- `internal/forge/model/model.go` — `AutoDeleteBranch bool` +
  `AutoDeleteKnown bool` on `model.Item`.
- `internal/forge/parse/parse.go` — `DeleteBranchOnMerge *bool` on the GH repo
  node; `autoDeleteFromGHRepo` (nil pointer ⇒ `known = false`); wired into
  `itemFromGHNode`.
- `internal/forge/github/github.go` — `deleteBranchOnMerge` added to the
  `ghPRFields` GraphQL query; the merge arg stays `--delete-branch` only when
  the toggle is yes.
- `internal/tui/sections.go` — `deleteLabel(it model.Item)` appends
  ` — repo auto-deletes head branches` when `AutoDeleteKnown && AutoDeleteBranch`
  and the toggle is off. The `yes` branch, an unknown policy and a policy-off
  repo are unchanged.
- Tests — `internal/forge/parse/merge_fields_test.go` (policy present ⇒
  known + value; absent ⇒ unknown) and
  `internal/tui/merge_delete_test.go::TestConfirmSaysWhenTheRepoAutoDeletes`.
- Docs — `docs/FEATURES.md` updated; new
  `docs/adr/0013-delete-branch-toggle-is-a-request.md`.

## Scope (in) — the remaining work

1. **Validate the prototype against this spec.** Confirm the caveat fires only
   when it should (auto-delete on **and** toggle off), and that the compact
   label and the ADR consequences match the code.
2. **`README.md` gap (prototype did NOT touch it).** The section
   `### The merge also names whether the branch is deleted` (~L263-299)
   describes the toggle as an unconditional promise and does not mention the
   GitHub auto-delete caveat. Make it consistent with the new behaviour: on
   auto-delete repos the toggle no longer reads as "the branch survives".
3. **`CHANGELOG.md` gap (prototype did NOT touch it).** Record the fix under
   `## [Unreleased]` / `### Fixed`.
4. **ADR coherence.** Keep ADR 0013 consistent with the final code and note that
   it is the required new ADR (observable behaviour changed).
5. **Gates.** `make test` green (build + vet + gofmt + `go test -race`), diff
   coverage at 100% and total at/above `scripts/coverage-floor`, and the
   `Mutation` gate clean (any new survivor allowlisted with its reason, or
   killed).
6. **Prepare the PR** from `fix/delete-branch-toggle` to `main`.
7. **Out-of-env step owned by the ORCHESTRATOR, not a subagent.** AGENTS.md
   requires updating the global skill `~/.agents/skills/prdash/SKILL.md` for any
   user-visible change. That file lives **outside** this working environment;
   the orchestrator updates it. It must reflect the merge-flow caveat.

## Out of scope (out)

- **Do NOT prevent the forge's delete.** The toggle cannot keep a "no" against a
  GitHub repo that auto-deletes; the fix is the truthful label, not enforcement.
- **Do NOT mutate the repo setting** (GraphQL mutation to disable/restore
  auto-delete before merge). Needs admin, races other merges: rejected in ADR
  0013.
- **Do NOT hide or repurpose the toggle.** It still controls the client-side
  delete and the explicit API call, so removing it would drop real capability.
- **Do NOT touch GitLab behaviour** (per-MR flag is genuine; policy reported as
  unknown, no caveat).
- **Do NOT invent a caveat when the policy is unknown** (`AutoDeleteKnown ==
  false`): not knowing is not the same as off.

## Acceptance criteria

- [ ] On a GitHub repo with `deleteBranchOnMerge` on and the toggle **off**, the
      confirmation shows the caveat (`repo auto-deletes head branches`) instead
      of promising "no".
- [ ] On that same repo with the toggle **on**, on a repo with the policy
      **off**, and when the policy is **unknown**, the confirmation is
      unchanged.
- [ ] The GitHub merge command still passes `--delete-branch` only when the
      toggle is yes; the policy is read from the existing `repository { … }`
      block with no extra round-trip.
- [ ] GitLab behaviour and its label are unchanged (per-MR flag honoured, policy
      unknown ⇒ no caveat).
- [ ] `README.md` (the delete-branch section) no longer describes the toggle as
      an unconditional promise and explains the GitHub auto-delete caveat.
- [ ] `CHANGELOG.md` records the fix under `[Unreleased] / Fixed`.
- [ ] `docs/FEATURES.md` and ADR 0013 are consistent with the final code.
- [ ] `make test` green (build + vet + gofmt + `go test -race`).
- [ ] Diff coverage 100% and total ≥ `scripts/coverage-floor`; `Mutation` gate
      clean (survivors killed or allowlisted with a reason).
- [ ] PR from `fix/delete-branch-toggle` to `main` prepared.

## Risks / pending verifications

- **Verify the caveat boundaries by test**, not by reading: auto-delete
  on/off/unknown × toggle on/off. A wrong branch of `deleteLabel` is the failure
  this fix exists to avoid.
- **Mutation gate**: the new conditional in `deleteLabel` / `autoDeleteFromGHRepo`
  is exactly the kind of logic a boundary/short-circuit mutant targets. Expect
  to kill it or allowlist it with a documented reason.
- **README wording trap**: keep the "three endings" table (deleted / delete
  refused / fork PR) intact and add the caveat without contradicting it.
- **Global skill** is out of the working environment: if it is not updated by
  the orchestrator, the runtime contract agents load stays stale — flag it in
  the PR/summary rather than silently skipping it.
