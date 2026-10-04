package worktree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/testutil"
)

var separadorGit = string(filepath.Separator) + ".git" + string(filepath.Separator)

// It leaves ruta's parent unwritable, so removing or creating fails.
func conPadreEnSoloLectura(t *testing.T, padre string) {
	t.Helper()
	if err := os.Chmod(padre, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// Restored, or t.TempDir() cannot clean up and the failure is reported as an unrelated
		// cleanup error.
		_ = os.Chmod(padre, 0o755)
	})
}

func TestCreateFallaSiElPadreDelDestinoNoSePuedeCrearYNoDejaNada(t *testing.T) {
	repo := repoConRama(t, "feat/x")
	raiz := t.TempDir()
	g := NewGitDirect(raiz)

	bloqueo := filepath.Join(raiz, "prdash-bloqueado")
	if err := os.WriteFile(bloqueo, []byte("soy un fichero"), 0o644); err != nil {
		t.Fatal(err)
	}

	destino := filepath.Join(bloqueo, "prdash-pr-7")
	_, err := g.Create(context.Background(), Spec{
		Repo: repo, Branch: "feat/x", Path: destino, Label: "prdash-pr-7",
	})
	if err == nil {
		t.Fatal("crear un worktree bajo un padre que es fichero dio nil")
	}
	if !strings.Contains(err.Error(), "prepare the worktree destination") {
		t.Errorf("el error %q no dice que falla preparar el destino", err)
	}
	raw, err := os.ReadFile(bloqueo)
	if err != nil || string(raw) != "soy un fichero" {
		t.Errorf("el bloqueo cambió: %q %v", raw, err)
	}
}

// The deletion that cannot delete.
func TestRemovePropagaElFalloDeQuitarElCheckout(t *testing.T) {
	for _, c := range []struct {
		nombre  string
		prepara func(t *testing.T, raiz string) string
		quiere  string
	}{
		{
			nombre: "huérfano sin repo detrás",
			prepara: func(t *testing.T, raiz string) string {
				d := filepath.Join(raiz, "prdash-pr-7")
				if err := os.MkdirAll(d, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(d, ".git"), []byte("gitdir: \n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(d, "trabajo.txt"), []byte("x"), 0o644); err != nil {
					t.Fatal(err)
				}
				return d
			},
			quiere: "remove the worktree checkout",
		},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			raiz := filepath.Join(t.TempDir(), "raiz")
			if err := os.MkdirAll(raiz, 0o755); err != nil {
				t.Fatal(err)
			}
			id := c.prepara(t, raiz)
			conPadreEnSoloLectura(t, raiz)

			err := NewGitDirect(raiz).Remove(context.Background(), id)
			if err == nil {
				t.Fatal("Remove con el padre en solo lectura dio nil: el checkout sigue ahí " +
					"y nadie se ha enterado")
			}
			if !strings.Contains(err.Error(), c.quiere) {
				t.Errorf("el error %q no dice %q", err, c.quiere)
			}
			if !strings.Contains(err.Error(), id) {
				t.Errorf("el error %q no nombra la ruta que no se pudo quitar", err)
			}
		})
	}
}

func TestRemoveIfCleanPropagaElFalloDeQuitarloCuandoEstaLimpio(t *testing.T) {
	raiz := filepath.Join(t.TempDir(), "raiz")
	if err := os.MkdirAll(raiz, 0o755); err != nil {
		t.Fatal(err)
	}
	d := filepath.Join(raiz, "prdash-pr-7")
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, ".git"), []byte("gitdir: \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	g := NewGitDirect(raiz)

	borrado, motivo, err := g.RemoveIfClean(context.Background(), d)
	if err != nil {
		t.Fatalf("RemoveIfClean de un huérfano: %v", err)
	}
	if borrado {
		t.Error("un huérfano sin repo se borró: no se puede comprobar que esté limpio")
	}
	if !strings.Contains(motivo, "status") {
		t.Errorf("motivo = %q, y debería decir que no se pudo leer el estado", motivo)
	}

	repo := repoConRama(t, "feat/x")
	raiz2 := filepath.Join(t.TempDir(), "raiz")
	if err := os.MkdirAll(raiz2, 0o755); err != nil {
		t.Fatal(err)
	}
	wt, err := NewGitDirect(raiz2).Create(context.Background(), Spec{
		Repo: repo, Branch: "feat/x", Path: filepath.Join(raiz2, "prdash-pr-7"),
		Label: "prdash-pr-7",
	})
	if err != nil {
		t.Fatal(err)
	}
	conPadreEnSoloLectura(t, raiz2)
	if err := os.Chmod(wt.Path, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(wt.Path, 0o755) })

	borrado, motivo, err = gEn(raiz2).RemoveIfClean(context.Background(), wt.Path)
	if err == nil {
		t.Fatal("el fallo de quitar no se propagó: el worktree sigue ahí sin que nadie lo sepa")
	}
	if borrado {
		t.Error("se dijo que se quitó y no se quitó")
	}
	if motivo != "" {
		t.Errorf("motivo = %q: un fallo de quitar no es un motivo de no quitar", motivo)
	}
	if !Exists(wt.Path) {
		t.Error("el worktree se quitó a pesar del error")
	}
}

