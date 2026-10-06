// The selected item's conversation, inside the detail panel. Comments are not a separate view the
// user asks for, so they are fetched when the cursor arrives: N extra calls per cycle otherwise.
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

// Storing the state and not just the list is what lets "no comments" and "I have not asked yet"
// paint differently: with only the list both are an empty list, and the first pretends otherwise.
type commentState struct {
	list  []model.Comment
	total int  // what the forge says there are, so "5 of 23" can be said
	ready bool // the query finished, either way
	err   string
}

// Without it a long comment eats the whole panel on a tall terminal and the other four are never
// seen: five comments were asked for, not one.
const maxCommentLines = 4

func (m *Model) commentsCmd() tea.Cmd {
	return tea.Tick(commentsPoll, func(time.Time) tea.Msg { return commentsTickMsg{} })
}

// Does not touch the events channel: sendEvent does not consume the bomb's reader, so the
// single-reader invariant holds and the re-armed tick keeps the chain from being cut.
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
	// Without a session it does not ask: the card deduces it from the forge's state, and spending a call
	// to fail again tells the user nothing the header did not already say.
	if st := m.statuses[it.Forge]; st != nil && !st.auth.OK {
		return tick
	}

	// Marked as asked BEFORE leaving: otherwise two consecutive ticks on the same item — the first still
	// in flight, the second already without an answer — would fire the same query twice.
	m.comments[id] = &commentState{}

	appCtx := m.ctx
	events := m.events
	ref, number := it.Ref, it.Number
	go func() {
		ctx, cancel := context.WithTimeout(appCtx, commentsTimeout)
		defer cancel()
		page, warns := a.Comments(ctx, ref, number)
		msg := commentsMsg{id: id, page: page}
		// A warning only becomes an error if nothing came back: what we have is better than an empty
		// card because of a warning that does not stop the reading.
		if len(warns) > 0 && len(page.Comments) == 0 {
			msg.err = warns[0].Msg
		}
		sendEvent(appCtx, events, msg)
	}()
	return tick
}

// No cycle check: the answer belongs to the item that was asked, not to the view, so it is still
// valid even if the inbox refreshed while it was in flight (same policy as applyAction's re-read).
func (m *Model) applyComments(msg commentsMsg) {
	st := &commentState{
		list:  msg.page.Comments,
		total: msg.page.Total,
		ready: true,
		err:   msg.err,
	}
	// GitLab exposes no count, so the total is what was read. It is floored at the list's height so the
	// count arithmetic does not invent comments that are not there.
	st.total = max(st.total, len(st.list))
	m.comments[msg.id] = st
}

// Comments go in their own titled box rather than as more card fields: a conversation is what
// people said about the PR, and a border says so without an explanation.
func (m *Model) commentLines(it model.Item, avail, inner int) []string {
	if avail <= 0 {
		return nil
	}
	st := m.comments[it.ID()]
	if st == nil {
		// Not asked and nothing in flight. The next tick might not even ask: without a session it does
		// not, and the header already says so. "Loading…" here would announce a query that does not exist.
		return nil
	}
	if !st.ready {
		// In flight: said rather than left blank, because a blank is indistinguishable from "this PR has
		// no comments" and we do not know that yet.
		return []string{label("Comments", styleDim.Render("loading…"))}
	}
	if st.err != "" {
		// No floor on the width: `inner` is never below 38 and labelWidth is 14, so the difference is
		// never negative. A floor would guard an impossible negative width.
		return []string{label("Comments", styleWarn.Render(truncate("not read: "+st.err, inner-labelWidth)))}
	}
	if len(st.list) == 0 {
		// No comments, no box: the box exists to separate the conversation from the fields, and a box
		// around the word "none" separates nothing.
		return []string{label("Comments", styleDim.Render("none"))}
	}

	// Never painted half: a top border with no bottom one is not a half box, it is noise in the
	// same room. And a clipped box does not look clipped — it looks like the only comment.
	if avail < commentChrome+len(st.list) {
		return nil
	}
	budget := avail - commentChrome

	// Bounded here and not only in the adapter. CommentLimit is the card's decision, and a card that
	// skipped it because it trusted whoever filled it would show comments that do not fit its own height.
	shown := st.list[:min(forge.CommentLimit, len(st.list))]
	// There is deliberately NO second floor on `len(shown)`: it would be `avail < chrome+len(shown)`
	// and the guard above already implies it, since `len(shown) <= len(st.list)` by the cap of five.

	bodyWidth := commentBodyWidth(inner)

	// Each comment's needed rows are counted by composing it at the tall cap, with the same code
	// that paints it, so the allocation cannot lie about what fits.
	need := make([]int, len(shown))
	for i, c := range shown {
		need[i] = len(commentBody(c, maxCommentLines, bodyWidth))
	}

	// The count lives on the bottom border, not in the body: every body row is something a person
	// wrote, and on the border it costs no height when the panel is tight.
	body := make([]string, 0, budget)

	// When they fit, each takes what it needs, which stops a six-paragraph comment starving four
	// one-liners. When they do not, everyone gets one row and the surplus goes to whoever has most
	// to lose. There is deliberately no fit check around the call: while the budget covers the sum,
	// allocate hands out the leftovers until every comment sits at its own count and returns `need`
	// itself, so one call covers both cases.
	rows := allocate(need, budget)

	for i, c := range shown {
		// No safety net here, and there used to be: `budget >= len(need)` already, so allocate
		// distributes `budget - len(need)`. A net that cannot fire is worse than none.
		body = append(body, commentBody(c, rows[i], bodyWidth)...)
	}
	return commentBox(body, commentLegend(len(shown), st.total, inner), inner)
}

