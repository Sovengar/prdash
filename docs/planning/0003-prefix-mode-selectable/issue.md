# 0003 — Prefijo de ruta seleccionable y toggleable (feature)

## Problema

El Inbox muestra **una sola sección a la vez** (ADR 0004) y su ruta se parte en
dos: un **prefijo común** en una línea fija atenuada al inicio del cuerpo, y la
**celda ITEM** con solo el sufijo (ADR 0002). Es la mejor reparto posible con una
sola opción, pero hoy es la **única** opción: no hay forma de elegir otra.

Ese diseño único no le sirve igual a todo el mundo, y los tres casos que falla no
son cuestión de gusto:

1. **Quien no lee dos sitios.** Con el prefijo en su línea, la referencia de un
   ítem no está en la fila: hay que mirar la línea de arriba y la de abajo. La
   ruta completa en la celda es más lenta de leer en tablas largas, pero es
   auto-contenida: vale para un ítem suelto, para copiar, para nombrar en voz
   alta y para una tabla que se lee fila a fila.
2. **Quien quiere densidad.** El prefijo común es generoso: `APPCITTI/vsocial/`
   ocupa una línea entera para el grupo y paga el mismo precio en cada fila.
   Cuando solo importa la hoja (`api-gateway#1234`), esa línea es un gasto.
3. **Quien cambia de sección con prefijos distintos.** `tab` salta entre
   secciones cuyo prefijo cambia, y cada salto reescribe la línea de arriba. Con
   la ruta en la celda, cada fila es estable y no hay que releer el contexto.

El ADR 0004 (§Consecuencias) dejó esta costura **explícitamente abierta**: el
prefijo se compone como unidad discreta desde una única fuente, así que una
feature posterior puede ocultarlo, alternarlo o hacerlo interactivo **sin tocar la
leyenda, el ancho de la tabla ni el cambio de sección**.

## Alcance (in)

Una tecla, **`p`** (acción `prefix-mode`, default `p`), cicla **3 modos de
prefijo**. Es **global**, no por sección, y **no se persiste** (al reabrir el
programa vuelve a `common`).

| Modo | Línea de prefijo | Celda ITEM | Para qué |
|---|---|---|---|
| `common` (default) | sí, con el prefijo común de la activa | solo el sufijo | comportamiento actual (ADR 0002/0004) |
| `full` | **no** | la referencia **completa** `proyecto/subgrupo#n` | fila auto-contenida |
| `leaf` | **no** | solo la **hoja** del proyecto más `#n` | máxima densidad |

- Ciclo: `common` → `full` → `leaf` → `common`.
- **Seleccionable = elegir qué prefijo se ve**, no a qué ítem se aplica. La
  columna ITEM ya se dimensiona por contenido de la sección y "el prefijo de un
  ítem concreto" es un caso degenerado que `leaf` ya resuelve mejor.
- El **ancho de ITEM se recalcula por modo** (sufijo más largo vs referencia
  completa vs hoja más larga), con el mismo acotado `[itemWidthMin, itemWidthCap]`
  (`[6, 34]`). **Sin parpadeo**: el layout se calcula **una vez por render** en
  `listLines`, nunca por fila.
- En `full` y `leaf` **no se pinta la línea de prefijo** (no hay prefijo que
  declarar) y la lista recupera esa línea de alto.
- **Degradación**: si la sección activa no tiene prefijo común (<2 ítems o nada en
  común), `common` **degrada al comportamiento de `full`**: no inventa un
  prefijo ni repite la ruta dos veces.
- El **hint de `p` nombra el modo actual** (`p prefix: full`), no la tecla a
  secas, para saber dónde se está sin contar pulsaciones.

## No-alcance (out)

- **NO persistir el modo.** Nada nuevo en `fileConfig` ni en el schema TOML. Al
  registrar la acción en `DefaultKeybindings` + `hintOrder`, `[keybindings]` la
  podrá sobreescribir por el mecanismo genérico que ya existe: es correcto, no
  hace falta plumb para eso.
- **NO tocar `--print`**: `cmd/prdash/print.go` no usa `newRefLayout` /
  `refSuffix` / `sectionPrefix` (verificado). Se mantiene esa independencia y se
  **añade un test que la fije**.
