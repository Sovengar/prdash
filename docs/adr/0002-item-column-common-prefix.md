# ADR 0002 — ITEM column with a common path prefix per section

- **Status**: Accepted
- **Date**: 2026-09-26
- **Decider**: user (buble)
- **Scope**: the TUI inbox, the ITEM column
- **Naming pattern**: `docs/adr/NNNN-slug.md`

## Context

The ITEM column painted `project#number` with a fixed width of 30 runes
(`colRef`) and truncated **from the head**. GitLab project paths with
subgroups did not fit: `APPCITTI/vsocial/backend/api-gateway#1234` is 41
runes and it printed `APPCITTI/vsocial/backend/api-…`.

Head truncation eats exactly what distinguishes one item from another: the
repo name (the leaf of the path) and the `#number`. Two rows from different
projects ended up visually identical, which is the worst way to lose
information: you do not notice it is missing.

It was not data loss: the detail (`enter`) and `--print` already showed the
full path. The problem was legibility in the table, and the convention already
established in the repo for the FORGE column was "short in the table, full in
the detail".

Starting constraints:

- The column must line up across sections and across rows, or the table dances
  when writing over it.
- STATE and CHECKS are never truncated; at narrow widths columns are lost to
  the right, not information inside them.
- The ability to reference an item by its number must not be lost, which is
  what is used when talking about it and when verifying a check.

## Decision

1. **Each section's common path prefix is declared in its header** and the
   ITEM cells only paint the suffix. The prefix is computed over the full
   segment intersection of all the items of the section, aligned on `/`
   boundaries and **never eating the final segment**: the cell always keeps
   the leaf and the number.
2. **The ITEM width comes from the content**, not from a constant: the longest
   suffix of all the sections plus the separating gap, bounded to `[6, 34]`
   runes of slot. The minimum keeps the separation from the neighbouring
   column; the ceiling keeps ITEM from eating TITLE.
3. **The width of a column includes its separating gap** (the usable text is
   one rune shorter, `textWidth`), so the text is truncated to the gap and
   never fills the slot. It was not a hypothetical case: ROLE with
   `"review req"` measured exactly its column width, so STATE came out glued
   to it, and the same happened with ITEM, whose dynamic width is computed by
   the content itself.
4. **Whatever still does not fit is truncated by the tail**, not by the head,
   so that the `#number` survives. The part lost at the front is the group,
   which the header already declares.
5. A single-item section declares no prefix (there is nothing to share) and
   its cell carries the path alone: in full if it fits, tail-truncated if not.

## Rejected alternatives

- **Tail truncation on the whole cell** (`APPCITTI/vs…/api-gateway#1234`).
  It is the cheapest fix and it repairs the lost number, but every row repeats
  the whole group: it still eats the space that the common prefix frees up.
- **Project leaf in ITEM and group in the FORGE column** (`GLab@app` +
  `api-gateway#1234`). Maximum compactness, but mixed semantics: FORGE stops
  being readable at a glance; and with items from different groups the group
  is lost.
- **Dynamic widths for all columns** measuring the content. It solves the
  generic symptom, but it is more invasive (it forces `fitColumns` to be
  resized) and it does not fix the long path repeating the group on every row.
- **Item on two lines** when the path does not fit. It breaks the compactness
  of the table and the arithmetic of the cursor, which is the most fragile
  part of the scrolled view.

## Consequences

**Positive**

- With long subgroups the row says which project and which number it is,
  without truncating: `mobile-frontend#1198` instead of
  `APPCITTI/vsocial/backend/mobile-…`.
- The column adapts to the real inbox: with short paths it narrows and TITLE
  gains space; with long paths it does not widen past the cap.
- It is deterministic: it comes only from the current item set, with no extra
  state nor keys that could diverge from what is painted.

**Negative / costs**

- The ITEM width depends on the content: when a page finishes loading, more
  items can shrink the common prefix and widen the column **once**. Bounded by
  the cap of 34, and it is a single reflow.
- A single-item section with a long path has no prefix to offset the
  truncation: you see `…social/backend/api-gateway#100`, without `APPCITTI/`.
  The detail keeps being the safety net.
- `refLayout` indexes the prefix by `model.Section`: two sections with the
  same `Kind` would overwrite each other. Today it is impossible
  (`inbox.Build` guarantees one per kind) and the failure mode is safe (the
  cell falls back to the full path), but it is an invariant to respect if
  there are ever duplicate sections.
- The ITEM width is computed once per render, in `listLines`. With many
  sections and items it is O(items × segments) on every frame; negligible at
  this scale, and it prevents header and rows from measuring differently.

**Verification**

- `internal/tui/refcol_test.go`: 10 targeted prefix cases (long subgroup, cut
  mid-path, nothing in common, `owner/repo`, identical projects, single-item
  section, partial prefixes, empty project, same path on different hosts),
  plus the suffix, truncation and width ones; two integration ones over
  `listLines`; and 300 random rounds over the invariants (non-empty cell, cell
  = exact truncation of the suffix, tail intact after truncating, width
  within bounds).
