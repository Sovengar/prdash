# ADR 0014 — resolve lazy mergeability and refuse a doomed merge before arming

- **Status**: Accepted
- **Date**: 2026-10-12
- **Decider**: user (buble)
- **Scope**: `internal/state/`, `internal/tui/`
- **Depends on**: ADR 0007 (the merge cleanup the refusal precedes).

## Context

A user armed the merge of a conflicting PR (`CONFLICTING`/`DIRTY`) and only
learned after the fact:

> `merge refused: the forge will not merge it as it is: rebase the branch onto the target and push`

The armed confirmation had been silent. The mechanism, verified: GitHub's
`mergeable` is **lazy** — the inbox query can return `UNKNOWN` until the forge
computes it — and `state.MergeBlock` only names a conflict when
`Mergeable.Known && Mergeable.Conflicted`. So a stale `UNKNOWN` item armed with
no warning at all, then hit the forge's refusal.

## Decision

1. **Resolve the lazy value on the selection tick.** The same poll that fetches
   the selected item's comments calls `ItemState` for an item whose mergeability
   is not known, so the gate reads a real value instead of a stale `UNKNOWN`. It
   runs in the background, never stacks on an in-flight fetch, and is capped at
   three attempts per item so a forge that keeps answering `UNKNOWN` cannot make
   the tick call it forever; the count resets as soon as a real value arrives,
   because a refresh can overwrite a resolved value back to `UNKNOWN`. A nil
   adapter, an unauthenticated forge or an already-known value is skipped.
2. **A known conflict is a hard block at arm time**, not a soft warning. The
   forge cannot merge it and the fix (rebase) lives outside prdash, so there is
   no mode the confirmation could still decide. `MergeBlock` returns the block
   with `Hard: true`; pressing `m` shows `merge blocked: the branch conflicts
   with <target>` and does not arm.
3. **`UNKNOWN` is never painted as mergeable.** A conflict gets a `✗` in the
   row's own marker gutter — a dedicated first column, not text inside the
   title — painted only for a conflict the forge reported. The detail card
   states it three ways (`Mergeable`: `yes` / `no (conflicts with <target>)` /
   `unknown`), so a field never rounds an absence of data up to a yes. Arming
   while still `UNKNOWN` goes ahead — the forge's verdict stays the fallback —
   but warns first (`state.UnmergeabilityUnknown`) rather than presenting a
   clean confirmation.
4. **The forge's refusal path is untouched.** This warns earlier; it does not
   pre-empt or replace the forge's own answer for anything prdash cannot know.

## Rejected alternatives

- **Block while `UNKNOWN`.** Honest, but it would forbid a merge the forge might
  accept whenever the lazy value is slow; the forge's verdict is the authority.
- **Show the conflict only in the confirmation suffix** (the previous soft
  behaviour). That is what the user never saw, and a suffix invites "press
  anyway" onto a merge that cannot succeed. Rejected as the inciting bug.
- **Block the Update handler on an `ItemState` call at arm time.** Blocks the
  whole TUI on a network round trip; the project already fetches in the
  background and delivers the result as an event.
- **Paint `UNKNOWN` with a "maybe" marker.** Every column would need a third
  state for a transient, and the marker would read as a verdict the forge has
  not given.

## Consequences

**Positive**
- A conflicting PR is refused with its target named, before any mode is chosen.
- The row shows the conflict while browsing, once resolved.
- `UNKNOWN` degrades openly (a warning) instead of looking mergeable.

**Negative / costs**
- One extra `ItemState` call per selected item whose mergeability is unknown,
  capped at three per item, on the existing tick.
- If the forge keeps returning `UNKNOWN`, the arm-time warning is the only
  guard; the forge's refusal still catches it.
- The conflict block no longer offers a forced path; a rebase (outside prdash)
  is the only way through, which matches the forge.

**Verification (closed with `make test`)**
- A conflict resolved from `UNKNOWN` marks the row and refuses arming with the
  target branch named.
- `UNKNOWN` still arms but warns; a known mergeable item is not re-checked.
- The row gutter marker is painted for a conflict and for neither `UNKNOWN` nor
  mergeable, and never inside the title; the detail field reads the three
  states, naming the target on a conflict; an empty re-read inserts nothing.
