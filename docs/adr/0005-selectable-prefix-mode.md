# ADR 0005 — Prefijo de ruta seleccionable y toggleable

- **Estado**: Accepted
- **Fecha**: 2026-09-26
- **Decisor**: usuario (buble)
- **Alcance**: inbox de la TUI, columna ITEM, línea de prefijo, barra de atajos
- **Patrón de nombres**: `docs/adr/NNNN-slug.md`
- **Sustituye a**: **ADR 0002**, en su §Decisión punto 1 (el prefijo de ruta común
  se declara en la cabecera de la sección) y en el punto 4 («el frente perdido es
  el grupo, que la cabecera ya declara»).
- **Sustituye a**: **ADR 0004**, en su §Decisión punto 4 (el prefijo pasa a una
  línea fija al inicio del cuerpo de la lista), que este ADR convierte en la
  línea que solo se pinta en el modo por defecto.
- **Deja vigente**: el cálculo del prefijo común (`sectionPrefix`: alineado en
  fronteras `/`, prefijo común estricto, sin comerse nunca el segmento final), la
  fórmula del ancho de ITEM (contenido más el hueco de separación, acotado a
  `[6, 34]`), el recorte por la cola (`truncateTail`), la regla de la sección de
  un solo ítem, y el punto 5 del ADR 0004 (el ancho se calcula sobre la sección
  activa).

## Contexto

Desde el ADR 0002 la ruta de un ítem se parte en dos: el prefijo que comparten los
ítems de la sección vive en una línea fija, y la celda ITEM solo pinta el sufijo.
El ADR 0004 movió esa línea al inicio del cuerpo de la lista, al pintar una sola
sección a la vez, y dejó escrita la costura:

> Deja abierta la **costura para el prefijo seleccionable/toggleable** (feature
> posterior): el prefijo se compone como una unidad discreta desde una única
> fuente, así que una feature posterior puede ocultarlo, alternarlo o hacerlo
> interactivo sin tocar la leyenda, el ancho de la tabla ni el cambio de sección.

El reparto del ADR 0002 es el mejor de los posibles **con una sola opción**, y es
justo lo que pasa: no hay manera de elegir otra. Y no todas las lecturas lo
prefieren igual:

- **La fila deja de ser auto-contenida.** Con el prefijo en su línea, la
  referencia de un ítem no está en la fila: hay que leer dos sitios. Para quien
  nombra un PR en voz alta, o lee la tabla fila a fila, o copia la referencia, la
  ruta completa en la celda es más directa aunque la tabla sea más ancha.
- **El prefijo es una línea fija que no se puede apagar.** Con la densidad como
  criterio, `APPCITTI/vsocial/` es una línea entera para el grupo y un texto
  idéntico en todas las filas. Cuando solo importa la hoja, sobra.
- **El prefijo cambia con la sección.** `tab` salta a secciones cuyo prefijo
  común es otro y reescribe la línea de arriba en cada salto. Con la ruta en la
  celda, cada fila es estable y no hay que releer el contexto.

Ninguno de los tres casos es un capricho: son tres lecturas legítimas de la misma
tabla, y el diseño solo hueco.

## Decisión

1. **Tres modos de prefijo, uno global, ciclados por una tecla.** La acción
   `prefix-mode` (default `p`) cicla `common → full → leaf → common`. El modo es
   **global**, no por sección, porque la barra lo nombra una vez y la vista pinta
   una sola sección: un modo por sección no tendría dónde anunciarse.

   | Modo | Línea de prefijo | Celda ITEM |
   |---|---|---|
   | `common` (por defecto) | sí, con el prefijo común de la activa | solo el sufijo |
   | `full` | no | la referencia completa `proyecto/subgrupo#n` |
   | `leaf` | no | la hoja del proyecto más `#n` |

2. **`common` es el comportamiento heredado** y no se toca: es el de los ADR
   0002/0004, y por ser el default es lo que ve quien no pulsa la tecla. La
   feature **no cambia nada** de lo que se veía antes.

3. **Fuera de `common` no hay línea de prefijo, porque no hay prefijo que
   declarar.** No es una decisión de la vista: el layout deja de calcular el
   prefijo común en esos modos, y la lista —que ya solo pintaba la línea si salía
   no vacío— deja de pintarla sola. La lista recupera esa línea de alto.