func gEn(raiz string) *GitDirect { return NewGitDirect(raiz) }

// Audit's SkipDir.
func TestAuditSaltaLosDirectoriosGitAnidados(t *testing.T) {
	raiz := t.TempDir()
	repo := filepath.Join(raiz, "no-es-prdash")
	testutil.InitRepo(t, repo)
	testutil.CommitFile(t, repo, "f.txt", "x", "x")

	origen := repoConRama(t, "feat/x")
	g := NewGitDirect(raiz)
	if _, err := g.Create(context.Background(), Spec{
		Repo: origen, Branch: "feat/x", Path: filepath.Join(raiz, "prdash-pr-7"),
		Label: "prdash-pr-7",
	}); err != nil {
		t.Fatal(err)
	}

	entradas := g.Audit(context.Background())
	if len(entradas) != 1 || entradas[0].Path != filepath.Join(raiz, "prdash-pr-7") {
		t.Fatalf("Audit devolvió %+v, want solo el worktree propio", entradas)
	}
	for _, e := range entradas {
		if strings.Contains(e.Path, separadorGit) {
			t.Errorf("Audit listó algo de dentro de un .git: %q", e.Path)
		}
	}
	for _, w := range g.List(context.Background()) {
		if strings.Contains(w.Path, separadorGit) {
			t.Errorf("List devolvió algo de dentro de un .git: %q", w.Path)
		}
	}
}

// The native provisioning delegates Audit to the scan and propagates its failures.
func TestLaProvisionNativaDelegaElAuditEnElEscaneoYPropagaSusFallos(t *testing.T) {
	raiz := t.TempDir()
	repo := repoConRama(t, "feat/x")
	g := NewGitDirect(raiz)
	if _, err := g.Create(context.Background(), Spec{
		Repo: repo, Branch: "feat/x", Path: filepath.Join(raiz, "prdash-pr-7"),
		Label: "prdash-pr-7",
	}); err != nil {
		t.Fatal(err)
	}
	nativo := NewHerdrNative(&fakeRunner{available: true}, raiz)

	entradas := nativo.Audit(context.Background())
	if len(entradas) != 1 {
		t.Errorf("Audit nativo devolvió %d entradas, want 1", len(entradas))
	}
	if len(nativo.List(context.Background())) != 1 {
		t.Errorf("List nativo devolvió %d worktrees, want 1", len(nativo.List(context.Background())))
	}

	fallido := NewHerdrNative(&fakeRunner{
		available: true, createErr: errors.New("workspace_limit"),
	}, filepath.Join(t.TempDir(), "raiz-vacia"))
	_, err := fallido.Create(context.Background(), Spec{
		Repo: repo, Branch: "feat/x", Path: filepath.Join(t.TempDir(), "prdash-pr-7"),
		Label: "prdash-pr-7",
	})
	if err == nil {
		t.Fatal("Create nativo con Herdr fallando dio nil: el popup se abriría sin review")
	}
	if !strings.Contains(err.Error(), "workspace_limit") {
		t.Errorf("el error %q no trae la causa de Herdr", err)
	}
	if !strings.Contains(err.Error(), "create the native worktree") {
		t.Errorf("el error %q no dice que falla la creación nativa", err)
	}

	if _, err := fallido.Create(context.Background(), Spec{
		Repo: repo, Branch: "feat/x", Path: filepath.Join(t.TempDir(), "no-existe"),
		Label: "prdash-pr-7",
	}); err == nil {
		t.Error("Create nativo sobre un destino vacío con Herdr que no crea dio nil")
	}
}
