# ADR 0002 — Columna ITEM con prefijo de ruta común por sección

- **Estado**: Accepted
- **Fecha**: 2026-09-26
- **Decisor**: usuario (buble)
- **Alcance**: inbox de la TUI, columna ITEM
- **Patrón de nombres**: `docs/adr/NNNN-slug.md`

## Contexto

La columna ITEM pintaba `proyecto#número` con un ancho fijo de 30 runes
(`colRef`) y recorte **por la cabeza**. Las rutas de proyecto de GitLab con
subgrupos no encajan: `APPCITTI/vsocial/backend/api-gateway#1234` son 41 runes y
salía `APPCITTI/vsocial/backend/api-…`.

El recorte por la cabeza se come exactamente lo que distingue un ítem de otro:
el nombre del repo (la hoja de la ruta) y el `#número`. Dos filas de proyectos
distintos quedaban visualmente idénticas, que es la peor forma de perder
información: no se nota que falta.

No era pérdida de dato: el detalle (`enter`) y `--print` ya mostraban la ruta
completa. El problema era de legibilidad en la tabla, y la convención ya
establecida en el repo para la columna FORGE era «corto en tabla, completo en
detalle».

Restricciones de partida:

- La columna debe alinearse entre secciones y entre líneas, o la tabla baila al
  escribir encima.
- STATE y CHECKS no se recortan nunca; en anchos estrechos se pierden columnas
  por la derecha, no información dentro de ellas.
- No se puede perder la posibilidad de referenciar un ítem por su número, que es
  lo que se usa al hablar de él y al verificar un check.

## Decisión

1. **El prefijo de ruta común de cada sección se declara en su cabecera** y las
   celdas de ITEM solo pintan el sufijo. El prefijo se calcula sobre la
   intersección de segmentos completa de todos los ítems de la sección, alineado
   en fronteras `/` y **nunca comiéndose el segmento final**: la celda siempre
   conserva la hoja y el número.
2. **El ancho de ITEM sale del contenido**, no de una constante: el sufijo más
   largo de todas las secciones más el hueco de separación, acotado a `[6, 34]`
   runes de ranura. El mínimo mantiene la separación con la columna vecina; el
   techo evita que ITEM se coma TITLE.
3. **El ancho de una columna incluye su hueco de separación** (el texto usable es
   un rune menos, `textWidth`), así que el texto se recorta al hueco y nunca
   llena la ranura. No era un caso hipotético: ROLE con `"review req"` medía
   exactamente el ancho de su columna, así que STATE salía pegada, y lo mismo
   pasaba con ITEM, cuyo ancho dinámico lo calcula el propio contenido.
4. **Lo que aun así no quepa se recorta por la cola**, no por la cabeza, para que
   el `#número` sobreviva. El frente perdido es el grupo, que la cabecera ya
   declara.
5. Una sección de un solo ítem no declara prefijo (no hay nada que compartir) y
   su celda lleva la ruta sola: entera si cabe, recortada por la cola si no.

## Alternativas consideradas y descartadas

- **Recorte por la cola en la celda completa** (`APPCITTI/vs…/api-gateway#1234`).
  Es lo más barato y arregla la pérdida del número, pero cada fila repite el
  grupo completo: se sigue comiendo el espacio que el prefijo común libera.
- **Hoja del proyecto en ITEM y grupo en la columna FORGE** (`GLab@app` +
  `api-gateway#1234`). Máxima compacidad, pero mezcla semántica: FORGE deja de
  ser legible de un vistazo; y con ítems de grupos distintos se pierde el grupo.
- **Anchos dinámicos de todas las columnas** midiendo el contenido. Resuelve el
  síntoma genérico, pero es más invasivo (obliga a redimensionar `fitColumns`) y
  no arregla que la ruta larga repita el grupo en cada fila.
- **Ítem en dos líneas** cuando la ruta no cabe. Rompe la compacidad de la tabla
  y la aritmética del cursor, que es la parte más frágil de la vista con scroll.

## Consecuencias

**Positivas**

- Con subgrupos largos la fila dice qué proyecto y qué número es, sin truncar:
  `mobile-frontend#1198` en vez de `APPCITTI/vsocial/backend/mobile-…`.
- La columna se ajusta al inbox real: con rutas cortas se estrecha y TITLE gana
  espacio; con rutas largas no se ensancha más del tope.
- Es determinista: sale solo del conjunto de ítems actual, sin estado extra ni
  claves que puedan divergir de lo pintado.

**Negativas / costes**

- El ancho de ITEM depende del contenido: al terminar una paginación, más ítems
  pueden reducir el prefijo común y ensanchar la columna **una vez**. Acotado por
  el tope de 34, y es un reflow único.
- Una sección de un solo ítem con ruta larga no tiene prefijo que compense el
  recorte: se ve `…social/backend/api-gateway#100`, sin `APPCITTI/`. El detalle
  sigue siendo la red de seguridad.
- `refLayout` indexa el prefijo por `model.Section`: dos secciones con el mismo
  `Kind` se pisarían. Hoy es imposible (`inbox.Build` garantiza una por kind) y
  el modo de fallo es seguro (la celda cae a la ruta completa), pero es un
  invariante a respetar si alguna vez hay secciones duplicadas.
- El ancho de ITEM se calcula una vez por render, en `listLines`. Con muchas
  secciones e ítems es O(ítems × segmentos) en cada frame; despreciable a esta
  escala, y evita que cabecera y filas midan distinto.

**Verificación**

- `internal/tui/refcol_test.go`: 10 casos dirigidos de prefijo (subgrupo largo,
  corte a media ruta, sin nada en común, `owner/repo`, proyectos idénticos,
  sección de un ítem, prefijos parciales, proyecto vacío, misma ruta en hosts
  distintos), más los de sufijo, recorte y ancho; dos de integración sobre
  `listLines`; y 300 rondas aleatorias sobre los invariantes (celda no vacía,
  celda = recorte exacto del sufijo, cola intacta al recortar, ancho dentro de
  los límites).
