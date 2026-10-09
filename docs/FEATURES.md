# Features

Concise feature inventory of prdash. Details live in `../README.md` and `adr/`.

## Feature inventory

| Feature | What it does | Trigger |
|---|---|---|
| [Inbox sections & navigation](#inbox-sections--navigation) | Three sections (Assigned, Mentioned, Mine) with per-section cursor/scroll memory | `tab`, `j`/`k`, `pgup`/`pgdn`, `home`/`end` |
| [Item card & comments](#item-card--comments) | Detail panel with fields, URL, and latest 5 comments | automatic on cursor move |
| [Prefix modes](#prefix-modes) | Cycle ITEM column prefix display: common, full, leaf | `p` |
| [Merge flow](#merge-flow) | Two-key merge with strategy picker and branch deletion toggle | `m` then `r`/`m`/`s`, `tab` |
| [Approve](#approve) | Approve a PR/MR (self-approve blocked) | `a` |
| [Retarget](#retarget) | Change target branch with forge branch list and filter | `e` |
| [Simulation](#simulation) | Render merge result with git-sim (graphics layer or half-blocks) | `v` |
| [Review mount & layout](#review-mount--layout) | Mount worktree + 2-tab Herdr layout (Review/Edit) | `r` |
| [Worktree management CLI](#worktree-management-cli) | List and remove prdash-owned review worktrees | `prdash worktrees …` |
| [Config & keybindings](#config--keybindings) | XDG config with remappable keys; degrades to defaults | — |
| [Degradation rules](#degradation-rules) | Missing tools warn and skip; never abort | — |
| [Refresh & backoff](#refresh--backoff) | Auto-refresh with exponential backoff on rate limit/timeout | `R`, auto-tick |
| [Open in browser](#open-in-browser) | Open item URL in system browser | `o` |
| [Auth detection](#auth-detection) | Per-forge auth status with reason in notices | automatic |
| [CLI print mode](#cli-print-mode) | Print inbox as plain text (includes mounted review paths) | `prdash --print` |

---

## Inbox sections & navigation

- **Three sections** — Assigned (review requests + assigned reviews), Mentioned, Mine (authored); `tab` cycles Assigned → Mentioned → Mine.
- **Per-section memory** — each section remembers its cursor and scroll position.
- **Navigation** — `j`/`k` or `↑`/`↓` move cursor; `pgup`/`pgdn` page; `home`/`end` jump to top/bottom.
- **Counts legend** — top border shows `Mine (n) · Assigned (n) · Mentioned (n)` with active section highlighted.
- **One section at a time** — only the active section is painted; no full-screen view.

## Item card & comments

- **Detail panel** — bottom 40% of screen; moves with cursor.
- **Fields grid** — short fields in two-column grid; URL on full-width row.
- **Latest comments** — up to 5 most recent comments in a rounded box; fetched on cursor arrival and cached.
- **Comment count** — bottom border shows `n of total`.
- **System notes filtered** — GitLab action history (assignments, commits) excluded.
- **All-or-nothing box** — if the box does not fit, it is not painted.
- **Degradation** — `loading…`, `not read: …`, or `none` shown when forge cannot answer.

## Prefix modes

- **Three modes** — `common` (default; shows common prefix line), `full` (whole path, tail-truncated), `leaf` (repo name only).
- **Cycle** — `p` advances common → full → leaf → common.
- **Keybar indicator** — mode name shown in keybar (`p prefix: full`).
- **Not persisted** — resets to `common` on reopen.

## Merge flow

- **Two-key merge** — first `m` arms; second key picks strategy.
- **Strategies** — `r` rebase, `m` merge commit, `s` squash; only modes the repository allows are offered.
- **Branch deletion** — `tab` toggles delete-after-merge (default: yes); shown in confirmation.
- **Cancel** — `esc` cancels; any other key cancels and is consumed (no re-dispatch).
- **Pre-merge checks** — draft/merged/closed = hard block; conflict/CI failing/CI running/changes requested = soft warning (arms with notice).
- **Commit pinning** — merge pinned to the commit read (`--match-head-commit` / `--sha`); refused if forge does not report it.
- **Post-merge cleanup** — worktree deleted only if clean; kept with notice if uncommitted changes or status unreadable.

## Approve

- **Approve** — `a` approves the selected item.
- **Self-approve blocked** — own PR/MR detected before calling; reason shown in notice.
- **Forge denial sticky** — `action disabled: …` shown on card while notice expires.

## Retarget

- **Branch chooser** — `e` opens popup with repository branches from the forge.
- **Filter** — type to search; `ctrl+u` clears; `backspace` deletes; `j`/`k` navigate only when filter empty.
- **Confirmation** — second `enter` confirms; `esc` returns to list.
- **Branch cache** — 5 minutes per repository.
- **Open items only** — merged/closed items cannot be retargeted.
- **Mounted review untouched** — notice warns that the mounted review still has the old base.

## Simulation

- **git-sim integration** — `v` renders how history would look after merge.
- **Strategy chooser** — only `merge` offered (git-sim 0.3.5 does not draw rebase).
- **Graphics layer** — inside Herdr with `terminal.kitty_graphics`, image published on pane graphics layer at native resolution.
- **Half-block fallback** — without Herdr or with layer off, image painted with half-blocks in truecolor.
- **Open image** — `o` opens rendered image in system viewer.
- **Cache** — images kept in `$XDG_CACHE_HOME/prdash/sim` (last 20).
- **Temporary clone** — render happens in a temporary clone; no refs or worktrees left behind.

## Review mount & layout

- **Mount review** — `r` mounts the worktree and opens 2-tab layout inside Herdr.
- **Two tabs** — `Review` (TUICR | editor) and `Edit` (Hunk | agent); panes at 50%.
- **Degradation** — without Herdr, worktree is mounted without panes; warning shown.
- **Missing tools** — if TUICR, diff, or agent binary missing, pane is skipped with warning; editor pane never skipped.
- **Pane commands** — each pane replaceable from `[commands]`; value used verbatim as full argv.
- **Environment** — panes receive `PRDASH_BASE`, `PRDASH_REPO`, `PRDASH_NUMBER`, `PRDASH_WORKTREE`, `PRDASH_BRANCH`, `PRDASH_URL`.

## Worktree management CLI

- **List** — `prdash worktrees` or `prdash worktrees list` shows path, branch, state; flags orphans.
- **Remove** — `prdash worktrees remove <path>…` deletes only prdash-owned worktrees.
- **Orphans** — `prdash worktrees remove --orphans` batch-deletes orphaned worktrees.
- **Dry run** — `prdash worktrees remove --orphans --dry-run` prints batch without deleting.
- **Mutual exclusion** — `--orphans` and explicit paths cannot be mixed.
- **Ownership guard** — only worktrees labeled `prdash-…` are listed or deleted.

## Config & keybindings

- **XDG config** — `$XDG_CONFIG_HOME/prdash/config.toml`.
- **Remappable keys** — `[keybindings]` maps action names to keys; missing actions keep defaults.
- **Config never aborts** — missing or malformed config degrades to defaults with warning.
- **Forge config** — `[forge.github]`, `[forge.gitlab]`, `[forge.bitbucket]` with host, clone_base, api_base, token_env.
- **Tools** — `[tools]` for tuicr, hunk, agent, editor, gh, glab binaries.
- **Commands** — `[commands]` for pane argv overrides and forge CLI paths.
- **Roots & dirs** — `roots`, `data_dir`, `clone_dir`, `worktree_dir`, `refresh_interval`.

## Degradation rules

- **Missing gh/glab** — warning; forge skipped.
- **Missing git-sim** — `v` warns and does nothing.
- **Missing Herdr** — `r` mounts worktree only; layout skipped with warning.
- **Missing TUICR/Hunk/agent** — pane skipped; warning shown.
- **Broken config** — defaults used; warning on startup.
- **Forge errors** — auth, timeout, rate limit, parse, network shown as warnings; inbox stays operational.

## Refresh & backoff

- **Auto-refresh** — every 60s (configurable via `refresh_interval`).
- **Manual refresh** — `R` triggers immediate refresh.
- **Exponential backoff** — on rate limit or timeout, interval doubles up to 10 minutes.

## Open in browser

- **Open URL** — `o` opens the selected item's URL in the system browser.
- **Cross-platform** — `open` (macOS), `rundll32` (Windows), `xdg-open` (Linux).

## Auth detection

- **Per-forge status** — auth is checked on every refresh cycle; a forge that is not authenticated is reported on the item card (`<forge>: <reason>`).
- **Auth reason** — adapter's reason carried in notices (e.g., `not implemented` vs `not authenticated`).
- **Action guard** — actions on unauthenticated forges are disabled with reason.

## CLI print mode

- **Plain text** — `prdash --print` outputs inbox as tab-separated text.
- **Includes reviews** — mounted review worktree path appended as `review:<path>`.
- **No TUI** — exits after printing; useful for scripts and CI.
