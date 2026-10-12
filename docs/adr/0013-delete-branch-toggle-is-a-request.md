# ADR 0013 — The delete-branch toggle is a request, not a promise

- Status: accepted
- Date: 2026-10-12

## Context

The merge confirmation offers `tab` to toggle `delete branch: yes|no`
(default `yes`). On GitHub the adapter appends `--delete-branch` only when the
toggle is `yes`, so the flag itself is honest. The lie is at the forge level:
when a repository has **"Automatically delete head branches"** enabled
(GraphQL `deleteBranchOnMerge`), GitHub deletes the head branch on merge
**regardless** of what the client sends — confirmed upstream in
[cli/cli#11508](https://github.com/cli/cli/issues/11508), where the maintainers
state that `--delete-branch` and the repository setting are independent and that
no client flag bypasses the setting. There, `delete branch: no` promises
something GitHub will not keep.

GitLab is different: `remove_source_branch_after_merge` on the project only sets
the UI default, while the per-MR `should_remove_source_branch` (sent by
`glab mr merge --remove-source-branch`) controls the actual behaviour, so the
toggle is honoured there.

## Decision

The toggle keeps its compact label and its meaning as a **request**, and the
confirmation stops lying where the request cannot be kept: when the item reports
`AutoDeleteKnown && AutoDeleteBranch` and the toggle is off, the label reads
`delete branch: no (tab) — repo auto-deletes head branches`. When the policy is
off or unknown, or when the request is `yes`, nothing changes.

The policy arrives with the data already fetched: the GitHub GraphQL PR query
adds `deleteBranchOnMerge` to the existing `repository { … }` block, so no extra
round-trip is spent. A `nil` field stays `AutoDeleteKnown == false` — not
knowing is not the same as the policy being off, so no caveat is invented.

## Consequences

- On a GitHub repo that auto-deletes, turning the toggle off no longer reads as
  "the branch survives"; the user sees why.
- The toggle is unchanged when it is honoured (GitHub with the setting off,
  GitLab), so the common path keeps its compact two-line confirmation.
- GitLab reports the field as unknown and gets no caveat: the per-MR flag there
  is genuine.
- `AutoDeleteBranch`/`AutoDeleteKnown` live on `model.Item`, beside `Merge`, so
  both list and detail paths carry them without a new adapter call.

## Rejected alternatives

- **Document the limitation only.** Cheapest, but leaves the TUI promising a
  "no" it cannot keep — the bug the task reports.
- **Disable the repo setting before merging and restore it after (GraphQL
  mutation).** Needs repository admin, races against other merges, and turns a
  one-key merge into a multi-call, revertible mutation. Out of scope.
- **Hide the toggle when the policy is on.** It still controls the client-side
  delete and the explicit API call, so removing it would drop a real capability
  to fix the wording.
