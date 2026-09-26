# 0003 — Prefijo de ruta seleccionable y toggleable — Plan

adr_required: true
adr_reason: la feature convierte el prefijo de ruta común (Accepted, ADR 0002 §Decisión 1, reubicado por el ADR 0004 §Decisión 4) en un modo **elegible** entre tres, lo que cambia qué se ve por defecto en la tabla y cuál es el ancho de ITEM. Es una decisión de diseño con alternativas reales (qué tres modos, si el prefijo se persiste, dónde vive la etiqueta del hint), no un refactor.
adr_title: adr-0005-selectable-prefix-mode
adr_path: docs/adr/0005-selectable-prefix-mode.md
adr_note: el ADR 0005 supersede la cláusula del ADR 0002 que ancla el prefijo en una cabecera (y la del ADR 0004 que lo fija en una línea siempre visible): pasa a ser el modo por defecto `common` de un modo seleccionable. El cálculo del prefijo (alineado en `/`, sin comerse la hoja), la fórmula del ancho acotado a `[6, 34]` y el recorte por la cola siguen vigentes tal cual.

## Resultado esperado

La columna ITEM deja de tener una única forma de mostrar la ruta. Una tecla **`p`**
(acción `prefix-mode`) cicla **tres modos globales**:

| Modo | Línea de prefijo | Celda ITEM | Ancho de ITEM (fixture de `behavior.feature`) |
|---|---|---|---|
| `common` (default) | sí, con el prefijo común de la activa | solo el sufijo | 24 |
| `full` | **no** | la referencia **completa** | 34 (topado) |
| `leaf` | **no** | la **hoja** del proyecto + `#n` | 16 |

El ancho de ITEM **se recalcula por modo** con el mismo acotado `[6, 34]`, una
sola vez por render. En `full` y `leaf` la lista **recupera la línea** que ocupaba
el prefijo. Si la sección activa no tiene prefijo común, `common` **degrada a
`full`** sin inventar nada. El hint de `p` nombra el **modo actual**
(`p prefix: full`). El modo **no se persiste** y `--print` no cambia.

## Alcance

- **In**: tipo `prefixMode` + ciclo; `prefixMode` en `Model` (default `common`); la
  etiqueta de la celda ITEM según modo; ancho de ITEM por modo; la línea de
  prefijo solo en `common`; acción `prefix-mode` (`p`) en `DefaultKeybindings` +
  `hintOrder`; hint con el modo actual; `syncScroll` al ciclar.
- **Out**: persistencia (nada en `fileConfig` ni en el schema TOML); `--print`;
  `inbox.Build` y la autoridad/dedupe de secciones; el ciclo de `tab` y el
  cursor/scroll por sección; `sectionPrefix` (el cálculo del prefijo común no
  cambia); el detalle; la leyenda; el pipeline de datos.

## Enfoque (alto nivel)

El prefijo **ya está compuesto como una unidad discreta desde una única fuente**
(`refLayout`), que es exactamente la costura que dejó el ADR 0004. La feature no
toca el pipeline de datos ni la maquetación: solo decide **qué etiqueta pone la
celda ITEM** y **a qué ancho se dimensiona la columna**, según un entero de
tres valores que vive en el `Model`.

El punto clave del diseño: `refLayout` ya calcula el prefijo por sección y
`listLines` **ya** solo pinta la línea de prefijo si sale no vacío. Así que en
`full` y `leaf` basta con que el layout **no** declare prefijo (vacío) y la línea
desaparece sola, sin tocar `listLines`. La degradación de `common` a `full` es el
mismo mecanismo: sin prefijo común, el prefijo ya sale vacío hoy.

## Decisiones clave

1. **Un tipo `prefixMode` de tres valores en `internal/tui`, no en `config`.**
   Es estado de vista puro: `config` no debe conocerlo (y no puede, sin ciclo de
   importación). Vive en `refcol.go`, que ya es dueño de "qué parte de la
   referencia se muestra".
   - `String()` → `common`/`full`/`leaf` (es lo que va al hint).
   - `next()` → `(p+1) % 3`: el ciclo es total y no necesita tabla, con lo que no
     puede desincronizarse del hint (que deriva del mismo `String()`).

2. **`newRefLayout` recibe el modo explícitamente** —
   `newRefLayout(sections []inbox.Section, mode prefixMode)`.
   - **Por qué no un default**: 9 llamadas de test y 1 de producción. A cambio, el
     **compilador obliga a cada punto a nombrar un modo**: no existe forma de
     pintar un layout en el modo que no sea el que se cree. Una firma con default
     dejaría `newRefLayout(secs)` significando "common" en unos sitios y
     "lo que hubiera" en otros.
   - Los 9 tests existentes pasan a `prefixCommon` explícito, y eso **convierte la
     suite actual en el guard de "el default es common y no cambia nada"**, que
     es justo el primer criterio de aceptación.

