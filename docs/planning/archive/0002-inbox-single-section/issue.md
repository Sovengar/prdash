# 0002 — Inbox de una sola sección con leyenda de conteos (feature)

## Problema

El Inbox pinta las tres secciones apiladas (Mine / Assigned / Mentioned) y el
usuario tiene que desplazarse por una lista larga para llegar a la sección que le
interesa. La tecla `tab` (acción `section-next`) hoy solo mueve el cursor al
primer ítem de la siguiente sección: no cambia el foco ni reduce el ruido de las
otras secciones, que siguen ocupando pantalla. No hay una forma directa de "ver
solo lo que me toca ahora" ni de saber de un vistazo cuántos ítems hay en cada
sección.

Se quiere que el Inbox muestre **una única sección a la vez**, con un ciclo por
`tab` y una **leyenda de conteos** en el borde superior, de modo que el foco y el
contexto (cuántos hay en cada sección) estén siempre visibles sin desplazarse.

## Alcance (in)

- **Sección activa única**: solo se pinta una sección del Inbox a la vez.
  - `Mine` = `authored`, `Assigned` = `review`, `Mentioned` = `mentions`.
  - Al abrir, la sección por defecto es **Assigned** (`review`).
  - `tab` cicla **Assigned → Mentioned → Mine → Assigned** (se conserva la tecla
    `tab`; la acción sigue siendo `section-next`).
- **Leyenda de conteos** en el borde **superior izquierdo** del Inbox, que
  **sustituye al título actual `Inbox`**. Formato exacto:
  `Mine (9) · Assigned (4) · Mentioned (0)`. Los números son el nº de ítems de
  cada sección (ya deduplicados por `inbox.Build`). La sección activa se
  **resalta** (color/negrita) y las otras van atenuadas.
- **Sin header interno de sección**: la línea `"<Sección> (n)"` dentro de la
  lista se elimina.
- **Cursor y scroll por sección**: cada sección recuerda su posición al volver
  a ella.
- Etiquetas de leyenda: `Mine`, `Assigned`, `Mentioned`.

## No-alcance (out)

- La consolidación/dedupe/autoridad de `inbox.Build` (autoría `authored > review
  > mentions`) no cambia: solo cambia qué se **pinta**.
- El modo `--print` (`cmd/prdash/print.go`) no cambia de comportamiento.
- El formato del snapshot/cache no cambia.
- La API de `[keybindings]` no cambia: `section-next` sigue existiendo y `tab`
  sigue siendo su default.
- No se tocan los adapters de forge ni el parseo.

## Criterios de aceptación

- [ ] Al abrir la TUI, el Inbox muestra solo la sección **Assigned**.
- [ ] `tab` cicla Assigned → Mentioned → Mine → Assigned y solo la sección activa
      se pinta.
- [ ] La leyenda del borde superior izquierdo tiene el formato
      `Mine (n) · Assigned (n) · Mentioned (n)` con los conteos reales, y la
      sección activa resaltada; no aparece el título `Inbox`.
- [ ] No existe header interno de sección en la lista.
- [ ] Volver a una sección restaura su cursor y su scroll.
- [ ] La sección activa vacía muestra `(empty)`; la que sigue paginando muestra
      `loading more…`; sus warnings se siguen mostrando (solo de la activa).
- [ ] `--print` sigue imprimiendo las tres secciones con sus nombres largos.
- [ ] `go build ./... && go vet ./... && go test ./...` pasa.

## Riesgos / verificaciones pendientes

- **Avisos de secciones no activas**: un fallo de consulta de una sección no
  activa ya no se ve en la lista (solo se ve su conteo). Se acepta: al tabular a
  esa sección aparece su aviso. Confirmar que no se rompe el principio "nunca
  mostrar vacío si hubo error" de forma confusa.
- **Leyenda en borde con ANSI**: el título del borde se envuelve con el color del
  borde; los tramos con su propio estilo y sus resets no deben romper el ancho ni
  el color de relleno de la línea superior. Verificar con terminales estrechos
  (la leyenda se trunca por la derecha, ANSI-aware).
- **Prefijo de ruta común (ADR 0002)**: hoy el prefijo vive en la cabecera de
  sección, que desaparece. Hay que decidir dónde queda (ver `plan.md`) para no
  perder la legibilidad que motivó el ADR.
- **Tests acoplados**: `TestViewBoxesEverySection` (espera caja `Inbox`),
  `TestSectionNext*`/`TestNavigationMovesCursor` (asumen cursor cross-sección),
  `TestListLines*` (asumen header de sección con prefijo) y
  `TestPaginationIndicator` (asume que authored se pinta) deben reescribirse.
