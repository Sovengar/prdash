---
name: prdash
description: "Trigger: prdash, PR inbox, MR inbox, review request, merge, review worktree, PR simulation, or prdash config. Operates and configures prdash."
---

## Activation Contract

Load for prdash pull-request or merge-request inboxes, reviews, worktrees, simulations, or configuration.

## Hard Rules

- Inspect the PR and repository state before review or mutation.
- Do not merge, approve, close, or submit a review without explicit authorization.
- Keep simulated results separate from live forge actions.

## Execution Steps

1. List and select the target review item.
2. Inspect metadata, diff, checks, and comments.
3. Run the requested review, worktree, or configuration action.
4. Report state, findings, and remaining blockers.

## Review Order

Review intent, changed files, tests, CI state, and unresolved comments in that order. Create a review worktree only when inspection or testing would alter the current checkout.

