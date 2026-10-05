# 0002 — Single-section inbox with a count legend (feature)

## Problem

The Inbox paints the three sections stacked (Mine / Assigned / Mentioned) and
the user has to scroll through a long list to reach the section they care
about. The `tab` key (action `section-next`) today only moves the cursor to the
first item of the next section: it does not change the focus nor reduce the
noise of the other sections, which keep taking screen. There is no direct way
to "see only what falls to me now" nor to know at a glance how many items are
in each section.

The goal is for the Inbox to show **a single section at a time**, with a cycle
through `tab` and a **count legend** on the top border, so that the focus and
the context (how many are in each section) are always visible without
scrolling.

## Scope (in)

- **Single active section**: only one section of the Inbox is painted at a
  time.
  - `Mine` = `authored`, `Assigned` = `review`, `Mentioned` = `mentions`.
  - On opening, the default section is **Assigned** (`review`).
  - `tab` cycles **Assigned → Mentioned → Mine → Assigned** (the `tab` key is
    kept; the action is still `section-next`).
- **Count legend** on the **top-left** border of the Inbox, which **replaces
  the current `Inbox` title**. Exact format:
  `Mine (9) · Assigned (4) · Mentioned (0)`. The numbers are the item count of
  each section (already deduped by `inbox.Build`). The active section is
  **highlighted** (color/bold) and the others are dimmed.
- **No internal section header**: the `"<Section> (n)"` line inside the list
  is removed.
- **Per-section cursor and scroll**: each section remembers its position when
  returning to it.
- Legend labels: `Mine`, `Assigned`, `Mentioned`.

## Out of scope (out)

- The consolidation/dedupe/authority of `inbox.Build` (authorship
  `authored > review > mentions`) does not change: only what is **painted**
  changes.
- The `--print` mode (`cmd/prdash/print.go`) does not change behavior.
- The snapshot/cache format does not change.
- The `[keybindings]` API does not change: `section-next` keeps existing and
  `tab` keeps being its default.
- Neither the forge adapters nor the parsing are touched.

## Acceptance criteria

- [ ] On opening the TUI, the Inbox shows only the **Assigned** section.
- [ ] `tab` cycles Assigned → Mentioned → Mine → Assigned and only the active
      section is painted.
- [ ] The legend on the top-left border has the format
      `Mine (n) · Assigned (n) · Mentioned (n)` with the real counts and the
      active section highlighted; the `Inbox` title does not appear.
- [ ] There is no internal section header in the list.
- [ ] Returning to a section restores its cursor and its scroll.
- [ ] The empty active section shows `(empty)`; the one still paginating shows
      `loading more…`; its warnings keep being shown (of the active one only).
- [ ] `--print` keeps printing the three sections with their long names.
- [ ] `go build ./... && go vet ./... && go test ./...` passes.

## Risks / pending verifications

- **Warnings of non-active sections**: a query failure of a non-active section
  is no longer seen in the list (only its count is seen). Accepted: on tabbing
  to that section its warning appears. Confirm that the "never show empty if
  there was an error" principle is not broken in a confusing way.
- **Legend on a border with ANSI**: the border title is wrapped with the
  border color; the spans with their own style and their resets must not break
  the width nor the fill color of the top line. Verify on narrow terminals (the
  legend is truncated on the right, ANSI-aware).
- **Common path prefix (ADR 0002)**: today the prefix lives in the section
  header, which disappears. It has to be decided where it stays (see
  `plan.md`) so the readability that motivated the ADR is not lost.
- **Coupled tests**: `TestViewBoxesEverySection` (expects the `Inbox` box),
  `TestSectionNext*`/`TestNavigationMovesCursor` (assume a cross-section
  cursor), `TestListLines*` (assume a section header with prefix) and
  `TestPaginationIndicator` (assumes authored is painted) must be rewritten.
