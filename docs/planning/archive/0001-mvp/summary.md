# 0001 — prdash MVP (F1 + F2) — Closing summary

- **Status**: completed. F1 and F2 implemented and merged/ready; F3 stays as a
  documented milestone, out of scope.
- **Version**: `0.1.0` (matches `min_herdr_version`/`version` of the plugin
  manifest). See `CHANGELOG.md`.
- **PRs**: [#1](https://github.com/Sovengar/prdash/pull/1) (F1, rebase merge,
  `mergeCommit 62c18dc`) and [#2](https://github.com/Sovengar/prdash/pull/2)
  (F2, this one). Base of both: `main`.
- **Expected behavior**: `behavior.feature` (single source; it is not Cucumber).
- **Plan**: `plan.md`. **Navigation context**: `context.md`.

## What F1 delivers — Cross-forge inbox

A Go TUI (bubbletea v2) that answers "which PR/MR falls to me?" mixing GitHub
and a self-managed GitLab:

- **Three sections**: created by me, review requested/assigned and mentions,
  with dedupe by identity (forge, host, project, number) and section
  precedence.
- **Rich data per forge**: GitHub via GraphQL (`reviewDecision`, checks) and
  self-managed GitLab via GraphQL (`currentUser`) + Todos API for mentions;
  REST under subfolder `/git/api/v4/` encapsulated behind the adapter.
- **Item detail** (title, author, branches, number, URL, review state and
  checks) without leaving the TUI, reflecting the live state.
- **Manual (`r`) and automatic refresh** (60s by default, configurable, `0`
  manual only) with a "last update" indicator **per forge** and backoff on rate
  limit/timeout; the refresh does not overwrite an action in progress.
- **Progressive loading and pagination without a cap** per section, with a
  snapshot in `cache` and incremental refresh by cursor.
- **approve/merge actions** from the inbox via `gh`/`glab` (or delegating to
  TUICR when applicable), with conflict/re-read when the item changed.
- **Honest degradation**: never "empty" if there was an error; explicit state
  per forge/section (401 = forge unavailable) and the TUI does not abort.
- **Bitbucket interface only**: registered adapter that answers "unsupported"
  with no network.
- **`prdash --print`**: the inbox in plain text, same order as the TUI, for
  scripts and to check the pipeline (and, since F2, the path of the worktree of
  the reviews already mounted).

## What F2 delivers — Review orchestrator

From an item, it leaves the review environment ready (worktree + layout)
without assembling anything by hand:

- **Repo resolver** (`reporesolver`, sole owner of the path namespace):
  remote→local index over `roots`, configurable bare clone if the repo is not
  local, fetch of the review ref (`refs/pull/N/head` /
  `refs/merge-requests/N/head`, forks included) and creation of the local
  working branch.
- **Worktree provisioning** (`worktree.Provisioner`) with two interchangeable
  implementations: **direct git** outside Herdr and **Herdr native** inside (it
  links the checkout to a workspace with stable IDs). The caller never knows
  which one runs. It reuses an existing worktree of an item and allows several
  per repo.
- **3-pane layout** over the worktree: TUICR over the PR/MR, Hunk with the
  diff and an opencode agent, with the right cwd/env/argv; a missing binary
  skips its pane with a warning without taking down the rest.
- **Herdr plugin**: `plugin/herdr/herdr-plugin.toml` with the inbox pane, the
  `mount-review` action, the link handler for PR/MR URLs (GitHub and
  self-managed GitLab) and `prdash herdr inbox|mount|link` subcommands; a
  single source of config/credentials/version (same binary + manifest).
- **Key `m`** in the TUI: mounts the review of the selected item; the
  `prdash.mount-review` action without a URL resolves it too (the TUI persists
  the selected item in `$XDG_STATE_HOME/prdash/selection.json`).
- **Degradation outside Herdr**: the worktree is mounted anyway (direct git)
  and the layout is reported as unavailable; F1 stays operational.
- **`prdash worktrees [list|remove]`**: lists the worktrees owned by prdash
  (`prdash-…` ownership), marks orphans and deletes only on explicit request,
  never someone else's. Worktrees are kept on close (no implicit deletion).

## Key decisions

- **ADR 0001** (`docs/adr/0001-worktree-provisioning.md`): prdash **always**
  does the ref fetch and the local branch creation, and then delegates the
  worktree provisioning; **Herdr native inside Herdr, `git worktree add`
  outside**. The coupling to Herdr is confined to a single port.
- **Plugin = same binary + subcommands + manifest**: no duplicated config,
  credentials nor version.
- **The comment→agent loop does not belong to prdash**: the responsibility ends
  when opening panes with the right cwd, env and argv (no daemon, no poller, no
  comment API).
- **Environment detection in a single place**: only `internal/herdr` reads
  `HERDR_ENV`; the rest receives the injected environment (TUI testable
  "outside Herdr").
- **F3 out of scope**: design/milestone only (see `f3-milestone.md`).

## How to use / install

```sh
make build            # builds ./bin/prdash
make install          # installs into ~/.local/bin/prdash
make config GITLAB_HOST=gitlab.miempresa.com   # ~/.config/prdash/config.toml

prdash                # the inbox TUI (key m: mount the review of the selection)
prdash --print        # the inbox as plain text (+ worktree path of each review)
prdash worktrees      # lists/cleans prdash review worktrees

make plugin-link      # herdr plugin link "$(pwd)/plugin/herdr" (dev)
```

Keybinding (in `~/.config/herdr/config.toml`, prdash does not edit it):

```toml
[[keys.command]]
key = "prefix+m"
type = "plugin_action"
command = "prdash.mount-review"
description = "prdash: mount the review of the PR/MR"
```

Followed by `herdr server reload-config`.

References: `docs/adr/0001-worktree-provisioning.md` (permanent decision) and
`docs/research/herdr-0.9.1-contract.md` (e2e contract with Herdr 0.9.1).

## Real e2e verification (Herdr 0.9.1)

Run against `herdr 0.9.1-preview.2026-09-21-0ff0f27e2226` with reproducible
fixtures (evidence in `docs/research/herdr-0.9.1-contract.md` §Local
verification and in ADR 0001 §Verification):

- `herdr worktree create` from a **bare clone** with an existing local branch
  and a `--path` destination: **OK** (closes the pending verification of the
  plan).
- `--label` labels the **workspace**, not the worktree → prdash keeps its
  ownership label on the caller side.
- `plugin link <dir>` is idempotent; `plugin action invoke` records its result
  in the plugin log.
- `pane split --no-focus` accepts `--ratio`/`--cwd`/`--env`.
- **Findings/side effects**: `worktree create` also opens the **source repo**
  as a workspace; this build of `workspace close` does not expose `--group`.

Note: the project suite is verified with
`go build ./... && go vet ./... && gofmt -l . && go test -race -count=1 ./...`.

## Declared debt / not included

- **F3 auto-review** (gate + allowlist, dry-run by default, never approve with
  a failed/partial analysis): documented in `f3-milestone.md` only; no code.
- **Pane readiness** with `herdr pane wait-output`: panes are launched
  fire-and-forget; the startup of each tool still needs to be confirmed.
- **Closing/management of the source repo workspace** opened by
  `herdr worktree create`: it is not closed when cleaning up.
- **Residual LOWs from the adversarial reviews** of F1 (PR #1) and F2 (PR #2),
  non-blocking: e.g. in F2, a malformed selection without `saved_at` is
  reported as "obsolete" (not "incomplete"), the 24h TTL with last write wins
  across TUI instances, `syncSelection` can persist the first item of the cache
  at startup, the `RemoveAll` fallback of `GitDirect.Remove` could delete a
  live blocked checkout, `removablePath` skips the base guard when `Base==""`,
  the `selection.Save` temp file is not unique, and `Fresh` accepts future
  dates. In F1, a manual `r` during an action in progress is not covered by the
  cycle guard. Details in the project's review observations.
- **Out of scope** (reminder): working Bitbucket, working gitlab.com, "all
  open" view, webhooks/daemon, local repo management beyond the worktree,
  multi-user.
