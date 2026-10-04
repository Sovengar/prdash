package worktree

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/testutil"
)

func wtConRamaMonta(t *testing.T, raiz, etiqueta, rama string) Worktree {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "repo")
	testutil.InitRepo(t, repo)
	testutil.CommitFile(t, repo, "base.txt", "base", "base")

	// The branch has to exist and must NOT be main: git does not allow two worktrees on the same
	//branch and the main repo already has it.
	if rama == "main" {
		rama = "pr-" + etiqueta
	}
	testutil.RunGit(t, repo, "branch", rama)
	spec := Spec{
		Repo:   repo,
		Branch: rama,
		Path:   filepath.Join(raiz, etiqueta),
		Label:  etiqueta,
	}
	wt, err := NewGitDirect(raiz).Create(context.Background(), spec)
	if err != nil {
		t.Fatalf("crear el worktree %s: %v", etiqueta, err)
	}
	return wt
}

// The asymmetry is the point: the SAME branch reuses, a DIFFERENT one does not.
func TestCrearSobreUnWorktreeQueYaEstaEnLaMismaRamaLoReutiliza(t *testing.T) {
	raiz := t.TempDir()
	primero := wtConRamaMonta(t, raiz, "prdash-pr-1", "feat/x")

	g := NewGitDirect(raiz)
	segundo, err := g.Create(context.Background(), Spec{
		Repo: primero.Repo, Branch: "feat/x",
		Path: primero.Path, Label: "prdash-pr-1",
	})
	if err != nil {
		t.Fatalf("volver a crear sobre la misma rama dio error: %v", err)
	}
	if segundo.Path != primero.Path || segundo.Branch != "feat/x" {
		t.Errorf("el worktree reutilizado no es el mismo: %+v", segundo)
	}
	if n := len(strings.Split(strings.TrimSpace(
		testutil.RunGit(t, primero.Repo, "worktree", "list")), "\n")); n != 2 {
		t.Errorf("hay %d líneas en worktree list, want 2", n)
	}

	testutil.RunGit(t, primero.Repo, "branch", "otra")
	_, err = g.Create(context.Background(), Spec{
		Repo: primero.Repo, Branch: "otra",
		Path: primero.Path, Label: "prdash-pr-1",
	})
	if err == nil {
		t.Fatal("crear con otra rama sobre el mismo worktree dio nil")
	}
	for _, quiere := range []string{"feat/x", "otra"} {
		if !strings.Contains(err.Error(), quiere) {
			t.Errorf("el error %q no menciona %q", err, quiere)
		}
	}
	if got := testutil.RunGit(t, primero.Path, "rev-parse", "--abbrev-ref", "HEAD"); got != "feat/x" {
		t.Errorf("tras el rechazo la rama quedó en %q, want feat/x", got)
	}
}

