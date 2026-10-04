package worktree

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Audit decides which worktrees are prdash's and which are alive.

// linkedGitDir's four negatives.
func TestElFicheroGitDeUnWorktreeDiceDondeEstaElOrigen(t *testing.T) {
	base := t.TempDir()

	abs := filepath.Join(base, "abs")
	mkdir(t, abs)
	escribirGit(t, abs, "gitdir: "+filepath.Join(base, "repo", ".git", "worktrees", "wt"))
	if got := linkedGitDir(abs); got != filepath.Join(base, "repo", ".git", "worktrees", "wt") {
		t.Errorf("una ruta absoluta dio %q", got)
	}

	rel := filepath.Join(base, "rel")
	mkdir(t, rel)
	escribirGit(t, rel, "gitdir: ../repo/.git/worktrees/wt")
	want := filepath.Join(base, "repo", ".git", "worktrees", "wt")
	if got := linkedGitDir(rel); got != want {
		t.Errorf("una ruta relativa dio %q, want %q (relativa al worktree)", got, want)
	}

	conSalto := filepath.Join(base, "con-salto")
	mkdir(t, conSalto)
	escribirGit(t, conSalto, "gitdir:   "+filepath.Join(base, "otro")+"  \n")
	if got := linkedGitDir(conSalto); got != filepath.Join(base, "otro") {
		t.Errorf("con espacios y salto dio %q", got)
	}

	casos := []struct {
		nombre  string
		valor   string
		ausente bool
	}{
		{"sin prefijo", "/otra/cosa\n", false},
		{"gitdir vacio", "gitdir:\n", false},
		{"gitdir con espacios", "gitdir:    \n", false},
		{"otro prefijo", "worktree: /x\n", false},
		{"fichero ausente", "", true},
	}
	for _, c := range casos {
		dir := filepath.Join(base, "neg-"+strings.ReplaceAll(c.nombre, " ", "-"))
		mkdir(t, dir)
		if !c.ausente {
			escribirGit(t, dir, c.valor)
		}
		if got := linkedGitDir(dir); got != "" {
			t.Errorf("%s: dio %q, want cadena vacia", c.nombre, got)
		}
	}

	repo := filepath.Join(base, "repo-normal")
	mkdir(t, filepath.Join(repo, ".git"))
	if got := linkedGitDir(repo); got != "" {
		t.Errorf("un repo normal dio gitdir %q: un .git que es directorio no es un enlace", got)
	}
}

func TestUnEnlaceRotoEsUnHuerfanoYUnoSanoNo(t *testing.T) {
	base := t.TempDir()
	origen := filepath.Join(base, "repo", ".git", "worktrees")
	mkdir(t, origen)

	sano := filepath.Join(base, "sano")
	mkdir(t, sano)
	escribirFichero(t, filepath.Join(origen, "wt-sano"), "")
	escribirGit(t, sano, "gitdir: "+filepath.Join(origen, "wt-sano"))
	if !sourceReachable(sano) {
		t.Error("un enlace sano dio false")
	}

	roto := filepath.Join(base, "roto")
	mkdir(t, roto)
	escribirGit(t, roto, "gitdir: "+filepath.Join(origen, "wt-que-no-existe"))
	if sourceReachable(roto) {
		t.Error("un enlace roto dio true: el worktree se reportaria vivo y ocuparia disco para siempre")
	}

	vacio := filepath.Join(base, "vacio")
	mkdir(t, vacio)
	if sourceReachable(vacio) {
		t.Error("un directorio sin .git dio true")
	}
	invalido := filepath.Join(base, "invalido")
	mkdir(t, invalido)
	escribirGit(t, invalido, "esto no es un enlace")
	if sourceReachable(invalido) {
		t.Error("un .git sin prefijo dio true")
	}
}

