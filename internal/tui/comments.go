// The selected item's conversation, inside the detail panel.
// Comments are not a separate view nor something the user asks for: they are part of the card, which
// is why they are fetched when the cursor reaches the item. What this file avoids is the other extreme,
// fetching them with the inbox: N extra calls per cycle for data belonging to a single row.
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
// paint differently. With only the list, an unasked item and a commentless one are both an empty list, and
// the first pretends the PR has no conversation.
type commentState struct {
	list  []model.Comment
	total int  // los que dice el forge que hay, para poder decir "5 de 23"
	ready bool // la consulta terminó, salga bien o mal
	err   string
}

// Without it a long comment eats the whole panel on a tall terminal and the other four are never
// seen: five comments were asked for, not one.
const maxCommentLines = 4

func (m *Model) commentsCmd() tea.Cmd {
	return tea.Tick(commentsPoll, func(time.Time) tea.Msg { return commentsTickMsg{} })
}

// Does not touch the events channel: the goroutine publishes with sendEvent, which does not consume the
// reader the bomb has, so the single-reader invariant holds.
//
// The re-armed tick comes back either way, so the chain is never cut and the next selection change is
// noticed without having to remember to re-arm it at the change site.
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
		// A warning only becomes an error if nothing came back. If the forge sent comments and also slipped
		// something in, what we have is better than an empty card because of a warning that does not stop
		// the reading.
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
	if st.total < len(st.list) {
		// GitLab exposes no count, so the total is what was read. It is floored at the list's height so the
		// count arithmetic does not invent comments that are not there.
		st.total = len(st.list)
	}
	m.comments[msg.id] = st
}

