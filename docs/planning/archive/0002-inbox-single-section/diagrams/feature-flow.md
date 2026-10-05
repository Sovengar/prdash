# Feature flow — 0002 single-section Inbox

Inbox behavior flow, derived from `behavior.feature`. Feature-aware: a single
active section, count legend and common path prefix of the active one.

```mermaid
flowchart TD
  Start([Open prdash]) --> Def["Active section = Assigned (review)<br/>on opening"]

  subgraph CYCLE["tab cycle (action section-next)"]
    A["Assigned"] -->|tab| M["Mentioned"]
    M -->|tab| MINE["Mine"]
    MINE -->|tab| A
  end
  Def --> A

  Def --> Legend["Legend on the TOP-LEFT border<br/>Mine (n) · Assigned (n) · Mentioned (n)<br/>active highlighted · rest dimmed"]
  Legend -. replaces .-> T["'Inbox' title"]

  Def --> Body{"Compose the body<br/>of the active section"}
  Body --> Pref{"Does the active one share<br/>a path prefix?"}
  Pref -->|yes| PL["Fixed dimmed line:<br/>· APPCITTI/vsocial/backend/"]
  Pref -->|no| FC["ITEM cells with full path<br/>(truncated from the tail)"]
  PL --> Content
  FC --> Content

  Content{"State of the active one"}
  Content -->|with items| Rows["column header + rows<br/>ITEM cells with only the suffix"]
  Content -->|no items and no warnings| Empty["(empty)"]
  Content -->|with warnings| Warn["⚠ forge: could not be queried (…)"]
  Content -->|paginating| More["loading more…<br/>(only if the active one paginates)"]

  Cycle["Change section"]
  A -. saves cursor/scroll .-> Mem[("Position per section")]
  Mem -. restores on return .-> A

  classDef active fill:#1f6feb,stroke:#1f6feb,color:#fff;
  class A active;
```

Notes:
- Only the active section is painted; the warnings and the `loading more…`
  belong to the active one (the ones of other sections are not painted).
- The common path prefix is kept (ADR 0002) and it is the seam for the future
  selectable/toggleable prefix (a later feature).
