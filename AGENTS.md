# AGENTS.md — prdash

A guide for agents with no prior context on this project.

## What it is

A multi-forge PR/MR inbox plus a review orchestrator on top of
[Herdr](https://herdr.dev). It mixes GitHub and one self-managed GitLab into a
single inbox and, when you pick an item, it leaves the review environment ready
(worktree + 2-tab layout) so the comment loop happens without assembling
anything by hand.

Status: **MVP F1 + F2**. F3 (auto-review with gate and allowlist) is documented
but **not implemented** — see `docs/planning/archive/0001-mvp/f3-milestone.md`.

## Stack

- Go 1.26+ (`go 1.26.3` in `go.mod`), module `prdash`
- `charm.land/bubbletea/v2`, `charm.land/lipgloss/v2`, `charm.land/bubbles/v2`
  (import paths `charm.land`, NOT `github.com/charmbracelet`)
- `github.com/charmbracelet/x/ansi` — it is used, for width/ANSI handling
- `go tool` gremlins for mutation testing
- At runtime: authenticated `gh` and `glab`, and Herdr 0.9.x (optional)

## Commands

```bash
make build         # builds ./bin/prdash
make install       # installs into ~/.local/bin/prdash
make test          # full gate: go build + go vet + gofmt + go test -race
make lint          # go vet + gofmt + golangci-lint v2.13.2 (pinned, via go run)
make check         # build + lint + test: the local gate equivalent to ci.yml
make run           # opens the TUI with go run
make print         # --print mode, no TUI (to check the pipeline)
make mutate-diff   # mutation testing scoped to the diff vs main (advisory)
```

`make test` **fails if any code is not `gofmt`-clean** (it checks
`gofmt -l $(git ls-files '*.go')`). It is a gate, not a warning.

## Crucial step after any code change

**Deploy the binary**: the user runs `~/.local/bin/prdash`, not the repo, and
tests with `go run` do not update the installed one.

```bash
make install
```

Without this, anything the user checks on the TUI runs the old version. Run it
ALWAYS at the end of a code task, after verifying.

**There is no need to close the TUI**: on Linux the binary is replaced on disk
while the process keeps running with its in-memory copy.

## New feature → docs/FEATURES.md + skill

Any **new feature** — and any user-visible change to an existing one — must be
documented in `docs/FEATURES.md` **in the same change** (create the file if it does
not exist yet): add or update its entry with what it does and how it is
triggered (key, flag or command). A feature that is not in `docs/FEATURES.md` does
not exist for the next reader. Keep it a concise inventory, not a tutorial: the
details live in `README.md` and `docs/adr/`.

The same change must also update the global skill
**`~/.agents/skills/prdash/SKILL.md`** — trigger, keys, subcommands/flags and
config keys that changed. The skill is the runtime contract agents load before
working on prdash; a feature absent from it does not exist for them.

## CI

Two workflows, same skeleton as dbx/gitdash/tsk/vroom: no `paths`, minimal
permissions, `concurrency` with cancellation off `main`, `ubuntu-24.04` and
actions pinned by SHA. Three required checks: `Lint`, `Test` and `Mutation`.

### `CI` (`.github/workflows/ci.yml`) — every PR, and pushes to `main`

The gate. `Mutation` is a job of this file: two jobs, `Lint` ∥ `Test`, plus the
mutation job on PRs. Runs on every `pull_request`, on push to `main`, and
manually (`workflow_dispatch`).

- **Lint**: `make lint` = `vet` + `fmt-check` (gofmt) + golangci-lint **v2.13.2**
  pinned in the Makefile, run with `go run` (no global binary). There is no
  `.golangci.yml`, so it applies the default set (errcheck, govet, ineffassign,
  staticcheck, unused). The `Makefile` is part of the module cache key so that
  bumping the version does not reuse old modules.
- **Test**: `go test -race -count=1 -covermode=atomic -coverprofile=coverage.out
  ./...` — the suite is self-contained (git fixtures in `t.TempDir()` via
  `internal/testutil`, fake adapters), so it does not need `gh`/`glab`/`herdr`/
  `git-sim` in the PATH. `-count=1` disables the test cache: with the build cache
  restored, a cached `ok` can never let a failure through.
- **Coverage gate**: `scripts/diff-coverage.sh` — the PR's **diff at 100%** and
  the **total against `scripts/coverage-floor`** (100.00, can only go up). The
  base is the explicit merge-base, hence `fetch-depth: 0`.

### `CI fast` (`.github/workflows/ci-fast.yml`) — every commit on a branch

Build + unit tests, no lint, no mutation. **Not required**: a red never blocks a
merge and a green never authorises one. Its 10m ceiling is for a cold runner, not
for the ~40s the suite takes warm.

### `Mutation` — a job of `ci.yml`, non-draft PRs only

Runs on non-draft PRs (not on push to `main`). The job always reports — no
`needs:`, no `continue-on-error`, the only `if:` is the event gate — which is
what makes it a required check.

- **Where the decision lives**: `scripts/mutate.sh` measures AND decides in one
  step (`scripts/mutate.sh --diff --ci --summary …`); the workflow only brings
  paths, refs and budget. *"Could not measure"* is a red, never a green: a
  missing `report.json`, an unparseable one, a silent run or an expired mutant
  nobody recorded all fail with the reason in the step summary.
- **Gate (blocking)**: it fails if there is any **new surviving** mutant in the
  diff that is not in `.mutation-allowlist`. For a mutant that is only accepted
  if it is demonstrably equivalent, add the line to the allowlist with a comment
  explaining why. A missing allowlist is a red with the command to seed it
  (`make mutate`), never a skip: a skipped required check blocks every PR.
- **Budget** (mandatory under `--ci`, asserted at startup):
  `2*CAP < STALL < CEILING`, `CEILING + SETUP_RESERVE < JOB_CEILING` →
  `180s · 4 workers · 8m · 13m · +600s · 25m`.
- **Scope**: `MUTATE_EXCLUDE` in the `Makefile`, read by `scripts/mutate.sh` so
  local and CI gate the same set; the `.go` scope is applied INSIDE the job, the
  same as the `make mutate-diff` guard, so no `paths` filter can deadlock a PR.
- **Where it runs**: the gate is CI (the `Mutation` job above). Locally it is
  never automatic: run it manually, only when needed, with `make mutate`
  (whole module) and `make mutate-diff` (the diff against `MUTATE_BASE`), same
  wiring as CI. `make coverage-check` is the local equivalent of the coverage
  gate.
- **Two counts, and they are different numbers**: `WATCH_LINES` (every mutant
  considered = the supervisor's denominator) and `EXPECTED_MEASURED` (in-scope
  `RUNNABLE` only = what the verdict compares against the report's
  `mutants_total`, which is `killed + lived + notViable`). `SKIPPED` leaves the
  scope and `NOT COVERED` sits in no cover block (Go cover starts a case
  clause after the colon), so neither can be measured and neither belongs in
  the denominator; the exclusion and its reason are published by the verdict.
- The gitconfig is neutralised (`GIT_CONFIG_GLOBAL`/`GIT_CONFIG_SYSTEM` →
  `/dev/null`, `GIT_CONFIG_NOSYSTEM=1`): gremlins computes its `--diff` ranges
  with a plain `git diff` run with the AMBIENT config, so a dev's
  `diff.algorithm`/`diff.interhunkcontext` would make the local loop measure a
  different mutant set than CI's defaults. Hermetic by construction.
- **Files that make the gate possible** (all committed): `.mutation-allowlist`
  (survivors accepted **by line**), `.mutation-timeouts` (`<file> <ceiling>` for
  mutants that expired and were never tested — gremlins omits TIMED OUT mutants
  from `report.json`, so silence can never be a green), `scripts/coverage-floor`,
  `scripts/watchdog.sh` + `scripts/watchdog_test.sh` (vendored frozen copy) and
  `scripts/mutate_test.sh` (the gate's red paths, ~1s, run as the `Shell suites`
  CI step).
- Uploads the report and the run log as an artifact (14 days).

`make check` (build + lint + test) is the local gate equivalent to `ci.yml`, and
`make test` (build + vet + gofmt + `go test -race`) the fastest: **run it before
opening the PR**. `make coverage-check` and `make mutate-diff` reproduce the two
blocking gates locally.

### `main` protection

Ruleset **`protect-main`**, reproducible with
`scripts/setup-repo-protection.sh` (idempotent, `--dry-run` shows without
mutating): merge only via PR with `Lint`, `Test` and `Mutation` green, force-push
and deletion of `main` blocked, `delete_branch_on_merge=true`. `Build` is not a
check anymore — it moved to the advisory `CI fast`, and a required `Build`
context would deadlock every PR. The admin bypass is deliberate: the working
intention is always the PR path.

### Waiting for CI

To follow a PR's checks, wait with `gh run watch <run-id> --exit-status` (or
`gh pr checks <n> --watch`). Never `sleep` + `gh pr checks`: runs go stale after
a force-push and the id has to be asked for again.

