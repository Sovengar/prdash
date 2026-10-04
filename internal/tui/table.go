// Rows, columns and cells of the inbox. Each cell returns plain text and style separately, and
// the render pads BEFORE styling, so ANSI codes never break the width.
package tui

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"prdash/internal/forge/model"
	"prdash/internal/state"
)

type tableColumn struct {
	title string
	width int
}

const (
	colForgeIdx = iota
	colRefIdx
	colTitleIdx
	colRoleIdx
	colStateIdx
	colChecksIdx
	colDiffIdx
)

// In priority order, dropped from the right at narrow widths. DIFF goes last: it is the only
// column that can be lost without losing information.
var tableColumns = []tableColumn{
	{"FORGE", colForge},
	{"ITEM", itemWidthMin},
	{"TITLE", colTitle},
	{"ROLE", colRole},
	{"STATE", colState},
	{"CHECKS", colChecks},
	{"DIFF", colDiff},
}

type span struct {
	text  string
	style lipglossStyle
}

type cell struct {
	text  string
	style lipglossStyle
	width int
	spans []span
}

// A column's width is its total size including the gap, not the room for its text: filling it
// exactly would leave pad() nothing and the cell would touch the next one.
func textWidth(w int) int {
	return max(w-1, 1)
}

func fitColumns(l refLayout, innerWidth int) int {
	used := 0
	for i, c := range l.cols {
		if used+c.width > innerWidth {
			return max(1, i)
		}
		used += c.width
	}
	return len(l.cols)
}

func headerLine(l refLayout, innerWidth int) string {
	var b strings.Builder
	for _, c := range l.cols[:fitColumns(l, innerWidth)] {
		b.WriteString(pad(c.title, c.width))
	}
	return styleCount.Render(strings.TrimRight(b.String(), " "))
}

func itemCells(it model.Item, sec model.Section, viewer string, l refLayout) []cell {
	refW := l.cols[colRefIdx].width
	// Clipped to textWidth rather than to the width, which is what leaves the gap even when the text fills
	// the column. Each column keeps its own clipping strategy (ITEM by the tail, the rest by the head).
	return []cell{
		{text: truncate(forgeBadge(it), textWidth(l.cols[colForgeIdx].width)), style: styleForge, width: l.cols[colForgeIdx].width},
		{text: truncateTail(refCellText(it, l.mode, l.prefixOf(sec)), textWidth(refW)), style: styleRef, width: refW},
		{text: truncate(it.Title, textWidth(l.cols[colTitleIdx].width)), style: styleTitle, width: l.cols[colTitleIdx].width},
		{text: truncate(roleText(it, viewer), textWidth(l.cols[colRoleIdx].width)), style: styleRole, width: l.cols[colRoleIdx].width},
		{text: truncate(state.Derive(it).String(), textWidth(l.cols[colStateIdx].width)), style: styleForState(state.Derive(it)), width: l.cols[colStateIdx].width},
		{text: truncate(checksText(it.Checks), textWidth(l.cols[colChecksIdx].width)), style: styleChecks(it.Checks), width: l.cols[colChecksIdx].width},
		diffCell(it.Diff, l.cols[colDiffIdx].width),
	}
}

func renderCells(cells []cell, l refLayout, innerWidth int) string {
	var b strings.Builder
	for _, c := range cells[:min(len(cells), fitColumns(l, innerWidth))] {
		b.WriteString(renderCell(c))
	}
	return b.String()
}

// Padding is measured on plain text, never on runes of an already coloured string.
func renderCell(c cell) string {
	if len(c.spans) == 0 {
		return c.style.Render(pad(c.text, c.width))
	}
	used := 0
	for _, s := range c.spans {
		used += utf8.RuneCountInString(s.text)
	}
	// Padding goes at the end of the last span, measured on what that span occupies: `used` includes
	// it, so without adding it back the cell measures less than its column.
	slack := c.width - used
	var b strings.Builder
	last := len(c.spans) - 1
	for i, s := range c.spans {
		text := s.text
		if i == last {
			text = pad(text, utf8.RuneCountInString(text)+slack)
		}
		b.WriteString(s.style.Render(text))
	}
	return b.String()
}

func forgeLabel(it model.Item) string {
	if it.Host == "" {
		return it.Forge
	}
	return it.Forge + "@" + it.Host
}

var forgeShortNames = map[string]string{
	"github":    "GH",
	"gitlab":    "GLab",
	"bitbucket": "BB",
}

var forgePublicHosts = map[string]string{
	"github":    "github.com",
	"gitlab":    "gitlab.com",
	"bitbucket": "bitbucket.org",
}

