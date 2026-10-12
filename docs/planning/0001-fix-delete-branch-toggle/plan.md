# Plan — fix/delete-branch-toggle (task #16)

- Branch: `fix/delete-branch-toggle`
- Type: fix · Slug: `delete-branch-toggle`
- Issue: `issue.md` (Status: approved) · Behavior: `behavior.feature` (approved)
- Prototype: `4e3fb8f` already on the branch tip — the pipeline's starting point.
- `adr_required: false` — observable behaviour changed, but ADR 0013 already
  exists and covers exactly this decision; the scope item is keeping it
  coherent, not authoring a new ADR (a second one would duplicate it).

## Intended outcome

A GitHub repo with "automatically delete head branches" enabled deletes the head
branch on merge whatever the client asks. Today the confirmation still offers
`delete branch: no` there, promising something the forge will not keep. After the
fix the toggle stays compact (`delete branch: yes|no (tab)`) but, when the
request is `no` **and** the repo is known to auto-delete, the confirmation names
the policy instead of implying the branch survives. The fix is honest signalling,
not enforcement: prdash cannot keep a `no` there and does not pretend to. Every
other combination — request `yes`, policy off, policy unknown, GitLab — keeps
today's exact output.

## Approach (high level)

- Carry the repository's auto-delete policy on the item, beside the existing
  merge facts, read from the query the inbox already runs (no extra round-trip).
  "Unknown" is a third state distinct from "off": an absent field must never be
  read as a guarantee.
- Compose the confirmation label from `(request, policy known, policy on)`; the
  caveat appears only in the one true case.
- Leave the merge command and the GitLab path untouched: the request still drives
  the client-side delete flag exactly as before.
- Close the two documentation gaps the prototype left: `README.md` (the
  delete-branch section) and `CHANGELOG.md` (`[Unreleased] / Fixed`), keeping
  ADR 0013 and `docs/FEATURES.md` coherent with the final code.

## Key decisions

- **State home**: the policy lives on the item beside the merge rules; both list
  and detail paths already carry it, so no new adapter call and no second source
  of truth.
- **Where the wording is chosen**: the confirmation label stays a TUI copy
  concern (a pure read of the request + the two policy flags). No gate logic, no
  I/O — it is a string, not a decision. If it ever needs the forge or changes
  control flow, it moves out of the TUI.
- **Unknown ≠ off**: the caveat is suppressed when the policy is unknown. Not
  knowing must not invent a warning.
- **GitLab untouched**: its per-merge-request flag is genuinely honoured, so it
  gets no caveat and no behaviour change.

## Gaps to close (the prototype did not)

- `README.md` "The merge also names whether the branch is deleted" — it still
  describes the toggle as an unconditional promise; add the GitHub auto-delete
  caveat without contradicting the existing "three endings" table.
- `CHANGELOG.md` — record the fix under `[Unreleased] / Fixed`.
- **Out-of-env step (orchestrator, not a subagent)**: `AGENTS.md` requires the
  global skill `~/.agents/skills/prdash/SKILL.md` to change with any user-visible
  feature. It lives outside this working environment, so the orchestrator updates
  it; the executor must not attempt it and should flag it in the PR/summary so it
  is not silently skipped.

## Risks

- **Mutation gate**: the two-flag short-circuit in the label and the nil check in
  the parser are exactly the targets of a boundary/short-circuit mutant. With no
  test for the policy-*known-off* case, the `||` mutant survives and kills the
  PR — that boundary test is mandatory, and the GitLab regression test pins
  "unchanged". Any survivor that is genuinely equivalent goes to the allowlist
  with its reason.
- **Narrow-terminal clipping**: the caveat lengthens the confirmation head, and
  the hint is clipped all-or-nothing at a line cap, so on a very narrow terminal
  the mode keys / `esc cancel` tail can disappear. The degradation is honest;
  pin the geometry with a test so the box never corrupts, and decide whether a
  shorter caveat is warranted if the tail is lost.
- **README wording trap**: keep the "three endings" table intact; the change is
  about the toggle's *promise*, not about the post-merge notice.
- **Stale global skill**: if the orchestrator does not update it, the runtime
  contract agents load stays wrong — surface it rather than skipping silently.

## Ordering (coarse)

1. Validate the prototype against `behavior.feature` and add the missing tests
   (policy-known-off boundary, GitLab regression, narrow-width geometry).
2. Close the README and CHANGELOG gaps; check ADR 0013 / `docs/FEATURES.md`
   coherence.
3. Pass the gates (`make test`, diff coverage, mutation) and prepare the PR.
4. Orchestrator (out-of-env) updates the global skill in the same change.

## Progress
Phase: execution
| Scenario (behavior.feature) | Status | Commit |
| --- | --- | --- |
| GitHub repo auto-deletes and the request is no — the confirmation says so | ✅ | 4e3fb8f |
| GitHub repo auto-deletes but the request is yes — no caveat | ✅ | 4e3fb8f |
| GitHub repo policy explicitly off and the request is no — no caveat | ✅ | be8ad59 |
| GitHub repo policy unknown and the request is no — no caveat | ✅ | be8ad59 |
| GitLab is unchanged | ✅ | be8ad59 |
| The request still drives the client-side delete flag | ✅ | 4e3fb8f |
| The toggle still turns off without disarming or firing | ✅ | 4e3fb8f |
| A narrow terminal degrades honestly | ✅ | be8ad59 |
| The policy travels with the item data | ✅ | 88d9dbe |
