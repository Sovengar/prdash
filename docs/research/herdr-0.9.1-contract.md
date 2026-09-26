---
title: Contrato de integración Herdr para prdash (F2 review orchestrator)
herdr: 0.9.1-preview.2026-09-21-0ff0f27e2226
protocol: 22
Fuente: inspección local + herdr.dev; ver UNVERIFIED
---

# Contrato de integración Herdr 0.9.1 para prdash (F2 review orchestrator)

Host: `herdr 0.9.1-preview.2026-09-21-0ff0f27e2226` · `protocol: 22` · `schema_version: 1` · canal `preview`. `herdr status` → client=server=`0.9.1-preview...`, `endpoint_compatible: yes`, socket `~/.config/herdr/herdr.sock`.

Formato global de salida CLI: **todas** las respuestas JSON son `{"id":"cli:<grupo>","result":{...}}`; los errores de servidor son JSON en **stderr** con exit **1**, los de sintaxis exit **2**. Parsear siempre `.result`.

---

## 1. Detección y entorno

Detección fiable (única oficial): `test "${HERDR_ENV:-}" = 1`. Valor documentado: `HERDR_ENV=1` en procesos dentro de un pane gestionado. En prdash: si falla, abortar (no controlar sesión desde fuera).

Env vars inyectadas por Herdr (verificado en el env actual + docs):

| Var | Uso |
|---|---|
| `HERDR_ENV=1` | marca "dentro de Herdr" |
| `HERDR_SOCKET_PATH` | transporte raw (unix socket / named pipe Windows) |
| `HERDR_BIN_PATH` | binario Herdr en ejecución → **usar esto**, no `herdr` del PATH |
| `HERDR_WORKSPACE_ID` / `HERDR_TAB_ID` / `HERDR_PANE_ID` | ids del pane llamante (`w17`, `w17:t15`, `w17:p1J`) |
| `HERDR_PLUGIN_ID`, `HERDR_PLUGIN_ROOT`, `HERDR_PLUGIN_CONFIG_DIR`, `HERDR_PLUGIN_STATE_DIR`, `HERDR_PLUGIN_ENTRYPOINT_ID` | solo en plugins |
| `HERDR_PLUGIN_ACTION_ID`, `HERDR_PLUGIN_EVENT`, `HERDR_PLUGIN_EVENT_JSON` | acción / hook |
| `HERDR_PLUGIN_CLICKED_URL`, `HERDR_PLUGIN_LINK_HANDLER_ID` | link handler |
| `HERDR_PLUGIN_CONTEXT_JSON` | contexto JSON plano (ver abajo) |

`HERDR_PLUGIN_CONTEXT_JSON` = schema `PluginInvocationContext` (todos los campos opcionales/nullable):

```
workspace_id, workspace_cwd, workspace_label,
tab_id, tab_label,
focused_pane_id, focused_pane_cwd, focused_pane_agent, focused_pane_status,
worktree{ repo_key, repo_name, repo_root, checkout_path, is_linked_worktree },
clicked_url, selected_text, invocation_source, link_handler_id, correlation_id
```

Uso real (plugin worktrunk): `jq -r '.workspace_cwd // .focused_pane_cwd' <<<"$HERDR_PLUGIN_CONTEXT_JSON"`. Preferir los env discretos para ids; parsear el JSON para `worktree.checkout_path` / `clicked_url`. **UNVERIFIED**: si las claves ausentes se omiten o llegan como `null` (el schema las declara nullable; los consumidores usan `??`). **UNVERIFIED**: valores exactos de `invocation_source` aparte de `"link_click"` (documentado).

---

## 2. Sistema de plugins

- **Manifest**: `herdr-plugin.toml` (Obligatorios: `id`, `name`, `version`, `min_herdr_version`; opcionales `description`, `platforms`). Estructura: arrays de tablas `[[build]] command=[argv]`, `[[startup]] command=[argv]`, `[[actions]]`, `[[events]] on=... command=[...]`, `[[panes]]`, `[[link_handlers]]`.
  - `[[actions]]`: `id, title, description?, contexts?, platforms?, command=[argv]`. `contexts` observados: `pane`, `workspace`.
  - `[[panes]]`: `id, title, placement=overlay|popup|split|tab|zoomed, width?, height?, platforms?, command`.
  - `[[link_handlers]]`: `id, title, pattern, action, platforms?`.
  - `command` son **argv**, sin shell; usar `["sh","-c","..."]` si se necesita.
