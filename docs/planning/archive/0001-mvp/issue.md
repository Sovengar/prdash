# 0001 — prdash MVP: multi-forge inbox + review orchestrator (F1+F2)

## Problem

Whoever reviews PRs/MRs spread across GitHub and a self-managed GitLab lives jumping from web to web: each forge has its own UI, its own state and its own notion of "pending for me". The result is that the real inbox is split across two tabs, context is lost between them and every review requires assembling the environment by hand (branch worktree, diff, agent, comments). There is no single point that answers "what do I have to review now?" nor that prepares the review ground automatically.

prdash exists to close those two gaps in a single tool: a cross-forge inbox that unifies what falls to me, and an orchestrator that, when a PR/MR is chosen, lefts the worktree and the review layout over Herdr ready so that the "I comment → the agent applies" loop happens without friction.

## Scope (in)

**F1 — Cross-forge inbox.**
- Go TUI (bubbletea v2) with three sections: *(1)* created by me, *(2)* review requested / assigned to me, *(3)* mentions.
- Working forges: GitHub (`github.com`, via `gh`) and self-managed GitLab (`gitlab.example.com`, REST under subfolder `/git/api/v4/`, via `glab`).
- Rich GitHub state via GraphQL (`gh api graphql`: `reviewDecision`, checks); GitLab via GraphQL (`glab api graphql`: `currentUser` authored / reviewRequested / assigned) + Todos API for mentions.
- Fast approve/merge from the inbox with direct `gh`/`glab`; they delegate to `tuicr` when applicable (tuicr already pushes real reviews via gh/glab).
- Documented forge adapter interface, with Bitbucket *interface only* (compiles, no real calls).

**F2 — Review orchestrator per PR/MR.**
- On selecting a PR/MR: create the worktree of its branch and mount the layout on Herdr with three panes: TUICR over the PR, Hunk (diff) and the opencode agent.
- Comment loop: the user comments in TUICR/Hunk; the agent reads them and applies them.
- Herdr plugin: Go binary + `herdr-plugin.toml` (pane entrypoint with placement, keybind and link handler for PR URLs). Target Herdr 0.9.x.
- Clean degradation outside Herdr: without `HERDR_ENV=1` the app reports and does not break.

## Out of scope (out)

- Working Bitbucket (only the adapter interface).
- Working gitlab.com (no token; left as future).
- F3 auto-review/auto-approve: design and post-MVP milestone only, no implementation.
- "All open PRs" view (the scope is my inbox, not someone else's backlog).
- Webhooks, server daemon or any push component.
- Local repo management (prdash neither discovers nor administers them beyond the review worktree).
- Multi-user / multitenancy: assumes one identity per forge already authenticated.

## Acceptance criteria

- [ ] The TUI starts and shows the 3 sections with real data from GitHub and from the self-managed GitLab.
- [ ] Every listed PR/MR reflects the state of the forge (e.g. `reviewDecision`/checks on GitHub, authorship/review-request on GitLab) without leaving the TUI.
- [ ] Mentions come from the corresponding source per forge (GraphQL on GitHub, Todos API on GitLab).
- [ ] Selecting a PR/MR creates the worktree of its branch.
- [ ] The Herdr layout opens the 3 expected panes (TUICR, Hunk, opencode agent).
- [ ] A comment written in TUICR is readable by the agent and it applies it.
- [ ] Fast approve/merge works from the inbox on GitHub and on the self-managed GitLab, or delegates to `tuicr` when applicable.
- [ ] Outside Herdr, the app reports the limitation and does not break (F1 stays operational).
- [ ] The Bitbucket adapter compiles and is documented, with no real call at all.
- [ ] `go build ./... && go vet ./... && go test ./...` passes and the binary is installable on PATH (`~/.local/bin/prdash`).

## Open questions (block approval)

1. **PR → local clone**: to create the worktree of a PR, prdash needs the local path of the repo of that PR (`owner/repo` or GL project → folder). It is not defined how it is resolved. Default proposal: `roots` in the TOML config (gitdash pattern) + remote→local index built when scanning those roots, and if there is no match, ask for the path once and remember it.
2. **Inbox refresh cadence**: automatic poll or manual refresh? Default proposal: manual refresh (`r`) + auto every 60s configurable, with a "last update" indicator.

## Risks / pending verifications

- **`tuicr pr` submit against the self-managed GitLab**: verify that tuicr pushes real reviews via `glab` on `gitlab.example.com` and not only on GitHub.
- **`herdr worktree create --branch` with a remote branch that is not local**: confirm the behavior when the PR branch does not yet exist in the local clone (implicit fetch, failure, or need of a previous fetch).
- **approve/merge permissions on the self-managed**: verify that the user (`<user>`) can approve/merge, or the quick button/action must be disabled.
- **Placement of the Herdr plugin pane**: validate that `herdr-plugin.toml` places the panes as expected on Herdr 0.9.x and that the link handler resolves PR URLs.
- **Table cell width/styles**: ANSI render and width computation can break the inbox table (known pattern: `pad()` before applying style).
- **Clean degradation**: check that `HERDR_ENV=1` detection fails soft and does not block the TUI startup.
