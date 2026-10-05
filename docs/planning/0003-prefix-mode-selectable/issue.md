# 0003 — Selectable and toggleable path prefix (feature)

## Problem

The Inbox shows **only one section at a time** (ADR 0004) and its path is split
in two: a **common prefix** on a fixed dimmed line at the start of the body, and
the **ITEM cell** with only the suffix (ADR 0002). It is the best split possible
with a single option, but today it is the **only** option: there is no way to
choose another one.

That single design does not serve everyone equally, and the three cases it fails
are not a matter of taste:

1. **Whoever does not read in two places.** With the prefix on its line, the
   reference of an item is not on the row: you have to look at the line above and
   the one below. The full path in the cell is slower to read on long tables, but
   it is self-contained: it works for a lone item, to copy, to say out loud and
   for a table read row by row.
2. **Whoever wants density.** The common prefix is generous: `APPCITTI/vsocial/`
   takes a whole line for the group and pays the same price on every row. When
   only the leaf matters (`api-gateway#1234`), that line is an expense.
3. **Whoever changes section with different prefixes.** `tab` jumps between
   sections whose prefix changes, and every jump rewrites the line above. With
   the path in the cell, every row is stable and the context does not have to be
   re-read.

ADR 0004 (§Consequences) left this seam **explicitly open**: the prefix is
composed as a discrete unit from a single source, so a later feature can hide
it, toggle it or make it interactive **without touching the legend, the table
width or the section change**.

## Scope (in)

One key, **`p`** (action `prefix-mode`, default `p`), cycles **3 prefix modes**.
It is **global**, not per section, and **not persisted** (on reopening the
program it goes back to `common`).

| Mode | Prefix line | ITEM cell | What for |
|---|---|---|---|
| `common` (default) | yes, with the common prefix of the active one | only the suffix | current behavior (ADR 0002/0004) |
| `full` | **no** | the **complete** reference `project/subgroup#n` | self-contained row |
| `leaf` | **no** | only the **leaf** of the project plus `#n` | maximum density |

- Cycle: `common` → `full` → `leaf` → `common`.
- **Selectable = choosing which prefix is seen**, not which item it applies to.
  The ITEM column is already sized by the content of the section and "the prefix
  of one particular item" is a degenerate case that `leaf` already solves better.
- The **ITEM width is recomputed per mode** (longest suffix vs full reference vs
  longest leaf), with the same bounds `[itemWidthMin, itemWidthCap]`
  (`[6, 34]`). **No flicker**: the layout is computed **once per render** in
  `listLines`, never per row.
- In `full` and `leaf` **the prefix line is not painted** (there is no prefix to
  declare) and the list recovers that line of height.
- **Degradation**: if the active section has no common prefix (<2 items or
  nothing in common), `common` **degrades to the behavior of `full`**: it does
  not invent a prefix nor repeat the path twice.
- The **hint of `p` names the current mode** (`p prefix: full`), not the bare
  key, so you know where you are without counting keystrokes.

## Out of scope (out)

- **Do NOT persist the mode.** Nothing new in `fileConfig` nor in the TOML
  schema. By registering the action in `DefaultKeybindings` + `hintOrder`,
  `[keybindings]` will be able to override it through the generic mechanism that
  already exists: that is correct, no plumbing is needed for that.
- **Do NOT touch `--print`**: `cmd/prdash/print.go` does not use `newRefLayout` /
  `refSuffix` / `sectionPrefix` (verified). That independence is kept and a
  **test that pins it is added**.
- **Do NOT change** section dedupe/authority nor the `tab` cycle.
- **Do NOT** make the prefix selectable per item (see Problem, decision 1).
- **Do NOT** change the computation of `sectionPrefix` (it stays the strict
  common prefix, aligned on `/` and never eating the leaf).
- Key `p`: does not collide with `q`, `R`, `r`, `a`, `m`, `tab`, `o`, `j`/`k`,
  `home`/`end`, `pgup`/`pgdn`, nor with the merge mode keys
  (`m`/`r`/`s`), which consume the whole keystroke while armed.

## Acceptance criteria

- [ ] On opening the TUI the mode is `common` and the view is **exactly** today's
      (prefix line + suffixes).
- [ ] `p` cycles `common` → `full` → `leaf` → `common`, in that order, and the
      cycle is global: it is not reset when changing section with `tab`.
- [ ] `full`: there is no prefix line and every ITEM cell carries the full
      reference, truncated from the tail if it does not fit.
- [ ] `leaf`: there is no prefix line and every ITEM cell carries
      `<last segment of the project>#n`.
- [ ] The **ITEM width changes with the mode** and stays within `[6, 34]` in the
      three of them; the layout is computed once per render (all rows of the
      table measure the same, no flicker when typing on top).
- [ ] `full` and `leaf` **recover the height line** the prefix used to take.
- [ ] With a section without common prefix, `common` looks **exactly** like
      `full`: no prefix line and with the full path in the cell.
- [ ] The bar hint names the current mode and changes when cycling; with an
      override in `[keybindings]` it shows the new key **and** the current mode.
- [ ] `tab` (section cycle) does not change the mode; the per-section cursor and
      scroll stay the same.
- [ ] `--print` **does not change**: it keeps printing the three sections with
      the complete `project/subgroup#n`.
- [ ] `make test` green (build + vet + gofmt + `go test -race`).
- [ ] Coherent ADRs: the prefix clause of ADR 0002 is updated or a 0005 is
      written to supersede it; CHANGELOG and README without contradictions.

