// La conversación del ítem seleccionado dentro del panel de detalle.
//
// Los comentarios no son una vista aparte ni un subproceso que el usuario pide:
// son parte de la ficha, y por eso se consultan solos al llegar el cursor al
// ítem. Lo que este archivo evita es el otro extremo, que es consultarlos con el
// inbox: son N llamadas más por ciclo para datos de una sola fila.
package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/forge/parse"
	"prdash/internal/tui/bordered"
)

// commentState es lo que la ficha sabe de un ítem en lo que a conversación se
// refiere.
//
// Guardar el estado y no solo la lista es lo que permite que "no hay comentarios"
// y "aún no lo he preguntado" se pinten distinto. Con solo la lista, un ítem sin
// consultar y otro sin comentarios serían los dos una lista vacía, y el primero
// se quedaría fingiendo que el PR no tiene conversación.
type commentState struct {
	list  []model.Comment
	total int  // los que dice el forge que hay, para poder decir "5 de 23"
	ready bool // la consulta terminó, salga bien o mal
	err   string
}

// maxCommentLines es el tope de filas por comentario. Sin él, un comentario largo
// se comería el panel entero en un terminal alto y los otros cuatro no se verían:
// lo pedido son cinco comentarios, no uno.
const maxCommentLines = 4

// commentsCmd agenda el siguiente tick de comentarios.
func (m *Model) commentsCmd() tea.Cmd {
	return tea.Tick(commentsPoll, func(time.Time) tea.Msg { return commentsTickMsg{} })
}

// requestComments lanza la consulta de la conversación del ítem seleccionado si
// todavía no se tiene. No toca el canal de eventos: la goroutine publica su
// resultado con sendEvent, pero eso no consume el lector que tiene la bomba, así
// que el invariante de un único lector sigue igual.
//
// Devuelve siempre el tick rearmado, tanto si consultó como si no: la cadena no
// se corta nunca y el próximo cambio de selección se nota sin tener que acordarse
// de rearmarla en el sitio del cambio.
func (m *Model) requestComments() tea.Cmd {
	tick := m.commentsCmd()
	it, ok := m.selected()
	if !ok {
		return tick
	}
	id := it.ID()
	if _, done := m.comments[id]; done {
		return tick
	}
	a := m.byForge[it.Forge]
	if a == nil {
		return tick
	}
	// Sin sesión no se pregunta: la ficha lo deduce del estado del forge, y
	// gastar una consulta para que vuelva a fallar no le dice nada al usuario que
	// no dijera ya la cabecera.
	if st := m.statuses[it.Forge]; st != nil && !st.auth.OK {
		return tick
	}

	// Se marca como pedida ANTES de salir. Si no, dos ticks seguidos sobre el
	// mismo ítem —el primero aún en vuelo, el segundo ya sin respuesta— lanzarían
	// la misma consulta dos veces.
	m.comments[id] = &commentState{}

	appCtx := m.ctx
	events := m.events
	ref, number := it.Ref, it.Number
	go func() {
		ctx, cancel := context.WithTimeout(appCtx, commentsTimeout)
		defer cancel()
		page, warns := a.Comments(ctx, ref, number)
		msg := commentsMsg{id: id, page: page}
		// Un warning solo se convierte en error si no vino nada. Si el forge
		// devolvió comentarios y además coleó algo, lo que se tiene es mejor que
		// dejar la ficha vacía por un aviso que no impide leer lo que sí llegó.
		if len(warns) > 0 && len(page.Comments) == 0 {
			msg.err = warns[0].Msg
		}
		sendEvent(appCtx, events, msg)
	}()
	return tick
}

// applyComments guarda la conversación de un ítem. No comprueba el ciclo: la
// respuesta es del ítem que se pidió, no de la vista, así que sigue siendo
// válida aunque el inbox se haya refrescado mientras volaba (misma política que
// applyAction con el estado releído).
func (m *Model) applyComments(msg commentsMsg) {
	st := &commentState{
		list:  msg.page.Comments,
		total: msg.page.Total,
		ready: true,
		err:   msg.err,
	}
	if st.total < len(st.list) {
		// GitLab no expone recuento: el total es lo leído. Se queda por lo menos
		// al alto de la lista para que la aritmética del recuento no invente que
		// hay más de los que se ven.
		st.total = len(st.list)
	}
	m.comments[msg.id] = st
}

