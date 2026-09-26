---
feature: 0003-prefix-mode-selectable
freshness: 1943444
codegraph: not_initialized
generated_by: codebase-researcher
---

# Context: Prefijo de ruta seleccionable y toggleable

## Scope
- In: 3 modos de prefijo (`common`/`full`/`leaf`) ciclados con `p` (acción `prefix-mode`), globales y sin persistir; etiqueta de celda ITEM y ancho de columna según modo; línea de prefijo solo en `common`; hint que nombra el modo actual.
- Out: persistencia (`fileConfig`/schema TOML), `--print`, `inbox.Build` y la autoridad de secciones, ciclo de `tab` y cursor/scroll por sección, `sectionPrefix`, detalle, leyenda, pipeline de datos.

## Files to Touch
| Symbol / Area | File | Lines | Why |
|---------------|------|-------|-----|
| `refLayout` | internal/tui/refcol.go | 27-30 | Añadir `mode prefixMode`: la celda lo necesita para etiquetar. |
| `newRefLayout` | internal/tui/refcol.go | 42-58 | Nueva firma `(sections, mode)`. El prefijo se calcula **solo** en `common`; en los otros dos queda `""`. El ancho mide la etiqueta del modo. |
| `prefixOf` | internal/tui/refcol.go | 62-64 | **Sin cambio**: ya devuelve `""` fuera de `common` y `list.go:46` ya lo comprueba. |
| `sectionPrefix` | internal/tui/refcol.go | 72-98 | **Sin cambios**: el cálculo del prefijo común no varía. |
| `refSuffix` | internal/tui/refcol.go | 103-113 | **Sin cambios**: pasa a ser la etiqueta de `common` y `full`; con prefijo vacío ya devuelve la referencia completa (degradación). |
| `truncateTail` | internal/tui/refcol.go | 118-130 | **Sin cambios**: el recorte por la cola vale en los tres modos. |
| `refCellText` / `refLeaf` | internal/tui/refcol.go | nuevo | Etiqueta de la celda por modo; `refLeaf` = hoja del proyecto + `#n`. |
| `prefixMode` (+`String`/`next`) | internal/tui/refcol.go | nuevo | Tipo de 3 valores; `String()` es lo que va al hint, `next()` el ciclo. |
| `listLines` | internal/tui/list.go | 32-70 | **Una línea**: pasar `m.prefixMode` a `newRefLayout` (40). El `if prefix != ""` (46) ya resuelve "no hay línea en `full`/`leaf`". |
| `itemCells` | internal/tui/table.go | 115-129 | Línea 122: `refSuffix(it, l.prefixOf(sec))` → `refCellText(it, l.mode, l.prefixOf(sec))`. |
| `handleKey` | internal/tui/update.go | 178-247 | Nuevo `case "prefix-mode"` en el `switch` de `ActionForKey` (221-245). |
| `cyclePrefixMode` | internal/tui/update.go | nuevo | Ciclo + `syncScroll()` (el modo quita una línea en `full`/`leaf`). |
| `Model` | internal/tui/app.go | 190-259 | Añadir `prefixMode`, default `prefixCommon`. |
| `HintState` / `Hints` | internal/config/config.go | 446-459 | `Hints(state HintState)`: une `label + ": " + state[action]`. `HintState = map[string]string` por acción. |
| `hint` | internal/config/config.go | 410-418 | **Sin cambio de campos**; `label` pasa a ser el valor por defecto. |
| `hintOrder` | internal/config/config.go | 430-441 | Añadir `{action: "prefix-mode", label: "prefix"}` **después de `refresh`**. |
| `DefaultKeybindings` | internal/config/config.go | 282-293 | Añadir `"prefix-mode": "p"`. |
| `hintLines` | internal/tui/sections.go | 209-214 | Línea 213: `m.cfg.Hints(m.dynamicHints())`. |
| `dynamicHints` | internal/tui/sections.go | nuevo | `config.HintState{"prefix-mode": m.prefixMode.String()}`. |
| `runPrint` | cmd/prdash/print.go | 31-70 | **Sin cambios** (verificado: no usa `newRefLayout`/`refSuffix`/`sectionPrefix`). Guard con test nuevo. |
| `TestHints` | internal/config/config_test.go | 327-336 | Lista `want` literal: añadir `p prefix: common` y el parámetro. |

