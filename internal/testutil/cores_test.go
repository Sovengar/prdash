package testutil

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The in-process half of the package's guards; the other half is the subprocess tests.

// Each case checks something different.
func TestTheGuardsReturnTheReason(t *testing.T) {
	dir := t.TempDir()

	// The reason comes from checkDir and the path prefix.
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		dir  string
		want string
	}{
		{"missing dir", filepath.Join(dir, "does-not-exist"), "does not exist"},
		{"empty dir", "", "empty dir"},
		{"dir that is not a directory", file, "is not a directory"},
	} {
		_, err := runGit(c.dir, "status")
		if err == nil {
			t.Errorf("%s: runGit gave nil", c.name)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: the error %q does not say %q", c.name, err, c.want)
		}
		if !strings.Contains(err.Error(), "testutil:") {
			t.Errorf("%s: the error does not carry the package prefix: %q", c.name, err)
		}
	}

	repo := filepath.Join(dir, "repo")
	InitRepo(t, repo)
	_, err := runGit(repo, "checkout", "branch-that-does-not-exist")
	if err == nil {
		t.Fatal("a checkout of a nonexistent branch gave nil")
	}
	for _, want := range []string{"checkout", "branch-that-does-not-exist", repo} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error %q does not say %q", err, want)
		}
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Errorf("the error does not wrap the *exec.ExitError, so it cannot be classified "+
			"without parsing text: %v", err)
	}

	out, err := runGit(repo, "status", "--porcelain")
	if err != nil {
		t.Fatalf("runGit on the good path: %v", err)
	}
	if out != "" {
		t.Errorf("a freshly created repo gave status %q, want empty", out)
	}
}

// The guard that cost the most.
func TestInitBareRefusesTheEmptyDirectoryWithTheReasonForTheDamage(t *testing.T) {
	err := initBare("")
	if err == nil {
		t.Fatal("initBare with an empty dir gave nil: `git init --bare` would run in the real repo")
	}
	if !strings.Contains(err.Error(), "core.bare") {
		t.Errorf("the error %q does not say what it would break in the real repo", err)
	}

	blocked := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blocked, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := initBare(filepath.Join(blocked, "repo.git")); err == nil {
		t.Error("initBare with a parent that is a file gave nil")
	}

	nested := filepath.Join(t.TempDir(), "a", "b", "c", "repo.git")
	if err := initBare(nested); err != nil {
		t.Errorf("nested initBare: %v", err)
	}
	if _, err := os.Stat(nested); err != nil {
		t.Errorf("initBare did not create the directory: %v", err)
	}
}

// Two guards that fail for different reasons.
func TestCommitFileDistinguishesTheTwoWriteFailures(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	InitRepo(t, repo)

	// The file's parent exists as a file, and the repo has to be real: with only the directory,
	//git refuses.
	blocked := filepath.Join(repo, "blocked")
	if err := os.WriteFile(blocked, []byte("I am a file"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := commitFile(repo, "blocked/uploaded.txt", "x")
	if err == nil {
		t.Fatal("commitFile with a parent that is a file gave nil")
	}
	// The message NAMES the file that could not be written.
	if !strings.Contains(err.Error(), "blocked/uploaded.txt") {
		t.Errorf("the error %q does not say which file could not be written", err)
	}

	onlyDir := filepath.Join(repo, "I-am-a-dir")
	if err := os.MkdirAll(onlyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := commitFile(repo, "I-am-a-dir", "x"); err == nil {
		t.Error("commitFile with a destination that is a directory gave nil")
	}

	if err := commitFile(repo, "nested/uploaded.txt", "content"); err != nil {
		t.Fatalf("commitFile on the good path: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(repo, "nested", "uploaded.txt"))
	if err != nil {
		t.Fatalf("reading what was written: %v", err)
	}
	if string(raw) != "content" {
		t.Errorf("the file has %q, want content", raw)
	}
}
