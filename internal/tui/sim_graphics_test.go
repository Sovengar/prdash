package tui

import (
	"context"
	"errors"
	"image"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"prdash/internal/herdr"
	"prdash/internal/sim"
)

// fakeGraphics registra lo que se publica en la capa del pane.
type fakeGraphics struct {
	available bool
	cellW     int
	cellH     int
	err       error

	// cleared avisa de cada limpieza, para poder esperarla: quitarla es asíncrono
	// porque el popup no puede bloquearse esperando al socket.
	cleared chan string

	sets   []placement
	clears int
	layer  string
}

type placement struct {
	col, row, cols, rows int
	img                  image.Image
}

func (f *fakeGraphics) Available() bool { return f.available }

func (f *fakeGraphics) CellSize(context.Context) (int, int) {
	if f.cellW <= 0 || f.cellH <= 0 {
		return 1, 2
	}
	return f.cellW, f.cellH
}

func (f *fakeGraphics) SetImage(_ context.Context, layer string, img image.Image, p herdr.Placement) error {
	f.layer = layer
	if f.err != nil {
		return f.err
	}
	f.sets = append(f.sets, placement{col: p.Col, row: p.Row, cols: p.Cols, rows: p.Rows, img: img})
	return nil
}

func (f *fakeGraphics) Clear(_ context.Context, layer string) error {
	f.clears++
	f.layer = layer
	if f.cleared != nil {
		f.cleared <- layer
	}
	return nil
}

// waitClear espera a que se limpie la capa, con un techo para no colgarse si el
// popup se olvidó de hacerlo.
func waitClear(t *testing.T, f *fakeGraphics) {
	t.Helper()
	if f.cleared == nil {
		if f.clears == 0 {
			t.Fatal("cerrar el popup no pidió quitar la capa: la imagen se quedaría encima de la TUI")
		}
		return
	}
	select {
	case <-f.cleared:
	case <-time.After(2 * time.Second):
		t.Fatal("la capa no se quitó")
	}
}

// simModelWith es simModel con la capa de gráficos inyectada.
func simModelWith(t *testing.T, g Graphics) Model {
	t.Helper()
	m := simModel(t, &fakeSimulator{available: true})
	m.SetGraphics(g)
	return m
}

// TestTheImageGoesToThePaneLayer: con la capa disponible, la imagen no se pinta con
// celdas: se publica en el pane, que es lo que la dibuja a resolución nativa. El
// popup se queda con el marco.
func TestTheImageGoesToThePaneLayer(t *testing.T) {
	path := writeJPEG(t)
	f := &fakeSimulator{available: true, res: sim.Result{Kind: sim.KindMerge, Path: path}}
	g := &fakeGraphics{available: true, cellW: 9, cellH: 19}
	m := simModel(t, f)
	m.SetGraphics(g)

	m = press(t, m, "v")
	m = press(t, m, "enter")
	m = send(t, m, simMsg{seq: m.simSeq, kind: sim.KindMerge, res: f.res})

	if !m.sim.viaGraphics {
		t.Fatal("la imagen no se publicó en la capa del pane")
	}
	if len(g.sets) != 1 {
		t.Fatalf("SetImage se llamó %d veces, want 1", len(g.sets))
	}
	if len(m.sim.cells) != 0 {
		t.Errorf("la caja pintó %d líneas de celdas con la imagen en la capa", len(m.sim.cells))
	}
	set := g.sets[0]
	if set.cols <= 0 || set.rows <= 0 {
		t.Errorf("rectángulo vacío: %+v", set)
	}
	// La colocación tiene que caer dentro de la vista y dejar el marco: empieza
	// una columna y una línea más allá de la esquina de la caja, que es el borde.
	if set.col < 1 || set.row < 1 {
		t.Errorf("la imagen empieza en (%d,%d): se saldría del marco", set.col, set.row)
	}
	// Y la imagen llega ajustada a los píxeles del rectángulo, no en 1920 px.
	b := set.img.Bounds()
	const cellPx = 9 // el ancho de celda que reporta Herdr en kitty
	if want := set.cols * cellPx; b.Dx() > want {
		t.Errorf("imagen de %d px para un rectángulo de %d: se manda más resolución de la que se ve", b.Dx(), want)
	}
	if b.Dx() < set.cols*4 {
		t.Errorf("imagen de %d px, demasiado poca para %d columnas", b.Dx(), set.cols)
	}
	// El marco sigue en pantalla: sin el título, la imagen taparía la única
	// pista de qué se está viendo.
	if text := viewText(m); !strings.Contains(text, "simulate: merge") {
		t.Errorf("el popup perdió su título:\n%s", text)
	}
}

// TestWithoutTheLayerTheCellsTakeOver: fuera de Herdr, o con la capa apagada, el
// popup se dibuja con half-blocks como antes. Es el camino de degradación, y tiene
// que existir.
func TestWithoutTheLayerTheCellsTakeOver(t *testing.T) {
	path := writeJPEG(t)
	f := &fakeSimulator{available: true, res: sim.Result{Kind: sim.KindMerge, Path: path}}

	for name, g := range map[string]Graphics{
		"sin capa":       nil,
		"capa ausente":   &fakeGraphics{available: false},
		"capa que falla": &fakeGraphics{available: true, err: errors.New("pane graphics are disabled")},
	} {
		m := simModel(t, f)
		if g != nil {
			m.SetGraphics(g)
		}
		m = press(t, m, "v")
		m = press(t, m, "enter")
		m = send(t, m, simMsg{seq: m.simSeq, kind: sim.KindMerge, res: f.res})

		if m.sim.viaGraphics {
			t.Errorf("%s: se usó la capa sin poder", name)
		}
		if len(m.sim.cells) == 0 {
			t.Errorf("%s: no se pintó ninguna celda", name)
		}
	}
}

