package tui

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"prdash/internal/forge/model"
	"prdash/internal/state"
)

func (m *Model) detailPane(rows int) []string {
	it, ok := m.selected()
	return m.detailLines(it, ok, rows)
}

// Kept as data rather than as pre-laid-out text so the panel can choose the arrangement.
type detailField struct{ key, value string }

const labelWidth = 14

const detailGap = 4

// Three blocks: the short fields in a two-column grid, the URL on a full-width row, and the comments
// below. Always in the grid, not only when the fields do not fit in one column: in a single column the
// card took 16 of the ~18 lines the 40% gives and not one comment fitted.
//
// Pulling the URL out of the grid costs no height, which is what used to look wrong: the 13 fields take
// 7 rows in two columns, and the remaining 12 plus the full-width URL still take 7. What changes is
// that the URL reads whole, and a URL that cannot be copied is no use at all.
func (m *Model) detailLines(it model.Item, ok bool, rows int) []string {
	if !ok {
		return []string{styleDim.Render("no selection: move the cursor onto an item")}
	}

	title := styleDetailTitle.Render(it.Title)
	viewer := m.viewerLogin(it.Forge)
	identity := []detailField{
		{"Item", refLabel(it)},
		{"Forge", forgeLabel(it)},
		{"Author", orDash(it.Author)},
		{"Source", orDash(it.SourceBranch)},
		{"Target", orDash(it.TargetBranch)},
		// There is no Number field because refLabel already ends in "#42": a whole grid row to repeat the
		// number three columns away. Draft takes that space, since it brings a datum that was not there.
	}
	// Review and Role go last on purpose: they are the two that say whether the action applies, and if the
	// clip eats them the card stops answering the question it exists for.
	status := []detailField{
		{"State", state.Derive(it).String()},
		// Draft is not inside State because they are different questions. State is attention priority, and by
		// its precedence an approved draft comes out as "approved" and the draft is lost.
		{"Draft", yesNo(it.IsDraft)},
		{"Checks", checksDetail(it.Checks)},
		{"Diff", diffDetail(it.Diff)},
		{"Updated", relativeTime(it.UpdatedAt)},
		{"Review", orDash(reviewLabel(it))},
		{"Role", roleText(it, viewer)},
	}
	inner := m.contentWidth()
	grid := detailGrid(withField(identity, status), inner)
	noDiff := detailGrid(withField(identity, withoutField(status, "Diff")), inner)
	url := []string{fullWidthField(detailField{"URL", orDash(it.URL)}, inner)}
	avisos := m.detailWarnings(it)

	// The comments' budget is what is left after the card's header, and it is computed before composing
	// them because it decides how many rows each may spend. Without it a block of five four-line comments
	// would not fit an 18-row panel and the ladder would drop the whole thing.
	avail := commentBudget(rows, len(grid), len(url), len(avisos))
	comments := m.commentLines(it, avail, inner)

	// What goes first is what can be asked for again, then the most recent. Comments go first because they
	// are the only thing that can be re-requested with a key (or the next tick) and the only thing that was
	// not on the card before this section existed; then the gap after the title, which is decorative and
	// the title's own style compensates for; then the URL row, a whole row sacrificed for a datum that `o`
	// re-reads; and last the diffstat, on the criterion the grid already had: a datum just lost is worse
	// than one never painted. Same criterion as the list's DIFF column, which is also last.
	//
	// The jump from the fourth to the fifth candidate drops three things at once because there is no
	// intermediate size: dropping only the URL leaves the same height, and dropping only the diffstat too,
	// since the grid goes from 13 to 12 fields and both fit in 6 rows.
	withGap := []string{title, ""}
	noGap := []string{title}
	layouts := [][]string{
		stackDetail(withGap, grid, url, comments, avisos),
		stackDetail(withGap, grid, url, avisos),
		stackDetail(noGap, grid, url, comments, avisos),
		stackDetail(noGap, grid, url, avisos),
		stackDetail(noGap, grid, noDiff, avisos),
	}
	for _, l := range layouts {
		if len(l) <= rows {
			return l
		}
	}
	return clipTop(stackDetail(noGap, noDiff, avisos), rows)
}

func withField(blocks ...[]detailField) []detailField {
	var out []detailField
	for _, b := range blocks {
		out = append(out, b...)
	}
	return out
}

