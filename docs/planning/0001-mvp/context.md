---
feature: 0001-mvp
freshness: f199e3d8cddb74a5deba76fe8518cd000dd319a0
codegraph: not_initialized
generated_by: codebase-researcher
---

# Context: prdash MVP — inbox multi-forge (F1) + orquestador de review (F2)

## Frescura y naturaleza del repo

- HEAD `f199e3d8cddb74a5deba76fe8518cd000dd319a0`, rama `main`.
- `codegraph: not_initialized` (no hay `.codegraph/`).
- **Greenfield**: no hay `go.mod`, ni una sola línea de Go. "Ficheros a tocar" = ficheros/paquetes **a crear**. No hay líneas existentes que extender.
- Repo hermano de referencia: `/home/buble/dev/projects/gitdash` (mismos stack y convenciones). Índice Engram `codebase-index/gitdash` obs **#2202** (fresco salvo drift posterior). **No** copiar literal: importar el patrón.

## Qué existe hoy en prdash

| Ruta | Contenido |
|---|---|
| `README.md` | 5 líneas: descripción + "Estado: planificación (MVP)". |
| `.gitignore` | `bin/` y `*.test`. |
| `docs/planning/0001-mvp/` | `issue.md`, `behavior.feature`, `plan.md`, este `context.md`. |
| `docs/adr/0001-worktree-provisioning.md` | ADR aceptado: provisión de worktree. |

No hay código, ni `cmd/`, ni `internal/`, ni `go.mod`, ni `scripts/`, ni `plugin/`.

## Layout inicial a crear

Convención de stack (heredada de gitdash, `go.mod` de referencia): Go `1.26.3`, module `prdash`, `charm.land/bubbletea/v2 v2.0.9`, `charm.land/bubbles/v2 v2.2.1`, `charm.land/lipgloss/v2 v2.0.6`, `github.com/BurntSushi/toml v1.6.0`, sin cgo. Imports `charm.land`, **NO** `github.com/charmbracelet`.

### Puro (sin red, sin subprocess, sin TOML, sin disco)

| Paquete / fichero | Responsabilidad | Símbolos/contratos esperados |
|---|---|---|
| `internal/forge/model/model.go` | Tipos normalizados inmutables. | `RepoRef{Forge,Host,Project,Owner,Name}`; `Item{Section,Forge,Host,Ref,Number,Title,Author,SourceBranch,TargetBranch,URL,State,ReviewDecision,Checks,UpdatedAt}`; `Section` (authored/review/mentions); `Warning{Forge,Section,Kind,Msg}`; identidad = `With(forge,host,project,number)`. |
| `internal/forge/parse/parse.go` | Traduce JSON de `gh`/`glab` (GraphQL, REST, Todos) a `model`. | `ParseGHGraphQLSearch`, `ParseGHAuthored`, `ParseGLGraphQL`, `ParseGLMRList`, `ParseGLTodos`, `ParseGHChecks`; **una función por forma de salida**, con fixtures string. Nunca lanza: devuelve items + error de parseo tipado. |
| `internal/inbox/inbox.go` | Consolida 3 secciones, deduplica y decide relevancia. | `Build(inputs []ForgeResult) Inbox`; regla de autoridad de sección (authored > review > mentions); dedupe por `RepoRef`+número. Sin red/disco. |
| `internal/state/state.go` | Estado derivado con precedencia y score de orden (atención primero), compartido por TUI y `--print`. | `type State int` + consts, `String()`, `Derive(item) State`, `Score() int`. Precedencia a definir (p. ej. `error > changes-requested > review-required > approved > pending > merged/closed > draft`). |
| `internal/review/plan/plan.go` | Dado `(Item, Worktree, Entorno)` produce el plan de panes sin tocar Herdr. | `Pane{Cwd,Argv,Label,Env,Kind}`; `Plan(toolArgs ToolArgs, wt Worktree, env Env, pr Item) Plan`. |

### I/O (adapters y puertos)

