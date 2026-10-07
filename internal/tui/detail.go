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

type detailField struct{ key, value string }

const labelWidth = 14

const detailGap = 4

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
	}
	status := []detailField{
		{"State", state.Derive(it).String()},
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

	avail := commentBudget(rows, len(grid), len(url), len(avisos))
	comments := m.commentLines(it, avail, inner)

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

func (m *Model) detailWarnings(it model.Item) []string {
	if reason := m.denied[it.ID()]; reason != "" {
		return []string{"", styleWarn.Render("  action disabled: " + reason)}
	}
	if st := m.statuses[it.Forge]; st != nil && !st.auth.OK {
		return []string{"", styleWarn.Render("  " + it.Forge + ": " + authReason(st.auth))}
	}
	return nil
}

func authReason(auth model.AuthState) string {
	if auth.Reason != "" {
		return auth.Reason
	}
	return "not authenticated"
}

func fullWidthField(f detailField, inner int) string {
	return label(f.key, styleDiffText(truncate(f.value, max(1, inner-labelWidth))))
}

func commentBudget(rows, gridLines, urlLines, warningLines int) int {
	return rows - gridLines - urlLines - 2 - warningLines
}

func detailGrid(fields []detailField, inner int) []string {
	cell := max(24, (inner-detailGap)/2)
	var out []string
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

func clipTop(lines []string, rows int) []string {
	if rows <= 0 {
		return lines
	}
	return lines[max(0, len(lines)-rows):]
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
	return relativeSince(time.Since(t))
}

func relativeSince(d time.Duration) string {
	if d < time.Minute {
		return "now"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
