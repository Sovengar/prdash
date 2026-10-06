# 0003 — Selectable and toggleable path prefix — Plan

adr_required: true
adr_reason: the feature turns the common path prefix (Accepted, ADR 0002 §Decision 1, relocated by ADR 0004 §Decision 4) into a mode **selectable** among three, which changes what is seen by default in the table and what the ITEM width is. It is a design decision with real alternatives (which three modes, whether the prefix is persisted, where the hint label lives), not a refactor.
adr_title: adr-0005-selectable-prefix-mode
adr_path: docs/adr/0005-selectable-prefix-mode.md
adr_note: ADR 0005 supersedes the clause of ADR 0002 that anchors the prefix in a header (and the one of ADR 0004 that fixes it in an always-visible line): it becomes the default mode `common` of a selectable mode set. The prefix computation (aligned on `/`, never eating the leaf), the width formula bounded to `[6, 34]` and the tail truncation stay in force exactly as they are.

## Expected outcome

The ITEM column stops having a single way of showing the path. One key **`p`**
(action `prefix-mode`) cycles **three global modes**:

| Mode | Prefix line | ITEM cell | ITEM width (`behavior.feature` fixture) |
|---|---|---|---|
| `common` (default) | yes, with the common prefix of the active one | only the suffix | 24 |
| `full` | **no** | the **complete** reference | 34 (capped) |
| `leaf` | **no** | the **leaf** of the project + `#n` | 16 |

The ITEM width **is recomputed per mode** with the same bounds `[6, 34]`, once
per render. In `full` and `leaf` the list **recovers the line** the prefix used
to take. If the active section has no common prefix, `common` **degrades to
`full`** without inventing anything. The hint of `p` names the **current mode**
(`p prefix: full`). The mode **is not persisted** and `--print` does not change.

## Scope

- **In**: the `prefixMode` type + cycle; `prefixMode` in `Model` (default
  `common`); the ITEM cell label per mode; ITEM width per mode; the prefix line
  only in `common`; action `prefix-mode` (`p`) in `DefaultKeybindings` +
  `hintOrder`; hint with the current mode; `syncScroll` when cycling.
- **Out**: persistence (nothing in `fileConfig` nor in the TOML schema);
  `--print`; `inbox.Build` and section authority/dedupe; the `tab` cycle and the
  per-section cursor/scroll; `sectionPrefix` (the common prefix computation does
  not change); the detail; the legend; the data pipeline.

## Approach (high level)

The prefix **is already composed as a discrete unit from a single source**
(`refLayout`), which is exactly the seam ADR 0004 left. The feature does not
touch the data pipeline nor the layout: it only decides **what label the ITEM
cell puts** and **at what width the column is sized**, according to an integer of
three values that lives in the `Model`.

The key point of the design: `refLayout` already computes the prefix per section
and `listLines` **already** only paints the prefix line if it comes out non-empty.
So in `full` and `leaf` it is enough for the layout to **not** declare a prefix
(empty) and the line disappears by itself, without touching `listLines`. The
`common` to `full` degradation is the same mechanism: with no common prefix, the
prefix already comes out empty today.

## Key decisions

1. **A three-value `prefixMode` type in `internal/tui`, not in `config`.**
   It is pure view state: `config` must not know about it (and cannot, without an
   import cycle). It lives in `refcol.go`, which already owns "what part of the
   reference is shown".
   - `String()` → `common`/`full`/`leaf` (that is what goes to the hint).
   - `next()` → `(p+1) % 3`: the cycle is total and needs no table, so it cannot
     fall out of sync with the hint (which derives from the same `String()`).

2. **`newRefLayout` receives the mode explicitly** —
   `newRefLayout(sections []inbox.Section, mode prefixMode)`.
   - **Why not a default**: 9 test calls and 1 production one. In exchange, the
     **compiler forces every site to name a mode**: there is no way to paint a
     layout in a mode other than the one it believes in. A signature with a
     default would leave `newRefLayout(secs)` meaning "common" in some places and
     "whatever there was" in others.
   - The 9 existing tests move to explicit `prefixCommon`, and that **turns the
     current suite into the guard of "the default is common and changes nothing"**,
     which is precisely the first acceptance criterion.

3. **A single labeling function per mode**: `refCellText(it, mode, prefix)`.
   - `prefixLeaf` → `refLeaf(it)` (leaf + `#n`).
   - `prefixCommon`/`prefixFull` → `refSuffix(it, prefix)`, which **already**
     degrades to the full reference when the prefix is empty.
   - The new name is explicit because `refSuffix` stops being "the" label of the
     cell: it is only the label of two of the three modes.

