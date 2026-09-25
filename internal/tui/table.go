// Filas, columnas y celdas del inbox.
//
// Cada celda devuelve su texto plano y su estilo por separado; el render hace
// pad(texto) ANTES de aplicar el estilo, así los códigos ANSI nunca rompen el
// ancho de la tabla. La única excepción es la columna DIFF, que lleva dos
// colores en la misma celda: ahí el texto son tramos, y el relleno se mide
// sumando sus anchos en plano.
package tui

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"prdash/internal/forge/model"
	"prdash/internal/state"
)

// tableColumn describe una columna (título y ancho).
type tableColumn struct {
	title string
	width int
}

// Índices de columna. ITEM es la única de ancho variable: el resto son
// constantes y solo hay que nombrarlas una vez.
const (
	colForgeIdx = iota
	colRefIdx
	colTitleIdx
	colRoleIdx
	colStateIdx
	colChecksIdx
	colDiffIdx
)

// tableColumns son las columnas en orden de prioridad: en anchos estrechos se
// omiten por la derecha para no truncar las columnas de estado. El ancho de ITEM
// es nominal: newRefLayout lo sustituye por el que pide el contenido.
//
// DIFF va la última a propósito. Es la única columna que se puede perder sin
// perder información — el detalle la trae siempre — y a un ancho de 124 con
// rutas de proyecto largas no cabe junto a las otras seis. Ponerla antes que
// CHECKS la haría subir de prioridad y le robaría el hueco a un dato que decide
// si se puede mergear, así que solo aparece con terminal ancha (~133).
var tableColumns = []tableColumn{
	{"FORGE", colForge},
	{"ITEM", itemWidthMin},
	{"TITLE", colTitle},
	{"ROLE", colRole},
	{"STATE", colState},
	{"CHECKS", colChecks},
	{"DIFF", colDiff},
}

// span es un tramo de celda con su propio estilo. Sirve para la única celda que
// necesita dos colores a la vez, la columna DIFF: verde lo que se añade y rojo
// lo que se quita, en el mismo ancho y con el mismo hueco de separación.
type span struct {
	text  string
	style lipglossStyle
}

// cell es una celda con su texto plano, su estilo y su ancho.
//
// El texto llega siempre plano y el estilo se aplica en el render, después de
// pad(): así los códigos ANSI nunca rompen el ancho de la tabla. Cuando hay
// spans, sustituyen a text y style, y el relleno se hace sobre el último tramo
// para que la celda siga midiendo exactamente lo que dice width.
type cell struct {
	text  string
	style lipglossStyle
	width int
	spans []span
}

// textWidth es el hueco de texto de una columna: su ancho menos el espacio que
// se reserva para separarla de la siguiente.
//
// El ancho de una columna es su tamaño total, gap incluido, no el espacio
// disponible para el texto. Por eso el texto se recorta a textWidth y no al
// ancho: si se llenara el ancho exacto, pad() no añadiría nada y la celda
// pegaría con la siguiente ("…gatewayreview req"). Toda columna acaba al menos
// en un espacio, así que el hueco entre dos celdas contiguas son dos.
func textWidth(w int) int {
	return max(w-1, 1)
}

// fitColumns devuelve cuántas columnas caben en el ancho disponible, dejando
// siempre al menos la de FORGE.
func fitColumns(l refLayout, innerWidth int) int {
	used := 0
	for i, c := range l.cols {
		if used+c.width > innerWidth {
			return max(1, i)
		}
		used += c.width
	}
	return len(l.cols)
}

// headerLine compone el header de columnas que cabe en el ancho dado.
func headerLine(l refLayout, innerWidth int) string {
	var b strings.Builder
	for _, c := range l.cols[:fitColumns(l, innerWidth)] {
		b.WriteString(pad(c.title, c.width))
	}
	return styleCount.Render(strings.TrimRight(b.String(), " "))
}

// itemCells compone las celdas de un ítem. `viewer` es el login del usuario en
// ese forge: lo necesita la columna ROLE para marcar los ítems propios. `sec` es
// la sección a la que pertenece: de ella sale el prefijo de ruta que la celda de
// ITEM no repite.
func itemCells(it model.Item, sec model.Section, viewer string, l refLayout) []cell {
	refW := l.cols[colRefIdx].width
	// Cada celda se recorta a textWidth, no a su ancho: es lo que deja el hueco
	// de separación aunque el texto llene la columna. Cada columna conserva su
	// estrategia de recorte (ITEM por la cola, el resto por la cabeza).
	return []cell{
		{text: truncate(forgeBadge(it), textWidth(l.cols[colForgeIdx].width)), style: styleForge, width: l.cols[colForgeIdx].width},
		{text: truncateTail(refSuffix(it, l.prefixOf(sec)), textWidth(refW)), style: styleRef, width: refW},
		{text: truncate(it.Title, textWidth(l.cols[colTitleIdx].width)), style: styleTitle, width: l.cols[colTitleIdx].width},
		{text: truncate(roleText(it, viewer), textWidth(l.cols[colRoleIdx].width)), style: styleRole, width: l.cols[colRoleIdx].width},
		{text: truncate(state.Derive(it).String(), textWidth(l.cols[colStateIdx].width)), style: styleForState(state.Derive(it)), width: l.cols[colStateIdx].width},
		{text: truncate(checksText(it.Checks), textWidth(l.cols[colChecksIdx].width)), style: styleChecks(it.Checks), width: l.cols[colChecksIdx].width},
		diffCell(it.Diff, l.cols[colDiffIdx].width),
	}
}

