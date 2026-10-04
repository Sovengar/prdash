package worktree

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"prdash/internal/testutil"
)

// The three survivors in worktree.go were three negatives in the provisioning path.

func TestLaEtiquetaDelWorktreeEsLaDelSpec(t *testing.T) {
	repo := newRepo(t)
	base := t.TempDir()

	// Real branches: Create takes them from the repo, so an invented name fails before
	// reaching the label and the test would prove nothing.
	for _, c := range []string{"con-etiqueta", "sin-etiqueta", "etiqueta-igual-al-dir",
		"directorio-anidado", "w-temp-8837"} {
		testutil.RunGit(t, repo, "branch", c)
	}

	casos := []struct {
		nombre    string
		dir       string
		label     string
		wantLabel string
		nota      string
	}{
		{"con-etiqueta", "prdash-acme-12", "prdash/acme#12", "prdash/acme#12",
			"la del spec manda: es la property con la que prdash reconoce el worktree"},
		{"sin-etiqueta", "prdash-acme-12", "", "prdash-acme-12",
			"sin etiqueta el nombre del directorio es el último recurso"},
		{"etiqueta-igual-al-dir", "prdash-acme-12", "prdash-acme-12", "prdash-acme-12",
			"coinciden, y da igual cuál de los dos gane"},
		{"directorio-anidado", "sub/dir/prdash-acme-12", "prdash/acme#12", "prdash/acme#12",
			"el directorio puede estar anidado; la etiqueta no se deduce de él"},
	}

	for _, c := range casos {
		dest := filepath.Join(base, c.nombre, c.dir)
		wt, err := NewGitDirect(base).Create(context.Background(),
			Spec{Repo: repo, Branch: c.nombre, Path: dest, Label: c.label})
		if err != nil {
			t.Fatalf("caso %q: Create: %v", c.nombre, err)
		}
		if wt.Label != c.wantLabel {
			t.Errorf("caso %q: la etiqueta salió %q, want %q. %s",
				c.nombre, wt.Label, c.wantLabel, c.nota)
		}
	}

	sinPista := filepath.Join(base, "w-temp-8837")
	wt, err := NewGitDirect(base).Create(context.Background(),
		Spec{Repo: repo, Branch: "w-temp-8837", Path: sinPista})
	if err != nil {
		t.Fatalf("Create sin pista: %v", err)
	}
	if wt.Label != "w-temp-8837" {
		t.Errorf("sin etiqueta y con un directorio sin pista salió %q, want el nombre "+
			"del directorio. Una etiqueta vacía en el header de Herdr deja el tab sin "+
			"nombre y no se sabe qué se está revisando", wt.Label)
	}
}

func TestCleanPartialNoBorraFueraDeLaRaiz(t *testing.T) {
	base := t.TempDir()
	hermano := filepath.Join(filepath.Dir(base), filepath.Base(base)+"-hermano")
	if err := os.MkdirAll(hermano, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(hermano) })
	marcador := filepath.Join(hermano, "NO-TOCAR")
	if err := os.WriteFile(marcador, []byte("importante"), 0o644); err != nil {
		t.Fatal(err)
	}

	g := NewGitDirect(base)

	g.cleanPartial(hermano)
	if _, err := os.Stat(marcador); err != nil {
		t.Errorf("la limpieza se llevó %s, que está FUERA de la raíz %q: el borrado es "+
			"recursivo y sin confirmar, así que esta comprobación es la única defensa",
			marcador, base)
	}
	if _, err := os.Stat(hermano); err != nil {
		t.Errorf("la limpieza borró el directorio hermano entero %s", hermano)
	}

	dentro := filepath.Join(base, "resto-fallido")
	if err := os.MkdirAll(dentro, 0o755); err != nil {
		t.Fatal(err)
	}
	g.cleanPartial(dentro)
	if _, err := os.Stat(dentro); err == nil {
		t.Errorf("la limpieza no borró %s, que está dentro de la raíz: sin esto los "+
			"restos de un worktree add fallido se quedan ahí y el siguiente intento "+
			"falla otra vez", dentro)
	}

	anidado := filepath.Join(base, "a", "b", "resto")
	if err := os.MkdirAll(anidado, 0o755); err != nil {
		t.Fatal(err)
	}
	g.cleanPartial(anidado)
	if _, err := os.Stat(anidado); err == nil {
		t.Errorf("la limpieza no borró %s, que está dentro de la raíz a dos niveles", anidado)
	}
}

func TestCleanPartialSinRaizNoBorraYConRaizNoTocaLosEnlazados(t *testing.T) {
	base := t.TempDir()
	g := NewGitDirect(base)

	enlazado := filepath.Join(base, "enlazado")
	if err := os.MkdirAll(enlazado, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(enlazado, ".git"),
		[]byte("gitdir: /repo/.git/worktrees/enlazado\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	g.cleanPartial(enlazado)
	if _, err := os.Stat(enlazado); err != nil {
		t.Errorf("la limpieza borró %s, que es un worktree ENLAZADO: llegó a "+
			"registrarse, así que es trabajo del usuario y borrarlo no se deshace",
			enlazado)
	}

	clonado := filepath.Join(base, "clonado")
	if err := os.MkdirAll(filepath.Join(clonado, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	g.cleanPartial(clonado)
	if _, err := os.Stat(clonado); err == nil {
		t.Errorf("la limpieza no borró %s: un `.git` que es un directorio no es un "+
			"worktree enlazado, y un clon no es resto de nada", clonado)
	}

	sinRaiz := NewGitDirect("")
	libres := t.TempDir()

	enlazadoSinRaiz := filepath.Join(libres, "enlazado")
	if err := os.MkdirAll(enlazadoSinRaiz, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(enlazadoSinRaiz, ".git"),
		[]byte("gitdir: /repo/.git/worktrees/x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sinRaiz.cleanPartial(enlazadoSinRaiz)
	if _, err := os.Stat(enlazadoSinRaiz); err != nil {
		t.Errorf("sin raíz propia la limpieza borró %s, que es un worktree enlazado: "+
			"esa es la guarda que no depende de la raíz, y es la que protects el trabajo "+
			"del usuario", enlazadoSinRaiz)
	}

	plano := filepath.Join(libres, "plano")
	if err := os.MkdirAll(plano, 0o755); err != nil {
		t.Fatal(err)
	}
	sinRaiz.cleanPartial(plano)
	if _, err := os.Stat(plano); err == nil {
		t.Errorf("sin raíz propia la limpieza NO borró %s. Hoy sí lo borra, y este "+
			"assert está para que se note si algún día deja de hacerlo", plano)
	}
}
