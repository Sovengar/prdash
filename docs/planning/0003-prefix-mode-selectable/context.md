---
feature: 0003-prefix-mode-selectable
freshness: 1943444
codegraph: not_initialized
generated_by: codebase-researcher
---

# Context: Selectable and toggleable path prefix

## Scope
- In: 3 prefix modes (`common`/`full`/`leaf`) cycled with `p` (action `prefix-mode`), global and not persisted; ITEM cell label and column width per mode; prefix line only in `common`; hint naming the current mode.
- Out: persistence (`fileConfig`/TOML schema), `--print`, `inbox.Build` and section authority, `tab` cycle and per-section cursor/scroll, `sectionPrefix`, detail, legend, data pipeline.

## Files to Touch
| Symbol / Area | File | Lines | Why |
|---------------|------|-------|-----|
| `refLayout` | internal/tui/refcol.go | 27-30 | Add `mode prefixMode`: the cell needs it for its label. |
| `newRefLayout` | internal/tui/refcol.go | 42-58 | New signature `(sections, mode)`. The prefix is computed **only** in `common`; in the other two it stays `""`. The width measures the mode label. |
| `prefixOf` | internal/tui/refcol.go | 62-64 | **No change**: it already returns `""` outside `common` and `list.go:46` already checks it. |
| `sectionPrefix` | internal/tui/refcol.go | 72-98 | **No changes**: the common-prefix computation does not vary. |
| `refSuffix` | internal/tui/refcol.go | 103-113 | **No changes**: it becomes the label of `common` and `full`; with an empty prefix it already returns the full reference (degradation). |
| `truncateTail` | internal/tui/refcol.go | 118-130 | **No changes**: tail truncation holds in all three modes. |
| `refCellText` / `refLeaf` | internal/tui/refcol.go | new | Cell label per mode; `refLeaf` = project leaf + `#n`. |
| `prefixMode` (+`String`/`next`) | internal/tui/refcol.go | new | 3-value type; `String()` is what goes to the hint, `next()` the cycle. |
| `listLines` | internal/tui/list.go | 32-70 | **One line**: pass `m.prefixMode` to `newRefLayout` (40). The `if prefix != ""` (46) already resolves "no line in `full`/`leaf`". |
| `itemCells` | internal/tui/table.go | 115-129 | Line 122: `refSuffix(it, l.prefixOf(sec))` → `refCellText(it, l.mode, l.prefixOf(sec))`. |
| `handleKey` | internal/tui/update.go | 178-247 | New `case "prefix-mode"` in the `switch` of `ActionForKey` (221-245). |
| `cyclePrefixMode` | internal/tui/update.go | new | Cycle + `syncScroll()` (the mode removes one line in `full`/`leaf`). |
| `Model` | internal/tui/app.go | 190-259 | Add `prefixMode`, default `prefixCommon`. |
| `HintState` / `Hints` | internal/config/config.go | 446-459 | `Hints(state HintState)`: joins `label + ": " + state[action]`. `HintState = map[string]string` per action. |
| `hint` | internal/config/config.go | 410-418 | **No field change**; `label` becomes the default value. |
| `hintOrder` | internal/config/config.go | 430-441 | Add `{action: "prefix-mode", label: "prefix"}` **after `refresh`**. |
| `DefaultKeybindings` | internal/config/config.go | 282-293 | Add `"prefix-mode": "p"`. |
| `hintLines` | internal/tui/sections.go | 209-214 | Line 213: `m.cfg.Hints(m.dynamicHints())`. |
| `dynamicHints` | internal/tui/sections.go | new | `config.HintState{"prefix-mode": m.prefixMode.String()}`. |
| `runPrint` | cmd/prdash/print.go | 31-70 | **No changes** (verified: it does not use `newRefLayout`/`refSuffix`/`sectionPrefix`). Guard with a new test. |
| `TestHints` | internal/config/config_test.go | 327-336 | Literal `want` list: add `p prefix: common` and the parameter. |

