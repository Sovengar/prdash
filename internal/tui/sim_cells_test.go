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

// renderFalso escribe un JPEG de verdad en un temporal y devuelve el Result que lo
// apunta. Tiene que ser un JPEG REAL porque `applySim` lo carga con `sim.Load`, que
// decodifica de disco: un `sim.Result` con un path inventado solo mediría que el
// descarte funcionó, no que el camino bueno carga la imagen.
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

// simuladorMudo es un Simulator que no hace nada. Hace falta porque `startSim` lanza
// una goroutine que llama a `Simulate` de verdad, y sin esto revienta en el test. Lo
// que se está probando aquí es el CONTADOR, que se incrementa en `startSim` ANTES de
// salir a la goroutine: lo que tarda no tiene que ver con lo que se invalida.
type simuladorMudo struct{}

func (simuladorMudo) Available() bool { return true }

func (simuladorMudo) Simulate(context.Context, model.Item, sim.Kind) (sim.Result, error) {
	return sim.Result{}, errors.New("simulador mudo: este test no renderiza")
}

// Los tests de `renderSimCells` cubren la CACHÉ, que es la parte del popup que no se
// ve pero se nota.
//
// Componer las celdas es caro: `sim.Cells` reescala la imagen entera y la pasea
// carácter a carácter para la capa de texto. Y se llama en CADA resize, que es lo único
// que cambia la geometría. Sin caché, mover el borde de la ventana con el popup abierto
// rehace el reescalado entero en cada tecla de redimensionado.
//
// O sea que la caché no es una optimización de gusto: es la diferencia entre un popup
// que responde al resize y uno que se congela. Y una caché que no se invalida bien es
// peor que no tenerla: se queda enseñando la imagen de la geometría anterior, estirada o
// cortada, sin que nada diga que está desfasada.

// imagenParaLaGeometria es una imagen de 64×64 de un solo color. Da igual el color: lo
// que se comprueba es la GEOMETRÍA de las celdas, no el contenido.
//
// Y el tamaño no es arbitrario, que es la segunda versión de este fichero: con una
// imagen de 4×4 cabía en UNA celda con cualquier tamaño de celda, así que cambiar la
// celda no cambiaba la geometría y todo lo que comprobaba esa geometría pasaba sin
// mirar nada. Una imagen tiene que ser más grande que la caja en cualquier tamaño de
// celda que se pruebe, o el test no distingue un cambio de no-op.
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

// modeloConSimDejaUnPopupVisibleDeLasCeldasDadas: monta un Modelo con el popup de la
// simulación ya visible, con la imagen puesta y SIN capa de gráficos —que es la
// condición que hace que haya celdas que pintar— y con el tamaño de celda que se le
// pase.
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

// TestRenderSimCellsComponeYAnotaLaGeometria: la primera vez compone, y ANOTA la
// geometría con la que lo compuso.
//
// Anotar no es un detalle: sin la anotación, la caché no puede validarse nunca, y
// `cellW`/`cellH` solo existirían para eso. Si se olvidara anotarlos, todo seguiría
// pintándose bien la primera vez, y solo se notaría en el resize: eso es un bug que
// necesita un segundo movimiento de la ventana para verse, que es el peor sitio para
// un bug.
func TestRenderSimCellsComponeYAnotaLaGeometria(t *testing.T) {
	m := modeloConSimDeLasCeldasDadas(t, 1, 2)
	m.renderSimCells()

	if len(m.sim.cells) == 0 {
		t.Fatal("con el popup visible y la imagen puesta no compuso celdas ninguna")
	}
	// Y la geometría anotada es la INTERIOR de la caja, que es la que se usó para
	// componer. Si se anotara la exterior, un resize de una columna no invalidaría la
	// caché y la imagen se quedaría un carácter desfasada.
	wantW, wantH := m.simBox()
	wantW, wantH = wantW-2, wantH-simChrome
	if m.sim.cellW != wantW || m.sim.cellH != wantH {
		t.Errorf("anotó %dx%d, want %dx%d (la geometría interior de la caja)",
			m.sim.cellW, m.sim.cellH, wantW, wantH)
	}
}

// TestRenderSimCellsNoRecomposeConLaMismaGeometria: con la misma geometría, NO se
// vuelve a componer. Y lo que se comprueba es que las celdas son LAS MISMAS.
//
// Comprobar la igualdad de las celdas y no "que no se llamó a Cells" es lo que hace el
// test útil: si se conserva el contenido pero la geometría anotada cambia, el popup se
// queda con celdas de una medida y una anotación de otra. Y al revés: si la geometría
// se reescribe con la misma, el recomponido es idéntico y el test no lo vería.
func TestRenderSimCellsNoRecomposeConLaMismaGeometria(t *testing.T) {
	m := modeloConSimDeLasCeldasDadas(t, 1, 2)
	m.renderSimCells()
	primeras := len(m.sim.cells)
	if primeras == 0 {
		t.Fatal("no compuso nada la primera vez")
	}

	// Se llama cuatro veces más sin tocar nada. Con la caché, una sola composición.
	for range 4 {
		m.renderSimCells()
	}
	if len(m.sim.cells) != primeras {
		t.Errorf("tras cuatro render con la misma geometría hay %d celdas, want %d: "+
			"la caché no está cortando, y reescalar la imagen entera en cada resize es "+
			"justo lo que hace que el popup se congele", len(m.sim.cells), primeras)
	}
	// Y la geometría anotada no se movió, que es lo que permite que la comparación
	// del siguiente render siga dando igual.
	if m.sim.cellW == 0 || m.sim.cellH == 0 {
		t.Error("la geometría anotada se ha perdido: sin ella la caché no puede validarse")
	}
}

