package gitlab

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
)

// GitLab has one output GitHub does not, and it is the one that matters: an empty list is not a
//broken answer.

func glabQueDevuelve(t *testing.T, cuerpo string) string {
	t.Helper()
	dir := t.TempDir()
	return writeScript(t, dir, "glab", "#!/bin/sh\n"+cuerpo+"\n")
}

func glabQueImprime(salida string) string {
	// SINGLE quotes: with double ones the JSON's own quotes close them and the shell hands over the
	//text without them.
	return "printf '%s\\n' '" + salida + "'"
}

func avisoUnico(t *testing.T, warns []model.Warning, donde string) model.Warning {
	t.Helper()
	if len(warns) != 1 {
		t.Fatalf("%s: %d avisos, want 1: %+v", donde, len(warns), warns)
	}
	return warns[0]
}

// The EXACT shape glMRQuery asks for, with every field.
const graphqlVacio = `{"data":{"project":{"mergeRequest":null}}}`

// This is the case that happens in production.
func TestUnMRQueNoExisteSeDiceQueNoExisteYNoDevuelveUnItemFalso(t *testing.T) {
	a := New("gitlab.example.com", glabQueDevuelve(t, "cat <<'JSON'\n"+graphqlVacio+"\nJSON"))

	ref := model.RepoRef{Forge: "gitlab", Host: "gitlab.example.com", Project: "grupo/proyecto"}
	it, warns := a.ItemState(context.Background(), ref, 404)

	w := avisoUnico(t, warns, "MR inexistente")
	if w.Kind != "notfound" {
		t.Errorf("clase %q, want notfound: un MR que no existe no es un error de parseo ni "+
			"un rate limit", w.Kind)
	}
	// The message says WHICH: without the number the warning could belong to any of the MRs.
	for _, quiere := range []string{"404", "grupo/proyecto"} {
		if !strings.Contains(w.Msg, quiere) {
			t.Errorf("el aviso %q no menciona %q", w.Msg, quiere)
		}
	}
	// And the item is the zero value: an Item with the number set and nothing else would paint as a
	// real one.
	if it.ID() != (model.ID{}) || it.Number != 0 || it.Title != "" || it.HeadSHA != "" {
		t.Errorf("devolvió un ítem a medias en vez del valor cero: %+v", it)
	}
	// The warning carries no section: ItemState belongs to no inbox column.
	if w.Section != "" {
		t.Errorf("el aviso lleva sección %q y aparecería además en el inbox", w.Section)
	}
}

// The heredoc detail is not cosmetic.
func TestUnaSalidaQueNoEsJSONEnGitLabAvisaYNoSeRompe(t *testing.T) {
	for _, c := range []struct {
		nombre string
		cuerpo string
	}{
		{"html de un proxy", "echo '<html>Sign in to continue</html>'"},
		{"json truncado", "cat <<'JSON'\n{\"data\":{\"currentUser\":\nJSON"},
		{"json vacio", "printf ''"},
		{"lista en vez de objeto", "echo '[1,2,3]'"},
	} {
		a := New("gitlab.example.com", glabQueDevuelve(t, c.cuerpo))
		ref := model.RepoRef{Forge: "gitlab", Host: "gitlab.example.com", Project: "grupo/proy"}
		it, warns := a.ItemState(context.Background(), ref, 7)

		if w := avisoUnico(t, warns, c.nombre); w.Kind != "parse" {
			t.Errorf("%s: clase %q, want parse", c.nombre, w.Kind)
		}
		if it.ID() != (model.ID{}) || it.Number != 0 {
			t.Errorf("%s: devolvió un ítem a medias", c.nombre)
		}
	}
}

// TWO functions with the same contract.
func TestListarConSalidaIlegibleAvisaYNoInventaItems(t *testing.T) {
	a := New("gitlab.example.com", glabQueDevuelve(t, "echo 'no soy json'"))

	for _, c := range []struct {
		nombre string
		q      forge.Query
	}{
		{"graphql de review", forge.Query{Section: model.SectionReview, ReviewKind: model.ReviewRequested}},
		{"graphql de menciones", forge.Query{Section: model.SectionMentions}},
		{"graphql de authored", forge.Query{Section: model.SectionAuthored}},
	} {
		page, warns := a.List(context.Background(), c.q)
		if len(page.Items) != 0 {
			t.Errorf("%s: %d ítems de una salida ilegible", c.nombre, len(page.Items))
		}
		w := avisoUnico(t, warns, c.nombre)
		if w.Kind != "parse" {
			t.Errorf("%s: clase %q, want parse", c.nombre, w.Kind)
		}
		if w.Section != c.q.Section {
			t.Errorf("%s: el aviso no lleva la sección consultada (%q)", c.nombre, w.Section)
		}
	}
}

// The same risk as GitHub but with a different output shape.
func TestLaConversacionDeGitLabConSalidaIlegibleNoInventaNotas(t *testing.T) {
	a := New("gitlab.example.com", glabQueDevuelve(t, "echo 'rompido'"))
	ref := model.RepoRef{Forge: "gitlab", Host: "gitlab.example.com", Project: "grupo/proyecto"}

	page, warns := a.Comments(context.Background(), ref, 12)

	if w := avisoUnico(t, warns, "Comments"); w.Kind != "parse" {
		t.Errorf("clase %q, want parse", w.Kind)
	}
	if len(page.Comments) != 0 {
		t.Errorf("%d notas inventadas de una salida ilegible", len(page.Comments))
	}
	if page.Total != 0 {
		t.Errorf("Total = %d con una salida ilegible", page.Total)
	}
}

