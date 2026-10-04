package tui

import (
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"prdash/internal/forge/model"
	"prdash/internal/herdr"
	"prdash/internal/sim"
)

// A real JPEG, because applySim loads it with sim.Load: an invented path would only measure the
// discard, not the good path loading the image.
func renderFalso(t *testing.T) sim.Result {
	t.Helper()
	path := filepath.Join(t.TempDir(), "render.jpg")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("crear el render falso: %v", err)
	}
	defer func() { _ = f.Close() }()
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := range 64 {
		for x := range 64 {
			img.Set(x, y, color.RGBA{R: 200, G: 100, B: 50, A: 255})
		}
	}
	if err := jpeg.Encode(f, img, nil); err != nil {
		t.Fatalf("codificar el render falso: %v", err)
	}
	return sim.Result{Kind: sim.KindMerge, Path: path, Ref: "HEAD", Base: "main"}
}

func simKindDePrueba(_ bool) sim.Kind { return sim.KindMerge }

func otraKind() sim.Kind { return sim.KindRebase }

// The counter is incremented in startSim BEFORE leaving to the goroutine, so what is slow here does
// not have to do with what is invalidated.
type simuladorMudo struct{}

func (simuladorMudo) Available() bool { return true }

func (simuladorMudo) Simulate(context.Context, model.Item, sim.Kind) (sim.Result, error) {
	return sim.Result{}, errors.New("simulador mudo: este test no renderiza")
}

// The 64x64 size is not arbitrary: with a 4x4 image it fitted in ONE cell at any cell size, so
// changing the cell changed nothing and every geometry assertion passed without looking.
func imagenParaLaGeometria() image.Image {
	const lado = 64
	img := image.NewRGBA(image.Rect(0, 0, lado, lado))
	for y := range lado {
		for x := range lado {
			img.Set(x, y, image.White)
		}
	}
	return img
}

func modeloConSimDeLasCeldasDadas(t *testing.T, w, h int) Model {
	t.Helper()
	m := newTestModel(t)
	m.width, m.height = 120, 40
	m.sim.state = simShowing
	m.sim.img = imagenParaLaGeometria()
	m.sim.viaGraphics = false
	m.sim.cellW_px, m.sim.cellH_px = w, h
	return m
}

func TestRenderSimCellsComponeYAnotaLaGeometria(t *testing.T) {
	m := modeloConSimDeLasCeldasDadas(t, 1, 2)
	m.renderSimCells()

	if len(m.sim.cells) == 0 {
		t.Fatal("con el popup visible y la imagen puesta no compuso celdas ninguna")
	}
	wantW, wantH := m.simBox()
	wantW, wantH = wantW-2, wantH-simChrome
	if m.sim.cellW != wantW || m.sim.cellH != wantH {
		t.Errorf("anotó %dx%d, want %dx%d (la geometría interior de la caja)",
			m.sim.cellW, m.sim.cellH, wantW, wantH)
	}
}

func TestRenderSimCellsNoRecomposeConLaMismaGeometria(t *testing.T) {
	m := modeloConSimDeLasCeldasDadas(t, 1, 2)
	m.renderSimCells()
	primeras := len(m.sim.cells)
	if primeras == 0 {
		t.Fatal("no compuso nada la primera vez")
	}

	for range 4 {
		m.renderSimCells()
	}
	if len(m.sim.cells) != primeras {
		t.Errorf("tras cuatro render con la misma geometría hay %d celdas, want %d: "+
			"la caché no está cortando, y reescalar la imagen entera en cada resize es "+
			"justo lo que hace que el popup se congele", len(m.sim.cells), primeras)
	}
	if m.sim.cellW == 0 || m.sim.cellH == 0 {
		t.Error("la geometría anotada se ha perdido: sin ella la caché no puede validarse")
	}
}

