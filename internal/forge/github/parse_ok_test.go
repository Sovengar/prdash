package github

import (
	"context"
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

// TestUnaRespuestaQueSeParseaDaElItemYNoUnAviso: cuando el parseo va bien, el item sale
// y no hay aviso.
//
// Y la condición invertida hace lo que peor se puede hacer con una comprobación de error:
// devuelve un aviso cuando NO hay error. O sea que no rompe el camino malo, rompe el
// BUENO, y de una forma silenciosa: el PR sale vacío con un aviso de parseo, y el usuario ve
// "no encontrado" cuando lo que pasó es que se leyó bien.
//
// Y el caso que lo distingue es el que devuelve un GraphQL con un PR dentro. La respuesta
// más corta que parsea es un objeto con la forma que espera el parser, y sin ella el aviso
// es legítimo —eso también se afirma—.
func TestUnaRespuestaQueSeParseaDaElItemYNoUnAviso(t *testing.T) {
	dir := t.TempDir()

	// Un PR dentro de la respuesta de búsqueda. La forma es la que
	// `ParseGHGraphQLSearch` entiende; con un PR dentro, `items` sale no vacío y el item
	// existe.
	conPR := `{"data":{"search":{"issueCount":1,"nodes":[{"__typename":"PullRequest","number":7,"title":"uno","state":"OPEN","updatedAt":"2026-03-17T10:00:00Z","url":"https://github.com/acme/widget/pull/7","author":{"login":"alice"},"headRefName":"feat/x","baseRefName":"main","mergeable":"MERGEABLE"}]}}}`
	script := writeScript(t, dir, "gh", "#!/bin/sh\necho '"+conPR+"'\n")
	a := New("github.com", script)

	it, warns := a.ItemState(context.Background(),
		model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget",
			Owner: "acme", Name: "widget"}, 7)

	if len(warns) != 0 {
		t.Errorf("una respuesta que se parsea dio %d avisos: %+v. Con la condición "+
			"invertida el camino bueno devuelve un aviso de parseo, y el PR sale vacío "+
			"sin que se note por qué", len(warns), warns)
	}
	if it.Number != 7 {
		t.Errorf("el item salió con número %d, want 7: la respuesta trae el PR 7", it.Number)
	}
	if it.Title != "uno" {
		t.Errorf("el item salió con título %q, want %q", it.Title, "uno")
	}

	// Y el otro lado: una respuesta que NO se parsea SÍ tiene que dar aviso. Sin esta
	// mitad, el assert de arriba pasa igual con la condición al revés, porque quitar el
	// aviso del camino bueno y ponerlo en el malo son el mismo cambio visto desde un
	// lado.
	vacio := `{"data":{"search":{"issueCount":0,"nodes":[]}}}`
	scriptVacio := writeScript(t, dir, "gh", "#!/bin/sh\necho '"+vacio+"'\n")
	aVacio := New("github.com", scriptVacio)
	_, warns = aVacio.ItemState(context.Background(),
		model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget",
			Owner: "acme", Name: "widget"}, 7)
	if len(warns) == 0 {
		t.Error("una respuesta sin PRs no dio ningún aviso, y debería decir que no se " +
			"ha encontrado ese número")
	}
	if warns[0].Kind != "notfound" && warns[0].Kind != "parse" {
		t.Errorf("el aviso salió de tipo %q, y aquí lo esperable es notfound o parse", warns[0].Kind)
	}

	// Y lo que SÍ se ve, que es lo que faltaba: una respuesta que no se puede deserializar
	// tiene que dar un aviso de PARSEO, y es el único camino que lo produce.
	//
	// La primera versión de este test solo afirmaba la respuesta buena y la lista vacía, y
	// con eso el mutante de quitar el aviso entero pasaba: ninguna de las dos respuestas
	// llegan a la rama de parseo, la buena porque parsea y la vacía porque devuelve cero
	// ítems. Lo que hace falta es un JSON que no se pueda leer, y eso solo se ve con
	// basura en la salida del binario.
	basura := writeScript(t, dir, "gh", "#!/bin/sh\necho 'esto no es json'\n")
	aBasura := New("github.com", basura)
	it, warns = aBasura.ItemState(context.Background(),
		model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget",
			Owner: "acme", Name: "widget"}, 7)
	if len(warns) == 0 {
		t.Fatal("una respuesta que no es JSON no dio ningún aviso: el aviso de parseo " +
			"es lo único que dice que la API cambió de formato")
	}
	if warns[0].Kind != "parse" {
		t.Errorf("una respuesta ilegible dio un aviso de tipo %q, want parse", warns[0].Kind)
	}
	if !strings.Contains(strings.ToLower(warns[0].Msg), "json") {
		t.Errorf("el aviso de parseo dice %q y no menciona el JSON: el mensaje es lo que "+
			"le dice al usuario si mirar su red o la API", warns[0].Msg)
	}
	if it.Number != 0 {
		t.Errorf("una respuesta ilegible devolvió el ítem %d: sin parsear no hay ítem", it.Number)
	}
}

// TestElHostPorDefTampocoSePisa: el suelo del host es el mismo que en el resto, y un
// host propio se respeta. Es la misma regla del adapter inerte, escrita donde se usa.
func TestElHostPorDefTampocoSePisa(t *testing.T) {
	for _, c := range []struct{ entra, want string }{
		{"", "github.com"},
		{"ghe.ejemplo.com", "ghe.ejemplo.com"},
	} {
		if got := New(c.entra, "").Host(); got != c.want {
			t.Errorf("New(%q).Host() = %q, want %q", c.entra, got, c.want)
		}
	}
}
