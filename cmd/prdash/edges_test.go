package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"prdash/internal/cache"
	"prdash/internal/config"
	"prdash/internal/forge/model"
	"prdash/internal/worktree"
)

type provisionerFailingRemove struct {
	removed  []string
	failWith error
	failsAt  map[string]bool
}

func (p *provisionerFailingRemove) Create(context.Context, worktree.Spec) (worktree.Worktree, error) {
	return worktree.Worktree{}, nil
}

func (p *provisionerFailingRemove) RemoveIfClean(context.Context, string) (bool, string, error) {
	return false, "", nil
}

func (p *provisionerFailingRemove) List(context.Context) []worktree.Worktree { return nil }

func (p *provisionerFailingRemove) Audit(context.Context) []worktree.Entry { return nil }

func (p *provisionerFailingRemove) Remove(_ context.Context, id string) error {
	p.removed = append(p.removed, id)
	if p.failsAt[id] {
		return p.failWith
	}
	return nil
}

// The exit code is the whole assertion: 0, because the session keeps going.
func TestWithNoForgesInTheConfigItWarnsAndTheSessionContinues(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := os.MkdirAll(filepath.Join(xdg, "prdash"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(xdg, "prdash", "config.toml"), []byte(`
[forge.github]
enabled = false

[forge.gitlab]
enabled = false

[forge.bitbucket]
enabled = false
`), 0o644); err != nil {
		t.Fatal(err)
	}

	// --print is the only path of run testable without a terminal.
	var stdout, stderr bytes.Buffer
	code := run([]string{"--print"}, &stdout, &stderr)

	if code != 0 {
		t.Errorf("exit code is %d without forges, want 0: the session has to start anyway "+
			"so the reason stays visible", code)
	}
	if !strings.Contains(stderr.String(), "no forges") {
		t.Errorf("stderr = %q, want the warning that there are no forges", stderr.String())
	}
	if strings.Contains(stdout.String(), "no forges") {
		t.Errorf("the warning went to stdout, which is what a script reads: %q", stdout.String())
	}
	// --print prints the headers even with no adapters, which tells "nothing to do" from "no forges".
	if strings.TrimSpace(stdout.String()) == "" {
		t.Error("stdout is empty: --print must print the table even with nothing to show")
	}
}

func TestAFailingRemovalDoesNotSinkTheRestOfTheBatch(t *testing.T) {
	base, owned, _ := worktreeFixture(t)
	other := filepath.Join(base, "prdash-pr-2")
	if err := writeAsWorktree(t, owned, other); err != nil {
		t.Fatal(err)
	}

	prov := &provisionerFailingRemove{
		failWith: errors.New("workspace is not empty"),
		failsAt:  map[string]bool{other: true},
	}
	var stdout, stderr bytes.Buffer

	code := removeWorktreesWithin(prov, &stdout, &stderr, false, false,
		[]string{owned, other, filepath.Join(base, "prdash-pr-3")}, 5*time.Second)

	if code != 1 {
		t.Errorf("code = %d with a failed removal, want 1", code)
	}
	// stdout names only what was deleted, so a script does not count a worktree still on disk.
	if !strings.Contains(stdout.String(), "worktree removed: "+owned) {
		t.Errorf("stdout does not say that %s was removed:\n%s", owned, stdout.String())
	}
	if strings.Contains(stdout.String(), other) {
		t.Errorf("stdout says that %s was removed, and it is still on disk:\n%s", other, stdout.String())
	}
	// stderr carries the cause, which is what tells whether retrying makes sense.
	errOut := stderr.String()
	if !strings.Contains(errOut, "workspace is not empty") {
		t.Errorf("stderr does not carry the cause of the failure:\n%s", errOut)
	}
	if !strings.Contains(errOut, other) {
		t.Errorf("stderr does not name the path that failed:\n%s", errOut)
	}
	// TWO removeOne calls, not three: the third does not exist and the entry guard rejects it.
	if len(prov.removed) != 2 {
		t.Errorf("tried to remove %d paths, want 2: %v", len(prov.removed), prov.removed)
	}
	if strings.Contains(stdout.String(), "prdash-pr-3") {
		t.Errorf("stdout mentions a path that was never even attempted:\n%s", stdout.String())
	}
	if !strings.Contains(errOut, "prdash-pr-3") {
		t.Errorf("stderr does not explain the rejection of the third path:\n%s", errOut)
	}
}

// Copies a real worktree's .git to get a second worktree without a git worktree add.
func writeAsWorktree(t *testing.T, source, target string) error {
	t.Helper()
	gitdir, err := os.ReadFile(filepath.Join(source, ".git"))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(target, ".git"), gitdir, 0o644)
}

