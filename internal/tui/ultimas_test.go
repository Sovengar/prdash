package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"image"
	"image/color"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/herdr"
	"prdash/internal/sim"
	"prdash/internal/testutil"
)

// A popup that cannot fit is not painted, and that is not an error: painting half would show a frame
// with nothing in it.
func TestUnPopupDeSimulacionQueNoCabeNoSePintaYNoEsUnError(t *testing.T) {
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	g := &graphicsContador{}
	m.graphics = g
	m.sim.state = simShowing

	// A terminal BELOW the popup's minimum. simMaxCols and simMaxRows have floors
	//(simMinCols/simMinRows), so what is really shown here is that the floor wins over the terminal.
	m.width = simMinCols - 1
	m.height = simMinRows - 1
	cols, _ := m.simBox()
	if cols < simMinCols {
		t.Errorf("con una terminal de %d columnas la caja del popup mide %d, por debajo del "+
			"suelo de %d: un popup más estrecho que el mínimo no se lee",
			m.width, cols, simMinCols)
	}
	if cols-2 <= 0 {
		t.Errorf("con el suelo aplicado el hueco interior es de %d columnas", cols-2)
	}

	m2 := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	g2 := &graphicsContador{}
	m2.graphics = g2
	m2.width, m2.height = 120, 40
	if !m2.publishSimImage(imagenDiminuta(8, 8)) {
		t.Fatal("con hueco de sobra no se publicó: la guarda no es lo que lo paró")
	}
	if g2.publicadas != 1 {
		t.Errorf("se publicaron %d imágenes, want 1", g2.publicadas)
	}
	if g2.ultima.Cols <= 0 || g2.ultima.Rows <= 0 {
		t.Errorf("el rectángulo publicado no tiene tamaño: %+v", g2.ultima)
	}
	if g2.ultima.Col <= 0 || g2.ultima.Row <= 0 {
		t.Errorf("la imagen se publicó en el origen del marco (%+v): taparía el borde",
			g2.ultima)
	}
}

func TestUnaImagenQueNoSePuedeRedimensionarNoSePublica(t *testing.T) {
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	g := &graphicsContador{}
	m.graphics = g
	m.width, m.height = 120, 40

	if m.publishSimImage(imagenRota{w: 0, h: 0}) {
		t.Error("una imagen que no se puede redimensionar se publicó")
	}
	if g.publicadas != 0 {
		t.Errorf("salieron %d publicaciones de una imagen rota", g.publicadas)
	}
}

// republishSimImage runs on EVERY resize while the image is in the layer, so the image-less path is
// not rare.
func TestRepublicarSinImagenNoHaceNadaNiFalla(t *testing.T) {
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	g := &graphicsContador{}
	m.graphics = g
	m.width, m.height = 120, 40
	m.sim.img = nil
	m.sim.viaGraphics = true

	m.republishSimImage()

	if g.publicadas != 0 {
		t.Errorf("se publicaron %d imágenes sin imagen que publicar", g.publicadas)
	}
	if !m.sim.viaGraphics {
		t.Error("sin imagen, republishSimImage turned off viaGraphics: the popup would have " +
			"neither layer nor half-blocks")
	}

	m2 := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	g2 := &graphicsContador{}
	m2.graphics = g2
	m2.width, m2.height = 120, 40
	m2.sim.viaGraphics = true
	m2.sim.img = imagenRota{w: 0, h: 0}
	m2.republishSimImage()
	if m2.sim.viaGraphics {
		t.Error("con una imagen que no se publica, viaGraphics sigue a true: la TUI cree que " +
			"hay imagen en la capa y no pinta los half-blocks de repuesto")
	}
	if g2.publicadas != 0 {
		t.Errorf("salieron %d publicaciones de una imagen que no se puede redimensionar",
			g2.publicadas)
	}
}

// The placeholder is not cosmetic: an item whose SourceBranch came back empty paints as
// "the PR branch".
func TestElChooserPoneLaRamaDelItemYConSuPlaceholderSiNoLaHay(t *testing.T) {
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	it := mkItem("github", "github.com", "acme/widget", "uno", 7, "")
	it.SourceBranch = "feat/mi-rama"
	m.sim.item = it

	if box := m.simChooserBox(); !strings.Contains(box, "feat/mi-rama") {
		t.Errorf("el popup no nombra la rama del ítem:\n%s", box)
	}
	if box := m.simChooserBox(); !strings.Contains(box, it.TargetBranch) {
		t.Errorf("el popup no nombra la base:\n%s", box)
	}

	m.sim.item.SourceBranch = ""
	box := m.simChooserBox()
	if !strings.Contains(box, "the PR branch") {
		t.Errorf("sin rama de origen el popup pone %q, y deja la línea a medias", box)
	}
	if strings.Contains(box, "  \n") {
		t.Errorf("el popup tiene una línea vacía:\n%s", box)
	}
	m.sim.item.SourceBranch = "   "
	if box := m.simChooserBox(); !strings.Contains(box, "the PR branch") {
		t.Errorf("una rama de solo espacios no cuenta como vacía:\n%s", box)
	}
}

