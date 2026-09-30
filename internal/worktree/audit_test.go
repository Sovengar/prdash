package worktree

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"prdash/internal/testutil"
)

// TestOwnedReconocesoloLaMarcaPrdash comprueba que el ownership se resuelve por
// el prefijo `prdash-` en la etiqueta o en el nombre de la ruta.
func TestOwnedReconocesoloLaMarcaPrdash(t *testing.T) {
	cases := []struct {
		label string
		path  string
		want  bool
	}{
		{"prdash-pr-3", "/x/y/prdash-pr-3", true},
		{"", "/x/y/prdash-pr-3", true},
		{"prdash-pr-3", "/x/y/otra-cosa", true},
		{"otra-herramienta", "/x/y/otra-herramienta", false},
		{"", "/x/y/otra-herramienta", false},
		{"repodash", "/x/y/repodash", false},
	}
	for _, c := range cases {
		if got := Owned(c.label, c.path); got != c.want {
			t.Errorf("Owned(%q, %q) = %v, quiero %v", c.label, c.path, got, c.want)
		}
	}
}

// TestAuditListsOnlyOwnedWorktrees comprueba que el listado ignora los
// worktrees ajenos que convivan bajo la misma raíz.
func TestAuditListsOnlyOwnedWorktrees(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "propia")
	testutil.RunGit(t, repo, "branch", "ajena")

	base := t.TempDir()
	owned := filepath.Join(base, "prdash-pr-1")
	foreign := filepath.Join(base, "otra-herramienta")
	if _, err := NewGitDirect(base).Create(context.Background(), Spec{Repo: repo, Branch: "propia", Path: owned, Label: "prdash-pr-1"}); err != nil {
		t.Fatalf("preparar worktree propio: %v", err)
	}
	testutil.RunGit(t, repo, "worktree", "add", "--quiet", foreign, "ajena")

	entries := NewGitDirect(base).Audit(context.Background())
	if len(entries) != 1 {
		t.Fatalf("Audit = %+v, quiero solo el worktree propio", entries)
	}
	if entries[0].Path != owned || entries[0].Orphan {
		t.Fatalf("entrada = %+v", entries[0])
	}
	if entries[0].Branch != "propia" {
		t.Fatalf("rama = %q", entries[0].Branch)
	}
}

// TestAuditOrdenaPorRuta: Audit garantiza el orden por ruta (está en su
// contrato) y de ese orden depende la limpieza por lotes, que borra una entrada
// detrás de otra.
//
// Sin afirmarlo, invertir la comparación del sort no lo detecta ningún test: el
// recorrido de directorios de filepath.WalkDir ya sale en orden lexicográfico, de
// modo que el sort parece un no-op y el listado sale igual. Solo se nota cuando se
// mete trabajo en la raíz que el walk NO puede ordenar por nosotros.
func TestAuditOrdenaPorRuta(t *testing.T) {
	repo := newRepo(t)
	base := t.TempDir()

	// Se crean en orden REVERSOS a como deben salir, y con números de dos cifras
	// para que un orden lexic-inglés ("10" < "2") no losconfunda.
	var quiere []string
	for _, n := range []string{"prdash-pr-10", "prdash-pr-2", "prdash-pr-1"} {
		testutil.RunGit(t, repo, "branch", n)
		path := filepath.Join(base, n)
		if _, err := NewGitDirect(base).Create(context.Background(), Spec{Repo: repo, Branch: n, Path: path, Label: n}); err != nil {
			t.Fatalf("crear %s: %v", n, err)
		}
		quiere = append(quiere, path)
	}
	// El orden que se espera es el LEXICOGRÁFICO de las rutas, no el de creación
	// ni el numérico: "prdash-pr-1" < "prdash-pr-10" < "prdash-pr-2". Con dos
	// cifras el orden numérico y el de cadena discrepan, que es justo lo que
	// hace que la prueba tenga contenido: un sort por número o por orden de
	// creación se confundiría con el de cadena solo en estos nombres.
	slices.Sort(quiere)

	entries := NewGitDirect(base).Audit(context.Background())
	if len(entries) != len(quiere) {
		t.Fatalf("Audit = %d entradas, want %d", len(entries), len(quiere))
	}
	for i, e := range entries {
		if e.Path != quiere[i] {
			var got []string
			for _, x := range entries {
				got = append(got, filepath.Base(x.Path))
			}
			t.Errorf("Audit no viene ordenado por ruta: %v", got)
			break
		}
	}
}

// TestAuditFlagsOrphanWhenSourceGone marca huérfano un worktree cuyo repo de
// origen desapareció: la limpieza debe poder reportarlo.
func TestAuditFlagsOrphanWhenSourceGone(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature")

	base := t.TempDir()
	dest := filepath.Join(base, "prdash-pr-1")
	if _, err := NewGitDirect(base).Create(context.Background(), Spec{Repo: repo, Branch: "feature", Path: dest, Label: "prdash-pr-1"}); err != nil {
		t.Fatalf("preparar worktree: %v", err)
	}
	if err := os.RemoveAll(repo); err != nil {
		t.Fatal(err)
	}

	entries := NewGitDirect(base).Audit(context.Background())
	if len(entries) != 1 || !entries[0].Orphan || entries[0].Reason == "" {
		t.Fatalf("esperaba un huérfano con motivo, got %+v", entries)
	}
}

