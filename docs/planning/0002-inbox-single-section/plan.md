# 0002 — Inbox de una sola sección con leyenda de conteos — Plan

adr_required: true
adr_reason: la feature elimina la cabecera interna de sección, que es justo donde el ADR 0002 (Accepted) fija que vive el prefijo de ruta común; reubicarlo es una decisión de diseño con alternativas reales descartadas, y además cambia el modelo de navegación (una sección activa en vez de un cursor plano cross-sección).
adr_title: adr-0004-inbox-single-section
adr_path: docs/adr/0004-inbox-single-section.md
adr_note: el ADR supersede la cláusula del ADR 0002 que ancla el prefijo en la cabecera de sección; el resto del ADR 0002 (cálculo del prefijo, ancho de ITEM, recorte por la cola) sigue vigente.

## Resultado esperado

El Inbox deja de pintar las tres secciones apiladas: muestra **una sola sección
activa** (`Mine`/`Assigned`/`Mentioned`), con **Assigned** al abrir, y `tab`
cicla `Assigned → Mentioned → Mine → Assigned`. El borde superior izquierdo del
Inbox lleva una **leyenda de conteos** con el formato exacto
`Mine (9) · Assigned (4) · Mentioned (0)`, con la activa resaltada, que sustituye
al título `Inbox`. El **prefijo de ruta común (ADR 0002) se conserva**: con una
única sección visible, todas las filas comparten sección, así que el prefijo se
calcula sobre la activa y se sigue mostrando. Cada sección recuerda su cursor y
su scroll. El modo `--print`, el snapshot/cache y la API de `[keybindings]` no
cambian.

## Alcance

- **In**: sección activa + default Assigned + ciclo de `tab`; leyenda de conteos
  en el borde superior izquierdo; sin cabecera interna de sección; prefijo común
  de la activa visible; cursor/scroll por sección; `(empty)`, `loading more…` y
  avisos de la sección activa.
- **Out**: `inbox.Build` (dedupe/autoridad) intacto; `--print` intacto; formato
  de cache intacto; API de config intacta; adapters de forge intactos; **el
  prefijo seleccionable/toggleable es una feature POSTERIOR** (aquí solo se deja
  la costura, sin config ni teclas nuevas).

## Enfoque (alto nivel)

Cambio de **vista y estado de UI** en `internal/tui`, más una etiqueta corta en
`forge/model`. El pipeline de datos (streams → `inbox.Build` → secciones) no se
toca: solo decide **qué sección se pinta** y **cómo se compone la lista**. El
estado pasa de un cursor plano sobre todas las filas a **una sección activa con
su propio cursor/scroll**, guardando la posición de cada sección al cambiar.

## Decisiones clave

1. **Sección activa única con estado propio.** `Model` gana una sección activa
   (`activeSection`, default `model.SectionReview` = Assigned) y un cursor/scroll
   por sección. `rows()`/`selected()`/`clampCursor()` operan sobre la sección
   activa. Al cambiar de sección se guarda la posición actual y se restaura la de
   la destino (default 0/0), acotada al contenido nuevo.

2. **`tab` cicla, reutilizando `section-next`.** Se conserva la acción y su tecla
   por defecto (`tab`); cambia su efecto: avanza la sección activa en el orden
   `Assigned → Mentioned → Mine → Assigned` (orden de **ciclo** distinto del orden
   de **leyenda**, ver decisión 3). **Cicla siempre**, aunque la sección destino
   esté vacía (el usuario quiere poder ver su estado vacío y su conteo).
   `gotoNextSection`, `sectionOffsets` y `sectionIndexAtCursor` quedan obsoletos y
   se eliminan. La barra de atajos (`hintOrder`, label `section`) y la API de
   `[keybindings]` no cambian.

3. **Leyenda en el borde superior izquierdo, sustituyendo `Inbox`.** Se compone
   iterando `m.inbox.Sections` (que ya viene en orden `authored > review >
   mentions` → `Mine · Assigned · Mentioned`) y se pasa como **título del borde**
   de la caja del Inbox. Formato exacto:
   `Mine (9) · Assigned (4) · Mentioned (0)`. La activa se pinta **resaltada**
   (color/negrita) y las demás **atenuadas**. En terminal estrecho el borde ya
   trunca el título (ANSI-aware) por la derecha, así que la caja no se descuadra.