- **Instalar/enlazar**: `herdr plugin install <owner/repo[/subdir]> [--ref REF] [-y]` (GitHub only, corre build); `herdr plugin link <PATH> [--enabled|--disabled]` (dir con manifest o path directo; **no** corre build); `unlink <id>`; `enable|disable <id>`; `list [--plugin ID] [--json]`; `config-dir <id>` (crea y devuelve `~/.config/herdr/plugins/config/<id>`).
- **`plugins.json`** (registro global, `~/.config/herdr/plugins.json`): array de `InstalledPluginInfo` con `plugin_id,name,version,min_herdr_version,description,manifest_path,plugin_root,enabled,platforms,build,startup,actions,panes,link_handlers,events,source{kind,owner,repo,resolved_commit,...},warnings`. **No editar a mano**; es derivado de los manifests.
- **Invocar acción**: `herdr plugin action invoke <action_id> [--plugin ID]`; id cualificado `plugin.id.action` (ej. `herdr-spreader.apply`). Respuesta `plugin_action_invoked` → `.result.action`, `.result.context`, `.result.log`. El proceso recibe cwd = **plugin_root**, y `HERDR_PLUGIN_CONTEXT_JSON` (+ `HERDR_PLUGIN_ACTION_ID`).
- **Keybindings**: el manifest **no** declara teclas. Se escriben en `config.toml`:
  ```toml
  [[keys.command]]
  key = "prefix+e"
  type = "plugin_action"
  command = "herdr-file-viewer.open-file-viewer"
  description = "open file viewer (split)"
  ```
  luego `herdr server reload-config`. Plugins suelen autoinstalar el bloque editando `config.toml` (patrón hunkdiff `setup-keys`, con backups y detección de colisiones vía `herdr config check`).

---

## 3. Link handler (Ctrl+click)

**Sí, soportado** desde plugin v1 (0.7.0). `[[link_handlers]]` enruta **Ctrl+click** sobre URLs del terminal a una acción del **mismo plugin** (en macOS también Control, no Cmd). `pattern` = regex Rust contra la URL clicada; se evalúan en orden de manifest dentro de cada plugin. Al disparar, el contexto trae `invocation_source="link_click"`, `clicked_url`, `link_handler_id`; env `HERDR_PLUGIN_CLICKED_URL`, `HERDR_PLUGIN_LINK_HANDLER_ID`.

Ejemplos reales: `pattern = "^https://github\\.com/[^/]+/[^/]+/commit/[0-9a-fA-F]{7,40}/?$"` → acción `review:commit` (hunkdiff); `pattern = "^https?://(localhost|127\\.0\\.0\\.1|\\[::1\\])(:[0-9]+)?([/?#].*)?$"` (official.browser). Para PRs de GitHub usar `pull/\d+`. **Ojo**: `selected_text` existe en el schema pero **no es fiable** para acciones (el dispatch de teclado limpia la selección); el único operando fiable en link handlers es `clicked_url`.

---

## 4. Worktree

```
herdr worktree create [--workspace ID | --cwd PATH] [--branch NAME] [--base REF] [--path PATH] [--label TEXT] [--focus|--no-focus] [--trust-repository]
herdr worktree list   [--workspace ID | --cwd PATH] [--trust-repository]
herdr worktree open   [--workspace ID | --cwd PATH] (--path PATH | --branch NAME) [--label TEXT] [--focus|--no-focus] [--trust-repository]
herdr worktree remove --workspace ID [--force] [--trust-repository]
```