func TestLaEtiquetaDelWorktreeLaPoneElNombreDelDirectorioSiNoViene(t *testing.T) {
	raiz := t.TempDir()
	repo := filepath.Join(t.TempDir(), "repo")
	testutil.InitRepo(t, repo)
	testutil.CommitFile(t, repo, "a.txt", "a", "a")

	g := NewGitDirect(raiz)

	testutil.RunGit(t, repo, "branch", "pr-7")
	wt, err := g.Create(context.Background(), Spec{
		Repo: repo, Branch: "pr-7", Path: filepath.Join(raiz, "prdash-pr-7"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if wt.Label != "prdash-pr-7" {
		t.Errorf("sin etiqueta salió %q, want el nombre del directorio", wt.Label)
	}
	if !Owned(wt.Label, wt.Path) {
		t.Errorf("la etiqueta por defecto %q no la reconoce Owned", wt.Label)
	}

	testutil.RunGit(t, repo, "branch", "pr-9")
	conEtiqueta, err := g.Create(context.Background(), Spec{
		Repo: repo, Branch: "pr-9",
		Path: filepath.Join(raiz, "sin-prefijo"), Label: "prdash-pr-9",
	})
	if err != nil {
		t.Fatal(err)
	}
	if conEtiqueta.Label != "prdash-pr-9" {
		t.Errorf("con etiqueta salió %q", conEtiqueta.Label)
	}
	if !Owned(conEtiqueta.Label, conEtiqueta.Path) {
		t.Errorf("con etiqueta %q en un directorio sin prefijo, Owned no lo reconoce",
			conEtiqueta.Label)
	}
	if conEtiqueta.ID != conEtiqueta.Path || conEtiqueta.Path == "" {
		t.Errorf("ID/Path no son la ruta: %+v", conEtiqueta)
	}
	if conEtiqueta.Repo != repo {
		t.Errorf("Repo = %q, want %q", conEtiqueta.Repo, repo)
	}
}

func TestRemoveIfCleanNoBorraUnWorktreeSucioYExplicaPorQue(t *testing.T) {
	raiz := t.TempDir()
	wt := wtConRamaMonta(t, raiz, "prdash-pr-1", "main")
	g := NewGitDirect(raiz)
	ctx := context.Background()

	borrado, motivo, err := g.RemoveIfClean(ctx, wt.Path)
	if err != nil {
		t.Fatalf("RemoveIfClean de un worktree limpio: %v", err)
	}
	if !borrado {
		t.Errorf("un worktree limpio no se borró: %q", motivo)
	}
	if motivo != "" {
		t.Errorf("tras borrar quedó el motivo %q, y no hay motivo de nada", motivo)
	}
	registro := testutil.RunGit(t, wt.Repo, "worktree", "list")
	if strings.Contains(registro, "prdash-pr-1") {
		t.Errorf("tras Remove el worktree sigue en el registro de git: %q", registro)
	}

	sucio := wtConRamaMonta(t, raiz, "prdash-pr-2", "main")
	if err := os.WriteFile(filepath.Join(sucio.Path, "cambiado.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	borrado, motivo, err = g.RemoveIfClean(ctx, sucio.Path)
	if err != nil {
		t.Fatalf("RemoveIfClean de un worktree sucio: %v", err)
	}
	if borrado {
		t.Error("un worktree con cambios sin commitear se borró: se perdieron las ediciones")
	}
	if !exists(t, sucio.Path) {
		t.Error("el worktree sucio desapareció igualmente")
	}
	if strings.TrimSpace(motivo) == "" {
		t.Fatal("no se borró y no hay motivo: el usuario no tiene forma de saber por qué")
	}
	if !strings.Contains(strings.ToLower(motivo), "uncommitted") &&
		!strings.Contains(strings.ToLower(motivo), "changes") {
		t.Logf("el motivo no menciona los cambios: %q", motivo)
	}
}

// Why `diff` is not enough: an untracked file is uncommitted work and diff ignores it.
func TestDirtyCuentaLosFicherosSinTrackear(t *testing.T) {
	raiz := t.TempDir()
	wt := wtConRamaMonta(t, raiz, "prdash-pr-1", "main")
	g := NewGitDirect(raiz)
	ctx := context.Background()

	sucio, err := g.dirty(ctx, wt.Path)
	if err != nil {
		t.Fatal(err)
	}
	if sucio {
		t.Error("un worktree recién creado salió sucio")
	}

	nuevo := filepath.Join(wt.Path, "nuevo.txt")
	if err := os.WriteFile(nuevo, []byte("trabajo sin commitear"), 0o644); err != nil {
		t.Fatal(err)
	}
	sucio, err = g.dirty(ctx, wt.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !sucio {
		t.Error("un fichero sin trackear no cuenta como trabajo: diff lo ignora y el " +
			"borrado lo tiraría")
	}

	if err := os.WriteFile(nuevo, []byte("cambiado otra vez"), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.RunGit(t, wt.Path, "add", "nuevo.txt")
	sucio, err = g.dirty(ctx, wt.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !sucio {
		t.Error("cambios en el índice sin commitear no cuentan como trabajo")
	}

	testutil.RunGit(t, wt.Path, "commit", "-m", "lo que sea")
	sucio, err = g.dirty(ctx, wt.Path)
	if err != nil {
		t.Fatal(err)
	}
	if sucio {
		t.Error("tras commitear sigue salido sucio")
	}
}

// Three refusals, each preventing a different damage.
func TestRemoveSeNiegaATocarLoQueNoEsPropio(t *testing.T) {
	raiz := t.TempDir()
	wt := wtConRamaMonta(t, raiz, "prdash-pr-1", "main")
	g := NewGitDirect(raiz)
	ctx := context.Background()

	ajeno := filepath.Join(t.TempDir(), "prdash-mio")
	testutil.InitRepo(t, ajeno)
	testutil.CommitFile(t, ajeno, "importante.txt", "no me borres", "importante")

	// debeSeguir says what has to happen to the path afterwards, because it is not the same in the three
	//cases: a path that does not exist cannot "stay there".
	carpeta := filepath.Join(raiz, "prdash-carpeta-vacia")
	if err := os.MkdirAll(carpeta, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		nombre     string
		ruta       string
		debeSeguir bool
	}{
		{"fuera de la raiz", ajeno, true},
		{"inexistente", filepath.Join(raiz, "prdash-no-existe"), false},
		{"no es worktree", carpeta, true},
	} {
		err := g.Remove(ctx, c.ruta)
		if err == nil {
			t.Errorf("%s: Remove dio nil, y tiene que negar", c.nombre)
			continue
		}
		if c.debeSeguir && !exists(t, c.ruta) {
			t.Errorf("%s: Remove borró %s, que no es suyo", c.nombre, c.ruta)
		}
	}

	if _, err := os.Stat(filepath.Join(ajeno, "importante.txt")); err != nil {
		t.Errorf("el repo ajeno perdió su contenido: %v", err)
	}

	if err := g.Remove(ctx, wt.Path); err != nil {
		t.Fatalf("Remove del worktree propio: %v", err)
	}
	if strings.Contains(testutil.RunGit(t, wt.Repo, "worktree", "list"), "prdash-pr-1") {
		t.Error("el worktree propio sigue en el registro tras Remove")
	}
}

// inspect exists to avoid creating the worktree just to read it.
func TestInspeccionarTraeElEstadoDelWorktreeSinMontarNada(t *testing.T) {
	raiz := t.TempDir()
	wt := wtConRamaMonta(t, raiz, "prdash-pr-1", "main")
	g := NewGitDirect(raiz)

	visto, ok, err := g.inspect(context.Background(), wt.Path)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if !ok {
		t.Fatal("inspect no vio un worktree que existe")
	}
	if visto.Branch == "" {
		t.Error("inspect no leyó la rama del worktree")
	}
	if visto.Path != wt.Path {
		t.Errorf("Path = %q, want %q", visto.Path, wt.Path)
	}

	carpeta := filepath.Join(raiz, "prdash-carpeta")
	if err := os.MkdirAll(carpeta, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := g.inspect(context.Background(), carpeta); ok || err != nil {
		t.Errorf("una carpeta suelta dio (ok=%v, err=%v), want (false, nil)", ok, err)
	}
	lineas := strings.Split(strings.TrimSpace(
		testutil.RunGit(t, wt.Repo, "worktree", "list")), "\n")
	if len(lineas) != 2 {
		t.Errorf("inspect dejó %d líneas en worktree list, want 2", len(lineas))
	}
}

func exists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	return err == nil
}