// A zero interval is the config with `refresh_interval = "0s"`, which is a MANUAL refresh: there is
// no tick to arm and arming one would tick with no work.
func TestElTickNoSeArmaConIntervaloCeroNiConRefrescoPausado(t *testing.T) {
	for _, d := range []time.Duration{0, -time.Second, -time.Hour} {
		m := newTestModel(t)
		m.cfg.RefreshInterval = d
		if cmd := m.tickCmd(); cmd != nil {
			t.Errorf("con refresh_interval=%v se armó un tick: la cadena consultaría sin parar",
				d)
		}
	}

	m2 := newTestModel(t)
	m2.cfg.RefreshInterval = 90 * time.Second
	if cmd := m2.tickCmd(); cmd == nil {
		t.Error("con un intervalo válido no se armó el tick")
	}
}

// tickToast comes from tea.Every, not the events channel, so it must NOT go through withPump.
func TestElTickDelToastEsUnRelojYNoUnLectorDelCanal(t *testing.T) {
	cmd := tickToast()
	if cmd == nil {
		t.Fatal("tickToast devolvió nil: la cadena de los avisos se corta")
	}
	msg := cmd()
	if _, ok := msg.(toastTickMsg); !ok {
		t.Errorf("tickToast devolvió %T, want toastTickMsg", msg)
	}

}

// The case that really happens: the forge returns the branches AND a warning, and changing the base on
// a partial list is worse than not changing it.
func TestUnListadoDeRamasConAvisosLosConvierteEnMensajeDeError(t *testing.T) {
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	m.retarget.state = retargetListing
	m.retarget.item = mkItem("github", "github.com", "acme/widget", "uno", 7, "")
	m.branchSeq = 1

	ok := send(t, m, branchesMsg{
		seq: 1, key: keyOf(m.retarget.item),
		names: []string{"main", "release/2.0"},
	})
	if ok.retarget.errMsg != "" {
		t.Errorf("un listado sin avisos puso un error: %q", ok.retarget.errMsg)
	}
	if ok.retarget.state != retargetChoosing {
		t.Errorf("un listado sin avisos no abrió el selector (state=%d)", ok.retarget.state)
	}

	m2 := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	m2.retarget.state = retargetListing
	m2.retarget.item = mkItem("github", "github.com", "acme/widget", "uno", 7, "")
	m2.branchSeq = 1

	conAviso := send(t, m2, branchesMsg{
		seq: 1, key: keyOf(m2.retarget.item),
		names:  []string{"main"},
		errMsg: "no se pudo consultar una rama",
	})
	if !strings.Contains(conAviso.retarget.errMsg, "no se pudo consultar una rama") {
		t.Errorf("el aviso del forge no llegó al popup: %q", conAviso.retarget.errMsg)
	}
	if conAviso.retarget.state == retargetChoosing {
		t.Error("un listado parcial abrió el selector: se presentaría como completo")
	}
}

func TestStartRetargetCierraElPopupSiLaAccionNoSePuedeHacer(t *testing.T) {
	for _, c := range []struct {
		nombre  string
		prepara func(*testing.T) Model
	}{
		{"forge desconocido", func(t *testing.T) Model {
			return newTestModel(t)
		}},
		{"acción en curso", func(t *testing.T) Model {
			m := newTestModel(t, &testutil.FakeAdapter{
				ForgeName: "github", HostName: "github.com",
			})
			m.actionBusy = true
			m.retarget.state = retargetConfirm
			m.retarget.item = mkItem("github", "github.com", "acme/widget", "uno", 7, "")
			m.retarget.view = []string{"main", "otra"}
			return m
		}},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			m := c.prepara(t)
			if cmd := m.startRetarget("otra"); cmd != nil {
				t.Error("una acción que no puede hacerse devolvió un comando: se aplicaría el " +
					"cambio de base igualmente")
			}
			if m.retarget.state != retargetClosed {
				t.Errorf("el popup quedó en %d, want cerrado: no hay nada que confirmar si la "+
					"acción no puede hacerse", m.retarget.state)
			}
		})
	}
}

type graphicsContador struct {
	publicadas int
	ultima     herdr.Placement
}

func (g *graphicsContador) Available() bool                     { return true }
func (g *graphicsContador) CellSize(context.Context) (int, int) { return 1, 2 }
func (g *graphicsContador) SetImage(_ context.Context, _ string, _ image.Image, p herdr.Placement) error {
	g.publicadas++
	g.ultima = p
	return nil
}
func (g *graphicsContador) Clear(context.Context, string) error { return nil }

