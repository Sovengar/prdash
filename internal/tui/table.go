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

func renderCell(c cell) string {
	if len(c.spans) == 0 {
		return c.style.Render(pad(c.text, c.width))
	}
	used := 0
	for _, s := range c.spans {
		used += utf8.RuneCountInString(s.text)
	}
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
		short = it.Forge // forge with no known abbreviation: shown as is
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

func diffColumnText(d model.DiffStat) string {
	if !d.Known {
		return "-"
	}
	return "+" + compactCount(d.Additions) + " -" + compactCount(d.Deletions)
}

func diffCell(d model.DiffStat, width int) cell {
	text := truncate(diffColumnText(d), textWidth(width))
	if !d.Known {
		return cell{text: text, style: styleDiffUnknown, width: width}
	}
	return cell{width: width, spans: diffSpans(text)}
}

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
		c := s[i]
		if (c >= '0' && c <= '9') || c == '.' || c == 'k' {
			continue
		}
		return false
	}
	return true
}

func compactCount(n int) string {
	if n < 1000 {
		return strconv.Itoa(n)
	}
	if n <= 9999 {
		if tenths := (n + 50) / 100; tenths < 100 {
			return strconv.Itoa(tenths/10) + "." + strconv.Itoa(tenths%10) + "k"
		}
		return strconv.Itoa((n+500)/1000) + "k"
	}
	return strconv.Itoa(n/1000) + "k"
}

func pad(s string, w int) string {
	return s + strings.Repeat(" ", max(0, w-utf8.RuneCountInString(s)))
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
		if r == '\x1b' {
			inSeq = true
		} else if inSeq && (r == 'm' || r == 'K') {
			inSeq = false
		} else if !inSeq {
			out.WriteRune(r)
		}
	}
	return out.String()
}
