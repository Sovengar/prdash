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

// --print is independent of the TUI's prefix mode by design: the TUI splits the path and --print
// does not.
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
	// The TUI's dimmed prefix line does not exist here: --print composes no list and declares no common
	// prefix.
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimLeft(line, " "), "· ") {
			t.Errorf("--print pintó una línea de prefijo: %q", line)
		}
	}
}

// It replaces the two fd captures that were there before.
func imprimeABuffer(t *testing.T, fn func(w io.Writer)) string {
	t.Helper()
	var buf bytes.Buffer
	fn(&buf)
	return buf.String()
}