var (
	_ = forge.ActionRetarget
	_ = model.SectionReview
	_ = sim.KindMerge
	_ = tea.KeyPressMsg{}
)

func imagenDiminuta(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{R: uint8(x * 8), G: uint8(y * 8), B: 64, A: 255})
		}
	}
	return img
}

// The ZERO size is what really returns nil, not incoherent Bounds: Resize downloads the image first.
type imagenRota struct{ w, h int }

func (i imagenRota) ColorModel() color.Model { return color.RGBAModel }
func (i imagenRota) Bounds() image.Rectangle { return image.Rect(0, 0, i.w, i.h) }
func (imagenRota) At(int, int) color.Color   { return color.RGBA{} }

// This test REPLACES the guard it documents: the inner gap is exactly what FitCells returned, and
// FitCells guarantees at least 1. What is checked is that the invariant HOLDS across the
// three states and terminal widths from 0 to 400, not that the formula is true.
func TestElHuecoInteriorDelPopupNuncaDesaparecePorMuchoQueSeEstrecheLaTerminal(t *testing.T) {
	for _, estado := range []simState{simShowing, simChoosing, simRendering} {
		for _, ancho := range []int{0, 1, 2, 10, 20, 38, 40, 64, 80, 200, 400} {
			m := newTestModel(t, &testutil.FakeAdapter{
				ForgeName: "github", HostName: "github.com",
			})
			g := &graphicsConteo{celdaW: 8, celdaH: 16}
			m.graphics = g
			m.sim.state = estado
			m.sim.img = imagenDiminuta(640, 480)
			m.width, m.height = ancho, 3

			cols, rows := m.simBox()
			innerCols, innerRows := cols-2, rows-simChrome
			if innerCols <= 0 || innerRows <= 0 {
				t.Errorf("estado %v en terminal de %d columnas: hueco interior de %dx%d. Es "+
					"lo que el guard eliminado comprobaba, y el suelo de contentWidth lo "+
					"impide", estado, ancho, innerCols, innerRows)
			}

			if !m.publishSimImage(m.sim.img) {
				t.Errorf("estado %v en terminal de %d columnas: no publicó con un hueco de "+
					"%dx%d", estado, ancho, innerCols, innerRows)
				continue
			}
			if g.llamadas != 1 {
				t.Errorf("estado %v en terminal de %d columnas: %d publicaciones, want 1",
					estado, ancho, g.llamadas)
			}
			if g.ultima.Cols <= 0 || g.ultima.Rows <= 0 {
				t.Errorf("estado %v en terminal de %d columnas: rectángulo de %dx%d celdas",
					estado, ancho, g.ultima.Cols, g.ultima.Rows)
			}
			if g.ultima.Cols != innerCols || g.ultima.Rows != innerRows {
				t.Errorf("estado %v en terminal de %d columnas: se publicó en %dx%d y el hueco "+
					"era de %dx%d: la imagen se saldría del marco",
					estado, ancho, g.ultima.Cols, g.ultima.Rows, innerCols, innerRows)
			}
		}
	}
}

// The previous test proves the lower bound (never smaller than the minimum); this one proves the
// other side, since a box wider than the terminal is cut and the popup looks split.
func TestConUnaImagenEnormeElPopupSeAjustaAlTerminalYNoLoDesborda(t *testing.T) {
	for _, tam := range [][2]int{{40, 12}, {80, 24}, {120, 40}} {
		m := newTestModel(t, &testutil.FakeAdapter{
			ForgeName: "github", HostName: "github.com",
		})
		m.sim.state = simShowing
		m.sim.img = imagenDiminuta(4000, 3000)
		m.width, m.height = tam[0], tam[1]

		cols, rows := m.simBox()
		if cols > tam[0] {
			t.Errorf("terminal de %d columnas: la caja del popup mide %d y se sale por el "+
				"borde derecho", tam[0], cols)
		}
		if rows > tam[1] {
			t.Errorf("terminal de %d filas: la caja del popup mide %d y se sale por el borde "+
				"inferior", tam[1], rows)
		}
	}
}

// It exists because graphicsCaptura does not count SetImage, and whether anything was published is
// exactly what the guard's false means.
type graphicsConteo struct {
	celdaW, celdaH int

	llamadas int
	ultima   herdr.Placement
}

func (g *graphicsConteo) Available() bool { return true }

func (g *graphicsConteo) CellSize(context.Context) (int, int) { return g.celdaW, g.celdaH }

func (g *graphicsConteo) SetImage(_ context.Context, layer string, _ image.Image, p herdr.Placement) error {
	g.llamadas++
	g.ultima = p
	return nil
}

func (g *graphicsConteo) Clear(context.Context, string) error { return nil }
