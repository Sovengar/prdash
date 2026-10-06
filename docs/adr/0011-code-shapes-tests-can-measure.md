# ADR 0011 — Code shapes that tests can measure

- Status: accepted
- Date: 2026-10-06
- Extends: the empty-allowlist policy of `.mutation-allowlist` and the coverage
  campaign recorded in `CHANGELOG.md`

## Context

The campaign to zero surviving mutants hit two positions no test can ever reach.
Both were measured, not assumed, with a toy module run through the same gremlins
and `go test -cover` the repository uses:

1. **Package-level declaration arithmetic.** `go cover` emits blocks for function
   bodies only. A `const DefaultTimeout = 30 * time.Second` or
   `const commentFetch = 3 * forge.CommentLimit` has no block on its line, so
   gremlins marks every operator there NOT COVERED and never runs the mutant —
   no test can change that, however many tests exist.
2. **Tagless `switch` case conditions.** A case block starts *after* the colon:
   `case c.Failing > 0:` leaves the comparison outside every block. The body is
   covered, the condition is not, so boundary and negation mutants on it are
   invisible. An `if` condition, by contrast, sits inside the preceding block and
   is measurable.

The campaign also produced the third case, the one the empty-allowlist file
already describes: a boundary mutant whose two sides render **byte-identical**
output on every input (`n > w` vs `n >= w` at `n == w` in the comment wrapper).
Before believing such a claim it is measured by hand; twice in earlier work a
"provable" equivalent turned out to be observable on real inputs.

## Decision

Three rules, in force for new code and applied repo-wide by the campaign:

1. **No operators on package-level declarations.** Write the precomputed literal
   and keep the reasoning in the comment: `const DefaultTimeout =
   time.Duration(30e9) // 30s; a literal because a const decl carries no
   coverage (ADR 0011)`. Where the literal derives from another constant
   (`commentFetch = 3 * forge.CommentLimit`), a drift test pins the formula
   (`TestCommentFetchKeepsTheMargin`).
2. **Tagless `switch` becomes an `if/else` chain**, in the same order. A
   string- or int-tagged `switch` has no case-condition operators and stays.
3. **A boundary tests cannot kill is exposed, refactored, or deleted — in that
   order of preference.** Exposed: the value becomes a test input
   (`relativeTime(t)` grew `relativeSince(d)`; `scrollFor` and
   `retargetWindowFor` gained zero-height/zero-rows cases; `compactCount` spells
   the bound `<= 9999` so the mutant dies at 9999). Refactored: the comparison
   disappears (`assignAuthority` iterates `sectionOrder` first-wins instead of
   comparing `rank`). Deleted: the guard was a no-op (`compactCount`'s
   `n < 0` routed to the same `Itoa` as `n < 1000`).

## The measurement hole that is not ours

`cmd/prdash` reports every mutant NOT COVERED although its tests cover it at
100%. Cause: gremlins v0.6.0 `removeModuleFromPath` runs
`strings.ReplaceAll(FileName, "prdash/", "")`, which also strips the
`cmd/prdash/` segment, so the coverage key becomes `cmd/main.go` while the
mutant position is `cmd/prdash/main.go`. Reproduced in a two-file probe module
and reported upstream
(go-gremlins/gremlins#319). Until a fixed release exists, the honest reading of a
full-module run is: **100% of everything gremlins can address**, with 41
positions in `cmd/prdash` owned by the tool. Renaming the directory to dodge a
third-party bug was rejected: `cmd/<binary-name>` is the convention every
sibling project uses.

## Consequences

- Full-module gremlins on `main` after the campaign: 1719 killed, 1 proven
  equivalent boundary (`comments.go` wrapper `n > w`), 47 not covered before the
  GraphQL-constant fix and the `cmd/prdash` attribution above, 11 timed out.
  TIMED OUT mutants are excluded from `report.json` entirely, so the survivor
  count from a loaded machine is a subset, never a superset — the number that
  matters is read after an idle run.
- The `+` chains of `ghPRFields` and `mrFields` became single literal lines:
  same queries, no reachable-by-nothing operators.
- The language audit ADR 0010 called the enforcement mechanism was committed,
  measured and removed: a token list cannot decide language (ADR 0012).

## Rejected alternatives

- **Allowlist the equivalent boundary.** The empty-allowlist file exists
  because entries rot silently; the CI gate accepts a proven equivalent with a
  comment, but the decision belongs to the operator, not to this ADR.
- **Rename `cmd/prdash`.** Punishes the conventional layout for a tool defect.
- **Fork or `replace` gremlins.** Maintenance burden for one path mapping; the
  upstream fix is a one-line `TrimPrefix`.