func TestRenderSimCellsRecomponeAlCambiarLaGeometria(t *testing.T) {
	casos := []struct {
		nombre string
		ancho  int
		alto   int
	}{
		{"más ancho", 2, 2},       // solo cambia w
		{"más alto", 1, 4},        // solo cambia h
		{"otro de las dos", 2, 3}, // cambian las dos
		{"más estrecho y más bajo", 1, 1},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			m := modeloConSimDeLasCeldasDadas(t, 1, 2)
			m.renderSimCells()
			if len(m.sim.cells) == 0 {
				t.Fatal("no compuso nada la primera vez")
			}
			antesW, antesH := m.sim.cellW, m.sim.cellH

			m.sim.cellW_px, m.sim.cellH_px = c.ancho, c.alto
			m.renderSimCells()

			wantW, wantH := m.simBox()
			wantW, wantH = wantW-2, wantH-simChrome

			if m.sim.cellW != wantW || m.sim.cellH != wantH {
				t.Errorf("tras cambiar la celda a %dx%d la geometría anotada quedó %dx%d, "+
					"want %dx%d: no se recompuso con la nueva, y la imagen se queda desfasada",
					c.ancho, c.alto, m.sim.cellW, m.sim.cellH, wantW, wantH)
			}
			// The new geometry has to DIFFER from the old one, or the case would not tell a recomposed from
			// a no-op and the assertion above would pass with nothing happening.
			if m.sim.cellW == antesW && m.sim.cellH == antesH {
				t.Errorf("la geometría anotada sigue en %dx%d tras cambiar la celda a "+
					"%dx%d: el caso no probaría el cambio, daría lo mismo Compose y no-op",
					antesW, antesH, c.ancho, c.alto)
			}
		})
	}
}

// The stale cells belong to the PREVIOUS image with a different geometry; leaving them would paint
// the old image inside the new frame, so the user sees a review that is not the one in front of them.
func TestRenderSimCellsSinImagenNoDejaCeldasViejas(t *testing.T) {
	m := modeloConSimDeLasCeldasDadas(t, 1, 2)
	m.renderSimCells()
	if len(m.sim.cells) == 0 {
		t.Fatal("no compuso nada la primera vez")
	}

	for _, c := range []struct {
		nombre  string
		prepara func(*Model)
	}{
		{"la imagen desaparece", func(m *Model) { m.sim.img = nil }},
		{"el popup ya no está visible", func(m *Model) { m.sim.state = simRendering }},
		{"la imagen va por la capa de gráficos", func(m *Model) { m.sim.viaGraphics = true }},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			m := modeloConSimDeLasCeldasDadas(t, 1, 2)
			m.renderSimCells()
			if len(m.sim.cells) == 0 {
				t.Fatal("no compuso nada la primera vez")
			}
			c.prepara(&m)
			m.renderSimCells()

			if len(m.sim.cells) != 0 {
				t.Errorf("quedaron %d celdas de la imagen anterior: se vería el review "+
					"viejo con el marco del nuevo, y sin nada que diga que está desfasado",
					len(m.sim.cells))
			}
			if m.sim.cellW != 0 || m.sim.cellH != 0 {
				t.Errorf("la geometría anotada quedó en %dx%d, want 0x0: sin celdas no "+
					"puede quedar una geometría, o la siguiente vuelta compararía contra ella",
					m.sim.cellW, m.sim.cellH)
			}
		})
	}
}

// Not a tautology: it CALLS simBox's formula, and `cols-2 > 0` alone is not enough. This
// REPLACES a test whose t.Fatalf never ran: it swept 4,500 combinations and always passed.
func TestLaCajaDelPopupSiempreTieneHuecoInterior(t *testing.T) {
	imagenes := []image.Image{
		imagenParaLaGeometria(), // cuadrada
		imagenDe(4, 512),        // vertical extrema: la de una columna
		imagenDe(2, 512),        // todavía más vertical
		imagenDe(512, 4),        // apaisada extrema
		imagenDe(64, 64),
	}

	for vi, img := range imagenes {
		for _, celda := range [][2]int{{1, 2}, {2, 1}, {9, 19}, {1, 19}} {
			for w := 40; w <= 200; w += 13 {
				for h := 6; h <= 120; h += 11 {
					m := modeloConSimDeLasCeldasDadas(t, celda[0], celda[1])
					m.width, m.height = w, h
					m.sim.state = simShowing
					m.sim.img = img
					cols, rows := m.simBox()
					if cols-2 <= 0 || rows-simChrome <= 0 {
						t.Fatalf("imagen %d, celda %dx%d, terminal %dx%d: el hueco interior "+
							"es de %dx%d, que no cabe ni una celda. simBox devolvió %dx%d, "+
							"y FitCells no puede devolver menos de 1 en ninguna dimensión",
							vi, celda[0], celda[1], w, h,
							cols-2, rows-simChrome, cols, rows)
					}
				}
			}
		}
	}
}

