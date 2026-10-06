# Changelog

All notable changes to prdash are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and
versioning follows [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- **`make audit-lang` — the English audit ADR 0010 promised.**
  `scripts/audit-lang.sh` flags Spanish-only tokens and accented characters in
  every tracked file, with a content whitelist for the unicode-width fixtures
  that need real accented strings. Advisory, like `make mutate-diff`: it
  reports and exits non-zero, and CI does not run it yet. The archive
  (`docs/planning/archive/`) is out of scope under its own AGENTS.md, and
  inline code spans and fenced blocks in Markdown are treated as quotations.

### Changed

- **Mutation: the code is shaped so tests can measure it (ADR 0011).** Every
  tagless `switch` became an `if/else` chain — go-cover never instruments
  case-clause conditions, so boundary and negation mutants on them were
  invisible; package-level const arithmetic became precomputed literals with
  drift pins (`TestCommentFetchKeepsTheMargin`) for the derived ones; the
  `n < 0` no-op guard in `compactCount` is gone; and boundaries that only an
  injected value can hold still (`relativeSince`, zero-height scroll windows,
  `<= 9999`) are now pinned by tests. Full-module gremlins after the campaign:
  1719 killed, one proven equivalent boundary left in the comment wrapper, and
  the 41 `cmd/prdash` positions attributed to gremlins v0.6.0
  (go-gremlins/gremlins#319), not to the suite.

- **The English migration is complete and now enforced.** 22 Spanish-named test
  files renamed, Spanish fixture values and test identifiers translated, the
  README config block and the AGENTS.md commit examples in English, and the
  language audit green over every tracked file.

### Fixed

- **`Version.AtLeast` covered its major-below branch.** Every test compared
  against a `0.x` minimum, whose `Major` is 0, so `v.Major < min.Major` never
  executed. A `1.0.0` minimum case closes it: `internal/herdr` back to 100%.

- **The authority comparison in the inbox could not be killed.**
  `rank(kind) < rank(prev)` had an equality case no input reaches — distinct
  sections have distinct ranks — so its boundary mutant survived every test.
  `assignAuthority` now keeps the first section of `sectionOrder`, which is the
  same authority order with no operator at all.


## [0.2.0] - 2026-10-06

### Added

- **`prdash worktrees remove --orphans` cleans orphaned worktrees in batch.**
  It deletes **only** what `prdash worktrees list` marks as `orphaned` (its source
  repo is no longer accessible) and it is mutually exclusive with explicit paths.
  Zero orphans is the happy path (exit 0). `--orphans --dry-run` prints the exact
  batch that would be deleted, through the same code path, without touching
  anything. An orphan whose `.git` link does not even declare a gitdir is also
  cleaned, by path or in batch, without taking the rest down.

- **The worktree of a PR merged from prdash is deleted only if it is clean.** If
  the checkout has uncommitted changes —including new untracked files— it is kept
  and the notice says so (`merged, but the worktree has uncommitted changes —
  kept`). If its git status cannot be checked, it is kept too, with a different
  notice (`worktree kept: could not read the worktree status`): when in doubt it
  is never deleted. It is the only implicit cleanup in prdash; approving,
  changing the base, a merge that goes wrong and closing the app still delete
  nothing. The full reasoning, and why the keep-always clause changes, in
  [ADR 0007](docs/adr/0007-worktree-cleanup-on-merge.md).

- **`e` changes the target branch of the PR/MR, with a searcher of the
  repository branches.** `e` opens a popup with the branches it asks the forge
  for, typeable to filter them, and the choice goes through a confirmation that
  names the two branches (`main → release/2.0`).

  Branches are listed instead of typed because changing the base to a branch
  that looks like it but is not (`main` for `main-2`) is accepted by the forge
  without complaining and goes unnoticed until the PR points at the wrong
  branch: a typo that neither the compiler nor the forge flags is exactly the
  one a searcher makes impossible. The listing is requested from the forge on
  open and cached 5 minutes per repository, so opening and closing the popup
  does not cost a call each time; `↑`/`↓` move and, with the filter empty, so do
  `j`/`k`, which as soon as there is typed text are two more letters of the
  filter. `esc` in the confirmation goes back to the list instead of closing,
  because pointing at the wrong row is the most likely mistake.

  The action enters through the same path as approve and merge —it re-reads the
  item, passes the guards, executes and re-reads—, so a merged or closed PR is
  not touched and the call is not wasted. Choosing the base the item already
  has is a no-op that never reaches the forge. The notices name the two branches
  (`retarget (main → release/2.0) ok`) and, if there was a review mounted, warn
  that **its worktree is still on the previous base**: it is neither rebased nor
  redone, because the worktree belongs to the user and may have uncommitted
  changes.

  `gh pr edit --base` is not used even though it is what `gh` documents: today
  it fails before touching anything with `GraphQL: Projects (classic) is being
  deprecated…`, the query with which `gh` checks whether the PR is in a project.
  Instead `gh api -X PATCH …/pulls/N -f base=` is used. In GitLab,
  `glab api -X PUT …/merge_requests/N -f target_branch=` and not
  `glab mr update --target-branch`, which is an editing command and whose whole
  reason for being is to open the editor. And the GitLab branch listing goes
  with `--output ndjson` because `glab api` has no `--jq`. All of this, and
  why, in [ADR 0006](docs/adr/0006-retarget-through-the-api.md).

- **A 422 rejection no longer comes out as "forge conflict", and it says why.**
  When an API call fails, the CLIs put on stderr **one line with the whole
  argv** and the real reason goes in the JSON body. `tool.Run` cut stderr and
  kept the first, so the reason the user saw was the command that failed
  —`gh api -X PATCH …: gh: Validation Failed (HTTP 422)`— and not why it
  failed. `tool.APIMessage` is added, which prefers `errors[].message` over
  `message` because GitHub writes the generic one in the former and the detail
  in the latter, so now it reads `Proposed base branch 'x' was not found`.

  The 422 also had no class of its own: since the stderr text contains neither
  "conflict" nor "not found", it fell into `network`, which the inbox translates
  to "forge conflict" —which promises a refresh that cannot fix a branch name
  that does not exist—. Now it is `validation`, which is neither conflict nor
  permission nor "the item does not exist": it is an error of the call. It
  affects **all** actions, because `kindForHTTP` is shared.

- **The merge Confirmation warns that the branches clash, and the rejection
  says rebase.** `mergeable` (GitHub) and `detailedMergeStatus` (GitLab) travel
  in the query that was already made for the item, so they **cost no call**: the
  "clashes with `main`" warning appears in the box before choosing the
  strategy, instead of appearing later as a CLI rejection. It is a **soft**
  block, like a red CI: it arms all the same and names the branch that has to be
  redone, because a rebase fixes it in one command and vetoing would leave the
  PR with no way out from here. Where the forge says nothing (GitHub's `UNKNOWN`
  while it computes, GitHub's REST fallback, GitLab's Todos API) nothing is
  announced: a conflict warning with no data is a false warning, and one that
  repeats trains you to ignore the box.

  The rejection also changes classification, and this was a bug: the text of a
  merge rejection over branches and the one for "the item is no longer as it
  was" share the word *conflict* and are **opposites**. With the single bucket,
  `forge conflict` promised a refresh that does not fix a rebase —a warning that
  does not say what to do, which is the worst way to be wrong because it looks
  actionable—. Now the branch rejection is classified separately, carries the
  canonical reason (`rebase the branch onto the target and push`) instead of the
  CLI's English, is **not** logged as a denial (a rebase makes it integrable,
  and once marked as denied the PR would never arm again) and comes out as
  `merge refused: …`.

  In GitLab `detailedMergeStatus` is requested and not `mergeStatus` because the
  simple one does not tell a conflict from a red pipeline, and a red CI is
  already announced by the gate on its own. The cost is that GitLab computes it
  per MR on every request —its REST list API returns it too—, so it is a
  computation per item and not an extra call.

- **The merge names whether it deletes the branch, and `tab` decides it.** The
  merge Confirmation —the box that replaces the shortcut bar with `m`— now
  shows `delete branch: yes (tab)`, and `tab` toggles the value before the key
  that fires it. Outside an armed merge it does not appear: it is not a decision
  that can be taken elsewhere. The default is **delete** (`--delete-branch` in
  `gh`, `--remove-source-branch` in `glab`), which is what the forges do on
  their own; `tab` turns it off for the rest of the session and `tab` again
  turns it back on. The value is per session, not per item: the housekeeping
  does not depend on which PR you have in front of you.

  The deletion goes in the **same command** as the merge, so its failure comes
  out as a failure of the whole command even though the integration is already
  done: without push, with a protected branch, or against a repo with a merge
  queue —which rejects `-d` before merging—. That is why the result is no
  longer reported as "merge failed" in that case: the re-read that the merge
  already did distinguishes "could not merge" from "it merged and the branch was
  not deleted", and the notice ends up `merge (squash) ok · branch not deleted:
  <reason>`. A fork PR either cannot be reported as branch deleted
  (`isCrossRepository`, which already came in the same item query): there is no
  branch to delete in the target repo and the forge does not complain. What the
  call does **not** touch is the local repository: with `--repo`/`-R` the CLI
  only deletes the remote branch, so bare clones and prdash's worktrees stay
  intact.

- **The path prefix of the ITEM column can be chosen with `p`.** An item's path
  was always painted with the same split —the section's common prefix on its
  line, the suffix in the cell (ADR 0002/0004)— and there was no way to choose.
  Now `p` cycles three modes, and the bar names the current one (`p prefix:
  full`) so you do not have to count keystrokes:

  - `common` (the one from before): prefix line with the common group, cell with
    the suffix. It is the default, so whoever does not press the key sees
    nothing new.
  - `full`: no prefix line and the reference in the cell, for whoever reads the
    table row by row and wants the reference on the row. **Truncated at the
    tail** when it does not fit —in a long subgroup you see
    `…kend/api-gateway#1016`— because the column has the same cap as always.
  - `leaf`: no prefix line and only the project's leaf plus the number, the
    maximum density.

  The mode is **global** (not per section) and **not persisted**: on reopening
  it goes back to `common`. The `p` key does become configurable through
  `[keybindings]`, through the generic mechanism that already existed for the
  rest of the actions — with the cost that, if you had another action on `p`,
  it now runs the prefix cycle—. The ITEM column is re-measured in every mode
  with the same bounding, and in `full`/`leaf` the list reclaims the line the
  prefix used. If the section has no common prefix, `common` looks exactly like
  `full` (ADR 0005).

  Two limits, because they are real: `leaf` **does not disambiguate** two repos
  from different groups with the same leaf (`acme/one#7` and `other/one#8` come
  out as `one#7` and `one#8`) —its value is density, not certainty— and the
  mode does not survive the next run. The full path is still in the item card
  and in `--print`, which **does not change**.
### Changed

- **The Inbox paints one section at a time, with a count legend on the edge.**
  On open, the active section is **Assigned**, and `tab` cycles
  `Assigned → Mentioned → Mine → Assigned` (always, even if the destination is
  empty). The `Inbox` title is replaced by the legend `Mine (n) · Assigned (n) ·
  Mentioned (n)`, with the active one highlighted; in a narrow terminal the edge
  truncates it without breaking the box. Each section remembers its cursor and
  its scroll, the common path prefix (ADR 0002) is shown on a fixed line of the
  active one —today that is only the default mode, toggleable with `p`, see
  above—, and `(empty)`, `loading more…` and the notices are those of the
  active section. `--print`, the cache format and the `[keybindings]` API do
  not change (ADR 0004).

### Fixed

- **The merge gate did not see draft PR/MRs.** `isDraft`/`draft` was requested
  in the item query from the start and discarded without looking, so no item
  ever arrived marked as draft. `MergeBlock` only compared `State == "draft"`,
  a value no forge emits —GitHub returns `OPEN` and GitLab `opened`—, so the
  blocking the README promised for drafts never happened: prdash armed the
  merge, spent the whole call and got the forge's rejection back. What held it
  up was a suite that injected `State="draft"` by hand, that is, it tested a
  value the real product never produced, and that is why the whole suite passed
  with the functionality broken.

  Now the draft is a property of the item (`Item.IsDraft`) and arrives through
  the four parsing paths —GitHub GraphQL and REST, GitLab GraphQL and REST
  list—, with tests that use JSON with the shape each forge really returns. The
  gate queries it regardless of the review decision: an approved draft came out
  as `approved` and the draft went unnoticed exactly on the item that gets
  watched the most. The card shows `Draft: yes/no` next to `State`, because they
  are different questions —`State` is attention priority, the draft is forge
  data— and that slot is taken by the `Number` row, which only repeated the
  number `refLabel` already shows three columns further on. In GitLab, `draft`
  comes in the same query as the rest and costs no call.

- **The simulation took up a corner of the popup and came out deformed.** The
  width of the cells and that of the box were decided by each on its own: the
  cells were computed for 96 columns and the box was drawn at the viewport
  width, so the image occupied the left third and the rest was left empty —with
  an empty edge that looked like part of the render—. Now `simBox` is the
  single source of the geometry: the box fits what the image needs, not the
  other way around (`TestTheImageFillsTheBox`).

  Besides, a terminal cell is twice as tall as it is wide, so height is paid
  double: a 16:9 image —the one git-sim produces— needs 3.56 columns per line,
  not 1.78. Without that factor the graph was stretched wide and the two
  commits in a row looked like a strip of ellipses. `sim.Fit` computes the
  largest size that fits keeping the ratio.

  And the popup is sized from the terminal instead of fixed caps: on a 240×70
  screen it goes from 94×23 to 174×49 cells —three times the detail—, which is
  the difference between an unreadable graph and a readable one, while still
  being an overlay: it keeps 75% of the height and leaves background around
  (`TestThePopupGrowsWithTheTerminal`).

  A third bug in the same vein: `simHeightShare = 3 / 4` as a Go constant is
  **0** (integer division at compile time), so the popup stayed stuck at its
  floor of 6 rows no matter how much space there was. Now they are numerator
  and denominator.
- **The left arrow started the render.** It navigated in a horizontal menu that
  does not exist —the popup offers one strategy—, so pressing it after `v`
  launched the render without confirming anything. With a single strategy there
  is nothing to traverse: the arrows fall to the default and close the popup,
  like any key that is not a choice. Navigation comes back on its own when there
  is a second strategy (`TestTheChooserNavigatesWhenThereIsSomethingToNavigate`).

- **A merge is no longer triggered by a key that was not the mode.** The second
  keystroke of the merge, not being a mode, was re-dispatched as if nothing had
  happened. That turned `m` followed by `a` into an **approve** of the PR —the
  key next to it on the keyboard, and with no confirmation at all—, and `m`
  followed by `m` into an instant merge commit. Now the key **cancels and is
  consumed**. The original intent of the old behavior —that a mistimed `m` would
  not leave the view waiting for a second keystroke— is still met: the view
  stops waiting, but without firing ANOTHER action. The test that armored the
  old behavior (`TestMergeArmedCancelsOnOtherKey`) asserted the navigation, so
  it was replaced by `TestMergeArmedConsumesOtherKey` and
  `TestMergeArmedApproveKeyDoesNotApprove` was added for the specific hole.
- **The merge goes pinned to the commit read from the item.** With
  `--match-head-commit` on GitHub and `--sha` on GitLab. Before, the command
  integrated the HEAD of the moment, and between the inbox refresh (60 s) and
  the keystroke the branch may have advanced: commits that nobody had reviewed
  were integrated, which is the worst possible outcome of an irreversible
  action. The SHA comes from the **re-read** that `RunAction` does immediately
  before merging, not from the TUI's in-memory copy, so the window is as small
  as possible. When the forge does not report the commit —`diffHeadSha` can
  come back null on GitLab— the merge **refuses** (`ErrMissingHeadSHA`) instead
  of going out unpinned: an unpinned merge is exactly the failure the pin
  fixes.
- **A red CI and requested changes are announced before the confirmation.**
  `state.Derive` already distinguished `StateError` (failed checks),
  `StateChangesRequested` and `StateDraft`, and the card shows them, but
  `Actionable` only denied `merged` and `closed`: `merge` came out the same in
  all three cases. Now `state.MergeBlock` decides at two levels:
  - **Hard block** (draft, already merged, already closed): it does not arm, and
    warns. They are forge properties —GitHub rejects merging a draft PR before
    looking at anything else— so offering it only spent a call to get an error
    back.
  - **Soft block** (red CI, running CI, requested changes): it arms, and the
    confirmation says so: `merge acme/widget#7 with CI is failing (2 of 5) ·
    press the mode anyway…`. Banning them outright would turn the tool into a
    wall —a flaky check would leave the PR with no way out—, and doing nothing
    would make them invisible.
  Choosing the mode **is** the confirmation, so a third key is not needed. On a
  healthy item no warning comes out: a gate that always warns trains you to
  ignore the warning.
- **The authentication reason reaches the screen.** `AuthState.Reason` was
  written by the adapter and was not painted anywhere: only the generic label
  `not authenticated`. With `bitbucket` (an inert adapter) that sent the person
  to authentication to look for a token that already worked. Now the notice and
  the card carry the adapter's reason, and bitbucket's says `not implemented in
  this version` instead of `not supported in this version`, because "not
  supported" and "not implemented" ask for the same thing and only one of the
  two is exact.
- **The re-read of `ReviewDecision` is normalized in the gate.** It was
  compared raw against `changes_requested`, and the forge sends
  `CHANGES_REQUESTED`: the gate never saw the requested changes in any case. It
  is compared with the same `normalize` that `Derive` already uses.
- **`MergeBlock` looks at the raw state, not the derived one.** `Derive` orders
  by attention to the operator, so a draft item that is also approved returns
  `StateApproved` and the draft was lost. The gate's question is another
  —can this forge integrate this?— and does not depend on the review decision.

### Changed

- **The merge confirmation only offers the modes the repository accepts.**
  GitHub publishes `mergeCommitAllowed` / `rebaseMergeAllowed` /
  `squashMergeAllowed` inside the item query, so the list is exact and **costs
  no call**: a repository with squash disabled does not show `s` and does not
  spend a rejection finding out. The mode is revalidated on confirm, so the
  filter is not just menu-deep.
  - In **GitLab it cannot be done** and it is not faked: `Project.mergeMethod`
    **does not exist** in the instance's GraphQL schema (checked with
    introspection), and reading it over REST would cost a call per repository.
    The rules arrive with `Known=false` and that restricts nothing, because not
    knowing is not the same as not allowing: an invented filter would leave out
    the only mode the repository perhaps does allow, and the user would be left
    with no legitimate way out.
  - The order of the list becomes `r` rebase, `m` merge commit, `s` squash.
    Rebase first because it is the only strategy that does not rewrite
    published history; it is not a default —there is no default— but it tilts
    the menu.
  - `Adapter.Merge` receives the head SHA as a new parameter. The pin being
    explicit in the signature is what prevents a new adapter from forgetting it.
