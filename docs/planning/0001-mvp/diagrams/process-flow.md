# Flujo de planning — 0001-mvp (prdash)

Proceso real seguido para ESTE cambio: `0001-mvp` (feature, greenfield), con sus
checkpoints, decisiones y cortes de alcance.

## Recorrido

```mermaid
flowchart TD
    S["slug: 0001-mvp · tipo: feature"] --> W["Repo greenfield → planning en main (docs-only)"]
    W --> IDX["Índice: sin código ni codegraph<br/>reuso de codebase-index/gitdash (#2202)"]
    IDX --> ISS["issue.md"]
    ISS --> Q1{{"Checkpoint 1 · issue"}}
    Q1 -->|aprobada| BA["brainstormer + architect (en paralelo)"]
    BA --> BF["behavior.feature (~30 escenarios)"]
    BF --> Q2{{"Checkpoint 2 · behavior"}}
    Q2 -->|aprobado| PL["plan.md · adr_required: true"]
    PL --> Q3{{"Checkpoint 3 · plan"}}
    Q3 -->|"aprobado + ADR pedido"| ADR["docs/adr/0001-worktree-provisioning.md<br/>permanente, sobrevive al archivado"]
    ADR --> CTX["context.md (codebase-researcher)"]
    CTX --> DG["diagramas: feature-flow · data-flow · review-sequence"]
    DG --> CM["commits docs-only en main"]
```

## Decisiones, cortes y riesgos de este cambio

```mermaid
flowchart LR
    subgraph Decisiones["Decisiones cerradas"]
        D1["Sin tope por sección<br/>→ paginación ilimitada + carga progresiva"]
        D2["Clon bare en XDG data<br/>si el repo no está local"]
        D3["1 layout abierto a la vez<br/>worktrees múltiples coexistentes"]
        D4["Worktree se conserva al cerrar<br/>+ limpieza explícita"]
        D5["Plugin = mismo binario<br/>+ subcomandos + manifiesto"]
    end

    subgraph Cortes["Scope cuts"]
        C1["Bitbucket solo interfaz"]
        C2["gitlab.com no funcional (401)"]
        C3["F3 auto-approve = milestone"]
    end

    subgraph Riesgos["Riesgos marcados"]
        R1["Rate limit al paginar sin tope"]
        R2["Auth self-managed vía /git/api/v4/"]
        R3["Drift de la CLI Herdr 0.9.x"]
    end
```

**Verificaciones que quedan para implementación**: `tuicr pr` submit contra self-managed;
`herdr worktree create` desde clon bare con `--path`; permisos approve/merge en el self-managed;
placement real del pane y link handler; fetch de refs de review en el host self-managed.