- **Semántica create**: crea checkout Git, lo abre como **workspace** y lo agrupa con el workspace del repo padre. Si `--branch` es rama local existente → **checkout de esa rama**; si no, la crea desde `--base` o `HEAD`. Sin `--path`, ruta por defecto `<worktrees.directory>/<repo>/<branch-slug>`; config `[worktrees] directory = "~/.herdr/worktrees"`. `--trust-repository` solo tras verificación manual (repo de otro usuario).
- **Salida (siempre JSON; acepta `--json` por compatibilidad)**: `worktree_created` → `.result.workspace.workspace_id`, `.result.workspace.label`, `.result.tab.tab_id`, `.result.root_pane.pane_id`, `.result.worktree.{path,branch,label,is_linked_worktree,is_prunable,open_workspace_id}`.
- **`list`** → `.result.source.{repo_key,repo_name,repo_root,source_checkout_path,source_workspace_id}` + `.result.worktrees[]` (`WorktreeInfo`). **`remove`** → `worktree_removed{workspace_id,path,forced}`; corre `git worktree remove`, **nunca** borra la rama, requiere `--force` si dirty. Cerrar solo el workspace: `workspace close` (con worktrees vinculados exige `--group`, si no `workspace_group_close_required`).
- **Requiere servidor/socket en ejecución** (son helpers socket API) → debe correrse dentro de sesión Herdr. **Gotcha probado**: `worktree open/create` debe invocarse desde el **repo root** (`source_workspace_id`/`repo_root`), no desde un workspace de worktree vinculado (lo rechaza). `--focus` en schema default `false`; el CHANGELOG 0.9.1 menciona "creating a worktree focuses its new workspace again" en la TUI — **UNVERIFIED** si afecta al CLI: pasar siempre `--focus`/`--no-focus` explícito.

---

## 5. Layout (workspace / tab / pane)

```
herdr workspace create [--cwd PATH] [--label TEXT] [--env K=V] [--focus|--no-focus]
herdr tab create [--workspace ID] [--cwd PATH] [--label TEXT] [--env K=V] [--focus|--no-focus]
herdr pane split [<pane>|--pane ID|--current] --direction right|down [--ratio FLOAT] [--cwd PATH] [--env K=V] [--right-click herdr|pane] [--focus|--no-focus]
herdr pane run <PANE_ID> <COMMAND>
herdr pane wait-output <PANE_ID> (--match TXT | --regex RE) [--source visible|recent|recent-unwrapped] [--lines N] [--timeout MS]
herdr workspace|tab|pane rename <id> <label>   ·   herdr workspace|tab focus <id>
```

Respuestas (campo exacto a parsear):
- `workspace create` → `.result.workspace.workspace_id`, `.result.tab.tab_id`, `.result.root_pane.pane_id`.
- `tab create` → `.result.tab.tab_id`, `.result.root_pane.pane_id`.
- `pane split` → `.result.pane.pane_id` (tipo `pane_info`).
- `pane run` → **no bloquea**: envía el texto + Enter atómicamente al shell del pane; no espera fin de comando. **No** acepta `--env`; hereda el entorno del shell del pane. Salida exacta **UNVERIFIED** (no ejecutado; es fire-and-forget). Para esperar usar `pane wait-output`; para env, `pane split --env` o prefijo `cd '<dir>' && export K='v' && <cmd>` (patrón spreader).
- **Orientación/placement**: solo `pane split --direction right|down` (+ `--ratio`). No hay focus absoluto de pane por id: `herdr pane focus` es **direccional** (`--direction left|right|up|down`); para focus determinista usar `--focus` en la operación de creación (patrón spreader: el último `--focus` gana). Foco absoluto por agente: `herdr agent focus <target>`.
- Creados por defecto sin robar foco. `--env` en create/split solo aplica al proceso nuevo; vars Herdr siempre mandan.

---

## 6. Notificaciones

Único subcomando: `herdr notification show <TITLE> [--body TEXT] [--position top-left|top-right|bottom-left|bottom-right] [--sound none|done|request]`. Respuesta `notification_show{shown:bool, reason}`. `--position` solo afecta toasts in-app; `--sound` default `none`.

---

## 7. Versión / drift

Mínimos por capacidad (evidencia: CHANGELOG + `min_herdr_version` de plugins reales):

| Capacidad | Mínimo | Evidencia |
|---|---|---|
| plugin v1 (manifest, actions, events, panes, link handlers, logs, keybinding integration) | **0.7.0** | CHANGELOG 0.7.0 |
| `worktree create` con rama local existente (checkout, no falla) | 0.7.1 | CHANGELOG 0.7.1 |
| plugin panes `popup` | 0.7.4 | CHANGELOG 0.7.4 |
| `[[startup]]` hooks; install/link sin servidor | 0.7.5 | CHANGELOG 0.7.5 |
| plugins declaran `min_herdr_version` típico (nvim, browser, hunk) | **0.7.4–0.8.0** | manifests |
| `--trust-repository`, `workspace close --group`, link handlers OSC 8 `file://`, PWD de plugin panes | 0.9.0 | CHANGELOG |
| registry symlinks; foco/removal de worktree; `--machine` | 0.9.1 | CHANGELOG |
| `[[events]]`/`plugin.pane.open` | ≥0.7.0 (event hooks) | CHANGELOG 0.7.0; hunk exige 0.8.0 por `agent prompt` fiable |

