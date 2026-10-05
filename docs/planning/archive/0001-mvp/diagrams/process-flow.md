# Planning flow — 0001-mvp (prdash)

Real process followed for THIS change: `0001-mvp` (feature, greenfield), with
its checkpoints, decisions and scope cuts.

## Path

```mermaid
flowchart TD
    S["slug: 0001-mvp · type: feature"] --> W["Greenfield repo → planning on main (docs-only)"]
    W --> IDX["Index: no code nor codegraph<br/>reuse of codebase-index/gitdash (#2202)"]
    IDX --> ISS["issue.md"]
    ISS --> Q1{{"Checkpoint 1 · issue"}}
    Q1 -->|approved| BA["brainstormer + architect (in parallel)"]
    BA --> BF["behavior.feature (~30 scenarios)"]
    BF --> Q2{{"Checkpoint 2 · behavior"}}
    Q2 -->|approved| PL["plan.md · adr_required: true"]
    PL --> Q3{{"Checkpoint 3 · plan"}}
    Q3 -->|"approved + ADR requested"| ADR["docs/adr/0001-worktree-provisioning.md<br/>permanent, survives the archiving"]
    ADR --> CTX["context.md (codebase-researcher)"]
    CTX --> DG["diagrams: feature-flow · data-flow · review-sequence"]
    DG --> CM["docs-only commits on main"]
```

## Decisions, cuts and risks of this change

```mermaid
flowchart LR
    subgraph DECISIONS["Closed decisions"]
        D1["No cap per section<br/>→ unlimited pagination + progressive loading"]
        D2["Bare clone in XDG data<br/>if the repo is not local"]
        D3["1 layout open at a time<br/>multiple coexisting worktrees"]
        D4["Worktree kept on close<br/>+ explicit cleanup"]
        D5["Plugin = same binary<br/>+ subcommands + manifest"]
    end

    subgraph CUTS["Scope cuts"]
        C1["Bitbucket interface only"]
        C2["gitlab.com not working (401)"]
        C3["F3 auto-approve = milestone"]
    end

    subgraph RISKS["Flagged risks"]
        R1["Rate limit when paginating without a cap"]
        R2["Self-managed auth via /git/api/v4/"]
        R3["Herdr 0.9.x CLI drift"]
    end
```

**Verifications left for implementation**: `tuicr pr` submit against
self-managed; `herdr worktree create` from a bare clone with `--path`;
approve/merge permissions on the self-managed; real placement of the pane and
link handler; fetch of review refs on the self-managed host.
