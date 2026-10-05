# ADR 0005 — Selectable and toggleable path prefix

- **Status**: Accepted
- **Date**: 2026-09-26
- **Decider**: user (buble)
- **Scope**: the TUI inbox, ITEM column, prefix line, hints bar
- **Naming pattern**: `docs/adr/NNNN-slug.md`
- **Supersedes**: **ADR 0002**, in its §Decision point 1 (the common path
  prefix is declared in the section header) and in point 4 ("the part lost at
  the front is the group, which the header already declares").
- **Also supersedes**: **ADR 0004**, in its §Decision point 4 (the prefix moves to
  a fixed line at the start of the list body), which this ADR turns into the
  line that is only painted in the default mode.
- **Leaves in force**: the common prefix computation (`sectionPrefix`:
  aligned on `/` boundaries, strict common prefix, never eating the final
  segment), the ITEM width formula (content plus the separating gap, bounded
  to `[6, 34]`), the tail truncation (`truncateTail`), the single-item section
  rule, and point 5 of ADR 0004 (the width is computed on the active section).

## Context

Since ADR 0002 an item's path is split in two: the prefix shared by the
section's items lives in a fixed line, and the ITEM cell only paints the
suffix. ADR 0004 moved that line to the start of the list body, by painting a
single section at a time, and left the seam written down:

> It leaves open the **seam for the selectable/toggleable prefix** (a later
> feature): the prefix is composed as a discrete unit from a single source,
> so a later feature can hide it, toggle it or make it interactive without
> touching the legend, the table width or the section change.

ADR 0002's split is the best of the possible ones **with a single option**, and
that is exactly what happens: there is no way to choose another one. And not
every reading prefers it equally:

- **The row stops being self-contained.** With the prefix on its own line, an
  item's reference is not in the row: you have to read two places. For whoever
  names a PR out loud, or reads the table row by row, or copies the reference,
  the full path in the cell is more direct even if the table is wider.
- **The prefix is a fixed line that cannot be turned off.** With density as
  the criterion, `APPCITTI/vsocial/` is a whole line for the group and
  identical text on every row. When only the leaf matters, it is in the way.
- **The prefix changes with the section.** `tab` jumps to sections whose
  common prefix is another one and rewrites the line above on every jump. With
  the path in the cell, every row is stable and the context does not have to
  be re-read.

None of the three cases is a whim: they are three legitimate readings of the
same table, and the design only offered one.

## Decision

1. **Three prefix modes, one global, cycled by a key.** The action
   `prefix-mode` (default `p`) cycles `common → full → leaf → common`. The
   mode is **global**, not per section, because the bar names it once and the
   view paints a single section: a per-section mode would have nowhere to
   announce itself.

   | Mode | Prefix line | ITEM cell |
   |---|---|---|
   | `common` (default) | yes, with the active section's common prefix | only the suffix |
   | `full` | no | the full reference `project/subgroup#n`, **tail-truncated if it does not fit** |
   | `leaf` | no | the project's leaf plus `#n` |

2. **`common` is the legacy behaviour** and it is not touched: it is the one
   of ADR 0002/0004, and being the default it is what whoever does not press
   the key sees. The feature **changes nothing** of what was seen before.

3. **Outside `common` there is no prefix line, because there is no prefix to
   declare.** It is not a decision of the view: the layout stops computing the
   common prefix in those modes, and the list —which already only painted the
   line when it came out non-empty— stops painting it by itself. The list
   reclaims that line of height.

4. **The ITEM width is recomputed per mode**, with the same formula and the
   same `[6, 34]` bound: the longest suffix in `common`, the full reference in
   `full`, the longest leaf in `leaf`. It is computed **once per render** (in
   `listLines`), never per row, so that header and rows measure the same and
   the table does not dance when writing over it.

5. **`common` degrades to `full` when the section has no common prefix** (a
   single item, or nothing in common): it does not invent a prefix nor repeat
   the path. It is not a special case but the behaviour it already had —with
   no prefix, the cell paints the whole reference—, and that is why the
   degradation needs no code of its own.

6. **The hint names the current mode** (`p prefix: full`), not the bare key:
   the information about which mode you are in has to be in the bar, or one
   would have to count keystrokes. Since the label depends on the state of the
   view and the config does not have it, `Config.Hints` receives a `HintState`
   (map by action) with the fragment the TUI composes. `hintOrder` keeps the
   list, the order and the default label; the rebind from `[keybindings]`
   stays generic.

7. **The mode is not persisted.** It lives in the `Model` and reopening the
   program goes back to `common`. No new field in `fileConfig` nor in the TOML
   schema.
   - Accepted consequence: by registering the action in
     `DefaultKeybindings`, the **key** `p` becomes TOML-configurable through
     the generic mechanism that already existed. The **mode** still does not
     persist. If it is ever wanted persistent, it is a new field in
     `fileConfig` and its test.

8. **`--print` does not change.** It always prints the full reference: it has
   no column nor terminal, so it does not have the problem the mode solves. A
   test pins its independence
   (`TestRunPrintDoesNotApplyThePrefixMode`).

9. **Selectable = choosing which prefix is seen**, not which item it applies
   to. The column is already dimensioned by the content of the section, and
   "the prefix of one particular item" is a degenerate case that `leaf`
   resolves better.

## Rejected alternatives

- **Per-item selectable prefix** (choosing a group and applying it to a single
  item). The column is dimensioned by the section's set; the prefix of one
  particular item is `full` if the path has no group, or `common` if it has
  one but the others do not share it. No use case that `leaf` does not cover.
- **More than three modes** (two-level prefix, path per subgroup): it multiplies
  the number of combinations of the bar and of the width without bringing a
  reading that cannot be achieved by cycling.
- **Prefix in the border legend.** It breaks the exact format of the legend
  (which is about counts) and mixes counts with path: it was already rejected
  in ADR 0004.
- **A bool flag in `[keybindings]` or in `[inbox]` like
  `prefix = "full"`.** It would make the mode persistent, which is exactly
  what was decided against. It can be added later without touching this: the
  mode already comes from a single type.
- **Substituting a placeholder in the label** (`label: "prefix: %s"` +
  `strings.Replace` in the TUI). It touches less code, but the bar seen from
  the config shows a raw `%s` and "the label is set by the TUI" blurs the
  invariant that `hintOrder` is the single source of the bar.
- **The TUI composing the `p` entry on its own** (as it already does with the
  merge Confirmation). It would take the `p` hint out of `hintOrder`'s order
  and out of `Config.Hints()`, which is exactly what the anti-drift guard
  `TestHintsCoverAllKeybindings` verifies.
- **Persisting the mode in the snapshot** (the mode is purely view state, but
  the snapshot already stores cursor and position). Rejected: the position is
  remembered because it is the user's work in progress; the prefix mode is a
  reading preference, and making it persistent ties the default behaviour to
  a previous run.

## Consequences

**Positive**

- The view adapts to the three readings without changing tool: the densest one
  for scanning, the most explicit one for reading or naming, the usual one for
  whoever already got used to it.
- The prefix stops being a fixed cost: in `full` and `leaf` the list
  reclaims the line it occupied.
- The seam ADR 0004 left open is closed **without touching** the legend, the
  section cycle, the per-section cursor nor the data pipeline.
- `hintOrder` remains the only source of the bar and the rebind via
  `[keybindings]` stays generic: the feature does not weaken any invariant of
  the bar.

**Negative / costs**

- **The reference stops being unique per row in `leaf`.** Two repos from
  different groups with the same leaf become indistinguishable
  (`acme/one#7` and `other/one#8` → `one#7` and `one#8`). It is inherent to
  the mode: its value is density when the leaves **are** unique, which is the
  normal case (the common prefix exists precisely because the leaves differ).
  The detail and `--print` keep the full path. **No disambiguation is
  promised.**
  - I correct here the justification that was being handled backwards: `leaf`
    is the mode that disambiguates the **least** of the two that do not use a
    prefix.
- **`newRefLayout` requires the mode as an explicit parameter** (16 call
  sites: 1 in production and 15 in tests). It is a deliberate cost: with a
  default, `newRefLayout(secs)` would mean "common" in some places and "whatever
  there was" in others, and the compiler would not catch the error.
- **`p` becomes configurable via `[keybindings]`** even though the mode is not
  persisted. It is the consequence of registering the action, and it is the
  generic mechanism that already existed; it is not new plumbing. **With a
  cost worth stating: `ActionForKey` resolves by alphabetical order of
  actions, so a user who had another action on `p` loses it** —`prefix-mode`
  goes before `quit`, `refresh`, `section-next` and `simulate`— with no
  warning. Duplicate-key detection is not added because it is a generic
  limitation of the map, previous to this feature, and fixing it exceeds its
  scope. Whoever has something on `p`, move it.
- **`full` is the mode that fares worst on a narrow terminal.** With ITEM at
  the cap (34) the column no longer fits next to FORGE (14) below **48 inner
  columns**, so `fitColumns` drops it to the right and the table is left with
  only FORGE —and precisely the reference you chose is the one lost—. In
  `common` and `leaf` the suffix is short and survives until 38. The cap of 34
  was set by ADR 0002 for the *suffix* and the formula is kept; the mechanism
  (losing columns to the right before information inside them) as well. It is
  accepted, and `TestFullLosesTheITEMColumnOnVeryNarrowTerminals` pins the
  threshold so that it does not move silently. **Fixing it at the root would
  require bounding ITEM as a function of the available width, not with a
  constant: it is a separate design decision, not a side effect of this
  one.**
- **The full reference almost never fits whole.** The usable text of ITEM is
  `itemWidthCap - 1 = 33` runes, so a 40-char subgroup path always comes out
  tail-truncated (`…/vsocial/backend/api-gateway#100`). `full` is not "the
  whole path" but "the reference in the cell, with the same truncation as
  always". The docs say so on purpose after having written it backwards.
- **Cycling the mode changes the width and the height of the list.**
  Re-synchronizing the scroll on every cycle is mandatory (without it, a
  scrolled list leaves the cursor outside the window). It is done and
  verified.
- **The name of the mode is lost below 42 columns of inner width.** The bar
  wraps and is bounded to `maxHintLines`; with 38 only three lines come out
  and the name goes with the truncation (bare `p prefix:`). It affects only
  `common`, whose name is the longest. It is the degradation of the bar the
  repo already accepts (that is why `quit` opens the list); `prefix-mode`
  goes behind `refresh` to get lost as late as possible, and the three modes
  read whole from 42 inner columns, that is, from a 44-column terminal
  (`TestTheHintNamesTheModeAtUsableWidths`).
- **One more cycle to memorize**, mitigated by the hint, which names the mode.

**Verification**

- `internal/tui/prefixmode_test.go`: `refLeaf` (subgroup, `owner/repo`, no
  subgroup, several items, empty project); the `common → full → leaf →
  common` cycle; the three exact `String()`s (they are what the user sees);
  `prefixOf` non-empty only in `common`; the width per mode with the
  `behavior.feature` fixture (24 / 34 capped / 16); the label per mode and the
  `common == full` degradation with no common prefix; the exact truncation in
  `full` (`…/vsocial/backend/api-gateway#100`); the case where `leaf` does
  **not** disambiguate.
- `internal/tui/prefixmode_list_test.go`: prefix line present only in
  `common` and absent in `full`/`leaf`; row label per mode; `full` and `leaf`
  reclaim exactly one line; the list in `common` with no common prefix is
  **identical** to the one in `full`.
- `internal/tui/prefixmode_key_test.go`: the cycle with the key pressed; the
  rebind (the new key cycles, the old one stops doing it); `tab` does not
  reset the mode nor when returning to the section; `p` does not arm merge, does
  not refresh, does not launch an action, does not leave a notice and does not
  move the cursor; the cursor and the window survive the cycle; a fresh model
  starts in `common` (it is not persisted).
- `internal/tui/prefixmode_hint_test.go`: the hint names the mode and changes
  when cycling; the rebind and the mode combine (`P prefix: leaf`); with the
  merge armed the Confirmation replaces the bar and the mode does not sneak
  into it.
- `internal/config/config_test.go`: the `p prefix` entry without state and
  `p prefix: full` with state; a state for an absent action does not invent an
  entry; the anti-drift guard `TestHintsCoverAllKeybindings` stays
  green.
- `internal/tui/refcol_test.go`: 300 random rounds of the column invariants,
  now **in the three modes** (non-empty cell, cell = exact truncation of the
  label, tail intact after truncating, width within `[6, 34]`, empty prefix
  outside `common`).
- `cmd/prdash/print_test.go`: `--print` prints the full path of a long
  subgroup, without `…` and without a prefix line.
- `make test` (build + vet + gofmt + `go test -race`). The repo has no CI
  configured: the guarantee is the local runner.
  **Correction (2026-10-04)**: the repo does have CI now —
  `.github/workflows/ci.yml` (build, lint, test) and
  `.github/workflows/mutation.yml` (mutation gate). `make check` is the local
  equivalent of `ci.yml`.