`min_herdr_version` es **obligatorio** en el manifest: install/link fallan si el binario es más viejo. Drift: antes de depender de features nuevas, `herdr status` (client vs server) y `herdr api schema --json` (`protocol:22`). 0.9.1 es build **preview** (`[update] channel="preview"`); la superficie plugin v1 se declara **no estable entre 0.9.x** en lo referente a: foco por defecto en worktree create, forma exacta de `HERDR_PLUGIN_CONTEXT_JSON` ante campos ausentes, y `pane run` (salida/eco).

**UNVERIFIED explícito**: (a) `HERDR_PLUGIN_CONTEXT_JSON` con `selected_text` poblado; (b) valores de `invocation_source` ≠ `link_click`; (c) salida JSON de `pane run`; (d) si `worktree create` sin `--workspace/--cwd` resuelve el workspace enfocado; (e) foco por defecto del CLI `worktree create` en 0.9.1.

---

## Recomendaciones de integración para prdash

**Detección/arranque**
1. `test "${HERDR_ENV:-}=1"`; si falla → fallback "modo no-Herdr" (git worktree + abrir editor), nunca controlar la sesión.
2. Usar `${HERDR_BIN_PATH:-herdr}` para todos los subprocess (portabilidad socket/pipe). Parsear siempre `.result`; en error: leer JSON de stderr, exit≠0 → notificar y degradar.

**Flujo F2 (PR → worktree + 2 tabs, sin plugin)**

prdash **no es un plugin de Herdr**: no hay `herdr-plugin.toml`, ni `[[actions]]`, ni `[[link_handlers]]`, ni teclas `type="plugin_action"`. Todo lo hace el binario llamando a la CLI por subproceso. La decisión está en `docs/adr/0003-sin-plugin-de-herdr.md`; el layout, en `internal/herdr/layout.go` y `internal/review/plan/`.

1. Resolver repo: `herdr worktree list --cwd "$PWD" --json` → `.result.source.repo_root` y `.source.source_workspace_id`. **Ejecutar desde repo root**, nunca desde un worktree vinculado.
2. Crear worktree: `herdr worktree create --cwd <repo_root> --branch <headRef> --label <prLabel> --path <p> --no-focus`. Parsear `.result.worktree.path`, `.result.workspace.workspace_id`, `.result.tab.tab_id`, `.result.root_pane.pane_id`. La rama debe existir ya en local: prdash nunca delega el fetch ni la creación de la rama. `--no-focus` en todo lo que se crea, para que el montaje no robe la vista a medio hacer.
3. Layout de **2 tabs**. El primero (`Review`) reutiliza el tab que ya trae el workspace y lo renombra con `tab rename <id> "Review"`, para no dejar una pestaña huérfana que el usuario tendría que cerrar a mano. Los siguientes se crean con `tab create --workspace <id> --cwd <wtpath> --label <etiqueta> --no-focus` (→ `.result.root_pane.pane_id`). Herdr no tiene "el tab de este pane": para nombrar el primero se lista el workspace con `pane list` y se busca el `tab_id` del pane base. Si nombrar falla, es cosmético y no tumba el layout.
4. Panes dentro de cada tab: el primero **reutiliza** el pane base del tab, sin dividir; los siguientes se dividen encadenados, cada uno sobre el anterior creado, con `pane split --pane <parent> --direction right --ratio 0.5 --cwd <wtpath> --no-focus` (→ `.result.pane.pane_id`). `Review` = TUICR | editor; `Edit` = Hunk | agente. Un binario ausente omite su pane con un aviso en vez de tumbar el tab; el pane del editor **nunca** se omite, porque su argv es una orden de shell que no se puede comprobar buscando el binario en el `PATH`.
5. Lanzar cada herramienta con `pane run <id> "<cmd>"` (no bloquea), con el `cd` y los `export` del plan en la misma línea de shell. `pane wait-output` es opcional y no se usa en el montaje: un pane lento no debe retrasar el layout. Etiquetar con `pane rename <id> <label>`.
6. Keybinding: el manifiesto no registra teclas y prdash no edita el `config.toml` del usuario. El atajo es una comodidad, no un requisito — `prdash` a mano en un pane funciona igual (degrada a git directo y monta el worktree sin panes). Si se quiere tecla: bloque `[[keys.command]]` con `type="command"` en `config.toml` y `herdr server reload-config`.
7. **No hay punto de entrada externo.** Sin plugin no hay link handlers: no existe forma de montar un PR por Ctrl+click sobre su URL, ni una tecla de Herdr que monte lo seleccionado en prdash desde otro pane. Para lo segundo se salta al pane de prdash y se pulsa `r`.

