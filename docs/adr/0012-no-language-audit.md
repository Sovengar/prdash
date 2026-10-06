# ADR 0012 — No language audit: a token list cannot decide language

- Status: accepted
- Date: 2026-10-06
- Supersedes: the enforcement clause of `0010-everything-in-english.md`

## Context

ADR 0010 named a script — "a word list of Spanish-only tokens plus an accent
check, run over every tracked file" — as the enforcement mechanism for the
one-language rule. The 100% campaign committed it as
`scripts/audit-lang.sh` (`make audit-lang`) and then measured it instead of
trusting it:

- **Recall is bounded by the list.** A pure-Spanish line whose words are not in
  the list passes: `// guarda archivo carpeta usuario ventana boton` is clean to
  the audit. During the campaign it missed ~15 real tokens
  (`cosas`, `etiqueta`, `rama`, `servicio`, `movido`, `ahora`, `segundos`,
  `cabe`, `intacto`, `negativa`, `representado`, `edad`, `dio`, …) until they
  were found by hand, after the script had already reported green.
- **The accent check only sees orthographic markers.** Accent-free Spanish is
  invisible to it.
- **Every exemption is a hole.** Legitimate unicode-width fixtures needed a
  content whitelist, the archive had to be excluded under its own AGENTS.md,
  Markdown code spans and fences had to be treated as quotations, and the
  GitLab API resource `todos` had to be exempted as vocabulary.

The tradeoff is structural, not a bug to patch: high precision (no false
positives on English) forces low recall, and "is this text Spanish?" is not
decidable by grep. A detector that reports green while Spanish passes is worse
than no detector, because it reads as a guarantee.

## Decision

The script, the `make audit-lang` target and every claim that it enforces ADR
0010 are removed. ADR 0010's language rule stays; its enforcement is code
review, as it was before the script existed. The one-time migration it drove —
22 renamed test files, translated fixtures and identifiers, the README and
AGENTS.md — is the deliverable, and it is done.

ADR 0010 itself is not rewritten: a past ADR records why a rule was made, and
editing it to agree with the present would destroy that record. This ADR is the
correction.

## Consequences

- `make` no longer offers `audit-lang`; `scripts/` is empty and gone.
- The English rule is reviewable but not machine-checked. That is stated
  instead of simulated.
- A future language check, if one is ever wanted, starts from the measurement
  above: it needs a probabilistic classifier over comments and strings only,
  with its own false-positive budget, and it must not be described as
  enforcement while it can be defeated by a word list's blind spots.

## Rejected alternatives

- **Keep it as an advisory tripwire.** It did catch 300+ real findings during
  the migration, but an advisory check that misses pure Spanish invites the
  same trust ADR 0010 placed in it, and the migration is over.
- **Replace the word list with a language detector (e.g. whatlanggo).** Better
  recall, still probabilistic, and noisy on code-heavy files where identifiers
  and URLs poison the signal; nobody asked for a second heuristic to maintain.
- **Edit ADR 0010 to remove the claim.** Destroys the record of why the audit
  was believed to work, which is the part worth keeping.
