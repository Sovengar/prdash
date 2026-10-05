# 0001 — prdash MVP — Plan

adr_required: true
adr_reason: the provisioning of the review worktree has real discarded alternatives (native Herdr vs direct git) and fixes the coupling between prdash and Herdr.
adr_title: adr-worktree-provisioning
adr_path: docs/adr/0001-worktree-provisioning.md
adr_note: the ADR is permanent (it lives in docs/adr/) and outlives the archiving of this plan.

## Expected outcome

A single tool (Go TUI + Herdr plugin) that answers "which PR/MR falls to me?" mixing GitHub and a self-managed GitLab in a 3-section inbox, and that when an item is chosen leaves the review environment ready (worktree + 3-pane layout) so that the "I comment → the agent applies" loop happens without assembling anything by hand. Outside Herdr, the inbox stays useful; the orchestrator degrades with a warning.

## Phase scope

- **F1 — Cross-forge inbox (MVP).** 3-section inbox (created by me / review requested or assigned / mentions) with rich data from GitHub and from the self-managed GitLab, item detail, manual + automatic refresh, and approve/merge actions. Bitbucket interface only.
- **F2 — Review orchestrator (MVP).** Resolve/create the local clone, provision the worktree of the PR branch (forks included), and mount the 3-pane layout over the worktree inside Herdr. The comment loop is the responsibility of the tools/agent, not of prdash.
- **F3 — Auto-review (milestone, not implemented here).** The agent reviews and, with a per-repo gate + allowlist, approves and notifies. Its fit is planned, not its code.

## Approach (high level)

F1 pipeline as in the gitdash sibling: *config → forge clients (subprocess `gh`/`glab`) → pure parsing → merge/dedupe → TUI model → render*. F2 pipeline: *item → repo resolver → fetch of the review ref → worktree provisioning → Herdr port (layout) → notification*. The parsing layer is pure and testable with string fixtures; the I/O lives in the adapters and the ports. No daemons nor long-lived state.

## Modules and contracts (boundaries, not file structure)

**Pure (no network, no subprocess, no TOML, no disk):**
- `forge/model` — normalized types: repo reference, item (section, forge, ref, state, decision, checks, URL, timestamp).
- `forge/parse` — translates JSON from `gh`/`glab` (GraphQL, REST, Todos) to `model`.
- `inbox` — consolidates the 3 sections, dedupes and decides "does it fall to me?".
- `state` — derived states with precedence and order score (attention first), shared with the `--print` mode.
- `review/plan` — given (item, worktree, environment) produces the pane plan (cwd, argv, labels) without touching Herdr.

**I/O (adapters and ports):**
- `config` — XDG TOML; `Load()` never fails (defaults + warning). roots, forges/hosts, cadence, bare clone/worktree paths, tool argv.
- `forge` + `forge/{github,gitlab,bitbucket}` — implement the `Adapter` contract (state/auth, list authored, review-requested/assigned, mentions, item state, approve, merge) returning items **plus typed warnings; they never fail hard**.
- `reporesolver` — sole owner of the path namespace: remote→local index over `roots`, path memory, bare clone, fetch of the review ref. It does not call the forge API (it resolves remotes of existing clones).
- `worktree` — provisioning port with two interchangeable implementations (Herdr native / direct git); the caller does not know which one runs.
- `herdr` — the **only** place that reads `HERDR_ENV` and parses Herdr output: availability, workspace/tab/pane, layout, notifications, link handlers.
- `review/executor` — applies the plan using the ports (reporesolver, worktree, herdr).
- `cache` — inbox snapshot and remembered paths; corrupt = silent.
- `tui` — bubbletea model, table/detail, cadence and event pump.
- `cmd/prdash` — entrypoint: TUI, `--print`, `herdr …` subcommands that the plugin consumes.

**Key contracts (roles, no signatures):** `forge.Adapter` (list + warnings, never a hard error), `reporesolver.Resolver` (resolve local path, ensure bare clone, fetch review ref), `worktree.Provisioner` (create/remove/list), `herdr.Port` (availability, layout, notify, open link). The non-crossable ones: `inbox`/`parse` touch neither network nor disk; adapters touch neither git/worktree/TUI; `reporesolver` does not touch the forge API; `herdr` touches neither git nor TUI; `review` only speaks through ports.

## Key decisions

