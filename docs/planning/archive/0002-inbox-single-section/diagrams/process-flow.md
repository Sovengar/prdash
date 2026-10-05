# Process flow — planning of 0002-inbox-single-section

Planning flow of THIS feature (slug `0002-inbox-single-section`), with the
real decisions taken.

```mermaid
flowchart TD
  W["wt switch --create feat/inbox-single-section"] --> Idx["Codebase index: no codebase-index/prdash<br/>codegraph: not_initialized → it is not built"]
  Idx --> Iss["issue.md: single section + legend"]
  Iss --> CP1{"Checkpoint issue"}
  CP1 -->|approved + feedback| FB["Added requirement: the common path prefix (ADR 0002)<br/>MUST stay visible; design open to a future toggle"]
  FB --> Beh["behavior.feature: 16 scenarios<br/>default Assigned · tab cycle · legend · prefix · cursor/scroll"]
  Beh --> CP2{"Checkpoint behavior"}
  CP2 -->|approved| Plan["plan.md — adr_required: true"]
  Plan --> Dec1["Decision: prefix = fixed line at the start of the body<br/>(discarded: legend, PRDash header, full path per cell)"]
  Plan --> Dec2["Decision: newRefLayout receives only the active section"]
  Plan --> Dec3["Decision: model.Section.Legend() without touching String() (--print untouched)"]
  Plan --> Cut["Scope cut: toggleable/selectable prefix = LATER feature"]
  Plan --> Ctx["codebase-researcher → context.md (freshness 143fd59b)"]
  Ctx --> Dia["diagrams/feature-flow.md"]
  Dia --> CP3{"Checkpoint plan"}
  CP3 -->|approved| Done["commit docs/planning/0002-inbox-single-section/"]
  CP3 -->|feedback| Plan

  classDef adr fill:#8250df,stroke:#8250df,color:#fff;
  class Dec1 adr;
```

Recorded decisions: `adr_required: true` → proposed ADR
`docs/adr/0004-inbox-single-section.md` (supersedes the clause of ADR 0002 that
anchors the prefix in the section header).
