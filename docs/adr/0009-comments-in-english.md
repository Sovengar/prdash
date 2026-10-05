# ADR 0009 — Comments in English, and only when they explain the why

- Status: accepted
- Date: 2026-10-04
- Language clause superseded in part by `0010-everything-in-english.md`

## Context

prdash had reached 4,896 lines of comments in the production code and 14,524 in
the tests: more comment lines than code lines in several files, and
`internal/tui/comments.go` had 287 comment lines against 232 lines of code.

The problem was not the volume in itself but **what** was being written. Most of
the comments explained how the code works, which is what the code already says
and what only changes when the code changes. An observation like "this loop is a
plain `break`" does not show up in a test: it shows up in the git log, and by
then it is noise.

The rest was real information, but buried: WHY a piece of code was implemented
one way rather than the obvious alternative. Cases that came up repeatedly when
reading the code:

- `shellSafe` does not include `#` in its list of safe characters, because at
  the start of a word it opens a comment in the shell and a `git checkout -b
  '#123'` branch vanished from the command with no visible error.
- `gitcmd.Env` filters out the localisation `GIT_*` variables because they beat
  `cmd.Dir`: with `GIT_DIR` set, `git config` writes into that repo no matter
  which directory it runs from, which is how the wrong branch gets deleted.
- `FitCells` pays the height at double, because with 1x2 cells a 16:9 image
  needs 3.56 columns per line and not 1.78, and without the factor the two
  commits of a row look like a strip of ellipses.
- The guards that "cannot fire" and were removed (the inner gap of the popup,
  `perRow <= 0` in `FitCells`, the later `if` of `centeredOrigin`), with the
  arithmetic written down so nobody adds them back.

Each one of those is impossible to reconstruct by reading the code. It is
exactly the content a comment should hold.

## Decision

Two rules.

**Language: English.** The language of the code is English, and a comment in a
language different from the rest of the code forces a context switch to read it.
ADRs, the README and the user documentation stayed in Spanish, because those
are read by the operator, not by whoever reads the diff. That half of the rule
was later reversed by ADR 0010: everything is English now, docs included.

**Criterion: a comment stays only if it justifies the WHY.** Specifically, if it
explains a decision, a constraint or a trap that the code cannot say by itself.
Everything that describes behaviour is deleted: doc comments that repeat the
function name, field comments, "what this loop does" comments, and the ones
explaining a Go mechanism the reader already knows.

What stays is compressed to **one line**, or two when the decision has two
sides. The test is arithmetic: an explanation that needs ten lines is usually an
explanation of *what*, not *why*, and if it really is a *why* it fits in the
sentence that states the decision.

## Consequences

- The production code goes from 4,896 to ~1,000 comment lines, and from 15,140
  to ~11,000 lines in total. The tests, which were at 14,524, were left out of
  this change; they are handled separately.
- A comment cannot go stale if it states a decision and not a behaviour:
  decisions change less often than code.
- What the *what* comments added is lost: orientation for whoever enters a
  file. That cost is accepted because the git log covers the same ground and the
  code is split into functions whose names already say it.
- The rule erodes on its own. A criterion that depends on somebody remembering
  it undoes itself in the next commit, so it lives here and in `AGENTS.md`, not
  in habit.

## Rejected alternatives

**Delete them all.** Not viable: half of the code's decisions cannot be checked
without their reason, and without the comment nobody writes the reason again. A
codebase with no explanation of its traps gets repaired twice.

**Keep them only in the "complex" files.** That turns the criterion into
"measurable per file", which is subjective and produces the opposite effect: the
file that needs it most is the one that ends up with the most comments, so it
becomes over-represented.

**One line per file instead of one per decision.** Shorter and easier to read,
but it forces a choice of which decision deserves the comment, and that choice
is made badly from the outside.