# 0001 — prdash MVP (F1 + F2) — Resumen de cierre

- **Estado**: completada. F1 y F2 implementadas y mergeadas/listas; F3 queda como
  milestone documentado, fuera de alcance.
- **Versión**: `0.1.0` (coincide con `min_herdr_version`/`version` del manifiesto
  del plugin). Ver `CHANGELOG.md`.
- **PRs**: [#1](https://github.com/Sovengar/prdash/pull/1) (F1, merge rebase,
  `mergeCommit 62c18dc`) y [#2](https://github.com/Sovengar/prdash/pull/2) (F2,
  este). Base de ambos: `main`.
- **Comportamiento esperado**: `behavior.feature` (fuente única; no es Cucumber).
- **Plan**: `plan.md`. **Contexto de navegación**: `context.md`.

## Qué entrega F1 — Inbox cross-forge

Una TUI Go (bubbletea v2) que responde "¿qué PR/MR me toca?" mezclando GitHub y
un GitLab self-managed:

- **Tres secciones**: creados por mí, review pedido/asignados y menciones, con
  dedupe por identidad (forge, host, proyecto, número) y precedencia de sección.
- **Datos ricos por forge**: GitHub vía GraphQL (`reviewDecision`, checks) y
  GitLab self-managed vía GraphQL (`currentUser`) + Todos API para menciones;
  REST bajo subfolder `/git/api/v4/` encapsulado tras el adapter.
- **Detalle del ítem** (título, autor, ramas, número, URL, estado de review y
  checks) sin salir de la TUI, reflejando el estado vivo.
- **Refresco manual (`r`) y automático** (60s por defecto, configurable, `0` solo
  manual) con indicador "última actualización" **por forge** y backoff ante rate
  limit/timeout; el refresco no pisa una acción en curso.
- **Carga progresiva y paginación sin tope** por sección, con snapshot en `cache`
  y refresco incremental por cursor.
- **Acciones approve/merge** desde el inbox vía `gh`/`glab` (o delegando en
  TUICR cuando aplica), con conflicto/relectura cuando el ítem cambió.
- **Degradación honesta**: nunca "vacío" si hubo error; estado explícito por
  forge/sección (401 = forge no disponible) y la TUI no aborta.
- **Bitbucket solo interfaz**: adapter registrado que responde "no soportado"
  sin red.
- **`prdash --print`**: el inbox en texto plano, mismo orden que la TUI, para
  scripts y para comprobar el pipeline (y, desde F2, la ruta del worktree de los
  reviews ya montados).

## Qué entrega F2 — Orquestador de review

A partir de un ítem, deja listo el entorno de review (worktree + layout) sin
montar nada a mano:

- **Repo resolver** (`reporesolver`, único dueño del namespace de rutas): índice
  remoto→local sobre `roots`, clon bare configurable si el repo no está local,
  fetch del ref de review (`refs/pull/N/head` / `refs/merge-requests/N/head`,
  incluye forks) y creación de la rama local de trabajo.
- **Provisión de worktree** (`worktree.Provisioner`) con dos implementaciones
  intercambiables: **git directo** fuera de Herdr y **nativo de Herdr** dentro
  (liga el checkout a un workspace con IDs estables). El llamador nunca sabe cuál
  corre. Reutiliza el worktree existente de un ítem y permite varios por repo.
- **Layout de 3 panes** sobre el worktree: TUICR sobre el PR/MR, Hunk con el
  diff y un agente opencode, con cwd/env/argv correctos; un binario ausente omite
  su pane con aviso sin tumbar el resto.
- **Plugin de Herdr**: `plugin/herdr/herdr-plugin.toml` con el pane del inbox,
  la acción `mount-review`, el link handler de URLs de PR/MR (GitHub y GitLab
  self-managed) y subcomandos `prdash herdr inbox|mount|link`; una sola fuente de
  config/credenciales/versión (mismo binario + manifiesto).
- **Tecla `m`** en la TUI: monta el review del ítem seleccionado; la acción
  `prdash.mount-review` sin URL también lo resuelve (la TUI persiste el ítem
  seleccionado en `$XDG_STATE_HOME/prdash/selection.json`).
- **Degradación fuera de Herdr**: el worktree se monta igual (git directo) y el
  layout se informa como no disponible; F1 sigue operativo.
- **`prdash worktrees [list|remove]`**: lista los worktrees propiedad de prdash
  (ownership `prdash-…`), marca huérfanos y borra solo a petición explícita,
  nunca los ajenos. Los worktrees se conservan al cerrar (sin borrado implícito).

