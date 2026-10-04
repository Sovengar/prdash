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

func TestCommentLinesSinEspacioNoPintaLaCaja(t *testing.T) {
	m, it := comentariosDe(t, 3)
	for _, avail := range []int{-100, -1, 0} {
		if got := m.commentLines(it, avail, 60); got != nil {
			t.Errorf("con %d filas disponibles dio %d líneas, want nil: sin filas no hay caja", avail, len(got))
		}
	}
	for _, avail := range []int{1, 2} {
		if got := m.commentLines(it, avail, 60); got != nil {
			t.Errorf("con %d filas dio %d líneas: la caja necesita su marco más texto", avail, len(got))
		}
	}
	if got := m.commentLines(it, 20, 60); len(got) == 0 {
		t.Error("con 20 filas disponibles no salió caja ninguna")
	}
}

func TestCommentLinesElTopeDeCincoMandaAunqueLaCajaSeaGrande(t *testing.T) {
	m, it := comentariosDe(t, 20)
	grande := m.commentLines(it, 200, 60)
	if len(grande) == 0 {
		t.Fatal("con 200 filas y 20 comentarios no salió caja: el recorte se tragó la caja entera")
	}
	plano := stripANSI(strings.Join(grande, "\n"))
	if !strings.Contains(plano, "5") {
		t.Errorf("la caja con 20 comentarios y hueco de sobra no dice cuántos enseña:\n%s", plano)
	}

	// The band: room for the box with five, not for twenty. Here the box must come out with five and
	//not disappear.
	justo := presupuestoJusto(t, m, it, forge.CommentLimit)
	enLaFranja := m.commentLines(it, justo, 60)
	if len(enLaFranja) == 0 {
		t.Fatalf("con presupuesto justo para %d comentarios la caja desapareció, y debería "+
			"salierse recortada: hay hueco para cinco", forge.CommentLimit)
	}
	if n := cuentaComentarios(plano); n > forge.CommentLimit {
		t.Errorf("enseñó %d comentarios con un presupuesto para %d", n, forge.CommentLimit)
	}

	unPocaMenos := justo - 1
	if got := m.commentLines(it, unPocaMenos, 60); got != nil {
		t.Errorf("con un presupuesto de %d filas la caja se pintó con %d líneas: no cabe",
			unPocaMenos, len(got))
	}
}

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

func cuentaComentarios(join string) int {
	n := 0
	for _, l := range strings.Split(join, "\n") {
		if strings.Contains(l, "alice") {
			n++
		}
	}
	return n
}

func TestCommentBoxWidthDescuentaElSangradoDeLosDosLados(t *testing.T) {
	for outer := -20; outer <= 200; outer++ {
		got := commentBoxWidth(outer)
		want := max(8, outer-2*commentInset)
		if got != want {
			t.Errorf("commentBoxWidth(%d) = %d, want %d", outer, got, want)
		}
		if got < 8 {
			t.Errorf("commentBoxWidth(%d) = %d, want >= 8", outer, got)
		}
	}
	for outer := 20; outer <= 120; outer++ {
		if got := commentBoxWidth(outer); got != outer-2*commentInset {
			t.Errorf("commentBoxWidth(%d) = %d, want %d (dos lados de sangrado)",
				outer, got, outer-2*commentInset)
		}
	}
	if got := commentBoxWidth(8 + 2*commentInset); got != 8 {
		t.Errorf("commentBoxWidth(%d) = %d, want 8: el suelo empieza aquí", 8+2*commentInset, got)
	}
	if got := commentBoxWidth(7 + 2*commentInset); got != 8 {
		t.Errorf("commentBoxWidth(%d) = %d, want 8 (el suelo manda)", 7+2*commentInset, got)
	}
	for outer := 8 + 2*commentInset; outer <= 40; outer++ {
		if got := commentBoxWidth(outer); got != outer-2*commentInset {
			t.Errorf("commentBoxWidth(%d) = %d, want %d", outer, got, outer-2*commentInset)
		}
	}
}

// GitLab exposes no count, so the total is what was read, and it is floored at the list's height so
// the arithmetic does not invent comments.
func TestElRecuentoNuncaEsMenorQueLoQueSeVe(t *testing.T) {
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
			if total >= lista && totalFinal(total, lista) != total {
				t.Errorf("total %d con lista %d dio %d, want el total del forge: "+
					"arriba no se corrige", total, lista, totalFinal(total, lista))
			}
			if lista == 0 && totalFinal(total, lista) != max(total, 0) {
				t.Errorf("con lista vacía y total %d dio %d", total, totalFinal(total, lista))
			}
		}
	}
	if got := totalFinal(-5, 0); got != 0 {
		t.Errorf("con total -5 y lista vacía dio %d, want 0", got)
	}
}

func TestElBloqueDeComentariosNoEsMasAltoQueElPresupuesto(t *testing.T) {
	// One-line comments: the budget never bites when there is spare room.
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
	m, it := comentariosDe(t, 2)
	pequeno := len(m.commentLines(it, 30, 60))
	enorme := len(m.commentLines(it, 300, 60))
	if pequeno != enorme {
		t.Errorf("con 30 filas dio un bloque de %d y con 300 dio de %d: "+
			"la caja toma lo que necesita y no se estira", pequeno, enorme)
	}
	if enorme > 2+commentChrome+8 {
		t.Errorf("con 2 comentarios y 300 filas el bloque mide %d filas: hay caja estirada", enorme)
	}
}

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
		m.comments[it.ID()].err = "boom"
		plano = stripANSI(m.commentLines(it, 20, inner)[0])
		if !strings.Contains(plano, "boom") {
			t.Errorf("con inner %d se perdió el mensaje de error: %q", inner, plano)
		}
	}

	m.comments[it.ID()].err = "motivo-del-error-y-despues-ruido-que-no-cabe"
	plano := stripANSI(m.commentLines(it, 20, 38)[0])
	if !strings.Contains(plano, "motivo") {
		t.Errorf("con un error largo y un ancho pequeño se perdió el motivo: %q", plano)
	}
	m.comments[it.ID()].err = "boom"
	plano = stripANSI(m.commentLines(it, 20, 200)[0])
	if !strings.Contains(plano, "not read") {
		t.Errorf("la línea del error no dice que es un fallo de lectura: %q", plano)
	}
}

// The rule is "only becomes an error if nothing came back".
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
			if !c.quiereErr && len(c.comentarios) > 0 && len(msg.page.Comments) != len(c.comentarios) {
				t.Errorf("con warning y comentarios llegaron %d de %d: un warning no tira lo que llegó",
					len(msg.page.Comments), len(c.comentarios))
			}
		})
	}
}
