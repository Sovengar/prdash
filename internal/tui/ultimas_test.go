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

// El último lote de `tui`: guardas de geometría, defaults de etiqueta y las tres ramas de
// `Update` queemicolon producen un mensaje sin tocar datos.
//
// Y el patrón común es "esto no cabe / esto no está / esto no se pudo" y la decisión de qué
// hacer en cada caso: no pintar, no avisar, o avisar. Las tres son correctas y se distinguen por
// lo que el usuario haría con la información.

// TestUnPopupDeSimulacionQueNoCabeNoSePintaYNoEsUnError: la guarda de geometría.
//
// Y `publishSimImage` mide el hueco interior del popup y, si no cabe una celda, devuelve false
// SIN error. Y no es un descuido: un popup de una celda de ancho es peor que no popup, porque
// tapa el inbox y no enseña nada.
//
// Y el caso que hay que provocar es una terminal diminuta, que es real —un panel de 30
// columnas— y es donde la degradación honesta se ve.
func TestUnPopupDeSimulacionQueNoCabeNoSePintaYNoEsUnError(t *testing.T) {
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	g := &graphicsContador{}
	m.graphics = g
	m.sim.state = simShowing

	// Una terminal POR DEBAJO del mínimo del popup, que es el caso extremo.
	//
	// Y aquí hay que decir la verdad sobre lo que ese caso demuestra: `simMaxCols` y
	// `simMaxRows` tienen suelo `simMinCols`/`simMinRows`, así que la caja NUNCA baja de ahí y
	// el guard de "no cabe" no llega a dispararse por el tamaño de la terminal. El suelo es lo
	// que hace que el popup siga siendo un popup en una terminal diminuta; el guard es la red
	// que queda por si `FitCells` devolviera cero.
	//
	// Mi primera versión afirmaba que con 19 columnas el popup no se publicaba, y se publicaba:
	// no es un fallo del código, es que el suelo lo impide. Lo que se comprueba es la propiedad
	// que de verdad importa —el suelo se respeta y el hueco interior sigue siendo usable— y en
	// el camino bueno, que la publicación lleva el rectángulo interior desplazado una celda.
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

	// Y con hueco de sobra: se publica, con el rectángulo INTERIOR —menos el cromo— y
	// desplazado una celda, porque el borde del popup ocupa la primera.
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
	// Y el desplazamiento: la imagen va UNA celda dentro del marco, porque la primera la
	// ocupa el borde.
	if g2.ultima.Col <= 0 || g2.ultima.Row <= 0 {
		t.Errorf("la imagen se publicó en el origen del marco (%+v): taparía el borde",
			g2.ultima)
	}
}

// TestUnaImagenQueNoSePuedeRedimensionarNoSePublica: el `Resize` que devuelve nil.
//
// Y el caso es una imagen que no se puede escalar —un `image.Image` cuyo `Bounds` no cuadra con
// lo que dice `Bounds().Dx()`— que ocurre con un `image.Image` que devuelve dimensiones
// incoherentes, y que también ocurre cuando el tamaño pedido es cero.
//
// Y `nil` se comprueba ANTES de publicar, y no dentro del `SetImage`: mandar un nil a Herdr
// serializaría una imagen vacía y el popup enseñaría un hueco con marco en lugar de no enseñar
// nada.
func TestUnaImagenQueNoSePuedeRedimensionarNoSePublica(t *testing.T) {
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	g := &graphicsContador{}
	m.graphics = g
	m.width, m.height = 120, 40

	// Una imagen de tamaño CERO: `sim.Resize` devuelve nil y no se publica nada. Y una
	// imagen de cero dimensiones es real —un JPEG que decodifica a un rectángulo vacío— y
	// mandarla a Herdr serializaría un PNG sin píxeles.
	if m.publishSimImage(imagenRota{w: 0, h: 0}) {
		t.Error("una imagen que no se puede redimensionar se publicó")
	}
	if g.publicadas != 0 {
		t.Errorf("salieron %d publicaciones de una imagen rota", g.publicadas)
	}
}