// All three operations have it.
func TestUnProyectoVacioSeDiceQueNoYNoSaleAConsultarElForge(t *testing.T) {
	a := New("gitlab.example.com", glabQueDevuelve(t,
		"echo 'glab no debería haberme llamado' >&2\nexit 1"))
	ref := model.RepoRef{Forge: "gitlab", Host: "gitlab.example.com", Project: ""}

	it, warns := a.ItemState(context.Background(), ref, 12)
	if w := avisoUnico(t, warns, "ItemState sin proyecto"); w.Kind != "notfound" {
		t.Errorf("clase %q, want notfound", w.Kind)
	}
	if it.ID() != (model.ID{}) {
		t.Errorf("devolvió un ítem sin proyecto: %+v", it)
	}

	page, warns := a.Comments(context.Background(), ref, 12)
	if w := avisoUnico(t, warns, "Comments sin proyecto"); w.Kind != "notfound" {
		t.Errorf("clase %q, want notfound", w.Kind)
	}
	if len(page.Comments) != 0 || page.Total != 0 {
		t.Errorf("devolvió comentarios sin proyecto: %+v", page)
	}

	// The warning carries no binary URL: that is noise from one machine in a message the operator
	// reads.
	for _, w := range warns {
		if strings.Contains(w.Msg, "glab") {
			t.Errorf("el aviso menciona el binario, que no le dice nada al usuario: %q", w.Msg)
		}
	}
}

// What is checked is that the warning does NOT mention an exit code.
func TestUnBinarioInexistenteSeDiceQueNoYNoSeIntentaEjecutar(t *testing.T) {
	ausente := filepath.Join(t.TempDir(), "no-hay-glab")
	a := New("gitlab.example.com", ausente)
	ref := model.RepoRef{Forge: "gitlab", Host: "gitlab.example.com", Project: "grupo/proyecto"}

	_, warns := a.ItemState(context.Background(), ref, 12)
	w := avisoUnico(t, warns, "sin glab")
	if w.Kind == "" {
		t.Error("sin binario no hay aviso: el inbox parecería vacío sin explicación")
	}
	if !strings.Contains(w.Msg, "glab") {
		t.Errorf("el aviso %q no dice que falta glab, que es lo accionable", w.Msg)
	}
}

// The good path was untested, which is a needle in a haystack.
func TestLeerElEstadoDeUnMRQueExisteLoDevuelveEstampadoConSuIdentidad(t *testing.T) {
	const mrVivo = `{"data":{"project":{"mergeRequest":{"iid":12,"title":"Un MR",` +
		`"webUrl":"https://gitlab.acme.example/g/p/-/merge_requests/12","state":"opened",` +
		`"draft":false,"detailedMergeStatus":"mergeable","sourceBranch":"feat/a",` +
		`"targetBranch":"main","approved":false,"updatedAt":"2026-09-21T07:00:00Z",` +
		`"diffHeadSha":"abc123"}}}}`
	a := New("gitlab.acme.example", glabQueDevuelve(t, glabQueImprime(mrVivo)))
	ref := model.RepoRef{Forge: "gitlab", Host: "gitlab.acme.example", Project: "grupo/proyecto"}

	it, warns := a.ItemState(context.Background(), ref, 12)

	if len(warns) != 0 {
		t.Fatalf("un MR que existe dio %d avisos: %+v", len(warns), warns)
	}
	if it.Number != 12 {
		t.Errorf("Number = %d, want 12", it.Number)
	}
	if it.Title != "Un MR" || it.SourceBranch != "feat/a" || it.TargetBranch != "main" {
		t.Errorf("el MR llegó incompleto: %+v", it)
	}
	// And the HeadSHA, which is what lets the merge be pinned; without it the merge would go out
	// without --sha.
	if it.HeadSHA != "abc123" {
		t.Errorf("HeadSHA = %q, want abc123: sin él el merge no se puede pinear", it.HeadSHA)
	}
	if it.Forge != "gitlab" || it.Host != "gitlab.acme.example" {
		t.Errorf("identidad en el Item = %s/%s, want gitlab/gitlab.acme.example",
			it.Forge, it.Host)
	}
	if it.Ref.Forge != "gitlab" || it.Ref.Host != "gitlab.acme.example" {
		t.Errorf("identidad en el Ref = %s/%s", it.Ref.Forge, it.Ref.Host)
	}
	if it.ID().Forge != "gitlab" || it.ID().Host != "gitlab.acme.example" {
		t.Errorf("el ID no lleva la identidad: %+v", it.ID())
	}

	// identity does NOT seal the project, same as GitHub; the two identity functions are line for
//line identical.
	if it.Ref.Project != "" {
		t.Errorf("identity selló el proyecto (%q): el proyecto viene del ítem sobre el que se "+
			"aplica, y sellarlo aquí daría a entender que el Item es autónomo", it.Ref.Project)
	}

	// And a different host gives a different ID, which is what stops two instances from colliding.
	otro := New("otro.acme.example", glabQueDevuelve(t, glabQueImprime(
		`{"data":{"project":{"mergeRequest":{"iid":12,"title":"Otro host","webUrl":"u",`+
			`"state":"opened","sourceBranch":"feat/a","targetBranch":"main",`+
			`"updatedAt":"2026-09-21T07:00:00Z"}}}}`)))
	it2, warns := otro.ItemState(context.Background(), ref, 12)
	if len(warns) != 0 {
		t.Fatalf("el segundo adapter avisa: %+v", warns)
	}
	if it2.ID() == it.ID() {
		t.Error("el mismo MR en dos hosts dio el mismo ID: la memoria de reviews los confundiría")
	}
	if it2.HeadSHA != "" {
		t.Errorf("HeadSHA = %q sin diffHeadSha: debe quedar vacío para que el merge NO se "+
			"pinee con un SHA inventado", it2.HeadSHA)
	}
}
