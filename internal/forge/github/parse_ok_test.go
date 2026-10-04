package github

import (
	"context"
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

// The INVERTED condition makes this work.
func TestUnaRespuestaQueSeParseaDaElItemYNoUnAviso(t *testing.T) {
	dir := t.TempDir()

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

	// The other half: a response that does NOT parse MUST give a warning. Without it the assertion above
	//passes with the condition reversed.
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

	// An undeserialisable response has to give a PARSE warning, the only path that reaches it.
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
