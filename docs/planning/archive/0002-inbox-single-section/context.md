---
feature: 0002-inbox-single-section
freshness: 143fd59b9d8ff54d15b3cae8c00651f33424f034
codegraph: not_initialized
generated_by: codebase-researcher
---

# Context: Single-section inbox with a count legend

## Scope
- In: single active section (default `review`) with `tab` cycling `review → mentions → authored → review`; per-section cursor/scroll; count legend on the top-left border replacing `Inbox`; fixed line with the common prefix of the active section; `(empty)`/`loading more…`/warnings of the active section only.
- Out: `inbox.Build`, `--print`, cache format, `[keybindings]` API, adapters; selectable/toggleable prefix (only the seam is left open).

## Files to Touch
| Symbol / Area | File | Lines | Why |
|---------------|------|-------|-----|
| `Section.String()` | internal/forge/model/model.go | 22-35 | Keep; add `Legend()` right after it (`Mine`/`Assigned`/`Mentioned`). |
| `Section` consts | internal/forge/model/model.go | 10-20 | Mapping of `Legend()` per kind. |
| `Model` struct | internal/tui/app.go | 181-263 | Add `activeSection` + per-section cursor/scroll state; `cursor`/`scroll` (191-195) become the active section's ones. |
| `rows()` | internal/tui/app.go | 704-711 | It starts returning only the items of the active section. |
| `selected()` | internal/tui/app.go | 713-720 | It operates on the active one (via `rows()`). |
| `clampCursor()` | internal/tui/app.go | 694-702 | Bounds the active section's cursor. |
| `refreshSelfDenied()` | internal/tui/app.go | 684-692 | Iterates `rows()` → active only (own approve veto). |
| `sectionItems()` | internal/tui/app.go | 722-725 | Keeps reading `inbox.Section(kind)`; base for active-section helpers. |
| `sectionLoadingMore()` | internal/tui/app.go | 488-496 | Reuse for the active one. |
| `sectionProblems()` | internal/tui/app.go | 727-741 | Reuse with the active one only. |
| `rebuild()` | internal/tui/app.go | 621-629 | `clampCursor`/`syncScroll` on the active one. |
| `handleKey` | internal/tui/update.go | 165-224 | `end` (189-191) uses `rows()`; `case "section-next"` (200-203) starts cycling the active section. |
| `moveCursor` | internal/tui/update.go | 469-477 | Moves the active section's cursor + `syncScroll`. |
| `goTop` | internal/tui/update.go | 479-486 | Resets the active section's cursor/scroll. |
| `pageBy` | internal/tui/update.go | 488-497 | Pages over the active one. |
| `gotoNextSection` | internal/tui/update.go | 509-524 | **Delete** (obsolete). |
| `sectionOffsets` | internal/tui/update.go | 526-535 | **Delete** (obsolete). |
| `sectionIndexAtCursor` | internal/tui/update.go | 537-548 | **Delete** (obsolete). |
| `listLines()` | internal/tui/list.go | 25-63 | Rewrite: composes **only the active one** (optional prefix line + warnings + column header + rows, or `(empty)`, + `loading more…`); no section title. |
| `newRefLayout(m.inbox.Sections)` call | internal/tui/list.go | 30 | Pass only the active section (a slice of 1). |
| `syncScroll` | internal/tui/list.go | 94-101 | Operates on the active section's lines. |
| `scrollFor` / `visibleList` | internal/tui/list.go | 79-89 / 107-113 | No changes; kept and tested. |
| `newRefLayout` / `prefixOf` | internal/tui/refcol.go | 40-56 / 60-62 | Signature `newRefLayout([]inbox.Section)` **untouched**; adjust only comments (they say "the whole inbox"/"between sections"). |
| `sectionPrefix` / `refSuffix` / `truncateTail` | internal/tui/refcol.go | 70-96 / 101-111 / 116-128 | No logic changes. |
| `sectionLines` | internal/tui/sections.go | 64-74 | It already supports a styled title; it is the point where the legend enters. |
| `listSection` | internal/tui/sections.go | 111-131 | Replace `"Inbox"` (line 130) with the count legend; use the active section's scroll/cursor. |
| `itemCells` | internal/tui/table.go | 115-129 | Uses `l.prefixOf(sec)` (line 122); no logic change, the prefix now comes from the active section. |
| `renderCell` / `pad()` | internal/tui/table.go | 132-167 / 362-369 | Cell pattern: plain text + `pad()` BEFORE style. |
| styles | internal/tui/styles.go | 41-97 | Add a style for the active span of the legend (reuse `styleCount`/`styleDim`) and for the prefix line (`styleDim`); `borderColor`/`styleBorder` to repaint the border. |
| `RenderWithTitle(s)` / `borderLine` | internal/tui/bordered/bordered.go | 26-86 | **No changes**; it accepts an already styled title and truncates ANSI-aware (`ansi.StringWidth`/`Truncate`, 63-67). |
| `DefaultKeybindings` / `hintOrder` | internal/config/config.go | 281-291 / 419-439 | **No changes** (`section-next=tab`, label `section`). |
| `runPrint` | cmd/prdash/print.go | 47-64 | **No changes**; it uses `sec.Kind.String()` and `box.Sections` (`String()` must not be touched). |

