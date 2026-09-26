---
feature: 0002-inbox-single-section
freshness: 143fd59b9d8ff54d15b3cae8c00651f33424f034
codegraph: not_initialized
generated_by: codebase-researcher
---

# Context: Inbox de una sola sección con leyenda de conteos

## Scope
- In: sección activa única (default `review`) con `tab` ciclando `review → mentions → authored → review`; cursor/scroll por sección; leyenda de conteos en el borde superior izquierdo sustituyendo `Inbox`; línea fija de prefijo común de la activa; `(empty)`/`loading more…`/avisos solo de la activa.
- Out: `inbox.Build`, `--print`, formato de cache, API de `[keybindings]`, adapters; prefijo seleccionable/toggleable (solo se deja la costura).

## Files to Touch
| Symbol / Area | File | Lines | Why |
|---------------|------|-------|-----|
| `Section.String()` | internal/forge/model/model.go | 22-35 | Conservar; añadir `Legend()` justo después (`Mine`/`Assigned`/`Mentioned`). |
| `Section` consts | internal/forge/model/model.go | 10-20 | Mapeo de `Legend()` por kind. |
| `Model` struct | internal/tui/app.go | 181-263 | Añadir `activeSection` + estado cursor/scroll por sección; `cursor`/`scroll` (191-195) pasan a ser los de la activa. |
| `rows()` | internal/tui/app.go | 704-711 | Pasa a devolver solo los ítems de la sección activa. |
| `selected()` | internal/tui/app.go | 713-720 | Opera sobre la activa (vía `rows()`). |
| `clampCursor()` | internal/tui/app.go | 694-702 | Acota el cursor de la activa. |
| `refreshSelfDenied()` | internal/tui/app.go | 684-692 | Itera `rows()` → solo activa (veto approve propio). |
| `sectionItems()` | internal/tui/app.go | 722-725 | Sigue leyendo `inbox.Section(kind)`; base para helpers de activa. |
| `sectionLoadingMore()` | internal/tui/app.go | 488-496 | Reusar para la activa. |
| `sectionProblems()` | internal/tui/app.go | 727-741 | Reusar solo con la activa. |
| `rebuild()` | internal/tui/app.go | 621-629 | `clampCursor`/`syncScroll` sobre la activa. |
| `handleKey` | internal/tui/update.go | 165-224 | `end` (189-191) usa `rows()`; `case "section-next"` (200-203) pasa a ciclar sección activa. |
| `moveCursor` | internal/tui/update.go | 469-477 | Mueve cursor de la activa + `syncScroll`. |
| `goTop` | internal/tui/update.go | 479-486 | Resetea cursor/scroll de la activa. |
| `pageBy` | internal/tui/update.go | 488-497 | Página sobre la activa. |
| `gotoNextSection` | internal/tui/update.go | 509-524 | **Eliminar** (obsoleto). |
| `sectionOffsets` | internal/tui/update.go | 526-535 | **Eliminar** (obsoleto). |
| `sectionIndexAtCursor` | internal/tui/update.go | 537-548 | **Eliminar** (obsoleto). |
| `listLines()` | internal/tui/list.go | 25-63 | Reescribir: compone **solo la activa** (línea de prefijo opcional + avisos + header de columnas + filas, o `(empty)`, + `loading more…`); sin título de sección. |
| `newRefLayout(m.inbox.Sections)` call | internal/tui/list.go | 30 | Pasar solo la sección activa (una rebanada de 1). |
| `syncScroll` | internal/tui/list.go | 94-101 | Opera sobre las líneas de la activa. |
| `scrollFor` / `visibleList` | internal/tui/list.go | 79-89 / 107-113 | Sin cambios; se mantienen y prueban. |
| `newRefLayout` / `prefixOf` | internal/tui/refcol.go | 40-56 / 60-62 | Firma `newRefLayout([]inbox.Section)` **intacta**; ajustar solo comentarios (dicen "todo el inbox"/"entre secciones"). |
| `sectionPrefix` / `refSuffix` / `truncateTail` | internal/tui/refcol.go | 70-96 / 101-111 / 116-128 | Sin cambios de lógica. |
| `sectionLines` | internal/tui/sections.go | 64-74 | Ya admite título estilizado; es el punto por donde entra la leyenda. |
| `listSection` | internal/tui/sections.go | 111-131 | Reemplazar `"Inbox"` (línea 130) por la leyenda de conteos; usar scroll/cursor de la activa. |
| `itemCells` | internal/tui/table.go | 115-129 | Usa `l.prefixOf(sec)` (línea 122); sin cambio de lógica, el prefijo ahora sale de la activa. |
| `renderCell` / `pad()` | internal/tui/table.go | 132-167 / 362-369 | Patrón de celdas: texto plano + `pad()` ANTES de estilo. |
| estilos | internal/tui/styles.go | 41-97 | Añadir estilo de tramo activo de la leyenda (reutilizar `styleCount`/`styleDim`) y de la línea de prefijo (`styleDim`); `borderColor`/`styleBorder` para repintar el borde. |
| `RenderWithTitle(s)` / `borderLine` | internal/tui/bordered/bordered.go | 26-86 | **Sin cambios**; admite título ya estilizado y trunca ANSI-aware (`ansi.StringWidth`/`Truncate`, 63-67). |
| `DefaultKeybindings` / `hintOrder` | internal/config/config.go | 281-291 / 419-439 | **Sin cambios** (`section-next=tab`, label `section`). |
| `runPrint` | cmd/prdash/print.go | 47-64 | **Sin cambios**; usa `sec.Kind.String()` y `box.Sections` (no debe tocarse `String()`). |