// Not a counter: a mechanism for INVALIDATING, which works only because the number always moves
// up. With `--` instead, closing with `++` lands back on 0 and an old render paints over the new.
func TestSimSeqSubeYNuncaBaja(t *testing.T) {
	m := newTestModel(t)
	m.SetSimulator(simuladorMudo{})
	partido := m.simSeq
	vistos := map[int]bool{}

	for i := range 20 {
		m.startSim(simKindDePrueba(false))
		// An increment of ONE, not just "it went up": if open and close summed more, two opens could land
		// on the same number and the discard would break unnoticed.
		if m.simSeq != partido+1 {
			t.Fatalf("abrir el popup (vuelta %d) dejó simSeq en %d, want %d: "+
				"tiene que subir de uno en uno", i, m.simSeq, partido+1)
		}
		partido = m.simSeq
		if vistos[m.simSeq] {
			t.Fatalf("simSeq=%d ya se había visto: un número repetido hace que un "+
				"resultado antiguo pase la comparación de applySim", m.simSeq)
		}
		vistos[m.simSeq] = true

		m.closeSim()
		if m.simSeq != partido+1 {
			t.Fatalf("cerrar el popup %d vez no subió simSeq: quedó en %d, tenía "+
				"que pasar de %d. Un cierre que no invalida deja que el render en vuelo "+
				"pinte su imagen en un popup que ya no existe", i, m.simSeq, partido)
		}
		partido = m.simSeq
		if vistos[m.simSeq] {
			t.Fatalf("simSeq=%d ya se había visto tras cerrar", m.simSeq)
		}
		vistos[m.simSeq] = true
	}
}

// The case that matters is the CHANGED strategy with the popup open: the first result arrives
// while the second is still rendering and would paint over it.
func TestApplySimDescartaLoObsoleto(t *testing.T) {
	for _, c := range []struct {
		nombre  string
		prepara func(*Model)
		quiere  string
	}{
		{
			"el popup se cerró mientras corría",
			func(m *Model) { m.closeSim() },
			"quedarse sin imagen",
		},
		{
			"el popup se reabrió con otra estrategia",
			func(m *Model) {
				m.sim.state = simRendering
				m.sim.kind = otraKind()
				m.simSeq++
			},
			"quedarse con la estrategia nueva y sin imagen",
		},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			m := modeloConSimDeLasCeldasDadas(t, 1, 2)
			m.SetSimulator(simuladorMudo{})
			m.startSim(simKindDePrueba(false))
			obsoleto := simMsg{seq: m.simSeq, kind: m.sim.kind}

			c.prepara(&m)
			antesSeq := m.simSeq

			m.applySim(obsoleto)

			if m.simSeq != antesSeq {
				t.Errorf("applySim movió simSeq de %d a %d al descartar un resultado "+
					"obsoleto: descartar no es avanzar, y avanzar invalida la petición "+
					"que sí está en vuelo", antesSeq, m.simSeq)
			}
			if len(m.sim.cells) != 0 {
				t.Errorf("un resultado obsoleto dejó %d celdas: se pintó el render de "+
					"la petición anterior, y no hay nada que diga que está desfasado",
					len(m.sim.cells))
			}
		})
	}

	t.Run("el resultado que sí es el actual se aplica", func(t *testing.T) {
		m := modeloConSimDeLasCeldasDadas(t, 1, 2)
		m.SetSimulator(simuladorMudo{})
		m.startSim(simKindDePrueba(false))
		actual := simMsg{seq: m.simSeq, kind: m.sim.kind, res: renderFalso(t)}

		m.applySim(actual)

		if m.sim.state != simShowing {
			t.Errorf("state=%v, want simShowing: un resultado actual tiene que abrir "+
				"el popup, o el descarte de los obsoletos no descarta nada",
				m.sim.state)
		}
		if m.sim.img == nil {
			t.Error("no se cargó la imagen del resultado actual")
		}
	})
}

// Storing the context is what makes the delete's timeout testable: "how long does this wait" is
// answered by the deadline the call received, not by the clock.
type graphicsCaptura struct {
	celdaW, celdaH int

	mu       sync.Mutex
	clearCtx []clearCall
	capas    []string
}

// The moment matters: releaseSimLayer does `defer cancel()`, so a double that stored the context
// would always read "cancelled" and conclude the delete inherits the app's context.
type clearCall struct {
	errAlLlamar   error
	plazoAlLlamar time.Duration
	hayPlazo      bool
}