## Contracts
- `inbox.Build(inputs) Inbox` → 3 `inbox.Section{Kind, Items}` in `authored > review > mentions` order/authority — `internal/inbox/inbox.go:38-59`. The order of `Sections` is the order of the legend (`Mine · Assigned · Mentioned`).
- `Inbox.Section(kind) Section` — `internal/inbox/inbox.go:106-114`; `Inbox.Empty()` — `:116-124`.
- `model.Section` + `String()` (`Created by me`/`Review / assigned`/`Mentions`) — `internal/forge/model/model.go:22-35`. **Do not touch** (it is used by `--print`). Add `Legend()` → `Mine`/`Assigned`/`Mentioned`.
- `newRefLayout([]inbox.Section) refLayout` — `internal/tui/refcol.go:40`; signature preserved (the `refcol` unit tests use it).
- `refLayout.prefixOf(model.Section) string` — `internal/tui/refcol.go:60`; `sectionPrefix(items)` — `:70`; `refSuffix(it, prefix)` — `:101`.
- `bordered.RenderWithTitles(border, borderFg, topTitle, topAlign, bottomTitle, bottomAlign, content, width)` receives the **already styled** title — `internal/tui/bordered/bordered.go:34`.
- `syncScroll` — `internal/tui/list.go:94`; `scrollFor(current,target,total,view)` — `:79`; `visibleList(lines,scroll,view)` — `:107`.

## Pattern to Follow
- **Boxes / border title** → `internal/tui/sections.go:64-74` (`sectionLines`: wraps `" "+title+" "` and calls `bordered.RenderWithTitle`) and `listSection` `:111-131` as the template for assembling the body (visible + fill to `lay.bodyLines`).
- **Cells (text, style) with `pad()` before style** → `internal/tui/table.go:132-167` (`renderCells`/`renderCell`) and `pad` `:362-369`.
- **Scroll that follows the cursor** → `internal/tui/list.go:94-101` (`syncScroll` → `scrollFor`) called from `internal/tui/update.go:469-497` (`moveCursor`/`pageBy`).
- **Direct model tests sending msgs** → `internal/tui/app_test.go:23-101` (`newTestModel`/`mkItem`/`page`/`send`/`press`).

## Tests
- Existing affected:
  - `internal/tui/list_test.go` — `TestViewBoxesEverySection` `:41-78` (expects the `Inbox` box, line 54); `TestScrollFollowsCursor` `:128-160`; `TestPageKeysMoveOneWindow` `:164-197` (uses `len(m.rows())`).
  - `internal/tui/table_test.go` — `TestNavigationMovesCursor` `:150-175` (cross-section cursor); `TestSectionNextHonorsRebind` `:177-204`; `TestColumnsSeparatedByOneSpace` `:74-107` (`newRefLayout(m.inbox.Sections)` line 81); `TestDiffColumnAppearsOnlyOnWideTerminals` `:264-293` (line 272).
  - `internal/tui/refcol_test.go` — integration `TestListLinesMuestranElPrefijoEnLaCabecera` `:157-189` and `TestListLinesWithNoPrefixKeepThePath` `:191-230`; the unit ones of `newRefLayout`/`sectionPrefix`/`refSuffix` (`:30-155`, `:235-303`) are kept.
  - `internal/tui/app_test.go` — `TestPaginationIndicator` `:177-188` (uses `authored` with default `review` → it stops being visible); `TestViewShowsThreeSectionsWithBothForges` `:121-143` (expects 3 section titles); `TestSectionEmptyVsError` `:145-158` (expects 2 `(empty)`); uses of `m.sectionItems(...)` (199, 205, 258, 443, 510, 583, 592, 745).
  - `internal/forge/model/model_test.go` — add a `Legend()` case.