## Contracts
- `inbox.Build(inputs) Inbox` → 3 `inbox.Section{Kind, Items}` en orden/autoridad `authored > review > mentions` — `internal/inbox/inbox.go:38-59`. El orden de `Sections` es el orden de la leyenda (`Mine · Assigned · Mentioned`).
- `Inbox.Section(kind) Section` — `internal/inbox/inbox.go:106-114`; `Inbox.Empty()` — `:116-124`.
- `model.Section` + `String()` (`Created by me`/`Review / assigned`/`Mentions`) — `internal/forge/model/model.go:22-35`. **No tocar** (lo usa `--print`). Añadir `Legend()` → `Mine`/`Assigned`/`Mentioned`.
- `newRefLayout([]inbox.Section) refLayout` — `internal/tui/refcol.go:40`; firma conservada (tests unitarios de `refcol` la usan).
- `refLayout.prefixOf(model.Section) string` — `internal/tui/refcol.go:60`; `sectionPrefix(items)` — `:70`; `refSuffix(it, prefix)` — `:101`.
- `bordered.RenderWithTitles(border, borderFg, topTitle, topAlign, bottomTitle, bottomAlign, content, width)` recibe el título **ya estilizado** — `internal/tui/bordered/bordered.go:34`.
- `syncScroll` — `internal/tui/list.go:94`; `scrollFor(current,target,total,view)` — `:79`; `visibleList(lines,scroll,view)` — `:107`.

## Pattern to Follow
- **Cajas / título de borde** → `internal/tui/sections.go:64-74` (`sectionLines`: envuelve `" "+title+" "` y llama `bordered.RenderWithTitle`) y `listSection` `:111-131` como plantilla de armado del cuerpo (visible + relleno a `lay.bodyLines`).
- **Celdas (texto, estilo) con `pad()` antes de estilo** → `internal/tui/table.go:132-167` (`renderCells`/`renderCell`) y `pad` `:362-369`.
- **Scroll que sigue al cursor** → `internal/tui/list.go:94-101` (`syncScroll` → `scrollFor`) llamado desde `internal/tui/update.go:469-497` (`moveCursor`/`pageBy`).
- **Tests de modelo directo enviando msgs** → `internal/tui/app_test.go:23-101` (`newTestModel`/`mkItem`/`page`/`send`/`press`).

## Tests
- Existing affected:
  - `internal/tui/list_test.go` — `TestViewBoxesEverySection` `:41-78` (espera caja `Inbox`, línea 54); `TestScrollFollowsCursor` `:128-160`; `TestPageKeysMoveOneWindow` `:164-197` (usa `len(m.rows())`).
  - `internal/tui/table_test.go` — `TestNavigationMovesCursor` `:150-175` (cursor cross-sección); `TestSectionNextHonorsRebind` `:177-204`; `TestColumnasSeparadasPorUnEspacio` `:74-107` (`newRefLayout(m.inbox.Sections)` línea 81); `TestDiffColumnAppearsOnlyOnWideTerminals` `:264-293` (línea 272).
  - `internal/tui/refcol_test.go` — integración `TestListLinesMuestranElPrefijoEnLaCabecera` `:157-189` y `TestListLinesSinPrefijoConservanLaRuta` `:191-230`; los unitarios de `newRefLayout`/`sectionPrefix`/`refSuffix` (`:30-155`, `:235-303`) se mantienen.
  - `internal/tui/app_test.go` — `TestPaginationIndicator` `:177-188` (usa `authored` con default `review` → deja de verse); `TestViewShowsThreeSectionsWithBothForges` `:121-143` (espera 3 títulos de sección); `TestSectionEmptyVsError` `:145-158` (espera 2 `(empty)`); usos de `m.sectionItems(...)` (199, 205, 258, 443, 510, 583, 592, 745).
  - `internal/forge/model/model_test.go` — añadir caso `Legend()`.
