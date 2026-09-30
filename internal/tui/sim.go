// Overlay de simulación: un popup que elige el comando, lo renderiza con
// git-sim y enseña la imagen resultante encima del inbox.
//
// El popup vive en la misma vista que el resto, no en una pantalla aparte: la
// simulación es una consulta sobre el ítem que ya está seleccionado, y tapar la
// vista entera para preguntar durante un par de segundos por un grafo esconde
// justo lo que hay que estar mirando mientras se responde.
package tui

import (
	"context"
	"image"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"prdash/internal/forge/model"
	"prdash/internal/herdr"
	"prdash/internal/sim"
	"prdash/internal/tui/bordered"
)

// simTimeout acota la simulación completa. El runner tiene el suyo propio y más
// corto; este es la red que cubre también preparar el directorio de trabajo.
const simTimeout = 90 * time.Second

// simLayer es la capa donde se publica la imagen. Vive en el herdr porque la capa
// es un recurso del pane, pero el nombre lo elige prdash y lo borra al cerrar el
// popup, para no tocar nada que no sea suyo.
const simLayer = herdr.GraphicsLayer

// Geometría del popup.
const (
	// simChooserWidth es el ancho del selector: la estrategia, su flujo y la
	// ayuda. Ancho de más solo añade aire.
	simChooserWidth = 64
	// simChrome son las líneas del popup que no son imagen: borde de arriba,
	// borde de abajo y la de ayuda.
	simChrome = 3
	// simHeightNum y simHeightDen son la fracción de la altura de la terminal que
	// el popup se queda, como numerador y denominador y no como una división: en
	// una constante de Go 3/4 vale 0, y el popup se quedaba con el suelo de filas.
	// No es el 100% porque un overlay que tapa la vista entera deja de ser un
	// overlay: perder el inbox justo cuando se está mirando una simulación es
	// perder el contexto de lo que se está mirando.
	simHeightNum = 3
	simHeightDen = 4
	// simMargin son las filas que el popup deja libres arriba y abajo, y
	// simSideMargin las columnas de los lados. El fondo se ve justo en eso.
	simMargin     = 2
	simSideMargin = 4
	// simMinCols y simMinRows son el suelo del popup, para que en una terminal
	// diminuta salga algo en vez de una caja de tres caracteres.
	simMinCols = 20
	simMinRows = 6
)

// simState es la fase del overlay.
type simState int

const (
	simClosed simState = iota
	// simChoosing muestra las dos Strategies y espera una tecla.
	simChoosing
	// simRendering muestra el progreso del render en segundo plano.
	simRendering
	// simShowing muestra la imagen ya generada.
	simShowing
)

// simKinds son las estrategias que se ofrecen, en el orden en que se recorren.
//
// Rebase está excluido a propósito, no por prudencia: git-sim 0.3.5 no lo sabe
// dibujar. Si la rama del ítem ya está basada en la base —el caso normal de una
// PR— responde "Branch 'main' is already based on active branch 'feat'" y sale
// con código 1, con el mensaje puesto del revés; y si las ramas divergen, revienta
// con un IndexError de Python. Merge funciona en los tres casos. Cuando el
// proyecto lo arregle, esta lista es lo único que haya que tocar.
var simKinds = []sim.Kind{sim.KindMerge}

// simPanel es el estado del overlay. Va en la Model y no aparte porque comparte
// ciclo de vida con ella: se abre con una tecla, vive mientras el render corre y
// se cierra con otra tecla.
type simPanel struct {
	state  simState
	item   model.Item
	cursor int
	kind   sim.Kind
	// img es la imagen decodificada: se conserva para no volver a leer el JPEG
	// en cada resize, que es lo único que cambia la geometría.
	img   image.Image
	cells []string
	cellW int
	cellH int
	image string
	// viaGraphics dice que la imagen está publicada en la capa del pane y que por
	// eso el popup no la pinta con celdas. La capa vive por encima del contenido
	// del pane: si el popup se cierra y no se quita, la imagen se queda encima de
	// la TUI.
	viaGraphics bool
	// cellW_px y cellH_px son los píxeles de una celda, medidos por Herdr. Cero =
	// no se sabe, y entonces se supone 1×2.
	cellW_px int
	cellH_px int
}

