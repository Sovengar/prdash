# ADR 0003 — prdash is not a Herdr plugin

- **Status**: Accepted
- **Date**: 2026-09-26
- **Decider**: user (buble)
- **Scope**: integration with Herdr, review assembly, `internal/herdr/`
- **Naming pattern**: `docs/adr/NNNN-slug.md`
- **Supersedes**: nothing. The plugin design never had an ADR; its plan stayed
  documented in `docs/research/herdr-0.9.1-contract.md` §F2 flow, which this
  ADR makes obsolete as far as the plugin is concerned.

## Context

prdash was born intending to be a Herdr plugin: a
`plugin/herdr/herdr-plugin.toml` with `[[actions]]` (assemble the review),
`[[panes]]` (the orchestrator's pane) and `[[link_handlers]]` (Ctrl+click on a
PR/MR URL), plus a dispatch `prdash herdr <inbox|mount|link>` and persistence
of the selection in `internal/selection/`.

On implementing it, it became clear the plugin bought nothing of what prdash
promises. Its only role was being **the entry point Herdr used to call
prdash**; everything behind that — resolving the repo, creating the worktree,
opening tabs and panes, launching the tools — the binary can do it by calling
Herdr's CLI as a subprocess (`herdr worktree create`, `herdr tab create`,
`herdr pane split`, `herdr pane run`). The `root_pane` returned by
`worktree create` and the `root_pane` returned by `tab create` are the same
base pane, so the layout does not need its own pane to anchor it.

The consequence of removing it is not only about code: **functionality is
lost**, and that is the part that has to be weighed.

## Decision

**prdash is not a Herdr plugin.** It is installed and launched as a binary
(`make install`, `prdash`) and talks to Herdr through the CLI. With that:

1. Gone go the manifest (`plugin/herdr/herdr-plugin.toml`), the subcommands
   `prdash herdr <inbox|mount|link>` and their dispatch.
2. Gone go the `plugin-link` / `plugin-unlink` targets in the `Makefile` and
   the `PLUGIN` var. `plugins.json` stops being a file derived from prdash.
3. Selection persistence (`internal/selection`, `SetSelectionPath`,
   `trackSelection`) goes away: it existed solely so that the plugin's global
   key could assemble the last selection made from another pane, and without
   the plugin nobody reads it.
4. `internal/herdr/` **stays**: the orchestrator still uses it for native
   worktree provisioning and the pane layout. It changes its entry point, not
   its purpose.
5. The layout goes from 3 panes to **2 tabs** (`Review`: TUICR | editor,
   `Edit`: Hunk | agent), which is what fits in the real width of a pane.
6. A shortcut in `config.toml` is a **convenience, not a requirement**.
   prdash does not edit the user's `config.toml`. Launched by hand from a
   Herdr pane it works the same; launched from a regular terminal, `r`
   degrades to plain git and mounts the worktree without panes.

## Rejected alternatives

- **Keeping the plugin just for the Ctrl+click.** It is the only thing the
  plugin really bought. It was rejected because link handlers are exclusively
  plugin territory: there is no way to keep that half without the whole
  manifest, and the whole manifest is exactly what is not wanted. The
  Ctrl+click on a PR URL is left lost with no alternative, and it is a cost
  consciously accepted.
- **PluginActions / Herdr commands as an intermediate entry point.** A lighter
  mechanism than the full manifest. It was not evaluated in depth because the
  layout already works without it, and adding it later is a commit, not a
  migration.
- **`config.toml` editor from prdash** (the hunkdiff `setup-keys` pattern,
  with backup and `herdr config check`). Rejected separately: touching the
  user's `config.toml` is not needed for anything to work, and it is surface
  for conflicts and permissions that a convenience does not pay for.
- **RConsole backing selection persistence for a Herdr keystroke.**
  Unnecessary: the selection state already lives in the TUI, and the secondary
  shortcut is resolved by jumping to the pane and pressing `r`.

## Consequences

**Positive**

- A single artifact to install (`prdash`) and a single process. No
  `plugins.json`, no `min_herdr_version` to keep up to date, nothing to
  register in the server config.
- The layout stops depending on Herdr creating a pane for the orchestrator: it
  uses the base pane that already comes with the worktree and the tab, and
  reuses the existing tab instead of leaving orphan tabs.
- Assembly is explicit and observable: every command prdash runs can be read
  in `internal/herdr/client.go` without going through a plugin runtime.
- Less code: gone go the dispatch, `internal/selection/` and, with this
  decision, `internal/forge/parse/url.go` (the URL parser for the link
  handler) and `forge.Registry` (adapter registry for the manifest). None of
  them had callers in production.

**Negative / costs**

- **Ctrl+click on a PR/MR URL no longer assembles the review.** There is no
  alternative: Herdr's link handler only exists inside a plugin. The remedy
  is manual: copy the URL and assemble it in the pane.
- **The Herdr key that assembled the selection from outside prdash no longer
  exists.** The remedy is to jump to the prdash pane and press `r`.
- The 2-tab layout depends on the pane width. In a narrow one, the editor pane
  and the TUICR pane become unreadable; the design assumes the review is read
  with the workspace enlarged.
- The ability to declare itself to Herdr is lost: if Herdr shows the installed
  plugins, prdash does not appear, and there is no surface of its own for
  future actions.

**Verification**

- `CHANGELOG.md` §Removed → "Out with the Herdr plugin" describes the change
  and its consequences for the user.
- `README.md` §"Inside Herdr" states explicitly that prdash is not a plugin
  and what is lost.
- `docs/research/herdr-0.9.1-contract.md` §F2 flow is rewritten to the current
  design; §Degradation on missing capability reflects that there is no path
  below 0.9.0.
- `internal/herdr/client_test.go` covers the mutation veto by minimum version,
  and `internal/herdr/layout_test.go` the 2-tab layout and its tolerance to
  partial failures.
