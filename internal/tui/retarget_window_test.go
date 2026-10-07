package tui

import (
	"testing"
	"time"
)

func TestBranchCacheFreshAtTheExactTTLStillServes(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

	for _, c := range []struct {
		name         string
		age          time.Duration
		wantsCurrent bool
	}{
		{"freshly made", 0, true},
		{"one second", time.Second, true},
		{"half of the TTL", branchCacheTTL / 2, true},
		{"a TTL minus one nanosecond", branchCacheTTL - time.Nanosecond, true},
		{"EXACTLY the TTL", branchCacheTTL, true},
		{"the TTL plus one nanosecond", branchCacheTTL + time.Nanosecond, false},
		{"double the TTL", 2 * branchCacheTTL, false},
		{"much older", time.Hour, false},
		{"in the future", -time.Hour, true},
	} {
		fetched := now.Add(-c.age)
		if got := branchCacheFresh(fetched, now); got != c.wantsCurrent {
			t.Errorf("%s: a listing from %v ago gave fresh=%v, want %v",
				c.name, c.age, got, c.wantsCurrent)
		}
	}
}

func TestRetargetWindowForFollowsTheCursor(t *testing.T) {
	const rows = 5

	for total := 0; total <= 22; total++ {
		for win := 0; win <= total+3; win++ {
			for cursor := 0; cursor <= total+3; cursor++ {
				got := retargetWindowFor(win, cursor, rows, total)

				if got < 0 {
					t.Fatalf("total=%d win=%d cursor=%d gave window %d: negative", total, win, cursor, got)
				}
				if maxWin := max(0, total-rows); got > maxWin {
					t.Fatalf("total=%d win=%d cursor=%d gave window %d, more than the last possible %d",
						total, win, cursor, got, maxWin)
				}

				if total <= rows {
					if got != 0 {
						t.Fatalf("total=%d win=%d cursor=%d gave window %d, want 0: "+
							"the list fits whole, there is nothing to scroll", total, win, cursor, got)
					}
					continue
				}

				if cursor < total {
					if cursor < got || cursor >= got+rows {
						t.Errorf("total=%d win=%d cursor=%d gave window %d, and the cursor "+
							"is outside: the selected row is not visible", total, win, cursor, got)
					}
				}
			}
		}
	}

	if got := retargetWindowFor(4, 4, rows, 20); got != 4 {
		t.Errorf("with the cursor on the last visible row it gave window %d, want 4: there is nothing to move", got)
	}
	if got := retargetWindowFor(4, 5, rows, 20); got != 4 {
		t.Errorf("with the cursor one row inside the window it gave %d, want 4: it stays visible", got)
	}
	if got := retargetWindowFor(4, 9, rows, 20); got != 5 {
		t.Errorf("with the cursor on the first row outside it gave window %d, want 5: it drops exactly", got)
	}
	if got := retargetWindowFor(4, 11, rows, 20); got != 7 {
		t.Errorf("with the cursor two rows outside it gave window %d, want 7", got)
	}
	// Zero rows is degenerate but defined, and it is what tells the `cursor <= win` boundary mutant from the real condition.
	if got := retargetWindowFor(4, 4, 0, 20); got != 5 {
		t.Errorf("with zero rows and cursor on win it gave window %d, want 5", got)
	}
	if got := retargetWindowFor(0, 19, rows, 20); got != 15 {
		t.Errorf("with the cursor on the last branch it gave window %d, want 15", got)
	}
	if got := retargetWindowFor(4, 2, rows, 20); got != 2 {
		t.Errorf("with the cursor above the window it gave %d, want 2: the window rises to the cursor", got)
	}
	if got := retargetWindowFor(500, 3, rows, 20); got != 3 {
		t.Errorf("with an impossible window it gave %d, want 3 (the cursors one)", got)
	}
}

func TestRetargetVisibleFromClipsOnBothSides(t *testing.T) {
	view := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}

	// The range that really arrives: win comes from retargetWindow, floored at 0.
	for _, win := range []int{0, 3, 9, 10, 11, 100} {
		for _, rows := range []int{0, 1, 3, 10, 20} {
			visibles, start := retargetVisibleFrom(view, win, rows)

			for i, v := range visibles {
				if i >= len(view) {
					t.Fatalf("win=%d rows=%d painted %d branches of the %d that exist", win, rows, len(visibles), len(view))
				}
				if view[start+i] != v {
					t.Fatalf("win=%d rows=%d row %d is %q, want %q: the window is not where it says",
						win, rows, i, v, view[start+i])
				}
			}
			if len(visibles) > len(view) {
				t.Errorf("win=%d rows=%d painted %d branches of the %d that exist", win, rows, len(visibles), len(view))
			}
			if len(visibles) > rows {
				t.Errorf("win=%d rows=%d painted %d branches, more than the %d that fit", win, rows, len(visibles), rows)
			}
			if start < 0 || start > len(view) {
				t.Errorf("win=%d rows=%d gave start=%d, outside the list of %d", win, rows, start, len(view))
			}
		}
	}

	vis, start := retargetVisibleFrom(view, 0, 3)
	if start != 0 || len(vis) != 3 || vis[0] != "a" || vis[2] != "c" {
		t.Errorf("with window 0 and 3 rows it gave %q from %d, want [a b c] from 0", vis, start)
	}
	vis, start = retargetVisibleFrom(view, 3, 3)
	if start != 3 || len(vis) != 3 || vis[0] != "d" {
		t.Errorf("with window 3 it gave %q from %d, want [d e f] from 3", vis, start)
	}
	vis, start = retargetVisibleFrom(view, 20, 3)
	if start != len(view) || len(vis) != 0 {
		t.Errorf("with an impossible window it gave %q from %d, want empty from %d", vis, start, len(view))
	}
	vis, start = retargetVisibleFrom(view, 0, 100)
	if start != 0 || len(vis) != len(view) {
		t.Errorf("with 100 rows it gave %d branches from %d, want the %d of the list", len(vis), start, len(view))
	}
	vis, start = retargetVisibleFrom(view, 0, 0)
	if start != 0 || len(vis) != 0 {
		t.Errorf("with zero rows it gave %q from %d, want empty from 0", vis, start)
	}

	vis, start = retargetVisibleFrom(nil, 0, 5)
	if start != 0 || len(vis) != 0 {
		t.Errorf("with an empty list it gave %q from %d, want empty from 0", vis, start)
	}
}
