package tui

import (
	"strings"
	"testing"

	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

func TestSinFilasNoSePintaNiUnAvisoDeCarga(t *testing.T) {
	adapt := &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"}

	estados := []struct {
		nombre string
		st     *commentState
	}{
		{"sin preguntar", &commentState{}},
		{"cargando", &commentState{list: []model.Comment{{Author: "a", Body: "b"}}}},
		{"con error", &commentState{ready: true, err: "boom"}},
		{"sin comentarios", &commentState{ready: true}},
		{"listo con comentarios", &commentState{ready: true,
			list: []model.Comment{{Author: "a", Body: "b"}}}},
	}

	for _, e := range estados {
		for _, avail := range []int{-5, 0} {
			m := newTestModel(t, adapt)
			it := mkItem("github", "github.com", "acme/widget", "uno", 1, "")
			st := *e.st
			m.comments[it.ID()] = &st

			if got := m.commentLines(it, avail, m.contentWidth()); len(got) != 0 {
				t.Errorf("estado %q con %d filas dio %d líneas (%q): sin filas no hay "+
					"caja, y un texto suelto no es una caja. Con la guarda puesta como "+
					"`avail < 0` el switch de estados devuelve cargando/error antes de "+
					"llegar a la comprobación que la absorbe",
					e.nombre, avail, len(got),
					primeraLineaCon(stripANSI(strings.Join(got, " ")), 80))
			}
		}
	}

	// This is what separates `avail <= 0` from `avail <= 1`: with the floor at one, a single row still
	//has room.
	conTexto := estados[1:4]
	for _, e := range conTexto {
		for _, avail := range []int{1, 2} {
			m := newTestModel(t, adapt)
			it := mkItem("github", "github.com", "acme/widget", "uno", 1, "")
			st := *e.st
			m.comments[it.ID()] = &st

			if got := m.commentLines(it, avail, m.contentWidth()); len(got) == 0 {
				t.Errorf("estado %q con %d filas no dio ninguna línea: los estados "+
					"sueltos son de una línea y con una fila hay sitio para ellos. Con "+
					"el suelo en uno se pierde, y una fila perdida de un estado suelto "+
					"hace que no se vea que el PR tiene conversación",
					e.nombre, avail)
			}
		}
	}
}

func TestLaAnchuraDeLaCajaDescuentaElSangradoYTieneSuelo(t *testing.T) {
	for _, outer := range []int{10, 11, 20, 60, 100, 200} {
		want := outer - 2*commentInset
		if got := commentBoxWidth(outer); got != want {
			t.Errorf("con un hueco de %d columnas la caja mide %d, want %d: el sangrado "+
				"es de %d por lado y son dos lados", outer, got, want, commentInset)
		}
	}

	for _, outer := range []int{-10, 0, 5, 9} {
		got := commentBoxWidth(outer)
		if got != 8 {
			t.Errorf("con un hueco de %d columnas la caja mide %d, want 8: por debajo del "+
				"suelo sale el suelo, porque un nombre de autor que no cabe ni truncado "+
				"hace la caja ilegible", outer, got)
		}
	}
}

// "In its place" is the part that matters: the last row is recomposed, not clipped.
func TestLaFilaDelCorteSeVuelveAComponerEnSuSitio(t *testing.T) {
	adapt := &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"}
	m := newTestModel(t, adapt)
	it := mkItem("github", "github.com", "acme/widget", "uno", 1, "")

	parrafos := make([]string, 10)
	for i := range parrafos {
		parrafos[i] = strings.Repeat("palabra de relleno para la frase ", 4)
	}
	cuerpo := strings.Join(parrafos, "\n\n")
	const inner = 40
	m.comments[it.ID()] = &commentState{ready: true, list: []model.Comment{
		{Author: "alice", Body: cuerpo},
	}, total: 1}

	for _, filas := range []int{1, 2, 3, 4, 5, 6} {
		got := m.commentLines(it, commentChrome+filas, inner)
		if len(got) == 0 {
			t.Fatalf("con %d filas no salió bloque ninguno", filas)
		}
		contenido := len(got) - commentChrome
		if contenido > filas {
			t.Errorf("con %d filas el bloque pintó %d de contenido: se pasó de largo",
				filas, contenido)
		}
		plano := stripANSI(strings.Join(got, "\n"))
		if !strings.Contains(plano, "…") {
			t.Errorf("con %d filas el bloque sale sin marca de corte: %q", filas,
				primeraLineaCon(plano, 160))
		}
		// The LAST row of the CONTENT, not of the box: the box's last row is the bottom border with the
		//legend.
		conLasMarcas := []string{}
		for _, l := range strings.Split(plano, "\n") {
			if strings.Contains(l, "…") {
				conLasMarcas = append(conLasMarcas, l)
			}
		}
		if len(conLasMarcas) != 1 {
			t.Errorf("con %d filas hay %d filas con la marca de corte, want 1: el corte "+
				"se marca una vez, en la última fila que se pintó", filas, len(conLasMarcas))
			continue
		}
		ultimaDeTexto := ""
		for _, l := range strings.Split(plano, "\n") {
			if strings.Contains(l, "─") {
				continue
			}
			ultimaDeTexto = l
		}
		if !strings.Contains(ultimaDeTexto, "…") {
			t.Errorf("con %d filas la marca de corte quedó en %q y la última fila de "+
				"texto es %q: la marca tiene que caer en la última fila que se pintó",
				filas, conLasMarcas[0], primeraLineaCon(ultimaDeTexto, 90))
		}

		// commentRow only branches on `idx == 0`, so the index passed to the recompose decides whether
		//that row still carries the author's name.
		primeraDeTexto := ""
		for _, l := range strings.Split(plano, "\n") {
			if strings.Contains(l, "─") {
				continue
			}
			primeraDeTexto = l
			break
		}
		if !strings.Contains(primeraDeTexto, "alice") {
			t.Errorf("con %d filas la primera fila de texto es %q y no lleva el autor: "+
				"es la fila cero y solo la fila cero lo lleva", filas,
				primeraLineaCon(primeraDeTexto, 90))
		}
		if filas > 1 && strings.Contains(ultimaDeTexto, "alice") {
			t.Errorf("con %d filas la última fila lleva también el autor: %q. Con más "+
				"de una fila solo la primera lo lleva, y verlo dos veces hace pensar "+
				"que hay dos comentarios", filas, primeraLineaCon(ultimaDeTexto, 90))
		}
	}
}

func TestElRepartoEsElMismoCuandoElPresupuestoDaExacto(t *testing.T) {
	casos := []struct {
		need   []int
		budget int
		nota   string
	}{
		{[]int{1, 1, 1}, 3, "tres de una fila con presupuesto exacto"},
		{[]int{1, 5}, 6, "uno corto y uno largo, presupuesto exacto"},
		{[]int{5, 1}, 6, "el largo primero: el reparto no depende del orden"},
		{[]int{1, 1, 1, 1, 1}, 5, "los cinco de una fila"},
		{[]int{3, 3, 3}, 9, "tres iguales con presupuesto exacto"},
		{[]int{10, 1, 1, 1}, 13, "uno enorme y tres de una"},
		{[]int{2, 2, 2, 2, 2}, 10, "cinco de dos filas"},
	}
	for _, c := range casos {
		got := allocate(c.need, c.budget)
		if len(got) != len(c.need) {
			t.Errorf("%s: allocate devolvió %d filas para %d comentarios",
				c.nota, len(got), len(c.need))
			continue
		}
		for i := range got {
			if got[i] != c.need[i] {
				t.Errorf("%s: allocate devolvió %v, want %v. Con el presupuesto justo "+
					"el reparto tiene que ser una identidad, o el `>` del reparto necesita "+
					"un `>=` que no tiene", c.nota, got, c.need)
				break
			}
		}
	}
}

// allocate starts at one row per comment, so a comment asking for ZERO rows would be left with a
// dangling index.
func TestNadiePideCeroFilas(t *testing.T) {
	cuerpos := []model.Comment{
		{Author: "alice"},
		{Author: "alice", Body: ""},
		{Author: "alice", Body: "\n\n\n"},
		{Author: "alice", Body: "   \n  \n"},
		{Author: "", Body: ""},
		{Author: "alice", Body: "una linea"},
	}
	for _, c := range cuerpos {
		for _, lineas := range []int{1, 2, 5, 20} {
			for _, ancho := range []int{12, 38, 60} {
				got := commentBody(c, lineas, commentBodyWidth(ancho))
				if len(got) == 0 {
					t.Errorf("comentario %+v con %d filas y ancho %d devolvió NADA: una "+
						"fila vacía se lee como un comentario borrado, y el suelo a una "+
						"fila es lo que además hace que el reparto con presupuesto justo "+
						"sea una identidad",
						c, lineas, ancho)
				}
				if len(got) > lineas && lineas >= 1 {
					t.Errorf("comentario %+q con tope de %d filas devolvió %d",
						c.Body, lineas, len(got))
				}
			}
		}
	}
}

func primeraLineaCon(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