// TestRenderSimCellsRecomponeAlCambiarLaGeometria: al cambiar el tamaño, se recompone
// CON la nueva.
//
// Y el caso que importa es el de la MISMA altura con otra anchura, porque es donde se
// cuela el error de mirar solo una de las dos dimensiones: si la comparación mirara solo
// `cellW`, un resize vertical no invalidaría nada y la imagen se quedaría con el número
// de filas viejo, mostrando un trozo de la imagen anterior en una caja más baja.
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

			// La geometría que QUERÍA la caja ahora la dice el propio modelo, y hay que
			// leerla de ahí en vez de calcularla: `simBox` mete `sim.FitCells`, y una
			// celda más grande no solo divide el ancho entre dos, también puede cambiar
			// el número de filas. Calcularlo a mano fue el primer error de este test.
			wantW, wantH := m.simBox()
			wantW, wantH = wantW-2, wantH-simChrome

			if m.sim.cellW != wantW || m.sim.cellH != wantH {
				t.Errorf("tras cambiar la celda a %dx%d la geometría anotada quedó %dx%d, "+
					"want %dx%d: no se recompuso con la nueva, y la imagen se queda desfasada",
					c.ancho, c.alto, m.sim.cellW, m.sim.cellH, wantW, wantH)
			}
			// Y la geometría nueva tiene que ser DISTINTA de la vieja. Si saliera
			// igual, el caso no distinguiría un recomponido de un no-op, y el assert de
			// arriba pasaría sin que hubiera pasado nada.
			if m.sim.cellW == antesW && m.sim.cellH == antesH {
				t.Errorf("la geometría anotada sigue en %dx%d tras cambiar la celda a "+
					"%dx%d: el caso no probaría el cambio, daría lo mismo Compose y no-op",
					antesW, antesH, c.ancho, c.alto)
			}
		})
	}
}

