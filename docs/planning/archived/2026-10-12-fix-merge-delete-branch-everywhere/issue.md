# Issue: fix/merge-delete-branch-everywhere

Status: implemented

Branch: `fix/merge-delete-branch-everywhere` (change 30, promoted prototype).

## Summary

After a successful merge, delete the branch in the review clone (`prdash/pr-N` +
tracking ref) and the local clone (PR source branch, never a checkout and never
the default branch), review-worktree first; plus pre-emptive mergeability (lazy
resolve, hard-block a known conflict, `✗` gutter marker, `Mergeable` detail
field).

## ADRs

- `docs/adr/0013-delete-the-branch-wherever-it-lives.md`
- `docs/adr/0014-pre-emptive-mergeability.md`