// Simulator renderiza simulaciones. Es un puerto opcional: sin él la acción
// explica que falta git-sim en vez de fallar.
type Simulator interface {
	Available() bool
	Simulate(ctx context.Context, it model.Item, kind sim.Kind) (sim.Result, error)
}

// Graphics publica la imagen en la capa gráfica del pane, que es lo que la pinta a
// resolución nativa en vez de a una celda por píxel de la imagen.
//
// Es un puerto opcional y el que decide la calidad: sin él, la imagen se dibuja con
// half-blocks, que se ve pixelado porque cuantiza a la rejilla de celdas (con una
// imagen de 1920 px en 84 columnas, cada bloque son 23×23 celdas). Con él, la escala
// la hace el terminal y se ve como una imagen.
type Graphics interface {
	// Available informa si la capa es un camino posible ahora mismo.
	Available() bool
	// CellSize son los píxeles de una celda del pane, que es lo que permite no
	// deformar la imagen y no mandar más resolución de la que se ve. (0, 0) si no
	// se sabe.
	CellSize(ctx context.Context) (cellW, cellH int)
	// SetImage publica la imagen en la capa, colocada en un rectángulo de celdas
	// del viewport del pane. La colocación es el tipo de Herdr porque es su
	// concepto: el pane es una rejilla y la imagen se coloca en celdas, no en
	// píxeles.
	SetImage(ctx context.Context, layer string, img image.Image, p herdr.Placement) error
	// Clear quita la capa.
	Clear(ctx context.Context, layer string) error
}

// simMsg entrega el resultado de una simulación. seq identifica la petición:
// un render que llega tarde, con el popup ya cerrado o reabierto, se descarta en
// lugar de saltar a la pantalla de arriba.
type simMsg struct {
	seq  int
	kind sim.Kind
	res  sim.Result
	err  error
}

// SetSimulator inyecta el simulador. nil lo deshabilita: la acción informa
// entonces que hace falta git-sim.
func (m *Model) SetSimulator(s Simulator) { m.simulator = s }

// SetGraphics inyecta la capa de gráficos del pane. nil deja el popup en
// half-blocks, que es el camino de fuera de Herdr.
func (m *Model) SetGraphics(g Graphics) { m.graphics = g }

// openSimulator abre el selector sobre el ítem seleccionado. Los guards son los
// mismos que los del resto de acciones: la simulación necesita un ítem y una
// rama base, y sin ellos la caja solo podría ofrecer una vista vacía.
//
// Con el merge armado esta tecla no llega aquí: la segunda pulsación de un merge
// solo puede ser un modo, así que la `v` desarma y se consume. Es lo mismo que
// hace con cualquier otra tecla, y evita que una pulsación a destiempo abra un
// popup modal.
func (m Model) openSimulator() (tea.Model, tea.Cmd) {
	if m.simulator == nil || !m.simulator.Available() {
		m.setNotice("simulate: git-sim is not installed", levelWarn)
		return m, nil
	}
	it, ok := m.selected()
	if !ok {
		m.setNotice("select an item first", levelWarn)
		return m, nil
	}
	if strings.TrimSpace(it.TargetBranch) == "" {
		m.setNotice("simulate: the forge reports no target branch for "+refLabel(it), levelWarn)
		return m, nil
	}
	// Abrir la simulación con un merge armado ya está resuelto antes de llegar
	// aquí, pero el disarmado se deja explícito: si mañana alguien enruta la
	// acción desde otro sitio, el popup no puede coexistir con una Confirmación
	// de merge esperando su segunda tecla.
	m.disarmMerge()
	m.sim = simPanel{state: simChoosing, item: it}
	return m, nil
}

