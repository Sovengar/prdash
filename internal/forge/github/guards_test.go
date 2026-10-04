package github

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
)

// A single segment is not a repo.
func TestSplitProjectSeparaDuePartsYNadaMas(t *testing.T) {
	casos := []struct {
		proyecto    string
		wantOwner   string
		wantName    string
		descripcion string
	}{
		{"owner/repo", "owner", "repo", "el caso normal"},
		{"acme/widget", "acme", "widget", "con Organization"},
		{"/owner/repo", "owner", "repo", "barra inicial"},
		{"owner/repo/", "owner", "repo", "barra final"},
		{"/owner/repo/", "owner", "repo", "barras a los dos lados"},
		{"repo", "", "repo", "un segmento"},
		{"", "", "", "vacío"},
		{"/", "", "", "solo barras"},
		{"///", "", "", "solo barras, varias"},
		{"a/b/c", "a", "b/c", "tres segmentos"},
	}

	for _, c := range casos {
		owner, name := splitProject(c.proyecto)
		if owner != c.wantOwner || name != c.wantName {
			t.Errorf("%s: splitProject(%q) = %q, %q; quiero %q, %q",
				c.descripcion, c.proyecto, owner, name, c.wantOwner, c.wantName)
		}
		if strings.Contains(name, "/") && c.descripcion != "tres segmentos" {
			t.Errorf("%s: el nombre %q sale con barras", c.descripcion, name)
		}
	}

	// The rule that matters, stated as a rule: owner and name, or it is not a repo.
	for _, proyecto := range []string{"", "repo", "/", "///"} {
		owner, name := splitProject(proyecto)
		if owner == "" || name == "" {
			continue // es lo esperado: no es un repo
		}
		t.Errorf("splitProject(%q) dio dueño %q y nombre %q, y un segmento solo no es un repo",
			proyecto, owner, name)
	}
}

// The warning is of type "notfound", not "network": nothing was queried.
func TestUnaReferenciaInvalidaNoSaleALaRed(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args.log")
	script := writeScript(t, dir, "gh", "#!/bin/sh\necho \"$@\" >> \""+argsFile+"\"\necho '{}'\n")
	a := New("h.example", script)

	for _, proyecto := range []string{"", "repo", "/", "///"} {
		_, warns := a.ItemState(context.Background(), model.RepoRef{Project: proyecto}, 1)
		if len(warns) == 0 {
			t.Errorf("proyecto %q: no salió ningún aviso, debería decir que no hay a qué preguntar", proyecto)
			continue
		}
		if warns[0].Kind != "notfound" {
			t.Errorf("proyecto %q: el aviso es de tipo %q, want notfound", proyecto, warns[0].Kind)
		}
	}
	if _, err := os.Stat(argsFile); err == nil {
		raw, _ := os.ReadFile(argsFile)
		t.Errorf("una referencia inválida salió a la red:\n%s", raw)
	}

	_, _ = a.ItemState(context.Background(), model.RepoRef{Project: "acme/widget"}, 1)
	raw, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal("con una referencia válida no salió a la red: el guard se tragó la llamada buena")
	}
	if !strings.Contains(string(raw), "acme") {
		t.Errorf("la llamada no lleva el proyecto: %s", raw)
	}
}

func TestFailureMsgPrefiereElMotivoDelServidor(t *testing.T) {
	err := errors.New("gh api graphql -f query=... (exit 1)")

	casos := []struct {
		nombre string
		body   string
		want   string
	}{
		{"con mensaje", `{"message":"Reference 'x' does not exist"}`, "Reference 'x' does not exist"},
		{"con errors", `{"errors":[{"message":"Bad credentials"}]}`, "Bad credentials"},
		{"cuerpo vacío", "", err.Error()},
		{"cuerpo en blanco", "   \n", err.Error()},
		{"no es json", "404 page not found", err.Error()},
		{"json sin mensaje", `{"documentation_url":"https://docs"}`, err.Error()},
		{"json vacío", `{}`, err.Error()},
		{"lista vacía", `[]`, err.Error()},
		{"mensaje vacío", `{"message":""}`, err.Error()},
	}

	for _, c := range casos {
		if got := failureMsg(c.body, err); got != c.want {
			t.Errorf("%s: failureMsg(%q) = %q, want %q", c.nombre, c.body, got, c.want)
		}
	}

	// What matters: the message is NEVER empty, because a failure with no text cannot be acted on.
	for _, body := range []string{"", "  ", "nada", "{}", `{"message":""}`, `{"message":null}`} {
		if strings.TrimSpace(failureMsg(body, err)) == "" {
			t.Errorf("con el cuerpo %q el mensaje quedó vacío", body)
		}
	}
	// And with an empty error either: the error's own message, empty text or not, is all there is.
}

