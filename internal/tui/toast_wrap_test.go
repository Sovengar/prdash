package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// The width is in COLUMNS.
func TestWrapTextPartePorPalabrasYRespetaElAncho(t *testing.T) {
	t.Run("cabe entero, no se parte", func(t *testing.T) {
		for _, max := range []int{11, 12, 100} {
			got := wrapText("hola mundo", max)
			if len(got) != 1 || got[0] != "hola mundo" {
				t.Errorf("max=%d dio %q, want una línea sin cambios", max, got)
			}
		}
	})

	t.Run("el ancho cuenta el espacio", func(t *testing.T) {
		if got := wrapText("ab cd", 5); len(got) != 1 {
			t.Errorf("max=5 dio %q, want una línea (mide exactamente 5)", got)
		}
		if got := wrapText("ab cd", 4); len(got) != 2 {
			t.Errorf("max=4 dio %q, want dos líneas (5 columnas no caben en 4)", got)
		}
	})

	t.Run("nadie desborda el ancho cuando las palabras caben", func(t *testing.T) {
		// Every word measures three, the minimum the consumer guarantees.
		text := strings.Repeat("uno dos ", 12)
		for max := 3; max <= 40; max++ {
			for _, l := range wrapText(text, max) {
				if w := ansi.StringWidth(l); w > max {
					t.Errorf("max=%d: la línea %q mide %d columnas", max, l, w)
				}
			}
		}
	})

	t.Run("sin ancho o sin palabras", func(t *testing.T) {
		for _, max := range []int{0, -1, -100} {
			if got := wrapText("uno dos tres", max); len(got) != 1 || got[0] != "uno dos tres" {
				t.Errorf("max=%d dio %q, want el texto entero en una línea", max, got)
			}
		}
		got := wrapText("     ", 3)
		if len(got) != 1 {
			t.Errorf("solo espacios dio %q, want una línea", got)
		}
		if got := wrapText("", 10); len(got) != 1 {
			t.Errorf("vacío dio %q, want una línea (no una lista vacía)", got)
		}
	})

	t.Run("no pierde ni inventa palabras", func(t *testing.T) {
		text := "uno dos tres cuatro cinco seis siete ocho nueve diez"
		for max := 4; max <= 30; max++ {
			var out []string
			for _, l := range wrapText(text, max) {
				out = append(out, strings.Fields(l)...)
			}
			if strings.Join(out, " ") != text {
				t.Errorf("max=%d: las palabras no se conservan:\n%q", max, strings.Join(out, " "))
			}
		}
	})

	t.Run("una palabra más larga que el ancho", func(t *testing.T) {
		got := wrapText("supercalifragilistico", 5)
		if len(got) != 1 || got[0] != "supercalifragilistico" {
			t.Errorf("palabra larga dio %q, want una línea con la palabra entera", got)
		}
		got = wrapText("ab supercalifragilistico", 5)
		if len(got) != 2 || got[0] != "ab" || got[1] != "supercalifragilistico" {
			t.Errorf("dio %q, want [ab supercalifragilistico]", got)
		}
	})
}

// Each one expires after ITS duration.
func TestToastCaducaPorSuDuracion(t *testing.T) {
	nuevo := func() *toastManager {
		tm := &toastManager{now: func() time.Time { return time.Unix(1_000, 0) }}
		return tm
	}

	tm := nuevo()
	tm.show("hola", toastInfo)
	if len(tm.texts()) != 1 {
		t.Fatalf("texts = %q, want 1 aviso", tm.texts())
	}
	tm.update() // now == created, d = 0 < duration
	if len(tm.texts()) != 1 {
		t.Errorf("un aviso recién creado no debería caducar: %q", tm.texts())
	}

	tm = nuevo()
	tm.show("hola", toastInfo)
	dur := tm.toasts[0].duration
	tm.now = func() time.Time { return time.Unix(1_000, 0).Add(dur) }
	tm.update()
	if len(tm.texts()) != 0 {
		t.Errorf("al cumplirse la duración el aviso debería caducar, quedan %q", tm.texts())
	}

	tm = nuevo()
	tm.show("hola", toastInfo)
	dur = tm.toasts[0].duration
	tm.now = func() time.Time { return time.Unix(1_000, 0).Add(dur - time.Nanosecond) }
	tm.update()
	if len(tm.texts()) != 1 {
		t.Errorf("un instante antes de cumplirse el aviso debería seguir, quedan %q", tm.texts())
	}

	// Each notice expires on ITS OWN duration: pruning with the first one's would drop the long
	// one early.
	tm = &toastManager{now: func() time.Time { return time.Unix(1_000, 0) }}
	tm.toasts = []toast{
		{message: "corto", created: time.Unix(1_000, 0), duration: time.Second},
		{message: "largo", created: time.Unix(1_000, 0), duration: time.Hour},
		{message: "intermedio", created: time.Unix(1_000, 0), duration: time.Minute},
	}
	tm.now = func() time.Time { return time.Unix(1_000, 0).Add(2 * time.Minute) }
	tm.update()
	if got := strings.Join(tm.texts(), ","); got != "largo" {
		t.Errorf("tras 2 minutos quedan %q, want solo \"largo\"", got)
	}

	tm = nuevo()
	tm.update()
	if len(tm.texts()) != 0 {
		t.Errorf("sin avisos, update dejó %q", tm.texts())
	}
	if tm.last() != "" {
		t.Errorf("last() sin avisos = %q, want \"\"", tm.last())
	}
}

func TestToastLastYBlocksConVarios(t *testing.T) {
	tm := &toastManager{now: func() time.Time { return time.Unix(1_000, 0) }}
	if got := tm.blocks(40); len(got) != 0 {
		t.Errorf("sin avisos, blocks = %v, want vacío", got)
	}

	tm.show("primero", toastInfo)
	tm.show("segundo", toastWarning)
	if got := tm.last(); got != "segundo" {
		t.Errorf("last() = %q, want el último creado", got)
	}
	if got := strings.Join(tm.texts(), ","); got != "primero,segundo" {
		t.Errorf("texts = %q, want los dos en orden de llegada", got)
	}
	boxes := tm.blocks(40)
	if len(boxes) != 2 {
		t.Fatalf("boxes = %d, want 2", len(boxes))
	}
	for i, want := range []string{"primero", "segundo"} {
		if !strings.Contains(stripANSI(boxes[i]), want) {
			t.Errorf("la caja %d no contiene %q: %q", i, want, stripANSI(boxes[i]))
		}
	}
}
