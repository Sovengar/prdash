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
	cfg := config.Defaults()
	got := names(buildAdapters(cfg))
	if strings.Join(got, ",") != "github,gitlab" {
		t.Fatalf("adapters = %v", got)
	}

	cfg.Forges.Bitbucket.Enabled = true
	got = names(buildAdapters(cfg))
	if strings.Join(got, ",") != "github,gitlab,bitbucket" {
		t.Fatalf("adapters with bitbucket = %v", got)
	}

	cfg = config.Defaults()
	cfg.Forges.GitHub.Enabled = false
	cfg.Forges.GitLab.Enabled = false
	if got := buildAdapters(cfg); len(got) != 0 {
		t.Fatalf("with no forges enabled there should be no adapters: %v", names(got))
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

	out := printToBuffer(t, func(w io.Writer) { runPrintTo(w, w, []forge.Adapter{fake}, nil) })

	for _, want := range []string{"Created by me", "github@github.com", "acme/widget#7", "Add widget"} {
		if !strings.Contains(out, want) {
			t.Errorf("the output does not contain %q:\n%s", want, out)
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
	out := printToBuffer(t, func(w io.Writer) {
		runPrintTo(w, w, []forge.Adapter{mk("github"), mk("gitlab")}, nil)
	})
	if strings.Index(out, "T-github") > strings.Index(out, "T-gitlab") {
		t.Errorf("the print order must follow the adapter order:\n%s", out)
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

	out := printToBuffer(t, func(w io.Writer) { runPrintTo(w, w, []forge.Adapter{fake}, lookup) })

	if !strings.Contains(out, "review:"+wtPath) {
		t.Errorf("the output does not integrate the active review path:\n%s", out)
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

	out := printToBuffer(t, func(w io.Writer) { runPrintTo(w, w, []forge.Adapter{fake}, nil) })

	if strings.Contains(out, "review:") {
		t.Errorf("without active reviews the F2 marker should not appear:\n%s", out)
	}
	if !strings.Contains(out, "Created by me") || !strings.Contains(out, "Add widget") {
		t.Errorf("the F1 output should not change:\n%s", out)
	}
}

func names(adapters []forge.Adapter) []string {
	out := make([]string, 0, len(adapters))
	for _, a := range adapters {
		out = append(out, a.Forge())
	}
	return out
}

// --print is independent of the TUI's prefix mode: the TUI splits the path, --print does not.
func TestRunPrintDoesNotApplyThePrefixMode(t *testing.T) {
	const (
		longPath    = "APPCITTI/vsocial/backend/api-gateway"
		shortPath   = "APPCITTI/vsocial/backend/web-app"
		firstNumber = 100
	)
	items := []model.Item{}
	for i, project := range []string{longPath, shortPath} {
		it := model.NewItem(model.RepoRef{Forge: "gitlab", Host: "gitlab.example.com", Project: project}, firstNumber+i)
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

	out := printToBuffer(t, func(w io.Writer) { runPrintTo(w, w, []forge.Adapter{fake}, nil) })

	for _, want := range []string{
		longPath + "#100",
		shortPath + "#101",
		"Review / assigned",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the output does not contain %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "…") {
		t.Errorf("--print truncated a reference, and it must not: the TUI is the one that truncates:\n%s", out)
	}
	// The TUI's dimmed prefix line does not exist here: --print declares no common prefix.
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimLeft(line, " "), "· ") {
			t.Errorf("--print painted a prefix line: %q", line)
		}
	}
}

func printToBuffer(t *testing.T, fn func(w io.Writer)) string {
	t.Helper()
	var buf bytes.Buffer
	fn(&buf)
	return buf.String()
}
