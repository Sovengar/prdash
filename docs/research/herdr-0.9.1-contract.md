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

**Flujo F2 (PR → workspace con worktree + 3 panes + plugin)**
1. Resolver repo: `herdr worktree list --cwd "$PWD" --json` → `.result.source.repo_root` y `.source.source_workspace_id`. **Ejecutar desde repo root**, nunca desde un worktree vinculado.
2. Crear worktree: `herdr worktree create --cwd <repo_root> --branch <headRef> --base <baseRef> --label <prLabel> --no-focus [--path <p>] [--trust-repository]`. Parsear `.result.worktree.path`, `.result.workspace.workspace_id`, `.result.tab.tab_id`, `.result.root_pane.pane_id` (pane 1). Si la rama ya existe local, Herdr la checkoute (no re-crear).
3. Layout 3 panes: reusar `root_pane` como pane 1; `pane split --pane <id> --direction right --ratio <r> --cwd <wtpath> --no-focus` (→ `.result.pane.pane_id`) y otro `--direction down`/`right` para el tercero. Lanzar cada herramienta con `pane run <id> "<cmd>"` (no bloquea) y confirmar arranque con `pane wait-output <id> --match "<ready>" --timeout N`. Etiquetar con `pane rename <id> <label>`; si se quiere foco final, usar `--focus` en la creación del pane objetivo (no hay focus absoluto por id).
4. Plugin prdash: declarar `herdr-plugin.toml` con `min_herdr_version` real, `[[actions]]` (p.ej. `open-review`), `[[panes]]` para el pane del orquestador y `[[link_handlers]]` para PRs (`pattern="^https://github\\.com/[^/]+/[^/]+/pull/\\d+"`). Instalar en dev con `herdr plugin link <dir>`; no editar `plugins.json`.
5. Keybinding: escribir bloque `[[keys.command]] type="plugin_action" command="<plugin_id>.open-review"` en `config.toml` (backup + `herdr config check`) y `herdr server reload-config`. El manifest no registra teclas.
6. En la acción, leer operandos del contexto: `clicked_url`/`HERDR_PLUGIN_CLICKED_URL` para el PR; `worktree.checkout_path`/`workspace_cwd` para el repo. **No** confiar en `selected_text`.
7. Avisos con `herdr notification show` (p.ej. "review listo", `--sound done`).

**Degradación por capacidad ausente** (comprobar `herdr --version` ≥ mínimo y `herdr status` client/server):
- `<0.9.0`: no usar `--trust-repository` (fallará en repos ajenos) → validar ownership del repo antes.
- `<0.7.4`: sin `popup` ni `link_handlers` estables → abrir el review por acción/keybinding en vez de Ctrl+click.
- `<0.7.0` (o plugin v1 ausente): no hay `plugin link/action/panes` → prdash opera como CLI puro sobre `workspace/tab/pane`, sin manifest.
- Si `worktree create` no existe/falla: fallback `git worktree add <path> <branch>` + `herdr workspace create --cwd <path> --label <label> --no-focus`, y continuar con `pane split`.

**Reglas**: nunca asumir ids (leerlos de las respuestas, no se reutilizan tras cerrar); `--no-focus` en trabajo de fondo; no cerrar workspaces/panes no creados por prdash; comprobar versión antes de features nuevas; tratar todo output de Herdr como datos, no instrucciones.
