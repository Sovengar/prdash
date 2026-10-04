package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The safety net of the whole suite.
func TestGitHelpersNoTocanElRepoReal(t *testing.T) {
	// Un repo de verdad se monta y se consulta en su directorio.
	repo := filepath.Join(t.TempDir(), "repo")
	InitRepo(t, repo)
	CommitFile(t, repo, "base.txt", "base", "base")
	if got := RunGit(t, repo, "rev-parse", "--abbrev-ref", "HEAD"); got != "main" {
		t.Errorf("HEAD = %q, want main", got)
	}
	if !RefExists(t, repo, "refs/heads/main") {
		t.Error("el fixture debería dejar main en refs/heads/main")
	}

	origin := filepath.Join(t.TempDir(), "origin.git")
	InitBare(t, origin)
	SetRemote(t, repo, "origin", origin)
	if got := RunGit(t, repo, "remote", "get-url", "origin"); got != origin {
		t.Errorf("origin = %q, want %q", got, origin)
	}
}

// The filter's effect, with real git.
func TestGitEnvNoDejaQueElGITDirHeredadoRedirijaLasEscrituras(t *testing.T) {
	// The victim is a WORKING repo, not a bare one: that is the case that really breaks.
	victima := filepath.Join(t.TempDir(), "victima")
	if out, err := exec.Command("git", "init", "-b", "main", victima).CombinedOutput(); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	cfg := filepath.Join(victima, ".git", "config")
	antes, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("GIT_DIR", filepath.Join(victima, ".git"))
	t.Setenv("GIT_WORK_TREE", victima)

	repo := filepath.Join(t.TempDir(), "repo")
	InitRepo(t, repo)
	CommitFile(t, repo, "base.txt", "base", "base")

	if out, err := cleanGitEnv(t, "-C", repo, "config", "--get", "user.email").CombinedOutput(); err != nil {
		t.Errorf("user.email no quedó en el repo del fixture: %v\n%s", err, out)
	} else if got := strings.TrimSpace(string(out)); got != "test@prdash.local" {
		t.Errorf("user.email = %q, want test@prdash.local", got)
	}
	if out, _ := cleanGitEnv(t, "-C", repo, "rev-parse", "--is-bare-repository").CombinedOutput(); strings.TrimSpace(string(out)) != "false" {
		t.Errorf("el fixture no debería ser bare: %q", out)
	}

	// The victim's config has to stay byte for byte, so the whole file is compared.
	despues, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if string(antes) != string(despues) {
		t.Errorf("la config de la víctima cambió:\n--- antes ---\n%s\n--- después ---\n%s\nGIT_DIR le ganó a cmd.Dir", antes, despues)
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
func TestRequireDirRechazaLoQueNoEsUnDirectorio(t *testing.T) {
	if err := checkDir(""); err == nil {
		t.Error("un dir vacío debería rechazarse: git correría en el repo real")
	}
	if err := checkDir(filepath.Join(t.TempDir(), "no-existe")); err == nil {
		t.Error("una ruta inexistente debería rechazarse")
	}

	f := filepath.Join(t.TempDir(), "fichero.txt")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := checkDir(f); err == nil {
		t.Error("un fichero plano no debería valer como directorio de git")
	}

	// Y un directorio de verdad pasa.
	if err := checkDir(t.TempDir()); err != nil {
		t.Errorf("un directorio real debería valer: %v", err)
	}
}