// renderCells pinta una fila: pad() sobre el texto plano y luego el estilo.
func renderCells(cells []cell, l refLayout, innerWidth int) string {
	var b strings.Builder
	for _, c := range cells[:min(len(cells), fitColumns(l, innerWidth))] {
		b.WriteString(renderCell(c))
	}
	return b.String()
}

// renderCell pinta una celda. Sin spans es pad() sobre el plano y el estilo
// encima; con spans, el relleno va al final del último tramo para que el ancho
// total siga siendo el de la columna. En los dos casos el relleno se mide en
// texto plano, nunca en runes de una cadena ya coloreada.
func renderCell(c cell) string {
	if len(c.spans) == 0 {
		return c.style.Render(pad(c.text, c.width))
	}
	used := 0
	for _, s := range c.spans {
		used += utf8.RuneCountInString(s.text)
	}
	// El relleno es el sobrante que le toca a la celda, y va al final del último
	// tramo. Se mide sobre el ancho que ya ocupa ese tramo, no sobre el total:
	// used lo incluye, así que sin sumarlo de vuelta el último tramo se queda
	// sin relleno y la celda mide menos que su columna.
	slack := c.width - used
	var b strings.Builder
	last := len(c.spans) - 1
	for i, s := range c.spans {
		text := s.text
		if i == last {
			text = pad(text, utf8.RuneCountInString(text)+slack)
		}
		b.WriteString(s.style.Render(text))
	}
	return b.String()
}

// forgeLabel indica forge y host del ítem. Es la forma larga: vive en el
// detalle, donde sí cabe la ruta completa.
func forgeLabel(it model.Item) string {
	if it.Host == "" {
		return it.Forge
	}
	return it.Forge + "@" + it.Host
}

// forgeShortNames mapea forge → etiqueta corta de la columna FORGE.
var forgeShortNames = map[string]string{
	"github":    "GH",
	"gitlab":    "GLab",
	"bitbucket": "BB",
}

// forgePublicHosts son los hosts públicos de cada forge: ahí la etiqueta corta
// ya es suficiente y no hace falta repetir el proveedor ni el host.
var forgePublicHosts = map[string]string{
	"github":    "github.com",
	"gitlab":    "gitlab.com",
	"bitbucket": "bitbucket.org",
}

// forgeBadge etiqueta la columna FORGE sin el proveedor ni el host entero: solo
// la abreviatura en el host estándar ("GH", "GLab") y abreviatura + primera
// etiqueta del host en uno self-hosted ("GLab@umane"). La ruta completa se
// reserva para el detalle, donde no estorba.
func forgeBadge(it model.Item) string {
	short := forgeShortNames[it.Forge]
	if short == "" {
		short = it.Forge // forge sin abreviatura conocida: se muestra tal cual
	}
	if short == "" || it.Host == "" {
		return short
	}
	if strings.EqualFold(it.Host, forgePublicHosts[it.Forge]) {
		return short
	}
	label, _, _ := strings.Cut(it.Host, ".")
	return short + "@" + label
}

// refLabel compone la referencia corta del ítem: "proyecto#número".
func refLabel(it model.Item) string {
	return it.Ref.Project + "#" + strconv.Itoa(it.Number)
}

// roleText indica el papel del usuario en el ítem: por qué lo tiene en el inbox
// y, cuando no hay review pendiente, si es el autor. "own" es la señal de que
// approve no va a funcionar, antes de pulsarlo.
func roleText(it model.Item, viewer string) string {
	switch it.ReviewKind {
	case model.ReviewRequested:
		return "review req"
	case model.ReviewAssigned:
		return "assigned"
	}
	if ok, _ := state.CanApprove(it, viewer); !ok {
		return "own"
	}
	return "-"
}

// checksText resume el estado de los checks.
func checksText(c model.Checks) string {
	switch c.State {
	case model.ChecksFailing:
		return "✗" + strconv.Itoa(c.Failing)
	case model.ChecksPending:
		return "…" + strconv.Itoa(c.Pending)
	case model.ChecksPassing:
		return "✓"
	default:
		return "-"
	}
}

// styleChecks elige el color de la columna de checks.
func styleChecks(c model.Checks) lipglossStyle {
	switch c.State {
	case model.ChecksFailing:
		return styleChecksFailing
	case model.ChecksPending:
		return styleChecksPending
	case model.ChecksPassing:
		return styleChecksPassing
	default:
		return styleDim
	}
}