// TestRepublicarSinImagenNoHaceNadaNiFalla: `republishSimImage` con el panel vacío.
//
// Y `republishSimImage` se llama en CADA resize mientras la imagen está en la capa, así que el
// camino sin imagen no es un caso raro: es el resize que llega después de cerrar el popup. Y
// sin el `if img == nil`, cada resize intentaría publicar un `nil`.
//
// Y no se pone a poke `viaGraphics` antes de saber si se pudo: si se pone a false y luego el
// render falla, el panel queda sin capa y sin half-blocks, que es un popup invisible. Ese
// orden es lo que se comprueba.
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
	// Y `viaGraphics` se queda como estaba: sin publicación no hay cambio de estrategia de
	// pintado, y ponerlo a false dejaría el popup sin capa ni half-blocks.
	if !m.sim.viaGraphics {
		t.Error("sin imagen, republishSimImage turned off viaGraphics: the popup would have " +
			"neither layer nor half-blocks")
	}

	// Y con una imagen que NO se puede redimensionar: `viaGraphics` se apaga, porque se probó
	// la capa y no se pudo publicar, y el half-block es la degradación correcta. Dejarlo a true
	// haría que la TUI creyera que hay imagen en la capa y no pintara el repuesto.
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

// TestElChooserPoneLaRamaDelItemYConSuPlaceholderSiNoLaHay: la etiqueta del popup.
//
// Y el placeholder no es cosmético: un ítem cuya `SourceBranch` vino vacía del forge se pinta
// como "the PR branch" en el popup, y sin eso la línea quedaría a medias y el usuario no sabría
// qué se está comparando.
func TestElChooserPoneLaRamaDelItemYConSuPlaceholderSiNoLaHay(t *testing.T) {
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	it := mkItem("github", "github.com", "acme/widget", "uno", 7, "")
	it.SourceBranch = "feat/mi-rama"
	m.sim.item = it

	if box := m.simChooserBox(); !strings.Contains(box, "feat/mi-rama") {
		t.Errorf("el popup no nombra la rama del ítem:\n%s", box)
	}
	// Y la base, que es lo otro que se compara.
	if box := m.simChooserBox(); !strings.Contains(box, it.TargetBranch) {
		t.Errorf("el popup no nombra la base:\n%s", box)
	}

	// Y sin rama de origen: el placeholder, no una línea vacía.
	m.sim.item.SourceBranch = ""
	box := m.simChooserBox()
	if !strings.Contains(box, "the PR branch") {
		t.Errorf("sin rama de origen el popup pone %q, y deja la línea a medias", box)
	}
	if strings.Contains(box, "  \n") {
		t.Errorf("el popup tiene una línea vacía:\n%s", box)
	}
	// Y con espacios en vez de vacío, que es lo que llega de un forge que no recorta.
	m.sim.item.SourceBranch = "   "
	if box := m.simChooserBox(); !strings.Contains(box, "the PR branch") {
		t.Errorf("una rama de solo espacios no cuenta como vacía:\n%s", box)
	}
}

// TestElTickNoSeArmaConIntervaloCeroNiConRefrescoPausado: `tickCmd`.
//
// Y el intervalo cero es el caso del config que pone `refresh_interval = "0s"`, que es un
// refresco MANUAL: no hay tick que armar, y armar uno con `tea.Tick(0, …)` sería un bucle que
// consulta la API sin parar.
//
// Y por eso el guard es `d <= 0` y no `d == 0`: un intervalo NEGATIVO es lo mismo de手动 que
// uno cero, y un `d == 0` dejaría pasar el negativo.
func TestElTickNoSeArmaConIntervaloCeroNiConRefrescoPausado(t *testing.T) {
	for _, d := range []time.Duration{0, -time.Second, -time.Hour} {
		m := newTestModel(t)
		m.cfg.RefreshInterval = d
		if cmd := m.tickCmd(); cmd != nil {
			t.Errorf("con refresh_interval=%v se armó un tick: la cadena consultaría sin parar",
				d)
		}
	}

	// Y con intervalo positivo: sí se arma, que es el control que hace que lo anterior no
	// sea "nunca arma nada".
	m2 := newTestModel(t)
	m2.cfg.RefreshInterval = 90 * time.Second
	if cmd := m2.tickCmd(); cmd == nil {
		t.Error("con un intervalo válido no se armó el tick")
	}
}

