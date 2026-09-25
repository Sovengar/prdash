package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

	out := captureStdout(t, func() { runPrint([]forge.Adapter{fake}, nil) })

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
	out := captureStdout(t, func() {
		runPrint([]forge.Adapter{mk("github"), mk("gitlab")}, nil)
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

	out := captureStdout(t, func() { runPrint([]forge.Adapter{fake}, lookup) })

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

	out := captureStdout(t, func() { runPrint([]forge.Adapter{fake}, nil) })

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

// captureStdout ejecuta fn capturando lo que escriba en stdout.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = old

	data, _ := io.ReadAll(r)
	_ = r.Close()
	return string(data)
}