3. **Una sola función de etiqueta por modo**: `refCellText(it, mode, prefix)`.
   - `prefixLeaf` → `refLeaf(it)` (hoja + `#n`).
   - `prefixCommon`/`prefixFull` → `refSuffix(it, prefix)`, que **ya** degrada a
     la referencia completa cuando el prefijo es vacío.
   - El nombre nuevo es explícito porque `refSuffix` pasa a no ser "la" etiqueta
     de la celda: solo es la de dos de los tres modos.

4. **El prefijo solo se calcula en `common`.** En `full` y `leaf` el mapa de
   prefijos queda con `""` para la sección, y `listLines:46` (que ya compara con
   `""`) no pinta la línea. **Cero cambios en `listLines` aparte de pasar el
   modo**, y la invariante "en `full`/`leaf` no hay línea de prefijo" no depende
   de un `if` nuevo que pueda olvidarse.

5. **`refLeaf(it)`** corta el proyecto por el **último** `/` y añade `#n`.
   `strings.LastIndex` sobre `""` devuelve `-1`, así que un proyecto vacío da
   `"#7"` — que es justo `refLabel` sin proyecto, y no rompe el ancho (ya hay
   test de proyecto vacío).

6. **`syncScroll()` al ciclar.** `p` no mueve el cursor, pero `full` y `leaf`
   **quitan una línea** de `listLines`. Sin re-sincronizar, un `scroll` alto
   dejaría el cursor fuera de la ventana justo en el momento de cambiar de modo.
   Es el mismo motivo por el que `goTop` resetea `scroll` a mano.

7. **Acción `prefix-mode` en `DefaultKeybindings` + `hintOrder`.** Default `p`,
   libre (verificado contra `q`,`ctrl+c`,`R`,`r`,`a`,`m`,`v`,`tab`,`o`,`j`/`k`,
   `up`/`down`,`home`/`end`,`pgup`/`pgdown` y las de modo de merge `m`/`r`/`s`).
   - `ActionForKey` resuelve por acción, así que el rebind de `[keybindings]` es
     **gratuito y genérico**: al registrar la acción, `p` pasa a ser configurable
     por TOML sin tocar una línea más. Es la consecuencia aceptada de la decisión
     "sin persistencia": la **tecla** sí es configurable, el **modo** no.
   - Orden en la barra: **después de `refresh`**, antes de las teclas fijas
     `j/k` y `pgup/dn`. El recorte a `maxHintLines` tira **por la cola**, así que
     al final desaparecería primero justo en terminal estrecho, que es donde más
     hace falta saber en qué modo se está.

8. **Hint dinámico por opción A: `Config.Hints(state HintState)`.**
   - `HintState` = `map[string]string` indexado **por acción**, cuyo valor es el
     fragmento que se une a la etiqueta con `": "`. `hintOrder` guarda el
     prefijo de la etiqueta (`"prefix"`), y la TUI pone el valor
     (`"full"`) → `p prefix: full`.
   - **Por qué no un marcador `%s` + `strings.Replace`**: la barra vista desde la
     config mostraría un `%s` crudo, y "la etiqueta la pone la TUI" difumina el
     invariante de que `hintOrder` es la fuente única de la barra.
   - **Por qué no lo compone la TUI entera** (como `mergeConfirmText`): sacaría el
     hint de `p` del orden de `hintOrder` y de `Config.Hints()`, que es
     precisamente lo que verifica `TestHintsCubrenTodosLosKeybindings`.
   - Firma con **parámetro explícito** (no variádico) para que ningún llamador
     olvide que la barra tiene una dependencia de estado.

9. **ADR 0005, no editar el 0002.** Los ADR son registro: el 0005 declara qué
   cláusula sustituye y de qué ADR, como hizo el 0004 con el 0002. Así la cadena
   queda legible (0002 → 0004 → 0005) sin reescribir historia. En el 0005 se
   corrige además la **justificación** de `leaf`: no desambigua (para
   `acme/one#7` y `other/one#8` pintaría `one#7` y `one#8`); su valor es la
   densidad cuando las hojas **sí** son únicas.

## Ficheros afectados

**Producción**

- `internal/tui/refcol.go`: tipo `prefixMode` (+`String`, `next`), `refLeaf`,
  `refCellText`; `refLayout` gana `mode`; `newRefLayout` calcula el prefijo solo
  en `common` y mide con la etiqueta del modo. `sectionPrefix`, `refSuffix` y
  `truncateTail` **sin cambios de lógica**.
- `internal/tui/app.go`: `prefixMode` en `Model` (default `prefixCommon`).
- `internal/tui/list.go`: pasar `m.prefixMode` a `newRefLayout` (única línea).
- `internal/tui/table.go:122`: `refCellText(it, l.mode, l.prefixOf(sec))`.
- `internal/tui/update.go`: `case "prefix-mode"` en el `switch` de
  `ActionForKey` + `cyclePrefixMode()` (ciclo + `syncScroll`).
- `internal/config/config.go`: `HintState` + `Hints(state)`; `"prefix-mode": "p"`
  en `DefaultKeybindings`; `{action: "prefix-mode", label: "prefix"}` en
  `hintOrder` detrás de `refresh`.
