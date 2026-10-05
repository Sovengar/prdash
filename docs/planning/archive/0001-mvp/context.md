---
feature: 0001-mvp
freshness: f199e3d8cddb74a5deba76fe8518cd000dd319a0
codegraph: not_initialized
generated_by: codebase-researcher
---

# Context: prdash MVP — multi-forge inbox (F1) + review orchestrator (F2)

## Repo freshness and nature

- HEAD `f199e3d8cddb74a5deba76fe8518cd000dd319a0`, branch `main`.
- `codegraph: not_initialized` (there is no `.codegraph/`).
- **Greenfield**: no `go.mod`, not a single line of Go. "Files to touch" = files/packages **to create**. There are no existing lines to extend.
- Reference sibling repo: `/home/buble/dev/projects/gitdash` (same stack and conventions). Engram index `codebase-index/gitdash` obs **#2202** (fresh except for later drift). **Do not** copy literally: import the pattern.

## What exists today in prdash

| Path | Content |
|---|---|
| `README.md` | 5 lines: description + "Status: planning (MVP)". |
| `.gitignore` | `bin/` and `*.test`. |
| `docs/planning/archive/0001-mvp/` | `issue.md`, `behavior.feature`, `plan.md`, this `context.md`. |
| `docs/adr/0001-worktree-provisioning.md` | Accepted ADR: worktree provisioning. |

There is no code, no `cmd/`, no `internal/`, no `go.mod`, no `scripts/`, no `plugin/`.

## Initial layout to create

Stack convention (inherited from gitdash, reference `go.mod`): Go `1.26.3`, module `prdash`, `charm.land/bubbletea/v2 v2.0.9`, `charm.land/bubbles/v2 v2.2.1`, `charm.land/lipgloss/v2 v2.0.6`, `github.com/BurntSushi/toml v1.6.0`, no cgo. Imports `charm.land`, **NOT** `github.com/charmbracelet`.

### Pure (no network, no subprocess, no TOML, no disk)

| Package / file | Responsibility | Expected symbols/contracts |
|---|---|---|
| `internal/forge/model/model.go` | Immutable normalized types. | `RepoRef{Forge,Host,Project,Owner,Name}`; `Item{Section,Forge,Host,Ref,Number,Title,Author,SourceBranch,TargetBranch,URL,State,ReviewDecision,Checks,UpdatedAt}`; `Section` (authored/review/mentions); `Warning{Forge,Section,Kind,Msg}`; identity = `With(forge,host,project,number)`. |
| `internal/forge/parse/parse.go` | Translates JSON from `gh`/`glab` (GraphQL, REST, Todos) to `model`. | `ParseGHGraphQLSearch`, `ParseGHAuthored`, `ParseGLGraphQL`, `ParseGLMRList`, `ParseGLTodos`, `ParseGHChecks`; **one function per output shape**, with string fixtures. It never panics: it returns items + a typed parse error. |
| `internal/inbox/inbox.go` | Consolidates the 3 sections, dedupes and decides relevance. | `Build(inputs []ForgeResult) Inbox`; section authority rule (authored > review > mentions); dedupe by `RepoRef`+number. No network/disk. |
| `internal/state/state.go` | Derived state with precedence and order score (attention first), shared by the TUI and `--print`. | `type State int` + consts, `String()`, `Derive(item) State`, `Score() int`. Precedence to define (e.g. `error > changes-requested > review-required > approved > pending > merged/closed > draft`). |
| `internal/review/plan/plan.go` | Given `(Item, Worktree, Environment)` produces the pane plan without touching Herdr. | `Pane{Cwd,Argv,Label,Env,Kind}`; `Plan(toolArgs ToolArgs, wt Worktree, env Env, pr Item) Plan`. |

### I/O (adapters and ports)