| Paquete / fichero | Responsabilidad | Símbolos/contratos esperados |
|---|---|---|
| `internal/config/config.go` | TOML XDG; `Load()` nunca falla (defaults + warning). | `Load() (Config,string)`, `LoadFrom(path)`, `Path()`, `Defaults()`, `expandAll()`, `Keybindings`, `Commands`, `HintBarLines()`. Ver §Config. |
| `internal/forge/forge.go` | Contrato `Adapter` + registro por forge. | `Adapter interface { Forge() string; Host(); Auth(ctx) AuthState; Authored(ctx) ([]model.Item,[]model.Warning); ReviewRequested(ctx); Mentions(ctx); ItemState(ctx,RepoRef,number); Approve(ctx,…); Merge(ctx,…) }`. **Nunca devuelve error duro**: ítems + `[]Warning`. |
| `internal/forge/github/github.go` | Adapter GitHub vía `gh` (GraphQL + REST). | Implementa `Adapter`; `exec.CommandContext("gh",…)`; parsea con `forge/parse`. |
| `internal/forge/gitlab/gitlab.go` | Adapter GitLab self-managed vía `glab` (GraphQL + REST + Todos). | Implementa `Adapter`; base URL configurable con subfolder `/git/api/v4/`. |
| `internal/forge/bitbucket/bitbucket.go` | Adapter registrado, **sin red**. | `Adapter` que responde `Warning{Kind:"unsupported"}` en todos los métodos; compila y pasa la suite de conformidad. |
| `internal/reporesolver/reporesolver.go` | **Único dueño del namespace de rutas**: índice remoto→local sobre `roots`, memoria de rutas, clon bare, fetch del ref de review, rama local de trabajo. NO llama a la API del forge. | `Resolver interface { ResolveLocal(RepoRef) (path, bool); EnsureBare(RepoRef) (path,error); FetchReviewRef(path, Item) (branch string, error); Remember(RepoRef,path) }`. |
| `internal/worktree/worktree.go` | Puerto de provisión con 2 implementaciones intercambiables. | `Provisioner interface { Create(Spec) (Worktree,error); Remove(id); List() []Worktree }`; `Spec{Cwd,Branch,Path,Label}`. Implementaciones: `herdrNative` (dentro de Herdr) y `gitDirect` (`git worktree add`). El llamador no sabe cuál corre. |
| `internal/herdr/herdr.go` | **Único** lugar que lee `HERDR_ENV` y parsea salida de Herdr. | `Port interface { Available() bool; WorktreeCreate(Spec) (Worktree,error); PaneSplit/Workspace/Tab…; Layout(plan.Plan) error; Notify(title string); LinkHandler(url) }`. Degradación fuera de Herdr. |
| `internal/review/executor/executor.go` | Aplica el plan usando los puertos. | `Mount(item, resolver, worktree, herdr) Result`; orquesta resolve→fetch→branch→provision→layout. Solo habla por puertos. |
| `internal/cache/cache.go` | Snapshot del inbox + rutas recordadas; corrupto = silencioso. | `Load(path) (File,bool)`, `Save(path, File)`; `FileName = "inbox.json"`, `DirName = "prdash"`; `version` para invalidar. |
| `internal/tui/app.go` | Modelo bubbletea + pipelines de fondo + event pump. | `Model`, `New(cfg)`, `Init()`, `Update`, `View`, `waitForEvent`, `withPump`, `startRefreshCmd`, `tickCmd`, `Msg` types. |
| `internal/tui/update.go` | `Update`/teclas/refresco/detalle. | `handleKey`, `actionForKey`, `View()` (alt-screen), render de 3 secciones. |
| `internal/tui/table.go` | Filas/orden/celdas por sección. | `row{item,state}`, `rows()`, `cells` que devuelven `(texto,style)`. `pad()` ANTES de estilo. |
| `internal/tui/detail.go` | Detalle del ítem. | `renderDetail`. |
| `internal/tui/styles.go` | Estilos lipgloss. | consts/vars. |
| `internal/testutil/testutil.go` | Fixtures: repos git reales + fakes de forge/Herdr. | Ver §Tests. |
| `cmd/prdash/main.go` | Entrypoint: TUI + dispatch de subcomandos. | `main()`; `--print`; subcomandos `herdr …` que consume el plugin. |
| `cmd/prdash/print.go` | Modo `--print` one-shot (tabwriter, mismo orden que TUI). | `runPrint(cfg)`. |
| `cmd/prdash/herdr.go` | Subcomandos invocados por el manifiesto del plugin (pane entrypoint, acción montar review, link handler). | `runHerdrInbox`, `runHerdrMount`, `runHerdrLink`. |
| `plugin/herdr/herdr-plugin.toml` | Manifiesto Herdr 0.9.x. | Pane del inbox (placement), acción "montar review" con keybind, link handler de URLs de PR; versión mínima. Ver §Contratos. |

