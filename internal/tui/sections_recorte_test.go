package tui

import (
	"strings"
	"testing"
)

func TestWrapHintRecortaAMaxHintLinesYNadaMas(t *testing.T) {
	text := strings.Join(muchasLineas(), " ")

	for _, ancho := range []int{6, 8, 10, 12, 16} {
		plano := wrapText(text, ancho)
		if len(plano) <= maxHintLines {
			t.Fatalf("con ancho %d el texto solo da %d líneas, y el test necesita más de "+
				"%d para poder probar el recorte", ancho, len(plano), maxHintLines)
		}
		got := wrapHint(text, ancho, func(s string) string { return "[" + s + "]" })
		if len(got) != maxHintLines {
			t.Errorf("con ancho %d dio %d líneas, want %d: la caja de atajos está acotada "+
				"y no crece con el texto, porque es un footer",
				ancho, len(got), maxHintLines)
		}
		// What is kept is the HEAD of the text: the hint is read from the top.
		for i, l := range got {
			want := "[" + plano[i] + "]"
			if l != want {
				t.Errorf("con ancho %d la línea %d es %q, want %q: el recorte se lleva las "+
					"últimas, no las primeras", ancho, i, l, want)
			}
		}
	}
}

// The zero width is the default.
func TestWrapHintConAnchoDeUnaYConCero(t *testing.T) {
	for _, ancho := range []int{-10, 0, 1, 2} {
		got := wrapHint("un texto con unas cuantas palabras", ancho, func(s string) string { return s })
		if len(got) == 0 {
			t.Errorf("con ancho %d no devolvió ninguna línea, y el texto sí tiene palabras", ancho)
		}
		for i, l := range got {
			if l == "" {
				t.Errorf("con ancho %d la línea %d está vacía: partir un texto de verdad "+
					"nunca da líneas vacías", ancho, i)
			}
		}
	}
}

// m.height > 0 IS the show that decides.
func TestLaAlturaEsElTercerArgumentoDelLayout(t *testing.T) {
	for _, h := range []int{-40, -1, 0} {
		m := newTestModel(t)
		m.height = h
		lay := m.layout()
		if lay != (layout{}) {
			t.Errorf("con altura %d el layout dio %+v, want vacío: sin altura conocida no "+
				"se recorta nada", h, lay)
		}
	}

	m := newTestModel(t)
	m.height = 40
	if lay := m.layout(); lay.bodyLines < 1 {
		t.Errorf("con altura 40 dio cuerpo=%d, want al menos 1", lay.bodyLines)
	}
	// The budget passed is the hints ALREADY WRAPPED to the inner width.
	m2 := newTestModel(t)
	m2.height = 40
	wantHoras := len(m2.hintLines())
	if got := m2.layout(); got.hintLines > wantHoras {
		t.Errorf("el layout promete %d líneas de atajos y la caja solo tiene %d",
			got.hintLines, wantHoras)
	}
}

func muchasLineas() []string {
	out := make([]string, 0, 40)
	for range 40 {
		out = append(out, "atajo")
	}
	return out
}

// With the text giving EXACTLY maxHintLines the top edge is the identity.
func TestWrapHintElBordeDelTopeEsIdentidad(t *testing.T) {
	const ancho = 12
	var palabras []string
	for {
		palabras = append(palabras, "atajo")
		if len(wrapText(strings.Join(palabras, " "), ancho)) > maxHintLines {
			break
		}
	}
	palabras = palabras[:len(palabras)-1] // la última palabra es la que se pasa
	text := strings.Join(palabras, " ")
	plano := wrapText(text, ancho)
	if len(plano) != maxHintLines {
		t.Fatalf("construcción fallida: quería exactamente %d líneas y el texto da %d "+
			"(con %d palabras)", maxHintLines, len(plano), len(palabras))
	}

	got := wrapHint(text, ancho, func(s string) string { return "[" + s + "]" })
	if len(got) != maxHintLines {
		t.Errorf("con un texto de exactamente %d líneas dio %d: el recorte en el borde "+
			"tiene que ser una identidad", maxHintLines, len(got))
	}
	for i := range got {
		if want := "[" + plano[i] + "]"; got[i] != want {
			t.Errorf("línea %d: %q, want %q: en el borde no se toca el texto", i, got[i], want)
		}
	}
}

// With the budget EQUAL to the line count the identity holds.
func TestKeybindsElBordeDelPresupuestoEsIdentidad(t *testing.T) {
	m := newTestModel(t)
	m.mergeArmed = true
	var todos []string
	for ancho := 1; ancho <= 60 && len(todos) < 2; ancho++ {
		m.width = ancho
		todos = m.hintLines()
	}
	if len(todos) < 2 {
		t.Fatalf("con el merge armado y el terminal más estrecho la barra tiene %d líneas, "+
			"y el test necesita 2", len(todos))
	}

	sec := m.keybindsSection(len(todos))
	if sec.lines() != m.keybindsSection(len(todos)+1).lines() {
		t.Errorf("con presupuesto %d y con %d la caja mide %d y %d filas: en el borde, "+
			"pedir una línea de más no debe cambiar nada",
			len(todos), len(todos)+1, sec.lines(), m.keybindsSection(len(todos)+1).lines())
	}

	anterior := m.keybindsSection(len(todos)).lines()
	// The NEGATIVE case is what separates max(0, hintLines) from hintLines.
	negativo := m.keybindsSection(-1)
	if negativo.lines() != m.keybindsSection(0).lines() {
		t.Errorf("con presupuesto -1 la caja mide %d filas y con 0 mide %d: el suelo a "+
			"cero tiene que convertir el negativo en el mismo caso que el cero",
			negativo.lines(), m.keybindsSection(0).lines())
	}

	// Zero against one is what separates max(0, ...) from max(1, ...).

	for p := len(todos) - 1; p >= 0; p-- {
		actual := m.keybindsSection(p).lines()
		if actual > anterior {
			t.Errorf("con presupuesto %d la caja mide %d filas y con %d mide %d: "+
				"pedir MENOS no puede ocupar más",
				p, actual, p+1, anterior)
		}
		anterior = actual
	}
}