| Package / file | Responsibility | Expected symbols/contracts |
|---|---|---|
| `internal/config/config.go` | XDG TOML; `Load()` never fails (defaults + warning). | `Load() (Config,string)`, `LoadFrom(path)`, `Path()`, `Defaults()`, `expandAll()`, `Keybindings`, `Commands`, `HintBarLines()`. See §Config. |
| `internal/forge/forge.go` | `Adapter` contract + per-forge registry. | `Adapter interface { Forge() string; Host(); Auth(ctx) AuthState; Authored(ctx) ([]model.Item,[]model.Warning); ReviewRequested(ctx); Mentions(ctx); ItemState(ctx,RepoRef,number); Approve(ctx,…); Merge(ctx,…) }`. **It never returns a hard error**: items + `[]Warning`. |
| `internal/forge/github/github.go` | GitHub adapter via `gh` (GraphQL + REST). | Implements `Adapter`; `exec.CommandContext("gh",…)`; parses with `forge/parse`. |
| `internal/forge/gitlab/gitlab.go` | Self-managed GitLab adapter via `glab` (GraphQL + REST + Todos). | Implements `Adapter`; configurable base URL with subfolder `/git/api/v4/`. |
| `internal/forge/bitbucket/bitbucket.go` | Registered adapter, **no network**. | An `Adapter` that answers `Warning{Kind:"unsupported"}` on every method; compiles and passes the conformance suite. |
| `internal/reporesolver/reporesolver.go` | **Sole owner of the path namespace**: remote→local index over `roots`, path memory, bare clone, fetch of the review ref, local working branch. It does NOT call the forge API. | `Resolver interface { ResolveLocal(RepoRef) (path, bool); EnsureBare(RepoRef) (path,error); FetchReviewRef(path, Item) (branch string, error); Remember(RepoRef,path) }`. |
| `internal/worktree/worktree.go` | Provisioning port with 2 interchangeable implementations. | `Provisioner interface { Create(Spec) (Worktree,error); Remove(id); List() []Worktree }`; `Spec{Cwd,Branch,Path,Label}`. Implementations: `herdrNative` (inside Herdr) and `gitDirect` (`git worktree add`). The caller does not know which one runs. |
| `internal/herdr/herdr.go` | The **only** place that reads `HERDR_ENV` and parses Herdr output. | `Port interface { Available() bool; WorktreeCreate(Spec) (Worktree,error); PaneSplit/Workspace/Tab…; Layout(plan.Plan) error; Notify(title string); LinkHandler(url) }`. Degradation outside Herdr. |
| `internal/review/executor/executor.go` | Applies the plan using the ports. | `Mount(item, resolver, worktree, herdr) Result`; orchestrates resolve→fetch→branch→provision→layout. It only speaks through ports. |
| `internal/cache/cache.go` | Inbox snapshot + remembered paths; corrupt = silent. | `Load(path) (File,bool)`, `Save(path, File)`; `FileName = "inbox.json"`, `DirName = "prdash"`; `version` to invalidate. |
| `internal/tui/app.go` | bubbletea model + background pipelines + event pump. | `Model`, `New(cfg)`, `Init()`, `Update`, `View`, `waitForEvent`, `withPump`, `startRefreshCmd`, `tickCmd`, `Msg` types. |
| `internal/tui/update.go` | `Update`/keys/refresh/detail. | `handleKey`, `actionForKey`, `View()` (alt-screen), render of the 3 sections. |
| `internal/tui/table.go` | Rows/order/cells per section. | `row{item,state}`, `rows()`, `cells` returning `(text,style)`. `pad()` BEFORE style. |
| `internal/tui/detail.go` | Item detail. | `renderDetail`. |
| `internal/tui/styles.go` | lipgloss styles. | consts/vars. |
| `internal/testutil/testutil.go` | Fixtures: real git repos + forge/Herdr fakes. | See §Tests. |
| `cmd/prdash/main.go` | Entrypoint: TUI + subcommand dispatch. | `main()`; `--print`; `herdr …` subcommands that the plugin consumes. |
| `cmd/prdash/print.go` | One-shot `--print` mode (tabwriter, same order as the TUI). | `runPrint(cfg)`. |
| `cmd/prdash/herdr.go` (removed by ADR 0003) | Subcommands invoked by the plugin manifest (pane entrypoint, mount-review action, link handler). | `runHerdrInbox`, `runHerdrMount`, `runHerdrLink`. |
| `plugin/herdr/herdr-plugin.toml` (removed by ADR 0003) | Herdr 0.9.x manifest. | Inbox pane (placement), "mount review" action with keybind, link handler for PR URLs; minimum version. See §Contracts. |

