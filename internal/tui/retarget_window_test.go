package tui

import (
	"testing"
	"time"
)

func TestBranchCacheFreshElTTLJustoSigueSirviendo(t *testing.T) {
	ahora := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

	for _, c := range []struct {
		nombre        string
		edad          time.Duration
		quiereVigente bool
	}{
		{"recién hecho", 0, true},
		{"un segundo", time.Second, true},
		{"la mitad del TTL", branchCacheTTL / 2, true},
		{"un TTL menos un nanosegundo", branchCacheTTL - time.Nanosecond, true},
		{"EXACTAMENTE el TTL", branchCacheTTL, true},
		{"el TTL más un nanosegundo", branchCacheTTL + time.Nanosecond, false},
		{"el doble del TTL", 2 * branchCacheTTL, false},
		{"mucho más viejo", time.Hour, false},
		{"en el futuro", -time.Hour, true},
	} {
		fetched := ahora.Add(-c.edad)
		if got := branchCacheFresh(fetched, ahora); got != c.quiereVigente {
			t.Errorf("%s: un listado de hace %v dio vigente=%v, want %v",
				c.nombre, c.edad, got, c.quiereVigente)
		}
	}
}

// The window moves just enough to follow the cursor.
func TestRetargetWindowForSigueAlCursor(t *testing.T) {
	const rows = 5

	for total := 0; total <= 22; total++ {
		for win := 0; win <= total+3; win++ {
			for cursor := 0; cursor <= total+3; cursor++ {
				got := retargetWindowFor(win, cursor, rows, total)

				if got < 0 {
					t.Fatalf("total=%d win=%d cursor=%d dio ventana %d: negativa", total, win, cursor, got)
				}
				if maxWin := max(0, total-rows); got > maxWin {
					t.Fatalf("total=%d win=%d cursor=%d dio ventana %d, más que la última posible %d",
						total, win, cursor, got, maxWin)
				}

				if total <= rows {
					if got != 0 {
						t.Fatalf("total=%d win=%d cursor=%d dio ventana %d, want 0: "+
							"la lista cabe entera, no hay nada que desplazar", total, win, cursor, got)
					}
					continue
				}

				if cursor < total {
					if cursor < got || cursor >= got+rows {
						t.Errorf("total=%d win=%d cursor=%d dio ventana %d, y el cursor "+
							"queda fuera: no se ve la fila seleccionada", total, win, cursor, got)
					}
				}
			}
		}
	}

	// The cases that say the most, one by one.
	// The cursor exactly on the last visible row: the window does not move.
	if got := retargetWindowFor(4, 4, rows, 20); got != 4 {
		t.Errorf("con el cursor en la última fila visible dio ventana %d, want 4: no hay nada que mover", got)
	}
	if got := retargetWindowFor(4, 5, rows, 20); got != 4 {
		t.Errorf("con el cursor una fila dentro de la ventana dio %d, want 4: sigue viendose", got)
	}
	// The real edge, the cursor on the first row OUTSIDE the window.
	if got := retargetWindowFor(4, 9, rows, 20); got != 5 {
		t.Errorf("con el cursor en la primera fila fuera dio ventana %d, want 5: baja lo justo", got)
	}
	// Two rows out and the window drops two, not all the way to the cursor.
	if got := retargetWindowFor(4, 11, rows, 20); got != 7 {
		t.Errorf("con el cursor dos filas fuera dio ventana %d, want 7", got)
	}
	if got := retargetWindowFor(0, 19, rows, 20); got != 15 {
		t.Errorf("con el cursor en la última rama dio ventana %d, want 15", got)
	}
	if got := retargetWindowFor(4, 2, rows, 20); got != 2 {
		t.Errorf("con el cursor por encima de la ventana dio %d, want 2: la ventana sube al cursor", got)
	}
	if got := retargetWindowFor(500, 3, rows, 20); got != 3 {
		t.Errorf("con una ventana imposible dio %d, want 3 (la del cursor)", got)
	}
}

// The slice to the window is floored on both sides.
func TestRetargetVisibleFromRecortaPorLosDosLados(t *testing.T) {
	view := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}

	// The range that really arrives: win comes from retargetWindow, floored at 0.
	for _, win := range []int{0, 3, 9, 10, 11, 100} {
		for _, rows := range []int{0, 1, 3, 10, 20} {
			visibles, start := retargetVisibleFrom(view, win, rows)

			for i, v := range visibles {
				if i >= len(view) {
					t.Fatalf("win=%d rows=%d pintó %d ramas de las %d que hay", win, rows, len(visibles), len(view))
				}
				if view[start+i] != v {
					t.Fatalf("win=%d rows=%d la fila %d es %q, want %q: la ventana no está donde dice",
						win, rows, i, v, view[start+i])
				}
			}
			if len(visibles) > len(view) {
				t.Errorf("win=%d rows=%d pintó %d ramas de las %d que hay", win, rows, len(visibles), len(view))
			}
			if len(visibles) > rows {
				t.Errorf("win=%d rows=%d pintó %d ramas, más de las %d que caben", win, rows, len(visibles), rows)
			}
			if start < 0 || start > len(view) {
				t.Errorf("win=%d rows=%d dio start=%d, fuera de la lista de %d", win, rows, start, len(view))
			}
		}
	}

	vis, start := retargetVisibleFrom(view, 0, 3)
	if start != 0 || len(vis) != 3 || vis[0] != "a" || vis[2] != "c" {
		t.Errorf("con ventana 0 y 3 filas dio %q desde %d, want [a b c] desde 0", vis, start)
	}
	vis, start = retargetVisibleFrom(view, 3, 3)
	if start != 3 || len(vis) != 3 || vis[0] != "d" {
		t.Errorf("con ventana 3 dio %q desde %d, want [d e f] desde 3", vis, start)
	}
	vis, start = retargetVisibleFrom(view, 20, 3)
	if start != len(view) || len(vis) != 0 {
		t.Errorf("con una ventana imposible dio %q desde %d, want vacío desde %d", vis, start, len(view))
	}
	vis, start = retargetVisibleFrom(view, 0, 100)
	if start != 0 || len(vis) != len(view) {
		t.Errorf("con 100 filas dio %d ramas desde %d, want las %d de la lista", len(vis), start, len(view))
	}
	vis, start = retargetVisibleFrom(view, 0, 0)
	if start != 0 || len(vis) != 0 {
		t.Errorf("con cero filas dio %q desde %d, want vacío desde 0", vis, start)
	}

	vis, start = retargetVisibleFrom(nil, 0, 5)
	if start != 0 || len(vis) != 0 {
		t.Errorf("con una lista vacía dio %q desde %d, want vacío desde 0", vis, start)
	}
}