## Patrones de gitdash a seguir (rutas reales)

| Concern | Fichero modelo | Qué importar de concepto |
|---|---|---|
| Config que nunca falla | `gitdash/internal/config/config.go:81-149` (`Load`/`LoadFrom`) y `Defaults()` `:196-213` | Fichero ausente → defaults silenciosos; TOML roto → defaults + warning string, nunca panic. `expandAll` `:216-228` expande `~`. Keybindings/Commands merge sobre defaults. |
| Parseo puro + fixtures | `gitdash/internal/gitstatus/parse.go:126` (`ParsePorcelain`), `:237` (`ParseWorktrees`) | Parseo en fichero separado del I/O, sin tocar red/subprocess; tests con fixtures string. En prdash: `forge/parse`. |
| Estado derivado + score | `gitdash/internal/gitstatus/parse.go:43-125` (`State`, `Derive`, `Score`) | Enum de estados con precedencia explícita y `Score()` para orden atención-primero compartido por TUI y `--print`. |
| Snapshot con error embebido | `gitdash/internal/gitstatus/status.go:23-55` (`Snapshot`, `State`) | El resultado lleva `Err` dentro; nunca falla duro. En prdash: `Warning` por forge/sección. |
| Pool concurrente con emisión | `gitdash/internal/gitstatus/status.go:126-154` (`StreamPool`) | Semáforo + goroutines que emiten por callback; patrón para paralelizar consultas por forge sin bloquear la UI. |
| Subprocess con LC_ALL=C | `gitdash/internal/gitstatus/status.go:202-267` (`gitEnv`, `runGit`, `runGitCombined`) | Forzar locale inglés para reconocer mensajes de error de `gh`/`glab` igual; timeout vía `exec.CommandContext`. |
| Event pump | `gitdash/internal/tui/app.go:29-82` (msgs), `:215-235` (`waitForEvent`/`sendEvent`/`tickCmd`), `gitdash/internal/tui/update.go:117-121` (`withPump`) | Cada `tea.Cmd` lee UN evento del canal; SIEMPRE re-armar `waitForEvent` en `Update`. Gotcha #1. |
| Pipeline de fondo | `gitdash/internal/tui/app.go:242-267` (`startScanCmd`), `:292` (`fetchBatchCmd`), `:343` (`startActionCmd`) | Goroutine + `sendEvent(ctx,…)`; guard "una operación a la vez"; refresco automático con `tea.Tick` (`tickCmd`). |
| Refresco que no pisa acción | `gitdash/internal/tui/update.go:15-115` (dispatcher por `Msg`) | Merge de resultados por clave sin revertir el estado de una acción en curso (escenario "un refresco no pisa una acción en curso"). |
| Estado UI persistido | `gitdash/internal/state/state.go:26-100` (`Store`, `SaveCollapsed`/`LoadCollapsed` atómico tmp+rename) | Store en `$XDG_STATE_HOME/prdash`. Corrupto/ausente = silencioso. |
| Cache instantánea | `gitdash/internal/cache/cache.go:55-110` (`Load`/`Save`, `version`) | Snapshot JSON para pintar al arrancar; validación best-effort; corrupto silencioso. |
| Modo `--print` | `gitdash/cmd/gitdash/print.go:27-111` (`runPrint`) | Reutiliza config+colección, tabwriter, **mismo orden** que la TUI (via `Score()`). |
| Fixtures de repos git | `gitdash/internal/testutil/testutil.go:13-172` (`Init`, `InitBare`, `AddUpstream`, `MakeWorktree`, `NewBranch`, `FetchLocal`) | Repos git reales en `t.TempDir()`; base para fixtures de worktree/fork. |
| Entrypoint | `gitdash/cmd/gitdash/main.go:20` (`main`) | `config.Load()` → warn a stderr → `tui.New(cfg)` + `tea.NewProgram`; flag `--print` → `runPrint`. |
| Agrupación/headers de tabla | `gitdash/internal/group/group.go:39-119` (`Arrange`) | Arreglo a 2 niveles con headers; análogo a las 3 secciones del inbox. |
| Celdas que devuelven (texto, estilo) | `gitdash/internal/tui/table.go` (funcs `*Cell`), `:render*` | `pad(texto)` ANTES de aplicar estilo (ANSI rompe el ancho). Gotcha de tabla. |
| Smoke TUI | convención gitdash (`AGENTS.md` Gotcha 3) | `tmux` + `capture-pane`; `script` NO sirve (bubbletea v2 bloquea el primer render). |

