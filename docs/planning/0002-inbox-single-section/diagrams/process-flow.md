# Process flow — planificación de 0002-inbox-single-section

Flujo de planificación de ESTA feature (slug `0002-inbox-single-section`),
con las decisiones reales tomadas.

```mermaid
flowchart TD
  W["wt switch --create feat/inbox-single-section"] --> Idx["Índice de codebase: sin codebase-index/prdash<br/>codegraph: not_initialized → no se construye"]
  Idx --> Iss["issue.md: sección única + leyenda"]
  Iss --> CP1{"Checkpoint issue"}
  CP1 -->|aprobado + feedback| FB["Requisito añadido: el prefijo de ruta común (ADR 0002)<br/>DEBE seguir visible; diseño abierto a toggle futuro"]
  FB --> Beh["behavior.feature: 16 scenarios<br/>default Assigned · ciclo tab · leyenda · prefijo · cursor/scroll"]
  Beh --> CP2{"Checkpoint behavior"}
  CP2 -->|aprobado| Plan["plan.md — adr_required: true"]
  Plan --> Dec1["Decisión: prefijo = línea fija al inicio del cuerpo<br/>(descartadas: leyenda, cabecera PRDash, ruta completa por celda)"]
  Plan --> Dec2["Decisión: newRefLayout recibe solo la sección activa"]
  Plan --> Dec3["Decisión: model.Section.Legend() sin tocar String() (--print intacto)"]
  Plan --> Cut["Scope cut: prefijo toggleable/seleccionable = feature POSTERIOR"]
  Plan --> Ctx["codebase-researcher → context.md (freshness 143fd59b)"]
  Ctx --> Dia["diagrams/feature-flow.md"]
  Dia --> CP3{"Checkpoint plan"}
  CP3 -->|aprobado| Done["commit docs/planning/0002-inbox-single-section/"]
  CP3 -->|feedback| Plan

  classDef adr fill:#8250df,stroke:#8250df,color:#fff;
  class Dec1 adr;
```

Decisiones registradas: `adr_required: true` → ADR propuesto
`docs/adr/0004-inbox-single-section.md` (supersede la cláusula del ADR 0002 que
ancla el prefijo en la cabecera de sección).