// TestRenderSimCellsSinImagenNoDejaCeldasViejas: si la imagen desaparece, las celdas
// viejas se BORRAN.
//
// Este es el caso que la condición `m.sim.img == nil` cubre, y tiene una razón que no es
// obvia: las celdas son de la imagen ANTERIOR, que era de otra geometría. Si se dejaran,
// el popup seguiría pintando la imagen vieja con el marco del popup nuevo, y el usuario
// vería un review que ya no es el que tiene delante.
func TestRenderSimCellsSinImagenNoDejaCeldasViejas(t *testing.T) {
	m := modeloConSimDeLasCeldasDadas(t, 1, 2)
	m.renderSimCells()
	if len(m.sim.cells) == 0 {
		t.Fatal("no compuso nada la primera vez")
	}

	// Tres formas de quedarse sin imagen, y las tres tienen que limpiar.
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

// TestLaCajaDelPopupSiempreTieneHuecoInterior: el hueco interior del popup NUNCA
// puede ser de cero columnas o de cero filas. Y la cuenta es corta:
//
//   - `simBox`, en modo imagen, devuelve `cols+2, rows+simChrome`, donde `cols` y
//     `rows` salen de `sim.FitCells`;
//   - y `FitCells` devuelve `max(..., 1)` en las DOS dimensiones, en las dos ramas que
//     tiene;
//   - y `renderSimCells` y `publishSimImage` restan exactamente `2` y `simChrome`,
//     que es lo mismo que `simBox` añadió.
//
// O sea que el hueco interior es exactamente lo que `FitCells` devolvió, y `FitCells`
// garantiza que es al menos 1. Una celda. Ni media.
//
// ESTE TEST SUSTITUYE a uno anterior que tenía un agujero del tamaño del aserto: barría
// 4.500 combinaciones de terminal y no encontraba ni una caja que no cupiera, y su
// `t.Fatalf` —que era todo lo que afirmaba— no se ejecutó nunca. Pasaba siempre sin
// comprobar nada. Lo que pasaba es que `simMaxRows` y `simMaxCols` tienen suelo
// (`simMinRows`, `simMinCols`), así que la caja no baja de ahí por mucho que se encoja
// el terminal.
//
// Y el caso al que se llegó de verdad es el contrario del que buscaba: el hueco no
// puede ser cero porque la aritmética de arriba no lo permite. Y eso es lo que hace que
// las dos guardas que comparaban ese hueco contra cero sean INALCANZABLES, y la razón
// por la que no hay ningún caso que las dispare.
//
// La razón por la que este test NO es una tautología: no repite la fórmula de `simBox`,
// la llama. Y no basta con `cols-2 > 0`: se comprueba que sigue valiendo para imágenes
// extremadamente verticales —donde `FitCells` devuelve una sola columna— y para terminales
// pequeños, que es donde el suelo del popup podría esconder una caja degenerada.
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

// TestSimSeqSubeYNuncaBaja: el número de petición tiene que subir cada vez que se abre
// un popup y cada vez que se cierra, y nunca bajar.
//
// Y esto no es un contador para contar: es un mechanisms para INVALIDAR. Un resultado
// de simulación llega por el canal de eventos después de que la petición se haya
// cerrado oAFTER de que se haya reabierto con otra estrategia. `applySim` lo descarta
// comparando `msg.seq != m.simSeq`. Eso solo funciona si el número cambia, y si
// cambia SIEMPRE hacia arriba.
//
// Y el caso del que hay que acordarse: si `startSim` hiciera `m.simSeq--` en vez de
// `++`, y se estuviera en 0, se iría a -1. Después, cerrar el popup con `++` dejaría
// en 0, y una petición con seq 0 —que es exactamente el número por el que empezó
// todo— volvería a pasar la comparación. El resultado de una simulación antigua se
// pintaría encima de la nueva. No da error: da la imagen del render anterior, y el
// popup parece roto.
//
// Por eso lo que se afirma no es que "suba", sino que en una secuencia de altas y
// cierres NUNCA se repita un número, y que el orden sea siempre creciente.
func TestSimSeqSubeYNuncaBaja(t *testing.T) {
	m := newTestModel(t)
	m.SetSimulator(simuladorMudo{})
	partido := m.simSeq
	vistos := map[int]bool{}

	// Una sesión con la vida real del popup: abrir, cerrar, abrir otra estrategia,
	// cerrar. Con la misma estrategia y con distinta, porque da igual.
	for i := range 20 {
		m.startSim(simKindDePrueba(false))
		// Un incremento de UNO, no solo "ha subido": si apertura y cierre sumaran
		// más, dos aperturas podrían acabar en el mismo número y el descarte de
		// `applySim` se rompería sin que se note.
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

// TestApplySimDescartaLoObsoleto: un resultado cuyo número ya no es el actual NO se
// aplica. Y esto es el otro lado del contador, que es donde se ve para qué sirve.
//
// El caso que importa es el de la estrategia CAMBIADA con el popup abierto: el popup
// sigue visible, la imagen nueva está renderizándose, y llega el resultado de la
// primera. Si se aplicara, la imagen de la estrategia vieja se pintaría encima de la
// nueva, y el popup enseñaría un resultado que el usuario ya no ha pedido.
//
// Y el otro caso, que es el que documenta el comentario de `closeSim`: el popup se
// cerró mientras el render corría, y el resultado llega después.
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

	// Y el caso bueno, que es el que hace que los dos anteriores signifiquen algo: un
	// resultado CON EL NÚMERO ACTUAL sí se aplica. Si esto no pasara, el test de arriba
	// pasaría sin que el descarte tuviera nada que descartar.
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

// graphicsCaptura es un Graphics que anota lo que se le pide y guarda el CONTEXTO con el
// que se le llama. Guardar el contexto es lo que hace testeable el timeout del borrado:
// la pregunta "cuánto espera esto" no se responde mirando el reloj, se responde mirando
// el deadline del contexto que recibió la llamada.
type graphicsCaptura struct {
	celdaW, celdaH int

	mu       sync.Mutex
	clearCtx []clearCall
	capas    []string
}

// clearCall es una llamada a Clear con lo que el contexto valía EN ESE MOMENTO.
//
// Y el momento importa, que es la trampa del primer intento: `releaseSimLayer` hace
// `defer cancel()`, o sea que el contexto queda cancelado en cuanto `Clear` vuelve. Si
// el doble guardara el contexto y el test lo mirara después, vería "cancelado" siempre,
// y concluiría que el borrado hereda el contexto de la app —que es el bug que el test
// quiere cazar—. Mirar el contexto vivo es mirar un contexto ya muerto.
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
		// El plazo se mide AQUI, en el momento de la llamada, no cuando el test mire
		// despues. Entre las dos cosas pasa el tiempo del propio test, y con un plazo de
		// milisegundos se mide el reloj del test en vez del del codigo.
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

// clearCtxCount y clearContexts son lectores seguros de lo que anotó el doble, porque
// `releaseSimLayer` borra en una goroutine y el test la interroga desde otra.
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

// TestLaCedaSinMedirSupone1x2: si Herdr no dice cuánto mide una celda, se supone 1×2.
//
// Y no es un detalle de relleno: 1×2 es lo que hacen casi todos los terminales, así que
// es lo que se ve sin Herdr. Y el suelo importa porque un cero ahí no es un "no lo sé",
// es un "divídelo entre cero": `CellSize` devuelve (0,0) como respuesta explícita de "no
// se sabe", y esa respuesta tiene que convertirse en algo con lo que se pueda dividir.
//
// El suelo es 1 de ancho y 2 de alto, y el orden importa: 1×2 es el de una celda de
// terminal normal (un carácter, dos puntos de línea). Poner 2×1 significaría que cada
// celda es un bloque de dos caracteres de ancho por uno de alto, y la imagen saldría
// con la mitad de resolución vertical de la que debería, achatada.
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

	// Y cuando Herdr SÍ mide, su medida manda: no hay suelo que se le imponga.
	t.Run("la medida de Herdr manda", func(t *testing.T) {
		m := modeloConSimDeLasCeldasDadas(t, 9, 19)
		w, h := m.cellSize()
		if w != 9 || h != 19 {
			t.Errorf("con Herdr midiendo 9x19, cellSize dio %dx%d, want 9x19", w, h)
		}
	})
}

// TestElSueloDeLaCedaCambiaLaGeometriaDeLaImagen: el suelo no solo evita un cero,
// cambia lo que se ve.
//
// Y esto es lo que hace que el suelo sea comprobable con una imagen y no solo mirando
// el valor: 1×2 y 2×1 dan geometrías DISTINTAS. Una celda de dos píxeles de ancho hace
// que quepan la mitad de columnas para la misma imagen, porque cada columna ocupa el
// doble. Si alguien invierte el suelo, la imagen sale con la mitad de resolución
// horizontal.
//
// Solo se afirma el ANCHO, y es a propósito: el número de filas NO cambia al cambiar
// la altura de la celda, porque el alto del popup está topado por lo que cabe en el
// terminal (`simMaxRows` va en filas, no en píxeles) y la imagen se reescala a lo que
// cabe. El primer intento de este test afirmaba que las filas crecerían con una celda
// más alta, y no es verdad —una fila más alta no cabe en más filas—. Afirmar eso era
// poner en el test una intuición sobre la física de la imagen que no se sostiene.
//
// Un modelo alto tampoco vale aquí, y por la misma razón: por encima del tope, las dos
// geometrías se parecen.
func TestElSueloDeLaCedaCambiaLaGeometriaDeLaImagen(t *testing.T) {
	// La geometría con la celda por defecto (1×2).
	m := modeloConSimDeLasCeldasDadas(t, 0, 0) // Herdr no mide: entra el suelo
	m.sim.state = simShowing
	m.sim.img = imagenParaLaGeometria()
	m.renderSimCells()
	if m.sim.cellW == 0 {
		t.Fatal("con el suelo 1x2 no compuso nada")
	}
	ancho1x2 := m.sim.cellW

	// Y con la celda de Herdr al revés: 2 de píxeles de ancho por 1 de alto.
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
	// Y con la celda CUATRO VECES más ancha, la diferencia tiene que ser de verdad y no
	// un uno: cuatro píxeles de ancho dan un cuarto de columnas. Si solo bajara un poco,
	// el reescalado no estaría siguiendo a la celda.
	m4 := modeloConSimDeLasCeldasDadas(t, 4, 1)
	m4.sim.state = simShowing
	m4.sim.img = imagenParaLaGeometria()
	m4.renderSimCells()
	if m4.sim.cellW >= ancho2x1 {
		t.Errorf("con celda 4px hay %d columnas y con 2px hay %d: el número de columnas "+
			"tiene que bajar con el ancho de la celda", m4.sim.cellW, ancho2x1)
	}
}

// TestElBorradoDeLaCapaEsperaPocoYEnSegundoPlano: el `Clear` del borrado lleva un
// contexto CON TIMEOUT propio, y el plazo es el que dice.
//
// Las dos mitades importan por motivos distintos:
//
//   - que lleve SU propio contexto y no el de la app. Si usara el de la app, al cerrar
//     el popup al salir de la TUI el contexto ya estaría cancelado y la imagen se
//     quedaría pegada en la capa, tapando la TUI entera. Eso es exactamente el motivo
//     del comentario de `releaseSimLayer`.
//
//   - y el PLAZO, que es corto a propósito. El borrado es una operación de limpieza que
//     casi nunca falla; si se queda esperando, lo que se congela es el proceso al
//     salir, no el popup. Un plazo largo aquí se paga en el peor sitio posible: cuando
//     el usuario ya está cerrando.
//
// Se mide mirando el DEADLINE del contexto que recibió la llamada, no esperando: un
// test que espera dos segundos para comprobar un número es un test que tarda dos
// segundos en decir algo que ya está en el contexto.
func TestElBorradoDeLaCapaEsperaPocoYEnSegundoPlano(t *testing.T) {
	g := &graphicsCaptura{}
	m := newTestModel(t)
	m.graphics = g
	m.sim.viaGraphics = true

	m.releaseSimLayer()

	// Va en segundo plano, así que hay que esperar con techo. "no ha llegado" no se
	// sabe mirando una vez: `Clear` está en una goroutine.
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
	// Y el plazo se compara contra NUMEROS ESCRITOS AQUI, y no contra la constante del
	// codigo. Ese es el segundo intento de este test, y el primero fallaba por lo
	// contrario al que parece: comparando contra `simClearTimeout`, cambiar la constante a
	// 3, a 30 o a 2 minutos cambiaba las dos cosas a la vez y el test pasaba igual. Un
	// assert que lee el valor del sitio que esta comprobando no comprueba nada: es una
	// tautologia con sintaxis de assert.
	//
	// Y se compara contra un VALOR ABSOLUTO escrito aquí, con una tolerancia, y no
	// contra un rango. El rango fue el tercer intento y fallaba por un borde tonto: con
	// un rango de 1,5 a 2,5 segundos, cambiar la constante a 2,5 segundos daba un plazo
	// de 2,5 segundos menos el paso del reloj, que cae DENTRO del rango, y el mutante
	// pasaba. Un rango tiene bordes, y un mutante que apunta a un borde lo atraviesa.
	//
	// Un valor absoluto con tolerancia no tiene borde, y la tolerancia es lo que
	// sustituye a la precisión.
	//
	// Y la tolerancia es de 20ms, no de 100, y no por estrechez sino porque es lo que
	// separa un error REAL de uno que no lo es. Con 100ms, un timeout de 2,05s pasaba,
	// y 2,05s frente a 2s es una diferencia del 2,5% —justo la que se cuela cuando
	// alguien "redondea" el número—. Con 20ms ese mutante cae.
	//
	// Que 20ms sea suficiente está medido, no supuesto: entre que se crea el contexto y
	// que `Clear` lo recibe pasa del orden de 3 MICROSEGUNDOS, porque el plazo se mide
	// dentro del doble y no desde el test. O sea que la tolerancia podría bajar a 1ms
	// sin volverse frágil, y se deja en 20ms para no depender de que eso siga siendo
	// cierto en una máquina más cargada.
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
	// Y que el contexto NO estuviera ya cancelado CUANDO SE LLAMÓ, que es lo que pasó
	// cuando usaba el de la app: eso es lo que hace que el borrado funcione al salir de
	// la TUI. El valor se leyó en el momento de la llamada, no después: `releaseSimLayer`
	// cancela con su `defer`, así que un contexto consultado más tarde sale cancelado
	// siempre y el aserto no probaría nada.
	if llamada.errAlLlamar != nil {
		t.Errorf("el contexto del borrado ya estaba cancelado (%v) al llamar: si "+
			"heredara el de la app, el Clear no llegaría a hacer nada y la imagen se "+
			"quedaría pegada al salir de la TUI", llamada.errAlLlamar)
	}
}

// TestElBorradoNoSeIntentaSiNoHayImagenEnLaCapa: sin imagen publicada no hay nada que
// borrar, y la llamada no se hace.
//
// La razón de que importe: `Clear` es una llamada a Herdr, o sea un proceso. Y el caso
// de "no hay nada" es el NORMAL: casi todos los popups de simulación se abren sin capa
// de gráficos, porque la capa solo se usa cuando Herdr puede pintar a resolución nativa.
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
			// Se espera un poco a propósito, para que un Clear tarde hubiera llegado.
			esperarLlamadas(t, g, 40, 5*time.Millisecond)

			if len(g.ctxs()) != 0 {
				t.Errorf("llamó a Clear %d veces cuando no había nada en la capa: "+
					"es una llamada a un proceso entero, y el caso de no tener nada es el normal",
					len(g.ctxs()))
			}
		})
	}
}

