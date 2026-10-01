package tui

import (
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

// TestRefLeafCortaPorElUltimoSeparador: la hoja de una ruta de proyecto es lo que va
// DESPUÉS de la última barra, y el número del ítem va detrás.
//
// La función es pura y se llama directamente. Lo que se afirma es qué es "la hoja" en
// las cuatro formas que puede tener un proyecto, y el orden de las barras es lo que
// decide:
//
//   - "grupo/proyecto" -> "proyecto". Se corta por la ÚLTIMA barra, no por la
//     primera: en un forge donde el grupo tiene subgrupos, "a/b/c" es un proyecto
//     llamado "c" y cortar por la primera daría "b", que no existe.
//   - "proyecto" (sin barras) -> "proyecto". Entero, porque no hay nada que cortar.
//   - "" (sin referencia) -> "". Un forge puede devolver la referencia sin parsear, y
//     aun así la celda tiene que seguir identificando el ítem, que es el número.
//
// Y el caso del BORDE, que es el que no se probaba: un proyecto que EMPIEZA por barra.
// "/proyecto" tiene una barra en la posición 0, y la hoja sigue siendo "proyecto". Con
// un "> 0" en vez de un ">= 0" la barra de la posición 0 se trataría como si no
// hubiera separador y la celda quedaría "/proyecto#7", que se sale de la columna.
//
// No es un input que produzca el producto: ParseRemoteURL quita la barra inicial del
// path. Pero la función tiene su propio contrato y ese contrato tiene un borde, y un
// borde que no se afirma es un borde que el próximo que lea el código decide por su
// cuenta.
func TestRefLeafCortaPorElUltimoSeparador(t *testing.T) {
	casos := []struct {
		proyecto string
		numero   int
		want     string
	}{
		{"acme/widget", 7, "widget#7"},
		{"grupo/sub/proyecto", 12, "proyecto#12"},
		{"a/b/c/d", 1, "d#1"},
		// Sin barras: entero, con el número detrás.
		{"widget", 7, "widget#7"},
		// Sin referencia: solo el número, que es lo único que sigue identificando.
		{"", 7, "#7"},
		// El borde: una barra en la posición 0. La hoja de "/proyecto" es
		// "proyecto", y la barra de la posición 0 es un separador como cualquier
		// otro.
		{"/proyecto", 7, "proyecto#7"},
		{"/a/b", 3, "b#3"},
		// Solo una barra: hoja vacía, y el número es lo que queda.
		{"/", 7, "#7"},
		{"a/", 7, "#7"},
		// Números raros, porque van detrás y no se deben cortar.
		{"acme/widget", 0, "widget#0"},
		{"acme/widget", -1, "widget#-1"},
		{"acme/widget", 12345, "widget#12345"},
	}

	for _, c := range casos {
		got := refLeaf(model.Item{Number: c.numero, Ref: model.RepoRef{Project: c.proyecto}})
		if got != c.want {
			t.Errorf("refLeaf(%q, %d) = %q, want %q", c.proyecto, c.numero, got, c.want)
		}
	}

	// Y la hoja NUNCA sale con una barra: la hoja de una ruta no es una ruta. Si
	// saliera, la celda mostraría una parte del camino, que es exactamente lo que
	// esta función existe para no mostrar.
	for _, proyecto := range []string{"a", "a/b", "a/b/c", "/a", "/a/b", "/"} {
		hoja := refLeaf(model.Item{Number: 1, Ref: model.RepoRef{Project: proyecto}})
		quedan := strings.TrimSuffix(hoja, "#1")
		if strings.Contains(quedan, "/") {
			t.Errorf("refLeaf(%q) dio %q: la hoja de una ruta no lleva barras", proyecto, hoja)
		}
	}

	// Y el número siempre está, porque sin él la celda no identifica el ítem y dos
	// hojas iguales de items distintos serían la misma fila.
	for _, proyecto := range []string{"", "a", "a/b", "/a"} {
		hoja := refLeaf(model.Item{Number: 42, Ref: model.RepoRef{Project: proyecto}})
		if !strings.HasSuffix(hoja, "#42") {
			t.Errorf("refLeaf(%q) dio %q, want el número detrás: sin él la celda no identifica el ítem",
				proyecto, hoja)
		}
	}
}

// TestSectionPrefixConProyectosDeProfundidadDistinta: el prefijo tiene que ser un
// directorio estricto para TODOS los ítems de la sección, y el mínimo se lleva.
//
// El que marca el suelo es un proyecto SIN barras: su profundidad es 0. Con un "<= "
// en la comparación del mínimo, un proyecto de profundidad 0 reventaría el mínimo y lo
// subiría al de la siguiente iteración, así que la celda de un repo sin grupo se
// quedaría sin prefijo y las secciones de distintos repos se leerían igual.
//
// El mínimo se lleva con "<" y no con "<=": cuando las profundidades son iguales, la
// asignación es una identidad. Por eso un "<=" ahí no se puede matar, y por eso no
// hay que intentar.
func TestSectionPrefixConProyectosDeProfundidadDistinta(t *testing.T) {
	casos := []struct {
		nombre    string
		proyectos []string
		want      string
	}{
		// Todos con la misma profundidad: el prefijo es todo menos la hoja.
		{"misma profundidad", []string{"acme/a", "acme/b"}, "acme"},
		// Con una barra de diferencia: la común, y solo como directorio estricto.
		{"profundidad distinta", []string{"a/b/c", "a/b/d"}, "a/b"},
		// Uno SIN barras: el mínimo es 0, y no hay prefijo que compartir.
		{"uno sin barras", []string{"widget", "acme/a"}, ""},
		{"uno sin barras primero", []string{"widget", "acme/a"}, ""},
		// Uno con 0 y otro con 0: tampoco.
		{"los dos sin barras", []string{"a", "b"}, ""},
		// Tres profundidades: el mínimo manda.
		{"tres profundidades", []string{"x/y/z", "x/y", "x/y"}, "x"},
		// Y el prefijo NO lleva barra final: es un prefijo, no un camino, y quien lo
		// junta con la hoja pone el separador. Acortarlo un segmento de más, o
		// alargar uno, cambia qué parte del proyecto se repite en la cabecera.
		{"el prefijo no lleva barra final", []string{"a/b/c", "a/b/d"}, "a/b"},
		// Y con un solo segmento común no hay nada que repetir.
		{"un solo segmento comun", []string{"a/b", "a/c"}, "a"},
		// Y sin nada común, prefijo vacío: la celda lleva la referencia entera.
		{"nada comun", []string{"a/b", "c/d"}, ""},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			items := make([]model.Item, len(c.proyectos))
			for i, p := range c.proyectos {
				items[i] = model.Item{Number: i + 1, Ref: model.RepoRef{Project: p}}
			}
			if got := sectionPrefix(items); got != c.want {
				t.Errorf("sectionPrefix(%v) = %q, want %q", c.proyectos, got, c.want)
			}
		})
	}

	// Y con un solo item no hay prefijo: no hay con quién compartirlo, y poner el
	// nombre entero del repo como prefija duplicaría lo que ya dice la hoja.
	if got := sectionPrefix([]model.Item{{Ref: model.RepoRef{Project: "acme/widget"}}}); got != "" {
		t.Errorf("sectionPrefix con un item dio %q, want vacío", got)
	}
}