// Both titles are embedded in them —"Comments" above and the count below — so neither costs an extra row.
const commentChrome = 2

const commentBoxBorder = 2

const commentTitle = "Comments"

// `outer` is the width inside the detail panel: the box is inset within it so it sits inside rather
// than stepping on the outer border.
func commentBox(body []string, legend string, outer int) []string {
	border := bordered.Rounded()
	// The dash goes OUTSIDE the count's style, and outside is not unstyled: a reset does not restore
	// the border's grey, so without repainting the dash came out in the foreground colour.
	legend = " " + legend + styleBorder.Render(" "+border.Bottom)
	lines := strings.Split(bordered.RenderWithTitles(
		border, borderColor, " "+commentTitle+" ", bordered.AlignLeft,
		legend, bordered.AlignRight,
		strings.Join(body, "\n"), commentBoxWidth(outer),
	), "\n")

	// Indented on both sides: with only the left one, the box ends up a column narrower than the panel
	// and the outer border looks displaced.
	inset := strings.Repeat(" ", commentInset)
	for i, l := range lines {
		lines[i] = inset + l + inset
	}
	return lines
}

// Its own function: a body two columns wider does not give a wider box — the box clips either
// way — but two fewer characters per line, and no render test sees that.
func commentBodyWidth(outer int) int {
	return max(8, commentBoxWidth(outer)-commentBoxBorder)
}

// Without it the box shares columns with its border and every row comes out as `||`. A lighter
// grey told them apart but goes yellowish on warm palettes: a colour fix for a shape problem.
const commentInset = 1

// One subtraction per side and not `2*commentInset`: the product's only mutant divides by an inset
// of one and comes out the same value, a survivor no test can kill.
func commentBoxWidth(outer int) int { return max(8, outer-commentInset-commentInset) }

// One at a time, not neediest first: levelling shares the damage among those who will suffer it,
// while "the first keeps it" depends only on who wrote first.
func allocate(need []int, budget int) []int {
	rows := make([]int, len(need))
	for i := range need {
		rows[i] = 1
	}
	for left := budget - len(need); left > 0; left-- {
		// The one with the least share and text still left. Ties go to the lower index, so the allocation does
		// not depend on the map's iteration order.
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

// It does not just say there is more conversation, it says where. Kept apart so that in a narrow panel
// it is what goes, not the numbers.
const commentHint = " · open the PR to read the rest"

const legendGap = 3

// Both numbers are always stated: "3 of 3" says nothing is left out, and 3 comments or 30 is not
// the same PR. The suffix only when it fits whole and something is hidden.
func commentLegend(shown, total, outer int) string {
	legend := fmt.Sprintf("%d of %d", shown, total)
	room := commentBoxWidth(outer) - commentBoxBorder - legendGap
	if total > shown && utf8.RuneCountInString(legend+commentHint) <= room {
		legend += commentHint
	}
	return styleCount.Render(legend)
}

// The whole body, not its first line: with only the first, a three-paragraph comment took one of
// four rows and wasted three. Paragraphs are respected so wrapping cannot read as a run-on.

// What does not fit is marked with "…", because a row stopping mid-sentence reads as the comment ending
// there.
func commentBody(c model.Comment, lines, inner int) []string {
	// Nobody goes without a row because of an allocation mismatch: an empty row beats an out-of-range
	// index when marking the cut.
	lines = max(1, lines)
	author := truncate(c.Author, maxCommentAuthor)
	first, cont := commentWidths(inner, author)

	// The last piece and its width are kept apart so the cut can be marked without measuring a row
	// already dressed in styles, which would cut an ANSI code in half.
	var (
		out       []string
		lastPiece string
		lastW     int
	)
	for _, src := range parse.CommentLines(c.Body) {
		// Each paragraph wraps at the width of the row it gets, not at the wider of the two: wrapping wide
		// and clipping after would cut words.
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
		// Empty body or only boilerplate: said, so the row does not read as a comment that says nothing.
		return []string{commentRow(0, author, commentSep, "(no text)", first, false)}
	}
	return out
}

// In RUNES and over plain text: measuring a row already dressed in styles gives a column count
// that is not the box's. The author's name counts because it pushes the first row's body.
func commentWidths(inner int, author string) (first, cont int) {
	first = max(8, inner-utf8.RuneCountInString(commentIndent+author+commentSep))
	cont = max(8, inner-utf8.RuneCountInString(contIndent))
	return first, cont
}

// The continuation does NOT line up with the first row's text: the indent is a constant, and
// aligning to a long author would leave the name a one-column body.
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
	// One call replaces the old `n > w` guard: clipRunes returns the piece unchanged whenever
	// it fits (w >= len), so the guard was a no-op whose equality case no test could tell apart
	// from this line — a mutant of a comparison with no observable difference.
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

// They are NOT aligned with the first row's text: that one carries the author, whose width would
// let a long name eat the body, and the name is not what says anything.
const (
	commentIndent = "  "
	contIndent    = "    "
	commentSep    = ": "
)