// moveSimCursor mueve el cursor del selector y dice si la tecla era de
// navegación. Solo navega cuando hay algo que recorrer: con una sola estrategia no
// hay menú, y una flecha que no mueve nada tiene que caer en el default —que
// cierra— en vez de quedarse swallowed sin hacer nada. La condición es explícita
// para que añadir una segunda estrategia no deje las flechas muertas.
func (m *Model) moveSimCursor(key string) bool {
	if len(simKinds) < 2 {
		return false
	}
	switch key {
	case "up", "k":
		m.sim.cursor = (m.sim.cursor - 1 + len(simKinds)) % len(simKinds)
	case "down", "j", "right", "tab":
		m.sim.cursor = (m.sim.cursor + 1) % len(simKinds)
	default:
		return false
	}
	return true
}

// closeSim cierra el overlay e invalida el render en vuelo: su resultado se
// descarta, aunque la imagen se haya quedado en el caché. Si había una imagen
// publicada en la capa del pane, la quita: la capa vive por encima del contenido
// del pane, así que quedarse ahí taparía la TUI entera.
func (m *Model) closeSim() {
	m.simSeq++
	m.releaseSimLayer()
	m.sim = simPanel{}
}

// releaseSimLayer quita la imagen de la capa de gráficos si se publicó. Va en
// segundo plano y con un contexto propio: si el popup se cierra al salir de la TUI,
// el contexto de la app ya está cancelado y la imagen se quedaría pegada.
func (m *Model) releaseSimLayer() {
	if !m.sim.viaGraphics || m.graphics == nil {
		return
	}
	g := m.graphics
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = g.Clear(ctx, simLayer)
	}()
}

// handleSimKey atiende el popup mientras está abierto. `q` y `ctrl+c` salen, como
// en el resto de la vista: cerrar con `esc` y salir con `q` son dos intenciones
// distintas.
//
// Cualquier otra tecla cierra el popup, y mientras se elige una estrategia solo
// cuentan las que la nombran. Es la misma política que el merge armado, y por el
// mismo motivo: una pulsación que no es una elección no debe poder caer en una
// acción.
func (m Model) handleSimKey(msg tea.KeyPressMsg, key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		m.closeSim()
		m.cancel()
		return m, tea.Quit
	case "o":
		if m.sim.state == simShowing && m.sim.image != "" {
			return m, m.openBrowserCmd(m.sim.image)
		}
		m.closeSim()
		return m, nil
	case "esc":
		m.closeSim()
		return m, nil
	}

	switch m.sim.state {
	case simChoosing:
		if m.moveSimCursor(key) {
			return m, nil
		}
		if key == "enter" {
			return m, m.startSim(simKinds[m.sim.cursor])
		}
		// Cualquier otra tecla cierra el popup sin más.
		m.closeSim()
		return m, nil

	default:
		// Renderizando o enseñando la imagen: el popup ya no espera ninguna
		// tecla en concreto, así que cualquier pulsación lo cierra.
		m.closeSim()
		return m, nil
	}
}

// startSim lanza el render en segundo plano. No bloquea la TUI: son un par de
// segundos, pero bloquear el update loop se nota como un congelón de la vista,
// que es justo lo que un overlay no puede hacer.
//
// Mientras corre, el resultado se manda al canal de eventos como cualquier otro
// trabajo, para que su entrega no dependa de que el usuario siga tocando teclas.
func (m *Model) startSim(kind sim.Kind) tea.Cmd {
	m.sim.state = simRendering
	m.sim.kind = kind
	m.simSeq++
	seq := m.simSeq

	appCtx, events, sim := m.ctx, m.events, m.simulator
	it := m.sim.item
	go func() {
		ctx, cancel := context.WithTimeout(appCtx, simTimeout)
		defer cancel()
		res, err := sim.Simulate(ctx, it, kind)
		sendEvent(appCtx, events, simMsg{seq: seq, kind: kind, res: res, err: err})
	}()
	return nil
}