func TestTheWorktreesSubcommandBuildsNeitherAdaptersNorExecutor(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := os.MkdirAll(filepath.Join(xdg, "prdash"), 0o755); err != nil {
		t.Fatal(err)
	}
	config := "[worktree]\ndir = " + filepath.Join(t.TempDir(), "worktrees") + "\n\n" +
		"[forge.github]\nenabled = false\n\n[forge.gitlab]\nenabled = false\n"
	if err := os.WriteFile(filepath.Join(xdg, "prdash", "config.toml"),
		[]byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", t.TempDir())

	var stdout, stderr bytes.Buffer
	code := run([]string{"worktrees", "list"}, &stdout, &stderr)

	if code != 0 {
		t.Errorf("exit code is %d, want 0: no worktrees is not a failure.\n%s",
			code, stderr.String())
	}
	// It says so instead of exiting with an empty map: the subcommand's output is read by scripts.
	if !strings.Contains(stdout.String()+stderr.String(), "worktree") {
		t.Errorf("the output does not mention worktrees:\nstdout: %s\nstderr: %s",
			stdout.String(), stderr.String())
	}
	// And it did NOT complain about forges, which is half the value of the branch.
	for _, want := range []string{"no forges", "gh:", "glab:"} {
		if strings.Contains(stderr.String(), want) {
			t.Errorf("the worktrees subcommand mentions %q: it built something it does "+
				"not need.\nstderr: %s", want, stderr.String())
		}
	}
}

func TestTheSimulationLocatorFindsTheRepoOfAMountedReview(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheDir)

	it := model.NewItem(model.RepoRef{
		Forge: "github", Host: "github.com", Project: "acme/widget",
		Owner: "acme", Name: "widget",
	}, 7)

	loc := simLocator{ex: buildExecutor(config.Defaults())}
	if _, ok := loc.Locate(it); ok {
		t.Fatal("with no review recorded the locator claimed it can locate one")
	}

	memo := cache.Memo{
		Version: 1,
		Reviews: map[string]cache.ReviewRecord{
			"github/github.com/acme/widget#7": {
				Repo:     "/clones/acme/widget.git",
				Worktree: "/wt/prdash-pr-7",
				Branch:   "prdash/pr-7",
				Label:    "prdash-pr-7",
			},
		},
	}
	route := filepath.Join(cacheDir, cache.DirName, cache.MemoFileName)
	if err := os.MkdirAll(filepath.Dir(route), 0o755); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(memo)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(route, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	loc = simLocator{ex: buildExecutor(config.Defaults())}
	place, ok := loc.Locate(it)
	if !ok {
		t.Fatal("with the review recorded in the memo the locator said no: the memo key " +
			"does not match the one the executor builds")
	}
	if place.Repo != "/clones/acme/widget.git" {
		t.Errorf("Repo = %q, want the recorded clone", place.Repo)
	}
	if place.Branch != "prdash/pr-7" {
		t.Errorf("Branch = %q, want the review's branch", place.Branch)
	}

	// A DIFFERENT item does not get confused with this one: the record is per item.
	other := model.NewItem(it.Ref, 8)
	if _, ok := loc.Locate(other); ok {
		t.Error("the locator found the review of another item: the record is per number")
	}
}
