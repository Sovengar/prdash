package worktree

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"prdash/internal/testutil"
)

// The three survivors in worktree.go were three negatives in the provisioning path.

func TestTheWorktreeLabelIsTheSpecs(t *testing.T) {
	repo := newRepo(t)
	base := t.TempDir()

	// Real branches: Create takes them from the repo, so an invented name fails before
	// reaching the label and the test would prove nothing.
	for _, b := range []string{"con-etiqueta", "sin-etiqueta", "etiqueta-igual-al-dir",
		"directorio-anidado", "w-temp-8837"} {
		testutil.RunGit(t, repo, "branch", b)
	}

	cases := []struct {
		name      string
		dir       string
		label     string
		wantLabel string
		note      string
	}{
		{"con-etiqueta", "prdash-acme-12", "prdash/acme#12", "prdash/acme#12",
			"the spec's wins: it is the property with which prdash recognises the worktree"},
		{"sin-etiqueta", "prdash-acme-12", "", "prdash-acme-12",
			"with no label the directory name is the last resort"},
		{"etiqueta-igual-al-dir", "prdash-acme-12", "prdash-acme-12", "prdash-acme-12",
			"they match, and it does not matter which of the two wins"},
		{"directorio-anidado", "sub/dir/prdash-acme-12", "prdash/acme#12", "prdash/acme#12",
			"the directory may be nested; the label is not deduced from it"},
	}

	for _, c := range cases {
		dest := filepath.Join(base, c.name, c.dir)
		wt, err := NewGitDirect(base).Create(context.Background(),
			Spec{Repo: repo, Branch: c.name, Path: dest, Label: c.label})
		if err != nil {
			t.Fatalf("case %q: Create: %v", c.name, err)
		}
		if wt.Label != c.wantLabel {
			t.Errorf("case %q: the label came out %q, want %q. %s",
				c.name, wt.Label, c.wantLabel, c.note)
		}
	}

	noHint := filepath.Join(base, "w-temp-8837")
	wt, err := NewGitDirect(base).Create(context.Background(),
		Spec{Repo: repo, Branch: "w-temp-8837", Path: noHint})
	if err != nil {
		t.Fatalf("Create without a hint: %v", err)
	}
	if wt.Label != "w-temp-8837" {
		t.Errorf("with no label and a directory with no hint it gave %q, want the directory "+
			"name. An empty label in Herdr's header leaves the tab without a "+
			"name and there is no telling what is being reviewed", wt.Label)
	}
}

func TestCleanPartialDoesNotDeleteOutsideTheRoot(t *testing.T) {
	base := t.TempDir()
	sibling := filepath.Join(filepath.Dir(base), filepath.Base(base)+"-hermano")
	if err := os.MkdirAll(sibling, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(sibling) })
	marker := filepath.Join(sibling, "NO-TOCAR")
	if err := os.WriteFile(marker, []byte("important"), 0o644); err != nil {
		t.Fatal(err)
	}

	g := NewGitDirect(base)

	g.cleanPartial(sibling)
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("the cleanup took %s, which is OUTSIDE the root %q: the deletion is "+
			"recursive and without confirmation, so this check is the only defence",
			marker, base)
	}
	if _, err := os.Stat(sibling); err != nil {
		t.Errorf("the cleanup deleted the whole sibling directory %s", sibling)
	}

	inside := filepath.Join(base, "resto-fallido")
	if err := os.MkdirAll(inside, 0o755); err != nil {
		t.Fatal(err)
	}
	g.cleanPartial(inside)
	if _, err := os.Stat(inside); err == nil {
		t.Errorf("the cleanup did not delete %s, which is inside the root: without this the "+
			"leftovers of a failed `worktree add` stay there and the next attempt "+
			"fails again", inside)
	}

	nested := filepath.Join(base, "a", "b", "resto")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	g.cleanPartial(nested)
	if _, err := os.Stat(nested); err == nil {
		t.Errorf("the cleanup did not delete %s, which is inside the root two levels down", nested)
	}
}

func TestCleanPartialWithoutARootDeletesNothingAndWithARootDoesNotTouchLinkedWorktrees(t *testing.T) {
	base := t.TempDir()
	g := NewGitDirect(base)

	linked := filepath.Join(base, "enlazado")
	if err := os.MkdirAll(linked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(linked, ".git"),
		[]byte("gitdir: /repo/.git/worktrees/enlazado\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	g.cleanPartial(linked)
	if _, err := os.Stat(linked); err != nil {
		t.Errorf("the cleanup deleted %s, which is a LINKED worktree: it got as far as "+
			"registering, so it is the user's work and deleting it cannot be undone",
			linked)
	}

	cloned := filepath.Join(base, "clonado")
	if err := os.MkdirAll(filepath.Join(cloned, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	g.cleanPartial(cloned)
	if _, err := os.Stat(cloned); err == nil {
		t.Errorf("the cleanup did not delete %s: a `.git` that is a directory is not a "+
			"linked worktree, and a clone is not a leftover of anything", cloned)
	}

	noRoot := NewGitDirect("")
	free := t.TempDir()

	linkedNoRoot := filepath.Join(free, "enlazado")
	if err := os.MkdirAll(linkedNoRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(linkedNoRoot, ".git"),
		[]byte("gitdir: /repo/.git/worktrees/x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	noRoot.cleanPartial(linkedNoRoot)
	if _, err := os.Stat(linkedNoRoot); err != nil {
		t.Errorf("without its own root the cleanup deleted %s, which is a linked worktree: "+
			"that is the guard that does not depend on the root, and it is the one that protects the user's "+
			"work", linkedNoRoot)
	}

	plain := filepath.Join(free, "plano")
	if err := os.MkdirAll(plain, 0o755); err != nil {
		t.Fatal(err)
	}
	noRoot.cleanPartial(plain)
	if _, err := os.Stat(plain); err == nil {
		t.Errorf("without its own root the cleanup did NOT delete %s. Today it does delete it, and this "+
			"assert is here so it shows if one day it stops doing so", plain)
	}
}
