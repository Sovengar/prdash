# Sequence — mounting the review layout and the comment loop

F2 path from the item selection to the comment→agent loop.
Derived from `behavior.feature` and the ADR
`docs/adr/0001-worktree-provisioning.md`.

```mermaid
sequenceDiagram
    actor U as User
    participant T as TUI prdash
    participant RR as reporesolver
    participant G as git
    participant H as herdr
    participant TU as TUICR
    participant HK as Hunk
    participant AG as opencode agent

    U->>T: mount the review of the item
    T->>RR: Resolve(repo)

    alt repo not cloned
        RR->>G: git clone --bare → XDG data/repos/...
    end

    RR->>G: fetch of the review ref
    Note right of G: refs/pull/N/head (GH)<br/>refs/merge-requests/N/head (GL)
    RR->>G: create the local working branch
    RR-->>T: local path + branch

    alt HERDR_ENV=1
        T->>H: herdr worktree create --branch <local> --path <dest>
        H-->>T: worktree linked to workspace
        T->>H: open layout (3 panes)
        H->>TU: pane tuicr pr N
        H->>HK: pane hunk session review
        H->>AG: pane opencode agent
    else outside Herdr
        T->>G: git worktree add
        T-->>U: warning: layout requires Herdr
    end

    U->>TU: writes a comment
    U->>HK: writes a comment
    AG->>TU: reads comments (JSON)
    AG->>HK: reads comments (JSON)
    AG->>G: applies changes in the worktree
    AG-->>U: result visible in the pane

    Note over T,H: on close, the worktree is kept (explicit cleanup)
    Note over RR,T: no clone/fetch permissions → clear error, no leftovers
```
