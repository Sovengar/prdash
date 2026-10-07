package tui

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/forge/parse"
	"prdash/internal/tui/bordered"
)

type commentState struct {
	list  []model.Comment
	total int  // what the forge says there are, so "5 of 23" can be said
	ready bool // the query finished, either way
	err   string
}

const maxCommentLines = 4

func (m *Model) commentsCmd() tea.Cmd {
	return tea.Tick(commentsPoll, func(time.Time) tea.Msg { return commentsTickMsg{} })
}

func (m *Model) requestComments() tea.Cmd {
	tick := m.commentsCmd()
	it, ok := m.selected()
	if !ok {
		return tick
	}
	id := it.ID()
	if _, done := m.comments[id]; done {
		return tick
	}
	a := m.byForge[it.Forge]
	if a == nil {
		return tick
	}
	if st := m.statuses[it.Forge]; st != nil && !st.auth.OK {
		return tick
	}

	m.comments[id] = &commentState{}

	appCtx := m.ctx
	events := m.events
	ref, number := it.Ref, it.Number
	go func() {
		ctx, cancel := context.WithTimeout(appCtx, commentsTimeout)
		defer cancel()
		page, warns := a.Comments(ctx, ref, number)
		msg := commentsMsg{id: id, page: page}
		if len(warns) > 0 && len(page.Comments) == 0 {
			msg.err = warns[0].Msg
		}
		sendEvent(appCtx, events, msg)
	}()
	return tick
}

func (m *Model) applyComments(msg commentsMsg) {
	st := &commentState{
		list:  msg.page.Comments,
		total: msg.page.Total,
		ready: true,
		err:   msg.err,
	}
	st.total = max(st.total, len(st.list))
	m.comments[msg.id] = st
}

func (m *Model) commentLines(it model.Item, avail, inner int) []string {
	if avail <= 0 {
		return nil
	}
	st := m.comments[it.ID()]
	if st == nil {
		return nil
	}
	if !st.ready {
		return []string{label("Comments", styleDim.Render("loading…"))}
	}
	if st.err != "" {
		return []string{label("Comments", styleWarn.Render(truncate("not read: "+st.err, inner-labelWidth)))}
	}
	if len(st.list) == 0 {
		return []string{label("Comments", styleDim.Render("none"))}
	}

	if avail < commentChrome+len(st.list) {
		return nil
	}
	budget := avail - commentChrome

	shown := st.list[:min(forge.CommentLimit, len(st.list))]
	bodyWidth := commentBodyWidth(inner)

	need := make([]int, len(shown))
	for i, c := range shown {
		need[i] = len(commentBody(c, maxCommentLines, bodyWidth))
	}

	body := make([]string, 0, budget)

	rows := allocate(need, budget)

	for i, c := range shown {
		body = append(body, commentBody(c, rows[i], bodyWidth)...)
	}
	return commentBox(body, commentLegend(len(shown), st.total, inner), inner)
}

// Both titles are embedded in them —"Comments" above and the count below — so neither costs an extra row.
const commentChrome = 2

const commentBoxBorder = 2

const commentTitle = "Comments"

func commentBox(body []string, legend string, outer int) []string {
	border := bordered.Rounded()
	legend = " " + legend + styleBorder.Render(" "+border.Bottom)
	lines := strings.Split(bordered.RenderWithTitles(
		border, borderColor, " "+commentTitle+" ", bordered.AlignLeft,
		legend, bordered.AlignRight,
		strings.Join(body, "\n"), commentBoxWidth(outer),
	), "\n")

	inset := strings.Repeat(" ", commentInset)
	for i, l := range lines {
		lines[i] = inset + l + inset
	}
	return lines
}

func commentBodyWidth(outer int) int {
	return max(8, commentBoxWidth(outer)-commentBoxBorder)
}

// The inset keeps the box's columns off its border; a lighter grey went yellowish on warm palettes.
const commentInset = 1

// One subtraction per side, not `2*commentInset`: the product's only mutant divides and ties.
func commentBoxWidth(outer int) int { return max(8, outer-commentInset-commentInset) }

// One at a time, not neediest first: levelling shares the damage instead of favouring the first writer.
func allocate(need []int, budget int) []int {
	rows := make([]int, len(need))
	for i := range need {
		rows[i] = 1
	}
	for left := budget - len(need); left > 0; left-- {
		// Least share and text left; ties to the lower index so map order cannot decide.
		best := -1
		for i := range need {
			if rows[i] >= need[i] {
				continue
			}
			if best < 0 || rows[i] < rows[best] {
				best = i
			}
		}
		if best < 0 {
			break // nobody can absorb more
		}
		rows[best]++
	}
	return rows
}

// It says where the conversation continues; kept apart so it is what a narrow panel drops.
const commentHint = " · open the PR to read the rest"

const legendGap = 3

// Both numbers always stated: the suffix only when it fits whole and something is hidden.
func commentLegend(shown, total, outer int) string {
	legend := fmt.Sprintf("%d of %d", shown, total)
	room := commentBoxWidth(outer) - commentBoxBorder - legendGap
	if total > shown && utf8.RuneCountInString(legend+commentHint) <= room {
		legend += commentHint
	}
	return styleCount.Render(legend)
}

func commentBody(c model.Comment, lines, inner int) []string {
	lines = max(1, lines)
	author := truncate(c.Author, maxCommentAuthor)
	first, cont := commentWidths(inner, author)

	var (
		out       []string
		lastPiece string
		lastW     int
	)
	for _, src := range parse.CommentLines(c.Body) {
		w := cont
		if len(out) == 0 {
			w = first
		}
		for _, piece := range wrapText(src, w) {
			if len(out) >= lines {
				// Text left unwritten: the last row is recomposed marking the cut.
				out[len(out)-1] = commentRow(len(out)-1, author, commentSep, lastPiece, lastW, true)
				return out
			}
			out = append(out, commentRow(len(out), author, commentSep, piece, w, false))
			lastPiece, lastW = piece, w
		}
	}

	if len(out) == 0 {
		return []string{commentRow(0, author, commentSep, "(no text)", first, false)}
	}
	return out
}

func commentWidths(inner int, author string) (first, cont int) {
	first = max(8, inner-utf8.RuneCountInString(commentIndent+author+commentSep))
	cont = max(8, inner-utf8.RuneCountInString(contIndent))
	return first, cont
}

func commentRow(idx int, author, sep, piece string, w int, cut bool) string {
	indent, prefix := contIndent, ""
	if idx == 0 {
		indent, prefix = commentIndent, styleDetailKey.Render(author+sep)
	}
	runes := []rune(piece)
	n := utf8.RuneCountInString(piece)
	if cut {
		room := max(1, w-1)
		if n <= room {
			return indent + prefix + piece + "…"
		}
		return indent + prefix + clipRunes(runes, room)
	}
	return indent + prefix + clipRunes(runes, w)
}

func clipRunes(runes []rune, n int) string {
	if n >= len(runes) {
		return string(runes)
	}
	if n <= 0 {
		return ""
	}
	if n == 1 {
		return "…"
	}
	return string(runes[:n-1]) + "…"
}

const maxCommentAuthor = 24

const (
	commentIndent = "  "
	contIndent    = "    "
	commentSep    = ": "
)