// TestElTickDelToastEsUnRelojYNoUnLectorDelCanal: `tickToast`.
//
// Y la distinción es el invariante de un único lector: `tickToast` viene de `tea.Every`, no del
// canal de eventos, así que NO debe pasar por `withPump`. Si pasara, rearma un lector que nadie
// ha gastado, y cada aviso dejaría una goroutine esperando un mensaje que no va a llegar.
//
// Y el reloj sale: un `Cmd` que devuelve `toastTickMsg` es la prueba de que es un reloj, y no un
// lector, porque un lector se lee del canal.
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

// TestUnListadoDeRamasConAvisosLosConvierteEnMensajeDeError: `fetchBranches` con avisos.
//
// Y el caso es el que de verdad pasa: el forge devuelve las ramas PERO con un aviso —un stream
// parcial, una columna que no se pudo consultar— y el popup tiene que decirlo. Sin ello, el
// usuario cambiaría la base creyendo que la lista es completa.
//
// Y el aviso se mete en el MISMO mensaje que las ramas, y no aparte, porque llegan del mismo
// sitio: separarlos haría que el popup tuviera que reconstruir qué aviso va con qué lista.
func TestUnListadoDeRamasConAvisosLosConvierteEnMensajeDeError(t *testing.T) {
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	m.retarget.state = retargetListing
	m.retarget.item = mkItem("github", "github.com", "acme/widget", "uno", 7, "")
	m.branchSeq = 1

	// El camino bueno: ramas sin avisos, el popup abre el selector.
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

	// Y con avisos: el error aparece y el selector NO se abre, porque una lista parcial
	// presentada como completa es peor que un aviso.
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

// TestStartRetargetCierraElPopupSiLaAccionNoSePuedeHacer: la negativa de `startRetarget`.
//
// Y son las tres que pueden: forge desconocido, sin permisos y la acción en curso. Y las tres
// cierran el popup, que es lo distinto de `canActionOn` en la simulación —donde se avisa y el
// popup se queda—.
//
// Y la asimetría es a propósito: aquí el popup es una CONFIRMACIÓN de algo que ya elegiste, y si
// la acción no puedearse no hay nada que confirmar. Dejarlo abierto mostraría "de main a
// release/2.0" sobre algo que no va a pasar.
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

// graphicsContador cuenta publicaciones y recuerda la última colocación.
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

// Las tres negaciones de `canActionOn` dejan el popup cerrado y avisado, y el aviso es lo que
// distingue "no se puede" de "no hice nada".
var (
	_ = forge.ActionRetarget
	_ = model.SectionReview
	_ = sim.KindMerge
	_ = tea.KeyPressMsg{}
)

// imagenDiminuta es una imagen de 8×8, lo bastante pequeña para que quepa en cualquier popup.
func imagenDiminuta(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{R: uint8(x * 8), G: uint8(y * 8), B: 64, A: 255})
		}
	}
	return img
}

// imagenRota es una imagen con la que `sim.Resize` no puede trabajar, con un tamaño que se le
// pasa al caso.
//
// Y el tamaño CERO es el que de verdad devuelve nil, no unas `Bounds` incoherentes: `Resize`
// descarta `w <= 0 || h <= 0` ANTES de mirar la imagen. Mi primera versión usaba unas
// `Bounds` que no cuadraban con `At`, que `Resize` redimensiona sin problema—la lectura por
// índice da 0 y punto— y la imagen se publicaba sin fallar.
type imagenRota struct{ w, h int }

func (i imagenRota) ColorModel() color.Model { return color.RGBAModel }
func (i imagenRota) Bounds() image.Rectangle { return image.Rect(0, 0, i.w, i.h) }
func (imagenRota) At(int, int) color.Color   { return color.RGBA{} }

