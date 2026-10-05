# ADR 0010 — Everything in English, docs included

- Status: accepted
- Date: 2026-10-04
- Supersedes: the language clause of `0009-comments-in-english.md`

## Context

ADR 0009 made English the language of the code and explicitly kept the ADRs, the
README and the user documentation in Spanish, on the grounds that those are read
by the operator and not by whoever reads the diff.

An audit of the repository found the split had not held. The drift was not
limited to comments:

- The production code still carried ~35 Spanish comment lines, four Spanish
  identifiers (`arrancaTUI`, `guardaJSON`, `reportaIncumplimientos`, `fuera`) and
  eight user-facing Spanish strings, including a Herdr notification
  (`"prdash: review listo"`) and the toasts of the browser opener.
- The tests were almost entirely Spanish: ~6,400 lines across 207 of 207 test
  files, from `TestRunDevuelveElCodigoDeUsoSinEjecutarNada` to the table-case
  labels. ADR 0009 had deferred them with "they are handled separately", and
  that "separately" never arrived.
- Two production files held the debris of the ADR 0009 pass: orphan comment
  fragments left behind by deleted sentences (`// archivos sin trackear).`).

The cost of the split is not the two languages. It is that a single question
("is this repository English?") has no answer that is true, so the rule has to
be enforced by memory, and a rule that depends on somebody remembering it is
the exact thing ADR 0009 identified as self-eroding. The audit had to be a
script over every tracked file; a convention that needs a script to verify is
already a convention that is failing.

## Decision

**One language for the whole repository: English.** Code, comments, identifiers,
test names, assertion messages, ADRs, planning documents, the README, the
CHANGELOG, `AGENTS.md`, the `Makefile`, `.config/wt.toml` and
`.mutation-allowlist`.

Three consequences worth writing down:

**Test names are not optional.** `TestElInformeSeLlamaUnaVezPorIncumplimiento`
is read in CI output and in `go test -run` invocations, which makes it
documentation. Renaming them is part of the change, not a follow-up.

**The ADR filenames changed.** Four were Spanish
(`0003-sin-plugin-de-herdr.md`, `0006-retarget-por-la-api.md`,
`0008-dispatch-de-accion-desconocida.md`, `0009-comentarios-en-ingles.md`) and
are now `0003-herdr-without-a-plugin.md`, `0006-retarget-through-the-api.md`,
`0008-dispatch-of-an-unknown-action.md`, `0009-comments-in-english.md`. A
numbered ADR is a permalink, so the number is preserved and only the slug moves.

**ADR 0009 stays, translated, and keeps its history.** Its content criterion
(only the WHY) is unaffected and still in force. Only its language clause is
reversed, and it says so at the top. Rewriting a past ADR to agree with the
present would destroy the record of why the rule changed.

The audit script itself is the enforcement mechanism: a word list of
Spanish-only tokens plus an accent check, run over every tracked file. It is
advisory, like `make mutate-diff` in local use.

## Consequences

- ~10,400 lines are translated: ~88 in production code, ~6,400 in tests, ~3,900
  in documentation and configuration.
- The operator-facing texts are English now. That is a deliberate reversal of
  ADR 0009's rationale: the cost of a single language is paid by the operator,
  the benefit is paid by everyone who touches the repository later, including
  contributors who do not read Spanish.
- `.mutation-allowlist` keys its entries by `<MUTATOR> <file>:<line>`, so
  translating a comment while keeping the line count stable is a hard
  requirement, not a style preference. An audit found 16 of its 34 entries
  already out of range at this revision, which is pre-existing debt: only a
  full-module `go tool gremlins unleash` run recalibrates them.
- Verification cost moved from memory to a script. The question "is this
  repository English?" now has an answer that can be produced on demand.

## Rejected alternatives

**Keep docs in Spanish, fix only the code.** The status quo of ADR 0009, and the
reason this ADR exists: it left 6,400 test lines and 3,900 doc lines in Spanish
and needed an audit script to prove it either way.

**English code, Spanish operator-facing strings only.** Keeps the operator's
screenshots readable, but the strings live in the same files as the code and the
same rule, and it doubles the surface the audit has to classify for no gain that
survives contact with the first new contributor.