## Decisiones clave

- **ADR 0001** (`docs/adr/0001-worktree-provisioning.md`): prdash hace **siempre**
  el fetch del ref y la creación de la rama local, y luego delega la provisión del
  worktree; **nativo de Herdr dentro de Herdr, `git worktree add` fuera**. El
  acoplamiento a Herdr queda confinado a un único puerto.
- **Plugin = mismo binario + subcomandos + manifiesto**: sin duplicar config,
  credenciales ni versión.
- **El loop comentario→agente no es de prdash**: la responsabilidad termina al
  abrir panes con cwd, env y argv correctos (sin daemon, sin poller, sin API de
  comentarios).
- **Detección de entorno en un solo sitio**: solo `internal/herdr` lee
  `HERDR_ENV`; el resto recibe el entorno inyectado (TUI testeable "fuera de
  Herdr").
- **F3 fuera de alcance**: solo diseño/milestone (ver `f3-milestone.md`).

## Cómo usar / instalar

```sh
make build            # compila en ./bin/prdash
make install          # instala en ~/.local/bin/prdash
make config GITLAB_HOST=gitlab.miempresa.com   # ~/.config/prdash/config.toml

prdash                # TUI del inbox (tecla m: montar review del seleccionado)
prdash --print        # inbox en texto plano (+ ruta del worktree de cada review)
prdash worktrees      # lista/limpieza de los worktrees de review de prdash

make plugin-link      # herdr plugin link "$(pwd)/plugin/herdr" (dev)
```

Keybinding (en `~/.config/herdr/config.toml`, prdash no lo edita):

```toml
[[keys.command]]
key = "prefix+m"
type = "plugin_action"
command = "prdash.mount-review"
description = "prdash: montar review del PR/MR"
```

Seguido de `herdr server reload-config`.

Referencias: `docs/adr/0001-worktree-provisioning.md` (decisión permanente) y
`docs/research/herdr-0.9.1-contract.md` (contrato e2e con Herdr 0.9.1).

## Verificación e2e real (Herdr 0.9.1)

Ejecutada contra `herdr 0.9.1-preview.2026-09-21-0ff0f27e2226` con fixtures
reproducibles (evidencia en `docs/research/herdr-0.9.1-contract.md`
§Verificación local y en el ADR 0001 §Verificación):

- `herdr worktree create` desde un **clon bare** con rama local existente y
  `--path` destino: **OK** (cierra la verificación pendiente del plan).
- `--label` etiqueta el **workspace**, no el worktree → prdash conserva su
  etiqueta de ownership del lado del llamador.
- `plugin link <dir>` es idempotente; `plugin action invoke` registra su
  resultado en el log de plugins.
- `pane split --no-focus` acepta `--ratio`/`--cwd`/`--env`.
- **Hallazgos/efectos laterales**: `worktree create` también abre el **repo
  fuente** como workspace; esta build de `workspace close` no expone `--group`.

Nota: la suite del proyecto se verifica con
`go build ./... && go vet ./... && gofmt -l . && go test -race -count=1 ./...`.

## Deuda declarada / no incluido

- **F3 auto-review** (gate + allowlist, dry-run por defecto, nunca aprobar con
  análisis fallido/parcial): solo documentado en `f3-milestone.md`; sin código.
- **Readiness de panes** con `herdr pane wait-output`: los panes se lanzan
  fire-and-forget; falta confirmar el arranque de cada herramienta.
- **Cierre/gestión del workspace del repo fuente** abierto por
  `herdr worktree create`: no se cierra al limpiar.
- **LOWs residuales de las reviews adversariales** de F1 (PR #1) y F2 (PR #2),
  no bloqueantes: p. ej. en F2, una selección malformada sin `saved_at` se reporta
  como "obsoleta" (no "incompleta"), el TTL de 24h con última escritura gana entre
  instancias de TUI, `syncSelection` puede persistir el primer ítem del cache al
  arrancar, el fallback `RemoveAll` de `GitDirect.Remove` podría borrar un
  checkout vivo bloqueado, `removablePath` omite el guard de base si `Base==""`,
  el temporal de `selection.Save` no es único, y `Fresh` acepta fechas futuras.
  En F1, un `r` manual durante una acción en curso no queda cubierto por el guard
  de ciclo. Detalle en las observaciones de revisión del proyecto.
- **Fuera de alcance** (recordatorio): Bitbucket funcional, gitlab.com funcional,
  vista de "todos los abiertos", webhooks/daemon, gestión de repos locales más
  allá del worktree, multi-usuario.