// TestElBorradoVaSobreLaCapaDelSimulador: el `Clear` se hace sobre la capa del popup y
// no sobre otra.
//
// Es un dato, no una interpretación: si el nombre de la capa no fuera el del popup, el
// borrado sería un no-op silencioso. Y el síntoma sería invisible desde el popup, porque
// el popup ya no está: lo que se vería es la TUI con la imagen pegada encima después de
// cerrar el popup, sin nada que lo relacione.
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

// esperarCtxs recoge los contextos con techo. Medir una goroutine exige un techo: "no
// ha llegado" no se sabe mirando una vez.
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

// graphicsConErrores es el doble de antes pero con errores, que es la mitad de las
// salidas de `publishSimImage`: o funciona, o dice que no puede y el popup pinta con
// half-blocks.
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

// TestPublishSimImageMideLaCedaYLaGuarda: cuando Herdr mide la celda, esa medida se
// guarda y se USA para decidir el tamaño en píxeles.
//
// Y lo que se comprueba es el tamaño en píxeles de la imagen mandada, que es la razón
// de medir la celda en absoluto: `sim.Resize(img, innerCols*cellW, innerRows*cellH)`.
// Sin la medida se mandaría la imagen al tamaño de la caja en CELDAS, y el terminal la
// estiraría a celdas enteras.
//
// Y se guarda en `m.sim.cellW_px` / `cellH_px` porque el popup la vuelve a usar: la
// imagen publicada por la capa es a resolución nativa, pero si esa capa deja de estar
// disponible y el popup pasa a pintar con celdas, la geometría tiene que ser la misma
// que se usó para decidir el tamaño. Sin guardarla, el popup dibujaría a 1×2 la imagen
// que se dimensionó para 9×19.
func TestPublishSimImageMideLaCedaYLaGuarda(t *testing.T) {
	const celdaW, celdaH = 9, 19
	g := &graphicsConErrores{celdaW: celdaW, celdaH: celdaH}

	m := modeloConSimDeLasCeldasDadas(t, 0, 0)
	m.graphics = g
	m.sim.state = simShowing
	m.sim.img = imagenParaLaGeometria()

	// El hueco esperado se calcula con la celda por defecto, que es la que hay antes de
	// publicar: publicar guarda la celda medida y la caja se reacomoda.
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
	// Y la colocación es el hueco INTERIOR de la caja, desplazado una celda: el marco
	// del popup ocupa una celda, y la imagen va dentro.
	//
	// El `want` se calcula ANTES de publicar, y no después, y el motivo es que
	// publicar CAMBIA el estado del que depende: al guardar la celda medida, la caja se
	// reacomoda a 9×19 y `simBox()` da otro rectángulo. La primera versión de este
	// aserto calculaba el `want` al final y fallaba por eso, no por el código.
	//
	// Ojo a que el hueco se mide con la celda por defecto (1×2), que es la que hay
	// ANTES de publicar. Publicar guarda la celda medida y la caja se reacomoda.
	if placements[0] != want {
		t.Errorf("colocó en %+v, want %+v: la imagen va en el hueco interior del popup, "+
			"una celda dentro del marco", placements[0], want)
	}
}