## Architecture

```
cmd/prdash/            → entry point
internal/config/       → XDG config, defaults, keybindings
internal/forge/        → GitHub / GitLab / Bitbucket (adapters)
internal/inbox/        → items, sections (Assigned/Mentioned/Mine), refresh
internal/reporesolver/ → resolves the target repo of each item
internal/gitcmd/       → git wrapper (worktrees, refs, merge)
internal/worktree/     → review worktree provisioning
internal/review/       → review assembly, 2-tab layout
internal/herdr/        → pane control (without Herdr's plugin)
internal/state/        → state persistence
internal/cache/        → cache (comments, etc.)
internal/sim/          → git-sim integration
internal/tui/          → Bubbletea v2: Inbox, detail, modals, merge
internal/testutil/     → test helpers
docs/adr/              → architecture decisions (numbered ADRs)
```

## Conventions

- **Files**: `snake_case.go`. **Types**: `PascalCase`.
- **Everything in English** — code, comments, identifiers, test names,
  assertion messages, docs, ADRs, the README and the CHANGELOG. One language
  for the whole repository, so the rule is checkable instead of remembered. See
  `docs/adr/0010-everything-in-english.md`.
- **Comments in English, and only when they explain the WHY.** A comment that
  explains *how* the code works is deleted — the code already says it and it
  changes faster. One that justifies a decision, a constraint or a trap stays,
  compressed to one line. See `docs/adr/0009-comments-in-english.md`.
