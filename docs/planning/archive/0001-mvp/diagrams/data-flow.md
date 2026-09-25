# Flujo de datos — prdash MVP

Dos recorridos: (1) el pipeline del inbox y (2) la provisión del review.
La capa pura (parse/inbox/state/plan) no toca red, disco ni subprocess.

## 1. Pipeline del inbox (F1)

```mermaid
flowchart LR
    subgraph Fuentes["Forge clients (I/O)"]
        GH["gh CLI<br/>github.com"]
        GL["glab CLI<br/>self-managed · /git/api/v4/"]
        BB["bitbucket adapter<br/>solo interfaz · sin red"]
    end

    subgraph Nucleo["Núcleo puro"]
        AD["forge.Adapter<br/>github · gitlab"]
        PA["forge/parse<br/>JSON/GraphQL → model"]
        IN["inbox<br/>merge · dedupe · ¿me toca?"]
        ST["state<br/>precedencia · score"]
    end

    subgraph Salida
        CA[("cache<br/>snapshot + rutas")]
        TU["tui<br/>tabla · detalle · cadencia"]
        PR["--print<br/>tabla one-shot"]
    end

    GH --> AD
    GL --> AD
    BB -.->|Unsupported| AD
    AD -->|items + warnings| PA
    PA --> IN
    IN --> ST
    ST --> TU
    ST --> PR
    IN <--> CA
    AD -->|401 gitlab.com · rate limit| TU
```

## 2. Provisión del worktree de review (F2)

```mermaid
flowchart LR
    IT["Item seleccionado"] --> RR["reporesolver<br/>roots + índice + memoria de rutas"]
    RR -->|no local| BARE["git clone --bare<br/>XDG data/repos/forge/host/owner/repo"]
    RR -->|local| WT
    BARE --> WT["worktree.Provisioner"]
    WT -->|"HERDR_ENV=1"| HN["herdr worktree create<br/>--branch &lt;local&gt; --path &lt;dest&gt;"]
    WT -->|fuera de Herdr| GW["git worktree add"]
    HN --> LAY["herdr.Port<br/>layout 3 panes"]
    LAY --> P1["pane TUICR"]
    LAY --> P2["pane Hunk"]
    LAY --> P3["pane opencode"]

    RR -.->|fetch antes de provisionar| REF["refs/pull/N/head<br/>refs/merge-requests/N/head<br/>→ rama local"]
    REF --> WT
```

**Fronteras**
- `reporesolver` es el único dueño del namespace de rutas (clon bare + worktrees).
- Solo el puerto `herdr` conoce la CLI nativa y aloja el fallback a git.
- El fetch del ref ocurre SIEMPRE antes de provisionar (ver ADR `docs/adr/0001-worktree-provisioning.md`).
