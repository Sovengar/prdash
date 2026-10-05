# 0002 — Single-section inbox with a count legend — Plan

adr_required: true
adr_reason: the feature removes the internal section header, which is exactly where ADR 0002 (Accepted) fixes that the common path prefix lives; relocating it is a design decision with real discarded alternatives, and it also changes the navigation model (one active section instead of a flat cross-section cursor).
adr_title: adr-0004-inbox-single-section
adr_path: docs/adr/0004-inbox-single-section.md
adr_note: the ADR supersedes the clause of ADR 0002 that anchors the prefix in the section header; the rest of ADR 0002 (prefix computation, ITEM width, tail truncation) stays in force.

## Expected outcome

The Inbox stops painting the three sections stacked: it shows **a single
active section** (`Mine`/`Assigned`/`Mentioned`), with **Assigned** on
opening, and `tab` cycles `Assigned → Mentioned → Mine → Assigned`. The
top-left border of the Inbox carries a **count legend** with the exact format
`Mine (9) · Assigned (4) · Mentioned (0)`, with the active one highlighted,
which replaces the `Inbox` title. The **common path prefix (ADR 0002) is
kept**: with a single section visible, all rows share a section, so the prefix
is computed on the active one and is still shown. Each section remembers its
cursor and its scroll. The `--print` mode, the snapshot/cache and the
`[keybindings]` API do not change.

## Scope

- **In**: active section + Assigned default + `tab` cycle; count legend on the
  top-left border; no internal section header; common prefix of the visible
  active section; per-section cursor/scroll; `(empty)`, `loading more…` and
  warnings of the active section.
- **Out**: `inbox.Build` (dedupe/authority) untouched; `--print` untouched;
  cache format untouched; config API untouched; forge adapters untouched;
  **the selectable/toggleable prefix is a LATER feature** (here only the seam
  is left open, with no new config nor keys).

## Approach (high level)

A **view and UI state** change in `internal/tui`, plus a short label in
`forge/model`. The data pipeline (streams → `inbox.Build` → sections) is not
touched: it only decides **which section is painted** and **how the list is
composed**. The state moves from a flat cursor over all rows to **one active
section with its own cursor/scroll**, saving the position of each section when
changing.

## Key decisions

1. **Single active section with its own state.** `Model` gains an active
   section (`activeSection`, default `model.SectionReview` = Assigned) and a
   per-section cursor/scroll. `rows()`/`selected()`/`clampCursor()` operate on
   the active section. On changing section the current position is saved and
   the destination one is restored (default 0/0), bounded to the new content.

2. **`tab` cycles, reusing `section-next`.** The action and its default key
   (`tab`) are kept; its effect changes: it advances the active section in the
   order `Assigned → Mentioned → Mine → Assigned` (**cycle** order different
   from the **legend** order, see decision 3). **It always cycles**, even if
   the destination section is empty (the user wants to be able to see its
   empty state and its count). `gotoNextSection`, `sectionOffsets` and
   `sectionIndexAtCursor` become obsolete and are removed. The shortcuts bar
   (`hintOrder`, label `section`) and the `[keybindings]` API do not change.

3. **Legend on the top-left border, replacing `Inbox`.** It is composed by
   iterating `m.inbox.Sections` (which already comes in `authored > review >
   mentions` order → `Mine · Assigned · Mentioned`) and it is passed as the
   **border title** of the Inbox box. Exact format:
   `Mine (9) · Assigned (4) · Mentioned (0)`. The active one is painted
   **highlighted** (color/bold) and the others **dimmed**. On a narrow terminal
   the border already truncates the title (ANSI-aware) on the right, so the box
   does not go out of alignment.

4. **Label mapping: `Legend()` without touching `String()`.**
   `func (s model.Section) Legend() string` → `Mine`/`Assigned`/`Mentioned`,
   next to `String()`. `String()` (`Created by me`/`Review / assigned`/
   `Mentions`) is **kept** because `--print` uses it (and it must not change)
   and it is the long label of the data mode. The legend uses `Legend()`; the
   rest, `String()`.

5. **Where the common path prefix lives (proposal).** On a **fixed line at the
   start of the list body**, dimmed, containing **only the prefix** (e.g.
   `  · APPCITTI/vsocial/backend/`), **with no title nor count**. If the active
   section has no common prefix (a single item, or nothing in common), the line
   is not painted and the ITEM cells carry the full path truncated from the
   tail (existing ADR 0002 rule).
   - **Why**: it keeps the ITEM box narrow (rows only paint the suffix), it
     respects the **exact format** of the legend (which is about counts) and it
     is the place where the prefix already was (it was part of the header).
     Also it is the **natural seam for the future toggle/selection**: the prefix
     is composed as a discrete unit from a single source, so a later feature
     can hide it, toggle it or make it interactive **without touching the
     legend, the table width nor the section change**. No config nor keys are
     added now.
   - **Discarded alternatives**:
     - *In the border legend*: it breaks the exact format and mixes the counts
       of the three sections with the path of only one; the legend is about
       counts.
     - *In the `PRDash` header*: it is far from the rows, it describes
       forge/refresh state, and the prefix changes with the active section →
       confusing.
     - *Full path in every ITEM cell (dropping the prefix)*: it loses exactly
       the readability that motivated ADR 0002 and contradicts the reason
       declared by the user for this feature.
     - *Reintroduce a section header with only the prefix*: it is the same as
       the chosen option but keeping "header" semantics and the duplicated
       count; it is discarded for nomenclature and for duplicating the legend.

6. **`newRefLayout` moves to the active section.** Today `listLines()` calls
   `newRefLayout(m.inbox.Sections)` and sizes ITEM by the longest suffix of
   **all** the sections. Now only one is painted, so it must receive **only the
   active section** (a one-element slice): that way the prefix and the ITEM
   width are the active section's, and no width is spent on sections that are
   not seen. The `newRefLayout([]inbox.Section)` **signature is kept** so the
   `refcol` unit tests do not break; the **caller** and the passed value change.

