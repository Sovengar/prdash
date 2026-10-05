# F3 — Auto-review with gate and allowlist (milestone, NOT implemented)

- **Status**: post-MVP milestone. Design approved in `behavior.feature`; **not
  implemented** in feature 0001.
- **Parent**: `docs/planning/archive/0001-mvp/plan.md` (phase F3) and
  `behavior.feature` (`@F3` block, which is the expected behavior and serves as
  a future criterion).
- **Decision**: the current release ends when opening the review panes with
  their right cwd, env and argv. Approving or commenting is the responsibility
  of the user and of the loop's tools/agent, not of prdash.

## Goal

That prdash can, optionally, let an agent complete the review loop and
**approve** a PR/MR without intervention, but only when all the safety
conditions hold. Automatic approval is a high-risk capability: the design
treats it as opt-in, auditable and conservative in the face of any doubt.

## Expected flow (high level)

1. The user enables the auto-review mode in the config and declares which repos
   are eligible.
2. The inbox item is mounted as a normal review (F2): worktree + panes.
3. The agent (opencode) analyzes the diff in the worktree and produces a result
   with a verdict and its findings.
4. The **gate** decides whether the result is conclusive and free of critical
   findings.
5. Only if the gate passes **and** the repo is on the **allowlist**, prdash runs
   the approval via the corresponding forge (`gh`/`glab`).
6. prdash notifies the result via Herdr; without Herdr, it logs the event
   locally and continues.

## Hard rules (non-negotiable)

- **Allowlist per repo.** A repo not on the allowlist **never** self-approves,
  even if the analysis is clean. The reason is recorded.
- **Explicit gate.** Self-approval requires `autoreview.enabled` and the gate
  enabled, plus 0 critical findings.
- **Dry-run by default.** With the mode enabled but without the gate, prdash
  logs what it *would* have approved and does not run the action. No code path
  approves by default.
- **Never approve with a failed or partial analysis.** An analysis that fails,
  is interrupted or stays incomplete is reported as not conclusive and is never
  turned into an approval.
- **Clean degradation.** Without Herdr available, an event that should have
  been notified does not break the process: it is logged locally.
- **Reversibility and traces.** Every decision (approved, skipped by
  allowlist, skipped by inconclusive analysis) must end up in a queryable log.
  Approval does not erase evidence: the analysis result is kept.

## Reserved configuration

These keys are already parsed in `internal/config` (no consumer yet); F3 would
use them as they are:

| Key | Type | Default | Use |
|---|---|---|---|
| `autoreview.enabled` | bool | `false` | Enables the mode. By itself it does not approve: the gate is needed. |
| `autoreview.allowlist` | `[]string` | `[]` | Repos (`host/owner/repo`) eligible for self-approval. |

## Future acceptance criteria

The scenarios of the `@F3` block of `behavior.feature` are the source of truth
for when it is implemented. They are summarized here for traceability:

1. A repo not on the allowlist does not self-approve and the reason is
   recorded.
2. With the gate satisfied and the repo allowlisted, an analysis with 0 critical
   findings approves via the forge and notifies via Herdr.
3. A failed or partial analysis never approves and reports that it was not
   conclusive.
4. Without Herdr, the notification degrades without breaking the process and is
   logged.

## Out of scope of this documented milestone

- Code implementation, tests or forge adapters for the approval.
- Richer gate policies (weights per severity, exceptions per author, time
  windows). They will be decided when implementing, on top of the minimal gate.
- Running the agent or managing its credentials: prdash does not own the
  comment loop (`plan.md`, key decision 3).