## gitdash patterns to follow (real paths)

| Concern | Model file | What to import as a concept |
|---|---|---|
| Config that never fails | `gitdash/internal/config/config.go:81-149` (`Load`/`LoadFrom`) and `Defaults()` `:196-213` | Missing file → silent defaults; broken TOML → defaults + warning string, never a panic. `expandAll` `:216-228` expands `~`. Keybindings/Commands merge over defaults. |
| Pure parsing + fixtures | `gitdash/internal/gitstatus/parse.go:126` (`ParsePorcelain`), `:237` (`ParseWorktrees`) | Parsing in a file separate from the I/O, touching neither network/subprocess; tests with string fixtures. In prdash: `forge/parse`. |
| Derived state + score | `gitdash/internal/gitstatus/parse.go:43-125` (`State`, `Derive`, `Score`) | State enum with explicit precedence and `Score()` for the attention-first order shared by the TUI and `--print`. |
| Snapshot with embedded error | `gitdash/internal/gitstatus/status.go:23-55` (`Snapshot`, `State`) | The result carries `Err` inside; it never fails hard. In prdash: `Warning` per forge/section. |
| Concurrent pool with emission | `gitdash/internal/gitstatus/status.go:126-154` (`StreamPool`) | Semaphore + goroutines that emit via callback; pattern to parallelize per-forge queries without blocking the UI. |
| Subprocess with LC_ALL=C | `gitdash/internal/gitstatus/status.go:202-267` (`gitEnv`, `runGit`, `runGitCombined`) | Force the English locale to recognize `gh`/`glab` error messages the same way; timeout via `exec.CommandContext`. |
| Event pump | `gitdash/internal/tui/app.go:29-82` (msgs), `:215-235` (`waitForEvent`/`sendEvent`/`tickCmd`), `gitdash/internal/tui/update.go:117-121` (`withPump`) | Each `tea.Cmd` reads ONE event from the channel; ALWAYS re-arm `waitForEvent` in `Update`. Gotcha #1. |
| Background pipeline | `gitdash/internal/tui/app.go:242-267` (`startScanCmd`), `:292` (`fetchBatchCmd`), `:343` (`startActionCmd`) | Goroutine + `sendEvent(ctx,…)`; "one operation at a time" guard; automatic refresh with `tea.Tick` (`tickCmd`). |
| Refresh that does not overwrite an action | `gitdash/internal/tui/update.go:15-115` (dispatcher by `Msg`) | Merging results by key without reverting the state of an action in progress ("a refresh does not overwrite an action in progress" scenario). |
| Persisted UI state | `gitdash/internal/state/state.go:26-100` (`Store`, atomic `SaveCollapsed`/`LoadCollapsed` tmp+rename) | Store in `$XDG_STATE_HOME/prdash`. Corrupt/absent = silent. |
| Instant cache | `gitdash/internal/cache/cache.go:55-110` (`Load`/`Save`, `version`) | JSON snapshot to paint at startup; best-effort validation; corrupt is silent. |
| `--print` mode | `gitdash/cmd/gitdash/print.go:27-111` (`runPrint`) | Reuses config+collection, tabwriter, **same order** as the TUI (via `Score()`). |
| Real git repo fixtures | `gitdash/internal/testutil/testutil.go:13-172` (`Init`, `InitBare`, `AddUpstream`, `MakeWorktree`, `NewBranch`, `FetchLocal`) | Real git repos in `t.TempDir()`; base for worktree/fork fixtures. |
| Entrypoint | `gitdash/cmd/gitdash/main.go:20` (`main`) | `config.Load()` → warn to stderr → `tui.New(cfg)` + `tea.NewProgram`; `--print` flag → `runPrint`. |
| Table grouping/headers | `gitdash/internal/group/group.go:39-119` (`Arrange`) | 2-level arrangement with headers; analogous to the 3 inbox sections. |
| Cells returning (text, style) | `gitdash/internal/tui/table.go` (funcs `*Cell`), `:render*` | `pad(text)` BEFORE applying style (ANSI breaks the width). Table gotcha. |
| TUI smoke | gitdash convention (`AGENTS.md` Gotcha 3) | `tmux` + `capture-pane`; `script` does NOT work (bubbletea v2 blocks the first render). |