4. **The prefix is only computed in `common`.** In `full` and `leaf` the prefix
   map keeps `""` for the section, and `listLines:46` (which already compares
   against `""`) does not paint the line. **Zero changes in `listLines` besides
   passing the mode**, and the invariant "in `full`/`leaf` there is no prefix
   line" does not depend on a new `if` that could be forgotten.

5. **`refLeaf(it)`** cuts the project at the **last** `/` and appends `#n`.
   `strings.LastIndex` on `""` returns `-1`, so an empty project yields `"#7"` —
   which is exactly `refLabel` without a project, and it does not break the width
   (there is already a test for the empty project).

6. **`syncScroll()` when cycling.** `p` does not move the cursor, but `full` and
   `leaf` **remove one line** from `listLines`. Without re-syncing, a high
   `scroll` would leave the cursor outside the window at the very moment the mode
   changes. It is the same reason `goTop` resets `scroll` by hand.

7. **Action `prefix-mode` in `DefaultKeybindings` + `hintOrder`.** Default `p`,
   free (verified against `q`,`ctrl+c`,`R`,`r`,`a`,`m`,`v`,`tab`,`o`,`j`/`k`,
   `up`/`down`,`home`/`end`,`pgup`/`pgdown` and the merge mode keys `m`/`r`/`s`).
   - `ActionForKey` resolves by action, so the `[keybindings]` rebind is
     **free and generic**: by registering the action, `p` becomes configurable
     by TOML without touching one more line. It is the accepted consequence of
     the "no persistence" decision: the **key** is configurable, the **mode** is
     not.
   - Order on the bar: **after `refresh`**, before the fixed keys `j/k` and
     `pgup/dn`. The trim to `maxHintLines` throws **from the tail**, so at the
     end it would disappear first precisely on a narrow terminal, which is where
     it is most needed to know which mode you are in.

8. **Dynamic hint through option A: `Config.Hints(state HintState)`.**
   - `HintState` = `map[string]string` indexed **by action**, whose value is the
     fragment joined to the label with `": "`. `hintOrder` keeps the prefix of
     the label (`"prefix"`), and the TUI puts the value (`"full"`) →
     `p prefix: full`.
   - **Why not a `%s` placeholder + `strings.Replace`**: the bar seen from the
     config would show a raw `%s`, and "the label is set by the TUI" blurs the
     invariant that `hintOrder` is the single source of the bar.
   - **Why not have the TUI compose it entirely** (as `mergeConfirmText` does):
     it would take the hint of `p` out of the order of `hintOrder` and of
     `Config.Hints()`, which is precisely what `TestHintsCoverAllKeybindings`
     verifies.
   - Signature with an **explicit parameter** (not variadic) so that no caller
     forgets that the bar has a state dependency.

9. **ADR 0005, do not edit the 0002.** ADRs are a record: the 0005 declares which
   clause it supersedes and from which ADR, as the 0004 did with the 0002. That
   way the chain stays readable (0002 → 0004 → 0005) without rewriting history.
   In the 0005 the **justification** of `leaf` is also corrected: it does not
   disambiguate (for `acme/one#7` and `other/one#8` it would paint `one#7` and
   `one#8`); its value is density when the leaves **are** unique.

## Affected files

**Production**

- `internal/tui/refcol.go`: `prefixMode` type (+`String`, `next`), `refLeaf`,
  `refCellText`; `refLayout` gains `mode`; `newRefLayout` computes the prefix
  only in `common` and measures with the mode label. `sectionPrefix`, `refSuffix`
  and `truncateTail` **without logic changes**.
- `internal/tui/app.go`: `prefixMode` in `Model` (default `prefixCommon`).
- `internal/tui/list.go`: pass `m.prefixMode` to `newRefLayout` (single line).
- `internal/tui/table.go:122`: `refCellText(it, l.mode, l.prefixOf(sec))`.
- `internal/tui/update.go`: `case "prefix-mode"` in the `switch` of
  `ActionForKey` + `cyclePrefixMode()` (cycle + `syncScroll`).
- `internal/config/config.go`: `HintState` + `Hints(state)`; `"prefix-mode": "p"`
  in `DefaultKeybindings`; `{action: "prefix-mode", label: "prefix"}` in
  `hintOrder` behind `refresh`.
- `internal/tui/sections.go:213`: `m.cfg.Hints(m.dynamicHints())` +
  `dynamicHints()` (returns the current mode).
