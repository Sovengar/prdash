// Composición de las cajas de la vista. Cada región de pantalla es una caja con
// borde redondeado y título embebido en la línea superior: cabecera, lista del
// inbox, panel de detalle del ítem seleccionado y atajos. Se apilan sin líneas en
// blanco entre ellas —los bordes ya separan— y todas usan el ancho exterior de
// la terminal, así que la suma de alturas da exactamente la altura del terminal.
package tui

import (
	"fmt"
	"strings"
	"time"

	"prdash/internal/forge/model"
	"prdash/internal/tui/bordered"
)

// view es la vista compuesta: su texto y, fila a fila, si un aviso puede
// superponerse a ella. Las filas van en el mismo orden que las líneas del texto,
// así que el overlay de avisos puede elegir dónde aterrizar sin romper un marco.
type view struct {
	text string
	rows []bool
}

// box es una caja ya compuesta. Toda caja tiene exactamente una línea de borde
// arriba y otra abajo, así que su interior son las líneas intermedias: es lo
// único donde puede aterrizar un aviso sin pisar un borde. paintable dice si
// además la caja cede su interior; la de atajos no, porque es la ayuda que hay
// que leer cuando no se entiende una tecla.
type box struct {
	text      string
	paintable bool
}

// lines son las líneas que ocupa la caja, bordes incluidos.
func (b box) lines() int { return strings.Count(b.text, "\n") + 1 }

// stack apila cajas llevándose la cuenta de qué filas admiten un aviso. Los
// bordes quedan fuera a propósito: es justo lo que evita que un aviso rompa un
// marco. Join no añade líneas (el salto de línea ES el separador), así que las
// filas de cada caja se encadenan sin huecos.
type stack struct {
	parts []string
	rows  []bool
}

func (s *stack) add(b box) {
	n := b.lines()
	for i := range n {
		s.rows = append(s.rows, b.paintable && i > 0 && i < n-1)
	}
	s.parts = append(s.parts, b.text)
}

func (s *stack) view() view {
	return view{text: strings.Join(s.parts, "\n"), rows: s.rows}
}

// sectionLines envuelve el contenido en una caja bordada con título embebido en
// la línea superior. El contenido llega ya partido en líneas porque quien compone
// sabe cuántas quiere (las rellena hasta su presupuesto) y así la caja no tiene
// que re-contarlas.
func (m Model) sectionLines(title string, content []string, paintable bool) box {
	if title != "" {
		title = " " + title + " "
	}
	return box{
		text: bordered.RenderWithTitle(
			bordered.Rounded(), borderColor, title, strings.Join(content, "\n"), m.outerWidth(),
		),
		paintable: paintable,
	}
}

// layout calcula el reparto de alto de la vista: lista arriba, panel de detalle
// abajo.
func (m Model) layout() layout {
	return computeLayout(m.height, len(m.hintLines()), m.height > 0)
}

// compose apila las cajas visibles: la cabecera, las que le pase el cuerpo
// (lista y panel de detalle) y los atajos.
func (m Model) compose(lay layout, boxes ...box) view {
	var s stack
	if lay.showHeader {
		s.add(m.headerSection())
	}
	for _, b := range boxes {
		s.add(b)
	}
	if lay.showKeybinds {
		s.add(m.keybindsSection(lay.hintLines))
	}
	return s.view()
}

// headerSection muestra el estado de cada forge. El nombre va en el borde de la
// caja, como en las demás, y el contenido se queda para lo que cambia: el
// indicador de refresco va primero para que sobreviva al recorte en anchos
// estrechos.
func (m Model) headerSection() box {
	parts := make([]string, 0, 2)
	if m.loading {
		parts = append(parts, m.spinner.View()+styleCount.Render(" refreshing…"))
	}
	parts = append(parts, m.forgesStatusLine(time.Now()))
	return m.sectionLines("PRDash", []string{strings.Join(parts, "  ")}, true)
}