1. **Worktree provisioning: Herdr native inside Herdr, `git worktree add` outside.** prdash always does the fetch and the ref resolution (incl. fork PR refs) and creates the local branch; then it delegates the provisioning to Herdr's native one (it links the worktree to a workspace, with stable IDs that the layout needs as a container) and falls back to direct git outside Herdr. Doing the fetch ourselves removes the risk of "--branch with a nonexistent remote branch": the native one always receives an already existing local branch.
2. **Plugin = same binary + subcommands + manifest.** The Herdr manifest declares the inbox pane, an action to mount the review of the selected item and a link handler for PR URLs that forwards to a subcommand. A single source of config/credentials/version; manifest and code do not diverge.
3. **The comment→agent loop does not belong to prdash.** The responsibility ends when opening panes with the right cwd, env and argv: no daemon, no poller, no comment API. It avoids coupling to TUICR/Hunk and to each forge's review API.
4. **Bitbucket is a registered adapter, not a dead stub.** It shares the conformance suite with the real implementations and must answer an explicit "unsupported", with no network.
5. **Environment detection in a single place.** Only `herdr` reads `HERDR_ENV`; the rest receives the injected environment, so the TUI is tested in "outside Herdr" mode.
6. **Single XDG config.** Adapters and TUI read no environment/TOML; the plugin uses the same TOML.

## No per-section cap: pagination and progressive loading (critical area)

It is decided **not to cap** the sections: paginate until each one is exhausted.

- **GitHub** via GraphQL with `pageInfo/endCursor` until `hasNextPage=false`. **GitLab** via page iteration on GraphQL/REST (or `glab api --paginate`).
- **Progressive loading:** the first render shows the first page of each section (fast) and the remaining pages arrive in the background without blocking the UI nor the refresh.
- **Impact on rate limits/refresh:** with automatic refresh every 60s and unlimited pagination, the cost per cycle can spike. Mandatory mitigation: reuse the snapshot in `cache` and refresh incrementally (first page + cursor comparison), backoff and respect of the rate limit headers (GitHub ~5000 pts/h; the 401 of gitlab.com is treated as forge unavailable), and pause the auto-refresh while an action is in progress or while paginating.
- **UI:** "last update" indicator per forge (a slow forge must not lie about the rest) and a "loading more…" counter while there is pending pagination.

## Risks

- **Rate limit / cost per `--paginate`** — mitigated with snapshot + incremental + backoff (see above).
- **Auth of the self-managed GitLab with subfolder `/git/api/v4/`** — encapsulated behind the adapter with a configurable base URL; read-only test against the real host.
- **Drift of the Herdr CLI (0.9.x)** — parsing isolated per command, minimum version in the manifest and fallback (git + splits) confined to `herdr`.
- **Drift of `gh`/`glab` output/exit codes** — pure parsing with string fixtures.
- **Orphan / trash worktrees** — ownership in the name, listing and an explicit cleanup command (worktrees are kept on close).
- **Silent degradation** (confusing error with empty) — explicit state per forge/section; never "empty" if there was an error.
- **TUICR/Hunk change flags** — they are configurable argv in the plan, not APIs; missing binary = pane skipped with a warning.
- **F3 auto-approve** — gate + allowlist in front; never approve with a failed or partial analysis; dry-run by default.

## Pending verifications

- `tuicr pr` submit (real review) against the self-managed GitLab, not only GitHub.
- `herdr worktree create` from a **bare clone** with the local branch already created and a `--path` destination (equivalent to the "--branch with a non-local remote branch" unknown, which the design removes by doing the fetch first).
- approve/merge permissions of the user on the self-managed (`<user>`); if missing, the action must be disabled with a reason.
- Real `placement` of the plugin pane and link handler resolution on Herdr 0.9.x.
- Behavior of `git clone --bare` + `fetch` of PR refs on the self-managed host (merge-request refs and fetch permissions).

## Work order (rough)

1. Skeleton: config + model + parse + GitHub adapter + GL adapter + inbox + read-only TUI (F1 core).
2. Complete F1: detail, refresh/pagination/progressive loading, approve/merge actions, per-forge degradation, Bitbucket adapter + conformance.
3. F2 core: repo resolver + bare clone + ref fetch + worktree provisioning.
4. F2 layout: Herdr port, pane plan, plugin/manifest, link handler, degradation outside Herdr.
5. Closing: worktree cleanup, `--print`, and leave F3 documented as a milestone.

## Out of scope (reminder)

Working Bitbucket, working gitlab.com, implemented F3, "all open" view, webhooks/daemon, local repo management beyond the worktree, multi-user.