## OPEN decision (blocks the plan) — the dynamic hint

**The statement asks for two things that are incompatible in the current code.**

- *"The hint of `p` must name the CURRENT mode"* → the label depends on runtime
  state (`m.prefixMode`).
- *"register the action in `DefaultKeybindings` + `hintOrder` … no new plumbing
  needed"* → the label lives in `hintOrder` and is resolved without state.

Verified in the code:

- `internal/config/config.go:414-418` — `type hint struct{ action, key, label string }`:
  `label` is a **static string**.
- `internal/config/config.go:430-441` — `var hintOrder = []hint{…}`: no state.
- `internal/config/config.go:446-459` — `func (c Config) Hints() []string`: a
  function **pure on `Config`**, with no state parameters.
- `internal/tui/sections.go:213` — sole caller: `m.cfg.Hints()`. The `Model`
  (and with it the mode) **is** available there.
- `internal/config` **cannot** import `internal/tui`: the cycle is the other way
  around (`internal/tui/app.go:191` already does `cfg config.Config`).

In other words: the **key** does get resolved by the generic mechanism that
already exists (`KeyFor` → `[keybindings]`, no plumbing). What has **no**
mechanism is the **dynamic label**. One of these three has to be chosen:

- **A (recommended) — `Config.Hints()` receives the dynamic state.** The label of
  `hintOrder` becomes the default value and the TUI overrides it:
  `m.cfg.Hints(map[string]string{"prefix-mode": "full"})`, or an equivalent
  shape (variadic/struct). **It preserves all the documented invariants**:
  `hintOrder` keeps being the only source of list, order and default label; the
  `[keybindings]` rebind keeps working; the anti-drift guard
  (`TestHintsCoverAllKeybindings`) still holds. Cost: the signature of
  `Hints()` changes and there is a map/struct per render.
- **B — placeholder in the label + substitution in the TUI.** `label: "prefix: %s"`
  and `strings.Replace` in `sections.go`. It touches less code, but the bar seen
  from the config shows a raw `%s`, `TestHints` (config_test.go:327, literal
  `want` list) would have to expect the placeholder, and "the label is set by the
  TUI" blurs the invariant of `hintOrder` as the single source.
- **C — the TUI composes that fragment separately**, as it already does with
  `mergeConfirmText()` when the merge is armed (`sections.go:209-213`). A real
  precedent, but it takes the hint of `p` out of the order of `hintOrder` and of
  `Config.Hints()`, which is exactly what the anti-drift guard verifies.

**Question for the user**: A, B or C? The statement assumed there was no
plumbing; the reality is that the dynamic label needs one of the three, and the
decision changes the signature of `Hints()` and the shape of the tests.

## Micro-decisions (non-blocking; proposed in the plan)

- **Order of the hint in the bar.** `maxHintLines = 3` and the trim throws
  **from the tail** (`config.go:426-429`, `wrapHint`): what goes at the end
  disappears first on a narrow terminal. Proposal: `p` goes **after `refresh`**
  and before the fixed keys `j/k` and `pgup/dn` — it survives better than the
  tail, but does not steal room from `quit`/`section`/actions.
- **`TestHints`** has a literal `want` list: `p prefix: …` has to be added
  there. That is the guard working, not a surprise.
- **State in `Model`, zero schema.** `prefixMode` as a field of `Model`, with
  default `common`. No `fileConfig`, no TOML, no migrations.

## Correction to the statement (non-blocking, but falsehoods are not documented)

The statement justifies `leaf` with the claim that it *"disambiguates when the
section mixes repos without a common prefix"*. **That is the other way around**:
`leaf` is precisely the mode that does **not** disambiguate that case. For
`a/one#1` and `b/one#2` (nothing in common) `leaf` paints `one#1` and `one#2` —
indistinguishable — while `full` paints `a/one#1` and `b/one#2`. The real value of
`leaf` is **density** when the leaves **are** unique (which is the normal case:
the common prefix exists precisely because the leaves differ), not
disambiguation. The mode is implemented the same way; what changes is the phrase
of the ADR, which must not assert a property the mode does not have.

## Risks / pending verifications

- **The mode is not persisted**: that is what was asked. The warning for the
  future is that `p` **does** end up configurable by TOML once it goes through
  `DefaultKeybindings` (generic mechanism), even though the mode is not
  plumbed. If the mode is ever wanted as persistent, it is a new field in
  `fileConfig` + its test.
- **Loss of context when moving to `full`/`leaf`**: the group information
  disappears from the view for the items of the row, not from the application.
  The detail and `--print` keep showing the full path. Verify that the view
  never shows an item without `project#n`.
- **Reflow when cycling**: changing mode changes the ITEM width and, in
  `full`/`leaf`, the list recovers one line. It is an instant and deterministic
  reflow (no per-item state), but the check that the **cursor does not leave the
  window** when cycling is needed: `p` does not move the cursor, but it does
  change `len(listLines)` (one line less), so the `scroll` can end up out of
  sync → `syncScroll()` is needed in the cycle.
- **`hintOrder` is global, not per section**: correct, but it means the hint
  names a mode that does **not** apply equally to the three sections at once.
  It is coherent with only one section being painted.
- **Coupled tests**: `TestHints` (literal list), the `refcol` ones on
  `newRefLayout` (its signature/computation), the `listLines` ones on the prefix
  line and the `section_test.go` ones on the prefix of the active one. All of
  them adapt to the default mode (`common`) and still hold as they are.
