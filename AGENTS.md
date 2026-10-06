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

## CI

Two workflows, same skeleton as dbx/gitdash/tsk/vroom: no `paths`, minimal
permissions, `concurrency` with cancellation off `main`, `ubuntu-24.04` and
actions pinned by SHA.

`.github/workflows/ci.yml` — **Build / Lint / Test**. Runs on every
`pull_request`, on push to `main`, and manually (`workflow_dispatch`).

- **Build**: `go build ./...` + `go vet ./...` (5 min).
- **Lint**: `make lint` = `vet` + `fmt-check` (gofmt) + golangci-lint **v2.13.2**
  pinned in the Makefile, run with `go run` (no global binary). There is no
  `.golangci.yml`, so it applies the default set (errcheck, govet, ineffassign,
  staticcheck, unused). The `Makefile` is part of the module cache key so that
  bumping the version does not reuse old modules.
- **Test**: `go test -race -count=1 -covermode=atomic -coverprofile=coverage.out
  ./...` plus a per-package coverage summary in the step summary. The suite is
  self-contained (git fixtures in `t.TempDir()` via `internal/testutil`, fake
  adapters), so it does not need `gh`/`glab`/`herdr`/`git-sim` in the PATH.
- `-count=1` disables the test cache: with the build cache restored, a cached
  `ok` can never let a failure through.

`.github/workflows/mutation.yml` — **mutation testing** with gremlins. Runs on
every `pull_request` and manually (`workflow_dispatch`). Not on push to `main`.

- **Deliberately WITHOUT `paths`**: a workflow with a filter that becomes a
  required check leaves that check `pending` forever in the PRs it skips, which
  deadlocks the PRs. The `.go` scope is applied INSIDE the job, the same as the
  `make mutate-diff` guard.
- **Scope**: it only mutates if the diff against the base has `.go` files
  (`make mutate-diff MUTATE_BASE=origin/<base>`). `ubuntu-24.04`, 20 min timeout.
- **Gate (blocking)**: it fails if there is any **new surviving** mutant in the
  diff that is not in `.mutation-allowlist`. For a mutant that is only accepted
  if it is demonstrably equivalent, add the line to the allowlist with a comment
  explaining why.
- **Degradations to know**: if there is no `report.json` (timeout or a gremlins
  crash) the gate **passes** — a crash is never read as a failure. And if
  `.mutation-allowlist` **does not exist**, the whole `Mutation` job is
  **SKIPPED** (visible as a not-a-pass) and does not block: without an allowlist
  the gate is uncalibrated and measures nothing. Note: gremlins does not report
  TIMED OUT mutants in `report.json` and the efficacy excludes them.
- Uploads `report.json` as an artifact (14 days).
- Consequence of the above: a skipped job is a skipped check, so `Mutation`
  **must not** be marked as a required status check (same class of deadlock as
  `paths`).

`make check` (build + lint + test) is the local gate equivalent to `ci.yml`, and
`make test` (build + vet + gofmt + `go test -race`) the fastest: **run it before
opening the PR**, because `mutation.yml` only measures mutation.

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