## Contracts
- `newRefLayout(sections []inbox.Section, mode prefixMode) refLayout` — `internal/tui/refcol.go:42`. **Signature with explicit mode**: the compiler forces every caller to name it (10 sites: 1 production + 9 tests).
- `refLayout.prefixOf(model.Section) string` — `:62`. Empty in `full`/`leaf` and when there is no common prefix.
- `refCellText(it model.Item, mode prefixMode, prefix string) string` — new. `leaf` → `refLeaf`; `common`/`full` → `refSuffix`.
- `refLeaf(it model.Item) string` — new. `strings.LastIndex(project, "/")`; with an empty project → `"#n"`.
- `prefixMode.String()` → `common`|`full`|`leaf`; `prefixMode.next()` → `(p+1)%3`.
- `Config.Hints(state HintState) []string` — `internal/config/config.go:446`. `nil`/empty = current bar without dynamic state.
- `Config.KeyFor(action)` / `ActionForKey(key)` — `:333` / `:342`: the `prefix-mode` rebind is generic, no new code.
- `Config.ActionForKey` tie-breaks by walking **ordered** actions and returns the first match: `prefix-mode` does not collide with any other key, so there is no ambiguity.
- `Config.Hints` is the **only** source of the bar; `hintLines()` (`sections.go:209`) only wraps it to the width. With `mergeArmed` the Confirmation replaces it.
- `TestHintsCoverAllKeybindings` (`config_test.go:342`) requires that the key of **every** action of `DefaultKeybindings` appear in the bar: registering `prefix-mode` without putting it in `hintOrder` breaks the test.
- `syncScroll()` — `internal/tui/list.go:101`; `scrollFor(current,target,total,view)` — `:86`; `visibleList` — `:114`.

## Pattern to Follow
- **View state in `Model` + effect in `Update`**: `activeSection`/`cycleSection` (`app.go`, `update.go`) are the template of "a global field that cycles with a key and is passed to the render".
- **Layout computed once per render**: `newRefLayout` in `listLines` (`list.go:40`), not per row. The "no flicker" requirement is this very invariant.
- **Configurable action**: register in `DefaultKeybindings` + `hintOrder` and dispatch through `m.cfg.ActionForKey(key)` (`update.go:221`). Zero hardcoded-key code.
- **Text+style cells with `pad()` before the style** → `internal/tui/table.go:132-167`.
- **Direct model tests sending msgs** → `internal/tui/app_test.go:23-101` (`newTestModel`/`mkItem`/`page`/`send`/`press`).
- **Table-driven targeted tests for render** → `internal/tui/list_test.go` (`visibleLines`/`stripANSI`).

## Tests
- Existing affected:
  - `internal/config/config_test.go` — `TestHints` `:327` (literal `want` list, need to add `p prefix: …` and the `HintState` parameter); `TestHintsCoverAllKeybindings` `:342` (must stay green unchanged); `TestHintsFollowTheRebind` `:353` (update the call); `TestDefaultKeybindingsCoverActions` `:366` (explicit action list: add `prefix-mode`).
  - `internal/tui/refcol_test.go` — calls to `newRefLayout` at `:100`, `:114`, `:125`, `:139`, `:280` (+`prefixCommon`); the random round at `:280` now iterates the other two modes as well.
  - `internal/tui/table_test.go` — calls at `:28`, `:45`, `:81`, `:295` (+`prefixCommon`).
  - `internal/tui/list_test.go` / `section_test.go` — the prefix-line and active-width ones **still hold unchanged** (the default is `common`): they are the guard of "this feature changes nothing by default".
- New:
  - `refcol_test.go`: `refLeaf` (with subgroup, without subgroup, empty project, single-segment project), `next()` cycle ×3 and back, `prefixOf` empty in `full`/`leaf`, width per mode (24/34/16 on the `behavior.feature` fixture, and bounded to `[6,34]` in the three of them).
  - `list_test.go`: prefix line present in `common` / absent in `full` and `leaf`; `full` and `leaf` get back one line (`len(listLines)`); `common` without a common prefix looks **exactly** like `full`.
  - `section_test.go`: `p` cycles the 3 modes and back; rebind of `prefix-mode` deactivates `p`; `tab` does not reset the mode nor the per-section cursor/scroll; `p` does not approve, does not merge, does not mount, does not refresh, does not quit and does not arm the merge.
  - `config_test.go`: `TestHintsWithDynamicState` — without state it prints `p prefix`; with state it prints `p prefix: full`; with rebind it prints `P prefix: leaf`.
  - `section_test.go` or `app_test.go`: the bar hint names the mode and changes when cycling (end to end, with `mergeArmed` the Confirmation keeps replacing the bar).
  - `cmd/prdash/print_test.go`: `TestRunPrintNoAplicaElModoPrefijo` — long subgroup path → comes out **whole** in `--print`, with no `…` and no prefix line.
