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

## Simulation (`v`)

- `merge` is always offered; `rebase` appears only when `git-sim --version` reports 0.4.0 or newer.
- Does not need the review mounted: without one it fetches the PR/MR head into a throwaway clone. Needs `git-sim` on the PATH and the repository cloned locally; otherwise it warns.
- A git-sim `IndexError` is reported as an upgrade hint, and its stdout `git-sim error:` line is preserved.
- The render is an HTML page opened with `terminal-browser open <path> --split right` (fallback `xdg-open`); `o` reopens it.

## Review Order

Review intent, changed files, tests, CI state, and unresolved comments in that order. Create a review worktree only when inspection or testing would alter the current checkout.