// listSection pinta la lista (headers de sección, avisos y filas) en la caja del
// inbox, recortada a la ventana visible. Rellena hasta el alto reservado para
// que la caja no encoja: si no, el detalle bailaría al añadir un ítem.
func (m Model) listSection(lay layout) box {
	all := m.listLines(m.contentWidth())

	// Antes del primer WindowSizeMsg no se conoce la altura: se pinta la lista
	// entera, sin recortar, que es lo que evita un render inicial vacío.
	var body []string
	if lay.bodyLines > 0 {
		for _, l := range visibleList(all, m.scroll, lay.bodyLines) {
			body = append(body, l.text)
		}
		for len(body) < lay.bodyLines {
			body = append(body, "")
		}
	} else {
		body = textOf(all)
	}
	return m.sectionLines("Inbox", body, true)
}

// textOf extrae el texto ya maquetado de las líneas de la lista. La caja solo
// necesita el texto: el índice de fila era para el auto-scroll, que corre antes
// de componer la vista.
func textOf(lines []listLine) []string {
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		out = append(out, l.text)
	}
	return out
}

// detailSection envuelve el detalle de un ítem en su caja, con la referencia como
// título: dice de un vistazo sobre qué ficha se trata. El cuerpo se rellena
// hasta su alto reservado para que la caja no dependa de lo largo que sea la
// ficha.
func (m Model) detailSection(it model.Item, ok bool, rows int) box {
	body := m.detailLines(it, ok, rows)
	for len(body) < rows {
		body = append(body, "")
	}
	title := "Detail"
	if ok {
		title = refLabel(it)
	}
	return m.sectionLines(title, body, true)
}

// keybindsSection muestra hasta hintLines líneas de la barra de atajos. No es
// paintable: es la ayuda, y un aviso encima la volvería ilegible justo cuando
// hace falta.
func (m Model) keybindsSection(hintLines int) box {
	lines := m.hintLines()
	if hintLines < len(lines) {
		lines = lines[:max(0, hintLines)]
	}
	return m.sectionLines("Keybinds", lines, false)
}

// hintSep une las partes de la barra de atajos. Lo compone la TUI y no la
// config porque es una decisión de maquetación, no de datos: la lista y su
// orden son cosa de config.Hints().
const hintSep = " · "

// hintLines parte la barra de atajos en las líneas que caben en el ancho interior
// de la caja, acotadas por maxHintLines. En un terminal estrecho los atajos se
// reparten en varias líneas en vez de perder la cola, y la cola es precisamente
// lo que config.Hints() ordena para que no se pierda lo imprescindible: `quit`
// va primero porque el recorte tira por el final.
//
// Con merge armado la caja deja de ser ayuda y pasa a ser la Confirmación: es el
// aviso que el usuario tiene que leer antes de la segunda pulsación, y por eso
// sustituye a la barra en vez de competir con ella. Vive aquí y no en un toast
// porque un toast caduca a los 4 s y una Confirmación a la que se contesta
// después de mirar a otro lado tiene que seguir ahí.
func (m Model) hintLines() []string {
	if m.mergeArmed {
		return wrapHint(m.mergeConfirmText(), m.contentWidth(), func(s string) string { return styleWarn.Render(s) })
	}
	return wrapHint(strings.Join(m.cfg.Hints(), hintSep), m.contentWidth(), func(s string) string { return styleHint.Render(s) })
}

// wrapHint parte el texto a lo ancho y lo viste línea a línea, para que el color
// no dependa de dónde caiga el corte.
func wrapHint(text string, width int, paint func(string) string) []string {
	plain := wrapText(text, width)
	if len(plain) > maxHintLines {
		plain = plain[:maxHintLines]
	}
	lines := make([]string, 0, len(plain))
	for _, l := range plain {
		lines = append(lines, paint(l))
	}
	return lines
}

// mergeConfirmText compone la Confirmación de merge. La segunda tecla ES el
// modo, así que no hay estrategia por defecto que se pueda ejecutar sin
// nombrarla: las tres opciones se leen enteras en la caja. Va en inglés como
// todos los avisos de la TUI.
func (m Model) mergeConfirmText() string {
	it, _ := m.selected()
	return fmt.Sprintf(
		"merge %s? Are you sure you want to merge this PR/MR · press the mode: m merge commit · r rebase · s squash · esc cancel",
		refLabel(it),
	)
}

// outerWidth es el ancho exterior de las cajas: el de la terminal. Antes del
// primer WindowSizeMsg se usa un ancho de trabajo por defecto, porque la caja se
// dibuja siempre a un ancho exacto.
func (m Model) outerWidth() int {
	if m.width <= 0 {
		return defaultOuterWidth
	}
	return m.width
}