- **NO cambiar** la deduplicación/autoridad de secciones ni el ciclo de `tab`.
- **NO** hacer el prefijo seleccionable por ítem (ver Problema, decisión 1).
- **NO** cambiar el cálculo de `sectionPrefix` (sigue siendo el prefijo común
  estricto, alineado en `/` y sin comerse la hoja).
- Tecla `p`: no colisiona con `q`, `R`, `r`, `a`, `m`, `tab`, `o`, `j`/`k`,
  `home`/`end`, `pgup`/`pgdn`, ni con las teclas de modo del merge
  (`m`/`r`/`s`), que consume la pulsación entera mientras está armado.

## Criterios de aceptación

- [ ] Al abrir la TUI el modo es `common` y la vista es **exactamente** la de hoy
      (línea de prefijo + sufijos).
- [ ] `p` cicla `common` → `full` → `leaf` → `common`, en ese orden, y el ciclo
      es global: no se reinicia al cambiar de sección con `tab`.
- [ ] `full`: no hay línea de prefijo y cada celda ITEM lleva la referencia
      completa, recortada por la cola si no cabe.
- [ ] `leaf`: no hay línea de prefijo y cada celda ITEM lleva
      `<último segmento del proyecto>#n`.
- [ ] El **ancho de ITEM cambia con el modo** y se mantiene en `[6, 34]` en los
      tres; el layout se calcula una vez por render (todas las filas de la tabla
      miden lo mismo, sin parpadeo al escribir encima).
- [ ] `full` y `leaf` **recuperan la línea de alto** que ocupaba el prefijo.
- [ ] Con una sección sin prefijo común, `common` se ve **exactamente** como
      `full`: sin línea de prefijo y con la ruta completa en la celda.
- [ ] El hint de la barra nombra el modo actual y cambia al ciclar; con un
      override en `[keybindings]` muestra la tecla nueva **y** el modo actual.
- [ ] `tab` (ciclo de sección) no cambia el modo; el cursor y el scroll por sección
      siguen igual.
- [ ] `--print` **no cambia**: sigue imprimiendo las tres secciones con
      `proyecto/subgrupo#n` completo.
- [ ] `make test` en verde (build + vet + gofmt + `go test -race`).
- [ ] ADR coherentes: se actualiza la cláusula del prefijo del ADR 0002 o se
      escribe un 0005 que lo supersede; CHANGELOG y README sin contradicciones.

## Decisión ABIERTA (bloquea el plan) — el hint dinámico

**El enunciado pide dos cosas que en el código actual son incompatibles.**

- *"El hint de `p` debe nombrar el modo ACTUAL"* → la etiqueta depende de estado
  de runtime (`m.prefixMode`).
- *"registrar la acción en `DefaultKeybindings` + `hintOrder` … no hace falta
  plumb nuevo"* → la etiqueta vive en `hintOrder` y se resuelve sin estado.

Verificado en el código:

- `internal/config/config.go:414-418` — `type hint struct{ action, key, label string }`:
  `label` es un **string estático**.
- `internal/config/config.go:430-441` — `var hintOrder = []hint{…}`: sin estado.
- `internal/config/config.go:446-459` — `func (c Config) Hints() []string`: función
  **pura de `Config`**, sin parámetros de estado.
- `internal/tui/sections.go:213` — único llamador: `m.cfg.Hints()`. El `Model` (y
  con él el modo) **sí** está disponible ahí.
- `internal/config` **no puede** importar `internal/tui`: el ciclo es al revés
  (`internal/tui/app.go:191` ya hace `cfg config.Config`).

Es decir: la **tecla** sí se resuelve por el mecanismo genérico que ya existe
(`KeyFor` → `[keybindings]`, sin plumb). Lo que **no** tiene mecanismo es la
**etiqueta dinámica**. Hace falta elegir una de estas tres:

- **A (recomendada) — `Config.Hints()` recibe el estado dinámico.** La etiqueta de
  `hintOrder` pasa a ser el valor por defecto y la TUI la sobreescribe:
  `m.cfg.Hints(map[string]string{"prefix-mode": "full"})`, o una forma equivalente
  (variádica/struct). **Conserva todos los invariantes documentados**: `hintOrder`
  sigue siendo la única fuente de lista, orden y etiqueta por defecto; el
  rebind de `[keybindings]` sigue funcionando; el guard anti-drift
  (`TestHintsCubrenTodosLosKeybindings`) sigue valiendo. Coste: la firma de
  `Hints()` cambia y hay un map/struct por render.