func (g *graphicsCaptura) Available() bool { return true }

func (g *graphicsCaptura) CellSize(context.Context) (int, int) { return g.celdaW, g.celdaH }

func (g *graphicsCaptura) SetImage(context.Context, string, image.Image, herdr.Placement) error {
	return nil
}

func (g *graphicsCaptura) Clear(ctx context.Context, layer string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	dl, hay := ctx.Deadline()
	var plazo time.Duration
	if hay {
		// The deadline is read HERE, at call time, not when the test looks later: the test's own time passes
		// in between, and with a millisecond deadline you would be measuring the test's clock.
		plazo = time.Until(dl)
	}
	g.clearCtx = append(g.clearCtx, clearCall{
		errAlLlamar:   ctx.Err(),
		plazoAlLlamar: plazo,
		hayPlazo:      hay,
	})
	g.capas = append(g.capas, layer)
	return nil
}

func (g *graphicsCaptura) ctxs() []clearCall {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]clearCall(nil), g.clearCtx...)
}

func (g *graphicsCaptura) capasVistas() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]string(nil), g.capas...)
}

func TestLaCedaSinMedirSupone1x2(t *testing.T) {
	for _, c := range []struct {
		nombre string
		medida [2]int
	}{
		{"Herdr no sabe nada", [2]int{0, 0}},
		{"Herdr mide ancho pero no alto", [2]int{9, 0}},
		{"Herdr mide alto pero no ancho", [2]int{0, 19}},
		{"Herdr devuelve negativos, que no son medidas", [2]int{-9, -19}},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			m := modeloConSimDeLasCeldasDadas(t, 1, 2)
			m.sim.state = simShowing
			m.sim.img = imagenParaLaGeometria()
			m.graphics = &graphicsCaptura{celdaW: c.medida[0], celdaH: c.medida[1]}

			w, h := m.cellSize()
			if w != 1 || h != 2 {
				t.Errorf("con CellSize devolviendo %v, cellSize dio %dx%d, want 1x2: "+
					"una celda de terminal es un carácter de ancho por dos puntos de alto",
					c.medida, w, h)
			}
		})
	}

	t.Run("la medida de Herdr manda", func(t *testing.T) {
		m := modeloConSimDeLasCeldasDadas(t, 9, 19)
		w, h := m.cellSize()
		if w != 9 || h != 19 {
			t.Errorf("con Herdr midiendo 9x19, cellSize dio %dx%d, want 9x19", w, h)
		}
	})
}

func TestElSueloDeLaCedaCambiaLaGeometriaDeLaImagen(t *testing.T) {
	m := modeloConSimDeLasCeldasDadas(t, 0, 0) // Herdr no mide: entra el suelo
	m.sim.state = simShowing
	m.sim.img = imagenParaLaGeometria()
	m.renderSimCells()
	if m.sim.cellW == 0 {
		t.Fatal("con el suelo 1x2 no compuso nada")
	}
	ancho1x2 := m.sim.cellW

	m2 := modeloConSimDeLasCeldasDadas(t, 2, 1)
	m2.sim.state = simShowing
	m2.sim.img = imagenParaLaGeometria()
	m2.renderSimCells()
	ancho2x1 := m2.sim.cellW

	if ancho2x1 >= ancho1x2 {
		t.Errorf("con celda 2px de ancho hay %d columnas y con 1px hay %d: una celda del "+
			"doble de ancho tiene que dar menos columnas para la misma imagen",
			ancho2x1, ancho1x2)
	}
	m4 := modeloConSimDeLasCeldasDadas(t, 4, 1)
	m4.sim.state = simShowing
	m4.sim.img = imagenParaLaGeometria()
	m4.renderSimCells()
	if m4.sim.cellW >= ancho2x1 {
		t.Errorf("con celda 4px hay %d columnas y con 2px hay %d: el número de columnas "+
			"tiene que bajar con el ancho de la celda", m4.sim.cellW, ancho2x1)
	}
}

