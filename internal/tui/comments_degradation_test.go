package tui

import (
	"strings"
	"testing"
	"time"

	"prdash/internal/cache"
	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

// comentariosDe arma un modelo con `n` comentarios ya cargados, que es el estado en
// el que la ficha compone: la consulta en vuelo y el error se prueban en otros lados.
func comentariosDe(t *testing.T, n int) (Model, model.Item) {
	t.Helper()
	m := newTestModel(t)
	it := mkItem("github", "github.com", "acme/widget", "uno", 1, "")
	list := make([]model.Comment, n)
	for i := range list {
		list[i] = model.Comment{
			Author: "alice",
			Body:   "linea " + strings.Repeat("x", i+1),
		}
	}
	m.comments[it.ID()] = &commentState{list: list, total: n, ready: true}
	return m, it
}

// TestCommentLinesSinEspacioNoPintaLaCaja: sin filas, no hay caja.
//
// Y no es "una caja vacía": es NADA. La diferencia se ve, porque una caja de
// comentarios vacía es un marco en la pantalla que no separa nada y que aparece y
// desaparece al mover el cursor. Con altura cero la ficha se queda con los campos y
// ya.
//
// El borde de esta guarda es el cero EXACTO, que es lo que se ve cuando el
// presupuesto de la cabecera se lo ha comido todo: un `avail` de 0 llega aquí, y
// tiene que salir por la puerta.
func TestCommentLinesSinEspacioNoPintaLaCaja(t *testing.T) {
	m, it := comentariosDe(t, 3)
	for _, avail := range []int{-100, -1, 0} {
		if got := m.commentLines(it, avail, 60); got != nil {
			t.Errorf("con %d filas disponibles dio %d líneas, want nil: sin filas no hay caja", avail, len(got))
		}
	}
	// Y con una sola fila tampoco: la caja necesita su marco y una fila de texto, y
	// un marco con una línea dentro no es una caja de comentarios.
	for _, avail := range []int{1, 2} {
		if got := m.commentLines(it, avail, 60); got != nil {
			t.Errorf("con %d filas dio %d líneas: la caja necesita su marco más texto", avail, len(got))
		}
	}
	// Y con hueco de verdad hay caja, y dice algo.
	if got := m.commentLines(it, 20, 60); len(got) == 0 {
		t.Error("con 20 filas disponibles no salió caja ninguna")
	}
}

// TestCommentLinesElTopeDeCincoMandaAunqueLaCajaSeaGrande: la caja enseña cinco
// comentarios como máximo, y el recorte no depende de lo grande que sea.
//
// Es la misma regla que en el adapter, y está aquí a propósito: la ficha no confía en
// quien la llenó. Un forge que devuelve veinte comentarios no puede hacer que la caja
// se coma el panel entero, porque el tope está en el lado que pinta.
//
// Y el caso de la franja, que es el que la campaña anterior dio por no comprobable y
// no lo era: con MÁS de cinco comentarios y un presupuesto que da para cinco y no
// para todos, el recorte tiene que pasar por el `CommentLimit` y no por la falta de
// sitio. Son dos guardas distintas y se distinguen por SI la caja sale.
func TestCommentLinesElTopeDeCincoMandaAunqueLaCajaSeaGrande(t *testing.T) {
	// Con hueco de sobra y más de cinco comentarios: la caja sale y enseña cinco.
	m, it := comentariosDe(t, 20)
	grande := m.commentLines(it, 200, 60)
	if len(grande) == 0 {
		t.Fatal("con 200 filas y 20 comentarios no salió caja: el recorte se tragó la caja entera")
	}
	// Y la caja dice cuántos hay de los que se ven, o el recorte sería invisible.
	plano := stripANSI(strings.Join(grande, "\n"))
	if !strings.Contains(plano, "5") {
		t.Errorf("la caja con 20 comentarios y hueco de sobra no dice cuántos enseña:\n%s", plano)
	}

	// La franja: hueco para la caja con cinco, pero no para los veinte. Aquí la caja
	// tiene que SALIR con cinco, y no desaparecer.
	// El presupuesto justo es el marco más lo que necesitan cinco comentarios de una
	// línea; con una fila menos la caja no cabe con veinte pero sí con cinco.
	justo := presupuestoJusto(t, m, it, forge.CommentLimit)
	enLaFranja := m.commentLines(it, justo, 60)
	if len(enLaFranja) == 0 {
		t.Fatalf("con presupuesto justo para %d comentarios la caja desapareció, y debería "+
			"salierse recortada: hay hueco para cinco", forge.CommentLimit)
	}
	// Y no enseña más de cinco, que es lo que se está probando.
	if n := cuentaComentarios(plano); n > forge.CommentLimit {
		t.Errorf("enseñó %d comentarios con un presupuesto para %d", n, forge.CommentLimit)
	}

	// Con una fila menos: ni cinco ni veinte. Aquí el que decide es la falta de sitio.
	unPocaMenos := justo - 1
	if got := m.commentLines(it, unPocaMenos, 60); got != nil {
		t.Errorf("con un presupuesto de %d filas la caja se pintó con %d líneas: no cabe",
			unPocaMenos, len(got))
	}
}

// presupuestoJusto busca el presupuesto mínimo con el que la caja sale, para poder
// afirmar justo por encima y justo por debajo sin hacer cuentas a mano.
func presupuestoJusto(t *testing.T, m Model, it model.Item, n int) int {
	t.Helper()
	for avail := 1; avail <= 300; avail++ {
		if len(m.commentLines(it, avail, 60)) > 0 {
			return avail
		}
	}
	t.Fatalf("con %d comentarios la caja no sale ni con 300 filas", n)
	return 0
}

// cuentaComentarios cuenta las filas del bloque que llevan un autor, que es como se
// distingue un comentario de la leyenda y del marco.
func cuentaComentarios(join string) int {
	n := 0
	for _, l := range strings.Split(join, "\n") {
		if strings.Contains(l, "alice") {
			n++
		}
	}
	return n
}

// TestCommentBoxWidthDescuentaElSangradoDeLosDosLados: el ancho exterior de la caja es
// el del hueco disponible menos el sangrado de los dos lados, con suelo.
//
// Se comprueba `commentBoxWidth` DIRECTO y no solo a través de `commentBodyWidth`, que
// es como estaba. La diferencia no es academica: `commentBodyWidth` compone las dos
// funciones, así que un error en `commentBoxWidth` se propaga y se ve como un ancho de
// texto raro. Mirándola sola, el error es un error de sangrado, que es lo que es.
func TestCommentBoxWidthDescuentaElSangradoDeLosDosLados(t *testing.T) {
	for outer := -20; outer <= 200; outer++ {
		got := commentBoxWidth(outer)
		want := max(8, outer-2*commentInset)
		if got != want {
			t.Errorf("commentBoxWidth(%d) = %d, want %d", outer, got, want)
		}
		// El suelo nunca deja bajar de 8: por debajo no hay ni icono ni texto.
		if got < 8 {
			t.Errorf("commentBoxWidth(%d) = %d, want >= 8", outer, got)
		}
	}
	// El sangrado es de LOS DOS LADOS, y por eso el descuento es el doble. Con uno
	// solo, la caja se quedaría una columna más ancha por un lado, que es
	// exactamente lo que hace que el borde de fuera se vea desplazado.
	for outer := 20; outer <= 120; outer++ {
		if got := commentBoxWidth(outer); got != outer-2*commentInset {
			t.Errorf("commentBoxWidth(%d) = %d, want %d (dos lados de sangrado)",
				outer, got, outer-2*commentInset)
		}
	}
	// Y el suelo empieza exactamente donde el descuento se come el mínimo.
	if got := commentBoxWidth(8 + 2*commentInset); got != 8 {
		t.Errorf("commentBoxWidth(%d) = %d, want 8: el suelo empieza aquí", 8+2*commentInset, got)
	}
	if got := commentBoxWidth(7 + 2*commentInset); got != 8 {
		t.Errorf("commentBoxWidth(%d) = %d, want 8 (el suelo manda)", 7+2*commentInset, got)
	}
	// Y a partir de ahí, uno por uno.
	for outer := 8 + 2*commentInset; outer <= 40; outer++ {
		if got := commentBoxWidth(outer); got != outer-2*commentInset {
			t.Errorf("commentBoxWidth(%d) = %d, want %d", outer, got, outer-2*commentInset)
		}
	}
}

// TestElRecuentoNuncaEsMenorQueLoQueSeVe: el total del forge no puede ser menor que la
// lista que se está enseñando.
//
// GitLab no expone el recuento, así que el total es lo leído. Y aun asi, si el total
// fuera menor que la lista, el rótulo diría "2 de 1", que es un número que no existe.
// En la redacción, menos no es "menos": es un dato que no cuadra.
//
// O sea que la condición se lee al revés de lo que parece: se sube el total para que
// nunca sea menos, y subir un total es inocuo porque solo se usa para el rótulo.
func TestElRecuentoNuncaEsMenorQueLoQueSeVe(t *testing.T) {
	// Lo que hace la aritmética del recuento, sin el mensaje entero: el total que se
	// acaba enseñando.
	totalFinal := func(total, lista int) int {
		if total < lista {
			return lista
		}
		return total
	}
	for total := -3; total <= 30; total++ {
		for lista := 0; lista <= 30; lista++ {
			if got := totalFinal(total, lista); got < lista {
				t.Errorf("total %d con lista %d dio un total final de %d, que es MENOR que la lista",
					total, lista, got)
			}
			// Y arriba no se toca: un total mayor es un total que el forge dijo.
			if total >= lista && totalFinal(total, lista) != total {
				t.Errorf("total %d con lista %d dio %d, want el total del forge: "+
					"arriba no se corrige", total, lista, totalFinal(total, lista))
			}
			if lista == 0 && totalFinal(total, lista) != max(total, 0) {
				t.Errorf("con lista vacía y total %d dio %d", total, totalFinal(total, lista))
			}
		}
	}
	// Y el caso que lo provoca: sin lista, un total de 0 es un total de 0, no menos.
	if got := totalFinal(-5, 0); got != 0 {
		t.Errorf("con total -5 y lista vacía dio %d, want 0", got)
	}
}

// TestElBloqueDeComentariosNoEsMasAltoQueElPresupuesto: el bloque que sale mide como
// mucho las filas que se le dieron, y el presupuesto es lo que sobra tras el marco.
//
// Esta es la invariante que hace que el presupuesto signifique algo. Con dos filas de
// más, el bloque se sale del detalle, y como el detalle se recorta por arriba —que es
// donde está el final de la ficha, el que dice si la acción procede—, lo que se pierde
// es el final y no el principio. O sea: un presupuesto mal calculado no se ve como un
// bloque más alto, se ve como una ficha a la que le falta el final.
//
// Por eso se afirma sobre TODOS los presupuestos y no solo sobre el justo: el justo
// es donde la caja aparece, y el error de dos filas se nota justo por encima de él.
func TestElBloqueDeComentariosNoEsMasAltoQueElPresupuesto(t *testing.T) {
	// Comentarios de UNA línea, que es lo que hay: con una línea cada uno, el
	// presupuesto no aprieta nunca, porque siempre sobra. El presupuesto solo
	// importa cuando los comentarios PIDEN más filas de las que hay, así que el
	// recorrido interesante es el de los largos.
	for _, n := range []int{1, 2, 3, 5, 6, 12, 40} {
		m, it := comentariosDe(t, n)
		for avail := 1; avail <= 60; avail++ {
			got := m.commentLines(it, avail, 60)
			if len(got) > avail {
				t.Errorf("%d comentarios cortos con %d filas dio un bloque de %d, want <= %d",
					n, avail, len(got), avail)
			}
		}
	}

	// Y ahora los largos, que es donde el presupuesto manda. Cada comentario pide
	// varias filas, así que hay un punto en el que empieza a repartir y ese punto se
	// mueve con cada fila de presupuesto.
	for _, lineas := range []int{1, 2, 4, 8} {
		for _, n := range []int{1, 3, 5, 9} {
			m := newTestModel(t)
			it := mkItem("github", "github.com", "acme/widget", "uno", 1, "")
			cuerpo := strings.TrimSpace(strings.Repeat("palabra "+strings.Repeat("y", 5)+"\n", lineas))
			list := make([]model.Comment, n)
			for i := range list {
				list[i] = model.Comment{Author: "alice", Body: cuerpo}
			}
			m.comments[it.ID()] = &commentState{list: list, total: n, ready: true}

			for avail := 1; avail <= 60; avail++ {
				got := m.commentLines(it, avail, 60)
				if len(got) > avail {
					t.Errorf("%d comentarios de %d líneas con %d filas dio un bloque de %d, want <= %d. "+
						"El presupuesto se descuenta del marco una vez, y dos filas de más se comen "+
						"el final de la ficha, que es el que dice si la acción procede",
						n, lineas, avail, len(got), avail)
				}
			}
		}
	}
	// Y el caso bueno: con hueco de sobra, el bloque se queda con lo que necesita y
	// NO se estira. Rellenar un panel vacío con una caja estirada es inventar altura
	// que no tiene, y se lee como comentarios que no se han escrito.
	m, it := comentariosDe(t, 2)
	pequeno := len(m.commentLines(it, 30, 60))
	enorme := len(m.commentLines(it, 300, 60))
	if pequeno != enorme {
		t.Errorf("con 30 filas dio un bloque de %d y con 300 dio de %d: "+
			"la caja toma lo que necesita y no se estira", pequeno, enorme)
	}
	// Y un presupuesto enorme no hace que aparezcan MÁS comentarios que los que hay.
	if enorme > 2+commentChrome+8 {
		t.Errorf("con 2 comentarios y 300 filas el bloque mide %d filas: hay caja estirada", enorme)
	}
}

// TestElErrorDeComentariosSeRecortaAlAnchoUtil: el texto del error va en la misma
// línea que los campos, y se recorta al hueco que queda.
//
// La línea es `label + valor`, y el hueco del valor es el ancho de la línea menos el
// de la etiqueta. Lo que se afirma es que el recorte usa ESE hueco, no el del marco
// entero: un valor que se pasa del hueco pisa el borde, y uno que se queda corto
// desperdicia el ancho que sí había.
//
// Y hay que mirar el valor YA VESTIDO, con el color puesto, porque un recorte por
// bytes dejaría un código de color partido y el texto se vería con el color a medias.
func TestElErrorDeComentariosSeRecortaAlAnchoUtil(t *testing.T) {
	m, it := comentariosDe(t, 1)
	m.comments[it.ID()].err = "el servidor dijo algo muy largo que no cabe en ninguna línea de la terminal"

	for inner := 38; inner <= 200; inner++ {
		lineas := m.commentLines(it, 20, inner)
		if len(lineas) == 0 {
			t.Fatalf("con inner %d no salió ninguna línea", inner)
		}
		plano := stripANSI(lineas[len(lineas)-1])
		if ancho := len([]rune(plano)); ancho > inner {
			t.Errorf("con inner %d la línea del error mide %d: %q",
				inner, ancho, plano)
		}
		// Y con un error corto, la línea entera cabe y no se toca.
		m.comments[it.ID()].err = "boom"
		plano = stripANSI(m.commentLines(it, 20, inner)[0])
		if !strings.Contains(plano, "boom") {
			t.Errorf("con inner %d se perdió el mensaje de error: %q", inner, plano)
		}
	}

	// Y el borde de verdad: el error se recorta, no se pierde. Con un error largo y un
	// ancho pequeño tiene que quedar un trozo, y ese trozo tiene que ser del principio
	// del mensaje, que es donde está el motivo.
	m.comments[it.ID()].err = "motivo-del-error-y-despues-ruido-que-no-cabe"
	plano := stripANSI(m.commentLines(it, 20, 38)[0])
	if !strings.Contains(plano, "motivo") {
		t.Errorf("con un error largo y un ancho pequeño se perdió el motivo: %q", plano)
	}
	// Y el prefijo que explica de qué es el error, siempre. Es lo que distingue un
	// fallo de lectura de un comentario.
	m.comments[it.ID()].err = "boom"
	plano = stripANSI(m.commentLines(it, 20, 200)[0])
	if !strings.Contains(plano, "not read") {
		t.Errorf("la línea del error no dice que es un fallo de lectura: %q", plano)
	}
}

// TestUnWarningSoloEsErrorSiNoVinoNada: un warning con comentarios NO es un error.
//
// La regla es "solo se convierte en error si no vino nada", y las dos mitades
// importan por separado:
//
//   - con comentarios y warning, se enseñan los comentarios. Lo que se tiene es mejor
//     que dejar la ficha vacía por un aviso que no impide leer lo que sí llegó. Con un
//     PR que tiene veinte comentarios y un warning de "no pude leer el primer comentario
//     del todo", tirar los veinte por un warning es la peor de las opciones.
//   - sin comentarios y con warning, el warning ES el error. Porque si no, no hay nada
//     que enseñar y la ficha se queda con un hueco en blanco que no se distingue de
//     "este PR no tiene conversación", que es una mentira.
//
// Se prueba por el camino real: la consulta sale en una goroutine y el resultado llega
// por el canal de eventos, que es el mismo que usa la TUI. Lo que se afirma es lo que
// acaba en la ficha, que es donde se ve.
func TestUnWarningSoloEsErrorSiNoVinoNada(t *testing.T) {
	for _, c := range []struct {
		nombre      string
		comentarios []model.Comment
		warn        string
		quiereErr   bool
	}{
		{
			"con comentarios y warning",
			[]model.Comment{{Author: "alice", Body: "hola"}},
			"no pude leer un comentario",
			false,
		},
		{
			"sin comentarios y con warning",
			nil,
			"no pude leer la conversación",
			true,
		},
		{
			"sin comentarios y sin warning",
			nil,
			"",
			false,
		},
		{
			"con comentarios y sin warning",
			[]model.Comment{{Author: "alice", Body: "hola"}},
			"",
			false,
		},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			clave := "acme/widget#1"
			f := &testutil.FakeAdapter{
				ForgeName: "github",
				HostName:  "github.com",
				Conversations: map[string]forge.CommentPage{
					clave: {Comments: c.comentarios},
				},
			}
			if c.warn != "" {
				f.CommentWarnings = map[string][]model.Warning{
					clave: {{Forge: "github", Kind: "degraded", Msg: c.warn}},
				}
			}
			m := newTestModel(t, f)
			m.applySnapshot(cache.File{Streams: []cache.Stream{{
				Forge: "github", Host: "github.com", Section: model.SectionReview,
				Kind:  model.ReviewRequested,
				Items: []model.Item{mkItem("github", "github.com", "acme/widget", "uno", 1, "")},
			}}})
			m.rebuild()
			m.statuses["github"].auth.OK = true

			it, ok := m.selected()
			if !ok {
				t.Fatal("no hay ítem seleccionado")
			}
			_ = m.requestComments()

			// Se espera el resultado con techo: la consulta va en una goroutine y
			// "no ha llegado" no se sabe mirando una vez.
			var msg commentsMsg
			for range 400 {
				select {
				case ev := <-m.events:
					if c, ok := ev.(commentsMsg); ok && c.id == it.ID() {
						msg = c
					}
				case <-time.After(5 * time.Millisecond):
				}
				if msg.id == it.ID() {
					break
				}
			}
			if msg.id != it.ID() {
				t.Fatal("no llegó el resultado de la consulta de comentarios")
			}
			if (msg.err != "") != c.quiereErr {
				t.Errorf("con %d comentarios y warning %q dio err=%q, quiere error=%v",
					len(c.comentarios), c.warn, msg.err, c.quiereErr)
			}
			// Y si no es error, la conversación está: la de verdad y la que vino, sin
			// haber tirado nada.
			if !c.quiereErr && len(c.comentarios) > 0 && len(msg.page.Comments) != len(c.comentarios) {
				t.Errorf("con warning y comentarios llegaron %d de %d: un warning no tira lo que llegó",
					len(msg.page.Comments), len(c.comentarios))
			}
		})
	}
}
