# ADR 0003 — prdash no es un plugin de Herdr

- **Estado**: Accepted
- **Fecha**: 2026-09-26
- **Decisor**: usuario (buble)
- **Alcance**: integración con Herdr, montaje del review, `internal/herdr/`
- **Patrón de nombres**: `docs/adr/NNNN-slug.md`
- **Sustituye a**: nada. El diseño con plugin nunca tuvo ADR; su plan quedó
  descrito en `docs/research/herdr-0.9.1-contract.md` §Flujo F2, que este ADR
  deja obsoleto en lo que al plugin se refiere.

## Contexto

prdash nació con la intención de ser un plugin de Herdr: un
`plugin/herdr/herdr-plugin.toml` con `[[actions]]` (montar el review), `[[panes]]`
(el pane del orquestador) y `[[link_handlers]]` (Ctrl+click sobre la URL de un
PR/MR), más un dispatch `prdash herdr <inbox|mount|link>` y la persistencia de la
selección en `internal/selection/`.

Al implementarlo quedó claro que el plugin no compraba nada de lo que prdash
promete. Su único papel era ser **el punto de entrada que Herdr usaba para llamar
a prdash**; todo lo que hay detrás —resolver el repo, crear el worktree, abrir
tabs y panes, lanzar las herramientas— lo puede hacer el binario llamando a la
CLI de Herdr por subproceso (`herdr worktree create`, `herdr tab create`,
`herdr pane split`, `herdr pane run`). El `root_pane` que devuelve
`worktree create` y el `root_pane` que devuelve `tab create` son el mismo pane
base, así que el layout no necesita un pane propio que lo ancle.

La consecuencia de quitarlo no es solo de código: **se pierde funcionalidad**, y
esa es la parte que hay que pesar.

## Decisión

**prdash no es un plugin de Herdr.** Se instala y se lanza como un binario
(`make install`, `prdash`) y habla con Herdr por CLI. Con ello:

1. Se van el manifiesto (`plugin/herdr/herdr-plugin.toml`), los subcomandos
   `prdash herdr <inbox|mount|link>` y su dispatch.
2. Se van los targets `plugin-link` / `plugin-unlink` del `Makefile` y la var
   `PLUGIN`. `plugins.json` deja de ser un fichero derivado de prdash.
3. Cae la persistencia de la selección (`internal/selection`, `SetSelectionPath`,
   `trackSelection`): existía únicamente para que la tecla global del plugin
   montara lo último seleccionado desde otro pane, y sin plugin nadie la lee.
4. `internal/herdr/` **se queda**: lo sigue usando el orquestador para la
   provisión nativa de worktrees y el layout de panes. Cambia de punto de
   entrada, no de propósito.
5. El layout pasa de 3 panes a **2 tabs** (`Review`: TUICR | editor, `Edit`:
   Hunk | agente), que es lo que cabe en el ancho real de un pane.
6. Un atajo en `config.toml` es una **comodidad, no un requisito**. prdash no
   edita el `config.toml` del usuario. Lanzado a mano desde un pane de Herdr
   funciona igual; lanzado desde una terminal normal, `r` degrada a git directo
   y monta el worktree sin panes.

## Alternativas consideradas y descartadas

- **Mantener el plugin solo por el Ctrl+click.** Es lo único que el plugin
  compraba de verdad. Se descartó porque los link handlers son exclusivamente de
  plugin: no hay forma de conservar esa mitad sin el manifiesto entero, y el
  manifiesto entero es justo lo que no se quiere. El Ctrl+click sobre la URL de
  un PR queda perdido sin alternativa, y es un coste asumido a conciencia.
- **PluginActions / comandos de Herdr como punto de entrada intermedio.** Un
  mecanismo más ligero que el manifiesto completo. No se evaluó a fondo porque
  el layout ya funciona sin él, y añadirlo después es un commit, no una
  migración.
- **Editor de `config.toml` desde prdash** (el patrón de hunkdiff `setup-keys`,
  con backup y `herdr config check`). Se descartó aparte: tocar el
  `config.toml` del usuario no es necesario para que nada funcione, y es
  superficie de conflicto y de permisos que no compensa una comodidad.
- **RConsola atrás la persistencia de selección para un keystroke de Herdr.**
  Innecesario: el estado de la selección ya vive en la TUI, y el atajo
  secundario se resuelve saltando al pane y pulsando `r`.

## Consecuencias

**Positivas**

- Un solo artefacto que instalar (`prdash`) y un solo proceso. Nada de
  `plugins.json`, nada de `min_herdr_version` que mantener al día, nada que
  registrar en el config del servidor.
- El layout deja de depender de que Herdr cree un pane para el orquestador: usa
  el pane base que ya viene con el worktree y el tab, y reutiliza el tab
  existente en vez de dejar pestañas huérfanas.
- El montaje es explícito y observable: cada comando que prdash ejecuta se puede
  leer en `internal/herdr/client.go` sin atravesar un runtime de plugins.
- Menos código: se van el dispatch, `internal/selection/` y, con esta decisión,
  `internal/forge/parse/url.go` (el parser de URLs para el link handler) y
  `forge.Registry` (registro de adapters para el manifiesto). Ninguno tenía
  callers en producción.

**Negativas / costes**

- **Ctrl+click sobre la URL de un PR/MR ya no monta el review.** No hay
  alternativa: el link handler de Herdr solo existe dentro de un plugin. El
  remedio es manual: copiar la URL y montarla en el pane.
- **La tecla de Herdr que montaba lo seleccionado desde fuera de prdash ya no
  existe.** El remedio es saltar al pane de prdash y pulsar `r`.
- El layout de 2 tabs depende del ancho del pane. En uno estrecho, el pane del
  editor y el de TUICR se quedan ilegibles; el diseño asume que el review se lee
  con el workspace ampliado.
- Se pierde la capacidad de declararse a sí mismo ante Herdr: si Herdr muestra
  los plugins instalados, prdash no aparece, y no hay superficie propia para
  futuras acciones.

**Verificación**

- `CHANGELOG.md` §Removed → "Fuera el plugin de Herdr" describe el cambio y sus
  consecuencias para el usuario.
- `README.md` §"Dentro de Herdr" dice explícitamente que prdash no es un plugin
  y qué se pierde.
- `docs/research/herdr-0.9.1-contract.md` §Flujo F2 está reescrito al diseño
  actual; §Degradación por capacidad ausente refleja que no hay camino por
  debajo de 0.9.0.
- `internal/herdr/client_test.go` cubre el veto de mutación por versión mínima, y
  `internal/herdr/layout_test.go` el layout de 2 tabs y su tolerancia a fallos
  parciales.