## Config TOML a definir (claves + defaults)

`config.toml` en `$XDG_CONFIG_HOME/prdash/config.toml`. Fichero ausente → defaults silenciosos.

| Clave | Tipo | Default | Uso |
|---|---|---|---|
| `roots` | `[]string` | `["~/dev"]` | Roots donde buscar clones locales (patrón gitdash). |
| `refresh_interval` | duration string | `"60s"` | Cadencia del auto-refresco. `0` = solo manual. |
| `forges` | tabla/array | habilitados github + gitlab | Ver subclaves. |
| `forge.github.host` | string | `"github.com"` | Host GitHub. |
| `forge.gitlab.host` | string | `"gitlab.example.com"` | Host GL self-managed. |
| `forge.gitlab.api_base` | string | `"/git/api/v4/"` | **Subfolder** obligatorio en GL self-managed. |
| `forge.gitlab.token_env` | string | (implícito, lo maneja glab) | No duplicar credenciales: `glab`/`gh` ya autenticados. |
| `forge.bitbucket.enabled` | bool | `false` | Adapter solo-interfaz; sin red. |
| `data_dir` / `clone_dir` | string | `~/.local/share/prdash/repos/<forge>/<host>/<owner>/<repo>` | Clon bare (configurable). |
| `worktree_dir` | string | bajo `data_dir` | Destino de worktrees (configurable). |
| `tools.tuicr` | string (argv) | `"tuicr"` | Binario/argv de TUICR. |
| `tools.hunk` | string (argv) | `"hunk"` | Binario/argv de Hunk. |
| `tools.agent` | string (argv) | `"opencode"` | Binario/argv del agente. |
| `tools.gh` / `tools.glab` | string | `"gh"` / `"glab"` | Binarios de forge (override). |
| `autoreview.enabled` | bool | `false` | F3 post-MVP; solo parseo, dry-run por defecto. |
| `autoreview.allowlist` | `[]string` | `[]` | Repos (`host/owner/repo`) permitidos para auto-approve (F3). |
| `keybindings` | map | ver abajo | Merge sobre defaults. |
| `commands` | map | `gh`/`glab` argv base | Merge sobre defaults. |

Keybinds sugeridos (merge sobre defaults, patrón `DefaultKeybindings` de gitdash): `quit=q`, `refresh=r`, `detail=enter`, `mount-review=m` (necesita Herdr), `approve=a`, `merge=M`, `section-next=tab`, `open-browser=o`.

## Contratos con herramientas externas (comandos EXACTOS)

**GitHub vía `gh`** (usuario autenticado `Sovengar`, scopes gist/read:org/repo/workflow; `gh` 2.67.0):
- Inbox rico (reviewDecision + checks): `gh api graphql` con `search(type:ISSUE, query:"is:pr is:open author:@me")`. **NO** usar `gh search prs` para el inbox rico (JSON limitado).
- Variantes de query: `author:@me`, `review-requested:@me`, `assignee:@me`, `mentions:@me` (menciones) — **todas vía GraphQL**.
- Ref de PR (fork): `refs/pull/<N>/head`.
- Estado de checks: campos GraphQL (`reviewDecision`) + `statusCheckRollup`/check runs.
- Rate limit: GraphQL ~5000 pts/h; respetar cabeceras y backoff.

**GitLab vía `glab`** (gitlab.com SIN token → 401; self-managed `gitlab.example.com` autenticado como `<usuario>`; `glab` 1.119.0):
- REST bajo subfolder **`/git/api/v4/`** (glab lo maneja; base URL configurable).
- Inbox: `glab api graphql` con `currentUser` → `authored` / `reviewRequested` / `assigned`. Alternativa REST `/merge_requests?scope=…`.
- Menciones: Todos API `glab api <base>/todos?action=mentioned`.
- `glab mr list -F json` = `BasicMergeRequest` (**sin pipeline**, 1 página, sin `--paginate`) → insuficiente para estado rico; usar GraphQL.
- Ref de MR: `refs/merge-requests/<N>/head`.
- Fallos esperados: `401` gitlab.com → tratar como forge no disponible (no vaciar inbox); sin permisos de approve/merge en el self-managed → deshabilitar acción con motivo.