func TestElBorradoDeLaCapaEsperaPocoYEnSegundoPlano(t *testing.T) {
	g := &graphicsCaptura{}
	m := newTestModel(t)
	m.graphics = g
	m.sim.viaGraphics = true

	m.releaseSimLayer()

	llamadas := esperarLlamadas(t, g, 200, 5*time.Millisecond)
	if len(llamadas) != 1 {
		t.Fatalf("se llamó a Clear %d veces, want 1: un reintento sobre una capa que ya "+
			"no está no la quita más rápido", len(llamadas))
	}
	llamada := llamadas[0]

	if !llamada.hayPlazo {
		t.Fatal("el contexto del borrado no tiene deadline: si Herdr no responde, el " +
			"Clear se queda esperando y lo que se congela es el proceso al salir")
	}
	const (
		want = 2 * time.Second
		tol  = 20 * time.Millisecond
	)
	if d := llamada.plazoAlLlamar - want; d > tol || d < -tol {
		t.Errorf("el plazo del borrado es de %v al llamar, want %v±%v (se aparta %v): "+
			"por debajo el Clear no tiene ni tiempo de contestarle a Herdr, y por arriba "+
			"lo que se congela es el proceso al salir",
			llamada.plazoAlLlamar.Round(time.Millisecond), want, tol,
			d.Round(time.Millisecond))
	}
	if llamada.errAlLlamar != nil {
		t.Errorf("el contexto del borrado ya estaba cancelado (%v) al llamar: si "+
			"heredara el de la app, el Clear no llegaría a hacer nada y la imagen se "+
			"quedaría pegada al salir de la TUI", llamada.errAlLlamar)
	}
}

func TestElBorradoNoSeIntentaSiNoHayImagenEnLaCapa(t *testing.T) {
	for _, c := range []struct {
		nombre  string
		prepara func(*Model)
	}{
		{"no se publicó nada", func(m *Model) { m.sim.viaGraphics = false }},
		{"no hay graphics", func(m *Model) { m.graphics = nil }},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			g := &graphicsCaptura{}
			m := newTestModel(t)
			m.graphics = g
			m.sim.viaGraphics = true
			c.prepara(&m)

			m.releaseSimLayer()
			esperarLlamadas(t, g, 40, 5*time.Millisecond)

			if len(g.ctxs()) != 0 {
				t.Errorf("llamó a Clear %d veces cuando no había nada en la capa: "+
					"es una llamada a un proceso entero, y el caso de no tener nada es el normal",
					len(g.ctxs()))
			}
		})
	}
}

func TestElBorradoVaSobreLaCapaDelSimulador(t *testing.T) {
	g := &graphicsCaptura{}
	m := newTestModel(t)
	m.graphics = g
	m.sim.viaGraphics = true

	m.releaseSimLayer()
	esperarLlamadas(t, g, 200, 5*time.Millisecond)

	capas := g.capasVistas()
	if len(capas) != 1 {
		t.Fatalf("se llamó a Clear %d veces, want 1", len(capas))
	}
	if capas[0] != simLayer {
		t.Errorf("borró la capa %q, want %q: un Clear sobre otra capa es un no-op "+
			"silencioso, y la imagen se queda pegada sin que nada lo relacione con el popup",
			capas[0], simLayer)
	}
}

func esperarLlamadas(t *testing.T, g *graphicsCaptura, intentos int, espera time.Duration) []clearCall {
	t.Helper()
	for range intentos {
		if c := g.ctxs(); len(c) > 0 {
			return c
		}
		time.Sleep(espera)
	}
	return g.ctxs()
}

type graphicsConErrores struct {
	celdaW, celdaH int
	celdaErr       error
	setErr         error

	mu        sync.Mutex
	setCalls  []herdr.Placement
	setCtxErr []error
	setImagen []image.Image
}

func (g *graphicsConErrores) Available() bool { return true }

func (g *graphicsConErrores) CellSize(context.Context) (int, int) {
	if g.celdaErr != nil {
		return 0, 0
	}
	return g.celdaW, g.celdaH
}

func (g *graphicsConErrores) SetImage(ctx context.Context, _ string, img image.Image, p herdr.Placement) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.setCalls = append(g.setCalls, p)
	g.setCtxErr = append(g.setCtxErr, ctx.Err())
	g.setImagen = append(g.setImagen, img)
	return g.setErr
}

func (g *graphicsConErrores) Clear(context.Context, string) error { return nil }

func (g *graphicsConErrores) placements() []herdr.Placement {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]herdr.Placement(nil), g.setCalls...)
}

func (g *graphicsConErrores) contextos() []error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]error(nil), g.setCtxErr...)
}

