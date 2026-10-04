package worktree

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"prdash/internal/testutil"
)

// Los tres de worktree.go que sobrevivían eran tres negaciones en el camino de
// provisioned, y los tres son la misma pregunta —«¿este path es mío?»— con tres formas
// de equivocarse.
//
// Y los tres estaban SIN TEST, que es lo que significa que un mutante de negación
// sobreviva: no hay ningún caso en el que la respuesta importe lo suficiente como para que
// algo se rompa al cambiarla.

// TestLaEtiquetaDelWorktreeEsLaDelSpec: si el spec trae etiqueta, esa es la del worktree,
// y el nombre del directorio es el último recurso.
//
// Y esto no es cosmético. La etiqueta es lo que se pinta en el header de Herdr y lo que
// `spec.Label` usa como PROPERTY para reconocer un worktree suyo en un workspace ajeno:
// prdash abre un review sobre un tab cuyo nombre dice `prdash/acme#12`, y si la etiqueta
// sale del nombre del directorio sale `acme-pr-12`, que prdash no reconoce y trata como
// hecho suyo para no duplicar.
//
// Con la condición invertida el spec con etiqueta la pierde y la reemplaza por el nombre
// del directorio — que es justo lo que el `else` tenía que hacer — y entonces el primer
// caso, el de la etiqueta puesta, no se distingue del segundo. Por eso el caso que
// importa es el que trae etiqueta Y un directorio con otro nombre: solo ahí se ven las dos.
func TestLaEtiquetaDelWorktreeEsLaDelSpec(t *testing.T) {
	repo := newRepo(t)
	base := t.TempDir()

	// Las ramas existen de verdad: `Create` saca la rama del repo, así que un nombre
	// inventado falla antes de llegar a la etiqueta y el test no probaría nada.
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

	// Y el caso que de verdad no se puede deducir del directorio: un directorio cuyo
	// nombre no dice nada del worktree. Sin etiqueta, el nombre del directorio es lo
	// único que hay, y tiene que salir ese y no una cadena vacía.
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

// TestCleanPartialNoBorraFueraDeLaRaiz: la limpieza de un `worktree add` fallido solo
// toca la raíz propia.
//
// Y es la única defensa que hay, porque el borrado es un `RemoveAll` recursivo sin
// confirmar. El path viene del layout, pero el layout lo deriva del destino que eligió el
// usuario, y un destino equivocado —un directorio de trabajo, una ruta mal pegada— con la
// comprobación mal puesta se lleva por delante un árbol entero.
//
// Y el caso que distingue es el `..`: `filepath.Rel` da una ruta con `..` cuando el path
// está FUERA de la base, y ese es el caso que hay que mirar. Un path dentro sale con una
// ruta relativa sin `..`, y borrar ahí es lo que se quiere.
func TestCleanPartialNoBorraFueraDeLaRaiz(t *testing.T) {
	base := t.TempDir()
	// Un directorio hermano, que es lo que sale cuando la ruta se sale de la base por
	// arriba: `/tmp/x/base` y `/tmp/x/hermano`.
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

	// Fuera de la base: no se toca. Ni el `..` ni el hermano.
	g.cleanPartial(hermano)
	if _, err := os.Stat(marcador); err != nil {
		t.Errorf("la limpieza se llevó %s, que está FUERA de la raíz %q: el borrado es "+
			"recursivo y sin confirmar, así que esta comprobación es la única defensa",
			marcador, base)
	}
	if _, err := os.Stat(hermano); err != nil {
		t.Errorf("la limpieza borró el directorio hermano entero %s", hermano)
	}

	// Dentro de la base: sí se borra, porque es un resto de algo que falló.
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

	// Y el anidado dentro de la base también se borra: `..` no aparece, así que la
	// comprobación lo deja pasar y es lo correcto.
	anidado := filepath.Join(base, "a", "b", "resto")
	if err := os.MkdirAll(anidado, 0o755); err != nil {
		t.Fatal(err)
	}
	g.cleanPartial(anidado)
	if _, err := os.Stat(anidado); err == nil {
		t.Errorf("la limpieza no borró %s, que está dentro de la raíz a dos niveles", anidado)
	}
}

// TestCleanPartialSinRaizNoBorraYConRaizNoTocaLosEnlazados: sin raíz propia no hay
// comparación que hacer, y un worktree ya enlazado no se toca aunque esté dentro.
//
// Las dos mitades de la condición. La primera es que `Base` vacío desactiva la
// comparación de contencion: sin raíz no hay contra qué comparar, y adivinar sería peor que
// no borrar. La segunda es que `isLinkedWorktree` va DESPUÉS, así que un worktree que sí
// llegó a registrarse se respeta aunque esté dentro de la raíz: es un worktree válido y
// borrarlo sería perder el trabajo del usuario.
//
// Y el caso de `isLinkedWorktree` es el que hace de contrapeso: sin él, la limpieza de un
// fallo de `worktree add` que SÍ llegó a registrarse se llevaría el worktree entero.
func TestCleanPartialSinRaizNoBorraYConRaizNoTocaLosEnlazados(t *testing.T) {
	base := t.TempDir()
	g := NewGitDirect(base)

	// Un worktree ENLAZADO dentro de la raíz: `.git` es un FICHERO, no un directorio.
	// Eso es lo que distingue un worktree de un repo clonado, y es lo que el `.git` de
	// un clon —un directorio— no es.
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

	// Y un repo clonado dentro de la raíz —`.git` es un DIRECTORIO— no está enlazado, así
	// que la limpieza sí lo trata como resto. Es lo que distingue una cosa de la otra.
	clonado := filepath.Join(base, "clonado")
	if err := os.MkdirAll(filepath.Join(clonado, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	g.cleanPartial(clonado)
	if _, err := os.Stat(clonado); err == nil {
		t.Errorf("la limpieza no borró %s: un `.git` que es un directorio no es un "+
			"worktree enlazado, y un clon no es resto de nada", clonado)
	}

	// Y SIN RAÍZ PROPIA la comparación de contención NO se hace, y el borrado sí. Lo
	// dice el código y hay que decirlo aquí, porque es lo contrario de lo que parece
	// por el nombre de la función: `Base` vacío no es un "no compares", es un "no hay
	// contra qué comparar, así que borra".
	//
	// La primera versión de este test afirmaba que sin raíz no se borraba nada, y
	// falló. El código la tiene por buena y no es un descuido mío: `cleanPartial` solo
	// se llama desde el fallo de `worktree add`, con un path que prdash acaba de elegir,
	// y sin raíz no hay un "fuera de casa" que evitar. Lo que protects de verdad —que no
	// se borre un worktree que llegó a registrarse— es `isLinkedWorktree`, que va
	// DESPUÉS y no depende de la raíz.
	//
	// Así que lo que se afirma aquí es esa parte: sin raíz, un worktree enlazado
	// tampoco se toca.
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

	// Y lo que sí borra sin raíz: un directorio corriente. Se afirma para que el cambio
	// de comportamiento sea visible si alguien lo cambia, no para aprobarlo.
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