// When there are comments they go in their own titled box rather than as more card fields: a
// conversation is not data about the PR but what people said about it, and a border says so without
// needing an explanation. The lone states (loading, error, none) stay as field lines because they are
// one-line messages, not conversation.
//
// The room is shared among the comments instead of going to the first, so all five are always visible
// and a tall terminal shows more than the first line of each.
func (m *Model) commentLines(it model.Item, avail, inner int) []string {
	if avail <= 0 {
		return nil
	}
	st := m.comments[it.ID()]
	switch {
	case st == nil:
		// Not asked and nothing in flight. The next tick might not even ask: without a session it does
		// not, and the header already says so. "Loading…" here would announce a query that does not exist.
		return nil
	case !st.ready:
		// In flight: said rather than left blank, because a blank is indistinguishable from "this PR has
		// no comments" and we do not know that yet.
		return []string{label("Comments", styleDim.Render("loading…"))}
	case st.err != "":
		// No floor on the width: `inner` comes from contentWidth, never below 38, and labelWidth is 14, so
		// the difference is never negative. A floor here would guard a truncate of an impossible negative
		// width and would hide that what decides is how much of the error fits.
		return []string{label("Comments", styleWarn.Render(truncate("not read: "+st.err, inner-labelWidth)))}
	case len(st.list) == 0:
		// No comments, no box. The box exists to separate the conversation from the fields, and a box around
		// the word "none" separates nothing — and it is the state of every commentless PR, so a border
		// appearing and disappearing with each cursor move is noise.
		return []string{label("Comments", styleDim.Render("none"))}
	}

	// Never painted half: a block with a top border and no bottom one is not a half box, it is noise taking
	// the same room as the whole block, so if both borders do not fit neither does the conversation.
	//
	// And the WHOLE conversation has to fit, not a part of it. The box costs two rows and one of them is a
	// border: on a 30-row terminal with three comments and a three-row budget, the block would fit three
	// comments with no box and one with the box. Seeing one and losing the other two is worse than seeing
	// none, because a clipped box does not look clipped: it looks like the PR only has that comment.
	if avail < commentChrome+len(st.list) {
		return nil
	}
	budget := avail - commentChrome

	// Bounded here and not only in the adapter. CommentLimit is the card's decision, and a card that
	// skipped it because it trusted whoever filled it would show comments that do not fit its own height.
	shown := st.list
	if len(shown) > forge.CommentLimit {
		shown = shown[:forge.CommentLimit]
	}
	// There is deliberately NO second floor on `len(shown)`. There used to be, and it was unreachable: the
	// floor above is `avail < commentChrome+len(st.list)` and this one would have been
	// `avail < commentChrome+len(shown)`, and since `len(shown) <= len(st.list)` by the cap of five the
	// first always holds whenever the second does. A guard the previous one already covers covers
	// nothing, and it reads as if the cap's clipping had a room check of its own.

	bodyWidth := commentBodyWidth(inner)

	// Each comment's needed rows are counted by composing it at the tall cap, which is the same code that
	// paints it, so the allocation cannot lie about what fits. Composing twice is cheap and beats
	// allocating blind.
	need := make([]int, len(shown))
	total := 0
	for i, c := range shown {
		need[i] = len(commentBody(c, maxCommentLines, bodyWidth))
		total += need[i]
	}

	// The count does not go in the body: it lives on the bottom border, on the right (see
	// commentLegend). Every body row is something a person wrote, and a count row belongs to nobody — and
	// on the border it costs no height, so it is not the first thing to go when the panel is tight.
	body := make([]string, 0, budget)

	// When they fit whole, each takes what it needs, which is what stops a six-paragraph comment sitting
	// next to four one-liners truncated to "the timeout is 30x too high…".
	// When they do not, everyone gets one row — all five present, which is what was asked — and the surplus
	// goes to whoever has most to lose, which beats handing the panel to the first.
	rows := need
	if total > budget {
		rows = allocate(need, budget)
	}

	for i, c := range shown {
		// There is no safety net here and there used to be. It was unreachable, and the count is short:
		// the guard above says `avail >= commentChrome+len(st.list)`, so `budget = avail - commentChrome` is
		// at least `len(st.list)`, and `len(shown)` is at most `len(st.list)` by the cap of five, so
		// `budget >= len(need)`.
		// So allocate hands out at most `min(budget, sum(need))`: it distributes `budget - len(need)`
		// extra rows and stops once everyone reaches their need. The sum never exceeds the budget, and
		// `max(1, rows[i])` cannot raise it, since allocate already leaves everyone at 1.
		// A net that cannot fire is worse than none: it makes the allocation look like it has a second
		// safeguard when what it has is arithmetic you have to read.
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
	// The legend does not sit on the corner: between it and the corner the border keeps a dash of its own.
	// Without it, a loose "3" with a gap on each side makes the bottom line read as broken rather than as
	// a border with something written inside it, which is exactly what is lost by writing on a border.
	//
	// The dash goes OUTSIDE the count's style: inside it would inherit its grey and the segment closing the
	// line would be a different colour from the segment that opens it. But outside the style is not unstyled:
	// the count's style ends with a reset, and a reset does not restore what was there, it takes the border's
	// grey with it. Without repainting, the dash and its space came out in the terminal's foreground colour,
	// which on many palettes is a yellowish white: a piece of border in another colour, right on the line
	// that closes the box.
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

// Its own function for the same reason as commentWidths: a body two columns wider does not give a
// wider box — the box clips either way — but two fewer characters per line, and no render test sees that.
// Being pure arithmetic it is checked directly, floor of 8 included.
func commentBodyWidth(outer int) int {
	return max(8, commentBoxWidth(outer)-commentBoxBorder)
}

// Without it the box spans the panel's full width and shares columns with its border: the verticals
// overlap and every row comes out as `||`. A lighter grey told the two borders apart, but lighter grey
// goes yellowish on warm palettes, and that was a colour problem covering a shape one: what is needed is
// for the two borders not to overlap, not for them to look different.
const commentInset = 1

func commentBoxWidth(outer int) int { return max(8, outer-2*commentInset) }

// One at a time rather than filling the neediest first, because that avoids the arbitrariness of order:
// "the first one keeps it" would let a six-paragraph comment at the top eat the panel while an equally
// long one at the bottom keeps its first phrase, which depends only on who wrote first. Levelling shares
// the damage among those who will suffer it, which is the only thing shareable without a better rule.
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
			break // nadie puede absorber más
		}
		rows[best]++
	}
	return rows
}

// It does not just say there is more conversation, it says where. Kept apart so that in a narrow panel
// it is what goes, not the numbers.
const commentHint = " · open the PR to read the rest"

