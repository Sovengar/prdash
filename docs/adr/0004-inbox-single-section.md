# ADR 0004 — Inbox de una sola sección con leyenda de conteos

- **Estado**: Accepted
- **Fecha**: 2026-09-26
- **Decisor**: usuario (buble)
- **Alcance**: inbox de la TUI, sección activa, leyenda del borde, columna ITEM
- **Patrón de nombres**: `docs/adr/NNNN-slug.md`
- **Sustituye a**: **ADR 0002**, en dos cláusulas:
  1. La que ancla el prefijo de ruta común **en la cabecera de cada sección**
     (§Decisión punto 1 y el punto 4 «el frente perdido es el grupo, que la
     cabecera ya declara»).
  2. El alcance **«todas las secciones»** de su punto 2: el ancho de ITEM ya no
     sale del sufijo más largo de todas las secciones sino **solo del de la
     sección activa** (decisión 5 de este ADR). La fórmula —contenido acotado a
     `[6, 34]`— y el recorte por la cola siguen vigentes.
  El resto del ADR 0002 —el cálculo del prefijo y la regla de la sección de un
  solo ítem— sigue vigente tal cual.

## Contexto

El inbox apilaba las tres secciones (`Mine`/`Assigned`/`Mentioned`) y cada una
declaraba su prefijo de ruta común en una cabecera interna con título y conteo.
Hasta aquí, ADR 0002.

El problema es que el inbox real no es una lista plana: es **una lista de
trabajo**. Con las tres secciones siempre visibles, el ojo tiene que separar tres
órdenes distintos en una sola pantalla, y el cursor navega cross-sección,
saltando entre elementos que no se comparan entre sí. Además, el prefijo común
(ADR 0002) pierde su razón de ser cuando el conjunto pintado mezcla secciones:
el ancho de ITEM se dimensionaba por el sufijo más largo de **todas**, gastando
espacio en secciones que quizá no interesan ahora.

El objetivo es mostrar **una sola sección a la vez** y, con ella, el prefijo de
ruta de esa sección. Al pintarse una sola, todas las filas comparten sección, el
prefijo se calcula sobre el conjunto visible y el ancho de ITEM es el suyo.

## Decisión

1. **Una sección activa, con posición propia.** El inbox pinta una sola sección.
   Al abrir, la activa es **`Assigned`** (`review`). Cada sección recuerda su
   cursor y su scroll; al cambiarla se guarda la posición de la que sale y se
   restaura la de la destino, acotada al contenido nuevo.
2. **`section-next` (por defecto `tab`) cicla la sección activa** en el orden
   `Assigned → Mentioned → Mine → Assigned`. Cicla siempre, aunque la destino
   esté vacía: su estado y su conteo son justo lo que se quiere ver. La acción y
   su tecla siguen saliendo de `[keybindings]`; la barra de hints no cambia.
3. **La leyenda de conteos sustituye al título `Inbox`** en el borde superior
   del inbox, con el formato exacto `Mine (9) · Assigned (4) · Mentioned (0)`:
   la etiqueta corta de cada sección (`Section.Legend()`) y su número de ítems.
   La activa va resaltada y las demás atenuadas. En un terminal estrecho el borde
   la trunca ANSI-aware por la derecha, así que la caja nunca se descuadra.
4. **El prefijo de ruta común pasa a una línea fija al inicio del cuerpo de la
   lista**, atenuada y con **solo el prefijo** (`  · APPCITTI/vsocial/backend/`),
   sin título ni conteo. Sustituye exactamente lo que antes hacía la cabecera de
   sección, pero sin duplicar la leyenda. Si la sección activa no tiene prefijo
   común (un solo ítem, o nada en común), la línea no se pinta y las celdas ITEM
   llevan la ruta completa recortada por la cola (regla de ADR 0002 intacta).
5. **El ancho de ITEM se calcula sobre la sección activa.** `newRefLayout` recibe
   solo la sección que se pinta, de modo que el prefijo y el ancho son los de
   ella y no se gasta ancho en sufijos de secciones que no se ven. La firma no
   cambia; cambia el llamador.