**tuicr** (ya sube reviews reales vía gh/glab/bkt/az):
- `tuicr pr <number|owner/repo#N|URL>` (alias `mr`).
- `tuicr review list|comments|add` → JSON; poll ~30s, sin push.
- MVP: delegar approve/comentar en tuicr cuando aplique; usar gh/glab directo para approve/merge rápido del inbox.

**hunk**:
- `hunk session review|navigate|reload|comment add|list --type user` → JSON.
- `hunk skill path` (descubrir binario/path).
- Binario ausente = pane omitido con aviso.

**herdr** (solo si `HERDR_ENV=1`):
- `herdr worktree create --cwd <repo> --branch <local> --path <dest> --label <prdash-…> --no-focus`; también `open`/`list`/`remove`.
- `herdr workspace|tab create`; `herdr pane split --cwd --ratio --no-focus`; `pane run`; `pane read`; `pane wait-output`.
- `herdr agent start <name> --kind opencode --pane ID`; `agent prompt --wait`; `agent read/wait/send-keys`.
- `herdr notification show <title> --sound request`.
- Plugin: `herdr-plugin.toml` → panes placement overlay/popup/split/tab, acciones con keybind, link handlers Ctrl+click, eventos; env `HERDR_BIN_PATH`/`HERDR_PLUGIN_*`. Docs: herdr.dev/docs. Target Herdr 0.9.x.
- **argv de todas las herramientas es configurable** (no API): drift de flags se absorbe en config.

**git** (provisión y refs):
- Resolución de repo: `git -C <root> remote get-url origin` para construir el índice remoto→local.
- Clon bare: `git clone --bare <url> <dest>`.
- Fetch de ref de review: `git fetch origin refs/pull/<N>/head:refs/heads/prdash/pr-<N>` (GH) o `refs/merge-requests/<N>/head` (GL).
- Rama local de trabajo + worktree fallback: `git worktree add <dest> <rama-local>`.
- Forzar `LC_ALL=C` en el env de los subprocess para parsear errores (patrón gitdash `gitEnv`).

## Estrategia de paginación y refresco (sin tope)

- **Sin capar** secciones: paginar hasta agotar.
- GitHub GraphQL: `pageInfo { hasNextPage endCursor }` hasta `hasNextPage=false`.
- GitLab: iteración de páginas GraphQL/REST (o `glab api --paginate`).
- **Carga progresiva**: primer render = primera página de cada sección (rápido); páginas restantes en segundo plano sin bloquear UI ni refresco. Indicador "cargando más…" por sección.
- **Refresco incremental**: reutilizar snapshot en `cache`; refrescar primera página + comparar por cursor.
- **Backoff** + respeto de cabeceras de rate limit; `401` de gitlab.com = forge no disponible.
- **Pausar auto-refresco** mientras hay una acción en curso o se está paginando.
- Indicador "última actualización" **por forge** (una forge lenta no debe mentir sobre el resto).

## Tests e infra

- Runner: `go build ./... && go vet ./... && go test ./...`.
- **Fixtures string** para `forge/parse` (payloads GraphQL/REST/Todos reales recortados → `model.Item`). Patrón gitdash `parse_test.go`.
- **Fakes de forge/Herdr**: implementaciones en memoria del contrato `Adapter`, `Port`, `Provisioner` para tests de `inbox`, `review/executor`, `state` y `tui` (sin red ni subprocess).
- **Fixtures de repos git reales** en `t.TempDir()` para `reporesolver`/`worktree`: reutilizar helpers gitdash (`Init`, `InitBare`, `AddUpstream`, `MakeWorktree`, `NewBranch`, `FetchLocal`).
- **Tests de modelo directo** (construir `Model`, enviar msgs con `Update`, inspeccionar estado) — sin teatest. Patrón `gitdash/internal/tui/app_test.go`.
- **Smoke TUI con tmux** (`capture-pane`); `script` NO sirve.
- Mapeo de escenarios sugerido: secciones/dedupe/estado rico → `inbox` + `parse`; degradación por forge → `forge/*` (401, Bitbucket) ; worktree/fork/reuso → `reporesolver` + `worktree` con repos reales; layout fallback → `review/plan` + `herdr` (fakes); refresh/paginación → `tui` + `state`.

## Convenciones y fronteras de módulo