// Same pattern as the binary: what is not set falls back.
func TestElHostPorDefectoEsGithubYElRestoNo(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args.log")
	script := writeScript(t, dir, "gh", "#!/bin/sh\necho \"$@\" >> \""+argsFile+"\"\necho '{}'\n")

	for _, c := range []struct{ dado, quiere string }{
		{"", "github.com"},
		{"github.com", "github.com"},
		{"h.example", "h.example"},
		{"gitlab.example.com", "gitlab.example.com"},
	} {
		a := New(c.dado, script)
		if a.host != c.quiere {
			t.Errorf("New(%q) dio host %q, want %q", c.dado, a.host, c.quiere)
		}
		// The host reaches the items, and NOT gh's argv: the binary is configured another way.
		items := []model.Item{{Number: 1}}
		a.stamp(items, forge.Query{Section: model.SectionReview})
		if items[0].Host != c.quiere {
			t.Errorf("con host %q el item quedó con host %q", c.quiere, items[0].Host)
		}
		if items[0].Ref.Host != c.quiere {
			t.Errorf("con host %q la referencia del item quedó con host %q", c.quiere, items[0].Ref.Host)
		}
	}

	otro := t.TempDir()
	registro := filepath.Join(otro, "args.log")
	scriptFalso := writeScript(t, otro, "falso", "#!/bin/sh\necho \"$0\" >> \""+registro+"\"\necho '{}'\n")
	a := New("h.example", scriptFalso)
	_, _ = a.ItemState(context.Background(), model.RepoRef{Project: "acme/widget"}, 1)
	if raw, _ := os.ReadFile(registro); !strings.Contains(string(raw), scriptFalso) {
		t.Errorf("la petición no salió por el binario dado: %s", raw)
	}
}

// Not an interesting test on its own, but it is the existence condition of the rest.
func TestUnRunnerQueFallaDaUnAvisoYNoUnPanic(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "gh", "#!/bin/sh\necho '{\"message\":\"boom\"}'\nexit 1\n")
	a := New("h.example", script)

	_, warns := a.ItemState(context.Background(), model.RepoRef{Project: "acme/widget"}, 1)
	if len(warns) == 0 {
		t.Fatal("un runner que falla no dio ningún aviso")
	}
	if strings.TrimSpace(warns[0].Msg) == "" {
		t.Errorf("el aviso quedó vacío: %+v", warns[0])
	}
	if warns[0].Forge != ForgeName {
		t.Errorf("el aviso no dice de qué forge es: %q", warns[0].Forge)
	}
}

// The review kind belongs to the review section.
func TestStampPoneElReviewKindSoloEnReview(t *testing.T) {
	for _, seccion := range []model.Section{model.SectionReview, model.SectionAuthored, model.SectionMentions} {
		a := New("h.example", "gh")
		items := []model.Item{{Number: 1, Ref: model.RepoRef{Forge: ForgeName, Host: "h.example", Project: "acme/widget", Owner: "acme", Name: "widget"}}}
		a.stamp(items, forge.Query{Section: seccion, ReviewKind: model.ReviewRequested})

		tiene := items[0].ReviewKind != ""
		quiere := seccion == model.SectionReview
		if tiene != quiere {
			t.Errorf("sección %v: ReviewKind %q presente=%v, quiere %v",
				seccion, items[0].ReviewKind, tiene, quiere)
		}
		// The section is stamped ALWAYS, because without it the item does not know which column it
		// belongs to.
		if items[0].Section != seccion {
			t.Errorf("sección %v: quedó estampada como %v", seccion, items[0].Section)
		}
	}
}
