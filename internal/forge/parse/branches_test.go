package parse

import (
	"strings"
	"testing"
)

// TestParseGLBranchesLeeCadaPagina: con `--paginate` la respuesta son varias
// páginas y una línea es una rama. Un decodificador que leyera un único valor JSON
// se comería la primera página y tiraría el resto, que es justo la parte del
// repositorio que no cabe en una pantalla.
func TestParseGLBranchesLeeCadaPagina(t *testing.T) {
	ndjson := `{"name":"main","commit":{"id":"abc"}}
{"name":"release/2.0"}
{"name":"fix/hunk-pane-argv","default":true}`

	names, err := ParseGLBranches(ndjson)
	if err != nil {
		t.Fatalf("ParseGLBranches = %v, want sin error", err)
	}
	if got := strings.Join(names, ","); got != "main,release/2.0,fix/hunk-pane-argv" {
		t.Errorf("ParseGLBranches = %q, want las tres ramas en orden", got)
	}
}

// TestParseGLBranchesSaltaLaLineaRara: una línea que no se entiende no tira el
// listado entero. Perder todas las ramas por una línea rara es peor que devolver
// las que sí se pudieron leer, porque el buscador se abre vacío y parece que el
// repositorio no tiene ramas.
func TestParseGLBranchesSaltaLaLineaRara(t *testing.T) {
	ndjson := "basura\n{\"name\":\"main\"}\n"

	names, err := ParseGLBranches(ndjson)
	if err != nil {
		t.Fatalf("ParseGLBranches = %v, want que la línea rara no tumbe la lectura", err)
	}
	if got := strings.Join(names, ","); got != "main" {
		t.Errorf("ParseGLBranches = %q, want solo la rama legible", got)
	}
}

// TestParseGLBranchesAvisaSiNoLeyoNada: una respuesta que no se entendió no es un
// repositorio sin ramas, y confundirlas deja el buscador vacío sin decir por qué.
func TestParseGLBranchesAvisaSiNoLeyoNada(t *testing.T) {
	for _, in := range []string{"basura\n", "[]\n", "{\"commit\":{}}\n"} {
		names, err := ParseGLBranches(in)
		if err == nil {
			t.Errorf("ParseGLBranches(%q) = %v, want error", in, names)
		}
	}
}

// TestParseGLBranchesArrastraLaCausa: si el listado no se pudo leer, el aviso
// tiene que decir por qué. Un error genérico deja al usuario mirando un buscador
// vacío sin saber si fue la red, la auth o una respuesta con otra forma.
func TestParseGLBranchesArrastraLaCausa(t *testing.T) {
	for _, in := range []string{"basura\n", "[]\nnot-json\n"} {
		names, err := ParseGLBranches(in)
		if err == nil {
			t.Fatalf("ParseGLBranches(%q) = %v, want error", in, names)
		}
		if !strings.HasPrefix(err.Error(), "could not read the branch list: ") {
			t.Errorf("ParseGLBranches(%q) = %q, want el motivo con la causa del unmarshal", in, err)
		}
	}
}

// TestParseGLBranchesSinCausaNoLaInventa: líneas legibles pero sin nombre no son
// un error de sintaxis, así que el aviso no debe colgarse una causa inventada.
func TestParseGLBranchesSinCausaNoLaInventa(t *testing.T) {
	names, err := ParseGLBranches(`{"commit":{"id":"abc"}}`)
	if err == nil {
		t.Fatalf("ParseGLBranches = %v, want error", names)
	}
	if got := err.Error(); got != "could not read the branch list" {
		t.Errorf("error = %q, want el aviso sin causa", got)
	}
}

// TestParseGLBranchesAceptaVacioDeVerdad: una respuesta vacía de verdad es un
// repositorio sin ramas, y eso no es un error: quien llama lo distingue porque lo
// dice con una lista vacía y sin warning.
func TestParseGLBranchesAceptaVacioDeVerdad(t *testing.T) {
	names, err := ParseGLBranches("")
	if err != nil || len(names) != 0 {
		t.Errorf("ParseGLBranches(\"\") = %v, %v; want lista vacía sin error", names, err)
	}
}

// TestParseGHBranchesParteLineas: el jq de gh deja un nombre por línea, y una ref
// no puede traer saltos de línea porque git lo prohíbe, así que una línea es una
// rama y no hay forma de que dos se fundan.
func TestParseGHBranchesParteLineas(t *testing.T) {
	names := ParseGHBranches("main\nrelease/2.0\n\n  main  \n")
	if got := strings.Join(names, "|"); got != "main|release/2.0|main" {
		t.Errorf("ParseGHBranches = %q, want los tres nombres con los huecos fuera", got)
	}
}