7. **States of the active section (open points resolved).**
   - **`(empty)`**: a line is shown in the list when the active section has no
     items and there are no warnings; the legend shows its count 0.
   - **`loading more…`**: it belongs to the **active section** and goes as a
     dimmed line at the end of the list body (not in the legend, which has an
     exact format, nor in `PRDash`, which is about forges). If the section
     paginating is not the active one, it is not shown; on returning to it, it
     reappears.
   - **Warnings (`sectionProblems`)**: they are shown **only for the active
     section**, as today (`⚠ <forge>: could not be queried (…)`), at the start
     of the body. Warnings of non-active sections are not painted (they appear
     on tabbing to that section). Accepted and documented risk.

8. **No changes in cache nor in `--print`.** The cache indexes by
   `(forge, section, kind)` and does not depend on what is painted →
   untouched. `--print` iterates `box.Sections` and uses `Section.String()` →
   untouched (that is why `String()` is not touched). It is verified, not
   modified.

## Affected files

**Production**

- `internal/forge/model/model.go`: add `Section.Legend()`; do not touch
  `String()`.
- `internal/tui/app.go`: `activeSection` (default `SectionReview`) +
  per-section cursor/scroll; `rows()`/`selected()`/`clampCursor()` on the
  active one; active-section helpers, active section prefix and legend
  descriptors.
- `internal/tui/update.go`: `case "section-next"` → active section cycle;
  remove `gotoNextSection`/`sectionOffsets`/`sectionIndexAtCursor`;
  `moveCursor`/`goTop`/`pageBy`/`syncScroll` on the active one.
- `internal/tui/list.go`: `listLines()` composes **only the active one**
  (optional prefix line + warnings + column header + rows, or `(empty)`, +
  `loading more…`); no section title; `newRefLayout` with the active section.
- `internal/tui/sections.go`: `listSection()` passes the **legend** as the
  border title (with highlight) instead of `"Inbox"`.
- `internal/tui/refcol.go`: `newRefLayout` stays the same; adjust comments and
  the use from `list.go`.
- `internal/tui/styles.go`: style for the active span of the legend and for the
  prefix line (reuse `styleDim` + a highlight).
- `internal/tui/bordered/bordered.go`: **no changes** (it accepts an already
  styled title); verify the ANSI case.
- `internal/config/config.go`: **no changes** (`section-next`/`tab` is kept).
- `cmd/prdash/print.go`: **no changes**.
- `docs/adr/0004-inbox-single-section.md`: new ADR (supersedes the ADR 0002
  clause about the header).

**Affected / new tests**

- `internal/tui/list_test.go`: rewrite `TestViewBoxesEverySection` (expects
  `Inbox`) and the ones that assume a flat list; new: exact legend + highlight,
  active `(empty)`, prefix line.
- `internal/tui/table_test.go`: rewrite `TestNavigationMovesCursor` and
  `TestSectionNextHonorsRebind` (active section cycle); update
  `newRefLayout(m.inbox.Sections)`.
- `internal/tui/refcol_test.go`: rewrite the integration ones on the header
  (`TestListLinesMuestranElPrefijoEnLaCabecera`,
  `TestListLinesWithNoPrefixKeepThePath`); the unit ones of `newRefLayout`
  are kept.
- `internal/tui/app_test.go`: `TestPaginationIndicator` must be on the active
  section (today it uses authored with default Assigned); rewrite
  `TestViewShowsThreeSectionsWithBothForges` (expects the 3 sections at once)
  and `TestSectionEmptyVsError` (expects several simultaneous `(empty)`, now
  only the active one is painted); keep the `sectionItems` ones.
- `internal/forge/model/model_test.go`: `Legend()` case.
- New: Assigned default; `tab` cycle; per-section cursor/scroll;
  `loading more…`/warnings of the active one; `--print` unchanged; legend on a
  narrow width.

## Risks

- **ANSI in the border title**: the border wraps the title with its color and
  the styled spans bring their resets; verify that neither the width nor the
  fill color of the top line break (the spans are composed with full styles,
  no "bare" characters).
- **Prefix + pagination**: the common prefix can shrink when more pages arrive
  and widen ITEM **once** (behavior already accepted in ADR 0002).
- **Warnings of non-active sections**: they stop being visible in the list
  (only their count). Accepted; the warning appears when tabbing to that
  section.
- **Tests coupled to the multi-section list**: the test rewrite scope is part
  of the work, not a minor side effect.
- **`syncScroll`/`scrollFor`**: they now operate on the active section's
  lines; make sure that saving/restoring per-section scroll does not leave the
  cursor outside the window.

## Work order (rough)

1. State: `activeSection` + per-section cursor/scroll + helpers; adapt
   `app.go`/`update.go`; remove the obsolete helpers.
2. Render: single-section `listLines` with prefix line; `newRefLayout` with the
   active one; `listSection` with the legend.
3. Labels: `model.Section.Legend()`; legend composition and highlight.
4. Open points: `(empty)`, `loading more…` and warnings of the active one.
5. Tests: rewrite the coupled ones and add the new ones.
6. ADR 0004 and closing.

## Verifications

- `go build ./... && go vet ./... && go test ./...` (repo runner, `make test`).
- Manual TUI smoke (tmux + `capture-pane`): Assigned default; `tab` cycle;
  exact legend with highlight; active section prefix visible; position
  remembered on return; `(empty)` and `loading more…`.
- `prdash --print` keeps showing the three sections with long names.
- Rebuild of the installed binary (`~/.local/bin/prdash`) when done.
