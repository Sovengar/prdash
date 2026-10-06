package reporesolver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

func TestEnsureBareCleansLeftoversOfAFailedAttempt(t *testing.T) {
	origin, _ := fixture(t)
	ref := ghRef()
	r := newResolver(t, origin, ref)

	dest := r.barePath(ref)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	junk := filepath.Join(dest, "BASURA")
	if err := os.WriteFile(junk, []byte("leftovers of a failed attempt"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := r.EnsureBare(t.Context(), ref)
	if err != nil {
		t.Fatalf("EnsureBare with previous leftovers gave %v, want nil: the leftovers are cleaned before retrying", err)
	}
	if got != dest {
		t.Fatalf("EnsureBare returned %q, want %q", got, dest)
	}
	if _, err := os.Stat(junk); err == nil {
		t.Error("the junk from the previous attempt is still there: the directory was used without cleaning")
	}
	if !isRepo(dest) {
		t.Error("where the clone should be there is no clone")
	}
}

// An empty directory is a leftover just as much as one with rubbish in it, and it is its own case.
func TestEnsureBareAlsoWorksWithAnEmptyLeftover(t *testing.T) {
	origin, _ := fixture(t)
	ref := ghRef()
	r := newResolver(t, origin, ref)

	dest := r.barePath(ref)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := r.EnsureBare(t.Context(), ref); err != nil {
		t.Fatalf("EnsureBare with an empty directory gave %v, want nil", err)
	}
	if !isRepo(dest) {
		t.Error("where the clone should be there is no clone")
	}
}

// The ROOT is not pruned even if its name starts with a dot.
func TestBuildIndexDoesNotEnterHiddenDirectoriesNorSkipTheRoot(t *testing.T) {
	// Two different origins so the indexed repo can be told apart.
	visibleOrigin := filepath.Join(t.TempDir(), "visible.git")
	testutil.InitBare(t, visibleOrigin)
	hiddenOrigin := filepath.Join(t.TempDir(), "hidden.git")
	testutil.InitBare(t, hiddenOrigin)

	visibleRef := ghRef()
	hiddenRef := glRef("gitlab.example.com", "group/hidden")

	root := filepath.Join(t.TempDir(), ".workspace")

	visible := filepath.Join(root, "project")
	testutil.InitRepo(t, visible)
	testutil.CommitFile(t, visible, "a.txt", "a", "a")
	testutil.SetRemote(t, visible, "origin", visibleOrigin)

	hidden := filepath.Join(root, ".cache", "hidden")
	testutil.InitRepo(t, hidden)
	testutil.CommitFile(t, hidden, "b.txt", "b", "b")
	testutil.SetRemote(t, hidden, "origin", hiddenOrigin)

	r := New(Options{
		Roots:    []string{root},
		CloneDir: filepath.Join(t.TempDir(), "repos"),
		MemoPath: filepath.Join(t.TempDir(), "memo.json"),
		Hosts:    map[string]string{"github.com": "github", "gitlab.example.com": "gitlab"},
		ParseRemote: func(raw string) (model.RepoRef, bool) {
			// The runner returns git's output WITH its trailing newline, so it has to be trimmed.
			switch strings.TrimSpace(raw) {
			case visibleOrigin:
				return visibleRef, true
			case hiddenOrigin:
				return hiddenRef, true
			}
			return model.RepoRef{}, false
		},
	})

	// The hidden root is indexed: it was configured, so the user's will wins.
	if got, ok := r.ResolveLocal(visibleRef); !ok || got != visible {
		t.Errorf("ResolveLocal for the visible repo = %q, %v; want %q: a root is indexed even if its name starts with a dot",
			got, ok, visible)
	}
	if got, ok := r.ResolveLocal(hiddenRef); ok {
		t.Errorf("ResolveLocal found %q under a hidden directory: a clone inside a cache is not a user's clone",
			got)
	}
}
