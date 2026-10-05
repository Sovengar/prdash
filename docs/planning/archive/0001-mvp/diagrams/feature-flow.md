# Behavior flow — prdash MVP

Derived from `behavior.feature`. User path: inbox (F1), review orchestrator
(F2), degradation and the F3 gate.

```mermaid
flowchart TD
    A["prdash starts"] --> B{"HERDR_ENV=1?"}
    B -->|No| C["F1 inbox operational<br/>F2 shows a warning: requires Herdr"]
    B -->|Yes| D["F1 inbox + F2 available"]

    C --> E["Inbox: Created by me · Review/assigned · Mentions"]
    D --> E
    E --> E1["Manual refresh r / automatic 60s<br/>last update indicator per forge"]
    E1 --> E2{"Does any forge fail?"}
    E2 -->|Yes| E3["Explicit error state per forge<br/>the rest stays visible"]
    E2 -->|No| E4["Empty sections vs with data"]

    E3 --> G
    E4 --> G
    G{"Item selection"} -->|detail| H["Detail: title, author, branches, number, URL, state"]
    G -->|approve / merge| I["Action via gh/glab or delegated to tuicr"]
    G -->|mount review| J{"Local repo resolved?"}

    I --> I1{"Did the item change on the forge?"}
    I1 -->|Yes| I2["Clear error + item refresh"]
    I1 -->|No| I3["Updated state"]

    J -->|No| K["Bare clone in XDG data<br/>repos/forge/host/owner/repo"]
    J -->|Yes| L["Review ref fetch<br/>refs/pull/N/head · refs/merge-requests/N/head"]
    K --> L
    L --> M["Create local working branch + provision worktree"]
    M --> N["Herdr layout: TUICR + Hunk + opencode agent"]
    N --> O["Loop: I comment in TUICR/Hunk → the agent reads and applies them"]
    O --> P["On close: worktree is kept<br/>cleanup by explicit command"]

    P --> Q{"F3 enabled and repo on the allowlist?"}
    Q -->|No| R["No self-approval · the reason is recorded"]
    Q -->|Yes| S{"0 critical findings and conclusive analysis?"}
    S -->|No| R
    S -->|Yes| T["Approves via forge + notifies via Herdr"]

    M -.->|no clone/fetch permissions| X["Clear error, leaving no half-done clone nor worktree"]
    N -.->|missing tool| Y["Pane skipped with a warning, the layout stays alive"]
```

**Behavior notes**
- "Empty" and "error" are never confused (explicit failure per
  forge/section).
- No per-section cap: the inbox paginates until exhausted, with progressive
  loading.
- Outside Herdr, F1 does not degrade; only F2 is reported.