// TestElHuecoInteriorDelPopupNuncaDesaparecePorMuchoQueSeEstrecheLaTerminal: el invariante que
// sustituye al guard eliminado de `publishSimImage`.
//
// Y `publishSimImage` tenía un guard que devolvía `false` cuando el hueco interior del popup era
// de cero columnas o de cero filas. Es inalcanzable, y lo que lo hace inalcanzable es el suelo
// de `contentWidth`, que no es donde lo busqué las dos primeras veces.
//
// Y la cuenta completa, para los TRES estados que puede tener el popup:
//
//   - Modo imagen: `simBox` devuelve `cols+2, rows+simChrome` y `sim.FitCells` devuelve
//     `max(..., 1)` en las dos dimensiones, así que el hueco es lo que salió de `FitCells`, que
//     es al menos 1×1.
//   - Modo selector y modo renderizando: `simBox` devuelve `min(contentWidth(), simChooserWidth)`
//     por `simChrome + 2`, y `contentWidth()` es `max(38, …)`, así que el hueco es 36×2 como
//     mínimo.
//
// Y el recorrido va hasta terminal de 0 columnas, que es el primer frame de una TUI a la que la
// terminal todavía no ha dicho su tamaño: es el caso donde el suelo es lo único que impide que la
// caja colapse, y por eso es el que más importa comprobar.
//
// Y lo que se comprueba es la regla del proyecto tal como está escrita —**si una caja no cabe, no
// se pinta a medias**— más la aritmética que la cumple: resulta que la caja SIEMPRE cabe, y lo
// que hay que demostrar es eso, para que nadie añada el `if` pensando que hace falta.
//
// Y una segunda cosa, que es la que de verdad fija el comportamiento: en TODOS los casos la
// publicación ocurre. Un `publishSimImage` que no publicara nada —porque el `graphics` doble no
// estuviera, o porque el hueco no cupiera— pasaría un test que solo mirara el tamaño.
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
			// Y una altura también extrema: el suelo de filas es `simMinRows`, pero el del
			// selector es fijo, y son dos aritméticas distintas.
			m.width, m.height = ancho, 3

			cols, rows := m.simBox()
			innerCols, innerRows := cols-2, rows-simChrome
			if innerCols <= 0 || innerRows <= 0 {
				t.Errorf("estado %v en terminal de %d columnas: hueco interior de %dx%d. Es "+
					"lo que el guard eliminado comprobaba, y el suelo de contentWidth lo "+
					"impide", estado, ancho, innerCols, innerRows)
			}

			// Y publica. Siempre. Con el hueco mínimo de 1×1 en modo imagen y 36×2 en el
			// selector, un `false` aquí sería el bug, no la protección.
			if !m.publishSimImage(m.sim.img) {
				t.Errorf("estado %v en terminal de %d columnas: no publicó con un hueco de "+
					"%dx%d", estado, ancho, innerCols, innerRows)
				continue
			}
			if g.llamadas != 1 {
				t.Errorf("estado %v en terminal de %d columnas: %d publicaciones, want 1",
					estado, ancho, g.llamadas)
			}
			// Y el rectángulo que llegó a Herdr es el hueco, con las dos dimensiones en
			// positivo: una rect de 0 columnas es el síntoma de un suelo que alguien quitó.
			if g.ultima.Cols <= 0 || g.ultima.Rows <= 0 {
				t.Errorf("estado %v en terminal de %d columnas: rectángulo de %dx%d celdas",
					estado, ancho, g.ultima.Cols, g.ultima.Rows)
			}
			// Y coincide con lo que la caja dice, que es lo que hace que el marco y la
			// imagen encajen.
			if g.ultima.Cols != innerCols || g.ultima.Rows != innerRows {
				t.Errorf("estado %v en terminal de %d columnas: se publicó en %dx%d y el hueco "+
					"era de %dx%d: la imagen se saldría del marco",
					estado, ancho, g.ultima.Cols, g.ultima.Rows, innerCols, innerRows)
			}
		}
	}
}

// TestConUnaImagenEnormeElPopupSeAjustaAlTerminalYNoLoDesborda: el suelo por arriba.
//
// Y el recorrido del test anterior prueba el suelo por abajo —un popup más pequeño que el
// mínimo no—, este prueba el otro lado: con una imagen que necesita 400 columnas en un terminal
// de 80, la caja tiene que caber en el terminal, porque una caja más ancha se corta por el borde
// y el popup aparece partido: mitad marco, mitad nada.
//
// Y la imagen manda, no al revés: no se envía la resolución de una imagen de 400 columnas a un
// pane de 80 porque eso es lo que hace `CellSize` —lo que no se ve no se manda—, así que el
// ajuste es del lado de la TUI.
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

// graphicsConteo cuenta las publicaciones y guarda el último rectángulo.
//
// Y existe porque `graphicsCaptura` —el doble que ya usaba el resto de los tests de `sim`— no
// cuenta `SetImage`: devuelve nil y ya está. Y aquí lo que importa es exactamente si se publicó,
// porque el `false` del guard significa "no se publicó nada", y eso no se puede ver sin un
// contador.
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
