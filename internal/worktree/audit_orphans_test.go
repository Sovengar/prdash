package worktree

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTheGitFileOfAWorktreeSaysWhereTheOriginIs(t *testing.T) {
	base := t.TempDir()

	abs := filepath.Join(base, "abs")
	mkdir(t, abs)
	writeGitFile(t, abs, "gitdir: "+filepath.Join(base, "repo", ".git", "worktrees", "wt"))
	if got := linkedGitDir(abs); got != filepath.Join(base, "repo", ".git", "worktrees", "wt") {
		t.Errorf("an absolute path gave %q", got)
	}

	rel := filepath.Join(base, "rel")
	mkdir(t, rel)
	writeGitFile(t, rel, "gitdir: ../repo/.git/worktrees/wt")
	want := filepath.Join(base, "repo", ".git", "worktrees", "wt")
	if got := linkedGitDir(rel); got != want {
		t.Errorf("a relative path gave %q, want %q (relative to the worktree)", got, want)
	}

	withNewline := filepath.Join(base, "with-newline")
	mkdir(t, withNewline)
	writeGitFile(t, withNewline, "gitdir:   "+filepath.Join(base, "other")+"  \n")
	if got := linkedGitDir(withNewline); got != filepath.Join(base, "other") {
		t.Errorf("with spaces and a newline gave %q", got)
	}

	cases := []struct {
		name    string
		value   string
		missing bool
	}{
		{"no prefix", "/other/cosa\n", false},
		{"empty gitdir", "gitdir:\n", false},
		{"gitdir with spaces", "gitdir:    \n", false},
		{"other prefix", "worktree: /x\n", false},
		{"missing file", "", true},
	}
	for _, c := range cases {
		dir := filepath.Join(base, "neg-"+strings.ReplaceAll(c.name, " ", "-"))
		mkdir(t, dir)
		if !c.missing {
			writeGitFile(t, dir, c.value)
		}
		if got := linkedGitDir(dir); got != "" {
			t.Errorf("%s: gave %q, want the empty string", c.name, got)
		}
	}

	repo := filepath.Join(base, "repo-normal")
	mkdir(t, filepath.Join(repo, ".git"))
	if got := linkedGitDir(repo); got != "" {
		t.Errorf("a normal repo gave gitdir %q: a .git that is a directory is not a link", got)
	}
}

func TestABrokenLinkIsAnOrphanAndAHealthyOneIsNot(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "repo", ".git", "worktrees")
	mkdir(t, source)

	healthy := filepath.Join(base, "sano")
	mkdir(t, healthy)
	writeFixtureFile(t, filepath.Join(source, "wt-sano"), "")
	writeGitFile(t, healthy, "gitdir: "+filepath.Join(source, "wt-sano"))
	if !sourceReachable(healthy) {
		t.Error("a healthy link gave false")
	}

	broken := filepath.Join(base, "roto")
	mkdir(t, broken)
	writeGitFile(t, broken, "gitdir: "+filepath.Join(source, "wt-that-does-not-exist"))
	if sourceReachable(broken) {
		t.Error("a broken link gave true: the worktree would be reported alive and occupy disk forever")
	}

	empty := filepath.Join(base, "vacio")
	mkdir(t, empty)
	if sourceReachable(empty) {
		t.Error("a directory without .git gave true")
	}
	invalid := filepath.Join(base, "invalido")
	mkdir(t, invalid)
	writeGitFile(t, invalid, "this is not a link")
	if sourceReachable(invalid) {
		t.Error("a .git without a prefix gave true")
	}
}

func TestAuditOnlyReturnsWhatBelongsToPrdashAndFlagsTheBroken(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "worktrees")
	mkdir(t, root)

	source := filepath.Join(base, "repo", ".git", "worktrees")
	mkdir(t, source)
	writeFixtureFile(t, filepath.Join(source, "wt-1"), "")
	writeFixtureFile(t, filepath.Join(source, "wt-ajeno"), "")

	userRepo := filepath.Join(base, "repo-of-the-user")
	mkdir(t, filepath.Join(userRepo, ".git"))

	alive := filepath.Join(root, "prdash-pr-1")
	mkdir(t, alive)
	writeGitFile(t, alive, "gitdir: "+filepath.Join(source, "wt-1"))

	broken := filepath.Join(root, "prdash-pr-2")
	mkdir(t, broken)
	writeGitFile(t, broken, "gitdir: "+filepath.Join(source, "wt-that-was-deleted"))

	foreign := filepath.Join(root, "vroom-pr-9")
	mkdir(t, foreign)
	writeGitFile(t, foreign, "gitdir: "+filepath.Join(source, "wt-ajeno"))

	mkdir(t, filepath.Join(root, "a-carpeta"))

	entries := NewGitDirect(root).Audit(context.Background())

	seen := map[string]Entry{}
	for _, e := range entries {
		seen[filepath.Base(e.Path)] = e
	}

	if len(entries) != 2 {
		t.Errorf("Audit returned %d entries, want 2 (only prdash's): %+v", len(entries), entries)
	}
	if _, ok := seen["repo-of-the-user"]; ok {
		t.Error("a normal user repo showed up in the listing: prdash worktrees remove could delete it")
	}
	if _, ok := seen["vroom-pr-9"]; ok {
		t.Error("a worktree of another tool showed up: ownership did not filter")
	}

	v, ok := seen["prdash-pr-1"]
	if !ok {
		t.Fatal("the live worktree did not show up")
	}
	if v.Orphan {
		t.Errorf("the live worktree showed up as an orphan: %+v", v)
	}
	r, ok := seen["prdash-pr-2"]
	if !ok {
		t.Fatal("the orphan did not show up: it must show up so it can be cleaned")
	}
	if !r.Orphan {
		t.Errorf("the worktree with the broken link did not show up as an orphan: %+v", r)
	}
	if strings.TrimSpace(r.Reason) == "" {
		t.Error("the orphan showed up with no reason: a warning with no reason does not say what to fix")
	}
	if !strings.Contains(r.Reason, "no longer reachable") {
		t.Errorf("the reason %q does not say the source repo is gone", r.Reason)
	}

	// The label wins over the directory name, so the listing says "prdash-pr-1".
	if v.Label != "prdash-pr-1" {
		t.Errorf("Label = %q, want prdash-pr-1", v.Label)
	}

	// Sorted by path, which is what makes the output stable between runs.
	for i := 1; i < len(entries); i++ {
		if entries[i-1].Path > entries[i].Path {
			t.Errorf("the listing is not sorted by path: %q before %q",
				entries[i-1].Path, entries[i].Path)
		}
	}
}

func TestAuditWithoutRootReturnsNothing(t *testing.T) {
	if got := (&GitDirect{}).Audit(context.Background()); got != nil {
		t.Errorf("without Base it returned %+v, want nil", got)
	}
	missing := filepath.Join(t.TempDir(), "no-existe")
	if got := NewGitDirect(missing).Audit(context.Background()); len(got) != 0 {
		t.Errorf("a nonexistent root returned %+v", got)
	}
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeFixtureFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeGitFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(path, ".git"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
