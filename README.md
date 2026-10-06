# prdash

Multi-forge PR/MR inbox + review orchestrator on top of [Herdr](https://herdr.dev).

It answers "which PR/MR is mine?" by mixing GitHub and a self-managed GitLab into
a single inbox, and when you pick an item it leaves the review environment ready
(worktree + 2-tab layout) so that the comment loop happens without assembling
anything by hand.

Status: **MVP F1 + F2**. F3 (auto-review with gate and allowlist) is a documented
milestone, not implemented: see `docs/planning/archive/0001-mvp/f3-milestone.md`.
Version history: [`CHANGELOG.md`](CHANGELOG.md).

## Requirements

- Go 1.26+ to build.
- `gh` authenticated against GitHub and `glab` authenticated against the
  self-managed GitLab.
- Optional: [Herdr](https://herdr.dev) 0.9.x inside the session for the review
  layout. Outside Herdr, the inbox (F1) stays operational and the review
  assembly reports that it needs Herdr.

Test PR 1/10: smoke note to practice the PR resolution cycle.

## Build and installation

```sh
make build      # compila en ./bin/prdash
make install    # instala en ~/.local/bin/prdash
make config GITLAB_HOST=gitlab.miempresa.com   # crea ~/.config/prdash/config.toml
```

The config lives in `$XDG_CONFIG_HOME/prdash/config.toml`. A missing or
malformed file degrades to defaults with a warning; it never aborts.

### Forges and relative URL root

Each `[forge.<name>]` accepts `host` and `clone_base`. `clone_base` is the
relative URL root where the instance publishes the clone/web when it is **not**
at the host root (e.g. `clone_base = "git"` for `https://host/git/…`). Empty =
root.

On GitLab, `api_base` (the REST base) is the usual source of that prefix: the
relative URL root is **derived** by stripping the `api/v4` suffix
(`"/git/api/v4/"` → `git`; `"/api/v4/"` → root). `clone_base` is an explicit
override and, with `clone_base = "/"`, forces the root. The default
`api_base = "/api/v4/"` assumes the GitLab instance sits at the root.

```toml
[forge.gitlab]
host = "gitlab.miempresa.com"
# Instancia en subcarpeta: https://gitlab.miempresa.com/git/grupo/proyecto
api_base = "/git/api/v4/"   # deriva clone_base = "git"
# clone_base = "git"        # or explicit (wins over api_base)

[forge.github]
host = "github.com"
# GitHub Enterprise en subcarpeta:
# clone_base = "ent"
```

## Usage

PR de prueba 7/10: nota de humo para practicar el ciclo de resolucion de PRs.

```sh
prdash            # the inbox TUI
prdash --print    # the inbox as plain text (includes the worktree path of the
                  # reviews already mounted), without opening the interface
prdash worktrees  # lists the review worktrees owned by prdash
```

Default keys: `j`/`k` move, `pgup`/`pgdn` page, `home`/`end` ends, `tab` switches
section (Assigned → Mentioned → Mine), `p` prefix mode (see
[Path prefix](#path-prefix-three-modes-with-p)), `r` mount the review (the
worktree always; the 2-tab layout requires Herdr), `R` refresh, `a` approve, `m`
merge, `e` change the target branch, `v` simulate, `o` open in the browser, `q`
quit. They are configurable in `[keybindings]`. With the merge armed,
`m`/`r`/`s` pick the strategy, `tab` toggles whether the branch is deleted and
any other key cancels (see [Merge](#merge-takes-two-keys-and-one-of-them-is-the-mode)).

The Inbox paints **one section at a time**: on open it shows **Assigned**, and the
top border carries the counts legend `Mine (n) · Assigned (n) · Mentioned (n)`,
with the active one highlighted. Each section remembers its cursor and its scroll.

### Path prefix: three modes with `p`

The ITEM column does not always show the path the same way. `p` cycles through
**three modes**, and the mode name shows up in the keybar (`p prefix: full`) so
that you never have to count keystrokes:

| Mode | The prefix line | The ITEM cell |
|---|---|---|
| `common` (on open) | with the active section's common prefix | the suffix only: `api-gateway#100` |
| `full` | not painted | the whole path, tail-truncated if it does not fit: `…kend/vsocial-api-actuacions#1016` |
| `leaf` | not painted | the leaf only: `vsocial-api-actuacions#1016` |

In `full` and `leaf` the list reclaims the line the prefix used, and the ITEM
column adjusts to what it shows now. Watch out with `full`: the reference of a
long subgroup does not fit at the top of the column, so it is **tail-truncated**
and you see `…kend/api-gateway#1016`, not the whole path. And since ITEM reaches
the top, it is the mode that tolerates a narrow terminal worst: below ~50 columns
the column disappears and the table keeps only FORGE. If the section has no
common prefix (a single item, or nothing in common), `common` looks the same as
`full`: it neither invents a prefix nor repeats the path.

Three honest caveats: the mode is **not persisted** (on reopen it goes back to
`common`), although the `p` key itself is configurable via `[keybindings]` —and
if you had another action bound to `p`, you lose it now: the keybar and the cycle
belong to `prefix-mode`. And `leaf` **does not disambiguate**: two repos from
different groups with the same leaf look the same (`acme/one#7` and `other/one#8`
→ `one#7` and `one#8`). To read the full path there is the item card and
`--print`. The decision and its alternatives are in
[ADR 0005](docs/adr/0005-selectable-prefix-mode.md).

### Simulation (`v`)

`v` opens a popup that renders with [git-sim](https://github.com/initialcommit/git-sim)
how the history would end up after integrating the PR, and shows it **on top** of
the inbox: the background view stays visible except where the box covers it.
`enter` renders, `esc` closes and `o` opens the image in the system viewer.

It is not a gate: git-sim draws, it does not execute, and its verdict only exists
inside the image. It is a viewer. To know whether a merge **collides**, `v` is no
use.

Requirements and limits:

- It needs `git-sim` on the PATH (with `manim`, `cv2` and Python). If it is not
  there, the action warns and does nothing.
- It needs the review mounted (`r`): the refs only exist locally after the
  `fetch`. Without it, it warns.
- The whole render happens in a **temporary clone** of those refs, with the base
  branch active, and it is deleted when done. It does not touch the review's
  worktree nor the user's clone: it leaves no refs, worktrees, branches or
  uncommitted changes behind.
- Only `merge` is offered. git-sim 0.3.5 does not know how to draw a `rebase`: if
  the PR branch is already based on the base —the normal case— it answers with an
  inverted message, and if they diverge it blows up with an `IndexError`. When the
  project fixes it, the strategy list in `internal/tui/sim.go` is the only thing
  that needs to change.
- The box is sized to what the image needs while keeping its aspect ratio, and it
  settles at 75% of the terminal height, leaving background around it.
- **Inside Herdr with `terminal.kitty_graphics` on** (and an outer terminal that
  supports it, like kitty), the image is published on the **graphics layer of the
  pane** and the terminal paints it at native resolution. That is what removes the
  mosaic look: the half-blocks are quantized to the cell grid, so a 1920 px image
  on 84 columns came out with every pixel turned into a 23×23-cell block.
- **Without Herdr, or with the layer off or unresponsive**, the image is painted
  with half-blocks in truecolor (two pixels per cell). It looks pixelated —it is
  the ceiling of a character grid— but it is the honest degradation. `o` opens it
  in the viewer in any case.
- The images are kept in `$XDG_CACHE_HOME/prdash/sim` (the last 20).

The screen splits in two: the scrollable list on top and the detail of the
selected item in the bottom 40%, which moves with the cursor. There is no
full-screen view: the panel is all there is. The card is three blocks: the short
fields in a two-column grid, the URL on a full-width row, and below that the
comments in their own box.

The fields **always** go in a grid, not only when they do not fit in one: in a
single column they took 16 of the ~18 lines that 40% of a normal terminal grants,
and not even one comment fit. The URL comes out of the grid because in half a
column you read 40 characters of an 80-character URL, and a URL that cannot be
copied in full is good for nothing. It costs no height: 12 fields in two columns
are 6 rows, the same 7 that the 13 used to take.

### The latest comments, in their own box

Below the card, up to 5 comments of the conversation are shown, in a rounded box
with "Comments" on the border. They are asked of the forge **when the cursor
arrives at the item** and cached: moving up and down does not ask again, and the
inbox refresh does not drop them (a conversation does not change at the pace of a
one-minute cycle). The only invalidation is an action on the item, which can in
fact write to the conversation.

They are part of the card, not a separate view: there is no key to press. If the
forge does not manage to answer, the panel says so (`loading…`, `not read: …`,
`none`) instead of leaving a gap, because a gap cannot be told apart from "this
PR has no conversation".

What is shown, and why:

- **The latest, not the first ones.** The end of the conversation is where the
  last thing said about the PR and the current state of the discussion are. They
  run from the oldest of those to the newest, which is how a discussion is read.
  The bottom border of the box carries the count (`3 of 3`, `5 of 23`): the total
  is what tells you it is worth opening the PR, and the number of the ones you see
  also counts, because the size of the conversation is part of the state of the PR.
- **One box, not more fields.** The conversation is not a datum of the PR but what
  people said about it, and a border says so without explaining it. It is indented
  one column on each side and uses the same border grey as the rest of the boxes:
  the indent is what says it is nested inside the panel. No comments —and also
  nothing yet from the forge— means no box: the usual field line stays, because a
  box around the word "none" separates nothing and it would appear and disappear
  with every cursor move.
- **The box is all or nothing.** If its two borders plus one row per comment do
  not fit, it is not painted. Seeing the whole card is better than a box with a
  single comment, because a clipped box does not look like a clip: it looks like
  the PR only has that one.
- **System notes out.** On GitLab, "assigned to @x" or "added 3 commits" is not
  conversation: it is the MR's action history, and mixed with what people wrote it
  would eat the five rows with noise that already lives elsewhere in the card.
- **The whole body, by paragraphs.** The rows are distributed according to what
  each comment needs: if they all fit, each one takes its own; if not, everyone
  gets a row —so that all five are there— and the leftover goes to whoever has the
  least. Paragraphs are not glued to each other, because "fix the timeout fix the
  backoff" says nothing, and whatever does not fit is marked with `…`.
- **No boilerplate.** The GitHub bots open with an invisible HTML comment
  (`<!-- ssf: origin=… -->`) that in a fixed-width panel would eat the row.

When the panel is too small the comments are the first thing to go, before a
field of the card: they are the only thing you can ask for again in an instant,
and a field that goes away does not come back.

### Why `approve` says nothing on the card

Approving your own PR is not allowed by any forge, and the veto does not show up
in the detail: only in the notice, when you press the key, which is when you can
act on it.

The card used to paint it on every render of all your PRs —almost all of them
"Created by me"— repeating what the `Role` field already says, and it took two
rows away from the comments on exactly the items where they are missed most. The
veto keeps working the same: `approve` does not show and the reason is explained
in full. What does stay on the card is the forge's denial (`action disabled: …`),
which is sticky while its notice expires.

### Merge takes two keys and one of them is the mode

`merge` does not run on the first press. The first keystroke only arms: the
Keybinds box is replaced by the confirmation and nothing is left in flight. The
second key **is** the choice of mode, and there is no default mode:

| Second key | Mode |
|---|---|
| `r` | rebase |
| `m` | merge commit |
| `s` | squash |
| `tab` | toggle branch deletion |
| `esc` | cancel |

The reason is that a merge rewrites history and cannot be undone with a command,
so no path should exist that fires it with a strategy you never named. Any other
key **cancels and is consumed**: before, it was re-dispatched as if nothing had
happened, and that turned a badly armed merge into an approve (`m` and then `a`
approved the PR) or into an immediate merge commit (`m` and then `m`). Cancelling
without re-dispatching does the same —the view stops waiting either way— but
without the side effect. `q` and `ctrl+c` still quit.

**Only the modes the repository allows are offered.** GitHub publishes
`mergeCommitAllowed` / `rebaseMergeAllowed` / `squashMergeAllowed` in the same
query of the item, so the list is exact and costs no call: a repo with squash
disabled does not show `s`. When the rules are not known —GitLab does not expose
them through GraphQL, `Project.mergeMethod` does not exist in its schema— all
three are offered, because not knowing is not the same as not allowing, and an
invented filter would leave the user with no legitimate way out.

The notices name the mode both when starting (`merge (rebase) in progress…`) and
when finishing (`merge (squash) ok · branch deleted`), because without that a
"merge ok" says nothing about what was done.

### The merge also names whether the branch is deleted

The second of the two things the merge decides is whether the source branch is
deleted once integrated. It lives in the same Confirmation and is changed with
the same key that already had another target: `tab`. The box shows it whenever
the merge is armed — `delete branch: yes (tab)` — and nowhere else, because it is
not a decision that can be taken anywhere else.

**The default is to delete.** That is what the forges do on their own and what
whoever cleans up after a merged PR expects; asking for an extra gesture to avoid
it is asking for the confirmations people get tired of. `tab` turns it off for
the rest of the session, and `tab` again brings it back. The value is per session,
not per item: the housekeeping delete does not depend on the PR you have in front
of you.

The final notice says what happened to the branch, and there are three different
endings because they are three different situations:

| Ending | Notice |
|---|---|
| Merge and delete | `merge (squash) ok · branch deleted` |
| Merge done, delete refused by the forge | `merge (squash) ok · branch not deleted: <reason>` |
| Fork PR | `merge (squash) ok · branch not deleted: the branch lives in a fork` |

The second one is the one that makes you look twice. The delete rides on the
**same command** as the merge (`gh pr merge --delete-branch`,
`glab mr merge --remove-source-branch`), so if the forge refuses it the CLI exits
with an error even though the integration is already done: without push access,
with a protected branch, or against a repo with a merge queue —which refuses `-d`
before merging—. Reporting it as "merge failed" would make the user go looking for
a state change of the forge that never happened. The re-read that the merge
already does is what tells the cases apart: if the item comes back merged, the
merge went through and what failed was the delete.

The third one is a no-op of the forge, not a failure: a fork PR has no branch to
delete in the target repo, and `gh` takes that for granted and exits successfully.
Without saying it, "branch deleted" would be a lie.

What the call does **not** touch is the local repository: with `--repo` (GitHub)
and `-R` (GitLab), the CLI only deletes the remote branch. The bare clones and
the worktrees that prdash manages stay intact.

### Change the target branch with `e`

`e` opens a popup with **the branches of the repository**, which are asked of the
forge when it opens, and lets you type to filter them. One is picked with `↑`/`↓`
(with the filter empty, `j`/`k` too) and `enter`; the second `enter` is the
confirmation.

```
╭ retarget acme/widget#7───────────────────────────────────────╮
│from main  ·  5 branches                                      │
│▸ main                                               · current│
│  develop                                                     │
│  feat/una-rama-deliberadamente-larguisima-que-no-cabe        │
│  fix/hunk-pane-argv                                          │
│  release/2.0                                                 │
│                                                              │
│filter (type to search)                                       │
│↑↓ move · enter choose · esc close                            │
╰──────────────────────────────────────────────────────────────╯
```

Three decisions, and why:

- **The branches come from the forge, not from a text field.** Changing the base
  to a branch that looks like it but is not (`main-2` instead of `main`, or
  `release/2.0-rc1` instead of `release/2.0`) is accepted by the forge without
  complaint and is not noticed until the PR points at the wrong branch. A typo
  that neither the compiler nor the forge flags is exactly the one that a
  searcher makes impossible. That is also why the listing is asked of the forge
  and not of the local clone: the clone only has the refs that were pulled down.
- **There is a confirmation.** Moving the base redoes the diff, the mergeability
  and the CI, and whatever would have been approved before is now compared
  against something else. The confirmation names the two branches
  —`main → release/2.0`— so that a stray finger cannot reorient the PR by one row
  too many. `esc` goes back to the list instead of closing: pointing at the wrong
  row is the most likely mistake and it should not cost the three keystrokes.
- **`j`/`k` navigate only with the filter empty.** As soon as there is text typed
  they are two more letters of the filter, because typing a branch name with a `j`
  in it has to be possible. `ctrl+u` clears the filter and gives them back their
  second trade.

Details worth knowing:

- The listing is cached **5 minutes per repository**, so opening and closing the
  popup does not cost a call each time. After the TTL it asks again, so that a
  freshly created branch shows up.
- Picking the branch the item **already has** does nothing: it warns and does not
  call the forge, which would answer "no changes".
- The notices name both branches: `retarget (main → release/2.0) ok`. Without
  them a "retarget ok" says nothing about what happened with the PR.
- **The already-mounted review is not touched.** If there was a worktree, the
  notice says so — `· the mounted review still has the old base…`— because it is
  still on the previous base. It is neither rebased nor rebuilt: the worktree
  belongs to the user and may have uncommitted changes.
- It only applies to **open** items: a merged or closed PR is not something whose
  base can be changed, and the call is not spent.
- The forge is the one that rules on permissions: if you are not a maintainer of
  the repository, its answer is shown as-is and the action stays disabled for
  that item.

An implementation detail that does not show in the UI: **`gh pr edit --base` is
not used** (even though it is what `gh` documents), because today it fails before
touching anything with `GraphQL: Projects (classic) is being deprecated…` —the
query with which `gh` checks whether the PR is in a project—. It uses
`gh api -X PATCH …/pulls/N -f base=`. On GitLab it uses
`glab api -X PUT …/merge_requests/N -f target_branch=` and not
`glab mr update --target-branch`, because that is an editing command and its
reason to exist is to open the editor.

### What the merge checks before it goes out

A merge is not a command: it is the action for which this outflow has no way
back. That is why there are three things that are looked at and were not looked
at before.

**CI and requested changes are announced before the confirmation, they are not
forbidden.** The state model already told apart an item with red checks from one
with requested changes from a healthy one —the card shows them— but the gate did
not look at them, so `merge` came out the same in the three cases. Now:

| State | What happens |
|---|---|
| Draft, already merged, already closed | **It does not arm.** It is a property of the forge: GitHub refuses the merge of a draft PR, and offering it only spends a call to receive an error. |
| **The branches conflict** | **It arms, and warns**, naming the branch: `merge acme/widget#6 with the branch conflicts with main · press the mode anyway…`. It is a rebase, and a rebase is done by the user. |
| CI red | It arms, and the confirmation says `merge acme/widget#7 with CI is failing (2 of 5) · press the mode anyway…` |
| CI still running | It arms, and warns: merging while the CI runs is the race that the head pin does not close, because the CI can pass *after* the merge. |
| Requested changes | It arms, and warns. |

Forbidding them outright would turn the tool into a wall —a flaky check would
leave the PR never able to merge—, and doing nothing would make them invisible. A
gate that always warns trains you to ignore the warning, so on a healthy item
none of them comes out.

The draft is looked at as what it is —a property of the forge— and not as a state
of the item, and that is why it stops you the same with the review approved or
without it: `State` sorts by attention to the operator and an approved draft comes
out as `approved`. The card says it in its own `Draft` row instead of hiding it
inside `State`.

The branch conflict is also warned about earlier, and for the same reason: it
comes out in the box (`the branch conflicts with main`) and not as a refusal of
the CLI after spending the call. The two data travel in the query that was
already made for the item (`mergeable` on GitHub, `detailedMergeStatus` on
GitLab), so they cost no call, and where the forge does not know it yet —GitHub
returns `UNKNOWN` while it computes it— nothing is said: a warning with no data
is a false warning.

When the refusal comes anyway —because the PR was pushed between the refresh and
the keystroke, or because the data was not there—, the notice says what to do and
does not promise a refresh that does not fix a rebase:

| Situation | Notice |
|---|---|
| The item changed while you were looking at it (closed, merged) | `forge conflict: …` — a refresh resolves it |
| The branches conflict | `merge refused: the forge will not merge it as it is: rebase the branch onto the target and push` |

That there are two messages and not one is the point: in the vocabulary of prdash
a "conflict" fixes itself, and mixing it with the refusal over branches forced
promising a refresh that was good for nothing.
Choosing the mode **is** the confirmation: for that a strategy has to be named,
and whoever names it after reading that the CI is red has already decided. No
third key is needed.

**The merge is pinned to the commit that was read.** `--match-head-commit` on
GitHub, `--sha` on GitLab. Without that the forge integrates the HEAD of the
moment, and between the refresh of the inbox (60 s) and the keystroke the branch
may have advanced: commits that nobody reviewed would be integrated. It is the
worst possible outcome of an irreversible action, and that is why when the forge
does not report the commit —a null `diffHeadSha` on GitLab, or an answer that
does not carry it— the merge is **refused** instead of going out without a pin.

**The auth notice carries the adapter's reason.** `bitbucket` answers
`not implemented in this version`, and before that was painted as
`not authenticated`: two things that call for opposite actions, because an
invalid token can be fixed and an unimplemented feature cannot.

`approve` does not apply to your own PR/MRs: no forge allows approving what you
write yourself (GitHub refuses it in the API and there is no option to turn it
on). prdash detects it before calling the CLI and marks those items with
`ROLE: own`; when you press `a` the reason comes out in the notice. `merge` does
work on them.

### Review layout

`r` on an item mounts the worktree and, inside its workspace, **two tabs**:

| Tab | Panes | Purpose |
|---|---|---|
| `Review` | TUICR \| editor | read the review and edit the code in parallel |
| `Edit` | Hunk \| agent | the diff against the target branch and the agent working |

The two panes of each tab open at 50 %, and the first one reuses the pane that
the workspace already brought, so no orphan tab is left behind.

The `Review` tab stays even if a tool is missing: if there is no TUICR, diff or
agent binary, its pane is not assembled and prdash warns instead of leaving a
mute gap. The editor pane is **never** skipped, because its command is usually a
shell function (the typical `vi` that expands to `nvim .`) that does not exist as
a binary on the `PATH`; if the command is misspelled, the error is visible in the
pane itself.

### Pane commands (`[commands]`)

Each pane can be replaced entirely from `[commands]`:

| Key | Default | What it opens |
|---|---|---|
| `tuicr` | `tuicr pr <item URL>` | TUICR review |
| `hunk` | `hunk diff` | the **working tree** diff with Hunk |
| `agent` | `opencode` | the agent |
| `editor` | `vi` | the editor in the worktree |

If you define the key, its value is used **verbatim** as the pane's full argv: no
item URL nor the diff target is appended to it. With no key the default is used.
The forge tools (`gh`/`glab`) are also configured here.

Hunk reviews the working tree, not the PR diff: the pane lives next to the editor,
so what is looked at is what is being touched. For the diff of the PR against the
target branch, change its key:

```toml
[commands]
hunk = "hunk diff main...HEAD --watch"
```

The target branch must be a **local ref**: prdash clones in bare
(`git clone --bare`), so remote branches end up in `refs/heads/*` and the
`origin/*` refs do not exist. In a review worktree use `main` (not `origin/main`).

```toml
[commands]
# Force Hunk's diff against main with auto-reload:
hunk = "hunk diff main...HEAD --watch"
```

The panes receive `PRDASH_BASE` with the target branch of the item, besides
`PRDASH_REPO`, `PRDASH_NUMBER`, `PRDASH_WORKTREE`, `PRDASH_BRANCH` and
`PRDASH_URL`.

### Worktree management (`prdash worktrees`)

The review worktrees are identified by their name/label `prdash-…`: prdash
**never** lists nor deletes worktrees that are not its own. Closing the app
deletes nothing: there is no implicit deletion on exit.

```sh
prdash worktrees                     # its own listing (path, branch, state); flags orphans
prdash worktrees list                # same, spelled out
prdash worktrees remove <path>…      # deletes ONLY what was asked, and only if it is prdash's
prdash worktrees remove --orphans    # deletes in a batch only the orphans `list` flags
prdash worktrees remove --orphans --dry-run   # prints the exact batch and deletes nothing
```

`--orphans` reuses the same source of truth as `list` (the `Audit` itself), so it
deletes **only** what `list` marks as `orphaned`; with zero orphans it reports
and exits 0. `--dry-run` prints the exact batch through the same code path,
deleting nothing, so that the irreversible operation can be seen before running
it. `--orphans` is **mutually exclusive** with explicit paths: mixing the two
modes is a usage error. An orphan whose `.git` does not even declare a gitdir (a
corrupt or truncated link) is also cleaned up, by path or in batch: there is no
repo to resolve and all that is left is deleting its checkout.

The only implicit cleanup is when **merging from prdash**: when a `merge` started
from the app finishes well, the worktree of that item is deleted **only if it is
clean**. If it has uncommitted changes (including new untracked files) it is
**kept** and the notice says so (`merged, but the worktree has uncommitted
changes — kept`). If its git status cannot be checked either, it is also **kept**,
with a different notice (`worktree kept: could not read the worktree status`).
When in doubt, never delete. No other path —approve, retarget, a merge that does
not go out well, the refresh that sees a PR merged outside prdash, closing the
app— deletes anything.

The editor is the only one that adjusts better with `[tools].editor`, which is a
shortcut for its base with no override:

```toml
[tools]
editor = "nvim ."   # in case you do not use your shell's `vi` → `nvim .`
```

It works the same with Herdr's native provisioning (inside Herdr) and with git
directly (outside): the ownership and managed-root guards apply on both paths, so
a path of your own outside the root is refused without touching anything.

## Inside Herdr

prdash **is not a Herdr plugin**. It talks to Herdr's CLI through a subprocess
(`herdr worktree create`, `herdr tab create`, `herdr pane split`, `herdr pane
run`), so it opens the worktree, the tabs and the panes itself when you press `r`
on an item.

The only thing it needs is **to be inside Herdr**: the client requires
`HERDR_ENV=1`, which Herdr only injects into its own children. Launched from a
normal terminal, `r` degrades to direct git and mounts the worktree without panes.

Launch it from any pane:

```sh
prdash
```

Test PR 10/10: smoke note to practice the PR resolution cycle.

If you want a key to open it, declare it yourself in `~/.config/herdr/config.toml`
(prdash does not edit that file) and reload:

```toml
[[keys.command]]
key = "prefix+p"
type = "command"
command = "prdash"
description = "prdash: PR/MR inbox"
```

```sh
herdr server reload-config
```

The keybinding is a convenience, not a requirement: typing `prdash` by hand in a
pane works the same. What is a requirement is that the pane be at the **repo
root**, because `herdr worktree create` is refused from a linked worktree
workspace.

**What is not there:** Ctrl+click on a PR/MR URL to mount it, nor a Herdr key
that mounts the last selection in prdash from another pane. Without a plugin
there are no link handlers; for the second one, jump to the prdash pane and press
`r`.

## Development

`make check` is the local equivalent of the CI gate (Build/Lint/Test jobs):

```sh
make check   # build + lint + test (nunca instala)
make test    # go build ./... && go vet ./... && gofmt check && go test -race -count=1 ./...
make lint    # go vet + gofmt + golangci-lint v2.13.2 (pinned, via go run)
make fmt     # formats the code
make print   # checks the pipeline without the TUI
```

CI: `.github/workflows/ci.yml` runs on every PR, on push to `main` and by hand
(Build, Lint and Test with a coverage summary);
`.github/workflows/mutation.yml` runs on every PR and by hand (mutation testing
with gremlins, blocking gate over the diff's surviving mutants).

Design: `docs/planning/archive/0001-mvp/` (plan, expected behaviour, context,
closing summary); permanent decisions in `docs/adr/`; the Herdr integration
contract in `docs/research/herdr-0.9.1-contract.md`.