// commentLines compone el bloque de comentarios del ítem para el hueco que sobra
// tras la ficha. `avail` son las filas que quedan; el bloque se queda en ellas o
// no se pinta.
//
// Cuando hay comentarios van en una caja propia titulada, no como campos más de la
// ficha: la conversación no es un dato del PR sino lo que la gente dijo de él, y un
// borde lo dice sin tener que explicarlo. Los estados sueltos (cargando, error,
// ninguno) se quedan como líneas de campo porque son mensajes de una línea, no
// conversación.
//
// El hueco se reparte entre los comentarios en vez de dárselo al primero: así los
// cinco se ven siempre y en un terminal alto se lee algo más que la primera
// línea de cada uno. Al revés, un comentario largo se comería el panel y la
// ficha enseñaría un comentario y un hueco.
func (m *Model) commentLines(it model.Item, avail, inner int) []string {
	if avail <= 0 {
		return nil
	}
	st := m.comments[it.ID()]
	switch {
	case st == nil:
		// Todavía no se ha preguntado y no hay nada en vuelo. Puede que el
		// siguiente tick ni siquiera lo pregunte: sin sesión no se pregunta, y ya
		// lo dice la cabecera. Poner "cargando…" aquí sería anunciar una consulta
		// que no existe.
		return nil
	case !st.ready:
		// La consulta está en vuelo. Se dice en vez de dejar un hueco en blanco:
		// un hueco no se distingue de "este PR no tiene comentarios" y aquí
		// todavía no se sabe.
		return []string{label("Comments", styleDim.Render("loading…"))}
	case st.err != "":
		return []string{label("Comments", styleWarn.Render(truncate("not read: "+st.err, max(1, inner-labelWidth))))}
	case len(st.list) == 0:
		// Sin comentarios no hay caja. La caja existe para separar la conversación
		// de los campos, y una caja alrededor de la palabra "none" no separa nada:
		// además es el estado de todos los PRs sin conversación, así que un borde
		// apareciendo y desapareciendo en cada movimiento del cursor es ruido.
		return []string{label("Comments", styleDim.Render("none"))}
	}

	// La caja no se pinta a medias. Un bloque con su borde de arriba y sin el de
	// abajo no es media caja, es ruido que ocupa lo mismo que el bloque entero: si
	// no caben los dos bordes, no cabe la conversación y se cae entera (ver la
	// escalera de degradación en detailLines).
	//
	// Y además tiene que caber TODA la conversación, no un trozo. La caja se come
	// dos filas, y una de las dos es borde: en un terminal de 30 filas, con tres
	// comentarios y tres filas de presupuesto, el bloque cabría sin caja para tres
	// comentarios y con caja para uno. Ver uno y perder los otros dos es peor que no
	// ver ninguno, porque un recorte de la caja no parece un recorte: parece que el
	// PR solo tiene ese comentario. Sin caja los pierde enteros, y el usuario ve la
	// ficha completa, que es lo que corresponde a un terminal corto.
	if avail < commentChrome+len(st.list) {
		return nil
	}
	budget := avail - commentChrome

	// Se acota aquí y no solo en el adapter. CommentLimit es una decisión de la
	// ficha, y una ficha que se la salta porque confió en quién la llenó enseñaría
	// comentarios que no caben en su propio alto.
	shown := st.list
	if len(shown) > forge.CommentLimit {
		shown = shown[:forge.CommentLimit]
	}
	// El mismo mínimo, medido ya sobre lo que se va a enseñar y no sobre lo que vino.
	if avail < commentChrome+len(shown) {
		return nil
	}

	// El cuerpo va dentro de la caja, así que el ancho de texto son dos runes
	// menos: los bordes verticales.
	bodyWidth := max(8, inner-commentBoxBorder)

	// Se cuenta cuántas filas necesita cada comentario componiéndolo con el tope
	// alto, que es el mismo código que lo pinta, así que el reparto no puede mentir
	// sobre lo que cabe. Componer dos veces es barato (cinco comentarios de texto) y
	// evita repartir a ciegas.
	need := make([]int, len(shown))
	total := 0
	for i, c := range shown {
		need[i] = len(commentBody(c, maxCommentLines, bodyWidth))
		total += need[i]
	}

	// El recuento NO va en el cuerpo: vive en el borde de abajo, a la derecha (ver
	// commentLegend). Aquí cada fila es una fila de lo que dijo la gente, y una de
	// recuento es una que no es de nadie; y en el borde no cuesta alto, así que no
	// es lo primero que se cae cuando el panel va justo, que era su destino.
	body := make([]string, 0, budget)

	// Si caben enteros, cada uno toma lo que necesita. Es lo que evita que un
	// comentario de seis párrafos se quede en "the timeout is 30x too high…" al lado
	// de cuatro de una línea, que es lo que pasaba con un reparto a ciegas.
	//
	// Y si no caben, a todos se les da una fila —para que los cinco estén presentes,
	// que es lo pedido— y el sobrante va a quién más tiene que perder. Es preferible
	// a darle el panel al primero, que se comería los cinco.
	rows := need
	if total > budget {
		rows = allocate(need, budget)
	}

	for i, c := range shown {
		lines := commentBody(c, max(1, rows[i]), bodyWidth)
		if len(body)+len(lines) > budget {
			// Red de seguridad: la comprobación de arriba ya garantiza que cada
			// comentario tiene al menos su fila, así que aquí no debería entrar. Si
			// entra, es que el reparto dio más de una fila a alguien y no se
			// descuadra la caja: se cae entera.
			return nil
		}
		body = append(body, lines...)
	}
	return commentBox(body, commentLegend(len(shown), st.total, inner), inner)
}

