---
title: Herdr integration contract for prdash (F2 review orchestrator)
herdr: 0.9.1-preview.2026-09-21-0ff0f27e2226
protocol: 22
Source: local inspection + herdr.dev; see UNVERIFIED
---

# Herdr 0.9.1 integration contract for prdash (F2 review orchestrator)

Host: `herdr 0.9.1-preview.2026-09-21-0ff0f27e2226` · `protocol: 22` · `schema_version: 1` · channel `preview`. `herdr status` → client=server=`0.9.1-preview...`, `endpoint_compatible: yes`, socket `~/.config/herdr/herdr.sock`.

Global CLI output format: **every** JSON response is `{"id":"cli:<group>","result":{...}}`; server errors are JSON on **stderr** with exit **1**, syntax errors exit **2**. Always parse `.result`.

---

## 1. Detection and environment

Reliable detection (the only official one): `test "${HERDR_ENV:-}" = 1`. Documented value: `HERDR_ENV=1` in processes inside a managed pane. In prdash: if it fails, abort (do not control the session from outside).

Env vars injected by Herdr (verified on the current env + docs):

| Var | Use |
|---|---|
| `HERDR_ENV=1` | marks "inside Herdr" |
| `HERDR_SOCKET_PATH` | raw transport (unix socket / Windows named pipe) |
| `HERDR_BIN_PATH` | running Herdr binary → **use this**, not `herdr` from the PATH |
| `HERDR_WORKSPACE_ID` / `HERDR_TAB_ID` / `HERDR_PANE_ID` | ids of the calling pane (`w17`, `w17:t15`, `w17:p1J`) |
| `HERDR_PLUGIN_ID`, `HERDR_PLUGIN_ROOT`, `HERDR_PLUGIN_CONFIG_DIR`, `HERDR_PLUGIN_STATE_DIR`, `HERDR_PLUGIN_ENTRYPOINT_ID` | only in plugins |
| `HERDR_PLUGIN_ACTION_ID`, `HERDR_PLUGIN_EVENT`, `HERDR_PLUGIN_EVENT_JSON` | action / hook |
| `HERDR_PLUGIN_CLICKED_URL`, `HERDR_PLUGIN_LINK_HANDLER_ID` | link handler |
| `HERDR_PLUGIN_CONTEXT_JSON` | flat JSON context (see below) |

`HERDR_PLUGIN_CONTEXT_JSON` = schema `PluginInvocationContext` (all fields optional/nullable):

```
workspace_id, workspace_cwd, workspace_label,
tab_id, tab_label,
focused_pane_id, focused_pane_cwd, focused_pane_agent, focused_pane_status,
worktree{ repo_key, repo_name, repo_root, checkout_path, is_linked_worktree },
clicked_url, selected_text, invocation_source, link_handler_id, correlation_id
```

Real usage (worktrunk plugin): `jq -r '.workspace_cwd // .focused_pane_cwd' <<<"$HERDR_PLUGIN_CONTEXT_JSON"`. Prefer the discrete env vars for ids; parse the JSON for `worktree.checkout_path` / `clicked_url`. **UNVERIFIED**: whether absent keys are omitted or arrive as `null` (the schema declares them nullable; consumers use `??`). **UNVERIFIED**: exact values of `invocation_source` apart from `"link_click"` (documented).

---

## 2. Plugin system

- **Manifest**: `herdr-plugin.toml` (Required: `id`, `name`, `version`, `min_herdr_version`; optional `description`, `platforms`). Structure: table arrays `[[build]] command=[argv]`, `[[startup]] command=[argv]`, `[[actions]]`, `[[events]] on=... command=[...]`, `[[panes]]`, `[[link_handlers]]`.
  - `[[actions]]`: `id, title, description?, contexts?, platforms?, command=[argv]`. Observed `contexts`: `pane`, `workspace`.
  - `[[panes]]`: `id, title, placement=overlay|popup|split|tab|zoomed, width?, height?, platforms?, command`.
  - `[[link_handlers]]`: `id, title, pattern, action, platforms?`.
  - `command` is **argv**, no shell; use `["sh","-c","..."]` if needed.