// diffColumnText resume el diffstat para la columna DIFF, compacto para que el
// ancho de la columna no dependa de si el PR toca 40 líneas o 40.000. Un
// diffstat desconocido se marca "-", igual que los checks sin datos: no se
// inventa un cero que parece "el PR no toca nada".
func diffColumnText(d model.DiffStat) string {
	if !d.Known {
		return "-"
	}
	return "+" + compactCount(d.Additions) + " -" + compactCount(d.Deletions)
}

// diffCell compone la celda de la columna DIFF con sus dos colores. El texto se
// formatea una sola vez, en plano, y de ahí sale tanto lo que se mide y se
// recorta como lo que se colorea: así el ancho y el color nunca pueden discrepar.
func diffCell(d model.DiffStat, width int) cell {
	text := truncate(diffColumnText(d), textWidth(width))
	if !d.Known {
		return cell{text: text, style: styleDiffUnknown, width: width}
	}
	return cell{width: width, spans: diffSpans(text)}
}

// diffSpans parte un diffstat en tramos coloreados: verde lo que el cambio
// añade, rojo lo que quita. Es notación de diff y nada más — no dice si el
// cambio es bueno, solo qué líneas son nuevas y cuáles desapareceron.
//
// La forma se reconoce a propósito y no se colorea nada que no encaje: un texto
// truncado, un "-" de diffstat desconocido o un "no changes" vuelven planos. Un
// color puesto sobre algo que no es una cifra sería mentir sobre el dato.
func diffSpans(plain string) []span {
	add, rest, ok := strings.Cut(plain, " ")
	if !ok || !isDiffCount(add, '+') {
		return nil
	}
	del, tail, hasTail := strings.Cut(rest, " ")
	if !isDiffCount(del, '-') {
		return nil
	}
	spans := []span{{add, styleDiffAdd}, {" ", styleTitle}, {del, styleDiffDel}}
	if hasTail && tail != "" {
		spans = append(spans, span{" " + tail, styleTitle})
	}
	return spans
}

// styleDiffText colorea un diffstat que ya se ha medido y recortado. Es la vía
// que usa el detalle, donde el valor se compone como texto y no como celda.
//
// El orden importa: se colorea DESPUÉS de recortar porque el ancho de la rejilla
// del detalle se calcula sobre el texto plano, y colorear antes haría que
// truncate contara los códigos ANSI como si fueran letras del valor.
func styleDiffText(plain string) string {
	spans := diffSpans(plain)
	if spans == nil {
		return plain
	}
	var b strings.Builder
	for _, s := range spans {
		b.WriteString(s.style.Render(s.text))
	}
	return b.String()
}

// isDiffCount dice si s es un recuento con signo: "+381", "-36" y sus formas
// abreviadas ("+1.2k", "-6.7k"). Sin esta comprobación, cualquier texto con un
// espacio se repartiría en dos colores arbitrarios.
func isDiffCount(s string, sign byte) bool {
	if len(s) < 2 || s[0] != sign {
		return false
	}
	for i := 1; i < len(s); i++ {
		switch c := s[i]; {
		case c >= '0' && c <= '9', c == '.', c == 'k':
		default:
			return false
		}
	}
	return true
}

// compactCount abrevia un recuento de líneas por encima del millar: décimas
// ("1.2k") mientras quepan en 4 runes, y enteros a partir de cinco dígitos
// ("12k"). El redondeo se hace en décimas enteras y no con fmt, porque 9999 con
// un decimal sale "10.0k": ni cabe en 4 runes ni es un recuento que signifique
// algo. Si el redondeo al alza se sale de la ventana, se delega en los enteros.
func compactCount(n int) string {
	switch {
	case n < 0:
		return strconv.Itoa(n)
	case n < 1000:
		return strconv.Itoa(n)
	case n < 10000:
		if tenths := (n + 50) / 100; tenths < 100 {
			return strconv.Itoa(tenths/10) + "." + strconv.Itoa(tenths%10) + "k"
		}
		return strconv.Itoa((n+500)/1000) + "k"
	default:
		return strconv.Itoa(n/1000) + "k"
	}
}

// pad rellena a la derecha midiendo runes (solo texto plano, sin ANSI).
func pad(s string, w int) string {
	n := utf8.RuneCountInString(s)
	if n >= w {
		return s
	}
	return s + strings.Repeat(" ", w-n)
}

// truncate recorta a w runes añadiendo "…" si hacía falta.
func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= w {
		return s
	}
	runes := []rune(s)
	if w == 1 {
		return "…"
	}
	return string(runes[:w-1]) + "…"
}

// stripANSI quita secuencias ANSI para comparar contenido en tests.
func stripANSI(s string) string {
	var out strings.Builder
	inSeq := false
	for _, r := range s {
		switch {
		case r == '\x1b':
			inSeq = true
		case inSeq && (r == 'm' || r == 'K'):
			inSeq = false
		case !inSeq:
			out.WriteRune(r)
		}
	}
	return out.String()
}
