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

// TestSplitProjectSeparaDuePartsYNadaMas: "owner/repo" son dos partes, y un solo
// segmento NO es un repo.
//
// Es pura, así que se llama. Y tiene un borde que importa: un proyecto de UN solo
// segmento no es un repositorio de GitHub, y devolverlo como "repo sin dueño"
// convertiría una referencia inválida en una petición a una URL que no existe.
//
// El prefijo y el sufijo `/` se quitan antes de mirar, porque un remoto puede venir
// como "/owner/repo/" y eso SÍ es un repo válido. Por eso el recorte va antes de la
// comparación y no después: al revés, "/owner/repo" se contaría como un segmento solo
// y se rechazaría.
func TestSplitProjectSeparaDuePartsYNadaMas(t *testing.T) {
	casos := []struct {
		proyecto    string
		wantOwner   string
		wantName    string
		descripcion string
	}{
		{"owner/repo", "owner", "repo", "el caso normal"},
		{"acme/widget", "acme", "widget", "con Organization"},
		// Los bordes de las barras: se recortan, y un repo entre barras es válido.
		{"/owner/repo", "owner", "repo", "barra inicial"},
		{"owner/repo/", "owner", "repo", "barra final"},
		{"/owner/repo/", "owner", "repo", "barras a los dos lados"},
		// Un segmento solo NO es un repo: no hay dueño.
		{"repo", "", "repo", "un segmento"},
		{"", "", "", "vacío"},
		{"/", "", "", "solo barras"},
		{"///", "", "", "solo barras, varias"},
		// Con más de una barra, todo lo que va después de la primera es el nombre.
		// En GitHub no hay subgroups, así que "a/b/c" no es un repo real, pero partir
		// por la PRIMERA barra es lo que evita inventarse un dueño.
		{"a/b/c", "a", "b/c", "tres segmentos"},
	}

	for _, c := range casos {
		owner, name := splitProject(c.proyecto)
		if owner != c.wantOwner || name != c.wantName {
			t.Errorf("%s: splitProject(%q) = %q, %q; quiero %q, %q",
				c.descripcion, c.proyecto, owner, name, c.wantOwner, c.wantName)
		}
		// Y el nombre nunca sale con barras, porque con barras no es un nombre de
		// repo: es un camino, y las APIs de GitHub lo Rechazan.
		if strings.Contains(name, "/") && c.descripcion != "tres segmentos" {
			t.Errorf("%s: el nombre %q sale con barras", c.descripcion, name)
		}
	}

	// Y la regla que de verdad importa, dicha como regla: hay dueño y hay nombre, o
	// no hay repo. UnItemState con cualquiera de los dos vacíos no sale a la red.
	for _, proyecto := range []string{"", "repo", "/", "///"} {
		owner, name := splitProject(proyecto)
		if owner == "" || name == "" {
			continue // es lo esperado: no es un repo
		}
		t.Errorf("splitProject(%q) dio dueño %q y nombre %q, y un segmento solo no es un repo",
			proyecto, owner, name)
	}
}

// TestUnaReferenciaInvalidaNoSaleALaRed: sin dueño o sin nombre no hay PR que
// preguntar, y se dice antes de tocar la red.
//
// El aviso es de tipo "notfound", no de red: no es que falte el PR, es que no hay a
// cuál preguntar. Y lo que se afirma es el "no sale", con el registro de args: una
// llamada de más por cada referencia inválida se paga del límite de peticiones del
// usuario y no compra nada.
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

	// Y con dueño y nombre sí sale. El guard no se come la llamada buena.
	_, _ = a.ItemState(context.Background(), model.RepoRef{Project: "acme/widget"}, 1)
	raw, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal("con una referencia válida no salió a la red: el guard se tragó la llamada buena")
	}
	if !strings.Contains(string(raw), "acme") {
		t.Errorf("la llamada no lleva el proyecto: %s", raw)
	}
}