func forgeBadge(it model.Item) string {
	short := forgeShortNames[it.Forge]
	if short == "" {
		short = it.Forge // forge sin abreviatura conocida: se muestra tal cual
	}
	if short == "" || it.Host == "" {
		return short
	}
	if strings.EqualFold(it.Host, forgePublicHosts[it.Forge]) {
		return short
	}
	label, _, _ := strings.Cut(it.Host, ".")
	return short + "@" + label
}

func refLabel(it model.Item) string {
	return it.Ref.Project + "#" + strconv.Itoa(it.Number)
}

// "own" is the signal that approve is not going to work, before pressing it.
func roleText(it model.Item, viewer string) string {
	switch it.ReviewKind {
	case model.ReviewRequested:
		return "review req"
	case model.ReviewAssigned:
		return "assigned"
	}
	if ok, _ := state.CanApprove(it, viewer); !ok {
		return "own"
	}
	return "-"
}

func checksText(c model.Checks) string {
	switch c.State {
	case model.ChecksFailing:
		return "✗" + strconv.Itoa(c.Failing)
	case model.ChecksPending:
		return "…" + strconv.Itoa(c.Pending)
	case model.ChecksPassing:
		return "✓"
	default:
		return "-"
	}
}

func styleChecks(c model.Checks) lipglossStyle {
	switch c.State {
	case model.ChecksFailing:
		return styleChecksFailing
	case model.ChecksPending:
		return styleChecksPending
	case model.ChecksPassing:
		return styleChecksPassing
	default:
		return styleDim
	}
}

// Compact so the width does not depend on whether the PR touches 40 lines or 40,000. An unknown
// diffstat is "-": inventing a zero would read as "touches nothing".
func diffColumnText(d model.DiffStat) string {
	if !d.Known {
		return "-"
	}
	return "+" + compactCount(d.Additions) + " -" + compactCount(d.Deletions)
}

// The text is formatted once, in plain, and both what is measured and what is coloured come from it,
// so the width and the colour can never disagree.
func diffCell(d model.DiffStat, width int) cell {
	text := truncate(diffColumnText(d), textWidth(width))
	if !d.Known {
		return cell{text: text, style: styleDiffUnknown, width: width}
	}
	return cell{width: width, spans: diffSpans(text)}
}

// Diff notation and nothing else: which lines are new and which disappeared. Nothing that does
// not fit the shape is coloured, because colouring a non-number would lie about the datum.
func diffSpans(plain string) []span {
	add, rest, ok := strings.Cut(plain, " ")
	if !ok || !isDiffCount(add, '+') {
		return nil
	}
	del, tail, hasTail := strings.Cut(rest, " ")
	if !isDiffCount(del, '-') {
		return nil
	}
	spans := []span{{add, styleDiffAdd}, {" ", styleTitle}, {del, styleDiffDel}}
	if hasTail && tail != "" {
		spans = append(spans, span{" " + tail, styleTitle})
	}
	return spans
}

// Colouring AFTER clipping: the detail's grid width is computed on the plain text, so colouring first
// would make truncate count the ANSI codes as if they were digits.
func styleDiffText(plain string) string {
	spans := diffSpans(plain)
	if spans == nil {
		return plain
	}
	var b strings.Builder
	for _, s := range spans {
		b.WriteString(s.style.Render(s.text))
	}
	return b.String()
}

func isDiffCount(s string, sign byte) bool {
	if len(s) < 2 || s[0] != sign {
		return false
	}
	for i := 1; i < len(s); i++ {
		switch c := s[i]; {
		case c >= '0' && c <= '9', c == '.', c == 'k':
		default:
			return false
		}
	}
	return true
}

// Rounded in integer tenths rather than with fmt, because 9999 with one decimal comes out
// "10.0k": four runes and no meaning. Rounding up out of the window falls back to integers.
func compactCount(n int) string {
	switch {
	case n < 0:
		return strconv.Itoa(n)
	case n < 1000:
		return strconv.Itoa(n)
	case n < 10000:
		if tenths := (n + 50) / 100; tenths < 100 {
			return strconv.Itoa(tenths/10) + "." + strconv.Itoa(tenths%10) + "k"
		}
		return strconv.Itoa((n+500)/1000) + "k"
	default:
		return strconv.Itoa(n/1000) + "k"
	}
}

func pad(s string, w int) string {
	n := utf8.RuneCountInString(s)
	if n >= w {
		return s
	}
	return s + strings.Repeat(" ", w-n)
}

func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= w {
		return s
	}
	runes := []rune(s)
	if w == 1 {
		return "…"
	}
	return string(runes[:w-1]) + "…"
}

func stripANSI(s string) string {
	var out strings.Builder
	inSeq := false
	for _, r := range s {
		switch {
		case r == '\x1b':
			inSeq = true
		case inSeq && (r == 'm' || r == 'K'):
			inSeq = false
		case !inSeq:
			out.WriteRune(r)
		}
	}
	return out.String()
}