- `docs/adr/0005-selectable-prefix-mode.md`: new.
- `README.md`: `p` in the key list + paragraph about the 3 modes.
- `CHANGELOG.md`: entry in `## [Unreleased] → ### Added`.

**Affected / new tests**

- `internal/config/config_test.go`: `TestHints` (literal `want` list + state),
  `TestHintsCoverAllKeybindings` (stays green alone), `TestHintsFollowTheRebind`
  (with `HintState`), **new** `TestHintsWithDynamicState` (default label +
  overwrite + rebind combined).
- `internal/tui/refcol_test.go`: 9 `newRefLayout(...)` → `+ prefixCommon`;
  **new**: `refLeaf` (leaf, subgroup, empty project, no subgroup), cycle of
  `next()`, width per mode (24/34/16), `prefixOf` empty in `full`/`leaf`.
- `internal/tui/table_test.go`: 3 `newRefLayout(...)` → `+ prefixCommon`.
- `internal/tui/list_test.go`: **new** integration ones on `listLines` — prefix
  line present in `common` and absent in `full`/`leaf`; `full`/`leaf` recover one
  line; degradation `common` == `full`.
- `internal/tui/section_test.go`: **new** ones for the `p` key — full cycle,
  rebind, `tab` does not reset it, and that `p` does not fire
  approve/merge/refresh/quit.
- `internal/tui/app_test.go` or `section_test.go`: **new** one for the hint — the
  bar names the mode and changes when cycling, and with rebind it shows the new
  key + mode.
- `cmd/prdash/print_test.go`: **new** `TestRunPrintNoAplicaElModoPrefijo` — the
  full path comes out whole with a subgroup path, without a prefix line, even if
  the TUI mode is `leaf`. It pins the independence of `--print`.

## Risks

- **Reflow when cycling**: it changes the ITEM width and, in `full`/`leaf`, the
  length of the list. Mitigated with `syncScroll()`; to verify with the cursor
  scrolled.
- **Loss of context in `leaf`**: the reference is no longer unique per row. It is
  inherent to the mode (and that is why the detail and `--print` keep the full
  path); the mode hint warns about which mode you are in.
- **The hints anti-drift guard**: `TestHintsCoverAllKeybindings` requires
  that the key of every action appear on the bar. `p` appears, so it passes. If
  someone registers an action and forgets `hintOrder`, that test trips.
- **Tests coupled to `newRefLayout`**: 9 calls. It is mechanical work, not risk,
  but they are 9 sites where a wrongly placed `prefixCommon` would turn green a
  test that no longer proves what it says.
- **Cost per render**: `Hints()` was already called on every render and now
  receives a 1-entry `map`. Negligible; `fmt` is kept off the hot path.
- **`leaf` with an empty project**: yields `"#7"`, which is correct but must be
  tested so nobody reads it as a bug.

## Work order (TDD)

1. **Tests that fail first**, in blocks, in this order:
   a. `refcol_test.go`: `refLeaf`, cycle of `next()`, width per mode, `prefixOf`
      empty outside `common` → then `refcol.go`.
   b. `list_test.go`: prefix line per mode, recovered height, degradation →
      then `refcol.go` + `list.go`.
   c. `section_test.go`: the `p` key (cycle, rebind, no reset with `tab`, no
      side effect) → then `update.go` + `app.go`.
   d. `config_test.go`: hint with state → then `config.go` + `sections.go`.
   e. `print_test.go`: independence of `--print` (green from the start: it is a
      guard, not a feature).
2. **Refactor of the 9 + 3 call sites** of `newRefLayout` to `prefixCommon`.
3. **ADR 0005** + `README` + `CHANGELOG`.
4. `make test` and smoke in tmux.

## Verifications

- `make test` (build + vet + gofmt + `go test -race ./...`). **It is the only
  guarantee: the repo has no CI configured.**
  _Correction (2026-10-04): CI does exist now — `.github/workflows/ci.yml`
  and `mutation.yml`; `make check` is its local equivalent._
- Manual smoke (tmux + `capture-pane`): `p` cycles the 3 modes; the hint names
  the mode; the column widens/narrows without flicker; in `full`/`leaf` the list
  gains one line and the cursor stays visible; `tab` does not change the mode;
  `--print` untouched.
- `prdash --print` with a subgroup path: full path, no prefix line.
- ADR coherence: 0002 → 0004 → 0005 chained by "Supersedes", with no orphan
  clauses.