4. **Mapeo de etiquetas: `Legend()` sin tocar `String()`.** Se añade
   `func (s model.Section) Legend() string` → `Mine`/`Assigned`/`Mentioned`, junto
   a `String()`. Se **conserva** `String()` (`Created by me`/`Review / assigned`/
   `Mentions`) porque lo usa `--print` (que no debe cambiar) y es la etiqueta
   larga del modo de datos. La leyenda usa `Legend()`; el resto, `String()`.

5. **Dónde vive el prefijo de ruta común (propuesta).** En una **línea fija al
   inicio del cuerpo de la lista**, atenuada, que contiene **solo el prefijo**
   (p. ej. `  · APPCITTI/vsocial/backend/`), **sin título ni conteo**. Si la
   sección activa no tiene prefijo común (un solo ítem, o nada en común), la
   línea no se pinta y las celdas ITEM llevan la ruta completa recortada por la
   cola (regla ya existente de ADR 0002).
   - **Por qué**: mantiene la caja ITEM estrecha (las filas solo pintan el
     sufijo), respeta el **formato exacto** de la leyenda (que es de conteos) y es
     el sitio donde el prefijo ya estaba (era parte de la cabecera). Además es la
     **costura natural para el futuro toggle/selección**: el prefijo se compone
     como una unidad discreta desde una única fuente, así que una feature
     posterior puede ocultarlo, alternarlo o hacerlo interactivo **sin tocar la
     leyenda, el ancho de la tabla ni el cambio de sección**. No se añade config
     ni teclas ahora.
   - **Alternativas descartadas**:
     - *En la leyenda del borde*: rompe el formato exacto y mezcla conteos de las
       tres secciones con la ruta de una sola; la leyenda es de conteos.
     - *En la cabecera `PRDash`*: está lejos de las filas, describe estado de
       forges/refresco, y el prefijo cambia con la sección activa → confuso.
     - *Ruta completa en cada celda ITEM (abandonar el prefijo)*: pierde
       exactamente la legibilidad que motivó el ADR 0002 y contradice el motivo
       declarado por el usuario para esta feature.
     - *Reintroducir una cabecera de sección solo con el prefijo*: es lo mismo que
       la opción elegida, pero conservando semántica de "header" y el conteo
       duplicado; se descarta por nomenclatura y por duplicar la leyenda.

6. **`newRefLayout` pasa a la sección activa.** Hoy `listLines()` llama
   `newRefLayout(m.inbox.Sections)` y dimensiona ITEM por el sufijo más largo de
   **todas** las secciones. Ahora solo se pinta una, así que debe recibir **solo
   la sección activa** (una rebanada de un elemento): así el prefijo y el ancho de
   ITEM son los de la activa, y no se gasta ancho en secciones que no se ven. Se
   **conserva la firma** `newRefLayout([]inbox.Section)` para no romper los tests
   unitarios de `refcol`; cambia el **llamador** y el valor pasado.

7. **Estados de la sección activa (puntos abiertos resueltos).**
   - **`(empty)`**: se muestra una línea en la lista cuando la activa no tiene
     ítems y no hay avisos; la leyenda muestra su conteo 0.
   - **`loading more…`**: es de la **sección activa** y va como línea atenuada al
     final del cuerpo de la lista (no en la leyenda, que tiene formato exacto ni
     en `PRDash`, que es de forges). Si la sección que pagina no es la activa, no
     se muestra; al volver a ella, reaparece.
   - **Avisos (`sectionProblems`)**: se muestran **solo de la sección activa**,
     como hoy (`⚠ <forge>: could not be queried (…)`), al inicio del cuerpo. Los
     avisos de secciones no activas no se pintan (al tabular a esa sección
     aparecen). Riesgo asumido y documentado.

8. **Sin cambios en cache y en `--print`.** El cache indexa por
   `(forge, section, kind)` y no depende de qué se pinta → intacto. `--print`
   itera `box.Sections` y usa `Section.String()` → intacto (por eso no se toca
   `String()`). Se verifica, no se modifica.

## Ficheros afectados

**Producción**

- `internal/forge/model/model.go`: añadir `Section.Legend()`; no tocar `String()`.
- `internal/tui/app.go`: `activeSection` (default `SectionReview`) + cursor/scroll
  por sección; `rows()`/`selected()`/`clampCursor()` sobre la activa; helpers de
  sección activa, prefijo de la activa y descriptores de la leyenda.
- `internal/tui/update.go`: `case "section-next"` → ciclo de sección activa;
  eliminar `gotoNextSection`/`sectionOffsets`/`sectionIndexAtCursor`;
  `moveCursor`/`goTop`/`pageBy`/`syncScroll` sobre la activa.