- Framework / runner: Go testing; repo runner `make test` = `go build ./... && go vet ./... && gofmt -l . && go test -race ./...`.
- Integration infra: unit only (fake adapters `testutil.FakeAdapter`; cache isolated with `XDG_CACHE_HOME` in `newTestModel` app_test.go:25).
- Suggested targets: active section/default/cycle → `table_test.go`; legend + prefix + `(empty)` + `loading more…` → `list_test.go`/`refcol_test.go`; `Legend()` → `model_test.go`; `--print` non-regression → `cmd/prdash`.

## Conventions & Boundaries
- Comments in **Spanish** and **without** references to specs/IDs/scenarios; the code is the source of truth.
- The TUI touches neither network nor disk; the pipeline (`streams → inbox.Build`) is not modified, only what is painted.
- Cells return `(text, style)`; `pad()` BEFORE applying style (ANSI breaks the width).
- `--print` (uses `Section.String()` + `box.Sections`) and the cache format (`(forge,section,kind)`) do not change.
- `section-next`/`tab` keep coming from `[keybindings]`; the hints bar (`hintOrder`) does not change.

## Integration Points (non-obvious)
- **Border title with ANSI**: `sectionLines` (`sections.go:64-74`) already wraps the title with `" " + title + " "`, and `borderLine` (`bordered.go:59-86`) measures with `ansi.StringWidth` and re-wraps each segment with the border style (a `\x1b[0m` reset of an inner span does not restore the border color → compose the legend with **full style spans, no "bare" characters**, and trust the ANSI-aware truncation of the border).
- **`newRefLayout` must receive ONLY the active section** (a slice of 1): if it receives `m.inbox.Sections` it sizes ITEM by the longest suffix of all the sections and computes prefixes of sections that are not seen (`list.go:30`, `refcol.go:32-35`).
- **Cycle order ≠ legend order**: the legend iterates `m.inbox.Sections` (`authored·review·mentions` → `Mine·Assigned·Mentioned`); the cycle `review → mentions → authored → review` is `sectionOrder` +1 mod 3 (`inbox.go:40-44`), starting at `review`.
- **`m.rows()` becomes the active section's** and drags `selected`, `clampCursor`, `refreshSelfDenied`, `end` (`update.go:189-191`) and `pageBy`; `Rebuild` must re-bound the active section's cursor/scroll to the new content.
- **Prefix + pagination**: the common prefix can shrink when pages arrive and widen ITEM **once** (behavior already accepted in ADR 0002). With no common prefix, every ITEM cell carries the full path truncated from the tail (`truncateTail`).
- **Warnings**: of the active section only (accepted risk). A non-active section with a failure only shows its count in the legend; on tabbing to it its warning reappears (`sectionProblems` already filters by `w.Section == kind`).
- **`(empty)` vs warning**: do not paint `(empty)` if the active section has warnings (the "never empty if there was an error" principle); today `listLines:57` respects it (`case len(problems) == 0`).

## Risks / Assumptions
- **Legend ANSI**: verify on a narrow terminal that the top line measures exactly `outerWidth` and that the border fill color is not lost after the resets of the active spans.
- **Coupled tests**: the rewrite scope (list/table/refcol/app) is part of the work, not a side effect; `TestViewShowsThreeSectionsWithBothForges` and `TestSectionEmptyVsError` also fall even though they are not in the plan.
- **`syncScroll`/`scrollFor`** now operate on the active section's lines; make sure that saving/restoring per-section scroll does not leave the cursor outside the window.
- **`Legend()` vs `String()`**: keeping `String()` intact is what protects `--print`; any change there breaks the non-regression scenario.
- **Default `review`**: `TestPaginationIndicator` uses `authored`; with default `review` the indicator is not painted until tabbing — rewrite the test on the active section.