## TOML config to define (keys + defaults)

`config.toml` at `$XDG_CONFIG_HOME/prdash/config.toml`. Missing file → silent defaults.

| Key | Type | Default | Use |
|---|---|---|---|
| `roots` | `[]string` | `["~/dev"]` | Roots where to search for local clones (gitdash pattern). |
| `refresh_interval` | duration string | `"60s"` | Auto-refresh cadence. `0` = manual only. |
| `forges` | table/array | github + gitlab enabled | See subkeys. |
| `forge.github.host` | string | `"github.com"` | GitHub host. |
| `forge.gitlab.host` | string | `"gitlab.example.com"` | Self-managed GL host. |
| `forge.gitlab.api_base` | string | `"/git/api/v4/"` | **Subfolder** required on self-managed GL. |
| `forge.gitlab.token_env` | string | (implicit, handled by glab) | Do not duplicate credentials: `glab`/`gh` already authenticated. |
| `forge.bitbucket.enabled` | bool | `false` | Interface-only adapter; no network. |
| `data_dir` / `clone_dir` | string | `~/.local/share/prdash/repos/<forge>/<host>/<owner>/<repo>` | Bare clone (configurable). |
| `worktree_dir` | string | under `data_dir` | Worktree destination (configurable). |
| `tools.tuicr` | string (argv) | `"tuicr"` | TUICR binary/argv. |
| `tools.hunk` | string (argv) | `"hunk"` | Hunk binary/argv. |
| `tools.agent` | string (argv) | `"opencode"` | Agent binary/argv. |
| `tools.gh` / `tools.glab` | string | `"gh"` / `"glab"` | Forge binaries (override). |
| `autoreview.enabled` | bool | `false` | F3 post-MVP; parsing only, dry-run by default. |
| `autoreview.allowlist` | `[]string` | `[]` | Repos (`host/owner/repo`) allowed for auto-approve (F3). |
| `keybindings` | map | see below | Merge over defaults. |
| `commands` | map | `gh`/`glab` base argv | Merge over defaults. |

