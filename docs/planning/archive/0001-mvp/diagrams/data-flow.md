# Data flow — prdash MVP

Two paths: (1) the inbox pipeline and (2) the review provisioning.
The pure layer (parse/inbox/state/plan) touches neither network, disk nor
subprocess.

## 1. Inbox pipeline (F1)

```mermaid
flowchart LR
    subgraph SOURCES["Forge clients (I/O)"]
        GH["gh CLI<br/>github.com"]
        GL["glab CLI<br/>self-managed · /git/api/v4/"]
        BB["bitbucket adapter<br/>interface only · no network"]
    end

    subgraph CORE["Pure core"]
        AD["forge.Adapter<br/>github · gitlab"]
        PA["forge/parse<br/>JSON/GraphQL → model"]
        IN["inbox<br/>merge · dedupe · does it fall to me?"]
        ST["state<br/>precedence · score"]
    end

    subgraph OUTPUT
        CA[("cache<br/>snapshot + paths")]
        TU["tui<br/>table · detail · cadence"]
        PR["--print<br/>one-shot table"]
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

## 2. Review worktree provisioning (F2)

```mermaid
flowchart LR
    IT["Selected item"] --> RR["reporesolver<br/>roots + index + path memory"]
    RR -->|not local| BARE["git clone --bare<br/>XDG data/repos/forge/host/owner/repo"]
    RR -->|local| WT
    BARE --> WT["worktree.Provisioner"]
    WT -->|"HERDR_ENV=1"| HN["herdr worktree create<br/>--branch &lt;local&gt; --path &lt;dest&gt;"]
    WT -->|outside Herdr| GW["git worktree add"]
    HN --> LAY["herdr.Port<br/>layout 3 panes"]
    LAY --> P1["pane TUICR"]
    LAY --> P2["pane Hunk"]
    LAY --> P3["pane opencode"]

    RR -.->|fetch before provisioning| REF["refs/pull/N/head<br/>refs/merge-requests/N/head<br/>→ local branch"]
    REF --> WT
```

**Boundaries**
- `reporesolver` is the sole owner of the path namespace (bare clone +
  worktrees).
- Only the `herdr` port knows the native CLI and hosts the git fallback.
- The ref fetch happens ALWAYS before provisioning (see ADR
  `docs/adr/0001-worktree-provisioning.md`).