- Framework / runner: Go testing; runner del repo `make test` = `go build ./... && go vet ./... && gofmt -l . && go test -race ./...`.
- Integration infra: unit only (adapters falsos `testutil.FakeAdapter`; cache aislado con `XDG_CACHE_HOME` en `newTestModel` app_test.go:25).
- Suggested targets: sección activa/default/ciclo → `table_test.go`; leyenda + prefijo + `(empty)` + `loading more…` → `list_test.go`/`refcol_test.go`; `Legend()` → `model_test.go`; no-regresión `--print` → `cmd/prdash`.

## Conventions & Boundaries
- Comentarios en **español** y **sin** referencias a specs/IDs/scenarios; el código es la fuente de verdad.
- TUI no toca red ni disco; el pipeline (`streams → inbox.Build`) no se modifica, solo qué se pinta.
- Celdas devuelven `(texto, estilo)`; `pad()` ANTES de aplicar estilo (ANSI rompe el ancho).
- `--print` (usa `Section.String()` + `box.Sections`) y el formato de cache (`(forge,section,kind)`) no cambian.
- `section-next`/`tab` siguen saliendo de `[keybindings]`; la barra de hints (`hintOrder`) no cambia.

## Integration Points (non-obvious)
- **Título del borde con ANSI**: `sectionLines` (`sections.go:64-74`) ya envuelve el título con `" " + title + " "`, y `borderLine` (`bordered.go:59-86`) mide con `ansi.StringWidth` y reenvuelve cada segmento con el estilo del borde (un reset `\x1b[0m` de un tramo interior no restaura el color del borde → componer la leyenda con **tramos de estilos completos, sin caracteres "desnudos"**, y confiar en el truncado ANSI-aware del borde).
- **`newRefLayout` debe recibir SOLO la sección activa** (una rebanada de 1): si recibe `m.inbox.Sections` dimensiona ITEM por el sufijo más largo de todas las secciones y calcula prefijos de secciones que no se ven (`list.go:30`, `refcol.go:32-35`).
- **Orden de ciclo ≠ orden de leyenda**: leyenda itera `m.inbox.Sections` (`authored·review·mentions` → `Mine·Assigned·Mentioned`); el ciclo `review → mentions → authored → review` es `sectionOrder` +1 mód 3 (`inbox.go:40-44`), empezando en `review`.
- **`m.rows()` pasa a ser de la activa** y arrastra a `selected`, `clampCursor`, `refreshSelfDenied`, `end` (`update.go:189-191`) y `pageBy`; `Rebuild` debe re-acotar cursor/scroll de la activa al contenido nuevo.
- **Prefijo + paginación**: el prefijo común puede encogerse al llegar páginas y ensanchar ITEM **una vez** (comportamiento ya aceptado en ADR 0002). Sin prefijo común, cada celda ITEM lleva la ruta completa recortada por la cola (`truncateTail`).
- **Avisos**: solo de la activa (riesgo asumido). Una sección no activa con fallo solo muestra su conteo en la leyenda; al tabular a ella reaparece su aviso (`sectionProblems` ya filtra por `w.Section == kind`).
- **`(empty)` vs aviso**: no pintar `(empty)` si la activa tiene warnings (principio "nunca vacío si hubo error"); hoy `listLines:57` lo respeta (`case len(problems) == 0`).

## Risks / Assumptions
- **ANSI de la leyenda**: verificar a terminal estrecho que la línea superior mide exactamente `outerWidth` y que el color de relleno del borde no se pierde tras los resets de los tramos activos.
- **Tests acoplados**: el alcance de reescritura (list/table/refcol/app) es parte del trabajo, no efecto colateral; `TestViewShowsThreeSectionsWithBothForges` y `TestSectionEmptyVsError` también caen aunque no estén en el plan.
- **`syncScroll`/`scrollFor`** operan ahora sobre las líneas de la activa; asegurar que guardar/restaurar scroll por sección no deja el cursor fuera de la ventana.
- **`Legend()` vs `String()`**: mantener `String()` intacto es lo que protege `--print`; cualquier cambio ahí rompe el escenario de no-regresión.
- **Default `review`**: `TestPaginationIndicator` usa `authored`; con default `review` el indicador no se pinta hasta tabular — reescribir el test sobre la sección activa.