## Contracts
- `newRefLayout(sections []inbox.Section, mode prefixMode) refLayout` — `internal/tui/refcol.go:42`. **Firma con modo explícito**: el compilador obliga a cada llamador a nombrarlo (10 sitios: 1 producción + 9 tests).
- `refLayout.prefixOf(model.Section) string` — `:62`. Vacío en `full`/`leaf` y cuando no hay prefijo común.
- `refCellText(it model.Item, mode prefixMode, prefix string) string` — nuevo. `leaf` → `refLeaf`; `common`/`full` → `refSuffix`.
- `refLeaf(it model.Item) string` — nuevo. `strings.LastIndex(project, "/")`; con proyecto vacío → `"#n"`.
- `prefixMode.String()` → `common`|`full`|`leaf`; `prefixMode.next()` → `(p+1)%3`.
- `Config.Hints(state HintState) []string` — `internal/config/config.go:446`. `nil`/vacío = barra actual sin estado dinámico.
- `Config.KeyFor(action)` / `ActionForKey(key)` — `:333` / `:342`: el rebind de `prefix-mode` es genérico, sin código nuevo.
- `Config.ActionForKey` desempata recorriendo acciones **ordenadas** y devuelve la primera que coincide: `prefix-mode` no colisiona con ninguna otra tecla, así que no hay ambigüedad.
- `Config.Hints` es la **única** fuente de la barra; `hintLines()` (`sections.go:209`) solo la envuelve al ancho. Con `mergeArmed` la sustituye la Confirmación.
- `TestHintsCubrenTodosLosKeybindings` (`config_test.go:342`) exige que la tecla de **toda** acción de `DefaultKeybindings` aparezca en la barra: registrar `prefix-mode` sin meterlo en `hintOrder` rompe el test.
- `syncScroll()` — `internal/tui/list.go:101`; `scrollFor(current,target,total,view)` — `:86`; `visibleList` — `:114`.

## Pattern to Follow
- **Estado de vista en `Model` + efecto en `Update`**: `activeSection`/`cycleSection` (`app.go`, `update.go`) son el molde de "un campo global que cicla con una tecla y se pasa al render".
- **Layout calculado una vez por render**: `newRefLayout` en `listLines` (`list.go:40`), no por fila. El requisito de "sin parpadeo" es este mismo invariante.
- **Acción configurable**: registrar en `DefaultKeybindings` + `hintOrder` y despachar por `m.cfg.ActionForKey(key)` (`update.go:221`). Cero código de tecla cableada.
- **Celdas texto+estilo con `pad()` antes del estilo** → `internal/tui/table.go:132-167`.
- **Tests de modelo directo enviando msgs** → `internal/tui/app_test.go:23-101` (`newTestModel`/`mkItem`/`page`/`send`/`press`).
- **Tests dirigidos de tabla para render** → `internal/tui/list_test.go` (`visibleLines`/`stripANSI`).

## Tests
- Existing affected:
  - `internal/config/config_test.go` — `TestHints` `:327` (lista `want` literal, hay que añadir `p prefix: …` y el parámetro `HintState`); `TestHintsCubrenTodosLosKeybindings` `:342` (debe seguir verde sin cambios); `TestHintsSiguenElRebind` `:353` (actualizar la llamada); `TestDefaultKeybindingsCoverActions` `:366` (lista explícita de acciones: añadir `prefix-mode`).
  - `internal/tui/refcol_test.go` — llamadas a `newRefLayout` en `:100`, `:114`, `:125`, `:139`, `:280` (+`prefixCommon`); la ronda aleatoria de `:280` pasa a iterar también los otros dos modos.
  - `internal/tui/table_test.go` — llamadas en `:28`, `:45`, `:81`, `:295` (+`prefixCommon`).
  - `internal/tui/list_test.go` / `section_test.go` — los de la línea de prefijo y del ancho de la activa **siguen valiendo sin cambios** (el default es `common`): son el guard de "esta feature no cambia nada por defecto".
- Nuevos:
  - `refcol_test.go`: `refLeaf` (con subgrupo, sin subgrupo, proyecto vacío, proyecto de un segmento), ciclo `next()` ×3 + vuelta, `prefixOf` vacío en `full`/`leaf`, ancho por modo (24/34/16 en el fixture de `behavior.feature`, y acotado a `[6,34]` en los tres).
  - `list_test.go`: línea de prefijo presente en `common` / ausente en `full` y `leaf`; `full` y `leaf` recuperan una línea (`len(listLines)`); `common` sin prefijo común se ve **exactamente** como `full`.
  - `section_test.go`: `p` cicla los 3 modos y vuelve; rebind de `prefix-mode` desactiva `p`; `tab` no reinicia el modo ni el cursor/scroll por sección; `p` no aprueba, no mergea, no monta, no refresca, no sale y no arma el merge.
  - `config_test.go`: `TestHintsConEstadoDinamico` — sin estado sale `p prefix`; con estado sale `p prefix: full`; con rebind sale `P prefix: leaf`.
  - `section_test.go` o `app_test.go`: el hint de la barra nombra el modo y cambia al ciclar (extremo a extremo, con `mergeArmed` la Confirmación sigue sustituyendo la barra).
  - `cmd/prdash/print_test.go`: `TestRunPrintNoAplicaElModoPrefijo` — ruta de subgrupo larga → sale **entera** en `--print`, sin `…` y sin línea de prefijo.