func stackDetail(parts ...[]string) []string {
	var out []string
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func withoutField(fields []detailField, key string) []detailField {
	out := make([]detailField, 0, len(fields))
	for _, f := range fields {
		if f.key != key {
			out = append(out, f)
		}
	}
	return out
}

// The forge's veto is also sticky: it is remembered per item until a refresh lifts it, so it can still be
// there after the header stopped warning, which is why it earns a row.
//
// The own-approval veto is deliberately NOT painted: it derives from the item and the login, so it
// would show on every render of every one of your PRs — which is nearly all of "Created by me" — and
// the card would end with a permanent line repeating what the Role field already says. It is also the
// only one that can be asked for elsewhere: the reason is delivered on the keypress, in the warning,
// which is when it can be acted on.
//
// An unauthenticated forge IS painted, with its reason, which is exactly what used to get lost: the
// adapter tells "the token is no good" from "this forge is not implemented", and the generic
// "not authenticated" label sent the user to authenticate a token that already worked.
func (m *Model) detailWarnings(it model.Item) []string {
	if reason := m.denied[it.ID()]; reason != "" {
		return []string{"", styleWarn.Render("  action disabled: " + reason)}
	}
	if st := m.statuses[it.Forge]; st != nil && !st.auth.OK {
		return []string{"", styleWarn.Render("  " + it.Forge + ": " + authReason(st.auth))}
	}
	return nil
}

// The reason decides what the operator does: "not authenticated" is fixed by resuming the token and
// "not implemented" is not fixed at all. Confusing them costs a whole debugging session.
func authReason(auth model.AuthState) string {
	if auth.Reason != "" {
		return auth.Reason
	}
	return "not authenticated"
}

// It exists for the URL, for a concrete reason: a self-managed GitLab URL with a subfolder easily
// goes past 80 characters, so in half a column you read 40 and get a useless remainder. The full
// width makes it readable without spending another row, since the 12 short fields still fit in the
// same 6.
func fullWidthField(f detailField, inner int) string {
	return label(f.key, styleDiffText(truncate(f.value, max(1, inner-labelWidth))))
}

// Its own function because it is the arithmetic that decides how much is seen, and inside the composition
// that arithmetic was hidden: a bigger budget does not give a taller block, it gives a block that
// clipTop trims at the end, so the trim cancelled out and the last line never changed. As a pure
// function the number is checked directly and the trim is seen for what it is.
//
// The 2 are the card's title and the blank line separating it from the fields: not comment content,
// but they take rows, so they are discounted. A budget that did not would give one comment row too
// many and the step below would lose it without warning.
//
// It may come out NEGATIVE and that is correct: it says the header does not fit even alone. A negative
// budget paints no comments rather than trying to paint a negative number of them.
func commentBudget(rows, gridLines, urlLines, warningLines int) int {
	return rows - gridLines - urlLines - 2 - warningLines
}

func detailGrid(fields []detailField, inner int) []string {
	cell := max(24, (inner-detailGap)/2)
	out := make([]string, 0, (len(fields)+1)/2)
	for i := 0; i < len(fields); i += 2 {
		left, leftW := detailCell(fields[i], cell)
		right := ""
		if i+1 < len(fields) {
			right, _ = detailCell(fields[i+1], cell)
		}
		out = append(out, left+strings.Repeat(" ", max(detailGap, cell-leftW))+right)
	}
	return out
}

func detailCell(f detailField, width int) (string, int) {
	value := truncate(f.value, max(1, width-labelWidth))
	return label(f.key, styleDiffText(value)), labelWidth + utf8.RuneCountInString(value)
}

// The END of the detail is the part that says whether the action applies, so that is what survives.
func clipTop(lines []string, rows int) []string {
	if rows <= 0 || len(lines) <= rows {
		return lines
	}
	return lines[len(lines)-rows:]
}

func label(key, value string) string {
	return styleDetailKey.Render(pad(key+":", labelWidth)) + value
}

func reviewLabel(it model.Item) string {
	decision := it.ReviewDecision
	switch decision {
	case "APPROVED":
		decision = "approved"
	case "CHANGES_REQUESTED":
		decision = "changes requested"
	case "REVIEW_REQUIRED":
		decision = "review required"
	}
	if decision == "" {
		decision = it.State
	}
	switch it.ReviewKind {
	case model.ReviewRequested:
		return decision + " · review requested"
	case model.ReviewAssigned:
		return decision + " · assigned"
	default:
		return decision
	}
}

func checksDetail(c model.Checks) string {
	if c.Total == 0 && c.State == model.ChecksUnknown {
		return "no checks"
	}
	base := string(c.State)
	if c.Failing > 0 || c.Pending > 0 {
		base += fmt.Sprintf(" (%d/%d failing, %d pending)", c.Failing, c.Total, c.Pending)
	} else {
		base += fmt.Sprintf(" (%d)", c.Total)
	}
	return base
}

func diffDetail(d model.DiffStat) string {
	if !d.Known {
		return "unknown (forge did not report it)"
	}
	if d.Total() == 0 {
		return "no changes"
	}
	noun := "files"
	if d.Files == 1 {
		noun = "file"
	}
	return fmt.Sprintf("+%d -%d (%d %s)", d.Additions, d.Deletions, d.Files, noun)
}

func relativeTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// Not orDash: a false here is the answer, not a missing datum.
func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
