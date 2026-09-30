package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// TestWrapTextPartePorPalabrasYRespetaElAncho: el ancho es en COLUMNAS, y el
// criterio es que una palabra más no quepa en la línea actual. El `+1` es el
// espacio de separación, y quitarlo de la cuenta deja pasar una palabra que
// desborda la caja, que es exactamente lo que un aviso no puede hacer.
//
// Un texto que ya cabe no se parte (una sola línea, idéntica), y con ancho cero o
// negativo tampoco: sin ancho no hay nada que ajustar.
func TestWrapTextPartePorPalabrasYRespetaElAncho(t *testing.T) {
	t.Run("cabe entero, no se parte", func(t *testing.T) {
		// "hola mundo" mide 11 columnas, así que con 11 o más cabe entero.
		for _, max := range []int{11, 12, 100} {
			got := wrapText("hola mundo", max)
			if len(got) != 1 || got[0] != "hola mundo" {
				t.Errorf("max=%d dio %q, want una línea sin cambios", max, got)
			}
		}
	})

	t.Run("el ancho cuenta el espacio", func(t *testing.T) {
		// "ab" + " " + "cd" mide 5. Con max 5 cabe justo; con max 4 no.
		if got := wrapText("ab cd", 5); len(got) != 1 {
			t.Errorf("max=5 dio %q, want una línea (mide exactamente 5)", got)
		}
		if got := wrapText("ab cd", 4); len(got) != 2 {
			t.Errorf("max=4 dio %q, want dos líneas (5 columnas no caben en 4)", got)
		}
	})

	t.Run("nadie desborda el ancho cuando las palabras caben", func(t *testing.T) {
		// Todas las palabras miden 3, que es el mínimo que garantiza el
		// consumidor. Por debajo de eso wrapText deja la palabra entera en su línea
		// —partir por letras no es lo suyo— y quien pinta recorta; ese caso se
		// afirma en el subtest de la palabra larga.
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
		// El ancho cero o negativo no parte: devuelve el texto entero, que es
		// mejor que un bucle infinito o un panic. Con varias palabras, porque con
		// una sola el resultado sería el mismo por los dos caminos y la prueba no
		// distinguiría nada.
		for _, max := range []int{0, -1, -100} {
			if got := wrapText("uno dos tres", max); len(got) != 1 || got[0] != "uno dos tres" {
				t.Errorf("max=%d dio %q, want el texto entero en una línea", max, got)
			}
		}
		// Solo espacios: no hay palabras que partir, así que se devuelve lo que
		// hay en vez de una lista vacía (que pintaría una caja sin texto).
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
		// Una palabra sola más ancha que la caja: no se puede partir por palabras,
		// así que la deja en su propia línea, que es lo que puede pintar el
		// recorte posterior. Lo que no puede hacer es perderla.
		got := wrapText("supercalifragilistico", 5)
		if len(got) != 1 || got[0] != "supercalifragilistico" {
			t.Errorf("palabra larga dio %q, want una línea con la palabra entera", got)
		}
		// Y con ella al lado, la larga se queda sola y la corta forma su línea.
		got = wrapText("ab supercalifragilistico", 5)
		if len(got) != 2 || got[0] != "ab" || got[1] != "supercalifragilistico" {
			t.Errorf("dio %q, want [ab supercalifragilistico]", got)
		}
	})
}

// TestToastCaducaPorSuDuracion: un aviso se va cuando pasa SU duración, y cada
// uno la suya. Un aviso que no caduca tapa la vista para siempre; uno que caduca
// antes de tiempo se pierde el mensaje que el usuario aún no ha leído.
//
// El borde es `< duration` (estrictamente): al cumplir la duración EXACTAMENTE el
// aviso caduca, no se queda un instante de más.
func TestToastCaducaPorSuDuracion(t *testing.T) {
	nuevo := func() *toastManager {
		tm := &toastManager{now: func() time.Time { return time.Unix(1_000, 0) }}
		return tm
	}

	// Un aviso con la duración por defecto sigue vivo antes de cumplirse.
	tm := nuevo()
	tm.show("hola", toastInfo)
	if len(tm.texts()) != 1 {
		t.Fatalf("texts = %q, want 1 aviso", tm.texts())
	}
	tm.update() // now == created, d = 0 < duration
	if len(tm.texts()) != 1 {
		t.Errorf("un aviso recién creado no debería caducar: %q", tm.texts())
	}

	// Justo al cumplirse la duración, caduca.
	tm = nuevo()
	tm.show("hola", toastInfo)
	dur := tm.toasts[0].duration
	tm.now = func() time.Time { return time.Unix(1_000, 0).Add(dur) }
	tm.update()
	if len(tm.texts()) != 0 {
		t.Errorf("al cumplirse la duración el aviso debería caducar, quedan %q", tm.texts())
	}

	// Un instante antes, sigue.
	tm = nuevo()
	tm.show("hola", toastInfo)
	dur = tm.toasts[0].duration
	tm.now = func() time.Time { return time.Unix(1_000, 0).Add(dur - time.Nanosecond) }
	tm.update()
	if len(tm.texts()) != 1 {
		t.Errorf("un instante antes de cumplirse el aviso debería seguir, quedan %q", tm.texts())
	}

	// Y cada aviso caduca por su cuenta: uno corto se va mientras uno largo
	// sigue. Si la poda usara la duración del primero para todos, el largo
	// desaparecería con él y el mensaje se perdería sin llegar a leerse.
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

	// Sin avisos, update no inventa ninguno.
	tm = nuevo()
	tm.update()
	if len(tm.texts()) != 0 {
		t.Errorf("sin avisos, update dejó %q", tm.texts())
	}
	if tm.last() != "" {
		t.Errorf("last() sin avisos = %q, want \"\"", tm.last())
	}
}

// TestToastLastYBlocksConVarios: last() y blocks() tienen que ser coherentes
// entre sí, porque la TUI usa las dos: blocks() pinta las cajas y last() decide
// cuál se ha de repetir en un mensaje nuevo.
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
	// Una caja por aviso, cada una con su mensaje.
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
