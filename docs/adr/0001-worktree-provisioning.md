# ADR 0001 — Review worktree provisioning

- **Status**: Accepted
- **Date**: 2026-09-24
- **Decider**: user (buble)
- **Scope**: prdash MVP, phase F2 (see `docs/planning/archive/0001-mvp/`)
- **Naming pattern**: `docs/adr/NNNN-slug.md`

## Context

prdash (F2) must, when a PR/MR is picked, get its code and assemble a review layout in Herdr on top of a worktree of the PR/MR branch. Starting constraints:

- Herdr 0.9.x exposes `herdr worktree create` (with `--cwd`, `--branch`, `--base`, `--path`, `--label`, `--no-focus`) and **binds the worktree to a Herdr workspace**.
- The PR's repo may not be cloned; it was decided to clone it **bare** under the project's XDG data directory.
- Fork PR branches **do not necessarily exist** as a branch of origin, so the review ref has to be brought in (`refs/pull/N/head` on GitHub, `refs/merge-requests/N/head` on GitLab).
- Outside Herdr the application must degrade cleanly.
- **Several coexisting worktrees** are needed (one per PR), also from the same repo.

## Decision

1. **prdash always does** the local repo resolution (roots + remote→local index + path memory; bare clone if missing), the **fetch of the review ref** and the **creation of a local working branch**. It never delegates the fetch nor the ref resolution.
2. **Inside Herdr** (`HERDR_ENV=1`), worktree provisioning is delegated to the native: `herdr worktree create --cwd <repo> --branch <local-branch> --path <dest> --label <prdash-…> --no-focus`.
3. **Outside Herdr**, provisioning falls back to a direct `git worktree add` and the layout is reported as unavailable (F1 stays operative).

## Rejected alternatives

- **`git worktree add` only.** Forces creating the Herdr workspace/tab/pane by hand and loses the worktree↔workspace link (session restore, stable IDs, closing by group), which the layout uses as a container.
- **Delegating everything to `herdr worktree create`, the fetch included.** The native would receive a remote branch that does not exist locally yet (open risk of the plan) and it covers neither the fork case nor the previous bare clone.
- **Always cloning from scratch with a plain `git clone`.** It does not allow multiple worktrees from a single clone; the bare clone is the right starting point for N worktrees.

## Consequences

**Positive**
- The "`--branch` with a non-local remote branch" risk disappears by design: the native always receives an **existing local branch**.
- A single owner of the path namespace (`reporesolver`) and multiple coexisting worktrees on top of a bare clone.
- The coupling to Herdr is confined to a single port: only that module knows the native and hosts the fallback.

**Negative / costs**
- prdash must implement and maintain its own fetch and per-forge ref resolution (two distinct ref paths).
- Dependency on the Herdr 0.9.x CLI (drift): mitigated with isolated parsing per command, a minimum version declared in the manifest and a localized fallback.

**Verification (closed)**
- **OK** `herdr worktree create` from a **bare clone** with an already created local branch and a `--path` destination, against `herdr 0.9.1-preview.2026-09-21-0ff0f27e2226`. Evidence and associated findings (semantics of `--label`) in `docs/research/herdr-0.9.1-contract.md` §Local verification.