// TestListExcludesForeignWorktrees fija que el listado del puerto respeta el
// ownership: los worktrees ajenos no se exponen.
func TestListExcludesForeignWorktrees(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "propia")
	testutil.RunGit(t, repo, "branch", "ajena")

	base := t.TempDir()
	owned := filepath.Join(base, "prdash-pr-1")
	foreign := filepath.Join(base, "otra-herramienta")
	if _, err := NewGitDirect(base).Create(context.Background(), Spec{Repo: repo, Branch: "propia", Path: owned, Label: "prdash-pr-1"}); err != nil {
		t.Fatalf("preparar worktree propio: %v", err)
	}
	testutil.RunGit(t, repo, "worktree", "add", "--quiet", foreign, "ajena")

	list := NewGitDirect(base).List(context.Background())
	if len(list) != 1 || list[0].Path != owned {
		t.Fatalf("List = %+v, quiero solo el worktree propio", list)
	}
}

// TestRemoveOrphanDeletesCheckout comprueba que un worktree huérfano (repo de
// origen desaparecido) se puede borrar por petición explícita.
func TestRemoveOrphanDeletesCheckout(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature")

	base := t.TempDir()
	dest := filepath.Join(base, "prdash-pr-1")
	g := NewGitDirect(base)
	if _, err := g.Create(context.Background(), Spec{Repo: repo, Branch: "feature", Path: dest, Label: "prdash-pr-1"}); err != nil {
		t.Fatalf("preparar worktree: %v", err)
	}
	if err := os.RemoveAll(repo); err != nil {
		t.Fatal(err)
	}

	if err := g.Remove(context.Background(), dest); err != nil {
		t.Fatalf("Remove de huérfano: %v", err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("el checkout huérfano debería haberse borrado: %v", err)
	}
}

// TestRemoveRefusesForeignWorktree comprueba que un worktree sin ownership
// prdash no se borra aunque su repo exista.
func TestRemoveRefusesForeignWorktree(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature")

	base := t.TempDir()
	foreign := filepath.Join(base, "otra-herramienta")
	testutil.RunGit(t, repo, "worktree", "add", "--quiet", foreign, "feature")

	g := NewGitDirect(base)
	if err := g.Remove(context.Background(), foreign); err == nil {
		t.Fatal("no debería borrar un worktree ajeno")
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Fatalf("el worktree ajeno no debería tocarse: %v", err)
	}
}

// TestRemoveRefusesPathOutsideBase comprueba que ni un worktree con nombre
// prdash se borra si queda fuera de la raíz gestionada.
func TestRemoveRefusesPathOutsideBase(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature")

	outside := filepath.Join(t.TempDir(), "prdash-pr-1")
	testutil.RunGit(t, repo, "worktree", "add", "--quiet", outside, "feature")

	g := NewGitDirect(t.TempDir()) // raíz gestionada distinta
	if err := g.Remove(context.Background(), outside); err == nil {
		t.Fatal("no debería borrar fuera de la raíz gestionada")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("el worktree fuera de la raíz no debería tocarse: %v", err)
	}
}

// TestAuditSkipsNonWorktreeDirs ignora directorios normales bajo la raíz.
func TestAuditSkipsNonWorktreeDirs(t *testing.T) {
	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, "prdash-pr-9"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := NewGitDirect(base).Audit(context.Background()); len(got) != 0 {
		t.Fatalf("un directorio sin worktree no debería listarse: %+v", got)
	}
}

// TestRemoveOrphanWithoutGitDirDeletesCheckout cubre el huérfano cuyo `.git`
// existe pero no declara un gitdir (corrupto o truncado): Audit lo marca huérfano
// y no hay repo que resolver, así que borrar el checkout es lo único que queda.
func TestRemoveOrphanWithoutGitDirDeletesCheckout(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature")

	base := t.TempDir()
	dest := filepath.Join(base, "prdash-pr-1")
	g := NewGitDirect(base)
	if _, err := g.Create(context.Background(), Spec{Repo: repo, Branch: "feature", Path: dest, Label: "prdash-pr-1"}); err != nil {
		t.Fatalf("preparar worktree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dest, ".git"), []byte("garbage\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := g.Remove(context.Background(), dest); err != nil {
		t.Fatalf("Remove de huérfano sin gitdir: %v", err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("el checkout debería haberse borrado: %v", err)
	}
}

// TestRemoveRefusesNonLinkedDir blinda el guarda del caso anterior: un directorio
// con nombre prdash que no es un worktree enlazado no se borra.
func TestRemoveRefusesNonLinkedDir(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "prdash-not-a-worktree")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := NewGitDirect(base).Remove(context.Background(), dir); err == nil {
		t.Fatal("un directorio que no es worktree enlazado no debería borrarse")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("el directorio no debería tocarse: %v", err)
	}
}