4. **El ancho de ITEM se recalcula por modo**, con la misma fórmula y el mismo
   acotado `[6, 34]`: el sufijo más largo en `common`, la referencia completa en
   `full`, la hoja más larga en `leaf`. Se calcula **una vez por render** (en
   `listLines`), nunca por fila, para que cabecera y filas midan lo mismo y la
   tabla no baile al escribir encima.

5. **`common` degrada a `full` cuando la sección no tiene prefijo común** (un solo
   ítem, o sin nada en común): no inventa un prefijo ni repite la ruta. No es un
   caso especial sino el comportamiento que ya tenía —sin prefijo, la celda pinta
   la referencia entera—, y por eso la degradación no necesita código propio.

6. **El hint nombra el modo actual** (`p prefix: full`), no la tecla a secas: la
   información de en qué modo se está tiene que estar en la barra, o habría que
   contar pulsaciones. Como la etiqueta depende del estado de la vista y la config
   no la tiene, `Config.Hints` recibe un `HintState` (map por acción) con el
   fragmento que la TUI compone. `hintOrder` conserva la lista, el orden y la
   etiqueta por defecto; el rebind de `[keybindings]` sigue siendo genérico.

7. **El modo no se persiste.** Vive en el `Model` y al reabrir el programa vuelve
   a `common`. No hay campo nuevo en `fileConfig` ni en el schema TOML.
   - Consecuencia asumida: al registrar la acción en `DefaultKeybindings`, la
     **tecla** `p` pasa a ser configurable por TOML por el mecanismo genérico que
     ya existía. El **modo** sigue sin persistir. Si algún día se quiere
     persistente, es un campo nuevo en `fileConfig` y su test.

8. **`--print` no cambia.** Imprime la referencia completa siempre: no tiene
   columna ni terminal, así que no tiene el problema que el modo resuelve. Fija
   su independencia un test (`TestRunPrintNoAplicaElModoDePrefijo`).

9. **Seleccionable = elegir qué prefijo se ve**, no a qué ítem se aplica. La
   columna ya se dimensiona por el contenido de la sección, y "el prefijo de un
   ítem concreto" es un caso degenerado que `leaf` resuelve mejor.

## Alternativas consideradas y descartadas

- **Prefijo seleccionable por ítem** (elegir un grupo y aplicarlo a un ítem
  suelto). La columna se dimensiona por el conjunto de la sección; el prefijo de
  un ítem concreto es `full` si la ruta no tiene grupo, o `common` si lo tiene
  pero no lo comparten los demás. Ningún caso de uso que `leaf` no cubra.
- **Más de tres modos** (prefijo de dos niveles, ruta por sotto):
  multiplica el número de combinaciones de la barra y del ancho sin aportar una
  lectura que no se pueda lograr cyclando.
- **Prefijo en la leyenda del borde.** Rompe el formato exacto de la leyenda
  (que es de conteos) y mezcla conteos con ruta: ya se descartó en el ADR 0004.
- **Un flag deBool en `[keybindings]` o en `[inbox]` tipo
  `prefix = "full"`.** Haría el modo persistente, que es justo lo decidido en
  contra. Se puede añadir después sin tocar esto: el modo ya sale de un solo tipo.
- **Sustitución de un marcador en la etiqueta** (`label: "prefix: %s"` +
  `strings.Replace` en la TUI). Toca menos código, pero la barra vista desde la
  config muestra un `%s` crudo y "la etiqueta la pone la TUI" difumina el
  invariante de que `hintOrder` es la fuente única de la barra.
- **Que la TUI componga la entrada de `p` por su cuenta** (como ya hace con la
  Confirmación de merge). Sacaría el hint de `p` del orden de `hintOrder` y de
  `Config.Hints()`, que es justo lo que verifica el guard anti-drift
  `TestHintsCubrenTodosLosKeybindings`.
- **Persistir el modo en el snapshot** (el modo es puro de la vista, pero el snapshot
  ya guarda cursor y posición). Se descarta: la posición se recuerda porque es
  trabajo del usuario en curso; el modo de prefijo es una preferencia de lectura,
  y hacerlo persistente ata el comportamiento por defecto a una ejecución
  anterior.

## Consecuencias

**Positivas**

- La vista se adapta a las tres lecturas sin cambiar de herramienta: la más
  densa para escanear, la más explícita para leer o nombrar, la de siempre para
  quien ya se acostumbró.