const legendGap = 3

// Both numbers are always stated, even with the whole conversation visible. "3 of 3" informs as much
// as "5 of 23": it says nothing is left out, and the size of the conversation is part of the item's state —
// 3 comments or 30 is not the same PR. The phrasing that only appeared when there was more was justified
// by costing a row, and that cost disappeared when it moved to the border: staying silent about the
// count no longer buys anything.
//
// Only the suffix if it fits whole and only when something is hidden: cut in half ("5 of 23 · open the PR
// to read…") it says less than the short form, and with no hidden comments there is nothing to open.
func commentLegend(shown, total, outer int) string {
	legend := fmt.Sprintf("%d of %d", shown, total)
	room := commentBoxWidth(outer) - commentBoxBorder - legendGap
	if total > shown && utf8.RuneCountInString(legend+commentHint) <= room {
		legend += commentHint
	}
	return styleCount.Render(legend)
}

// The whole body and not just its first line: with the first line, a three-paragraph comment took one of
// the four rows it was given and wasted the other three, which is exactly the room the field grid freed so
// the comments would fit. Paragraphs are respected as they are (see parse.CommentLines), because wrapping
// them continuously produces "fix the timeout fix the backoff".

// What does not fit is marked with "…", because a row stopping mid-sentence reads as the comment ending
// there.
func commentBody(c model.Comment, lines, inner int) []string {
	// Nobody goes without a row because of an allocation mismatch: an empty row beats an out-of-range
	// index when marking the cut.
	lines = max(1, lines)
	author := truncate(c.Author, maxCommentAuthor)
	first, cont := commentWidths(inner, author)

	// The last written piece and its width are kept apart so it can be clipped when marking the cut
	// without measuring a row already dressed with styles: clipping the whole row would cut an ANSI code in
	// half.
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

// In RUNES and over plain text: the width to fill is the text's, and measuring a row already dressed
// with styles would give a column count that is not the box's. The author's name is inside because that
// is what pushes the first row's body, which is why it depends on `author` and not only on `inner`.
func commentWidths(inner int, author string) (first, cont int) {
	first = max(8, inner-utf8.RuneCountInString(commentIndent+author+commentSep))
	cont = max(8, inner-utf8.RuneCountInString(contIndent))
	return first, cont
}

// Note that the block below says the opposite: the continuation does NOT line up with the first row's
// text. The continuation indent is a constant (4 columns) and the first row's text starts at 2 + name + 2,
// so with a short author the body is staggered to the left. What makes it read as a block is that all
// the following rows share an indent, not that they line up with the first. Actually aligning them to the
// author would give a long name a one-column body, and the name is not what says anything.
//
// `cut` means there was more text behind. Clipping is not enough: if the last piece fit exactly it would
// carry no "…", and a row that looks finished says the comment ended there.
func commentRow(idx int, author, sep, piece string, w int, cut bool) string {
	indent, prefix := contIndent, ""
	if idx == 0 {
		indent, prefix = commentIndent, styleDetailKey.Render(author+sep)
	}
	runes := []rune(piece)
	switch n := utf8.RuneCountInString(piece); {
	case cut:
		room := max(1, w-1)
		if n <= room {
			return indent + prefix + piece + "…"
		}
		return indent + prefix + clipRunes(runes, room)
	case n > w:
		// A single word wider than the box: there is nowhere to break it, so it is clipped.
		return indent + prefix + clipRunes(runes, w)
	default:
		return indent + prefix + piece
	}
}

func clipRunes(runes []rune, n int) string {
	switch {
	case n >= len(runes):
		return string(runes)
	case n <= 0:
		return ""
	case n == 1:
		return "…"
	default:
		return string(runes[:n-1]) + "…"
	}
}

const maxCommentAuthor = 24

// Separates them from the card without needing a blank line, which in an 18-row panel is expensive,
// and the following rows share a FIXED indent so the body reads as a block.
//
// They are NOT aligned with the first row's text: that one carries the author and its width depends on
// how long the name is, so aligning to it would let a long name eat the body, and the name is not what
// says anything.
const (
	commentIndent = "  "
	contIndent    = "    "
	commentSep    = ": "
)