// commentChrome son las filas que cuesta la caja: borde de arriba y de abajo. Los
// dos títulos van embebidos en ellas —el "Comments" arriba y el recuento abajo—, así
// que ninguno gasta una más.
const commentChrome = 2

// commentBoxBorder son las columnas que se come la caja: un borde a cada lado.
const commentBoxBorder = 2

// commentTitle es el título de la caja. Va en el borde y no como etiqueta de
// campo: eso es justo lo que distingue este bloque de los datos de la ficha.
const commentTitle = "Comments"

// commentBox envuelve el bloque de comentarios en una caja redondeada titulada y
// con el recuento en el borde de abajo, y devuelve sus líneas sueltas. El ancho es
// el interior del panel de detalle, para que la caja quede dentro y no se pase del
// borde de la de fuera.
func commentBox(body []string, legend string, width int) []string {
	border := bordered.Rounded()
	// La leyenda no se apoya en la esquina: entre ella y la esquina se queda una raya
	// del propio borde. Sin ella, un "3" suelto con un hueco a cada lado hace que la
	// línea de abajo se lea como partida —no como un borde con algo escrito dentro—,
	// que es justo lo que se pierde al escribir en un borde.
	//
	// Y la raya va FUERA del estilo del recuento: dentro heredaría su gris y el
	// tramo que cierra la línea se vería de otro color que la línea que cierra, que
	// es la forma más obvia de delatar el truco.
	legend = " " + legend + " " + border.Bottom
	text := bordered.RenderWithTitles(
		border, commentBorderColor, " "+commentTitle+" ", bordered.AlignLeft,
		legend, bordered.AlignRight,
		strings.Join(body, "\n"), width,
	)
	return strings.Split(text, "\n")
}

