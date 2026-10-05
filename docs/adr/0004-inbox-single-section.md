# ADR 0004 — Single-section inbox with a counts legend

- **Status**: Accepted
- **Date**: 2026-09-26
- **Decider**: user (buble)
- **Scope**: the TUI inbox, active section, border legend, ITEM column
- **Naming pattern**: `docs/adr/NNNN-slug.md`
- **Supersedes**: **ADR 0002**, in two clauses:
  1. The one that anchors the common path prefix **in the header of each
     section** (§Decision point 1 and point 4 "the part lost at the front is
     the group, which the header already declares").
  2. The **"all the sections"** scope of its point 2: the ITEM width no longer
     comes from the longest suffix of all the sections but **only from that of
     the active section** (decision 5 of this ADR). The formula —content
     bounded to `[6, 34]`— and the tail truncation remain in force.
  The rest of ADR 0002 —the prefix computation and the single-item section
  rule— stays in force as is.

## Context

The inbox stacked the three sections (`Mine`/`Assigned`/`Mentioned`) and each
one declared its common path prefix in an internal header with a title and a
count. So far, ADR 0002.

The problem is that the real inbox is not a flat list: it is **a work list**.
With the three sections always visible, the eye has to separate three distinct
orders on a single screen, and the cursor navigates cross-section, jumping
between elements that are not comparable with each other. Besides, the common
prefix (ADR 0002) loses its reason to exist when the painted set mixes
sections: the ITEM width was dimensioned by the longest suffix of **all of
them**, spending space on sections that may not be of interest right now.

The goal is to show **a single section at a time** and, with it, the path
prefix of that section. Since only one is painted, all rows share the section,
the prefix is computed over the visible set and the ITEM width is its own.

## Decision

1. **One active section, with its own position.** The inbox paints a single
   section. On opening, the active one is **`Assigned`** (`review`). Each
   section remembers its cursor and its scroll; when switching, the position
   of the one being left is stored and the one of the target is restored,
   bounded to the new content.
2. **`section-next` (default `tab`) cycles the active section** in the order
   `Assigned → Mentioned → Mine → Assigned`. It always cycles, even if the
   target is empty: its state and its count are exactly what you want to see.
   The action and its key still come from `[keybindings]`; the hints bar does
   not change.
3. **The counts legend replaces the `Inbox` title** on the top border of the
   inbox, with the exact format `Mine (9) · Assigned (4) · Mentioned (0)`: the
   short label of each section (`Section.Legend()`) and its item count. The
   active one is highlighted and the others dimmed. On a narrow terminal the
   border ANSI-aware truncates it to the right, so the box never goes out of
   alignment.
4. **The common path prefix moves to a fixed line at the start of the list
   body**, dimmed and with **only the prefix**
   (`  · APPCITTI/vsocial/backend/`), with no title nor count. It replaces
   exactly what the section header used to do, but without duplicating the
   legend. If the active section has no common prefix (a single item, or
   nothing in common), the line is not painted and the ITEM cells carry the
   full path tail-truncated (ADR 0002's rule intact).
5. **The ITEM width is computed on the active section.** `newRefLayout`
   receives only the section being painted, so the prefix and the width are
   its own and no width is spent on suffixes of sections that are not seen.
   The signature does not change; the caller does.
6. **`(empty)`, `loading more…` and the notices belong to the active
   section.** A non-active section only contributes its count to the legend;
   when tabbing to it, its empty state, its pagination indicator and its
   notices appear.
7. **The `--print` mode, the cache format and the config API do not change.**
   `Section.String()` is kept for `--print`; the legend uses
   `Section.Legend()`.

## Rejected alternatives

- **Prefix in the border legend.** It breaks the exact format of the legend
  (which is about counts), mixes the counts of the three sections with the
  path of a single one, and makes the line longer than what the border can
  truncate elegantly.
- **Prefix in the `PRDash` header.** It is far from the rows, it describes the
  state of the forges and of the refresh, and the prefix changes with the
  active section: reading it next to the refresh state confuses two unrelated
  things.
- **Full path in every ITEM cell (dropping the prefix).** Loses exactly the
  legibility that motivated ADR 0002: every row repeats the group and the
  table width eats the title.
- **Reintroducing a section header with only the prefix.** It is the chosen
  option under another name: it keeps the "header" semantics and the count
  duplicated with respect to the legend. It is rejected on grounds of
  nomenclature and duplication.
- **Skipping the empty sections when cycling.** It would force not being able
  to see the empty state of a section nor its count. It always cycles.

## Consequences

**Positive**

- A single list and a single order: the cursor navigates within the section
  being looked at, with no jumps between sets that are not comparable.
- The common path prefix (ADR 0002) is kept and put to better use: by being
  computed on the only visible section, it frees ITEM width without competing
  with absent sections.
- The legend gives at a glance the distribution of the work —`Mine (9) ·
  Assigned (4) · Mentioned (0)`— where before it took reading three stacked
  headers.
- It leaves open the **seam for the selectable/toggleable prefix** (a later
  feature): the prefix is composed as a discrete unit from a single source, so
  a later feature can hide it, toggle it or make it interactive without
  touching the legend, the table width or the section change. This ADR
  **does not** implement config nor new keys.

**Negative / costs**

- **The notices of non-active sections stop being seen.** A section with a
  failure only shows its count in the legend; the notice appears when tabbing
  to it. Cost accepted in exchange for the single-section view.
- **The prefix can shrink when more pages arrive** and widen ITEM once: it is
  the same single reflow ADR 0002 already accepted, now bounded to the active
  section.
- **The top border carries ANSI** in the highlighted stretch. Each stretch of
  the legend is composed with a full style, with no "bare" characters,
  because the border re-wraps each segment with its colour and an inner reset
  does not restore the border's colour. Verified at several widths.

**Verification**

- `internal/tui/section_test.go`: default `Assigned`; the `tab` cycle
  (including the empty section); legend with exact format, highlighting and
  deduplicated counts; prefix of the active one, absence of common prefix and
  table width at several widths; cursor/scroll remembered per section; refresh
  that keeps the position; `(empty)`, `loading more…` and notices only from
  the active one; legend truncated on a narrow terminal.
- `internal/tui/refcol_test.go`: the prefix appears only once (its line) and
  the rows only paint the suffix.
- `internal/forge/model/model_test.go`: `Legend()` with `String()` intact.
- `cmd/prdash/print_test.go`: the `--print` mode still prints the three
  sections with their long names.