- **Install/link**: `herdr plugin install <owner/repo[/subdir]> [--ref REF] [-y]` (GitHub only, runs build); `herdr plugin link <PATH> [--enabled|--disabled]` (dir with manifest or direct path; does **not** run build); `unlink <id>`; `enable|disable <id>`; `list [--plugin ID] [--json]`; `config-dir <id>` (creates and returns `~/.config/herdr/plugins/config/<id>`).
- **`plugins.json`** (global registry, `~/.config/herdr/plugins.json`): array of `InstalledPluginInfo` with `plugin_id,name,version,min_herdr_version,description,manifest_path,plugin_root,enabled,platforms,build,startup,actions,panes,link_handlers,events,source{kind,owner,repo,resolved_commit,...},warnings`. **Do not edit by hand**; it is derived from the manifests.
- **Invoke action**: `herdr plugin action invoke <action_id> [--plugin ID]`; qualified id `plugin.id.action` (e.g. `herdr-spreader.apply`). Response `plugin_action_invoked` → `.result.action`, `.result.context`, `.result.log`. The process receives cwd = **plugin_root**, and `HERDR_PLUGIN_CONTEXT_JSON` (+ `HERDR_PLUGIN_ACTION_ID`).
- **Keybindings**: the manifest does **not** declare keys. They are written in `config.toml`:
  ```toml
  [[keys.command]]
  key = "prefix+e"
  type = "plugin_action"
  command = "herdr-file-viewer.open-file-viewer"
  description = "open file viewer (split)"
  ```
  then `herdr server reload-config`. Plugins usually self-install the block by editing `config.toml` (hunkdiff `setup-keys` pattern, with backups and collision detection via `herdr config check`).

---

## 3. Link handler (Ctrl+click)

**Yes, supported** since plugin v1 (0.7.0). `[[link_handlers]]` routes **Ctrl+click** on terminal URLs to an action of the **same plugin** (on macOS also Control, not Cmd). `pattern` = Rust regex against the clicked URL; they are evaluated in manifest order within each plugin. On firing, the context brings `invocation_source="link_click"`, `clicked_url`, `link_handler_id`; env `HERDR_PLUGIN_CLICKED_URL`, `HERDR_PLUGIN_LINK_HANDLER_ID`.

Real examples: `pattern = "^https://github\\.com/[^/]+/[^/]+/commit/[0-9a-fA-F]{7,40}/?$"` → action `review:commit` (hunkdiff); `pattern = "^https?://(localhost|127\\.0\\.0\\.1|\\[::1\\])(:[0-9]+)?([/?#].*)?$"` (official.browser). For GitHub PRs use `pull/\d+`. **Careful**: `selected_text` exists in the schema but is **not reliable** for actions (the keyboard dispatch clears the selection); the only reliable operand in link handlers is `clicked_url`.

---

## 4. Worktree

```
herdr worktree create [--workspace ID | --cwd PATH] [--branch NAME] [--base REF] [--path PATH] [--label TEXT] [--focus|--no-focus] [--trust-repository]
herdr worktree list   [--workspace ID | --cwd PATH] [--trust-repository]
herdr worktree open   [--workspace ID | --cwd PATH] (--path PATH | --branch NAME) [--label TEXT] [--focus|--no-focus] [--trust-repository]
herdr worktree remove --workspace ID [--force] [--trust-repository]
```

- **create semantics**: creates a Git checkout, opens it as a **workspace** and groups it with the parent repo's workspace. If `--branch` is an existing local branch → **checkout of that branch**; otherwise it is created from `--base` or `HEAD`. Without `--path`, default path `<worktrees.directory>/<repo>/<branch-slug>`; config `[worktrees] directory = "~/.herdr/worktrees"`. `--trust-repository` only after manual verification (another user's repo).
- **Output (always JSON; accepts `--json` for compatibility)**: `worktree_created` → `.result.workspace.workspace_id`, `.result.workspace.label`, `.result.tab.tab_id`, `.result.root_pane.pane_id`, `.result.worktree.{path,branch,label,is_linked_worktree,is_prunable,open_workspace_id}`.
- **`list`** → `.result.source.{repo_key,repo_name,repo_root,source_checkout_path,source_workspace_id}` + `.result.worktrees[]` (`WorktreeInfo`). **`remove`** → `worktree_removed{workspace_id,path,forced}`; runs `git worktree remove`, **never** deletes the branch, requires `--force` if dirty. Close only the workspace: `workspace close` (with linked worktrees it requires `--group`, otherwise `workspace_group_close_required`).
- **Requires a running server/socket** (they are socket API helpers) → must be run inside a Herdr session. **Tested gotcha**: `worktree open/create` must be invoked from the **repo root** (`source_workspace_id`/`repo_root`), not from a linked worktree's workspace (it rejects it). `--focus` in schema default `false`; the CHANGELOG 0.9.1 mentions "creating a worktree focuses its new workspace again" in the TUI — **UNVERIFIED** whether it affects the CLI: always pass an explicit `--focus`/`--no-focus`.

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