- `internal/tui/sections.go:213`: `m.cfg.Hints(m.dynamicHints())` +
  `dynamicHints()` (devuelve el modo actual).
- `docs/adr/0005-selectable-prefix-mode.md`: nuevo.
- `README.md`: `p` en la lista de teclas + párrafo de los 3 modos.
- `CHANGELOG.md`: entrada en `## [Unreleased] → ### Added`.

**Tests afectados / nuevos**

- `internal/config/config_test.go`: `TestHints` (lista `want` literal + estado),
  `TestHintsCubrenTodosLosKeybindings` (sigue verde sola), `TestHintsSiguenElRebind`
  (con `HintState`), **nuevo** `TestHintsConEstadoDinamico` (etiqueta por defecto
  + sobrescritura + rebind combinado).
- `internal/tui/refcol_test.go`: 9 `newRefLayout(...)` → `+ prefixCommon`;
  **nuevos**: `refLeaf` (hoja, subgrupo, proyecto vacío, sin subgrupo), ciclo de
  `next()`, ancho por modo (24/34/16), `prefixOf` vacío en `full`/`leaf`.
- `internal/tui/table_test.go`: 3 `newRefLayout(...)` → `+ prefixCommon`.
- `internal/tui/list_test.go`: **nuevos** de integración sobre `listLines` — línea
  de prefijo presente en `common` y ausente en `full`/`leaf`; `full`/`leaf`
  recuperan una línea; degradación `common` == `full`.
- `internal/tui/section_test.go`: **nuevos** de la tecla `p` — ciclo completo,
  rebind, `tab` no lo reinicia, y que `p` no dispara approve/merge/refresh/salir.
- `internal/tui/app_test.go` o `section_test.go`: **nuevo** del hint — la barra
  nombra el modo y cambia al ciclar, y con rebind muestra la tecla nueva + modo.
- `cmd/prdash/print_test.go`: **nuevo** `TestRunPrintNoAplicaElModoPrefijo` — la
  ruta completa sale entera con ruta de subgrupo, sin línea de prefijo, aunque el
  modo de la TUI sea `leaf`. Fija la independencia de `--print`.

## Riesgos

- **Reflow al ciclar**: cambia el ancho de ITEM y, en `full`/`leaf`, la longitud
  de la lista. Mitigado con `syncScroll()`; a verificar con cursor desplazado.
- **Pérdida de contexto en `leaf`**: la referencia ya no es única por fila. Es
  inherente al modo (y por eso el detalle y `--print` conservan la ruta completa);
  el hint del modo avisa de en qué modo se está.
- **El guard anti-drift de hints**: `TestHintsCubrenTodosLosKeybindings` exige que
  la tecla de cada acción aparezca en la barra. `p` aparece, así que pasa. Si
  alguien registra una acción y olvida `hintOrder`, ese test salta.
- **Tests acoplados a `newRefLayout`**: 9 llamadas. Es trabajo mechanical, no
  riesgo, pero son 9 sitios donde un `prefixCommon` mal puesto haría verde un test
  que ya no prueba lo que dice.
- **Coste por render**: `Hints()` ya se llamaba en cada render y ahora recibe un
  `map` de 1 entrada. Despreciable; se evita `fmt` en el camino caliente.
- **`leaf` con proyecto vacío**: da `"#7"`, que es correcto pero debe estar
  testeado para que nadie lo lea como un bug.

## Orden de trabajo (TDD)

1. **Tests que fallen primero**, por bloques, en este orden:
   a. `refcol_test.go`: `refLeaf`, ciclo de `next()`, ancho por modo, `prefixOf`
      vacío fuera de `common` → luego `refcol.go`.
   b. `list_test.go`: línea de prefijo por modo, alto recuperado, degradación →
      luego `refcol.go` + `list.go`.
   c. `section_test.go`: la tecla `p` (ciclo, rebind, no reinicio con `tab`, ni
      efecto colateral) → luego `update.go` + `app.go`.
   d. `config_test.go`: hint con estado → luego `config.go` + `sections.go`.
   e. `print_test.go`: independencia de `--print` (verde desde el principio: es
      un guard, no una funcionalidad).
2. **Refactor de los 9 + 3 call sites** de `newRefLayout` a `prefixCommon`.
3. **ADR 0005** + `README` + `CHANGELOG`.
4. `make test` y smoke en tmux.

## Verificaciones

- `make test` (build + vet + gofmt + `go test -race ./...`). **Es la única
  garantía: el repo no tiene CI configurada.**
- Smoke manual (tmux + `capture-pane`): `p` cicla los 3 modos; el hint nombra el
  modo; la columna se ensancha/estrecha sin parpadeo; en `full`/`leaf` la lista
  gana una línea y el cursor sigue visible; `tab` no cambia el modo; `--print`
  intacto.
- `prdash --print` con una ruta de subgrupo: ruta completa, sin línea de prefijo.
- Coherencia de ADRs: 0002 → 0004 → 0005 encadenados por "Sustituye a", sin
  cláusulas huérfanas.