// allocate reparte un presupuesto de filas entre comentarios que piden más de la
// que les toca. Cada uno arranca en una fila, que es lo mínimo para que se vea, y
// las sobrantes van una a una a quien menos tiene.
//
// Ir de una en una y no llenando primero a los más necesitados es lo que evita la
// arbitrariedad del orden: "el primero se lo queda" haría que un comentario de seis
// párrafos al principio se comiera el panel y uno igual de largo al final se quedara
// en su primera frase, y eso solo depende de quién escribió antes. Igualar niveles
// reparte el daño por igual entre los que lo van a sufrir, que es lo único que se
// puede repartir sin un criterio mejor.
//
// El presupuesto puede ser menor que el número de comentarios: entonces no caben
// todos y cada uno se queda con su fila mínima. Quien recorta el bloque es quien lo
// compone, con su propio tope de filas.
func allocate(need []int, budget int) []int {
	rows := make([]int, len(need))
	for i := range need {
		rows[i] = 1
	}
	for left := budget - len(need); left > 0; left-- {
		// El que menos cuota tiene y todavía le queda texto. Entre iguales gana el
		// de índice menor, para que el reparto no dependa del recorrido del mapa.
		best := -1
		for i := range need {
			if rows[i] >= need[i] {
				continue
			}
			if best < 0 || rows[i] < rows[best] {
				best = i
			}
		}
		if best < 0 {
			break // nadie puede absorber más
		}
		rows[best]++
	}
	return rows
}

// commentCount describe cuántos comentarios se ven de los que hay. El "5 de 23"
// es lo que dice que la ficha se está perdiendo conversación, que es el momento de
// abrir el PR. Con todos a la vista no hay nada que avisar y solo queda la cifra.
func commentCount(shown, total int) string {
	if total > shown {
		return fmt.Sprintf("%d of %d", shown, total)
	}
	return strconv.Itoa(shown)
}

// commentHint es lo que hace accionable el recuento: no dice solo que hay más
// conversación, dice dónde está. Va separada porque en un panel estrecho no cabe y
// entonces se cae ella y no los números.
const commentHint = " · open the PR to read the rest"

// legendGap son las columnas que la leyenda deja sin usar junto a la esquina: el
// hueco de texto a cada lado más la raya del propio borde que cierra la línea
// (ver commentBox). Sin eso el "3" se apoya en la esquina y la línea de abajo parece
// partida en vez de un borde con algo escrito dentro.
const legendGap = 3

// commentLegend compone el texto del recuento que va embebido en el borde de abajo,
// a la derecha, que es donde va: el cuerpo de la caja son las filas que dijo la
// gente, y una fila de recuento es una que no es de nadie. En el borde además es
// gratis, así que no es lo primero que se cae cuando el panel va justo.
//
// Solo dice "de N" cuando hay más comentarios de los que caben, que es lo único que
// la frase tiene que anunciar: con todo a la vista, la cifra ya está contando la
// conversación entera y un "3 de 3" no informa de nada.
//
// Lo que le cabe es el interior del borde menos las dos esquinas y el hueco que
// commentBox deja alrededor. La forma larga solo se usa si cabe entera: recortada a
// media frase ("5 of 23 · open the PR to read…") dice menos que la corta, que sigue
// siendo un "5 of 23" y además se lee como texto estropeado en vez de como el final
// de una frase.
func commentLegend(shown, total, width int) string {
	legend := commentCount(shown, total)
	if room := width - commentBoxBorder - legendGap; total > shown &&
		utf8.RuneCountInString(legend+commentHint) <= room {
		legend += commentHint
	}
	return styleCount.Render(legend)
}