Suggested keybinds (merge over defaults, gitdash's `DefaultKeybindings` pattern): `quit=q`, `refresh=r`, `detail=enter`, `mount-review=m` (needs Herdr), `approve=a`, `merge=M`, `section-next=tab`, `open-browser=o`.

## Contracts with external tools (EXACT commands)

**GitHub via `gh`** (authenticated user `Sovengar`, scopes gist/read:org/repo/workflow; `gh` 2.67.0):
- Rich inbox (reviewDecision + checks): `gh api graphql` with `search(type:ISSUE, query:"is:pr is:open author:@me")`. Do **NOT** use `gh search prs` for the rich inbox (limited JSON).
- Query variants: `author:@me`, `review-requested:@me`, `assignee:@me`, `mentions:@me` (mentions) — **all via GraphQL**.
- PR ref (fork): `refs/pull/<N>/head`.
- Checks state: GraphQL fields (`reviewDecision`) + `statusCheckRollup`/check runs.
- Rate limit: GraphQL ~5000 pts/h; respect the headers and backoff.

**GitLab via `glab`** (gitlab.com WITHOUT a token → 401; self-managed `gitlab.example.com` authenticated as `<user>`; `glab` 1.119.0):
- REST under subfolder **`/git/api/v4/`** (glab handles it; configurable base URL).
- Inbox: `glab api graphql` with `currentUser` → `authored` / `reviewRequested` / `assigned`. REST alternative `/merge_requests?scope=…`.
- Mentions: Todos API `glab api <base>/todos?action=mentioned`.
- `glab mr list -F json` = `BasicMergeRequest` (**no pipeline**, 1 page, no `--paginate`) → insufficient for rich state; use GraphQL.
- MR ref: `refs/merge-requests/<N>/head`.
- Expected failures: `401` on gitlab.com → treat as forge unavailable (do not empty the inbox); without approve/merge permissions on the self-managed → disable the action with a reason.

**tuicr** (it already pushes real reviews via gh/glab/bkt/az):
- `tuicr pr <number|owner/repo#N|URL>` (alias `mr`).
- `tuicr review list|comments|add` → JSON; poll ~30s, no push.
- MVP: delegate approve/commenting to tuicr when applicable; use direct gh/glab for fast approve/merge from the inbox.

**hunk**:
- `hunk session review|navigate|reload|comment add|list --type user` → JSON.
- `hunk skill path` (discover binary/path).
- Missing binary = pane skipped with a warning.

**herdr** (only if `HERDR_ENV=1`):
- `herdr worktree create --cwd <repo> --branch <local> --path <dest> --label <prdash-…> --no-focus`; also `open`/`list`/`remove`.
- `herdr workspace|tab create`; `herdr pane split --cwd --ratio --no-focus`; `pane run`; `pane read`; `pane wait-output`.
- `herdr agent start <name> --kind opencode --pane ID`; `agent prompt --wait`; `agent read/wait/send-keys`.
- `herdr notification show <title> --sound request`.
- Plugin: `herdr-plugin.toml` → panes placement overlay/popup/split/tab, actions with keybind, link handlers Ctrl+click, events; env `HERDR_BIN_PATH`/`HERDR_PLUGIN_*`. Docs: herdr.dev/docs. Target Herdr 0.9.x.
- **The argv of every tool is configurable** (not an API): flag drift is absorbed in config.

**git** (provisioning and refs):
- Repo resolution: `git -C <root> remote get-url origin` to build the remote→local index.
- Bare clone: `git clone --bare <url> <dest>`.
- Review ref fetch: `git fetch origin refs/pull/<N>/head:refs/heads/prdash/pr-<N>` (GH) or `refs/merge-requests/<N>/head` (GL).
- Local working branch + worktree fallback: `git worktree add <dest> <local-branch>`.
- Force `LC_ALL=C` in the subprocess env to parse errors (gitdash pattern `gitEnv`).

## Pagination and refresh strategy (no cap)

- **Do not cap** the sections: paginate until exhausted.
- GitHub GraphQL: `pageInfo { hasNextPage endCursor }` until `hasNextPage=false`.
- GitLab: GraphQL/REST page iteration (or `glab api --paginate`).
- **Progressive loading**: first render = first page of each section (fast); remaining pages in the background without blocking the UI nor the refresh. "loading more…" indicator per section.
- **Incremental refresh**: reuse the snapshot in `cache`; refresh first page + compare by cursor.
- **Backoff** + respect of rate limit headers; `401` from gitlab.com = forge unavailable.
- **Pause the auto-refresh** while an action is in progress or while paginating.
- "last update" indicator **per forge** (a slow forge must not lie about the rest).

## Tests and infra

- Runner: `go build ./... && go vet ./... && go test ./...`.
- **String fixtures** for `forge/parse` (real trimmed GraphQL/REST/Todos payloads → `model.Item`). gitdash pattern `parse_test.go`.
- **Forge/Herdr fakes**: in-memory implementations of the `Adapter`, `Port`, `Provisioner` contracts for tests of `inbox`, `review/executor`, `state` and `tui` (no network nor subprocess).
- **Real git repo fixtures** in `t.TempDir()` for `reporesolver`/`worktree`: reuse gitdash helpers (`Init`, `InitBare`, `AddUpstream`, `MakeWorktree`, `NewBranch`, `FetchLocal`).
- **Direct model tests** (build `Model`, send msgs with `Update`, inspect state) — no teatest. Pattern `gitdash/internal/tui/app_test.go`.
- **TUI smoke with tmux** (`capture-pane`); `script` does NOT work.
- Suggested scenario mapping: sections/dedupe/rich state → `inbox` + `parse`; per-forge degradation → `forge/*` (401, Bitbucket); worktree/fork/reuse → `reporesolver` + `worktree` with real repos; layout fallback → `review/plan` + `herdr` (fakes); refresh/pagination → `tui` + `state`.

## Conventions and module boundaries

- Code comments in **Spanish**, **without** references to specs/requirement IDs/scenarios (the code is the source of truth). Do not create SDD artifacts inside the repo beyond the existing `docs/planning/`.
- **Non-crossable boundaries** (plan §Modules): `inbox`/`parse` touch neither network nor disk; adapters touch neither git/worktree/TUI; `reporesolver` does not touch the forge API; `herdr` touches neither git nor TUI and is the **only** one that reads `HERDR_ENV`; `review` only speaks through ports.
- Adapters return **typed items + warnings, never a hard error**.
- Table cells return `(text, style)`; `pad()` before style.
- Derived state with explicit precedence and `Score()` for the attention-first order (TUI/`--print` shared).
- SUBPROCESS: always `exec.CommandContext` with timeout and `LC_ALL=C`.
- Single XDG config; adapters and TUI read no environment/TOML (the plugin uses the same TOML).

## Non-obvious integration points / gotchas

1. **Self-managed GitLab subfolder**: the REST lives under `/git/api/v4/`; any URL built by hand must include it. Encapsulated behind `forge/gitlab`.
2. **Isolated `HERDR_ENV`**: only `herdr` reads it; the rest receives an injected environment (it allows testing the TUI in "outside Herdr" mode with no global variables).
3. **Path ownership**: only `reporesolver` decides where the bare clone and the worktrees live; no other module builds paths. `cache` remembers paths already resolved.
4. **Configurable argv** of tuicr/hunk/agent: they are plan data, not APIs; a flag change is absorbed in config without touching code.
5. **No re-entry of the plugin pane**: the manifest pane entrypoint must NOT relaunch the whole TUI inside itself; it dispatches to a subcommand that prints/serves a unit of work (inbox pane or action).
6. **Event pump**: re-arm `withPump` after every consumed event or the inbox does not paint (gitdash Gotcha #1).
7. **Fork ref ≠ origin branch**: an explicit `fetch` of `refs/pull/N/head` / `refs/merge-requests/N/head` is needed and the local branch must be created **before** asking Herdr's native for the worktree.
8. **"Empty" vs "error" detection**: never show "empty" if the forge returned an error; explicit state/`Warning` per forge and per section.
9. **No TTY in scripts**: no command (git/gh/glab/herdr) must launch an editor/pager/REPL; pass non-interactive flags and `GIT_TERMINAL_PROMPT=0`, `GH_PROMPT_DISABLED=1`, non-interactive `glab`.
10. **Rebuild of the installed binary**: on finishing changes, `go build -o ~/.local/bin/prdash ./cmd/prdash` (the user runs that one; a stale binary produces false symptoms).

## Risks inherited from the plan + pending verifications

- **`tuicr pr` submit against the self-managed GitLab**: verify that it pushes real reviews via `glab` on `gitlab.example.com`, not only GitHub.
- **`herdr worktree create` from a bare clone** with the local branch already created and a `--path` destination (the design removes the "remote branch not local" case).
- **approve/merge permissions on the self-managed** (`<user>`): if missing, disable the action with a reason.
- **Plugin pane placement and link handler** on real Herdr 0.9.x.
- **`git clone --bare` + MR refs fetch** on the self-managed host (fetch permissions).
- **Rate limit / pagination cost** with 60s auto-refresh: mitigated with snapshot + incremental + backoff; verify the headers.
- **`gh`/`glab`/`herdr` drift**: pure parsing with string fixtures and isolated per command.
- **Orphan worktrees**: ownership in the name (`prdash-pr-<N>`), explicit listing and cleanup; they are kept on close.
- **Cell width/styles**: verify `pad()` before style on Art/emoji/check badges.
- **Clean degradation outside Herdr**: check that the lack of `HERDR_ENV` fails soft and blocks neither startup nor leaves orphan processes.
- **Auth glab gitlab.com**: `401` treated as forge unavailable, not as an empty inbox.