- **`internal/` exclusively** — there are no exported packages.
- **Config never aborts**: a missing or malformed config degrades to defaults
  with a warning. That is a project pattern, not an exception: do not introduce
  `os.Exit` nor `panic` for an invalid config at runtime.
- **TUI layouts**: honest, explicit degradation. If a box does not fit, it is
  not painted halfway — all or nothing. The floor that makes it possible is
  written next to the arithmetic, because a guard that cannot fire hides the
  account.
- **Merge takes two keys** and one of them is the confirmation mode.
- **Worktrunk worktrees** live under `.worktrees/`.

## Commits

Conventional Commits, as observed in the history:

```
fix(tooling): guard mutate-diff against gremlins empty-diff full-module fallback
feat(retarget): change the PR/MR target branch with `e` and a branch searcher (#20)
chore(repo): Worktrunk worktrees under .worktrees/
docs(readme): 1/10 smoke test note (#5)
```

Scope in the language of the subsystem touched. The PR/MR number goes in
parentheses at the end when the work comes from an issue.

## Rules for code review

- **`make test` must remain the gate**: nothing that breaks `gofmt`, `go vet` or
  the suite with `-race`.
- **Do not introduce `github.com/charmbracelet/bubbletea`**: the project's
  import path is `charm.land/*` v2.
- **Do not introduce dependencies without going through `go mod tidy`**
  (`make tidy`).
- **Do not hardcode forges, hosts or branches**: all of that is derived from
  the XDG config or autodetected. The default branch is derived, not written by
  hand.
- **Do not break the degradation**: if an external dependency (`gh`, `glab`,
  `git-sim`, Herdr) is missing, the action warns and does not break the TUI.
- **Do not add business logic to the render**: decisions go in
  `internal/<subsystem>/`, the TUI paints.
- If a change alters observable behaviour, the decision and its alternatives go
  in a **new ADR** in `docs/adr/` (next number, existing format).
- `make mutate` / `make mutate-diff` in **local** are *advisory*: they report
  and never block. In **CI** the `Gate` step does block (see the CI section): a
  new surviving mutant in the diff kills the PR unless allowlisted.