func TestPublishSimImageMideLaCedaYLaGuarda(t *testing.T) {
	const celdaW, celdaH = 9, 19
	g := &graphicsConErrores{celdaW: celdaW, celdaH: celdaH}

	m := modeloConSimDeLasCeldasDadas(t, 0, 0)
	m.graphics = g
	m.sim.state = simShowing
	m.sim.img = imagenParaLaGeometria()

	cols, rows := m.simBox()
	col, row := m.simBoxOrigin(cols, rows)
	want := herdr.Placement{
		Col:  col + 1,
		Row:  row + 1,
		Cols: cols - 2,
		Rows: rows - simChrome,
	}

	if !m.publishSimImage(m.sim.img) {
		t.Fatal("publishSimImage dijo que no pudo publicar, y el doble no falla en nada")
	}

	if m.sim.cellW_px != celdaW || m.sim.cellH_px != celdaH {
		t.Errorf("guardó la celda como %dx%d, want %dx%d: sin guardarla, si la capa deja "+
			"de estar disponible el popup dibuja a 1x2 una imagen que se dimensionó para "+
			"%dx%d", m.sim.cellW_px, m.sim.cellH_px, celdaW, celdaH, celdaW, celdaH)
	}

	placements := g.placements()
	if len(placements) != 1 {
		t.Fatalf("mandó %d imágenes, want 1", len(placements))
	}
	if placements[0] != want {
		t.Errorf("colocó en %+v, want %+v: la imagen va en el hueco interior del popup, "+
			"una celda dentro del marco", placements[0], want)
	}
}

func TestPublishSimImageSinCeldaMedidaAsume1x2(t *testing.T) {
	for _, c := range []struct {
		nombre string
		medida [2]int
	}{
		{"Herdr devuelve (0,0)", [2]int{0, 0}},
		{"Herdr mide solo el ancho", [2]int{9, 0}},
		{"Herdr mide solo el alto", [2]int{0, 19}},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			g := &graphicsConErrores{celdaW: c.medida[0], celdaH: c.medida[1]}
			m := modeloConSimDeLasCeldasDadas(t, 0, 0)
			m.graphics = g
			m.sim.state = simShowing
			m.sim.img = imagenParaLaGeometria()

			if !m.publishSimImage(m.sim.img) {
				t.Fatal("sin celda medida no pudo publicar: el suelo 1x2 tiene que " +
					"dejar el camino de la capa gráfica disponible")
			}
			if m.sim.cellW_px != 1 || m.sim.cellH_px != 2 {
				t.Errorf("guardó la celda como %dx%d, want 1x2",
					m.sim.cellW_px, m.sim.cellH_px)
			}
		})
	}
}

func TestPublicarUsaElContextoPropioYNoElDeLaApp(t *testing.T) {
	g := &graphicsConErrores{celdaW: 9, celdaH: 19}
	m := modeloConSimDeLasCeldasDadas(t, 0, 0)
	m.graphics = g
	m.sim.state = simShowing
	m.sim.img = imagenParaLaGeometria()

	ctx, cancel := context.WithCancel(m.ctx)
	cancel()
	m.ctx = ctx

	if !m.publishSimImage(m.sim.img) {
		t.Fatal("no pudo publicar con el contexto de la app cancelado: la publicación " +
			"tiene que llevar el suyo, o al salir de la TUI la imagen se queda pegada")
	}

	ctxs := g.contextos()
	if len(ctxs) != 1 {
		t.Fatalf("mandó %d imágenes, want 1", len(ctxs))
	}
	if ctxs[0] != nil {
		t.Errorf("el contexto de la publicación estaba cancelado (%v) al mandar la "+
			"imagen: heredó el de la app, que al salir de la TUI ya está muerto",
			ctxs[0])
	}
}

func TestPublishSimImageNoPublicaSiElSetFalla(t *testing.T) {
	g := &graphicsConErrores{celdaW: 9, celdaH: 19, setErr: errors.New("layer busy")}
	m := modeloConSimDeLasCeldasDadas(t, 0, 0)
	m.graphics = g
	m.sim.state = simShowing
	m.sim.img = imagenParaLaGeometria()

	if m.publishSimImage(m.sim.img) {
		t.Fatal("dijo que publicó cuando Herdr la rechazó")
	}
	if m.sim.viaGraphics {
		t.Error("marcó viaGraphics con la publicación fallida: el popup no pintaría nada " +
			"y no sabría que lo espera, o sea un marco vacío")
	}
}