- El prefijo deja de ser un coste fijo: en `full` y `leaf` la lista recupera la
  línea que ocupaba.
- La costura que el ADR 0004 dejó abierta queda cerrada **sin tocar** la leyenda,
  el ciclo de sección, el cursor por sección ni el pipeline de datos.
- `hintOrder` sigue siendo la única fuente de la barra y el rebind por
  `[keybindings]` sigue siendo genérico: la feature no deblanda ningún invariante
  de la barra.

**Negativas / costes**

- **La referencia deja de ser única por fila en `leaf`.** Dos repos de grupos
  distintos con la misma hoja se vuelven indistinguibles
  (`acme/one#7` y `other/one#8` → `one#7` y `one#8`). Es inherente al modo: su
  valor es la densidad cuando las hojas **sí** son únicas, que es el caso normal
  (el prefijo común existe justamente porque las hojas difieren). El detalle y
  `--print` conservan la ruta completa. **No se promete desambiguación.**
  - Corrijo aquí la justificación que se manejaba al revés: `leaf` es
    el modo que **menos** desambigua de los dos que no usan prefijo.
- **`newRefLayout` exige el modo como parámetro explícito** (10 call sites). Es
  un coste deliberado: con un default, `newRefLayout(secs)` significaría "common"
  en unos sitios y "lo que hubiera" en otros, y el error no lo vería el
  compilador.
- **`p` pasa a ser configurable por `[keybindings]`** aunque el modo no se
  persista. Es la consecuencia de registrar la acción, y es el mecanismo
  genérico que ya existía; no es plumb nuevo.
- **Ciclar el modo cambia el ancho y la altura de la lista.** Resincronizar el
  scroll en cada ciclo es obligatorio (sin él, una lista desplazada deja el
  cursor fuera de la ventana). Está hecho y verificado.
- **Un ciclo más que memorizar**, mitigado por el hint, que nombra el modo.

**Verificación**

- `internal/tui/prefixmode_test.go`: `refLeaf` (subgrupo, `owner/repo`, sin
  subgrupo, varios ítems, proyecto vacío); el ciclo `common → full → leaf →
  common`; los tres `String()` exactos (son lo que ve el usuario); `prefixOf` no
  vacío solo en `common`; el ancho por modo con el fixture de
  `behavior.feature` (24 / 34 topado / 16); la etiqueta por modo y la degradación
  `common == full` sin prefijo común; el recorte exacto en `full`
  (`…/vsocial/backend/api-gateway#100`); el caso en que `leaf` **no** desambigua.
- `internal/tui/prefixmode_list_test.go`: línea de prefijo presente solo en
  `common` y ausente en `full`/`leaf`; etiqueta de la fila por modo; `full` y
  `leaf` recuperan exactamente una línea; la lista en `common` sin prefijo común
  es **idéntica** a la de `full`.
- `internal/tui/prefixmode_key_test.go`: el ciclo con la tecla pulsada; el rebind
  (la tecla nueva cicla, la vieja deja de hacerlo); `tab` no reinicia el modo ni
  al volver a la sección; `p` no arma merge, no refresca, no lanza acción, no
  deja aviso y no mueve el cursor; el cursor y la ventana sobreviven al ciclo;
  un modelo nuevo arranca en `common` (no se persiste).
- `internal/tui/prefixmode_hint_test.go`: el hint nombra el modo y cambia al
  ciclar; el rebind y el modo se combinan (`P prefix: leaf`); con el merge
  armado la Confirmación sustituye a la barra y el modo no se cuela en ella.
- `internal/config/config_test.go`: la entrada `p prefix` sin estado y
  `p prefix: full` con estado; un estado de una acción ausente no inventa una
  entrada; el guard anti-drift `TestHintsCubrenTodosLosKeybindings` sigue
  verde.
- `internal/tui/refcol_test.go`: 300 rondas aleatorias de los invariantes de la
  columna, ahora **en los tres modos** (celda no vacía, celda = recorte exacto de
  la etiqueta, cola intacta al recortar, ancho dentro de `[6, 34]`, prefijo vacío
  fuera de `common`).
- `cmd/prdash/print_test.go`: `--print` imprime la ruta completa de un subgrupo
  largo, sin `…` y sin línea de prefijo.
- `make test` (build + vet + gofmt + `go test -race`). El repo no tiene CI
  configurada: la garantía es el runner local.