- Comentarios de código en **español**, **sin** referencias a specs/IDs de requisito/escenarios (el código es la fuente de verdad). No crear artefactos SDD dentro del repo más allá de `docs/planning/` ya existente.
- **Fronteras no cruzables** (plan §Módulos): `inbox`/`parse` no tocan red ni disco; los adapters no tocan git/worktree/TUI; `reporesolver` no toca la API del forge; `herdr` no toca git ni TUI y es el **único** que lee `HERDR_ENV`; `review` solo habla por puertos.
- Adapters devuelven **items + warnings tipados, nunca error duro**.
- Celdas de tabla devuelven `(texto, estilo)`; `pad()` antes de estilo.
- Estado derivado con precedencia explícita y `Score()` para el orden atención-primero (compartido TUI/`--print`).
- SUBPROCESS: siempre `exec.CommandContext` con timeout y `LC_ALL=C`.
- Config única XDG; adapters y TUI no leen entorno/TOML (el plugin usa el mismo TOML).

## Puntos de integración no obvios / gotchas

1. **Subfolder GitLab self-managed**: el REST vive bajo `/git/api/v4/`; cualquier URL construida a mano debe incluirlo. Encapsulado tras `forge/gitlab`.
2. **`HERDR_ENV` aislado**: solo `herdr` lo lee; el resto recibe entorno inyectado (permite testear la TUI en modo "fuera de Herdr" sin variables globales).
3. **Ownership de rutas**: solo `reporesolver` decide dónde viven clon bare y worktrees; ningún otro módulo construye rutas. `cache` recuerda rutas ya resueltas.
4. **argv configurable** de tuicr/hunk/agente: son datos del plan, no APIs; un cambio de flags se absorbe en config sin tocar código.
5. **No re-entrada del pane del plugin**: el pane entrypoint del manifiesto NO debe relanzar la TUI completa dentro de sí misma; dispatcha a un subcomando que imprime/atiende una unidad de trabajo (pane del inbox o acción).
6. **Event pump**: rearmar `withPump` tras cada evento consumido o el inbox no pinta (Gotcha #1 gitdash).
7. **Ref de fork ≠ rama de origin**: hay que `fetch` explícito de `refs/pull/N/head` / `refs/merge-requests/N/head` y crear rama local **antes** de pedir el worktree al nativo de Herdr.
8. **Detección de "vacío" vs "error"**: nunca mostrar "vacío" si la forge devolvió error; estado/`Warning` explícito por forge y por sección.
9. **Sin TTY en scripts**: ningún comando (git/gh/glab/herdr) debe lanzar editor/pager/REPL; pasar flags no interactivos y `GIT_TERMINAL_PROMPT=0`, `GH_PROMPT_DISABLED=1`, `glab` no interactivo.
10. **Rebuild del binario instalado**: al terminar cambios, `go build -o ~/.local/bin/prdash ./cmd/prdash` (el usuario ejecuta ese; un bin stale produce síntomas falsos).

## Riesgos heredados del plan + verificaciones pendientes

- **`tuicr pr` submit contra el GitLab self-managed**: verificar que sube reviews reales por `glab` en `gitlab.example.com`, no solo GitHub.
- **`herdr worktree create` desde clon bare** con rama local ya creada y `--path` destino (el diseño elimina el caso "rama remota no local").
- **Permisos approve/merge en el self-managed** (`<usuario>`): si faltan, deshabilitar acción con motivo.
- **Placement del pane del plugin y link handler** en Herdr 0.9.x reales.
- **`git clone --bare` + fetch de refs de MR** en el host self-managed (permisos de fetch).
- **Rate limit / coste de paginación** con auto-refresco 60s: mitigado con snapshot + incremental + backoff; verificar cabeceras.
- **Drift de `gh`/`glab`/`herdr`**: parseo puro con fixtures string y aislado por comando.
- **Worktrees huérfanos**: ownership en el nombre (`prdash-pr-<N>`), listado y limpieza explícita; se conservan al cerrar.
- **Ancho/estilos de celdas**: verificar `pad()` antes de estilo en Art/emoji/badges de checks.
- **Degradación limpia fuera de Herdr**: comprobar que falta de `HERDR_ENV` falla suave y no bloquea arranque ni deja procesos huérfanos.
- **Auth glab gitlab.com**: `401` tratado como forge no disponible, no como inbox vacío.