func TestPublishSimImageMarcaLaBanderaYVaciaLasCeldas(t *testing.T) {
	g := &graphicsConErrores{celdaW: 9, celdaH: 19}
	m := modeloConSimDeLasCeldasDadas(t, 1, 2)
	m.graphics = g
	m.sim.state = simShowing
	m.sim.img = imagenParaLaGeometria()

	m.renderSimCells()
	if len(m.sim.cells) == 0 {
		t.Fatal("no dejó celdas previas: el caso necesita celdas que puedan quedar")
	}

	if !m.publishSimImage(m.sim.img) {
		t.Fatal("no pudo publicar")
	}
	if !m.sim.viaGraphics {
		t.Error("no marcó viaGraphics tras publicar: el popup se pintaría a sí mismo " +
			"una imagen que la capa ya está pintando")
	}
	if len(m.sim.cells) != 0 {
		t.Errorf("quedaron %d celdas tras publicar en la capa: son de la imagen anterior, "+
			"que es de otra geometría, y se pintarían encima de la nueva", len(m.sim.cells))
	}
}

func TestPublishSimImageSinGraficosNiCapaNoIntentaNada(t *testing.T) {
	m := modeloConSimDeLasCeldasDadas(t, 0, 0)
	m.graphics = nil
	m.sim.state = simShowing
	m.sim.img = imagenParaLaGeometria()
	if m.publishSimImage(m.sim.img) {
		t.Error("publicó sin tener Herdr")
	}
	if m.sim.viaGraphics || m.sim.cellW_px != 0 {
		t.Error("sin Herdr no debe tocar ni la bandera ni la celda guardada")
	}
}

func TestPublishSimImageUsaLaCapaDelSimulador(t *testing.T) {
	g := &graphicsConErrores{celdaW: 9, celdaH: 19}
	m := modeloConSimDeLasCeldasDadas(t, 0, 0)
	m.graphics = g
	m.sim.state = simShowing
	m.sim.img = imagenParaLaGeometria()

	m.publishSimImage(m.sim.img)
	if simLayer == "" {
		t.Error("la capa del simulador está vacía")
	}
}

func TestPublishSimImageNoPublicaConUnaCajaQueNoCabe(t *testing.T) {
	g := &graphicsConErrores{celdaW: 9, celdaH: 19}

	m := modeloConSimDeLasCeldasDadas(t, 0, 0)
	m.graphics = g
	m.sim.state = simShowing
	m.sim.img = imagenParaLaGeometria()

	for w := 20; w >= 4; w-- {
		for h := 8; h >= 4; h-- {
			m.width, m.height = w, h
			antes := len(g.placements())
			m.publishSimImage(m.sim.img)
			cols, rows := m.simBox()
			if cols-2 > 0 && rows-simChrome > 0 {
				continue // la caja todavía cabe
			}
			if len(g.placements()) != antes {
				t.Fatalf("con %dx%d la caja no cabe (%dx%d) y se publicó algo: un hueco "+
					"interior de 0 columnas es una imagen de 0 píxeles",
					w, h, cols-2, rows-simChrome)
			}
		}
	}
}

type casoGeom struct {
	img        image.Image
	col, filas int
}

