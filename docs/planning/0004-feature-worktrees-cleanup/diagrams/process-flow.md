# Flujo de planning (feature-aware) — 0004 worktrees-cleanup

Recorrido real de esta feature por el pipeline `swe`, con las decisiones
tomadas. Slug: `worktrees-cleanup`. Tipo: `feature`. Rama: `feat/worktrees-cleanup`.

```mermaid
flowchart TD
  W["orchestrator: worktree + rama feat/worktrees-cleanup<br/>planning_dir 0004-feature-worktrees-cleanup"] --> IDX{"¿índice fresco?"}
  IDX -- "no: codegraph sin init" --> IDX1["codebase-explorer:<br/>codegraph init + sync<br/>commit 9252700"]
  IDX -- "sí" --> REF["idea-refiner → issue.md"]
  IDX1 --> REF

  REF --> C1{"⏸️ CHECKPOINT issue"}
  C1 -- "APROBADA + decisión B = borrar solo si limpio" --> BA["brainstormer + architect (paralelo)"]
  BA --> BE["behavior.feature<br/>A: --orphans excluyente<br/>B: limpio borra / sucio conserva<br/>negativos: sin borrado al cerrar"]

  BE --> C2{"⏸️ CHECKPOINT behavior"}
  C2 -- "APROBADO" --> PL["plan.md<br/>adr_required: TRUE → ADR 0007"]

  PL --> PL1["decisiones fijadas:<br/>--orphans excluyente + --dry-run<br/>RemoveIfClean (fail-safe)<br/>puerto ReviewRemover + reviewCleanupMsg"]
  PL1 --> CTX["codebase-researcher → context.md<br/>codegraph: ready · freshness 9252700"]
  CTX --> D["diagramas<br/>feature-flow · process-flow"]

  D --> C3{"⏸️ CHECKPOINT plan (estás aquí)"}
  C3 -- "APROBADO" --> EX["swe-executor<br/>TDD: worktree → cmd → executor → tui → docs"]
  EX --> V{"make test"}
  V -- "verde" --> DONE["review + PR a main"]

  C1 -. "iterar" .-> REF
  C2 -. "iterar" .-> BE
  C3 -. "iterar" .-> PL

  subgraph CORRECCIONES["Correcciones al enunciado (verificadas)"]
    K1["remove YA acepta varias rutas<br/>lo único que falta es --orphans (y el README miente)"]
    K2["prdash NO tiene acción 'close'<br/>B = 'mergeado desde prdash', no 'cerrado'"]
  end
  REF -. "se documentan" .-> CORRECCIONES

  subgraph FUERA["Fuera de alcance (explícito)"]
    F1["borrado al cerrar la app"]
    F2["script del orquestador swe-workspace-manager.sh"]
    F3["artefacto no versionado .codegraph/"]
  end
  PL -. "no entra" .-> FUERA
```
