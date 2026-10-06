#!/usr/bin/env bash
# Advisory language audit for ADR 0010 (everything in English).
#
# ADR 0010 names this audit as the enforcement mechanism, and a convention that
# cannot be verified on demand is already failing. It is advisory, like
# `make mutate-diff` in local use: it reports and exits non-zero on findings,
# and CI does not run it yet. One language for the repository means the answer
# to "is this repo English?" is this script's exit code.
#
# Two detectors:
#   1. Spanish-only tokens, matched as whole tokens so identifiers and English
#      words that merely contain them stay clean.
#   2. Accented characters, outside the whitelist of intentional fixtures:
#      unicode-width tests need real accented strings to measure columns.
set -uo pipefail

cd "$(git rev-parse --show-toplevel)" || exit 2

# High-precision Spanish-only tokens. Single/double letters that collide with
# English (y, e, u in "x, y", "e.g.") are deliberately absent, and so are
# "todos" (the GitLab API resource, /todos) and "del" (the diff abbreviation
# for deleted): the audit must have no false positives, or it stops being run.
TOKENS='(^|[^[:alnum:]_])(el|la|los|las|al|un|una|unos|unas|que|pero|porque|está|están|estan|hay|más|mas|también|tambien|después|despues|antes|cuando|donde|mientras|hacia|desde|hasta|sobre|entre|este|esta|estos|estas|ese|esa|eso|aquel|aquella|otro|otra|otros|otras|mismo|misma|mismos|mismas|nuevo|nueva|nuevos|nuevas|único|única|unico|unica|dicho|dicha|cada|muy|solo|sólo|sí|si|él|ella|ellos|ellas|nosotros|vosotros|mí|mi|tu|su|nuestro|nuestra|vuestro|vuestra|se|le|les|lo|nos|con|para|por|según|segun|sin|uno|proyecto|grupo|oculto|cosas|etiqueta|rama|servicio|movido|ahora|segundos|cabe|caben|intacto|negativa|representado|edad|dio)([^[:alnum:]_]|$)'

# Accent whitelist by content, not by line number: a line may carry accented
# characters only when it is visibly one of the unicode-width fixtures.
ACCENTS='ábc|áéíóúñ|áéí|áéíóú|áé|ñ…|naïve|unicode:|accents|año|larguísimo|falló|rompió|salió|"é"|"ñ"'

flagged=0

while IFS= read -r f; do
    # The archive is historical record under its own AGENTS.md ("do not modify"):
    # translating it would falsify the audit trail, so it is out of scope.
    case "$f" in
        docs/planning/archive/*) continue ;;
    esac
    if [ "${f##*.}" = "md" ]; then
        # ADRs quote artifacts verbatim (Spanish commands, branch names, old identifiers)
        # inside code spans and fenced blocks; a quote is not prose.
        body=$(sed -e 's/`[^`]*`//g' -e '/^```/,/^```/d' "$f")
    else
        body=$(cat "$f")
    fi
    hits=$(printf '%s\n' "$body" | grep -nE "$TOKENS" 2>/dev/null || true)
    if [ -n "$hits" ]; then
        echo "== Spanish tokens: $f"
        echo "$hits"
        flagged=1
    fi
    ahits=$(printf '%s\n' "$body" | grep -nP '[áéíóúüñÁÉÍÓÚÜ]' 2>/dev/null | grep -vE "$ACCENTS" || true)
    if [ -n "$ahits" ]; then
        echo "== Accents outside the fixture whitelist: $f"
        echo "$ahits"
        flagged=1
    fi
done < <(git ls-files)

if [ "$flagged" -ne 0 ]; then
    echo
    echo "audit-lang: Spanish content found (ADR 0010: everything in English)."
    exit 1
fi
echo "audit-lang: clean — every tracked file is English."
