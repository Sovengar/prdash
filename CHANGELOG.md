# Changelog

Todos los cambios notables de prdash se documentan en este fichero.

El formato sigue [Keep a Changelog](https://keepachangelog.com/es-ES/1.1.0/) y el
versionado sigue [Semantic Versioning](https://semver.org/lang/es/).

## [Unreleased]

### Added

- **Los últimos 5 comentarios del PR/MR se ven en el detalle.** En una caja propia
  con su "Comments" en el borde, debajo de la ficha y con su autor. Se piden al
  forge al llegar el cursor al ítem y se cachean, así que navegar no vuelve a
  preguntar y el refresco del inbox no los tira; solo una acción sobre el ítem los
  invalida, que es lo único que puede escribir en la conversación. No hay tecla
  nueva: son parte de la ficha, no una vista aparte, y si el forge no responde el
  panel lo dice (`loading…` / `not read: …` / `none`) en vez de dejar un hueco que
  no se distingue de "este PR no tiene comentarios".
  - Se enseña el **final** de la conversación: los últimos 5, en orden
    cronológico y del más antiguo de esos al más nuevo, que es como se lee una
    discusión. Es donde está lo último que se dijo del PR. El recuento va en el
    borde de abajo de la caja y dice siempre los dos números (`3 of 3`, `5 of 23`):
    el segundo es lo que indica que conviene abrir el PR, y el primero también
    informa con todo a la vista, porque el tamaño de la conversación es parte del
    estado del PR —3 comentarios o 30 no es el mismo PR—. Antes solo salía cuando
    faltaba algo, y ese filtro venía justificado por su coste de una fila, que se
    fue con el recuento al borde.
  - Van en una **caja redondeada con "Comments" en el borde**, no como campos más de
    la ficha: la conversación no es un dato del PR sino lo que la gente dijo de él, y
    un borde lo dice sin tener que explicarlo. La caja va **sangrada una columna a
    cada lado** y con **el mismo gris de borde que el resto** de las cajas. El
    sangrado es lo que hace legible el anidamiento: pegada al borde del panel, sus
    verticales se solapan con las de fuera y cada fila sale `││`, y con el mismo color
    los dos bordes se leerían como un trazo gordo. Para separarlos hubo un tiempo un
    gris un tono más claro, pero en las paletas cálidas ese tono sale amarillento, y
    era un problema de color tapando uno de forma.
  - **Sin comentarios no hay caja.** Una caja alrededor de la palabra "none" no
    separa nada, y como es el estado de todos los PRs sin conversación, un borde
    apareciendo y desapareciendo en cada movimiento del cursor sería ruido. Se queda
    la línea de campo de siempre. Lo mismo con `loading…` y `not read: …`: son
    mensajes de una línea, no conversación.
  - La caja es **todo o nada**: si el presupuesto no da para sus dos bordes más una
    fila por comentario, no se pinta. En un terminal de 30 filas con tres comentarios
    y tres filas libres, la caja cabría para un comentario y perdería los otros dos;
    ver uno y perder dos es peor que no ver ninguno, porque un recorte de la caja no
    parece un recorte: parece que el PR solo tiene ese comentario.
  - El recuento va **embebido en el borde de abajo, a la derecha**, y no
    como una fila suelta del cuerpo. El cuerpo de la caja son las filas que dijo la
    gente, y una de recuento es una que no es de nadie; en el borde, además, es
    gratis, así que ya no es lo primero que se cae cuando el panel va justo, que era
    su destino. El borde llega hasta la esquina y no se apoya en ella, o la línea de
    abajo se leería como partida. La coletilla (`· open the PR to read the rest`) solo
    sale si hay comentarios fuera y si cabe entera: recortada a media frase diría
    menos que la corta.
  - Las **notas de sistema de GitLab se descartan** ("assigned to @x", "added 3
    commits"): no son conversación sino historial de acciones del MR, y llenaban las
    cinco filas con ruido que ya está en otra parte de la ficha. Por eso se piden
    3× el tope y se recorta por la cola después.
  - Para que cupieran, los campos de la ficha pasan a **rejilla de dos columnas
    siempre**, no solo en terminales bajos. En una columna ocupaban 16 de las ~18
    líneas que da el 40% de un terminal normal y no cabía ni un comentario; en
    rejilla ocupan 6. El comentario del README que describía la rejilla como
    fallback de emergencia era ya falso.
  - El **URL sale de la rejilla a una fila a ancho completo**. En media columna se
    leían 40 caracteres de una URL de 80 y quedaba un resto inútil, y una URL que
    no se puede copiar entera no sirve para nada, que es para lo que está. No
    cuesta alto: los 13 campos ocupaban 7 filas y los 12 que quedan más el URL a
    ancho completo siguen siendo 7.
  - `Review` y `Role` se ordenan **al final** de los campos de estado, para que caigan
    en la última fila de la rejilla, que es lo único que sobrevive al recorte. Sacar
    el URL los desplazó una posición y `Review` empujó fuera de la ventana: una
    ficha recortada sin ellos deja de responder a la pregunta para la que está.
  - El cuerpo se lee **entero y por párrafos**, no solo la primera línea, y las filas
    se reparten **según lo que cada comentario necesita**: si caben enteros, cada
    uno toma lo suyo; si no, todos reciben una fila —para que los cinco estén, que
    es lo pedido— y el sobrante va a quien menos tiene, una fila cada vez. El
    reparto a ciegas que hubo antes daba la misma cuota a todos y cortaba un
    comentario de seis párrafos a su primera frase mientras sobraba una fila.
  - Los párrafos no se pegan entre sí, lo que no cabe se marca con `…` (una fila
    que para en mitad de una frase se lee como si el comentario se acabara ahí) y se
    saltan los comentarios HTML de markdown con los que abren los bots de GitHub,
    que en un panel de ancho fijo se comerían la fila.
  - Cuando el panel es pequeño los comentarios son lo **primero que se cae**,
    antes que un campo de la ficha: son lo único que se puede volver a pedir en un
    instante, y un campo que se va no vuelve.

### Removed

- El veto de aprobar un PR/MR propio **ya no ocupa una línea de la ficha** ("approve
  unavailable: … · merge still applies"). Se queda solo en el aviso, que salta al pulsar
  la tecla, que es cuando se puede actuar sobre él. La ficha lo pintaba en todos los
  renders de todos tus PRs —casi todos los de "Created by me"— repitiendo lo que el
  campo `Role` ya dice, y le quitaba dos filas a los comentarios en justo los ítems
  donde más se echa de menos. El veto en sí no cambia: `approve` sigue sin salir y la
  razón se sigue explicando entera. La denegación del forge (`action disabled: …`),
  que es pegajosa y su aviso caduca, sigue en la ficha.

### Fixed

- Reutilizar un worktree cuyo workspace de Herdr estaba cerrado ya no produce un
  review en un workspace suelto. `worktree list` puede devolver un
  `open_workspace_id` **obsoleto**: Herdr lo guarda en su sesión persistida, así
  que un workspace cerrado deja el id apuntando a nada y `pane list` responde
  `workspace_not_found` (verificado en 0.9.1). prdash lo aceptaba sin
  comprobarlo, se quedaba sin pane base, y el layout abría un `workspace create`
  de repuesto: el review aparecía como un workspace independiente del worktree
  que lo contiene, y uno nuevo en cada montaje. Por eso el montaje de un PR
  **nuevo** salía bien y el de uno ya existente no. Ahora el id se valida con
  `pane list` —que de paso da el pane base, y ahorra un viaje— y si no responde
  se cae a la adopción de siempre.
- Un `pane list` que falla ya no degrada en silencio a abrir un workspace
  distinto. Si el contenedor dice que el worktree vive en un workspace y ese
  workspace no responde, el montaje falla nombrando el id en vez de montar el
  review en un sitio que no es el suyo. Es la diferencia entre un error legible y
  un workspace fantasma.
- El review ya no aparece como un workspace suelto de Herdr. `herdr worktree
  create` se niega a abrir un path que ya existe (`fatal: '…' already exists`,
  verificado en 0.9.1), así que al reutilizar un checkout de una sesión anterior
  prdash no tenía forma de crear el workspace nativo: `worktree list` no
  devolvía `open_workspace_id`, el worktree volvía sin contenedor y el layout se
  fabricaba un `workspace create` propio. El resultado era un workspace
  desligado del worktree que lo contiene, y uno nuevo en cada montaje. Ahora la
  reutilización **adopta** el checkout: si no hay workspace abierto, se abre uno
  con cwd en el propio worktree, que es lo que Herdr registra como su workspace
  (comprobado contra 0.9.1: `open_workspace_id` aparece). Si ni eso se puede, el
  montaje falla nombrando la ruta a limpiar en vez de fingir que salió bien.
- El pane de Hunk revisa el **working tree** (`hunk diff` sin revspec) en vez del
  diff del PR contra la rama destino. El pane comparte tab con el editor y el
  agente, así que lo que interesa es lo que se está tocando: el diff del PR es un
  objetivo fijo que no se mueve mientras editas y esconde los cambios en curso.
  El diff del PR sigue disponible por `[commands].hunk` (`hunk diff main...HEAD`).
  La rama destino no se pierde: sigue llegando al pane por `PRDASH_BASE`.

### Changed

- El montaje de `r` abre **dos tabs** en vez de un layout de tres panes: `Review`
  (TUICR + editor) y `Edit` (Hunk + agente), con los dos panes de cada tab al 50 %.
  Leer y editar son dos modos de atención distintos y meterlos en un mismo grid
  hacía que el diff, la review y el agente peleasen por el mismo espacio. El
  primer tab **renombra** el tab que ya trae el worktree en vez de crear uno
  nuevo, precisamente para no dejar una pestaña huérfana de la que el usuario
  tendría que acordarse de cerrar; el segundo lo crea Herdr con `tab create
  --no-focus`. Un tab que se queda sin panes no se abre: una pestaña en blanco es
  ruido, no un layout.
- El pane del editor es nuevo y su orden sale de `[tools].editor` (default `vi`),
  con override verbatim por `[commands].editor`. A diferencia de tuicr, hunk y el
  agente, **nunca se omite**: su orden es un comando de shell y el caso normal es
  justo el que un chequeo de binarios no puede ver — el `vi` que expande a `nvim .`
  en tu rc no existe en el `PATH`, así que buscarlo lo declararía ausente y el tab
  de review se quedaría con un solo pane sin explicación. Una orden mal escrita se
  ve en el propio pane, que es cuando el usuario la está mirando.

### Removed

- **Fuera el plugin de Herdr.** No era necesario para nada de lo que prdash
  promete: el worktree y los panes los abre el propio binario llamando a la CLI
  de Herdr por subproceso (`herdr worktree create`, `herdr pane split`, `herdr
  pane run`). El plugin era solo el punto de entrada que Herdr usaba para llamar
  a prdash. Se van el manifiesto, los subcomandos `prdash herdr <inbox|mount|link>`
  y su dispatch.
  - Con ello cae también la persistencia de la selección
    (`internal/selection`, `SetSelectionPath`, `trackSelection`): existía únicamente
    para que la tecla global del plugin montara lo último seleccionado desde otro
    pane, y sin plugin nadie la lee.
  - **Se pierde** el Ctrl+click sobre una URL de PR/MR para montarla, y la tecla
    de Herdr que montaba lo seleccionado desde fuera de prdash. Para lo segundo
    se salta al pane de prdash y se pulsa `r`. No hay alternativa para lo primero:
    los link handlers son exclusivamente de plugin.
  - **No cambia** cómo se abre el inbox. Sigue siendo `prdash`, y sigue montando
    worktree y panes igual. Lo único que hace falta es lanzarlo desde un pane de
    Herdr (el cliente exige `HERDR_ENV=1`, que Herdr solo inyecta a sus hijos);
    un atajo en `config.toml` es una comodidad, no un requisito.
  - `internal/herdr/` **se queda**: lo sigue usando el orquestador para el layout
    de panes y la provisión nativa de worktrees.

### Added

- Merge con modo elegible y doble confirmación. `m` ya no mergea a la primera: la
  primera pulsación solo arma, la caja de Keybinds se sustituye por la
  confirmación, y la segunda tecla **es** la elección del modo — `m` merge commit,
  `r` rebase, `s` squash, `esc` cancelar. No hay modo por defecto: un merge
  reescribe historia y no se deshace con un comando, así que no debe existir
  ningún camino que lo dispare con una estrategia que el usuario no ha nombrado.
  Cualquier otra tecla desarma y hace lo que haría normalmente, para que un `m` a
  destiempo no deje la vista esperando la segunda pulsación; `q` y `ctrl+c`
  siguen cerrando. Los guards se comprueban al armar y no al confirmar, y el
  armado fija el ítem: si un refresco recoloca el cursor entre medias, el merge
  sale sobre lo que se confirmó o no sale. Los avisos nombran el modo al empezar y
  al terminar, porque un "merge ok" a secas no dice si se aplicó el rebase que
  nadie pidió. Cada forge traduce el modo a su flag: `gh pr merge --merge /
  --rebase / --squash` y `glab mr merge` con `--rebase` / `--squash` o sin
  estrategia (merge commit es ahí la ausencia de flag). Un modo desconocido es un
  warning y no lanza la CLI, porque `gh pr merge` sin flag de estrategia abre un
  prompt que en un subproceso no interactivo se queda colgado.
- Esquema de teclas reorganizado: `r` monta el review, `R` refresca, `m` mergea y
  `a` aprueba. Antes `m` era el review y `M` el merge, así que la tecla de la
  acción destructiva y la de la de montar vivían en el mismo dedo. Un test
  comprueba que los defaults no comparten tecla: `ActionForKey` resuelve por
  orden alfabético, así que una colisión deja una acción muerta sin avisar.

### Fixed

- GitLab ya no reporta "merge ok" sin haber mergeado. `glab mr merge` tiene
  `--auto-merge` en **true** por defecto, así que con un pipeline en marcha la
  orden no mergeaba: solo dejaba el MR en cola de auto-merge y salía con exit 0.
  prdash ahora pasa `--auto-merge=false` siempre. Era un bug silencioso en la
  dirección más incómoda posible: la UI confirmaba algo que no había ocurrido.

- La caja de atajos ya no esconde dos teclas que sí funcionaban. `m` (montar
  review) y `o` (abrir en el navegador) estaban bound y operativas, pero no se
  pintaban nunca: la barra se componía con una lista a mano de seis acciones que
  se había quedado desfasada, mientras la config sí sabía de las ocho. Ahora hay
  una sola fuente —`config.Hints()`, una lista ordenada de la que salen tanto las
  acciones configurables como las teclas fijas—, y un test vigila que ninguna
  acción de `[keybindings]` se quede fuera, así que el desfase no puede volver a
  colarse en silencio.
- `section-next` ya responde a su propio atajo. Estaba cableada a `tab` por
  encima del dispatcher, así que rebindearla en `[keybindings]` anunciaba una tecla
  en la barra de atajos que no hacía nada. Sale ya del mapa configurado.
- La tecla de salir sobrevive al recorte de la barra. Con terminal estrecho las
  líneas de atajos se acotan a tres y se perdía la cola, que era justo donde
  estaba `quit` —el propio comentario del código decía que no podía faltar,
  mientras lo colocaba donde más fácil se perdía. `quit` abre ahora la lista, y
  como el recorte tira por el final es lo último que se cae.

- Fuera la vista de detalle a pantalla completa y su tecla `enter`. El panel
  inferior del 40% ya está siempre visible y se mueve con el cursor, así que la
  ficha no tenía nada que aportar y solo costaba un estado `abierto/cerrado` que
  había que mantener, sincronizar con el refresco y cerrar con `esc`. El detalle
  se lee ahora siempre en el panel, que cuando no cabe entero pasa a rejilla de
  dos columnas antes que recortar campos. Se va con ella la acción `detail` de
  `[keybindings]`.

### Added

- Diffstat en el detalle y en la lista: cuántas líneas añade y borra un PR/MR, y
  sobre cuántos ficheros. Salen de los datos que el inbox ya traía, así que no
  cuestan ninguna llamada extra: en GitHub son los escalares `additions`,
  `deletions` y `changedFiles` del PR, y en GitLab el `diffStats` del MR. El
  detalle los enseña sin compactar (`+381 -36 (11 files)`); la columna DIFF de la
  lista los abrevia para que el ancho no dependa del tamaño del cambio
  (`+381 -36`, `+1.2k -6.7k`). Lo añadido va en verde y lo quitado en rojo, tanto
  en la columna como en el detalle: es notación de diff y nada más — no dice si
  el cambio es bueno, solo qué líneas son nuevas y cuáles desaparecieron, y por
  eso el recuento de ficheros se queda sin colorear. Un diffstat que el forge no
  reportó no se colorea: es una ausencia, no una cifra.
- Avisos transitorios (toasts) superpuestos abajo a la derecha de la vista, con
  caducidad propia (4 s) y tick de 500 ms. El texto se envuelve por palabras y
  la caja se acota al ancho útil, así que un aviso largo nunca desborda ni
  desalinea la vista. El reloj es inyectable: la caducidad se testea sin dormir.
- Panel de detalle siempre visible: la lista queda arriba con scroll y el detalle
  del ítem seleccionado ocupa el 40 % inferior, separado por una regla. La vista
  se rellena hasta su alto reservado, así que ni la tabla ni la barra de atajos
  se mueven al añadir o quitar un ítem.

### Changed

- La columna DIFF va la última y es lo primero que se omite cuando no cabe: a 124
  de terminal con rutas de proyecto largas no hay sitio para las siete columnas,
  y antes que recortar un título se pierde un dato que el detalle trae siempre.
  Aparece con terminal ancha (~133). En el detalle el criterio es el mismo: si el
  panel no cabe, el diffstat se omite en vez de empujar el título fuera.
- Un diffstat que el forge no reportó se distingue de uno de cero líneas. La API
  de Todos de GitLab y el respaldo REST de GitHub no lo traen, así que esos ítems
  muestran `-` en la lista y `unknown` en el detalle en lugar de un `+0 -0` que
  parecería un PR vacío. El número de ficheros de GitLab sale de la longitud de
  `diffStats`, y como el forge colapsa los diffs que superan su límite, en un MR
  enorme las cifras son un mínimo.
- La columna ITEM ya no recorta la ruta de proyecto por la cabeza. Cada sección
  declara en su cabecera el prefijo de ruta que comparten sus ítems y las filas
  solo pintan el sufijo (`mobile-frontend#1198` en vez de
  `APPCITTI/vsocial/backend/mobile-…`). El prefijo se alinea en fronteras `/` y
  nunca se come el segmento final, así que la celda siempre conserva el nombre
  del proyecto y su número. El ancho de la columna sale del contenido —el sufijo
  más largo del inbox más su hueco de separación, acotado a 34 runes de
  ranura— y lo que aun así no cabe se recorta por la cola, nunca por el frente.
  El detalle (`enter`) y `--print` siguen mostrando la ruta completa. Decisión y
  alternativas en [ADR 0002](docs/adr/0002-item-column-common-prefix.md).
- Las columnas de la tabla ya no se pegan entre sí. El ancho de una columna
  incluye su hueco de separación, así que el texto se recorta un rune antes de
  llenarla: antes, cualquier texto que midiera el ancho exacto —ROLE con
  `review req`, o ITEM con su ancho dinámico— quedaba pegado a la columna
  siguiente.
- La celda de la tabla admite ahora varios tramos con estilo propio, que es lo
  que permite los dos colores de la columna DIFF. El ancho se sigue midiendo en
  texto plano —el relleno va al final del último tramo— así que los códigos ANSI
  no descuadran la tabla. El detalle colorea después de medir y recortar, por el
  mismo motivo.

### Fixed

- La query de un MR concreto ya no falla siempre en GitLab. `mergeRequest(iid: 7)`
  pasaba el iid como literal entero, pero el schema lo declara `String!` y GraphQL
  no coacciona Int a String, así que la respuesta era un
  `argumentLiteralsIncompatible` y nada más. Como `ItemState` es el refresco que
  se hace tras `approve` y `merge`, actuar sobre un MR dejaba el ítem sin
  actualizar y con un aviso de parseo. Los tests no lo cazaban porque el runner
  falso devuelve JSON sin validar la query; ahora hay un test que fija el tipo
  del literal. No confundir con el `flexInt` del `iid` en la respuesta.
- Aprobar un PR/MR propio ya no se intenta: la TUI lo corta antes de llamar a la
  CLI, con lo que se ahorran las tres llamadas por pulsación. La identidad del
  usuario sale del probe de sesión que ya se hacía (`gh auth status` /
  `glab auth status`), sin llamadas nuevas; si no se puede leer, se decide por
  sección. `merge` no cambia: sigue valiendo sobre PR/MR propios.
- El rechazo de auto-aprobación ya no se confunde con un conflicto ni con un
  fallo de red: `conflicto en el forge: gh pr review … (exit 1)` era en realidad
  `GraphQL: Review Can not approve your own pull request`. Se clasifica como
  denegación permanente y muestra el motivo, no el stderr de la CLI.

## [0.1.0] - 2026-09-25

Primera versión: inbox cross-forge (F1) y orquestador de review (F2). Coincide
con la versión del manifiesto del plugin (`plugin/herdr/herdr-plugin.toml`).

### Added

**F1 — Inbox cross-forge** ([#1](https://github.com/Sovengar/prdash/pull/1)):

- TUI (bubbletea v2) con tres secciones —creados por mí, review pedido/asignados
  y menciones— con dedupe por identidad y precedencia de sección.
- Forges GitHub (vía `gh`, GraphQL con `reviewDecision` y checks) y GitLab
  self-managed (vía `glab`, GraphQL + Todos API, REST bajo `/git/api/v4/`).
- Detalle del ítem (título, autor, ramas, número, URL, review y checks) sin
  salir de la TUI.
- Refresco manual (`r`) y automático (60s configurable) con indicador de última
  actualización por forge, backoff ante rate limit/timeout y guard para no pisar
  una acción en curso.
- Paginación sin tope con carga progresiva, snapshot en `cache` y refresco
  incremental por cursor.
- Acciones approve/merge vía `gh`/`glab` (o delegando en `tuicr`), con manejo de
  conflicto y relectura del ítem.
- Degradación honesta: estado explícito por forge/sección (nunca "vacío" si hubo
  error); Bitbucket presente como adapter no operativo, sin red.
- Modo `prdash --print` (texto plano, mismo orden que la TUI).

**F2 — Orquestador de review** ([#2](https://github.com/Sovengar/prdash/pull/2)):

- Resolución del repo local, clon bare configurable, fetch del ref de review
  (`refs/pull/N/head`, `refs/merge-requests/N/head`, incluidos forks) y rama
  local de trabajo.
- Provisión de worktree con dos implementaciones intercambiables: git directo
  (fuera de Herdr) y nativa de Herdr (dentro), con reuso del worktree existente y
  varios worktrees por repo.
- Layout de 3 panes sobre el worktree (TUICR, Hunk, agente opencode), con
  omisión avisada de las herramientas ausentes.
- Plugin de Herdr (`plugin/herdr/herdr-plugin.toml`): pane del inbox, acción
  `mount-review`, link handler de URLs de PR/MR y subcomandos
  `prdash herdr inbox|mount|link`.
- Atajo `m` para montar el review del ítem seleccionado; `prdash.mount-review`
  sin URL resuelve la selección persistida en
  `$XDG_STATE_HOME/prdash/selection.json`.
- Degradación fuera de Herdr: F1 sigue operativo y el layout se informa como no
  disponible.
- Comando `prdash worktrees [list|remove]`: lista los worktrees con ownership
  `prdash-…`, marca huérfanos y borra solo a petición explícita (nunca los
  ajenos); los worktrees se conservan al cerrar la app.

### Known limitations

- **F3 (auto-review con gate y allowlist) no incluido**: solo diseño/milestone
  documentado en `docs/planning/archive/0001-mvp/f3-milestone.md`.
- Readiness de panes con `herdr pane wait-output` pendiente: los panes se lanzan
  fire-and-forget.
- `herdr worktree create` abre también el workspace del repo fuente; su
  cierre/gestión queda pendiente.
- LOWs residuales no bloqueantes de las reviews adversariales de F1 (PR #1) y
  F2 (PR #2).
- Fuera de alcance: Bitbucket funcional, gitlab.com funcional, vista de "todos
  los abiertos", webhooks/daemon, gestión de repos locales más allá del worktree
  y multi-usuario.
