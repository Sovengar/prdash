# ADR 0008 — the unknown-action dispatch is cut before re-reading the item

- **Status**: Accepted
- **Date**: 2026-10-03
- **Decider**: user (buble)
- **Scope**: `internal/forge/`, `internal/tui/`, `internal/review/executor/`
- **Depends on**: ADR 0006 (retarget through the API), which is what added the
  third `ActionKind` and left the dispatch's `default` as the only place where
  an action could slip in without being executed.

## Context

`forge.RunAction` dispatches the inbox's quick actions with a two-branch
`switch` —`approve` and `merge`— and a `default` that returned `nil`. That was
correct as long as the quick actions were exactly two.

The problem is what `nil` means further down. `classifyAction` —which
translates an action's warnings to `(ok, conflict, perm, msg)`— **treats an
empty list as "there were no warnings"**, which is its way of saying the
action went well. So the `default` was not a containment: it was a **success
report of something that was not done**.

The symptom in production was impossible to see for two reinforcing reasons:

1. The TUI filters by `canActionOn`, which only lets `approve` and `merge`
   through. The unknown action never reached `RunAction`.
2. `retarget` —the third `ActionKind`, the one from ADR 0006— has its own path
   and never goes through `RunAction`.

That is: the bug was behind two guards that work, plus one that got consumed
when the third action was added. There is no test that would reveal it because
there is no path that reaches it green.

## Considered alternatives

### A. Leave it, because the guards above cover it

It is what was there. It is rejected for a concrete reason: **a containment
that reports success does not contain.** The day a new `ActionKind` is routed
through `RunAction` without adding its branch, the user sees "done" in the
header for an action that did not happen, and nothing in the output contradicts
it. The cost of the fix is one `if`; the cost of the failure is a phantom
approve.

### B. Make the `default` return an `unsupported` warning

Rejected. It would be correct as to the outcome (`classifyAction` maps it to
`perm`) and shorter to write, but it has two defects:

- **It costs a trip to the forge.** `runOn` starts by re-reading the item's
  state, so dispatching an impossible action makes a round-trip call to GitHub
  to find out it was not going to do anything.
- **It classifies wrongly.** `unsupported` means "the forge does not have this
  operation". Here no operation is missing: the dispatch is missing. Having the
  TUI register the item as denied forever would be **permanent and wrong** —a
  new Kind fixed on restarting prdash would leave all those items with the
  action disabled.

### C. Cut at the entry of `RunAction` — chosen

The cut goes before `runOn`, and the `Outcome` is built there. It removes the
`exec`'s `default` instead of giving it a new answer.

## Decision

`RunAction` validates the `ActionKind` before doing anything. If it is not
`approve` nor `merge`, it returns an `Outcome` with `OK: false`, **no flags**
and a canonical reason that names the action.

The `exec`'s `default` is deleted. It is not left as a second net: two places
that say the same thing diverge, and an unreachable `default` is code that
reads as if it protected something.

The cut goes before `runOn` and not inside the `exec` for two reasons that are
paid on every call: `runOn` re-reads the item from the forge, and the answer
does not come from a forge —routing it through `classifyAction`, which
classifies forge responses, would put it in a category that does not belong to
it.

### Why with no flags

`Outcome` has three flags and none describes this:

- `Conflict` is "the item changed in the forge, refresh". The item has not
  changed.
- `Perm` is "this action is disabled for this item". It is permanent and
  wrong, and it is option B.
- `Unmergeable` is merge-specific.

It is a failure of **the caller**, not of the item nor of the session. Adding
a fourth flag would be new state in the `Outcome` that only this path would
set, and its only consumer would be the message —which already exists. With
`OK: false` and a reason, the TUI shows what corresponds: nothing was done,
and this is why.

### The `default` that is deleted

The two-branch `switch` with a guard above is more honest than a `switch` with
a `default` that is never reached: the `default` documented a tolerance the
code no longer has.

## Consequences

- `RunAction` with an unknown `ActionKind` is observable: `OK: false` and a
  reason that names the action. There is no way for it to report success
  without having done anything.
- A new `ActionKind` shows up in the message as soon as it is routed through
  here, instead of appearing as "done".
- The `exec` of `runOn` no longer has a `default`. If someone routes it
  without going through the entry guard, the type prevents dispatching:
  `kind` only has two valid values in that position.
- The cost of the good path is an `if` that compares two strings.

## Alternative for the future

If `RunAction` ever has to dispatch more than two actions —a
`request-changes`, a `dismiss`—, the guard becomes a list of admitted
`ActionKind`s and the `switch` becomes a `switch` again. The rule —cut before
re-reading, and never report success of nothing— does not change.