Responses (exact field to parse):
- `workspace create` → `.result.workspace.workspace_id`, `.result.tab.tab_id`, `.result.root_pane.pane_id`.
- `tab create` → `.result.tab.tab_id`, `.result.root_pane.pane_id`.
- `pane split` → `.result.pane.pane_id` (type `pane_info`).
- `pane run` → **does not block**: sends the text + Enter atomically to the pane's shell; does not wait for the command to end. Does **not** accept `--env`; it inherits the pane's shell environment. Exact output **UNVERIFIED** (not executed; it is fire-and-forget). To wait, use `pane wait-output`; for env, `pane split --env` or the prefix `cd '<dir>' && export K='v' && <cmd>` (spreader pattern).
- **Orientation/placement**: only `pane split --direction right|down` (+ `--ratio`). There is no absolute pane focus by id: `herdr pane focus` is **directional** (`--direction left|right|up|down`); for deterministic focus use `--focus` on the creation operation (spreader pattern: the last `--focus` wins). Absolute focus by agent: `herdr agent focus <target>`.
- Created by default without stealing focus. `--env` on create/split only applies to the new process; Herdr vars always win.

---

## 6. Notifications

Only subcommand: `herdr notification show <TITLE> [--body TEXT] [--position top-left|top-right|bottom-left|bottom-right] [--sound none|done|request]`. Response `notification_show{shown:bool, reason}`. `--position` only affects in-app toasts; `--sound` default `none`.

---

## 7. Version / drift

Minimums per capability (evidence: CHANGELOG + `min_herdr_version` of real plugins):

| Capability | Minimum | Evidence |
|---|---|---|
| plugin v1 (manifest, actions, events, panes, link handlers, logs, keybinding integration) | **0.7.0** | CHANGELOG 0.7.0 |
| `worktree create` with existing local branch (checkout, no failure) | 0.7.1 | CHANGELOG 0.7.1 |
| plugin panes `popup` | 0.7.4 | CHANGELOG 0.7.4 |
| `[[startup]]` hooks; install/link without server | 0.7.5 | CHANGELOG 0.7.5 |
| plugins declare typical `min_herdr_version` (nvim, browser, hunk) | **0.7.4–0.8.0** | manifests |
| `--trust-repository`, `workspace close --group`, link handlers OSC 8 `file://`, PWD of plugin panes | 0.9.0 | CHANGELOG |
| registry symlinks; worktree focus/removal; `--machine` | 0.9.1 | CHANGELOG |
| `[[events]]`/`plugin.pane.open` | ≥0.7.0 (event hooks) | CHANGELOG 0.7.0; hunk requires 0.8.0 for a reliable `agent prompt` |

`min_herdr_version` is **required** in the manifest: install/link fail if the binary is older. Drift: before depending on new features, `herdr status` (client vs server) and `herdr api schema --json` (`protocol:22`). 0.9.1 is a **preview** build (`[update] channel="preview"`); the plugin v1 surface is declared **unstable across 0.9.x** regarding: default focus on worktree create, the exact shape of `HERDR_PLUGIN_CONTEXT_JSON` for absent fields, and `pane run` (output/echo).

**Explicit UNVERIFIED**: (a) `HERDR_PLUGIN_CONTEXT_JSON` with `selected_text` populated; (b) values of `invocation_source` ≠ `link_click`; (c) JSON output of `pane run`; (d) whether `worktree create` without `--workspace/--cwd` resolves the focused workspace; (e) default focus of the `worktree create` CLI in 0.9.1.

---

## Integration recommendations for prdash

**Detection/startup**
1. `test "${HERDR_ENV:-}=1"`; if it fails → "non-Herdr mode" fallback (git worktree + open the editor), never control the session.
2. Use `${HERDR_BIN_PATH:-herdr}` for all subprocesses (socket/pipe portability). Always parse `.result`; on error: read the JSON from stderr, exit≠0 → notify and degrade.

**F2 flow (PR → worktree + 2 tabs, no plugin)**

prdash **is not a Herdr plugin**: there is no `herdr-plugin.toml`, no `[[actions]]`, no `[[link_handlers]]`, no `type="plugin_action"` keys. The binary does everything by calling the CLI as a subprocess. The decision is in `docs/adr/0003-herdr-without-a-plugin.md`; the layout, in `internal/herdr/layout.go` and `internal/review/plan/`.