// TestTheRealCellRatioMakesTheImageBigger: con celdas de 9×19 en vez de 1×2, la
// imagen sale más grande. Un 5% no suena a nada, pero un 5% que se acumula con un
// tamaño supuesto es justo el error que hace que algo "casi cuadre".
func TestTheRealCellRatioMakesTheImageBigger(t *testing.T) {
	path := writeJPEG(t)
	f := &fakeSimulator{available: true, res: sim.Result{Kind: sim.KindMerge, Path: path}}

	assumed := simModel(t, f)
	assumed = press(t, assumed, "v")
	assumed = press(t, assumed, "enter")
	assumed = send(t, assumed, simMsg{seq: assumed.simSeq, kind: sim.KindMerge, res: f.res})
	assumedW, _ := assumed.simBox()

	measured := simModel(t, f)
	measured.SetGraphics(&fakeGraphics{available: true, cellW: 9, cellH: 19})
	measured = press(t, measured, "v")
	measured = press(t, measured, "enter")
	measured = send(t, measured, simMsg{seq: measured.simSeq, kind: sim.KindMerge, res: f.res})

	if measured.sim.cellW_px != 9 || measured.sim.cellH_px != 19 {
		t.Fatalf("no se guardó la celda medida: %dx%d", measured.sim.cellW_px, measured.sim.cellH_px)
	}
	if got, _ := measured.simBox(); got <= assumedW {
		t.Errorf("con la celda medida (9x19) la caja mide %d columnas, no más que las %d del supuesto 1x2",
			got, assumedW)
	}
}

// TestClosingThePopupClearsTheLayer: la capa vive por encima del contenido del
// pane. Si al cerrar el popup no se quita, la imagen se queda encima de la TUI y no
// hay forma de quitarla desde dentro.
func TestClosingThePopupClearsTheLayer(t *testing.T) {
	path := writeJPEG(t)
	f := &fakeSimulator{available: true, res: sim.Result{Kind: sim.KindMerge, Path: path}}
	g := &fakeGraphics{available: true, cellW: 9, cellH: 19, cleared: make(chan string, 4)}
	m := simModel(t, f)
	m.SetGraphics(g)

	m = press(t, m, "v")
	m = press(t, m, "enter")
	m = send(t, m, simMsg{seq: m.simSeq, kind: sim.KindMerge, res: f.res})
	if g.clears != 0 {
		t.Fatalf("se limpió la capa %d veces con el popup abierto", g.clears)
	}

	m = press(t, m, "esc")
	waitClear(t, g)
	if g.layer != simLayer {
		t.Errorf("se quitó la capa %q, want %q", g.layer, simLayer)
	}
}

// TestResizeRepositionsTheLayer: la colocación va en celdas, así que un resize
// cambia el rectángulo. Sin recolocar, la imagen se queda en el sitio viejo, que ya
// no es donde está el marco.
func TestResizeRepositionsTheLayer(t *testing.T) {
	path := writeJPEG(t)
	f := &fakeSimulator{available: true, res: sim.Result{Kind: sim.KindMerge, Path: path}}
	g := &fakeGraphics{available: true, cellW: 9, cellH: 19}
	m := simModel(t, f)
	m.SetGraphics(g)
	m = press(t, m, "v")
	m = press(t, m, "enter")
	m = send(t, m, simMsg{seq: m.simSeq, kind: sim.KindMerge, res: f.res})
	before := g.sets[len(g.sets)-1]

	m = send(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	if len(g.sets) < 2 {
		t.Fatalf("el resize no recolocó la imagen (%d publicaciones)", len(g.sets))
	}
	after := g.sets[len(g.sets)-1]
	if after == before {
		t.Error("la colocación no cambió con el resize: la imagen quedó en el rectángulo viejo")
	}
	if after.col > 100 || after.col+after.cols > 100 {
		t.Errorf("la imagen se sale de la terminal: %+v", after)
	}
}

// TestResizeFallsBackWhenTheLayerGoesAway: si en el resize la capa deja de
// responder, se vuelve a half-blocks en vez de dejar un rectángulo vacío.
func TestResizeFallsBackWhenTheLayerGoesAway(t *testing.T) {
	path := writeJPEG(t)
	f := &fakeSimulator{available: true, res: sim.Result{Kind: sim.KindMerge, Path: path}}
	g := &fakeGraphics{available: true, cellW: 9, cellH: 19}
	m := simModel(t, f)
	m.SetGraphics(g)
	m = press(t, m, "v")
	m = press(t, m, "enter")
	m = send(t, m, simMsg{seq: m.simSeq, kind: sim.KindMerge, res: f.res})

	g.err = errors.New("pane not found")
	m = send(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	if m.sim.viaGraphics {
		t.Error("siguió creyendo que la imagen está en la capa")
	}
	if len(m.sim.cells) == 0 {
		t.Error("no cayó a half-blocks: el popup quedó con un hueco donde iba la imagen")
	}
}

// TestSimulateDoesNotLeaveGraphicsOn: sin capa inyectada no hay nada que limpiar ni
// que romper. Es el caso de los tests y de una TUI sin Herdr.
func TestSimulateDoesNotLeaveGraphicsOn(t *testing.T) {
	path := writeJPEG(t)
	f := &fakeSimulator{available: true, res: sim.Result{Kind: sim.KindMerge, Path: path}}
	m := simModelWith(t, nil)
	m = press(t, m, "v")
	m = press(t, m, "enter")
	m = send(t, m, simMsg{seq: m.simSeq, kind: sim.KindMerge, res: f.res})
	m = press(t, m, "esc")

	if m.sim.state != simClosed {
		t.Errorf("state = %v, want cerrado", m.sim.state)
	}
}