// commentBody compone un comentario en como mucho `lines` filas: el autor en la
// primera y el cuerpo debajo, alineado bajo el texto.
//
// Se usa el cuerpo entero y no solo su primera línea. Con la primera línea, un
// comentario de tres párrafos ocupaba una fila de las cuatro que le tocaban y
// desperdiciaba las otras tres, que es justo el espacio que la rejilla de campos
// liberó para que los comentarios cupieran. Los párrafos se respetan como están
// (ver parse.CommentLines): envolverlos de corrido produciría "fix the timeout fix
// the backoff".
//
// Lo que no cabe se marca con "…", porque una fila que para en mitad de una frase
// se lee como si el comentario se acabara ahí.
func commentBody(c model.Comment, lines, inner int) []string {
	// Nadie se queda sin fila por un descuadre de reparto: una fila vacía es mejor
	// que un índice fuera de rango al marcar el corte.
	lines = max(1, lines)
	author := truncate(c.Author, maxCommentAuthor)

	// La primera fila lleva el autor delante, así que su cuerpo dispone de menos
	// ancho. Todo se mide en plano y sobre runes: el ancho que hay que rellenar es
	// el del texto, y cortar un texto multibyte por bytes deja un rune partido en
	// pantalla.
	first := max(8, inner-utf8.RuneCountInString(commentIndent+author+commentSep))
	cont := max(8, inner-utf8.RuneCountInString(contIndent))

	// Se guardan aparte el último trozo escrito y el ancho que tenía, para poder
	// recortarlo al marcar el corte sin volver a medir una fila ya vestida con
	// estilos: truncar la fila entera cortaría por la mitad de un código ANSI.
	var (
		out       []string
		lastPiece string
		lastW     int
	)
	for _, src := range parse.CommentLines(c.Body) {
		// Cada párrafo se parte al ancho de la fila que le toca, no al más ancho de
		// las dos: partirlo al ancho grande y recortarlo después cortaría palabras.
		w := cont
		if len(out) == 0 {
			w = first
		}
		for _, piece := range wrapText(src, w) {
			if len(out) >= lines {
				// Queda texto sin escribir: la última fila se vuelve a componer
				// marcando el corte, porque una fila que para en mitad de una frase
				// se lee como si el comentario se acabara ahí.
				out[len(out)-1] = commentRow(len(out)-1, author, commentSep, lastPiece, lastW, true)
				return out
			}
			out = append(out, commentRow(len(out), author, commentSep, piece, w, false))
			lastPiece, lastW = piece, w
		}
	}

	if len(out) == 0 {
		// Cuerpo vacío o solo boilerplate: se dice, para que la fila no se lea como
		// un comentario que no dice nada.
		return []string{commentRow(0, author, commentSep, "(no text)", first, false)}
	}
	return out
}

// commentRow compone una fila de comentario. La primera lleva el autor delante y
// las siguientes se sangran hasta donde empieza su texto, para que el cuerpo se
// lea como un bloque y no como trozos sueltos.
//
// cut marca que quedaba más texto detrás. No basta con recortar: si el último
// trozo cabía justo, se quedaría sin "…" y una fila que parece acabada dice que
// el comentario se acababa ahí. El hueco del "…" se descuenta del ancho para que
// la fila no se pase de la caja.
func commentRow(idx int, author, sep, piece string, w int, cut bool) string {
	indent, prefix := contIndent, ""
	if idx == 0 {
		indent, prefix = commentIndent, styleDetailKey.Render(author+sep)
	}
	runes := []rune(piece)
	switch n := utf8.RuneCountInString(piece); {
	case cut:
		room := max(1, w-1)
		if n <= room {
			return indent + prefix + piece + "…"
		}
		return indent + prefix + clipRunes(runes, room)
	case n > w:
		// Una palabra suelta más ancha que la caja: no hay por dónde partirla, así
		// que se recorta.
		return indent + prefix + clipRunes(runes, w)
	default:
		return indent + prefix + piece
	}
}

// clipRunes corta un texto a n runes marcando con "…" que se perdió más: sin la
// marca, una fila que para en media frase se lee como el final del comentario.
func clipRunes(runes []rune, n int) string {
	switch {
	case n >= len(runes):
		return string(runes)
	case n <= 0:
		return ""
	case n == 1:
		return "…"
	default:
		return string(runes[:n-1]) + "…"
	}
}

// maxCommentAuthor acota el ancho del autor en la primera fila. Un nombre de
// usuario largo no puede comerse el cuerpo del comentario, que es lo único que
// dice algo.
const maxCommentAuthor = 24

// Sangrado de los comentarios. Los separan de la ficha sin necesitar una línea en
// blanco, que en un panel de 18 filas es cara; y las filas siguientes se alinean
// con el texto de la primera para que el cuerpo se lea como un bloque.
const (
	commentIndent = "  "
	contIndent    = "    "
	// commentSep separa el autor de su texto.
	commentSep = ": "
)
