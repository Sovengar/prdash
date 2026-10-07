package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// The width is in COLUMNS.
func TestWrapTextSplitsByWordsAndRespectsTheWidth(t *testing.T) {
	t.Run("it fits whole, it does not split", func(t *testing.T) {
		for _, max := range []int{11, 12, 100} {
			got := wrapText("hola mundo", max)
			if len(got) != 1 || got[0] != "hola mundo" {
				t.Errorf("max=%d gave %q, want one unchanged line", max, got)
			}
		}
	})

	t.Run("the width counts the space", func(t *testing.T) {
		if got := wrapText("ab cd", 5); len(got) != 1 {
			t.Errorf("max=5 gave %q, want one line (it measures exactly 5)", got)
		}
		if got := wrapText("ab cd", 4); len(got) != 2 {
			t.Errorf("max=4 gave %q, want two lines (5 columns do not fit in 4)", got)
		}
	})

	t.Run("nobody overflows the width when the words fit", func(t *testing.T) {
		// Every word measures three, the minimum the consumer guarantees.
		text := strings.Repeat("one two ", 12)
		for max := 3; max <= 40; max++ {
			for _, l := range wrapText(text, max) {
				if w := ansi.StringWidth(l); w > max {
					t.Errorf("max=%d: the line %q measures %d columns", max, l, w)
				}
			}
		}
	})

	t.Run("no width or no words", func(t *testing.T) {
		for _, max := range []int{0, -1, -100} {
			if got := wrapText("one two three", max); len(got) != 1 || got[0] != "one two three" {
				t.Errorf("max=%d gave %q, want the whole text in one line", max, got)
			}
		}
		got := wrapText("     ", 3)
		if len(got) != 1 {
			t.Errorf("only spaces gave %q, want one line", got)
		}
		if got := wrapText("", 10); len(got) != 1 {
			t.Errorf("empty gave %q, want one line (not an empty list)", got)
		}
	})

	t.Run("it neither loses nor invents words", func(t *testing.T) {
		text := "one two three cuatro cinco six siete ocho nueve diez"
		for max := 4; max <= 30; max++ {
			var out []string
			for _, l := range wrapText(text, max) {
				out = append(out, strings.Fields(l)...)
			}
			if strings.Join(out, " ") != text {
				t.Errorf("max=%d: the words are not preserved:\n%q", max, strings.Join(out, " "))
			}
		}
	})

	t.Run("a word longer than the width", func(t *testing.T) {
		got := wrapText("supercalifragilistic", 5)
		if len(got) != 1 || got[0] != "supercalifragilistic" {
			t.Errorf("long word gave %q, want one line with the whole word", got)
		}
		got = wrapText("ab supercalifragilistic", 5)
		if len(got) != 2 || got[0] != "ab" || got[1] != "supercalifragilistic" {
			t.Errorf("gave %q, want [ab supercalifragilistic]", got)
		}
	})
}

func TestToastExpiresByItsDuration(t *testing.T) {
	new := func() *toastManager {
		tm := &toastManager{now: func() time.Time { return time.Unix(1_000, 0) }}
		return tm
	}

	tm := new()
	tm.show("hola", toastInfo)
	if len(tm.texts()) != 1 {
		t.Fatalf("texts = %q, want 1 notice", tm.texts())
	}
	tm.update()
	if len(tm.texts()) != 1 {
		t.Errorf("a freshly created notice should not expire: %q", tm.texts())
	}

	tm = new()
	tm.show("hola", toastInfo)
	dur := tm.toasts[0].duration
	tm.now = func() time.Time { return time.Unix(1_000, 0).Add(dur) }
	tm.update()
	if len(tm.texts()) != 0 {
		t.Errorf("once the duration elapses the notice should expire, %q stays", tm.texts())
	}

	tm = new()
	tm.show("hola", toastInfo)
	dur = tm.toasts[0].duration
	tm.now = func() time.Time { return time.Unix(1_000, 0).Add(dur - time.Nanosecond) }
	tm.update()
	if len(tm.texts()) != 1 {
		t.Errorf("an instant before it elapses the notice should remain, %q stays", tm.texts())
	}

	// Each notice expires on ITS OWN duration: pruning with the first one's would drop the long one early.
	tm = &toastManager{now: func() time.Time { return time.Unix(1_000, 0) }}
	tm.toasts = []toast{
		{message: "short", created: time.Unix(1_000, 0), duration: time.Second},
		{message: "long", created: time.Unix(1_000, 0), duration: time.Hour},
		{message: "intermedio", created: time.Unix(1_000, 0), duration: time.Minute},
	}
	tm.now = func() time.Time { return time.Unix(1_000, 0).Add(2 * time.Minute) }
	tm.update()
	if got := strings.Join(tm.texts(), ","); got != "long" {
		t.Errorf("after 2 minutes %q stays, want only \"long\"", got)
	}

	tm = new()
	tm.update()
	if len(tm.texts()) != 0 {
		t.Errorf("with no notices, update left %q", tm.texts())
	}
	if tm.last() != "" {
		t.Errorf("last() with no notices = %q, want \"\"", tm.last())
	}
}

func TestToastLastAndBlocksWithSeveral(t *testing.T) {
	tm := &toastManager{now: func() time.Time { return time.Unix(1_000, 0) }}
	if got := tm.blocks(40); len(got) != 0 {
		t.Errorf("with no notices, blocks = %v, want empty", got)
	}

	tm.show("primero", toastInfo)
	tm.show("second", toastWarning)
	if got := tm.last(); got != "second" {
		t.Errorf("last() = %q, want the last one created", got)
	}
	if got := strings.Join(tm.texts(), ","); got != "primero,second" {
		t.Errorf("texts = %q, want the two in arrival order", got)
	}
	boxes := tm.blocks(40)
	if len(boxes) != 2 {
		t.Fatalf("boxes = %d, want 2", len(boxes))
	}
	for i, want := range []string{"primero", "second"} {
		if !strings.Contains(stripANSI(boxes[i]), want) {
			t.Errorf("box %d does not contain %q: %q", i, want, stripANSI(boxes[i]))
		}
	}
}