// TestFailureMsgPrefiereElMotivoDelServidor: cuando el cuerpo trae un motivo, ese es
// el mensaje; si no, el del error.
//
// Es pura, y la razón de existir está en la diferencia entre los dos:
//
//   - "Reference 'x' does not exist" dice qué hacer. Un 404 de GitHub con ese cuerpo
//     es un error de graphQL con la respuesta en stdout.
//   - "gh api graphql ... (exit 1)" no dice nada. Es el argv y el código de salida.
//
// Y el caso intermedio es el que importa: un cuerpo que NO trae motivo no puede
// pisar un mensaje que sí dice algo. Un cuerpo vacío, o ilegible, o un JSON sin el
// campo, tienen que dejar pasar el error.
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

	// Y la propiedad que de verdad importa: el mensaje NUNCA queda vacío. Un fallo sin
	// texto se pinta como un fallo sin explicación, que es peor que no pintarlo.
	for _, body := range []string{"", "  ", "nada", "{}", `{"message":""}`, `{"message":null}`} {
		if strings.TrimSpace(failureMsg(body, err)) == "" {
			t.Errorf("con el cuerpo %q el mensaje quedó vacío", body)
		}
	}
	// Y con un error vacío tampoco: el mensaje del error, aunque sea su texto vacío,
	// es lo único que hay. Por eso el aserto de arriba usa TrimSpace sobre el
	// resultado de un error REAL.
}

// TestElHostPorDefectoEsGithubYElRestoNo: sin host se usa github.com, y con host se
// respeta el que se pidió.
//
// Es el mismo patrón que el binario: lo que no se dice, se deduce; lo que se dice, se
// respeta. Y el host importa más que el binario, porque va dentro de cada petición:
// un host equivocado no da un error de "no encuentro el repo", da un PR del repo que
// se llame igual en otro sitio.
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
		// Y el host llega a los ítems, que es donde importa. OJO: no llega a los
		// args de `gh api`, porque `gh` toma el host de su propia configuración, y
		// ponerlo en el argv a mano sería mentir sobre quién decide.
		items := []model.Item{{Number: 1}}
		a.stamp(items, forge.Query{Section: model.SectionReview})
		if items[0].Host != c.quiere {
			t.Errorf("con host %q el item quedó con host %q", c.quiere, items[0].Host)
		}
		if items[0].Ref.Host != c.quiere {
			t.Errorf("con host %q la referencia del item quedó con host %q", c.quiere, items[0].Ref.Host)
		}
	}

	// Y el binario vacío es "gh", que es el que está en el PATH. Tampoco se mira en
	// el Adapter, que no lo guarda: se mira en la petición, que es donde se ve qué
	// binario se acabó usando.
	otro := t.TempDir()
	registro := filepath.Join(otro, "args.log")
	scriptFalso := writeScript(t, otro, "falso", "#!/bin/sh\necho \"$0\" >> \""+registro+"\"\necho '{}'\n")
	a := New("h.example", scriptFalso)
	_, _ = a.ItemState(context.Background(), model.RepoRef{Project: "acme/widget"}, 1)
	if raw, _ := os.ReadFile(registro); !strings.Contains(string(raw), scriptFalso) {
		t.Errorf("la petición no salió por el binario dado: %s", raw)
	}
}

// TestUnRunnerQueFallaDaUnAvisoYNoUnPanic: la ruta de error de una consulta.
//
// No es un test interesante por sí mismo, pero es la condición de existencia de la
// de abajo: si esto no da un aviso, la de abajo no tiene contra qué comparar.
func TestUnRunnerQueFallaDaUnAvisoYNoUnPanic(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "gh", "#!/bin/sh\necho '{\"message\":\"boom\"}'\nexit 1\n")
	a := New("h.example", script)

	_, warns := a.ItemState(context.Background(), model.RepoRef{Project: "acme/widget"}, 1)
	if len(warns) == 0 {
		t.Fatal("un runner que falla no dio ningún aviso")
	}
	// Y el aviso nombra lo que pasó, no solo un código de salida.
	if strings.TrimSpace(warns[0].Msg) == "" {
		t.Errorf("el aviso quedó vacío: %+v", warns[0])
	}
	if warns[0].Forge != ForgeName {
		t.Errorf("el aviso no dice de qué forge es: %q", warns[0].Forge)
	}
}

// TestStampPoneElReviewKindSoloEnReview: el tipo de review es de la sección de
// review.
//
// La misma regla que en el resto de adaptadores, y con el mismo motivo: la lista de
// asignados y la de menciones se pintan con los mismos ítems, así que un ReviewKind
// fuera de la sección de review se acaba leyendo en una lista donde no significa nada.
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
		// Y la sección se estampa SIEMPRE, que es el otro campo del stamp: sin ella
		// el ítem no sabe a qué lista pertenece.
		if items[0].Section != seccion {
			t.Errorf("sección %v: quedó estampada como %v", seccion, items[0].Section)
		}
	}
}
