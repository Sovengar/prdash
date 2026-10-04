package reporesolver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

// The three discards are different and all three matter.
func TestElIndiceIgnoraLoQueNoEsUnRepoConRemotoInterpretable(t *testing.T) {
	raiz := t.TempDir()

	bueno := filepath.Join(raiz, "bueno")
	testutil.InitRepo(t, bueno)
	testutil.CommitFile(t, bueno, "a.txt", "a", "a")
	testutil.SetRemote(t, bueno, "origin", "https://github.com/acme/proyecto.git")

	sinRemoto := filepath.Join(raiz, "sin-remoto")
	testutil.InitRepo(t, sinRemoto)
	testutil.CommitFile(t, sinRemoto, "a.txt", "a", "a")

	conPath := filepath.Join(raiz, "con-path")
	testutil.InitRepo(t, conPath)
	testutil.CommitFile(t, conPath, "a.txt", "a", "a")
	testutil.SetRemote(t, conPath, "origin", filepath.Join(raiz, "otro-lugar"))

	plano := filepath.Join(raiz, "plano")
	if err := os.MkdirAll(plano, 0o755); err != nil {
		t.Fatal(err)
	}

	r := New(Options{
		Roots:    []string{raiz},
		CloneDir: filepath.Join(t.TempDir(), "clones"),
		MemoPath: filepath.Join(t.TempDir(), "memo.json"),
		Hosts:    map[string]string{"github.com": "github"},
	})
	idx := r.buildIndex()

	ref := model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/proyecto"}
	key := repoKey(ref)
	if got, ok := idx[key]; !ok {
		t.Errorf("el repo con remoto de GitHub no se indexó; el índice tiene %v", idx)
	} else if got != bueno {
		t.Errorf("el repo quedó en %q, want %q", got, bueno)
	}
	for _, no := range []string{sinRemoto, conPath, plano} {
		for k, v := range idx {
			if v == no {
				t.Errorf("%q se indexó bajo la clave %q: no es un repo con remoto interpretable", no, k)
			}
		}
	}
	if len(idx) != 1 {
		t.Errorf("el índice tiene %d entradas, want 1: %v", len(idx), idx)
	}
}

// Two similar things that are not the same: `.git` is the directory and a linked worktree is a FILE.
func TestElIndiceNoEntraEnLosGitNiEnLosWorktrees(t *testing.T) {
	raiz := t.TempDir()
	repo := filepath.Join(raiz, "repo")
	testutil.InitRepo(t, repo)
	testutil.CommitFile(t, repo, "a.txt", "a", "a")
	testutil.SetRemote(t, repo, "origin", "https://github.com/acme/proyecto.git")

	wt := filepath.Join(raiz, "mi-worktree")
	testutil.RunGit(t, repo, "branch", "feat/x")
	testutil.RunGit(t, repo, "worktree", "add", "--quiet", wt, "feat/x")

	r := New(Options{
		Roots:    []string{raiz},
		CloneDir: filepath.Join(t.TempDir(), "clones"),
		MemoPath: filepath.Join(t.TempDir(), "memo.json"),
		Hosts:    map[string]string{"github.com": "github"},
	})
	idx := r.buildIndex()

	for k, v := range idx {
		if strings.Contains(v, string(filepath.Separator)+"mi-worktree") {
			t.Errorf("el worktree se indexó como repo propio bajo %q: %q", k, v)
		}
	}
	ref := model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/proyecto"}
	if got := idx[repoKey(ref)]; got != repo {
		t.Errorf("el índice resolvió a %q, want el repo principal %q", got, repo)
	}
	for _, v := range idx {
		if strings.Contains(v, string(filepath.Separator)+".git"+string(filepath.Separator)) {
			t.Errorf("el índice tiene una entrada de dentro de un .git: %q", v)
		}
	}
}