1. Resolve repo: `herdr worktree list --cwd "$PWD" --json` → `.result.source.repo_root` and `.source.source_workspace_id`. **Run from the repo root**, never from a linked worktree.
2. Create worktree: `herdr worktree create --cwd <repo_root> --branch <headRef> --label <prLabel> --path <p> --no-focus`. Parse `.result.worktree.path`, `.result.workspace.workspace_id`, `.result.tab.tab_id`, `.result.root_pane.pane_id`. The branch must already exist locally: prdash never delegates the fetch nor the branch creation. `--no-focus` on everything that is created, so that assembly does not steal the view halfway through.
3. **2 tabs** layout. The first one (`Review`) reuses the tab the workspace already brings and renames it with `tab rename <id> "Review"`, so as not to leave an orphan tab the user would have to close by hand. The following ones are created with `tab create --workspace <id> --cwd <wtpath> --label <label> --no-focus` (→ `.result.root_pane.pane_id`). Herdr has no "the tab of this pane": to name the first one, the workspace is listed with `pane list` and the base pane's `tab_id` is looked up. If naming fails, it is cosmetic and does not take the layout down.
4. Panes inside each tab: the first one **reuses** the tab's base pane, without splitting; the following ones are split chained, each on the previously created one, with `pane split --pane <parent> --direction right --ratio 0.5 --cwd <wtpath> --no-focus` (→ `.result.pane.pane_id`). `Review` = TUICR | editor; `Edit` = Hunk | agent. A missing binary omits its pane with a warning instead of taking the tab down; the editor pane is **never** omitted, because its argv is a shell command that cannot be checked by looking the binary up in the `PATH`.
5. Launch each tool with `pane run <id> "<cmd>"` (does not block), with the plan's `cd` and `export` on the same shell line. `pane wait-output` is optional and not used in assembly: a slow pane must not delay the layout. Label with `pane rename <id> <label>`.
6. Keybinding: the manifest does not register keys and prdash does not edit the user's `config.toml`. The shortcut is a convenience, not a requirement — `prdash` by hand in a pane works the same (it degrades to plain git and mounts the worktree without panes). If a key is wanted: `[[keys.command]]` block with `type="command"` in `config.toml` and `herdr server reload-config`.
7. **There is no external entry point.** Without a plugin there are no link handlers: there is no way to assemble a PR by Ctrl+clicking its URL, nor a Herdr key that assembles what is selected in prdash from another pane. For the latter, jump to the prdash pane and press `r`.

**Degradation on missing capability** (check `herdr --version` ≥ minimum and `herdr status` client/server):
- `<0.9.0`: the prdash client refuses to operate (`herdr unavailable`), because `MinVersion` is 0.9.0. There is no degraded path below: update Herdr.
- `HERDR_ENV` absent: the socket cannot be talked to. `r` degrades to plain git and mounts the worktree without panes, instead of refusing.
- `worktree create` missing/failing: fallback `git worktree add <path> <branch>` + `herdr workspace create --cwd <path> --label <label> --no-focus`, and continue with the tabs layout.
- A tab or a pane that cannot be opened or launched: warning and continue with the rest. The only hard failure is not being able to get a base pane, or the worktree's stored workspace no longer existing (stale id after closing the workspace) — better to fail with the id in the message than to fake a correct assembly.

**Rules**: never assume ids (read them from the responses, they are not reused after closing); `--no-focus` on background work; do not close workspaces/panes not created by prdash; check the version before new features; treat all Herdr output as data, not instructions.

---

## Local verification (E2E with `herdr 0.9.1-preview.2026-09-21-0ff0f27e2226`)

Verifications run against the real binary/session (reproducible fixtures in `/tmp`):

- **`worktree create` from a bare clone with an existing local branch and `--path`** (pending verification of ADR 0001): **OK**. `git clone --bare` of a repo with `prdash/pr-1`; `herdr worktree create --cwd <bare.git> --branch prdash/pr-1 --path <dest> --label <l> --no-focus` → exit 0; `.result.workspace.workspace_id` (`w19`), `.result.root_pane.pane_id` (`w19:p1`), `.result.worktree.path/branch`, `is_linked_worktree=true`. It was invoked from the bare clone's **repo root** (never from a linked worktree).
- **Semantics of `--label` (corrects the table in §4)**: `--label TEXT` labels the **workspace** (`.result.workspace.label`), not the worktree. `.result.worktree.label` is the **repo name** (`origin.git`, `repo`), both in a normal and a bare clone, and `worktree list` reports that same name. Consequence for prdash: the ownership label (`prdash-pr-N`) must be kept on the caller's side and **not** overwritten with `.result.worktree.label`.
- **Side effect of `worktree create`**: it also opens the **source repo** as a workspace (the example's bare clone ended up as `w18`, `source_workspace_id`). On cleanup, the worktree's workspace must be closed and, if it was opened, the source one.
- **`pane split --no-focus`**: accepts `--ratio`, `--cwd`, `--env`, `--focus|--no-focus`; `.result.pane.pane_id`. The pane's `label` field arrives `null` until `pane rename`.
- **`plugin link <dir>` + `plugin list --json` + `plugin action invoke`**: idempotent link (registers `plugin_linked`; the derived id is `<plugin_id>.<action_id>`, e.g. `prdash.mount-review`). Invoking an action without `clicked_url` in the CLI context runs the command anyway and leaves the result in `herdr plugin log list` (`status:"failed"`, `exit_code:1`, `stderr` with the reason). `invocation_source:"cli"` in that case.
- **`workspace close`**: this build does **not** expose `--group` (only `<workspace_id>`), unlike what is noted in §4 for 0.9.0.