- Framework / runner: Go testing; `make test` = `go build ./... && go vet ./... && gofmt -l . && go test -race ./...`.
- Integration infra: unit only (`testutil.FakeAdapter`, cache aislado con `XDG_CACHE_HOME` en `newTestModel`).

## Conventions & Boundaries
- Comentarios en **español**, sin referencias a specs/IDs/escenarios; el código es la fuente de verdad.
- TUI no toca red ni disco; el pipeline (`streams → inbox.Build`) no se modifica.
- Nada nuevo en `fileConfig`: el modo **no** es persistente (decisión del usuario).
- `--print` usa `Section.String()` + `it.Ref.Project` crudo y **no** debe cambiar.
- La TUI no puede importar nada de `config` que no exista ya, y `config` **no** puede importar `tui` (ciclo: `tui/app.go` ya importa `config`).

## Integration Points (non-obvious)
- **La invariante "sin línea de prefijo" no necesita `if` nuevo**: `list.go:46` ya compara `prefix != ""`, y en `full`/`leaf` el prefijo es `""` por construcción (`newRefLayout`). Por eso `listLines` solo cambia en la llamada a `newRefLayout`. Si alguien "optimiza" `prefixOf` para que devuelva el prefijo común siempre, la línea reaparece en `full` sin que ninguna otra parte avise.
- **La degradación `common` → `full` es el mecanismo ya existente**, no un caso especial: sin prefijo común, `sectionPrefix` devuelve `""` y `refSuffix` devuelve la referencia completa. Los tests existentes de "sección sin prefijo" (`refcol_test.go`) ya la cubren en `common`.
- **`refLayout` indexa el prefijo por `model.Section`**: dos secciones con el mismo `Kind` se pisarían. Hoy es imposible (`inbox.Build` garantiza una por kind) y sigue siéndolo — `list.go:40` sigue pasando una rebanada de 1.
- **`syncScroll` es necesario aunque `p` no mueva el cursor**: `full`/`leaf` acortan `listLines` en una línea, así que un `scroll` alto puede dejar el cursor fuera de la ventana. Mismo motivo que `goTop` (`update.go:506`).
- **Orden en `hintOrder` = prioridad de supervivencia**: el recorte a `maxHintLines` (3) corta **por la cola** (`config.go:426-429`, `wrapHint` en `sections.go:218`). `p` detrás de `refresh` sobrevive a terminales estrechos; al final de la lista desaparecería primero.
- **La etiqueta dinámica rompe la pureza de `Config.Hints()`**: es el precio consciente de nombrar el modo actual. Se paga con un parámetro explícito (`HintState`), no con un placeholder en el string, para que `hintOrder` siga siendo la fuente de lista, orden y etiqueta por defecto, y para que el rebind siga siendo genérico.
- **`ActionForKey` desempata por orden alfabético de acciones**: si algún día `prefix-mode` compartiera tecla con otra acción, ganaría la alfabéticamente anterior. Hoy no comparte.
- **Modo global + `tab`**: `tab` cambia `activeSection` y su prefijo común, pero **no** el modo. El hint nombra un modo que no aplica igual a las tres secciones; es coherente con que solo se pinte una, y es lo pedido.

## Risks / Assumptions
- **Firma de `newRefLayout`**: 10 call sites. Todos explícitos a propósito (que el compilador los obligue), pero un `prefixCommon` mal puesto en un test haría verde una prueba que ya no prueba lo que dice.
- **Ronda aleatoria de `refcol_test.go` (`:280`)**: hoy itera invariantes de `common`; si se extiende a los tres modos, `truncateTail` debe seguir garantizando cola intacta **en los tres** (en `leaf` la cola es `#n`, en `full` la hoja + `#n`).
- **`p` durante el merge armado**: `handleMergeArmed` consume la pulsación y cancela (`update.go:306-309`); `p` no es un modo de merge, así que desarma sin mergear. Es el comportamiento correcto y no se toca.
- **Overlay de simulación abierto**: `handleSimKey` captura el teclado entero; `p` cierra el overlay como cualquier tecla no-opción. No se toca.
- **ADR**: el 0005 sustituye la cláusula del 0002 (prefijo anclado en cabecera) **y** la del 0004 §4 (línea siempre visible). `sectionPrefix`, la fórmula del ancho acotado y el recorte por la cola siguen vigentes: no se reescriben ADRs aceptados, se encadena.