- `internal/tui/list.go`: `listLines()` compone **solo la activa** (línea de
  prefijo opcional + avisos + header de columnas + filas, o `(empty)`, + `loading
  more…`); sin título de sección; `newRefLayout` con la sección activa.
- `internal/tui/sections.go`: `listSection()` pasa la **leyenda** como título del
  borde (con resaltado) en vez de `"Inbox"`.
- `internal/tui/refcol.go`: `newRefLayout` sigue igual; ajustar comentarios y el
  uso desde `list.go`.
- `internal/tui/styles.go`: estilo del tramo activo de la leyenda y de la línea
  de prefijo (reutilizar `styleDim` + un resaltado).
- `internal/tui/bordered/bordered.go`: **sin cambios** (admite título ya
  estilizado); verificar el caso ANSI.
- `internal/config/config.go`: **sin cambios** (se mantiene `section-next`/`tab`).
- `cmd/prdash/print.go`: **sin cambios**.
- `docs/adr/0004-inbox-single-section.md`: nuevo ADR (supersede la cláusula del
  ADR 0002 sobre la cabecera).

**Tests afectados / nuevos**

- `internal/tui/list_test.go`: reescribir `TestViewBoxesEverySection` (espera
  `Inbox`) y los que asumen una lista plana; nuevos: leyenda exacta + resaltado,
  `(empty)` de la activa, línea de prefijo.
- `internal/tui/table_test.go`: reescribir `TestNavigationMovesCursor` y
  `TestSectionNextHonorsRebind` (ciclo de sección activa); actualizar
  `newRefLayout(m.inbox.Sections)`.
- `internal/tui/refcol_test.go`: reescribir los de integración sobre cabecera
  (`TestListLinesMuestranElPrefijoEnLaCabecera`,
  `TestListLinesSinPrefijoConservanLaRuta`); los unitarios de `newRefLayout`
  se mantienen.
- `internal/tui/app_test.go`: `TestPaginationIndicator` debe ser sobre la sección
  activa (hoy usa authored con default Assigned); reescribir
  `TestViewShowsThreeSectionsWithBothForges` (espera las 3 secciones a la vez) y
  `TestSectionEmptyVsError` (espera varios `(empty)` simultáneos, ahora solo se
  pinta el de la activa); conservar los de `sectionItems`.
- `internal/forge/model/model_test.go`: caso de `Legend()`.
- Nuevos: default Assigned; ciclo `tab`; cursor/scroll por sección; `loading
  more…`/avisos de la activa; `--print` sin cambios; leyenda en ancho estrecho.

## Riesgos

- **ANSI en el título del borde**: el borde envuelve el título con su color y los
  tramos estilizados traen sus resets; verificar que ni el ancho ni el color de
  relleno de la línea superior se rompen (se componen los tramos con estilos
  completos, sin caracteres "desnudos").
- **Prefijo + paginación**: el prefijo común puede encogerse al llegar más
  páginas y ensanchar ITEM **una vez** (comportamiento ya aceptado en ADR 0002).
- **Avisos de secciones no activas**: dejan de verse en la lista (solo su
  conteo). Aceptado; el aviso aparece al tabular a esa sección.
- **Tests acoplados a la lista multi-sección**: el alcance de reescritura de tests
  es parte del trabajo, no un efecto colateral menor.
- **`syncScroll`/`scrollFor`**: operan ahora sobre las líneas de la activa;
  asegurar que el guardado/restauración de scroll por sección no deje el cursor
  fuera de la ventana.

## Orden de trabajo (grueso)

1. Estado: `activeSection` + cursor/scroll por sección + helpers; adaptar
   `app.go`/`update.go`; eliminar los helpers obsoletos.
2. Render: `listLines` de una sola sección con línea de prefijo; `newRefLayout`
   con la activa; `listSection` con la leyenda.
3. Etiquetas: `model.Section.Legend()`; composición y resaltado de la leyenda.
4. Puntos abiertos: `(empty)`, `loading more…` y avisos de la activa.
5. Tests: reescribir los acoplados y añadir los nuevos.
6. ADR 0004 y cierre.

## Verificaciones

- `go build ./... && go vet ./... && go test ./...` (runner del repo, `make test`).
- Smoke manual de TUI (tmux + `capture-pane`): default Assigned; ciclo de `tab`;
  leyenda exacta con resaltado; prefijo visible de la activa; posición recordada
  al volver; `(empty)` y `loading more…`.
- `prdash --print` sigue mostrando las tres secciones con nombres largos.
- Rebuild del binario instalado (`~/.local/bin/prdash`) al terminar.
