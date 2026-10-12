# Feature flow — fix/delete-branch-toggle

Behaviour of the merge confirmation's delete-branch toggle, derived from
`behavior.feature`. One true caveat case; everything else is unchanged.

```mermaid
flowchart TD
    A[Merge armed: 'delete branch: yes (tab)' default] --> B{Press tab}
    B -->|request = no| C[Label: 'delete branch: no (tab)']
    B -->|request = yes| A

    C --> D{Forge}
    D -->|GitLab| E[No caveat]
    D -->|GitHub| F{Repo auto-deletes head branches?}
    F -->|known ON| G[Append caveat:<br/>'— repo auto-deletes head branches']
    F -->|known OFF| H[No caveat]
    F -->|unknown / absent| I[No caveat<br/>not knowing is not 'off']

    G --> J[Pick strategy m/r/s]
    E --> J
    H --> J
    I --> J

    J --> K{Request = no ?}
    K -->|yes| L[Send NO delete flag<br/>gh: no --delete-branch · glab: no --remove-source-branch]
    K -->|no, request on| M[Send delete flag]
```

Width note: the confirmation is the bottom Keybinds bar, clipped all-or-nothing
at `maxHintLines = 3` — the caveat lengthens the head, so on a very narrow
terminal the `press the mode … esc cancel` tail can drop (honest degradation).