func TestElAnchoNoDeterminaLaAltura(t *testing.T) {
	const (
		anchoVista, altoVista = 60, 100
		celdaW, celdaH        = 1, 2
	)

	var primero, segundo *casoGeom

	for lado := 8; lado <= 128 && segundo == nil; lado += 8 {
		for altoPx := 8; altoPx <= 128 && segundo == nil; altoPx += 8 {
			m := modeloConSimDeLasCeldasDadas(t, celdaW, celdaH)
			m.width, m.height = anchoVista, altoVista
			m.sim.state = simShowing
			m.sim.img = imagenDe(lado, altoPx)
			m.renderSimCells()
			if m.sim.cellW == 0 || m.sim.cellH == 0 {
				continue
			}
			caso := &casoGeom{img: m.sim.img, col: m.sim.cellW, filas: m.sim.cellH}
			switch {
			case primero == nil:
				primero = caso
			case segundo == nil && caso.col == primero.col && caso.filas != primero.filas:
				segundo = caso
			}
		}
	}

	if segundo == nil {
		t.Fatal("no se encontró ninguna imagen que dé el mismo ancho de caja con " +
			"distinta altura. Si ya no existe, el ancho determina la altura, la " +
			"comparación de cellH de la caché se puede quitar, y este test está " +
			"diciendo algo que ha dejado de ser verdad")
	}
	t.Logf("mismas %d columnas: %d filas con la primera imagen y %d con la segunda",
		primero.col, primero.filas, segundo.filas)

	m := modeloConSimDeLasCeldasDadas(t, celdaW, celdaH)
	m.width, m.height = anchoVista, altoVista
	m.sim.state = simShowing
	m.sim.img = primero.img
	m.renderSimCells()
	if m.sim.cellW != primero.col || m.sim.cellH != primero.filas {
		t.Fatalf("la primera imagen dio %dx%d, want %dx%d",
			m.sim.cellW, m.sim.cellH, primero.col, primero.filas)
	}
	celdasPrimeras := len(m.sim.cells)

	m.sim.img = segundo.img
	m.renderSimCells()

	if m.sim.cellH != segundo.filas {
		t.Errorf("con la segunda imagen (mismo ancho de %d, %d filas) se quedó en %d: "+
			"no recompuso, así que las celdas son de la imagen anterior —%d de ellas— y "+
			"la nueva se sale por abajo del marco",
			segundo.col, segundo.filas, m.sim.cellH, celdasPrimeras)
	}
	if m.sim.cellW != primero.col {
		t.Errorf("el ancho cambió a %d con la segunda imagen, y el caso era de mismo "+
			"ancho (%d)", m.sim.cellW, primero.col)
	}
}

func imagenDe(anchoPx, altoPx int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, anchoPx, altoPx))
	for y := range altoPx {
		for x := range anchoPx {
			img.Set(x, y, image.White)
		}
	}
	return img
}

func TestPadRightRellenarODejarlo(t *testing.T) {
	casos := []struct {
		nombre     string
		s          string
		n          int
		quiereCols int
	}{
		{"corta, rellena", "ab", 6, 6},
		{"justa, no toca", "abcd", 4, 4},
		{"justa con una menos", "abcd", 3, 4},  // no cabe: se deja como está
		{"larguísima", "muchas letras", 3, 13}, // ni la toca
		{"vacía", "", 5, 5},
		{"a cero", "abc", 0, 3},
		{"n negativa", "abc", -4, 3},
		{"con acentos", "áéí", 6, 3}, // 3 columnas, 6 bytes
		{"con emoji", "🙂", 4, 2},     // 2 columnas, 4 bytes
		{"emoji y texto", "a🙂b", 6, 4},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got := padRight(c.s, c.n)
			want := max(ansi.StringWidth(c.s), c.n)

			if w := ansi.StringWidth(got); w != want {
				t.Errorf("padRight(%q, %d) mide %d columnas visibles, want %d. La entrada "+
					"mide %d bytes y %d columnas: rellenar por bytes desalinea las etiquetas, "+
					"que es lo único que esta función arregla",
					c.s, c.n, w, want, len(c.s), ansi.StringWidth(c.s))
			}
			if len(got) < len(c.s) || got[:len(c.s)] != c.s {
				t.Errorf("padRight(%q, %d) devolvió %q, que no empieza por el texto original: "+
					"no se recorta, se rellena", c.s, c.n, got)
			}
			for i, r := range got[len(c.s):] {
				if r != ' ' {
					t.Errorf("el relleno lleva %q en la posición %d, want un espacio: "+
						"un carácter distinto se lee como parte de la etiqueta", r, i)
					break
				}
			}
		})
	}
}

func TestPadRightConElBordeExacto(t *testing.T) {
	for _, s := range []string{"", "a", "ab", "áé", "🙂"} {
		ancho := ansi.StringWidth(s)
		got := padRight(s, ancho)
		if got != s {
			t.Errorf("con el ancho justo (%q ocupa %d) devolvió %q, want el texto sin cambios",
				s, ancho, got)
		}
		if ansi.StringWidth(got) != ancho {
			t.Errorf("con el ancho justo devolvió %d columnas, want %d", ansi.StringWidth(got), ancho)
		}
		if p := padRight(s, ancho+1); ansi.StringWidth(p) != ancho+1 {
			t.Errorf("con una columna de más devolvió %d columnas, want %d: el relleno "+
				"no ocurre, y el aserto del borde exacto pasaría igual",
				ansi.StringWidth(p), ancho+1)
		}
	}
}