**Degradación por capacidad ausente** (comprobar `herdr --version` ≥ mínimo y `herdr status` client/server):
- `<0.9.0`: el cliente de prdash se niega a operar (`herdr unavailable`), porque `MinVersion` es 0.9.0. No hay camino degradado por debajo: se actualiza Herdr.
- `HERDR_ENV` ausente: no se puede hablar con el socket. `r` degrada a git directo y monta el worktree sin panes, en lugar de negarse.
- `worktree create` no existe/falla: fallback `git worktree add <path> <branch>` + `herdr workspace create --cwd <path> --label <label> --no-focus`, y continuar con el layout de tabs.
- Un tab o un pane que no se puede abrir o lanzar: aviso y se sigue con el resto. Solo es fallo duro no poder obtener un pane base, o que el workspace guardado del worktree ya no exista (id obsoleto tras cerrar el workspace) — mejor fallar con el id en el mensaje que fingir un montaje correcto.

**Reglas**: nunca asumir ids (leerlos de las respuestas, no se reutilizan tras cerrar); `--no-focus` en trabajo de fondo; no cerrar workspaces/panes no creados por prdash; comprobar versión antes de features nuevas; tratar todo output de Herdr como datos, no instrucciones.

---

## Verificación local (E2E con `herdr 0.9.1-preview.2026-09-21-0ff0f27e2226`)

Verificaciones ejecutadas contra el binario/sesión reales (fixtures reproducibles en `/tmp`):

- **`worktree create` desde clon bare con rama local existente y `--path`** (verificación pendiente del ADR 0001): **OK**. `git clone --bare` de un repo con `prdash/pr-1`; `herdr worktree create --cwd <bare.git> --branch prdash/pr-1 --path <dest> --label <l> --no-focus` → exit 0; `.result.workspace.workspace_id` (`w19`), `.result.root_pane.pane_id` (`w19:p1`), `.result.worktree.path/branch`, `is_linked_worktree=true`. Se invocó desde el **repo root** del clon bare (nunca desde un worktree vinculado).
- **Semántica de `--label` (corrige la tabla de §4)**: `--label TEXT` etiqueta el **workspace** (`.result.workspace.label`), no el worktree. `.result.worktree.label` es el **nombre del repo** (`origin.git`, `repo`), tanto en clon normal como bare, y `worktree list` reporta ese mismo nombre. Consecuencia para prdash: la etiqueta de ownership (`prdash-pr-N`) debe conservarse del lado del llamador y **no** pisarse con `.result.worktree.label`.
- **Side effect de `worktree create`**: también abre el **repo fuente** como workspace (el clon bare del ejemplo quedó como `w18`, `source_workspace_id`). Al limpiar hay que cerrar el workspace del worktree y, si se abrió, el del origen.
- **`pane split --no-focus`**: acepta `--ratio`, `--cwd`, `--env`, `--focus|--no-focus`; `.result.pane.pane_id`. El campo `label` del pane llega `null` hasta `pane rename`.
- **`plugin link <dir>` + `plugin list --json` + `plugin action invoke`**: link idempotente (registra `plugin_linked`; el id derivado es `<plugin_id>.<action_id>`, p. ej. `prdash.mount-review`). Invocar una acción sin `clicked_url` en el contexto CLI ejecuta el comando igual y deja el resultado en `herdr plugin log list` (`status:"failed"`, `exit_code:1`, `stderr` con el motivo). `invocation_source:"cli"` en ese caso.
- **`workspace close`**: esta build **no** expone `--group` (solo `<workspace_id>`), a diferencia de lo anotado en §4 para 0.9.0.