6. **`(empty)`, `loading more…` y los avisos son de la sección activa.** Una
   sección no activa solo aporta su conteo a la leyenda; al tabular a ella
   aparecen su estado vacío, su indicador de paginación y sus avisos.
7. **El modo `--print`, el formato de cache y la API de config no cambian.**
   `Section.String()` se conserva para `--print`; la leyenda usa `Section.Legend()`.

## Alternativas consideradas y descartadas

- **Prefijo en la leyenda del borde.** Rompe el formato exacto de la leyenda
  (que es de conteos), mezcla los conteos de las tres secciones con la ruta de
  una sola y hace la línea más larga de lo que el borde puede truncar con
  elegancia.
- **Prefijo en la cabecera `PRDash`.** Está lejos de las filas, describe el
  estado de forges y del refresco, y el prefijo cambia con la sección activa:
  leerlo junto al estado de refresco confunde dos cosas sin relación.
- **Ruta completa en cada celda ITEM (abandonar el prefijo).** Pierde justo la
  legibilidad que motivó el ADR 0002: cada fila repite el grupo y el ancho de la
  tabla se come el título.
- **Reintroducir una cabecera de sección solo con el prefijo.** Es la opción
  elegida con otro nombre: conserva la semántica de «header» y el conteo
  duplicado respecto a la leyenda. Se descarta por nomenclatura y duplicación.
- **Saltar las secciones vacías al ciclar.** Obligaría a no poder ver el estado
  vacío de una sección ni su conteo. Se cicla siempre.

## Consecuencias

**Positivas**

- Una sola lista y un solo orden: el cursor navega dentro de la sección que se
  está mirando, sin saltos entre conjuntos que no se comparan.
- El prefijo de ruta común (ADR 0002) se conserva y se aprovecha mejor: al
  calcularse sobre la única sección visible, libera ancho de ITEM sin competir
  con secciones ausentes.
- La leyenda da de un vistazo el reparto de trabajo —`Mine (9) · Assigned (4) ·
  Mentioned (0)`— donde antes hacía falta leer tres cabeceras apiladas.
- Deja abierta la **costura para el prefijo seleccionable/toggleable** (feature
  posterior): el prefijo se compone como una unidad discreta desde una única
  fuente, así que una feature posterior puede ocultarlo, alternarlo o hacerlo
  interactivo sin tocar la leyenda, el ancho de la tabla ni el cambio de sección.
  Este ADR **no** implementa config ni teclas nuevas.

**Negativas / costes**

- **Los avisos de secciones no activas dejan de verse.** Una sección con un
  fallo solo muestra su conteo en la leyenda; el aviso aparece al tabular a ella.
  Coste asumido a cambio de la vista de una sola sección.
- **El prefijo puede encogerse al llegar más páginas** y ensanchar ITEM una vez:
  es el mismo reflow único que ya aceptaba el ADR 0002, ahora acotado a la
  sección activa.
- **El borde superior lleva ANSI** en el tramo resaltado. Cada tramo de la
  leyenda se compone con un estilo completo, sin caracteres «desnudos», porque
  el borde reenvuelve cada segmento con su color y un reset interior no restaura
  el color del borde. Verificado a varios anchos.

**Verificación**

- `internal/tui/section_test.go`: default `Assigned`; ciclo de `tab` (incluida la
  sección vacía); leyenda con formato exacto, resaltado y conteos deduplicados;
  prefijo de la activa, ausencia de prefijo común y ancho de tabla a varios
  anchos; cursor/scroll recordados por sección; refresco que conserva la
  posición; `(empty)`, `loading more…` y avisos solo de la activa; leyenda
  truncada en terminal estrecho.
- `internal/tui/refcol_test.go`: el prefijo aparece una sola vez (su línea) y las
  filas solo pintan el sufijo.
- `internal/forge/model/model_test.go`: `Legend()` con `String()` intacto.
- `cmd/prdash/print_test.go`: el modo `--print` sigue imprimiendo las tres
  secciones con sus nombres largos.