- **`FakeAdapter` refuses to merge without a pin**, just like the two real
  adapters, and before recording anything. A fake that accepts what production
  rejects makes every TUI merge test cover a path that does not exist, and the
  real failure shows up in the adapter, where no test reaches. `mkItem` now
  sets a default `HeadSHA` and `MergeRules`, which is what a real forge
  returns.

### Added

- **`v` simulates the merge of the PR and shows it in a popup over the inbox.**
  It renders with [git-sim](https://github.com/initialcommit/git-sim) how the
  history would end up after integrating the PR and paints the resulting image
  **on top of** the view, so the background is still readable except where the
  box covers it. `enter` renders, `esc` closes, `o` opens the image in the
  viewer.

  It is not a gate and it is worth saying so: git-sim **draws**, it does not
  execute, and its verdict only exists inside the image, so it is no use to
  decide whether a merge clashes. It is a visualizer, and what it brings is
  understanding *why* the history ends up the way it ends up.

  Decisions that are not obvious:

  - **The whole render happens in a temporary clone** of the review's refs
    (`--shared`, with the active base branch), not in the review's worktree. The
    reason is hard: git-sim needs a `HEAD` hooked to a real branch, and the
    only way to have the active base without switching the branch of the
    worktree the review has open is another directory. The clone is deleted
    when finished, so it leaves no refs, worktrees, branches or uncommitted
    changes in the user's repo (`TestSimulateLeavesNoTraceInTheLocalRepo`).
  - **`git_sim_auto_open=false` in the environment, not a flag.** git-sim ends
    up handing the image to the desktop viewer and, without a display, that
    call never returns: it is 2 s with the variable set and a hang without it.
    The flag has no negative form in the CLI, but its `Settings` reads the
    `git_sim_*` variables, so that is where it is turned off.
  - **No `--quiet`.** Essential and counterintuitive: git-sim prints the image
    path *only* when it is not silent, so asking for both things —which is the
    reasonable thing— leaves the output empty and the simulation fails with no
    explanation. There is a test that pins it (`TestArgsNeverAskForQuietAndThePath`).
  - **The image is decoded in Go** (`image/jpeg`) and painted with half-blocks
    `▀` in truecolor, two pixels per cell, instead of depending on `chafa` or
    the system's `img2txt`: zero new dependencies and twice the vertical
    resolution, which is what makes a commit graph readable.
  - **Only `merge` is offered.** git-sim 0.3.5 does not know how to draw a
    `rebase`: with the PR's branch already based on the base —the normal case
    of a PR— it answers "Branch 'main' is already based on active branch 'feat'"
    with the message inverted, and with diverged branches it blows up with a
    Python `IndexError`. Verified in all three shapes. Offering it would be an
    option that can only fail; when upstream fixes it, `simKinds` is the only
    thing to touch.
  - **With the merge armed, `v` opens nothing**: it disarms and is consumed,
    just like any other key. Opening a modal from a mistimed keystroke is the
    same trap the armed merge already avoids.
  - Requires `git-sim` in the PATH and the review mounted (`r`); without
    either, the action warns and does nothing.
  - **The image goes to the pane's graphics layer when Herdr has one.**
    Half-blocks have a ceiling that cannot be raised: a terminal is a grid of
    cells, so a 1920 px image in 84 columns is a 23× reduction and every pixel
    becomes a 23×23 cell block — the steps you saw on the commit curves—. It is
    not a size bug: it is the grid. The way out is not to use the grid, and
    Herdr has a graphics layer per pane (`terminal.kitty_graphics`, which the
    outer terminal has to support —kitty and its derivatives do—). The image is
    published on it, already fitted to the pixels of the rectangle, and the
    terminal paints it with its own scaling. The half-blocks stay as the
    degradation path: outside Herdr, with the layer off, or if the pane does not
    answer —and in that case it falls back to half-blocks instead of leaving a
    hole—.

  What it took to know, and it is in the code:
  - The graphics API **only exists over socket**: `herdr pane graphics` is not
    a subcommand, so prdash's Herdr client (which talks over CLI) is no good;
    there is a new JSON-RPC client in `internal/herdr/graphics.go` with one
    connection per request, because the server closes it after each response.
  - **Placement is in cells**, not pixels, and it is shared with the overlay.
    That is why the box's origin lives in `centeredOrigin`: if the frame and
    the image each computed their place on their own, they would land on
    different rectangles.
  - **The cell is not 1×2.** Herdr measures it and in kitty with the default
    font it is 9×19 px. Assuming it deformed the image by 5% and, worse, made
    it send almost double the resolution that is seen. Now the pixel size comes
    from the measured cell.
  - **The layer has to be removed when the popup closes**, and in its own
    context: it lives above the pane's content, so if it stays it covers the
    whole TUI; and if the popup closes on exit, the app's context is already
    cancelled.
  - Every resize repositions the image, because the placement is in cells.
- **The last 5 comments of the PR/MR are visible in the detail.** In a box of
  its own with its "Comments" on the edge, below the card and with their
  author. They are requested from the forge when the cursor reaches the item
  and cached, so navigating does not ask again and the inbox refresh does not
  throw them away; only an action on the item invalidates them, which is the
  only thing that can write to the conversation. There is no new key: they are
  part of the card, not a separate view, and if the forge does not answer the
  panel says so (`loading…` / `not read: …` / `none`) instead of leaving a hole
  that cannot be told apart from "this PR has no comments".
  - It shows the **end** of the conversation: the last 5, in chronological
    order and from the oldest of those to the newest, which is how a
    discussion is read. That is where the last thing said about the PR is. The
    count goes on the bottom edge of the box and always says both numbers
    (`3 of 3`, `5 of 23`): the second is what tells you it is worth opening
    the PR, and the first also informs with everything in view, because the
    size of the conversation is part of the state of the PR —3 comments or 30
    is not the same PR—. Before it only came out when something was missing,
    and that filter was justified by its cost of one row, which went away with
    the count on the edge.
  - They go in a **rounded box with "Comments" on the edge**, not as more
    fields of the card: the conversation is not a datum of the PR but what
    people said about it, and a border says so without having to explain it.
    The box goes **indented one column on each side** and with **the same
    border gray as the rest** of the boxes. The indent is what makes the
    nesting readable: flush against the panel's edge, its verticals overlap
    with the outer ones and every row comes out `││`, and with the same color
    the two borders would read as one fat stroke. To separate them there was
    once a gray one shade lighter, but in warm palettes that shade comes out
    yellowish, and it was a color problem covering up one of form.
  - **No comments, no box.** A box around the word "none" separates nothing,
    and since it is the state of every PR without a conversation, a border
    appearing and disappearing on every cursor move would be noise. The usual
    field line stays. Same with `loading…` and `not read: …`: they are
    one-line messages, not conversation.
  - The box is **all or nothing**: if the budget does not allow for its two
    borders plus one row per comment, it is not painted. On a 30-row terminal
    with three comments and three free rows, the box would fit one comment and
    lose the other two; seeing one and losing two is worse than seeing none,
    because a cut of the box does not look like a cut: it looks like the PR
    only has that comment.
  - The count goes **embedded in the bottom edge, on the right**, and not
    as a loose row of the body. The body of the box is the rows people said,
    and a count row is one that belongs to nobody; on the edge, besides, it is
    free, so it is no longer the first thing to fall when the panel is tight,
    which was its fate. The border reaches the corner and does not rest on it,
    or the bottom line would read as broken. The footer (`· open the PR to
    read the rest`) only comes out if there are comments outside and if it
    fits whole: cut to half a phrase it would say less than the short one.
  - The **GitLab system notes are discarded** ("assigned to @x", "added 3
    commits"): they are not conversation but the MR's action history, and they
    filled the five rows with noise that is already elsewhere in the card.
    That is why 3× the cap is requested and the tail is truncated afterwards.
  - To make them fit, the card's fields move to a **two-column grid always**,
    not only in short terminals. In one column they took 16 of the ~18 lines
    that 40% of a normal terminal gives and not even one comment fit; in grid
    they take 6. The README comment that described the grid as an emergency
    fallback was already false.
  - The **URL leaves the grid onto a full-width row**. In half a column you
    read 40 characters of an 80-character URL and a useless remainder was
    left, and a URL you cannot copy whole is no use at all, which is what it
    is for. It does not cost much: the 13 fields took 7 rows and the 12 that
    remain plus the full-width URL are still 7.
  - `Review` and `Role` are ordered **at the end** of the status fields, so
    they fall on the last row of the grid, which is the only thing that
    survives the truncation. Pulling the URL out shifted them one position and
    `Review` was pushed out of the window: a truncated card without them stops
    answering the question it exists for.
  - The body is read **whole and by paragraphs**, not just the first line, and
    the rows are split **according to what each comment needs**: if they all
    fit, each takes its own; if not, everyone gets one row —so that all five
    are there, which is what is asked— and the leftover goes to whoever has
    the least, one row at a time. The blind split that existed before gave
    everyone the same quota and cut a six-paragraph comment to its first
    sentence while a row went spare.
  - Paragraphs are not glued to each other, what does not fit is marked with
    `…` (a row that stops mid-phrase reads as if the comment ended there) and
    the markdown HTML comments with which GitHub's bots open are skipped,
    which on a fixed-width panel would eat the row.
  - When the panel is small the comments are the **first thing to fall**,
    before a card field: they are the only thing that can be requested again
    in an instant, and a field that goes does not come back.

### Removed

- The veto on approving your own PR/MR **no longer takes a row of the card**
  ("approve unavailable: … · merge still applies"). It stays only in the
  notice, which fires on pressing the key, which is when you can act on it. The
  card painted it on every render of all your PRs —almost all of the "Created
  by me"— repeating what the `Role` field already says, and it took two rows
  away from the comments on exactly the items where they are missed most. The
  veto itself does not change: `approve` still does not come out and the
  reason is still explained in full. The forge's denial (`action disabled: …`),
  which is sticky and whose notice expires, stays on the card.

### Fixed

- Reusing a worktree whose Herdr workspace was closed no longer produces a
  review in a detached workspace. `worktree list` can return a **stale**
  `open_workspace_id`: Herdr stores it in its persisted session, so a closed
  workspace leaves the id pointing at nothing and `pane list` answers
  `workspace_not_found` (verified in 0.9.1). prdash accepted it without
  checking, was left with no base pane, and the layout opened a fallback
  `workspace create`: the review appeared as a workspace detached from the
  worktree that contains it, and a new one on every mount. That is why
  mounting a **new** PR went fine and one already existing did not. Now the id
  is validated with `pane list` —which by the way gives the base pane and saves
  a trip— and if it does not answer it falls back to the usual adoption.
- A failing `pane list` no longer silently degrades into opening a different
  workspace. If the container says the worktree lives in a workspace and that
  workspace does not answer, the mount fails naming the id instead of mounting
  the review somewhere that is not its home. It is the difference between a
  readable error and a ghost workspace.
- The review no longer shows up as a detached Herdr workspace. `herdr worktree
  create` refuses to open a path that already exists (`fatal: '…' already
  exists`, verified in 0.9.1), so when reusing a checkout from a previous
  session prdash had no way to create the native workspace: `worktree list` did
  not return `open_workspace_id`, the worktree came back without a container
  and the layout fabricated its own `workspace create`. The result was a
  workspace detached from the worktree that contains it, and a new one on every
  mount. Now reuse **adopts** the checkout: if there is no open workspace, one
  is opened with cwd on the worktree itself, which is what Herdr records as its
  workspace (checked against 0.9.1: `open_workspace_id` appears). If not even
  that is possible, the mount fails naming the path to clean instead of faking
  success.
- The Hunk pane reviews the **working tree** (`hunk diff` without revspec)
  instead of the PR diff against the target branch. The pane shares its tab
  with the editor and the agent, so what matters is what is being touched: the
  PR diff is a fixed target that does not move while you edit and hides work in
  progress. The PR diff remains available through `[commands].hunk`
  (`hunk diff main...HEAD`). The target branch is not lost: it still reaches
  the pane through `PRDASH_BASE`.

### Changed

- The mount of `r` opens **two tabs** instead of a three-pane layout: `Review`
  (TUICR + editor) and `Edit` (Hunk + agent), with the two panes of each tab at
  50%. Reading and editing are two different modes of attention and putting
  them in the same grid made the diff, the review and the agent fight for the
  same space. The first tab **renames** the tab the worktree already brings
  instead of creating a new one, precisely so as not to leave an orphan tab the
  user would have to remember to close; Herdr creates the second one with
  `tab create --no-focus`. A tab left with no panes is not opened: a blank tab
  is noise, not a layout.
- The editor pane is new and its command comes from `[tools].editor` (default
  `vi`), with verbatim override through `[commands].editor`. Unlike tuicr,
  hunk and the agent, it is **never omitted**: its command is a shell command
  and the normal case is exactly the one a binary check cannot see — the `vi`
  that expands to `nvim .` in your rc does not exist in `PATH`, so looking for
  it would declare it absent and the review tab would be left with a single
  pane with no explanation. A badly written command is visible in the pane
  itself, which is when the user is looking at it.

### Removed

- **Out with the Herdr plugin.** It was not needed for anything prdash
  promises: the worktree and the panes are opened by the binary itself by
  calling Herdr's CLI as a subprocess (`herdr worktree create`,
  `herdr pane split`, `herdr pane run`). The plugin was only the entry point
  Herdr used to call prdash. Gone go the manifest, the
  `prdash herdr <inbox|mount|link>` subcommands and their dispatch.
  - With that also falls the selection persistence
    (`internal/selection`, `SetSelectionPath`, `trackSelection`): it existed
    solely so the plugin's global key could mount the last selection from
    another pane, and without the plugin nobody reads it.
  - **Lost** is Ctrl+click on a PR/MR URL to mount it, and the Herdr key
    that mounted the selection from outside prdash. For the second, jump to
    the prdash pane and press `r`. There is no alternative for the first:
    link handlers are exclusively plugin.
  - **Unchanged** is how the inbox opens. It is still `prdash`, and it still
    mounts worktrees and panes the same way. All that is needed is to launch
    it from a Herdr pane (the client requires `HERDR_ENV=1`, which Herdr only
    injects into its children); a shortcut in `config.toml` is a convenience,
    not a requirement.
  - `internal/herdr/` **stays**: the orchestrator still uses it for the pane
    layout and native worktree provisioning.

### Added

- Merge with choosable mode and double confirmation. `m` no longer merges on
  the first press: the first keystroke only arms, the Keybinds box is replaced
  by the confirmation, and the second key **is** the mode choice — `m` merge
  commit, `r` rebase, `s` squash, `esc` cancel. There is no default mode: a
  merge rewrites history and cannot be undone with a command, so no path
  should exist that fires it with a strategy the user did not name. Any other
  key disarms and does what it normally would, so that a mistimed `m` does not
  leave the view waiting for the second keystroke; `q` and `ctrl+c` keep
  closing. The guards are checked on arming and not on confirming, and the
  arming pins the item: if a refresh repositions the cursor in between, the
  merge comes out over whatever was confirmed or does not come out at all. The
  notices name the mode at the start and at the end, because a bare "merge ok"
  does not say whether the rebase nobody asked for was applied. Each forge
  translates the mode to its flag: `gh pr merge --merge /
  --rebase / --squash` and `glab mr merge` with `--rebase` / `--squash` or
  with no strategy (merge commit is there the absence of a flag). An unknown
  mode is a warning and does not launch the CLI, because `gh pr merge` with no
  strategy flag opens a prompt that hangs in a non-interactive subprocess.
- Key scheme reorganized: `r` mounts the review, `R` refreshes, `m` merges and
  `a` approves. Before `m` was the review and `M` the merge, so the key of the
  destructive action and that of mounting lived on the same finger. A test
  checks that the defaults do not share a key: `ActionForKey` resolves in
  alphabetical order, so a collision leaves a dead action with no warning.

### Fixed

- GitLab no longer reports "merge ok" without having merged. `glab mr merge`
  has `--auto-merge` **true** by default, so with a pipeline in flight the
  command did not merge: it only queued the MR for auto-merge and exited with
  0. prdash now always passes `--auto-merge=false`. It was a silent bug in the
  most awkward direction possible: the UI confirmed something that had not
  happened.

- The shortcut box no longer hides two keys that did work. `m` (mount review)
  and `o` (open in the browser) were bound and operational, but they were
  never painted: the bar was composed with a hand-written list of six actions
  that had fallen out of sync, while the config did know about all eight. Now
  there is a single source —`config.Hints()`, an ordered list out of which
  come both the configurable actions and the fixed keys—, and a test watches
  that no `[keybindings]` action is left out, so the skew cannot sneak back in
  silently.
- `section-next` now answers its own shortcut. It was wired to `tab` above
  the dispatcher, so rebinding it in `[keybindings]` announced a key in the
  shortcut bar that did nothing. It now comes from the configured map.
- The quit key survives the truncation of the bar. With a narrow terminal the
  shortcut lines are capped at three and the tail was lost, which was exactly
  where `quit` was —the code's own comment said it could not be missing, while
  placing it where it is easiest to lose—. `quit` now opens the list, and
  since the truncation throws away from the end it is the last thing to fall.

- Out with the full-screen detail view and its `enter` key. The bottom 40%
  panel is now always visible and moves with the cursor, so the card had
  nothing to add and only cost an `open/closed` state that had to be
  maintained, synced with the refresh and closed with `esc`. The detail is now
  always read in the panel, which when it does not fit whole switches to a
  two-column grid before truncating fields. Gone with it is the `detail`
  action of `[keybindings]`.

### Added

- Diffstat in the detail and in the list: how many lines a PR/MR adds and
  deletes, and across how many files. They come from the data the inbox
  already brought, so they cost no extra call: on GitHub they are the PR's
  `additions`, `deletions` and `changedFiles` scalars, and on GitLab the MR's
  `diffStats`. The detail shows them uncompact (`+381 -36 (11 files)`); the
  DIFF column of the list abbreviates them so the width does not depend on the
  size of the change (`+381 -36`, `+1.2k -6.7k`). What is added goes in green
  and what is removed in red, both in the column and in the detail: it is diff
  notation and nothing more — it does not say whether the change is good, only
  which lines are new and which disappeared, and that is why the file count
  stays uncolored. A diffstat the forge did not report is not colored: it is
  an absence, not a figure.
- Transient notices (toasts) overlaid at the bottom right of the view, with
  their own expiry (4 s) and a 500 ms tick. The text wraps by words and the
  box is bounded to the usable width, so a long notice never overflows or
  misaligns the view. The clock is injectable: the expiry is tested without
  sleeping.
- Always-visible detail panel: the list stays on top with scroll and the
  detail of the selected item takes the bottom 40%, separated by a rule. The
  view fills up to its reserved height, so neither the table nor the shortcut
  bar moves when an item is added or removed.

### Changed

- The DIFF column goes last and is the first thing omitted when it does not
  fit: at 124 columns of terminal with long project paths there is no room for
  the seven columns, and rather than truncating a title, a datum that the
  detail always brings is dropped. It appears with a wide terminal (~133). In
  the detail the criterion is the same: if the panel does not fit, the
  diffstat is omitted instead of pushing the title out.
- A diffstat the forge did not report is told apart from one of zero lines.
  GitLab's Todos API and GitHub's REST fallback do not bring it, so those items
  show `-` in the list and `unknown` in the detail instead of a `+0 -0` that
  would look like an empty PR. GitLab's file count comes from the length of
  `diffStats`, and since the forge collapses diffs that exceed its limit, on a
  huge MR the figures are a minimum.
- The ITEM column no longer truncates the project path at the head. Each
  section declares in its header the path prefix its items share and the rows
  only paint the suffix (`mobile-frontend#1198` instead of
  `APPCITTI/vsocial/backend/mobile-…`). The prefix is aligned on `/` boundaries
  and never eats the final segment, so the cell always keeps the project name
  and its number. The column width comes from the content —the longest suffix
  in the inbox plus its separator slot, capped at 34 runes of slot— and
  whatever still does not fit is truncated at the tail, never at the front.
  The detail (`enter`) and `--print` keep showing the full path. Decision and
  alternatives in [ADR 0002](docs/adr/0002-item-column-common-prefix.md).
- The table columns no longer stick to each other. A column's width includes
  its separator slot, so the text is truncated one rune before filling it:
  before, any text that measured exactly the width —ROLE with `review req`,
  or ITEM with its dynamic width— ended up glued to the next column.
- The table cell now accepts several spans with their own style, which is what
  allows the two colors of the DIFF column. The width is still measured in
  plain text —the padding goes at the end of the last span— so the ANSI codes
  do not break the table's alignment. The detail colors after measuring and
  truncating, for the same reason.

### Fixed

- The query for a specific MR no longer always fails on GitLab.
  `mergeRequest(iid: 7)` passed the iid as an integer literal, but the schema
  declares it `String!` and GraphQL does not coerce Int to String, so the
  response was an `argumentLiteralsIncompatible` and nothing else. Since
  `ItemState` is the refresh done after `approve` and `merge`, acting on an MR
  left the item unrefreshed and with a parse notice. The tests did not catch it
  because the fake runner returns JSON without validating the query; now there
  is a test that pins the literal's type. Do not confuse it with the response's
  `flexInt` of the `iid`.
- Approving your own PR/MR is no longer attempted: the TUI cuts it before
  calling the CLI, saving the three calls per keystroke. The user's identity
  comes from the session probe that was already done (`gh auth status` /
  `glab auth status`), with no new calls; if it cannot be read, it decides by
  section. `merge` does not change: it still applies to your own PR/MRs.
- The self-approval rejection is no longer confused with a conflict or a
  network failure: `forge conflict: gh pr review … (exit 1)` was in reality
  `GraphQL: Review Can not approve your own pull request`. It is classified as
  a permanent denial and shows the reason, not the CLI's stderr.

## [0.1.0] - 2026-09-25

First version: cross-forge inbox (F1) and review orchestrator (F2). It matches
the plugin manifest version (`plugin/herdr/herdr-plugin.toml`).

### Added

**F1 — Cross-forge inbox** ([#1](https://github.com/Sovengar/prdash/pull/1)):

- TUI (bubbletea v2) with three sections —created by me, requested/assigned
  review and mentions— with identity dedupe and section precedence.
- Forges GitHub (via `gh`, GraphQL with `reviewDecision` and checks) and
  self-managed GitLab (via `glab`, GraphQL + Todos API, REST under
  `/git/api/v4/`).
- Item detail (title, author, branches, number, URL, review and checks)
  without leaving the TUI.
- Manual (`r`) and automatic (60s configurable) refresh with last-update
  indicator per forge, backoff on rate limit/timeout and a guard not to
  clobber an action in progress.
- Unbounded pagination with progressive loading, snapshot in `cache` and
  incremental refresh by cursor.
- approve/merge actions via `gh`/`glab` (or delegating to `tuicr`), with
  conflict handling and item re-read.
- Honest degradation: explicit state per forge/section (never "empty" if
  there was an error); Bitbucket present as a non-operational adapter, no
  network.
- `prdash --print` mode (plain text, same order as the TUI).

**F2 — Review orchestrator** ([#2](https://github.com/Sovengar/prdash/pull/2)):

- Resolution of the local repo, configurable bare clone, fetch of the review
  ref (`refs/pull/N/head`, `refs/merge-requests/N/head`, including forks) and
  local working branch.
- Worktree provisioning with two interchangeable implementations: direct git
  (outside Herdr) and Herdr-native (inside), with reuse of the existing
  worktree and several worktrees per repo.
- 3-pane layout over the worktree (TUICR, Hunk, opencode agent), with a
  warned omission of missing tools.
- Herdr plugin (`plugin/herdr/herdr-plugin.toml`): inbox pane, `mount-review`
  action, PR/MR URL link handler and `prdash herdr inbox|mount|link`
  subcommands.
- `m` shortcut to mount the review of the selected item; `prdash.mount-review`
  with no URL resolves the selection persisted in
  `$XDG_STATE_HOME/prdash/selection.json`.
- Degradation outside Herdr: F1 stays operational and the layout is reported
  as unavailable.
- `prdash worktrees [list|remove]` command: lists the worktrees with
  `prdash-…` ownership, marks orphans and deletes only on explicit request
  (never other people's); the worktrees are kept when the app closes.

### Known limitations

- **F3 (auto-review with gate and allowlist) not included**: only
  design/milestone documented in `docs/planning/archive/0001-mvp/f3-milestone.md`.
- Pane readiness with `herdr pane wait-output` pending: panes are launched
  fire-and-forget.
- `herdr worktree create` also opens the source repo's workspace; its
  close/management is pending.
- Residual non-blocking LOWs from the adversarial reviews of F1 (PR #1) and
  F2 (PR #2).
- Out of scope: functional Bitbucket, functional gitlab.com, the "all open"
  view, webhooks/daemon, local repo management beyond the worktree and
  multi-user.
