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
	"prdash/internal/sim"
	"prdash/internal/tui/bordered"
)

// simTimeout acota la simulación completa. El runner tiene el suyo propio y más
// corto; este es la red que cubre también preparar el directorio de trabajo.
const simTimeout = 90 * time.Second

// Geometría del popup.
const (
	// simBoxMaxWidth es el ancho máximo del popup: más ancho que esto y la
	// imagen se ve más grande que el hueco que hay en la terminal.
	simBoxMaxWidth = 96
	// simBoxMaxRows acota el alto para que el popup no se coma el inbox entero.
	simBoxMaxRows = 26
	// simChooserWidth es el ancho del selector: dos opciones y una ayuda, sin
	// más.
	simChooserWidth = 64
	// simChrome son las líneas del popup que no son imagen: borde de arriba,
	// borde de abajo y la de ayuda.
	simChrome = 3
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
}

// Simulator renderiza simulaciones. Es un puerto opcional: sin él la acción
// explica que falta git-sim en vez de fallar.
type Simulator interface {
	Available() bool
	Simulate(ctx context.Context, it model.Item, kind sim.Kind) (sim.Result, error)
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

// closeSim cierra el overlay e invalida el render en vuelo: su resultado se
// descarta, aunque la imagen se haya quedado en el caché.
func (m *Model) closeSim() {
	m.simSeq++
	m.sim = simPanel{}
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
		switch key {
		case "up", "k":
			m.sim.cursor = (m.sim.cursor - 1 + len(simKinds)) % len(simKinds)
		case "down", "j", "right", "tab":
			m.sim.cursor = (m.sim.cursor + 1) % len(simKinds)
		case "enter", "left":
			return m, m.startSim(simKinds[m.sim.cursor])
		default:
			m.closeSim()
		}
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
	m.renderSimCells(m.simBoxWidth(), m.simBoxHeight())
}

// simBoxWidth es el ancho exterior del popup, acotado al de la vista.
func (m Model) simBoxWidth() int {
	if m.sim.state == simChoosing {
		return min(m.contentWidth(), simChooserWidth)
	}
	return min(m.contentWidth(), simBoxMaxWidth)
}

// simBoxHeight es el alto exterior del popup: deja un margen de filas para que
// se vea que hay algo detrás, que es media razón del overlay.
func (m Model) simBoxHeight() int {
	rows := min(m.height-2, simBoxMaxRows)
	if rows < simChrome+1 {
		rows = simChrome + 1
	}
	return rows
}

// renderSimCells (re)dibuja la imagen a la geometría del popup. Se llama al
// llegar el resultado y en cada resize, que es lo único que cambia el ancho útil.
func (m *Model) renderSimCells(boxW, boxH int) {
	w, h := boxW-2, boxH-simChrome
	if m.sim.state != simShowing || w <= 0 || h <= 0 || m.sim.img == nil {
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
// que es el caso normal.
func (m Model) simOverlay(width int) (string, bool) {
	switch m.sim.state {
	case simChoosing:
		return m.simChooserBox(width), true
	case simRendering:
		return m.simBusyBox(width), true
	case simShowing:
		return m.simImageBox(width), true
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
func (m Model) simChooserBox(width int) string {
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
func (m Model) simBusyBox(width int) string {
	body := m.spinner.View() + styleInfo.Render(" rendering "+string(m.sim.kind)+"…")
	body += "\n" + styleDim.Render("this takes a couple of seconds · esc close")
	return borderedBox(" simulate: "+string(m.sim.kind), body, width)
}

// simImageBox es la fase final: la imagen del render, con su pie de ayuda.
func (m Model) simImageBox(width int) string {
	body := make([]string, 0, len(m.sim.cells)+2)
	body = append(body, m.sim.cells...)
	body = append(body, styleDim.Render("esc close · o open image"))
	return borderedBox(" simulate: "+string(m.sim.kind)+" "+refLabel(m.sim.item), strings.Join(body, "\n"), width)
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