// The whole walk.
func TestAuditSoloDevuelveLoQueEsDePrdashYMarcaLoRoto(t *testing.T) {
	base := t.TempDir()
	raiz := filepath.Join(base, "worktrees")
	mkdir(t, raiz)

	origen := filepath.Join(base, "repo", ".git", "worktrees")
	mkdir(t, origen)
	escribirFichero(t, filepath.Join(origen, "wt-1"), "")
	escribirFichero(t, filepath.Join(origen, "wt-ajeno"), "")

	repoUsuario := filepath.Join(base, "repo-del-usuario")
	mkdir(t, filepath.Join(repoUsuario, ".git"))

	vivo := filepath.Join(raiz, "prdash-pr-1")
	mkdir(t, vivo)
	escribirGit(t, vivo, "gitdir: "+filepath.Join(origen, "wt-1"))

	roto := filepath.Join(raiz, "prdash-pr-2")
	mkdir(t, roto)
	escribirGit(t, roto, "gitdir: "+filepath.Join(origen, "wt-que-borre"))

	ajeno := filepath.Join(raiz, "vroom-pr-9")
	mkdir(t, ajeno)
	escribirGit(t, ajeno, "gitdir: "+filepath.Join(origen, "wt-ajeno"))

	mkdir(t, filepath.Join(raiz, "una-carpeta"))

	entradas := NewGitDirect(raiz).Audit(context.Background())

	vistos := map[string]Entry{}
	for _, e := range entradas {
		vistos[filepath.Base(e.Path)] = e
	}

	if len(entradas) != 2 {
		t.Errorf("Audit devolvio %d entradas, want 2 (solo las de prdash): %+v", len(entradas), entradas)
	}
	if _, ok := vistos["repo-del-usuario"]; ok {
		t.Error("un repo normal del usuario salio en el listado: prdash worktrees remove podria borrarlo")
	}
	if _, ok := vistos["vroom-pr-9"]; ok {
		t.Error("un worktree de otra herramienta salio: el ownership no filtro")
	}

	v, ok := vistos["prdash-pr-1"]
	if !ok {
		t.Fatal("el worktree vivo no salio")
	}
	if v.Orphan {
		t.Errorf("el worktree vivo salio huerfano: %+v", v)
	}
	r, ok := vistos["prdash-pr-2"]
	if !ok {
		t.Fatal("el worktree huerfano no salio: deberia salir para que se pueda limpiar")
	}
	if !r.Orphan {
		t.Errorf("el worktree con el enlace roto no salio huerfano: %+v", r)
	}
	if strings.TrimSpace(r.Reason) == "" {
		t.Error("el huerfano salio sin motivo: un aviso sin motivo no dice que arreglar")
	}
	if !strings.Contains(r.Reason, "no longer reachable") {
		t.Errorf("el motivo %q no dice que el repo de origen ya no esta", r.Reason)
	}

	// The LABEL wins over the directory name, which is what makes the listing say
	// "prdash-pr-1" and not the whole path.
	if v.Label != "prdash-pr-1" {
		t.Errorf("Label = %q, want prdash-pr-1", v.Label)
	}

	// Sorted by path, which is what makes the output stable between runs.
	for i := 1; i < len(entradas); i++ {
		if entradas[i-1].Path > entradas[i].Path {
			t.Errorf("el listado no viene ordenado por ruta: %q antes que %q",
				entradas[i-1].Path, entradas[i].Path)
		}
	}
}

// An empty root is not an error.
func TestAuditSinRaizNoDevuelveNada(t *testing.T) {
	if got := (&GitDirect{}).Audit(context.Background()); got != nil {
		t.Errorf("sin Base devolvio %+v, want nil", got)
	}
	inexistente := filepath.Join(t.TempDir(), "no-existe")
	if got := NewGitDirect(inexistente).Audit(context.Background()); len(got) != 0 {
		t.Errorf("una raiz inexistente devolvio %+v", got)
	}
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func escribirFichero(t *testing.T, path, contenido string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contenido), 0o644); err != nil {
		t.Fatal(err)
	}
}

func escribirGit(t *testing.T, path, contenido string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(path, ".git"), []byte(contenido), 0o644); err != nil {
		t.Fatal(err)
	}
}
