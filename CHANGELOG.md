# Changelog

Todos los cambios notables de prdash se documentan en este fichero.

El formato sigue [Keep a Changelog](https://keepachangelog.com/es-ES/1.1.0/) y el
versionado sigue [Semantic Versioning](https://semver.org/lang/es/).

## [Unreleased]

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
