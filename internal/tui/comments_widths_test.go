package tui

import (
	"strings"
	"testing"
	"unicode/utf8"

	"prdash/internal/forge/model"
)

// The first row carries the author and the rest only the continuation indent.
func TestCommentWidths(t *testing.T) {
	for _, inner := range []int{0, 1, 5, 10, 20, 30, 40, 60, 80, 120, 200} {
		for _, author := range []string{"", "a", "alice", "someone-with-a-long-name"} {
			first, cont := commentWidths(inner, author)

			if first > cont {
				t.Errorf("inner=%d author=%q: the first row (%d) is WIDER than the following ones (%d)",
					inner, author, first, cont)
			}
			// The floor of 8: below it the text is unreadable.
			if first < 8 || cont < 8 {
				t.Errorf("inner=%d author=%q: (%d, %d), want >= 8 on both: below that there is no reading",
					inner, author, first, cont)
			}
			wantFirst := max(8, inner-utf8.RuneCountInString(commentIndent+author+commentSep))
			wantCont := max(8, inner-utf8.RuneCountInString(contIndent))
			if first != wantFirst || cont != wantCont {
				t.Errorf("inner=%d author=%q: gave (%d, %d), want (%d, %d)",
					inner, author, first, cont, wantFirst, wantCont)
			}
			// The count is in RUNES: an author with accents or emoji takes one column per character.
			if author == "ñ" {
				if first != cont {
					t.Errorf("a one rune author takes one column, but the first row (%d) differs from the following ones (%d)",
						first, cont)
				}
			}
		}
	}
	// Two authors of the SAME length give the same width: what counts is the length, not the text.
	aF, _ := commentWidths(60, "alice")
	bF, _ := commentWidths(60, "bobby")
	if aF != bF {
		t.Errorf("authors of the same length gave different widths: %d vs %d", aF, bF)
	}
	// A longer author makes the first row NARROWER: the name eats the body's space.
	shortF, _ := commentWidths(60, "a")
	largoF, _ := commentWidths(60, strings.Repeat("x", 40))
	if largoF >= shortF {
		t.Errorf("a 40 character author gave a first row of %d and a one char one gave %d: the name should push",
			largoF, shortF)
	}
}

func TestCommentBodyWidthDiscountsIndentAndBorders(t *testing.T) {
	for outer := -20; outer <= 200; outer++ {
		got := commentBodyWidth(outer)
		want := max(8, commentBoxWidth(outer)-commentBoxBorder)
		if got != want {
			t.Errorf("commentBodyWidth(%d) = %d, want %d", outer, got, want)
		}
		if got < 8 {
			t.Errorf("commentBodyWidth(%d) = %d, want >= 8", outer, got)
		}
	}
	for outer := 40; outer <= 120; outer++ {
		want := outer - 2*commentInset - commentBoxBorder
		if got := commentBodyWidth(outer); got != want {
			t.Errorf("commentBodyWidth(%d) = %d, want %d", outer, got, want)
		}
	}
	if got := commentBodyWidth(10); got != 8 {
		t.Errorf("commentBodyWidth(10) = %d, want 8 (the floor)", got)
	}
	if got := commentBodyWidth(20); got != 20-2*commentInset-commentBoxBorder {
		t.Errorf("commentBodyWidth(20) = %d, want the exact discount: at 20 the floor is not reached yet", got)
	}
}

