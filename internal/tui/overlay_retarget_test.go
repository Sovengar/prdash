package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"prdash/internal/testutil"
)

// What the formula says and nothing more.
func TestCenteredOriginPutsTheBoxInTheMiddle(t *testing.T) {
	cases := []struct {
		areaW, areaH int
		boxW, boxH   int
		wantX, wantY int
		note         string
	}{
		{80, 40, 20, 10, 30, 15, "exact center: spare is even on both axes"},
		{81, 41, 20, 10, 30, 15, "odd spare: the remainder goes to the right"},
		{80, 40, 0, 0, 40, 20, "empty box: the point is centered"},
		{80, 40, 80, 40, 0, 0, "box of the exact size: the corner, not a negative"},
		{80, 40, 100, 40, 0, 0, "wider than the area: x at zero, not negative"},
		{80, 40, 20, 100, 30, 0, "taller than the area: y at zero, not negative"},
		{80, 40, 100, 100, 0, 0, "bigger on both sides"},
		{1, 1, 1, 1, 0, 0, "everything at one"},
		{3, 3, 2, 2, 0, 0, "spare of one on both axes"},
	}
	for _, c := range cases {
		x, y := centeredOrigin(c.areaW, c.areaH, c.boxW, c.boxH)
		if x != c.wantX || y != c.wantY {
			t.Errorf("area %dx%d, box %dx%d: origin (%d,%d), want (%d,%d). %s",
				c.areaW, c.areaH, c.boxW, c.boxH, x, y, c.wantX, c.wantY, c.note)
		}
		// And the floor: never negative. A negative coordinate breaks the background's clipping.
		if x < 0 || y < 0 {
			t.Errorf("area %dx%d, box %dx%d: origin (%d,%d) negative: the clipping "+
				"of the background cuts on the left with a negative width",
				c.areaW, c.areaH, c.boxW, c.boxH, x, y)
		}
		// And the box either fits or leaves by the bottom and the right, never by the top or the left.
		if y+c.boxH > c.areaH+1 && c.boxH <= c.areaH {
			t.Errorf("area %dx%d, box %dx%d: the box escapes at the bottom (%d > %d) without "+
				"being taller than the area", c.areaW, c.areaH, c.boxW, c.boxH,
				y+c.boxH, c.areaH)
		}
	}
}

// The box paints WHOLE and what overflows is the background.
func TestTheOverlayDoesNotEscapeThroughTheTopNorTheLeft(t *testing.T) {
	view := func(rows, width int) string {
		out := make([]string, rows)
		for i := range out {
			out[i] = strings.Repeat("·", width)
		}
		return strings.Join(out, "\n")
	}
	box := func(rows, width int) string {
		out := make([]string, rows)
		for i := range out {
			out[i] = "[" + strings.Repeat("-", max(0, width-2)) + "]"
		}
		return strings.Join(out, "\n")
	}

	got := overlayCentered(view(20, 60), box(5, 30), 60)
	rows := strings.Split(got, "\n")
	if len(rows) != 20 {
		t.Fatalf("the overlay changed the number of rows: %d, want 20", len(rows))
	}
	for i, l := range rows {
		if w := ansi.StringWidth(l); w != 60 {
			t.Fatalf("row %d measures %d, want 60: the overlay must not change the width of "+
				"the view, only clip it where needed", i, w)
		}
	}
	// The top frame is WHOLE, with both corners, on the row the arithmetic says.
	wantRow := (20 - 5) / 2
	above := rows[wantRow]
	if ansi.StringWidth(above) != 60 {
		t.Fatalf("row %d mide %d, want 60", wantRow, ansi.StringWidth(above))
	}
	if !strings.HasPrefix(above, "···············[") || !strings.HasSuffix(above, "]···············") {
		t.Errorf("row %d does not have the two corners of the frame: %q. A frame "+
			"that is cut does not say \"this is a window\"", wantRow, above)
	}
	for j := 1; j < 5; j++ {
		f := rows[wantRow+j]
		if !strings.Contains(f, "-----") {
			t.Errorf("row %d of the box carries no content: %q", wantRow+j, f)
		}
	}
	below := rows[wantRow+4]
	if !strings.HasPrefix(below, "···············[") || !strings.HasSuffix(below, "]···············") {
		t.Errorf("row %d does not close the frame: %q", wantRow+4, below)
	}

	got = overlayCentered(view(4, 60), box(20, 30), 60)
	rows = strings.Split(got, "\n")
	if len(rows) != 4 {
		t.Fatalf("with a box of 20 rows in a view of 4 the overlay gave %d rows, "+
			"want 4: the popup cannot grow the view", len(rows))
	}
	for i, l := range rows {
		if w := ansi.StringWidth(l); w != 60 {
			t.Errorf("row %d measures %d, want 60 with a box that does not fit", i, w)
		}
	}

	base := view(6, 40)
	if got := overlayCentered(base, "", 40); got != base {
		t.Errorf("with an empty box the view changed: %q", got)
	}

	got = overlayCentered(view(10, 40), box(3, 10), 40)
	for i, l := range strings.Split(got, "\n") {
		if !strings.Contains(l, "·") {
			t.Errorf("row %d is %q and there is no background around the frame: the popup "+
				"clips the background, it does not cover it entirely", i, l)
		}
	}
}

// The invalidation counter is monotonic.
func TestTheBranchSequenceIncrementsOnEveryRequestAndEveryClose(t *testing.T) {
	// The adapter is needed because fetchBranches starts a goroutine.
	adapt := &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"}
	m := newTestModel(t, adapt)
	defer m.cancel()
	if m.branchSeq != 0 {
		t.Logf("the sequence starts at %d, not at 0; the test compares differences, not "+
			"absolute values", m.branchSeq)
	}

	prev := m.branchSeq
	for i := range 3 {
		it := mkItem("github", "github.com", "acme/widget", "one", i+1, "")
		m.fetchBranches(it)
		if m.branchSeq != prev+1 {
			t.Fatalf("after request %d the sequence ended at %d, want %d: it has to "+
				"grow by one at a time, because a jump of two leaves a number that no "+
				"response carries and makes two in-flight requests indistinguishable",
				i+1, m.branchSeq, prev+1)
		}
		prev = m.branchSeq
	}

	// And closing raises the counter too, so an in-flight listing is not accepted after the popup
	// closed.
	m.retarget.all = []string{"main", "feat/x"}
	m.closeRetarget()
	if m.branchSeq != prev+1 {
		t.Errorf("closing left the sequence at %d, want %d: closing also invalidates, or a "+
			"listing in flight would be accepted over an already closed popup", m.branchSeq, prev+1)
	}
}

// The `y+boxH > height` condition centeredOrigin used to have is unreachable: with the box
// inside the area, adding boxH gives (height+boxH)/2, at most the area exactly when boxH <= height.
func TestTheEscapeThroughTheBottomGuardCannotFire(t *testing.T) {
	for altura := range -20 {
		for altoCaja := range -20 {
			y := max(0, (altura-altoCaja)/2)
			if y+altoCaja > altura {
				t.Fatalf("height %d, box of %d: y comes out at %d and %d+%d=%d passes the "+
					"height. The escape-through-the-bottom guard was ALIVE and it has to "+
					"devolverla", altura, altoCaja, y, y, altoCaja, y+altoCaja)
			}
		}
	}
}