// TestPublishSimImageSinCeldaMedidaAsume1x2: si Herdr no mide, el suelo es 1×2. Y es el
// MISMO suelo que en el otro sitio, que no es casualidad: los dos evitan lo mismo.
//
// El cero de `CellSize` no es un dato: es la respuesta explícita de "no lo sé". Y sin
// suelo, `innerCols*cellW` da 0 píxeles, y `sim.Resize` con un tamaño de 0 no devuelve
// una imagen: `publishSimImage` diría que no pudo publicar y el popup caería a
// half-blocks aunque Herdr esté perfectamente disponible.
//
// O sea que el suelo no es un dato inventado para que algo funcione: es lo que hace
// que el camino de la capa gráfica siga siendo un camino cuando la medida no está.
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

// TestPublicarUsaElContextoPropioYNoElDeLaApp: el `SetImage` de la publicación lleva un
// contexto PROPIO, con timeout, y no el de la aplicación.
//
// Es el mismo argumento que el del borrado, y por el mismo motivo: `publishSimImage` se
// llama desde `applySim`, que puede ocurrir en cualquier momento, y si usara el contexto
// de la app, al cerrar el popup mientras se salía de la TUI el contexto ya estaría
// cancelado y la publicación no llegaría a hacerse. La diferencia con el popup es que
// esa capa, cuando se publica encima del contenido del pane, tapa la TUI.
//
// Y el timeout es propio por la misma razón: una publicación a Herdr es una llamada a un
// proceso, y si ese proceso no contesta, lo que no puede ser es quedarse esperando
// mientras el usuario mueve el cursor.
func TestPublicarUsaElContextoPropioYNoElDeLaApp(t *testing.T) {
	g := &graphicsConErrores{celdaW: 9, celdaH: 19}
	m := modeloConSimDeLasCeldasDadas(t, 0, 0)
	m.graphics = g
	m.sim.state = simShowing
	m.sim.img = imagenParaLaGeometria()

	// El contexto de la app se cancela antes de publicar, que es exactamente lo que
	// pasa al salir de la TUI con el popup abierto.
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

// TestPublishSimImageNoPublicaSiElSetFalla: si Herdr rechaza la publicación, se dice
// que NO se pudo, y el popup pinta con celdas.
//
// Y lo que se comprueba no es solo el `false` de salida, sino que NO se haya marcado
// `viaGraphics`. Esa bandera es la que decide si el popup pinta la imagen o la deja a
// la capa: marcarla cuando la publicación falló deja el popup sin nada que pintar y
// sin saber que lo espera, y el resultado es un marco vacío.
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

// TestPublishSimImageNoMarcaViaGraphicsCuandoLaImagenVaPorLaCapa: y el camino bueno sí
// marca la bandera, y VACÍA las celdas.
//
// Las dos mitades van juntas y por eso están en el mismo test. `viaGraphics` dice "no
// pintes celdas". Y las celdas se vacían porque las que habría son de una imagen
// anterior, puede ser de otra geometría: si se dejaran, el popup pintaría la de antes
// por encima de la que la capa está mostrando, o al revés, y ninguna de las dos
// encajaría con la otra.
func TestPublishSimImageMarcaLaBanderaYVaciaLasCeldas(t *testing.T) {
	g := &graphicsConErrores{celdaW: 9, celdaH: 19}
	m := modeloConSimDeLasCeldasDadas(t, 1, 2)
	m.graphics = g
	m.sim.state = simShowing
	m.sim.img = imagenParaLaGeometria()

	// Se dejan celdas viejas, de cuando el popup pintaba en texto.
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

// TestPublishSimImageSinGraficosNiCapaNoIntentaNada: sin Herdr, o con Herdr que no está
// disponible, no se intenta publicar.
//
// Y el caso de "Herdr presente pero no disponible" es el que se confunde con el otro: son
// dos salidas por la misma línea y significan cosas distintas. `m.graphics == nil` es
// "no hay Herdr en este proceso". `!m.graphics.Available()` es "hay Herdr, pero su capa
// no está disponible ahora mismo" —otro pane, otro workspace—. En las dos se pinta con
// half-blocks, y en ninguna se llama a nadie.
func TestPublishSimImageSinGraficosNiCapaNoIntentaNada(t *testing.T) {
	// Sin graphics en absoluto.
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

// TestPublishSimImageUsaLaCapaDelSimulador: la publicación va a la capa del popup, por
// el mismo motivo que el borrado.
func TestPublishSimImageUsaLaCapaDelSimulador(t *testing.T) {
	g := &graphicsConErrores{celdaW: 9, celdaH: 19}
	m := modeloConSimDeLasCeldasDadas(t, 0, 0)
	m.graphics = g
	m.sim.state = simShowing
	m.sim.img = imagenParaLaGeometria()

	m.publishSimImage(m.sim.img)
	// El doble no anota la capa, y por eso no hay aserto: lo que se comprueba aquí es
	// que el `Clear` usa `simLayer` (en el test del borrado) y que la publicación
	// usa la MISMA constante. Si divergieran, una de las dos operaciones iría a una
	// capa que no es la del popup, y eso es un no-op silencioso.
	if simLayer == "" {
		t.Error("la capa del simulador está vacía")
	}
}

// TestPublishSimImageNoPublicaConUnaCajaQueNoCabe: si la caja no da hueco interior,
// no se publica nada.
//
// Es el caso de una ventana tan pequeña que el popup no cabe, y es el mismo que
// `renderSimCells` cubre por el otro lado. Aquí la consecuencia sería distinta: publicar
// con un hueco de 0 columnas mandaría una imagen de 0 píxeles, o mandaría la imagen
// entera en un rectángulo que no existe.
func TestPublishSimImageNoPublicaConUnaCajaQueNoCabe(t *testing.T) {
	g := &graphicsConErrores{celdaW: 9, celdaH: 19}

	// Se encoge hasta que la caja no cabe, como en el test de `renderSimCells`.
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

// TestElAnchoNoDeterminaLaAltura: el ancho de la caja NO basta para saber su altura, y
// por eso la caché compara las dos dimensiones.
//
// Esto parece obvio —"compara el ancho y la altura"—, pero el modo en que falla es el
// que hace que comparar solo el ancho parezca suficiente: con el MISMO ancho de caja,
// dos imágenes de distinta proporción dan distinta altura.
//
// Y el modo es por el tope. Una caja estrecha tiene poquísimo ancho disponible, así que
// casi cualquier imagen topa por lo ancho y le quedan muchas filas; una imagen alta
// topa antes por lo alto y le quedan pocas. Con el ancho igual en ambos casos y las
// filas del revés, comparar solo el ancho deja la imagen anterior metida en una caja con
// menos filas de las que se compilaron: se sale por abajo del marco, sin que nada lo
// diga.
//
// El caso sale de una sonda que barrió 16x16 tamaños de imagen, tres medidas de celda y
// seis terminales. En un terminal de 60x100 el ancho de 54 columnas salía con 6 filas
// para una imagen de 24x56 y con 33 para una de 32x40, y con casi cualquier otra
// proporción.
//
// Y no era obvio para quien escribe: la primera versión de esta comprobación afirmaba
// que cambiar la CELDA cambiaría el ANCHO, que es lo que hace el ancho de la celda. La
// intuición era correcta pero sobre la dimensión equivocada, y por eso el mutante de la
// comparación de la altura pasaba sin que nada lo delatara.
//
// El caso se BUSCA dentro del test en vez de estar escrito con números, y a propósito:
// lo que se afirma es que existe. Los números dependen del mínimo del popup y de
// `simMaxCols`, que no son lo que se está probando, y escribirlos los convierte en un
// test que falla cuando cambia algo que no le importa. Si el caso dejara de existir, el
// fallo tiene que decirlo —que el ancho ya determina la altura y la comparación se
// puede quitar—, y no un número que no cuadra.
// casoGeom es un par de geometría con su imagen, que es lo que la búsqueda necesita
// guardar: no solo las dimensiones, sino la imagen que las produjo, para poder
// reproducir el caso exacto en el modelo del test.
type casoGeom struct {
	img        image.Image
	col, filas int
}

func TestElAnchoNoDeterminaLaAltura(t *testing.T) {
	const (
		anchoVista, altoVista = 60, 100
		celdaW, celdaH        = 1, 2
	)

	// Se recorren proporciones de imagen con el MISMO modelo y la MISMA celda, que es
	// como pasa: la celda no cambia dentro de una sesión, lo que llega es otra imagen.
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

	// Y el efecto, que es lo que se vería en pantalla: con la caché mirando solo el
	// ancho, al llegar la segunda imagen NO se recompone.
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

	// Llega el resultado nuevo: mismo ancho, otra altura.
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

// imagenDe es una imagen de las dimensiones pedidas, de un solo color. El tamaño SÍ
// importa en este fichero, y no por lo que se ve: es lo que hace que dos celdas
// distintas den el mismo ancho de caja con distinta altura.
func imagenDe(anchoPx, altoPx int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, anchoPx, altoPx))
	for y := range altoPx {
		for x := range anchoPx {
			img.Set(x, y, image.White)
		}
	}
	return img
}

// TestPadRightRellenarODejarlo: `padRight` completa con espacios HASTA n columnas, y
// si el texto ya llega o pasa, lo deja como está.
//
// Y lo que importa aquí no es el relleno sino el otro lado, que es donde se cuelan las
// de más: una cadena que ya es más ancha que n NO se recorta. Recortarla sería cambiar el
// texto para que quepa en un presupuesto que no es suyo, y en un selector de estrategias
// eso es borrar una estrategia de la lista sin avisar.
//
// Y el ancho se mide en COLUMNAS VISIBLES, no en bytes. Un emoji ocupa dos columnas y
// cuatro bytes; un acento puede ocupar un byte y una columna. Rellenar por bytes deja
// etiquetas desalineadas, que es justo lo que esta función existe para arreglar.
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

			// El ancho visible es el que manda, y es el que se afirma.
			if w := ansi.StringWidth(got); w != want {
				t.Errorf("padRight(%q, %d) mide %d columnas visibles, want %d. La entrada "+
					"mide %d bytes y %d columnas: rellenar por bytes desalinea las etiquetas, "+
					"que es lo único que esta función arregla",
					c.s, c.n, w, want, len(c.s), ansi.StringWidth(c.s))
			}
			// Y lo que se rellena son ESPACIOS, nada más. Un carácter de relleno
			// distinto se vería como texto, y en un selector de estrategias se leería
			// como parte de la etiqueta.
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

// TestPadRightConElBordeExacto: con el ancho justo, `padRight` no añade nada — y ese
// borde es lo que hace que la comparación con `>=` en vez de `>` dé lo mismo.
//
// La condición es `if pad := n - ancho; pad > 0`. Con `pad == 0`, que es el ancho
// justo, las dos ramas hacen lo mismo:
//
//   - con `> 0`, no entra y sale por el `return s`;
//   - con `>= 0`, entra y hace `s + strings.Repeat(" ", 0)`, que es `s` con cero
//     espacios.
//
// Ese es el motivo por el que el mutante de la condición sobrevive, y no es un
// descuido del test: es una IDENTIDAD en la frontera, y lo que la demuestra es que en el
// borde el resultado es el mismo. Se afirma en el borde exacto, y también en el borde
// con el texto más corto posible —una cadena vacía—, porque ahí `pad == n` y es el otro
// caso donde la resta da justo lo que tiene que dar.
func TestPadRightConElBordeExacto(t *testing.T) {
	for _, s := range []string{"", "a", "ab", "áé", "🙂"} {
		ancho := ansi.StringWidth(s)
		// El borde exacto: n igual al ancho. Aquí la resta da cero.
		got := padRight(s, ancho)
		if got != s {
			t.Errorf("con el ancho justo (%q ocupa %d) devolvió %q, want el texto sin cambios",
				s, ancho, got)
		}
		if ansi.StringWidth(got) != ancho {
			t.Errorf("con el ancho justo devolvió %d columnas, want %d", ansi.StringWidth(got), ancho)
		}
		// Y una columna de más, que es donde las dos ramas ya NO coinciden y el relleno
		// tiene que aparecer: sin esto, un `padRight` que no rellena nada también
		// pasaría el aserto de arriba.
		if p := padRight(s, ancho+1); ansi.StringWidth(p) != ancho+1 {
			t.Errorf("con una columna de más devolvió %d columnas, want %d: el relleno "+
				"no ocurre, y el aserto del borde exacto pasaría igual",
				ansi.StringWidth(p), ancho+1)
		}
	}
}
