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
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/forge/parse"
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
// Se reparte el hueco entre los comentarios en vez de darle al primero todo lo que
// sobre: así los cinco se ven siempre, y en un terminal alto se lee algo más que
// la primera línea de cada uno. Al revés, un comentario largo se comería el
// panel y la ficha enseñaría un comentario y un hueco.
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
		// Todavía no se sabe.
		return []string{label("Comments", styleDim.Render("loading…"))}
	case st.err != "":
		return []string{label("Comments", styleWarn.Render(truncate("not read: "+st.err, max(1, inner-labelWidth))))}
	case len(st.list) == 0:
		return []string{label("Comments", styleDim.Render("none"))}
	}

	// Se acota aquí y no solo en el adapter. CommentLimit es una decisión de la
	// ficha, y una ficha que se la salta porque confió en quién la llenó enseñaría
	// comentarios que no caben en su propio alto.
	shown := st.list
	if len(shown) > forge.CommentLimit {
		shown = shown[:forge.CommentLimit]
	}

	out := []string{label("Comments", styleCount.Render(commentCount(len(shown), st.total)))}

	// Se cuenta cuántas filas necesita cada comentario componiéndolo con el tope
	// alto, que es el mismo código que lo pinta, así que el reparto no puede mentir
	// sobre lo que cabe. Componer dos veces es barato (cinco comentarios de texto) y
	// evita repartir a ciegas.
	need := make([]int, len(shown))
	total := 0
	for i, c := range shown {
		need[i] = len(commentBody(c, maxCommentLines, inner))
		total += need[i]
	}

	// Si caben enteros, cada uno toma lo que necesita. Es lo que evita que un
	// comentario de seis párrafos se quede en "the timeout is 30x too high…" al lado
	// de cuatro de una línea, que es lo que pasaba con un reparto a ciegas.
	//
	// Y si no caben, a todos se les da una fila —para que los cinco estén presentes,
	// que es lo pedido— y el sobrante va a quien más tiene que perder, ordenado por
	// lo que le falta. Es preferible a darle el panel al primero, que se comería
	// los cinco.
	rows := need
	if total > avail-1 {
		rows = allocate(need, avail-1)
	}

	for i, c := range shown {
		if len(out) >= avail {
			break
		}
		out = append(out, commentBody(c, max(1, rows[i]), inner)...)
	}
	return out
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
// abrir el PR. Con todos a la vista no hay nada que avisar y no se cuenta.
func commentCount(shown, total int) string {
	if total > shown {
		return fmt.Sprintf("%d of %d (open the PR to read the rest)", shown, total)
	}
	return fmt.Sprintf("%d", shown)
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