// Proved with the real thing, not a double: filepath.Abs on a RELATIVE root whose working
// directory is gone.
func TestUnaRaizInaccesibleNoRompeElIndice(t *testing.T) {
	desaparecido := filepath.Join(t.TempDir(), "se-vale")
	if err := os.MkdirAll(desaparecido, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(desaparecido); err != nil {
		t.Fatal(err)
	}

	bueno := filepath.Join(t.TempDir(), "bueno")
	testutil.InitRepo(t, bueno)
	testutil.CommitFile(t, bueno, "a.txt", "a", "a")
	testutil.SetRemote(t, bueno, "origin", "https://github.com/acme/proyecto.git")

	t.Run("raíz relativa con cwd muerto", func(t *testing.T) {
		otro := filepath.Join(t.TempDir(), "cwd-muerto")
		if err := os.MkdirAll(otro, 0o755); err != nil {
			t.Fatal(err)
		}
		t.Chdir(otro)

		r := New(Options{
			Roots:    []string{"repo-que-no-existe", bueno},
			CloneDir: filepath.Join(t.TempDir(), "clones"),
			MemoPath: filepath.Join(t.TempDir(), "memo.json"),
			Hosts:    map[string]string{"github.com": "github"},
		})
		idx := r.buildIndex()
		ref := model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/proyecto"}
		if got := idx[repoKey(ref)]; got != bueno {
			t.Errorf("una raíz inválida se llevó el índice entero: resolvió a %q, want %q",
				got, bueno)
		}
	})

	r := New(Options{
		Roots:    []string{filepath.Join(t.TempDir(), "no-existe"), bueno},
		CloneDir: filepath.Join(t.TempDir(), "clones"),
		MemoPath: filepath.Join(t.TempDir(), "memo.json"),
		Hosts:    map[string]string{"github.com": "github"},
	})
	idx := r.buildIndex()
	ref := model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/proyecto"}
	if got := idx[repoKey(ref)]; got != bueno {
		t.Errorf("una raíz inexistente se llevó el índice entero: resolvió a %q, want %q", got, bueno)
	}
}

func TestUnRemotoConUnaParteVaciaNoSeConvierteEnUnRepo(t *testing.T) {
	for _, remote := range []string{
		"https://github.com//proyecto.git",
		"https://github.com/acme//proyecto.git",
		"https://github.com///.git",
		"git@github.com:acme//proyecto.git",
		"ssh://git@github.com",
		"no-es-un-remoto",
	} {
		ref, ok := parseRemoteDePrueba(remote)
		if ok {
			t.Errorf("%q se convirtió en un repo: %+v", remote, ref)
			continue
		}
		if ref.Project != "" {
			t.Errorf("%q dio ok=false pero Project=%q: un Caller que ignorara el ok "+
				"tendría una clave de índice con una barra", remote, ref.Project)
		}
	}

	// The good path, because otherwise "everything is rejected" would count as a test.
	// Three forms that my first version put on the rejected list.
	for _, bueno := range []string{
		"https://github.com/acme/proyecto.git",
		"https://github.com/acme/proyecto//.git",
		"git@github.com:acme/proyecto.git",
		"ssh://git@github.com/acme/proyecto.git",
	} {
		if _, ok := parseRemoteDePrueba(bueno); !ok {
			t.Errorf("%q no se interpretó, y es una forma válida de remoto", bueno)
		}
	}

	ref, ok := parseRemoteDePrueba("https://github.com/acme/grupo/proyecto.git")
	if !ok {
		t.Fatal("un remoto bien escrito no se interpretó")
	}
	if ref.Project != "acme/grupo/proyecto" {
		t.Errorf("Project = %q, want acme/grupo/proyecto: el proyecto es la ruta COMPLETA, "+
			"con grupos, porque es lo que va a `git clone`", ref.Project)
	}
	if ref.Forge != "github" || ref.Host != "github.com" {
		t.Errorf("forge/host = %s/%s", ref.Forge, ref.Host)
	}
	if ref.Owner != "grupo" || ref.Name != "proyecto" {
		t.Errorf("Owner/Name = %s/%s, want grupo/proyecto", ref.Owner, ref.Name)
	}
}

// It delegates to the package's REAL parsing, with the same hosts table production uses, so it does
// not test a copy.
func parseRemoteDePrueba(raw string) (model.RepoRef, bool) {
	return ParseRemoteURL(raw, hostsDePrueba(), nil)
}

// The message has to say WHICH ref was attempted, because there are two.
func TestFetchReviewRefPropagaElFalloDelFetchConSuRef(t *testing.T) {
	origin, repo := fixture(t)
	r := newResolver(t, origin, ghRef())
	it := model.NewItem(ghRef(), 7)
	it.Number = 7

	_, err := r.FetchReviewRef(context.Background(), repo, it)
	if err == nil {
		t.Fatal("traer un ref que no existe dio nil")
	}
	if !strings.Contains(err.Error(), "fetch") {
		t.Errorf("el error %q no dice que falla el fetch", err)
	}
	if !strings.Contains(err.Error(), "acme") {
		t.Errorf("el error %q no nombra el proyecto del que seerró", err)
	}
	for _, ref := range []string{"prdash/pr-7", "refs/prdash/github/7"} {
		if testutil.RefExists(t, repo, ref) {
			t.Errorf("el fetch fallido dejó %s en el repo: Create lo reutilizaría y el "+
				"review se montaría vacío", ref)
		}
	}
}