func TestCommentRowReservesTheGapForTheEllipsis(t *testing.T) {
	const w = 20
	// With clipping, a text of exactly (w-1) characters fits JUST with the mark.
	exact := strings.Repeat("x", w-1)
	got := stripANSI(commentRow(0, "alice", ": ", exact, w, true))
	if !strings.HasSuffix(got, exact+"…") {
		t.Errorf("a text of %d columns with a mark should enter whole plus the mark, gave %q", w-1, got)
	}
	if width := utf8.RuneCountInString(got) - utf8.RuneCountInString(commentIndent) -
		utf8.RuneCountInString("alice: "); width > w {
		t.Errorf("the row measures %d columns of text, more than the width %d: the mark left the box", width, w)
	}
	oneMore := strings.Repeat("x", w)
	got = stripANSI(commentRow(0, "alice", ": ", oneMore, w, true))
	if utf8.RuneCountInString(got) > utf8.RuneCountInString(commentIndent)+utf8.RuneCountInString("alice: ")+w {
		t.Errorf("a text of %d columns went past the width %d: %q", w, w, got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("a clipped text should carry the mark, gave %q", got)
	}
	// The clipping SATURATES: w columns and w+5 give the same row.
	twoMore := strings.Repeat("x", w+5)
	if a, b := stripANSI(commentRow(0, "alice", ": ", oneMore, w, true)),
		stripANSI(commentRow(0, "alice", ": ", twoMore, w, true)); a != b {
		t.Errorf("a text of %d and another of %d gave different rows %q and %q: both cut at the marks gap",
			w, w+5, a, b)
	}

	// Without clipping a text that fits comes whole and WITHOUT a mark.
	withoutCut := strings.Repeat("x", w-3)
	if got := stripANSI(commentRow(0, "alice", ": ", withoutCut, w, false)); strings.Contains(got, "…") {
		t.Errorf("a text that fits whole should not carry a mark: %q", got)
	}
	// The EDGE: a text of EXACTLY w columns goes in whole with no mark. A `>=` would add one.
	exactNoCut := strings.Repeat("x", w)
	got = stripANSI(commentRow(0, "alice", ": ", exactNoCut, w, false))
	if strings.Contains(got, "…") {
		t.Errorf("a text of exactly %d columns with no cut should enter whole, gave %q", w, got)
	}
	if n := utf8.RuneCountInString(got) - utf8.RuneCountInString(commentIndent) -
		utf8.RuneCountInString("alice: "); n != w {
		t.Errorf("a text of %d columns gave a row of %d: the exact row must not change length", w, n)
	}
	got = stripANSI(commentRow(0, "alice", ": ", strings.Repeat("x", w+1), w, false))
	if !strings.Contains(got, "…") {
		t.Errorf("a text of %d columns in a row of %d should be clipped, gave %q", w+1, w, got)
	}

	width := strings.Repeat("x", w+10)
	got = stripANSI(commentRow(0, "alice", ": ", width, w, false))
	if !strings.Contains(got, "…") {
		t.Errorf("a word too wide should be clipped with a mark, gave %q", got)
	}
	if utf8.RuneCountInString(got) > utf8.RuneCountInString(commentIndent)+utf8.RuneCountInString("alice: ")+w {
		t.Errorf("the clipped word went past the width: %q", got)
	}

	first := stripANSI(commentRow(0, "alice", ": ", "text", w, false))
	next := stripANSI(commentRow(1, "alice", ": ", "text", w, false))
	if !strings.HasPrefix(first, commentIndent+"alice: ") {
		t.Errorf("the first row should carry the author in front, gave %q", first)
	}
	if strings.Contains(next, "alice") {
		t.Errorf("the second row should not carry the author: %q", next)
	}
	// The continuation carries a FIXED indent whatever the author, which is what makes the body read
	//as a block.
	if !strings.HasPrefix(next, contIndent) {
		t.Errorf("the second row should go with the continuation indent, gave %q", next)
	}
	for _, autor := range []string{"a", "alice", "someone-with-a-long-name"} {
		first := stripANSI(commentRow(0, autor, ": ", "text", w, false))
		next := stripANSI(commentRow(1, autor, ": ", "text", w, false))
		colText := func(s string) int {
			return utf8.RuneCountInString(s[:strings.Index(s, "text")])
		}
		want := utf8.RuneCountInString(commentIndent + autor + commentSep)
		if colText(first) != want {
			t.Errorf("with the author %q the first rows text starts at column %d, want %d",
				autor, colText(first), want)
		}
		if colText(next) != utf8.RuneCountInString(contIndent) {
			t.Errorf("with the author %q the continuation starts at column %d, want %d (fixed, it does not follow the author)",
				autor, colText(next), utf8.RuneCountInString(contIndent))
		}
	}
}

// When the comment does not fit in the rows it gets.
func TestTheCutMarksTheLastRowAndAddsNone(t *testing.T) {
	const inner = 40
	first, _ := commentWidths(inner, "alice")

	palabra := strings.Repeat("x", first)
	for _, chunks := range []int{1, 2, 3, 4, 5, 6} {
		body := palabra
		for i := 1; i < chunks; i++ {
			body += " " + palabra
		}
		c := model.Comment{Author: "alice", Body: body}

		wantChunks := len(wrapText(body, first))
		if wantChunks != chunks {
			t.Fatalf("the body of %d words gives %d chunks, not %d: the width changed and the "+
				"test no longer measures what it thinks it measures", chunks, wantChunks, chunks)
		}

		all := commentBody(c, chunks+3, inner)
		if len(all) != chunks {
			t.Errorf("with %d chunks and an oversized budget %d rows came out, want %d", chunks, len(all), chunks)
		}
		for i, l := range all {
			if strings.Contains(stripANSI(l), "…") {
				t.Errorf("with a spare budget row %d carries a cut mark: %q", i, stripANSI(l))
			}
		}

		exact := commentBody(c, chunks, inner)
		if len(exact) != chunks {
			t.Errorf("with %d chunks and an exact budget %d rows came out, want %d", chunks, len(exact), chunks)
		}
		for i, l := range exact {
			if strings.Contains(stripANSI(l), "…") {
				t.Errorf("with an EXACT budget row %d carries a mark: nothing was lost, gave %q", i, stripANSI(l))
			}
		}

		// One piece less: one row less, the last one marked, and only the last.
		asks := max(1, chunks-1)
		short := commentBody(c, asks, inner)
		if len(short) != asks {
			t.Errorf("with %d chunks and a budget of %d %d rows came out, want %d: not one more than asked",
				chunks, asks, len(short), asks)
		}
		if len(short) == 0 {
			continue
		}
		// The mark only appears when something was really lost: with one chunk and enough budget there
		// is nothing to say.
		if asks < chunks {
			if last := stripANSI(short[len(short)-1]); !strings.Contains(last, "…") {
				t.Errorf("with %d chunks and a budget of %d the last row carries no mark: %q",
					chunks, asks, last)
			}
		}
		for i, l := range short[:len(short)-1] {
			if strings.Contains(stripANSI(l), "…") {
				t.Errorf("with a short budget row %d carries a mark: only the last one can be cut", i)
			}
		}
		for i, l := range short {
			if width := utf8.RuneCountInString(stripANSI(l)) -
				utf8.RuneCountInString(commentIndent) - utf8.RuneCountInString("alice: "); width > first && i == 0 {
				t.Errorf("the first row measures %d columns of text and the width is %d", width, first)
			}
		}
	}

	// A second paragraph: the cut has to land on the last chunk written.
	body := palabra + "\n\n" + palabra + " " + palabra + " " + palabra
	c := model.Comment{Author: "alice", Body: body}
	firstParagraph := len(wrapText(palabra, first))
	second := len(wrapText(strings.TrimSpace(strings.TrimPrefix(strings.SplitN(body, "\n\n", 2)[1], " ")), first))
	withTwo := commentBody(c, firstParagraph+second, inner)
	if len(withTwo) != firstParagraph+second {
		t.Errorf("with two paragraphs (%d+%d) %d rows came out, want %d",
			firstParagraph, second, len(withTwo), firstParagraph+second)
	}

	// And the empty body: one row saying so, instead of an empty list that would look broken.
	for _, empty := range []string{"", "   ", "\n\n"} {
		got := commentBody(model.Comment{Author: "alice", Body: empty}, 5, inner)
		if len(got) != 1 {
			t.Errorf("an empty body gave %d rows, want 1 (the one that says so)", len(got))
		} else if !strings.Contains(stripANSI(got[0]), "no text") {
			t.Errorf("an empty body gave %q, want the row that says so", stripANSI(got[0]))
		}
	}

	// With zero rows of budget there is one, not zero: nobody loses their row to a rounding.
	for _, n := range []int{-5, 0, 1} {
		if got := commentBody(model.Comment{Author: "alice", Body: "hola"}, n, inner); len(got) == 0 {
			t.Errorf("with budget %d no row came out: an empty row is better than an out of range index", n)
		}
	}
}
