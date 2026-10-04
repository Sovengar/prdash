package main

import (
	"io"
	"path/filepath"
	"strings"
	"testing"

	"bytes"
	"prdash/internal/config"
	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/testutil"
	"prdash/internal/worktree"
)

// TestBuildAdaptersWiring comprueba que el wiring de adapters respeta la config.
func TestBuildAdaptersWiring(t *testing.T) {
	cfg := config.Defaults() // github + gitlab habilitados, bitbucket no
	got := names(buildAdapters(cfg))
	if strings.Join(got, ",") != "github,gitlab" {
		t.Fatalf("adapters = %v", got)
	}

	cfg.Forges.Bitbucket.Enabled = true
	got = names(buildAdapters(cfg))
	if strings.Join(got, ",") != "github,gitlab,bitbucket" {
		t.Fatalf("adapters con bitbucket = %v", got)
	}

	cfg = config.Defaults()
	cfg.Forges.GitHub.Enabled = false
	cfg.Forges.GitLab.Enabled = false
	if got := buildAdapters(cfg); len(got) != 0 {
		t.Fatalf("sin forges habilitados no debería haber adapters: %v", names(got))
	}
}

// TestRunPrint cubre el modo --print con un adapter falso (sin red).
func TestRunPrint(t *testing.T) {
	item := model.NewItem(model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget"}, 7)
	item.Title = "Add widget"
	fake := &testutil.FakeAdapter{
		ForgeName: "github",
		HostName:  "github.com",
		Pages: map[testutil.FakeKey][]forge.Page{
			{Section: model.SectionAuthored}: {{Items: []model.Item{item}}},
		},
	}

	out := imprimeABuffer(t, func(w io.Writer) { runPrintTo(w, w, []forge.Adapter{fake}, nil) })

	for _, want := range []string{"Created by me", "github@github.com", "acme/widget#7", "Add widget"} {
		if !strings.Contains(out, want) {
			t.Errorf("la salida no contiene %q:\n%s", want, out)
		}
	}
}

// TestRunPrintKeepsOrder comprueba que el orden de las secciones es determinista
// aunque los forges se consulten en paralelo.
func TestRunPrintKeepsOrder(t *testing.T) {
	mk := func(forgeName string) *testutil.FakeAdapter {
		it := model.NewItem(model.RepoRef{Forge: forgeName, Host: forgeName + ".com", Project: "o/r"}, 1)
		it.Title = "T-" + forgeName
		return &testutil.FakeAdapter{
			ForgeName: forgeName,
			HostName:  forgeName + ".com",
			Pages: map[testutil.FakeKey][]forge.Page{
				{Section: model.SectionAuthored}: {{Items: []model.Item{it}}},
			},
		}
	}
	out := imprimeABuffer(t, func(w io.Writer) {
		runPrintTo(w, w, []forge.Adapter{mk("github"), mk("gitlab")}, nil)
	})
	if strings.Index(out, "T-github") > strings.Index(out, "T-gitlab") {
		t.Errorf("el orden de impresión debe seguir el de los adapters:\n%s", out)
	}
}

// TestRunPrintShowsActiveReview comprueba que --print añade de forma
// determinista la ruta del worktree del review activo de cada ítem.
func TestRunPrintShowsActiveReview(t *testing.T) {
	it := model.NewItem(model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget"}, 7)
	it.Title = "Add widget"
	fake := &testutil.FakeAdapter{
		ForgeName: "github",
		HostName:  "github.com",
		Pages: map[testutil.FakeKey][]forge.Page{
			{Section: model.SectionAuthored}: {{Items: []model.Item{it}}},
		},
	}
	wtPath := filepath.Join(t.TempDir(), "worktrees", "prdash-pr-7")
	lookup := func(i model.Item) (worktree.Worktree, bool) {
		if i.ID() == it.ID() {
			return worktree.Worktree{Path: wtPath, Branch: "prdash/pr-7", Label: "prdash-pr-7"}, true
		}
		return worktree.Worktree{}, false
	}

	out := imprimeABuffer(t, func(w io.Writer) { runPrintTo(w, w, []forge.Adapter{fake}, lookup) })

	if !strings.Contains(out, "review:"+wtPath) {
		t.Errorf("la salida no integra la ruta del review activo:\n%s", out)
	}
}

// TestRunPrintWithoutReviewsKeepsF1 comprueba que sin resolvedor de reviews la
// salida de --print no cambia respecto a F1.
func TestRunPrintWithoutReviewsKeepsF1(t *testing.T) {
	it := model.NewItem(model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget"}, 7)
	it.Title = "Add widget"
	fake := &testutil.FakeAdapter{
		ForgeName: "github",
		HostName:  "github.com",
		Pages: map[testutil.FakeKey][]forge.Page{
			{Section: model.SectionAuthored}: {{Items: []model.Item{it}}},
		},
	}

	out := imprimeABuffer(t, func(w io.Writer) { runPrintTo(w, w, []forge.Adapter{fake}, nil) })

	if strings.Contains(out, "review:") {
		t.Errorf("sin reviews activos no debería aparecer la marca de F2:\n%s", out)
	}
	if !strings.Contains(out, "Created by me") || !strings.Contains(out, "Add widget") {
		t.Errorf("la salida de F1 no debería cambiar:\n%s", out)
	}
}

func names(adapters []forge.Adapter) []string {
	out := make([]string, 0, len(adapters))
	for _, a := range adapters {
		out = append(out, a.Forge())
	}
	return out
}

// TestRunPrintNoAplicaElModoDePrefijo fija la independencia de --print respecto
// al modo de prefijo de la TUI. Son dos salidas distintas por diseño: la TUI
// reparte la ruta entre una línea y las celdas porque el ancho es el recurso
// escaso, y --print no tiene columna ni terminal, así que imprime la referencia
// entera siempre.
//
// El fixture usa una ruta de subgrupo larga —la que en la TUI se recortaría por
// la cola y pondría el grupo en la línea de prefijo— para que un acoplamiento
// accidental se notara: si --print heredara el modo, saldría "…" o una línea
// "· APPCITTI/vsocial/".
func TestRunPrintNoAplicaElModoDePrefijo(t *testing.T) {
	const (
		largo  = "APPCITTI/vsocial/backend/api-gateway"
		corto  = "APPCITTI/vsocial/backend/web-app"
		numUno = 100
	)
	items := []model.Item{}
	for i, project := range []string{largo, corto} {
		it := model.NewItem(model.RepoRef{Forge: "gitlab", Host: "gitlab.example.com", Project: project}, numUno+i)
		it.Title = "T"
		items = append(items, it)
	}
	fake := &testutil.FakeAdapter{
		ForgeName: "gitlab",
		HostName:  "gitlab.example.com",
		Pages: map[testutil.FakeKey][]forge.Page{
			{Section: model.SectionReview, Kind: model.ReviewRequested}: {{Items: items}},
		},
	}

	out := imprimeABuffer(t, func(w io.Writer) { runPrintTo(w, w, []forge.Adapter{fake}, nil) })

	// La ruta completa, sin recortar y sin línea de prefijo.
	for _, want := range []string{
		largo + "#100",
		corto + "#101",
		"Review / assigned",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("la salida no contiene %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "…") {
		t.Errorf("--print recortó una referencia, y no debe: la TUI es la que recorta:\n%s", out)
	}
	// La línea de prefijo atenuada de la TUI no existe aquí: --print no compone
	// ninguna lista ni declara ningún prefijo común.
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimLeft(line, " "), "· ") {
			t.Errorf("--print pintó una línea de prefijo: %q", line)
		}
	}
}

// imprimeABuffer ejecuta fn pasándole un buffer como stdout y stderr, y devuelve lo que
// salió por stdout.
//
// Y sustituye a los dos capturadores por fd que había antes, y el motivo no es que
// `bytes.Buffer` sea más limpio: es que **`os.Pipe` tiene un búfer de 64 KiB**. Un
// capturador por fd solo devuelve si lo escrito cabe; en cuanto `fn` pasa de 64 KiB se
// bloquea escribiendo en una tubería que nadie lee hasta que `fn` vuelva, y `fn` no vuelve
// porque está bloqueado. Un test que pasa con la tabla del inbox entera y se cuelga cuando la
// tabla crece es un test que falla por la razón equivocada.
//
// Y con un buffer no hay límite ni carrera: `fn` escribe en memoria y el contenido se lee
// después.
func imprimeABuffer(t *testing.T, fn func(w io.Writer)) string {
	t.Helper()
	var buf bytes.Buffer
	fn(&buf)
	return buf.String()
}
