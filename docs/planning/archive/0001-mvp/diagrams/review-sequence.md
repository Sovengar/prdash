# Secuencia — montaje del layout de review y loop de comentarios

Recorrido de F2 desde la selección del ítem hasta el loop comento→agente.
Derivado de `behavior.feature` y del ADR `docs/adr/0001-worktree-provisioning.md`.

```mermaid
sequenceDiagram
    actor U as Usuario
    participant T as TUI prdash
    participant RR as reporesolver
    participant G as git
    participant H as herdr
    participant TU as TUICR
    participant HK as Hunk
    participant AG as agente opencode

    U->>T: montar review del ítem
    T->>RR: Resolve(repo)

    alt repo no clonado
        RR->>G: git clone --bare → XDG data/repos/...
    end

    RR->>G: fetch del ref de review
    Note right of G: refs/pull/N/head (GH)<br/>refs/merge-requests/N/head (GL)
    RR->>G: crear rama local de trabajo
    RR-->>T: ruta local + rama

    alt HERDR_ENV=1
        T->>H: herdr worktree create --branch <local> --path <dest>
        H-->>T: worktree ligado a workspace
        T->>H: abrir layout (3 panes)
        H->>TU: pane tuicr pr N
        H->>HK: pane hunk session review
        H->>AG: pane opencode agent
    else fuera de Herdr
        T->>G: git worktree add
        T-->>U: aviso: layout requiere Herdr
    end

    U->>TU: escribe comentario
    U->>HK: escribe comentario
    AG->>TU: lee comentarios (JSON)
    AG->>HK: lee comentarios (JSON)
    AG->>G: aplica cambios en el worktree
    AG-->>U: resultado visible en el pane

    Note over T,H: al cerrar, el worktree se conserva (limpieza explícita)
    Note over RR,T: sin permisos de clon/fetch → error claro, sin restos
```
