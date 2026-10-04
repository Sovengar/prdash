package parse

import (
	"strings"
	"testing"
)

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

// A line that cannot be understood does not throw away the whole listing.
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

func TestParseGLBranchesAvisaSiNoLeyoNada(t *testing.T) {
	for _, in := range []string{"basura\n", "[]\n", "{\"commit\":{}}\n"} {
		names, err := ParseGLBranches(in)
		if err == nil {
			t.Errorf("ParseGLBranches(%q) = %v, want error", in, names)
		}
	}
}

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

func TestParseGLBranchesSinCausaNoLaInventa(t *testing.T) {
	names, err := ParseGLBranches(`{"commit":{"id":"abc"}}`)
	if err == nil {
		t.Fatalf("ParseGLBranches = %v, want error", names)
	}
	if got := err.Error(); got != "could not read the branch list" {
		t.Errorf("error = %q, want el aviso sin causa", got)
	}
}

func TestParseGLBranchesAceptaVacioDeVerdad(t *testing.T) {
	names, err := ParseGLBranches("")
	if err != nil || len(names) != 0 {
		t.Errorf("ParseGLBranches(\"\") = %v, %v; want lista vacía sin error", names, err)
	}
}

func TestParseGHBranchesParteLineas(t *testing.T) {
	names := ParseGHBranches("main\nrelease/2.0\n\n  main  \n")
	if got := strings.Join(names, "|"); got != "main|release/2.0|main" {
		t.Errorf("ParseGHBranches = %q, want los tres nombres con los huecos fuera", got)
	}
}