- Framework / runner: Go testing; `make test` = `go build ./... && go vet ./... && gofmt -l . && go test -race ./...`.
- Integration infra: unit only (`testutil.FakeAdapter`, cache isolated with `XDG_CACHE_HOME` in `newTestModel`).

## Conventions & Boundaries
- Comments in **Spanish**, no references to specs/IDs/scenarios; the code is the source of truth.
- The TUI touches neither network nor disk; the pipeline (`streams → inbox.Build`) is not modified.
- Nothing new in `fileConfig`: the mode is **not** persistent (user decision).
- `--print` uses `Section.String()` + raw `it.Ref.Project` and **must not** change.
- The TUI cannot import anything from `config` that does not already exist, and `config` **cannot** import `tui` (cycle: `tui/app.go` already imports `config`).

## Integration Points (non-obvious)
- **The "no prefix line" invariant needs no new `if`**: `list.go:46` already compares `prefix != ""`, and in `full`/`leaf` the prefix is `""` by construction (`newRefLayout`). That is why `listLines` changes only in the call to `newRefLayout`. If someone "optimizes" `prefixOf` to always return the common prefix, the line reappears in `full` without any other part warning.
- **The `common` → `full` degradation is the already existing mechanism**, not a special case: with no common prefix, `sectionPrefix` returns `""` and `refSuffix` returns the full reference. The existing tests of "section without prefix" (`refcol_test.go`) already cover it in `common`.
- **`refLayout` indexes the prefix by `model.Section`**: two sections with the same `Kind` would overwrite each other. Today it is impossible (`inbox.Build` guarantees one per kind) and it stays that way — `list.go:40` still passes a slice of 1.
- **`syncScroll` is needed even though `p` does not move the cursor**: `full`/`leaf` shorten `listLines` by one line, so a high `scroll` can leave the cursor outside the window. Same reason as `goTop` (`update.go:506`).
- **Order in `hintOrder` = survival priority**: the trim to `maxHintLines` (3) cuts **from the tail** (`config.go:426-429`, `wrapHint` in `sections.go:218`). `p` behind `refresh` survives narrow terminals; at the end of the list it would disappear first.
- **The dynamic label breaks the purity of `Config.Hints()`**: it is the conscious price of naming the current mode. It is paid with an explicit parameter (`HintState`), not with a placeholder in the string, so that `hintOrder` keeps being the source of list, order and default label, and so the rebind keeps being generic.
- **`ActionForKey` tie-breaks by alphabetical order of actions**: if someday `prefix-mode` shared a key with another action, the alphabetically earlier one would win. Today it shares none.
- **Global mode + `tab`**: `tab` changes `activeSection` and its common prefix, but **not** the mode. The hint names a mode that does not apply equally to the three sections; that is coherent with only one being painted, and it is what was asked for.

## Risks / Assumptions
- **Signature of `newRefLayout`**: 10 call sites. All explicit on purpose (so the compiler forces them), but a wrongly placed `prefixCommon` in a test would turn green a test that no longer proves what it says.
- **Random round of `refcol_test.go` (`:280`)**: today it iterates `common` invariants; if it is extended to the three modes, `truncateTail` must keep guaranteeing an intact tail **in the three** (in `leaf` the tail is `#n`, in `full` the leaf + `#n`).
- **`p` during an armed merge**: `handleMergeArmed` consumes the keystroke and cancels (`update.go:306-309`); `p` is not a merge mode, so it disarms without merging. That is the correct behavior and it is not touched.
- **Simulation overlay open**: `handleSimKey` captures the whole keyboard; `p` closes the overlay like any key that is not an option. Not touched.
- **ADR**: the 0005 supersedes the clause of the 0002 (prefix anchored in a header) **and** the one of the 0004 §4 (always-visible line). `sectionPrefix`, the bounded width formula and the tail truncation stay in force: accepted ADRs are not rewritten, the chain is extended.
