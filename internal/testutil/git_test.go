package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The safety net of the whole suite.
func TestGitHelpersDoNotTouchTheRealRepo(t *testing.T) {
	// A real repo is set up and queried in its directory.
	repo := filepath.Join(t.TempDir(), "repo")
	InitRepo(t, repo)
	CommitFile(t, repo, "base.txt", "base", "base")
	if got := RunGit(t, repo, "rev-parse", "--abbrev-ref", "HEAD"); got != "main" {
		t.Errorf("HEAD = %q, want main", got)
	}
	if !RefExists(t, repo, "refs/heads/main") {
		t.Error("the fixture should leave main at refs/heads/main")
	}

	origin := filepath.Join(t.TempDir(), "origin.git")
	InitBare(t, origin)
	SetRemote(t, repo, "origin", origin)
	if got := RunGit(t, repo, "remote", "get-url", "origin"); got != origin {
		t.Errorf("origin = %q, want %q", got, origin)
	}
}

// The filter's effect, with real git.
func TestGitEnvDoesNotLetTheInheritedGITDirRedirectTheWrites(t *testing.T) {
	// The victim is a WORKING repo, not a bare one: that is the case that really breaks.
	victim := filepath.Join(t.TempDir(), "victim")
	if out, err := exec.Command("git", "init", "-b", "main", victim).CombinedOutput(); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	cfg := filepath.Join(victim, ".git", "config")
	before, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("GIT_DIR", filepath.Join(victim, ".git"))
	t.Setenv("GIT_WORK_TREE", victim)

	repo := filepath.Join(t.TempDir(), "repo")
	InitRepo(t, repo)
	CommitFile(t, repo, "base.txt", "base", "base")

	if out, err := cleanGitEnv(t, "-C", repo, "config", "--get", "user.email").CombinedOutput(); err != nil {
		t.Errorf("user.email was not left in the fixture repo: %v\n%s", err, out)
	} else if got := strings.TrimSpace(string(out)); got != "test@prdash.local" {
		t.Errorf("user.email = %q, want test@prdash.local", got)
	}
	if out, _ := cleanGitEnv(t, "-C", repo, "rev-parse", "--is-bare-repository").CombinedOutput(); strings.TrimSpace(string(out)) != "false" {
		t.Errorf("the fixture should not be bare: %q", out)
	}

	// The victim's config has to stay byte for byte, so the whole file is compared.
	after, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Errorf("the victim's config changed:\n--- before ---\n%s\n--- after ---\n%s\nGIT_DIR beat cmd.Dir", before, after)
	}
}

// git runs without the test environment's GIT_* localisation vars, which would point it
// somewhere else.
func cleanGitEnv(t *testing.T, args ...string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Env = gitEnv()
	return cmd
}

// The guard has to cut both the empty dir (git would run in the real repo) and the rest.
func TestRequireDirRefusesWhatIsNotADirectory(t *testing.T) {
	if err := checkDir(""); err == nil {
		t.Error("an empty dir should be refused: git would run in the real repo")
	}
	if err := checkDir(filepath.Join(t.TempDir(), "does-not-exist")); err == nil {
		t.Error("a nonexistent path should be refused")
	}

	f := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := checkDir(f); err == nil {
		t.Error("a plain file should not serve as a git directory")
	}

	// And a real directory passes.
	if err := checkDir(t.TempDir()); err != nil {
		t.Errorf("a real directory should serve: %v", err)
	}
}
