# Behavior flow — 0004 worktree cleanup

Derived from `../behavior.feature`. It is not Cucumber: it is a view of the
expected behavior for human review.

## A — `prdash worktrees remove --orphans`

```mermaid
flowchart TD
  A0["prdash worktrees remove ..."] --> A1{"valid usage?<br/>--orphans with paths<br/>--dry-run without --orphans<br/>unknown flag"}
  A1 -- "no" --> A2["stderr + exit 2<br/>deletes nothing"]
  A1 -- "yes" --> A3["Audit() → filter Entry.Orphan<br/>(same source as 'worktrees list')"]
  A3 --> A4{"how many orphans?"}
  A4 -- "0" --> A5["stdout: 'no orphaned worktrees'<br/>exit 0 (happy path)"]
  A4 -- "N" --> A6{"--dry-run?"}
  A6 -- "yes" --> A7["prints the exact batch<br/>deletes nothing · exit 0"]
  A6 -- "no" --> A8["Remove() per orphan<br/>guards: Owned + under managed root"]
  A8 --> A9["exit 0<br/>(1 if any deletion fails)"]

  A8 -. "never touches" .-> AX1["healthy own worktree"]
  A8 -. "never touches" .-> AX2["foreign worktree"]
```

## B — self-deletion on merge **from prdash**

```mermaid
flowchart TD
  B0["merge action launched by prdash"] --> B1{"out.Kind == ActionMerge<br/>&& out.OK"}
  B1 -- "no: approve, retarget,<br/>failure, conflict, permission,<br/>no-mergeable" --> B2["touches no worktree"]
  B1 -- "yes" --> B3["ActiveReview(item)<br/>maps item → worktree"]
  B3 --> B4{"is there a mounted review<br/>and does the path exist?"}
  B4 -- "no" --> B5["no-op without error<br/>the merge still ok"]
  B4 -- "yes" --> B6{"git status --porcelain<br/>clean tree?"}
  B6 -- "dirty (includes untracked)" --> BKEEP["KEEPS<br/>'merged, but the worktree<br/>has uncommitted changes — kept'"]
  B6 -- "unreadable state" --> BKEEP2["KEEPS (fail-safe)<br/>'could not read the state'"]
  B6 -- "clean" --> B7["RemoveIfClean → deletes<br/>ForgetReview (forgets the record)"]
  B7 --> B8["notice: 'merge ok · worktree removed'"]
  BKEEP --> BNOTICE["composed notice:<br/>merge + branch (DeleteMsg) + worktree<br/>without losing any fact"]
  BKEEP2 --> BNOTICE
  B8 --> BNOTICE

  BQ["closing the app (q / ctrl+c)"] --> BQ1["deletes no worktree"]
  BF["PR merged OUTSIDE prdash"] --> BF1["the refresh sees it<br/>but does NOT trigger cleanup"]
```

## Common guards for the two paths

```mermaid
flowchart LR
  G0["any deletion attempt<br/>(explicit path · --orphans · B)"] --> G1{"Owned?<br/>label or name 'prdash-…'"}
  G1 -- "no" --> G2["rejected"]
  G1 -- "yes" --> G3{"under the managed root?"}
  G3 -- "no" --> G2
  G3 -- "yes" --> G4["can be deleted"]
```
