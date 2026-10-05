# ADR 0006 — the target branch is changed through the API, not through the edit command

- **Status**: Accepted
- **Date**: 2026-09-27
- **Decider**: user (buble)
- **Scope**: retarget action, `internal/forge/github/`, `internal/forge/gitlab/`,
  `internal/forge/tool/`
- **Depends on**: nothing. It neither closes nor reopens any previous ADR.
- **Supersedes**: nothing.

## Context

prdash talks to the forges through the CLI: `gh pr review`, `gh pr merge`,
`glab mr approve`, `glab mr merge`. For the new action of **changing the
target branch** of the PR/MR, each CLI has the documented command:

- GitHub: `gh pr edit <n> -B <branch>`
- GitLab: `glab mr update <n> --target-branch <branch>`

The first option is the right one: it is the documented one, it is readable
and it fits the rest of the adapter, which already uses `gh pr …` and
`glab mr …` for everything else.

Tested against a real PR (`Sovengar/prdash#13`), **`gh pr edit` does not work**:

```
$ gh pr edit 13 -R Sovengar/prdash -B una-rama-que-no-existe
GraphQL: Projects (classic) is being deprecated in favor of the new Projects
experience, see: … (repository.pullRequest.projectCards)
```

It fails **before touching anything**. It is not a permissions problem nor a
PR problem: it is the query with which `gh` checks whether the PR is in a
project, and `projectCards` blows up in the repos where that field errors. The
same `gh` with the REST API does the operation:

```
$ gh api -X PATCH repos/Sovengar/prdash/pulls/13 -f base=una-rama-que-no-existe
gh: Validation Failed (HTTP 422)          # and the reason goes in the JSON body
```

GitLab is not broken in this sense, but `glab mr update` is an **edit
command**: its reason to exist is to open title and description in the editor.
With an open field flag, that door is left ajar. In a subprocess with `stdin`
on `/dev/null` it does not hang —it fails—, and a failure caused by an editor
the user does not see is the worst kind of breakdown: it says nothing about
what happened.

## Rejected alternatives

- **`gh pr edit --base`, when `gh` fixes it.** It is what anyone who read the
  documentation would end up doing. Rejected today because it does not work,
  and with no signal that it has been fixed: if it ever does, the change is
  one line and a test that already says what the good argv looks like.
- **`glab mr update --target-branch`.** Rejected because of the editor. Also,
  glab reinterprets the default method according to the flags you pass it,
  which is the same kind of trap that is already documented in this repo's
  merge.
- **GraphQL for GitHub.** The `updatePullRequest` mutation needs the PR's node
  ID and the query that fetches it already returns something else; the PATCH
  does the same in one request, with no fields that can be withdrawn.
- **Direct CRUD with `net/http` and the token.** Rejected upfront: the rest of
  the adapter talks to the CLIs and authenticating on its own would duplicate
  what `gh` and `glab` already do well (enterprise hosts, `GH_TOKEN`, `glab`
  with multiple hosts).

## Decision

**The retarget goes through each forge's REST API**, and the rest of the
actions keep going through each CLI's edit commands:

- GitHub: `gh api -X PATCH repos/<o>/<r>/pulls/<n> -f base=<branch>`
- GitLab: `glab api -X PUT projects/<o>%2F<r>/merge_requests/<iid> -f target_branch=<branch>`

The criterion that separates the two cases is not "the API is better", which
is not true in general. It is: **an edit command whose editor can be opened is
no good for a subprocess**, and **a command that today fails without saying
why is no good either**. Both fail differently, but in both cases what fails
is the way of saying it, not the operation, and the API says the same in one
line.

The branch listing that feeds the searcher goes through the API as well, and
with explicit pagination: `?per_page=100` + `--paginate`, because a subset of
the branches would leave out the searched target without warning. On GitHub
the filtering is done with `--jq '.[].name'`; on GitLab **not**, because
`glab api` has no `--jq`, and the NDJSON of `--output ndjson` is read instead.

### The rejection reason is read from the body, not from stderr

This came out of the same work and is half the value of the decision. When
the call fails with a non-zero code, the CLIs put on stderr **one line with
the whole argv** and the real reason goes in the JSON body:

```
stderr: gh api -X PATCH repos/o/r/pulls/1 -f base=x: gh: Validation Failed (HTTP 422)
body: {"message":"Validation Failed","errors":[{"message":"Proposed base branch 'x' was not found", …}]}
```

`tool.Run` cut stderr and kept the first part, so the reason the user saw was
the command that failed. `tool.APIMessage` is added, which prefers
`errors[].message` over `message` —GitHub writes the generic one in the first
and the detail in the second, so the other way round it would show "Validation
Failed"— and falls back to `message`, which is how GitLab sends it.

And with the reason in view it can be classified properly: that 422 is
`tool.Kind == "validation"`, a class that **did not exist**. Before, it fell
into `network`, which `classifyAction` translates to "forge conflict", which
promises a refresh that cannot fix a branch name that does not exist. Nor is
it `permission` —that would register the item as denied and take its action
away forever— nor `notfound` —an item that disappeared and a branch that does
not exist get confused, and only the second one is fixed by typing another
name—.

## Consequences

**Positive**

- The action works, which was the goal, and it works through the path the API
  documents instead of the one `gh` documents and does not honour.
- The rejection states the reason: `error: Proposed base branch 'x' was not
  found`.
- A new, named failure class instead of a `network` that meant nothing.
- The `422` stops being misclassified **in every action**, not only in the
  retarget: `kindForHTTP` is shared.

**Negative / costs**

- **The CLI's validation is lost.** `gh pr edit` and `glab mr update` check
  things before sending that the API does not check (permissions, protected
  branches, MR states). In practice the forge returns a 4xx with the reason,
  which is good enough, but the message comes from the server and not from the
  CLI: it can be rawer.
- **The GitLab project has to be urlencoded by hand** (`grp%2Fproj`). `glab`
  does it by itself only with the `-R` flag; by API it has to be done, and
  getting it wrong produces a 404 at a place that does not exist and does not
  say why.
- **The forge's reason is shown as is**, in English, and without the
  translation that the canonical reasons do have (`ErrMissingHeadSHA`,
  `unmergeable`, `SelfReviewReason`). Accepted: inventing a translation for a
  text that can be anything would be worse than showing it.
- If `gh pr edit` gets fixed, this is technical debt knowingly paid. It is
  documented here and in the README so that it does not look like an
  oversight.
