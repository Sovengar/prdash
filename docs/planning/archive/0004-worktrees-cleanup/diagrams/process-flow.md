# Planning flow (feature-aware) — 0004 worktrees-cleanup

Real path of this feature through the `swe` pipeline, with the decisions
taken. Slug: `worktrees-cleanup`. Type: `feature`. Branch: `feat/worktrees-cleanup`.

```mermaid
flowchart TD
  W["orchestrator: worktree + branch feat/worktrees-cleanup<br/>planning_dir 0004-feature-worktrees-cleanup"] --> IDX{"is the index fresh?"}
  IDX -- "no: codegraph not initialized" --> IDX1["codebase-explorer:<br/>codegraph init + sync<br/>commit 9252700"]
  IDX -- "yes" --> REF["idea-refiner → issue.md"]
  IDX1 --> REF

  REF --> C1{"⏸️ CHECKPOINT issue"}
  C1 -- "APPROVED + decision B = delete only if clean" --> BA["brainstormer + architect (parallel)"]
  BA --> BE["behavior.feature<br/>A: --orphans exclusive<br/>B: clean deletes / dirty keeps<br/>negatives: no deletion on close"]

  BE --> C2{"⏸️ CHECKPOINT behavior"}
  C2 -- "APPROVED" --> PL["plan.md<br/>adr_required: TRUE → ADR 0007"]

  PL --> PL1["decisions settled:<br/>--orphans exclusive + --dry-run<br/>RemoveIfClean (fail-safe)<br/>ReviewRemover port + reviewCleanupMsg"]
  PL1 --> CTX["codebase-researcher → context.md<br/>codegraph: ready · freshness 9252700"]
  CTX --> D["diagrams<br/>feature-flow · process-flow"]

  D --> C3{"⏸️ CHECKPOINT plan (you are here)"}
  C3 -- "APPROVED" --> EX["swe-executor<br/>TDD: worktree → cmd → executor → tui → docs"]
  EX --> V{"make test"}
  V -- "green" --> DONE["review + PR to main"]

  C1 -. "iterate" .-> REF
  C2 -. "iterate" .-> BE
  C3 -. "iterate" .-> PL

  subgraph CORRECTIONS["Corrections to the statement (verified)"]
    K1["remove ALREADY accepts several paths<br/>the only thing missing is --orphans (and the README lies)"]
    K2["prdash has NO 'close' action<br/>B = 'merged from prdash', not 'closed'"]
  end
  REF -. "they are documented" .-> CORRECTIONS

  subgraph OUT_OF_SCOPE["Out of scope (explicit)"]
    F1["deletion on closing the app"]
    F2["orchestrator's script swe-workspace-manager.sh"]
    F3["untracked artifact .codegraph/"]
  end
  PL -. "does not enter" .-> OUT_OF_SCOPE
```