- **B — marcador en la etiqueta + sustitución en la TUI.** `label: "prefix: %s"` y
  `strings.Replace` en `sections.go`. Toca menos código, pero la barra vista desde
  la config muestra un `%s` crudo, `TestHints` (config_test.go:327, lista `want`
  literal) tendría que esperar el marcador, y "la etiqueta la pone la TUI"
  difumina el invariante de `hintOrder` como fuente única.
- **C — la TUI compone ese fragmento aparte**, como ya hace con
  `mergeConfirmText()` cuando el merge está armado (`sections.go:209-213`).
  Precedente real, pero saca el hint de `p` del orden de `hintOrder` y de
  `Config.Hints()`, que es justo lo que el guard anti-drift verifica.

**Pregunta para el usuario**: ¿A, B o C? El enunciado asumía que no había
plumb; la realidad es que la etiqueta dinámica necesita una de las tres, y la
decisión cambia la firma de `Hints()` y la forma de los tests.

## Micro-decisiones (no bloquean; se proponen en el plan)

- **Orden del hint en la barra.** `maxHintLines = 3` y el recorte tira **por la
  cola** (`config.go:426-429`, `wrapHint`): lo que va al final desaparece primero
  en terminal estrecho. Propuesta: `p` va **después de `refresh`** y antes de las
  teclas fijas `j/k` y `pgup/dn` — sobrevive mejor que la cola, pero no roba
  sitio a `quit`/`section`/acciones.
- **`TestHints`** tiene una lista `want` literal: hay que añadir `p prefix: …`
  ahí. Es el guard funcionando, no una sorpresa.
- **Estado en `Model`, cero schema.** `prefixMode` como campo de `Model`, con
  default `common`. Sin `fileConfig`, sin TOML, sin migraciones.

## Corrección al enunciado (no bloquea, pero no se documenta falso)

El enunciado justifica `leaf` con que *"desambigua cuando la sección mezcla repos
sin prefijo común"*. **Eso es al revés**: `leaf` es justamente el modo que **no**
desambigua ese caso. Para `a/one#1` y `b/one#2` (nada en común) `leaf` pinta
`one#1` y `one#2` — indistinguibles — mientras `full` pinta `a/one#1` y
`b/one#2`. El valor real de `leaf` es **densidad** cuando las hojas **sí** son
únicas (que es el caso normal: el prefijo común existe justamente porque las
hojas difieren), no desambiguación. El modo se implementa igual; lo que cambia es
la frase del ADR, que no debe afirmar una propiedad que el modo no tiene.

## Riesgos / verificaciones pendientes

- **El modo no se persiste**: es lo pedido. El aviso para el futuro es que
  `p` **sí** acaba siendo configurable por TOML al pasar por `DefaultKeybindings`
  (mecanismo genérico), aunque no se plumbée el modo. Si algún día se quiere el
  modo persistente, es un campo nuevo en `fileConfig` + su test.
- **Pérdida de contexto al pasar a `full`/`leaf`**: la información de grupo
  desaparece de la vista paraítems de la fila, no de la aplicación. El detalle y
  `--print` siguen mostrando la ruta completa. Verificar que la vista nunca
  muestra un ítem sin `proyecto#n`.
- **Reflow al ciclar**: cambiar de modo cambia el ancho de ITEM y, en
  `full`/`leaf`, la lista recupera una línea. Es un reflow instantáneo y
  determinista (sin estado por ítem), pero hay que comprobar que el **cursor no
  se sale de la ventana** al ciclar: `p` no mueve el cursor, pero sí cambia
  `len(listLines)` (una línea menos), así que el `scroll` puede quedar
  desfasado → hay que `syncScroll()` en el ciclo.
- **`hintOrder` es global, no por sección**: correcto, pero significa que el hint
  nombra un modo que **no** aplica igual a las tres secciones a la vez. Es
  coherente con que solo se pinte una sección.
- **Tests acoplados**: `TestHints` (lista literal), los de `refcol` sobre
  `newRefLayout` (su firma/cálculo), los de `listLines` sobre la línea de prefijo
  y los de `section_test.go` sobre el prefijo de la activa. Todos se adaptan al
  modo por defecto (`common`) y siguen valiendo como está.