// applySim recoge el resultado de una simulación. Un fallo cierra el popup y
// avisa en la cabecera, que es donde viven los avisos: un popup explicando su
// propio error encima de la vista es un popup más difícil de leer que el aviso.
func (m *Model) applySim(msg simMsg) {
	if msg.seq != m.simSeq {
		return // petición obsoleta: el popup se cerró o se reabrió mientras corría
	}
	if msg.err != nil {
		m.closeSim()
		m.setNotice("simulate "+string(msg.kind)+": "+msg.err.Error(), levelError)
		return
	}
	img, err := sim.Load(msg.res.Path)
	if err != nil {
		m.closeSim()
		m.setNotice("simulate: "+err.Error(), levelError)
		return
	}
	m.sim.state = simShowing
	m.sim.image = msg.res.Path
	m.sim.img = img
	if !m.publishSimImage(img) {
		m.renderSimCells()
	}
}

// publishSimImage intenta publicar la imagen en la capa de gráficos del pane y dice
// si lo consiguió. Si no, el popup la pintará con half-blocks.
//
// La imagen se reescala al tamaño en píxeles del rectángulo antes de mandarla: el
// terminal la va a dibujar a ese tamaño, así que mandar los 1920 px originales solo
// añade bytes en base64 sin ganar un detalle que se pueda ver. Y se ajusta al alto
// real de la celda, que no es 2× el ancho sino lo que mida el terminal.
func (m *Model) publishSimImage(img image.Image) bool {
	if m.graphics == nil || !m.graphics.Available() {
		return false
	}
	cols, rows := m.simBox()
	col, row := m.simBoxOrigin(cols, rows)
	innerCols, innerRows := cols-2, rows-simChrome
	if innerCols <= 0 || innerRows <= 0 {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cellW, cellH := m.graphics.CellSize(ctx)
	if cellW <= 0 || cellH <= 0 {
		cellW, cellH = 1, 2
	}
	m.sim.cellW_px, m.sim.cellH_px = cellW, cellH
	// El tamaño en píxeles del rectángulo, con la celda medida. Mandar la imagen
	// original aquí solo añadiría bytes: el terminal la va a dibujar a este tamaño.
	resized := sim.Resize(img, innerCols*cellW, innerRows*cellH)
	if resized == nil {
		return false
	}
	if err := m.graphics.SetImage(ctx, simLayer, resized, herdr.Placement{
		Col: col + 1, Row: row + 1, Cols: innerCols, Rows: innerRows,
	}); err != nil {
		return false
	}
	m.sim.viaGraphics = true
	m.sim.cells, m.sim.cellW, m.sim.cellH = nil, 0, 0
	return true
}

// republishSimImage vuelve a publicar la imagen en la capa con la geometría
// actual. Se llama en cada resize: la colocación va en celdas, así que cambia con
// el tamaño de la terminal igual que el marco.
func (m *Model) republishSimImage() {
	img := m.sim.img
	if img == nil {
		return
	}
	m.sim.viaGraphics = false
	if !m.publishSimImage(img) {
		// Si la capa deja de estar disponible (pane oculto, Herdr que no
		// responde), se vuelve a half-blocks en vez de dejar un hueco.
		m.renderSimCells()
	}
}

// simBox es la geometría del popup: sus dimensiones exteriores. Es la única
// fuente, y las celdas de la imagen se derivan de aquí.
//
// Que sea una sola fuente no es PURITANISMO: cuando el ancho de la caja y el de
// las celdas los decidía cada uno por su cuenta, la imagen se dibujaba a 94
// columnas dentro de una caja de 200 y ocupaba el tercio izquierdo del popup, con
// un borde vacío a su derecha que parecía parte de la imagen.
func (m Model) simBox() (w, h int) {
	switch m.sim.state {
	case simChoosing:
		// El selector no enseña imagen: es una caja estrecha con tres líneas.
		return min(m.contentWidth(), simChooserWidth), simChrome + 2
	case simRendering:
		return min(m.contentWidth(), simChooserWidth), simChrome + 2
	}

	// La imagen manda: la caja se ajusta a lo que la imagen necesita, no al revés.
	cellW, cellH := m.cellSize()
	cols, rows := sim.FitCells(m.sim.img, cellW, cellH, m.simMaxCols(), m.simMaxRows())
	return cols + 2, rows + simChrome
}

// cellSize son los píxeles de una celda. Herdr los mide (en kitty con la fuente por
// defecto son 9×19, no 2×1) y usarlos hace dos cosas a la vez: que la imagen no se
// deforme y que no se mande más resolución de la que se ve. Sin Herdr se supone 1×2,
// que es lo que hacen casi todos los terminales.
func (m Model) cellSize() (w, h int) {
	if m.sim.cellW_px > 0 && m.sim.cellH_px > 0 {
		return m.sim.cellW_px, m.sim.cellH_px
	}
	return 1, 2
}

// simBoxOrigin es la esquina del popup en la vista, en celdas. Lo comparte con el
// overlay: es lo que permite que la imagen en la capa de gráficos caiga justo en el
// hueco del marco.
func (m Model) simBoxOrigin(cols, rows int) (col, row int) {
	return centeredOrigin(m.contentWidth(), m.height, cols, rows)
}

// simMaxCols son las columnas que el popup puede usar, dejando fondo a los lados.
func (m Model) simMaxCols() int {
	return max(simMinCols, m.contentWidth()-simSideMargin)
}

// simMaxRows son las líneas que el popup puede usar, dejando fondo arriba y abajo.
func (m Model) simMaxRows() int {
	free := m.height - 2*simMargin
	return max(simMinRows, free*simHeightNum/simHeightDen)
}

// renderSimCells (re)dibuja la imagen a la geometría del popup. Se llama al llegar
// el resultado y en cada resize, que es lo único que cambia el tamaño disponible.
func (m *Model) renderSimCells() {
	if m.sim.state != simShowing || m.sim.img == nil || m.sim.viaGraphics {
		// Con la imagen en la capa de gráficos no hay celdas que pintar: el popup
		// solo dibuja el marco y Herdr pone la imagen encima.
		m.sim.cells, m.sim.cellW, m.sim.cellH = nil, 0, 0
		return
	}
	cols, rows := m.simBox()
	w, h := cols-2, rows-simChrome
	if w <= 0 || h <= 0 {
		m.sim.cells, m.sim.cellW, m.sim.cellH = nil, 0, 0
		return
	}
	if m.sim.cells != nil && m.sim.cellW == w && m.sim.cellH == h {
		return
	}
	m.sim.cells = sim.Cells(m.sim.img, w, h)
	m.sim.cellW, m.sim.cellH = w, h
}

// simOverlay compone la caja del popup. Devuelve false si no hay nada que pintar,
// que es el caso normal. La anchura con la que se centra el overlay es la de la
// vista, no la de la caja: son cosas distintas y confundirlas es lo que dejaba la
// imagen en un rincón.
func (m Model) simOverlay() (string, bool) {
	switch m.sim.state {
	case simChoosing:
		return m.simChooserBox(), true
	case simRendering:
		return m.simBusyBox(), true
	case simShowing:
		return m.simImageBox(), true
	default:
		return "", false
	}
}

// simChooserBox es la primera fase: confirmar la estrategia. Ninguna tiene modo
// por defecto, igual que en el merge: la simulación que se enseña tiene que ser la
// que el usuario quiso ver, y una estrategia implícita la cambiaría por otra sin
// dejar rastro. Con una sola estrategia disponible el popup no pregunta, sino
// confirma: enseñar qué se va a simular antes de gastar el render es justo lo
// que hace útil un popup de dos segundos.
func (m Model) simChooserBox() string {
	width, _ := m.simBox()
	it := m.sim.item
	branch := strings.TrimSpace(it.SourceBranch)
	if branch == "" {
		branch = "the PR branch"
	}
	base := it.TargetBranch

	lines := make([]string, 0, 4)
	for i, kind := range simKinds {
		flow := base + " ← " + branch
		if kind == sim.KindRebase {
			flow = branch + " → " + base
		}
		marker := "  "
		if i == m.sim.cursor {
			marker = styleCursor.Render("▸ ")
		}
		lines = append(lines, marker+styleRef.Render(padRight(string(kind), 8))+styleDim.Render(flow))
	}
	lines = append(lines, styleDim.Render("enter render · esc close"))

	return borderedBox(" simulate "+refLabel(it), strings.Join(lines, "\n"), width)
}

// simBusyBox es la fase intermedia: el render corre y se dice cuánto se espera de
// forma honesta, que es "un momento". Solo se ve unos segundos y por eso no
// necesita un cronómetro: untruecer un progreso que no se puede medir es peor que
// no medirlo.
func (m Model) simBusyBox() string {
	width, _ := m.simBox()
	body := m.spinner.View() + styleInfo.Render(" rendering "+string(m.sim.kind)+"…")
	body += "\n" + styleDim.Render("this takes a couple of seconds · esc close")
	return borderedBox(" simulate: "+string(m.sim.kind), body, width)
}

// simImageBox es la fase final: la imagen del render, con su pie de ayuda.
//
// Con la imagen en la capa de gráficos, el interior va vacío de propósito: la
// imagen la pinta Herdr por encima. Rellenarlo con celdas sería mandar por el
// terminal lo que ya está en pantalla y encima lo taparía dos veces.
func (m Model) simImageBox() string {
	width, height := m.simBox()
	// El cuerpo se rellena hasta el alto MENOS el marco y la línea del pie, que es
	// justo lo que mide simChrome. Antes se rellenaba hasta alto - marco - 1, y la
	// caja salía una fila más corta de lo que decía simBox: como el alto lo
	// heredan el centrado y la colocación de la imagen, esa fila se nota.
	body := padLines(m.sim.cells, height-simChrome)
	body = append(body, styleDim.Render("esc close · o open image"))
	return borderedBox(" simulate: "+string(m.sim.kind)+" "+refLabel(m.sim.item), strings.Join(body, "\n"), width)
}

// padLines completa una lista de líneas con líneas vacías hasta que mida `want`.
//
// Está en su propia función por la misma razón que la geometría de los
// comentarios: el relleno decide el ALTO de la caja, y dentro del pintado ese alto
// lo absorbe el borde. Una caja una fila más corta no se ve como una caja más
// corta, se ve como una caja con el pie pegado al borde de arriba.
//
// El relleno es un SUELO y no un recorte: si ya hay más líneas de las pedidas, se
// devuelven todas. Recortar el contenido para forzar una altura sería tirar
// imagen, y la imagen es lo que el usuario está mirando.
func padLines(lines []string, want int) []string {
	out := make([]string, 0, max(0, want))
	out = append(out, lines...)
	for len(out) < want {
		out = append(out, "")
	}
	return out
}

// simBorder es el color del popup: el de la información, para que se distinga de
// las cajas de la vista sin parecer un aviso.
var simBorder = lipgloss.Color("39")

// borderedBox dibuja el popup con su título embebido en el borde superior.
func borderedBox(title, content string, width int) string {
	return bordered.RenderWithTitle(bordered.Rounded(), simBorder, title, content, width)
}

// padRight completa con espacios hasta n columnas visibles, para que las
// etiquetas del selector queden alineadas.
func padRight(s string, n int) string {
	if pad := n - ansi.StringWidth(s); pad > 0 {
		return s + strings.Repeat(" ", pad)
	}
	return s
}
