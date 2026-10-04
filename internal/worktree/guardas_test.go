package worktree

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/testutil"
)

// Estos son los fallos de `worktree` contra git y el disco de verdad. Y la mayoría no se pueden
// provocar con un doble de `gitcmd.Runner` porque no son un error de git: son un repo que no
// está, una rama que no existe, un `.git` que es un directorio en vez de un fichero, y un
// destino que ya hay.
//
// Y la razón por la que merece la pena probarlos uno a uno es que cada uno tiene una
// consecuencia DISTINTA en el árbol de ficheros, y solo una de ellas es la que el mensaje de
// error sugiere:
//
//   - Un spec incompleto no toca nada.
//   - Un `.git` que es un directorio no se toca, porque el directorio puede ser un repo entero.
//   - Una rama que no existe deja un residuo que `cleanPartial` tiene que quitar.
//   - Un destino occupied no se pisa.
//
// Y el residuo es el que más cuesta ver: `git worktree add` falla a mitad y deja el directorio
// creado. Si el código no lo limpia, el siguiente intento encuentra un directorio donde quiere
// poner el worktree y falla con "already exists", que no dice nada del primer fallo.

// repoConRama deja un repo con `main` y devuelve su ruta.
func repoConRama(t *testing.T, extra ...string) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "repo")
	testutil.InitRepo(t, repo)
	testutil.CommitFile(t, repo, "base.txt", "base", "base")
	for _, r := range extra {
		testutil.RunGit(t, repo, "branch", r)
	}
	return repo
}

// TestUnSpecIncompletoNoSeProcesaNiSeTocaNada: la primera guarda de `Create`.
//
// Y es la guarda mas nfia del fichero porque `Spec` tiene tres campos obligatorios y los tres
// se rellenan desde un ítem del forge más una ruta derivada. Un ítem sin rama de origen
// —un MR de GitLab cuyo `source_branch` vino vacío— produce un spec con la rama vacía, y sin
// esta guarda el `git worktree add` correría con un argumento vacío que git interpretaría como
// la rama por defecto.
func TestUnSpecIncompletoNoSeProcesaNiSeTocaNada(t *testing.T) {
	repo := repoConRama(t)
	raiz := t.TempDir()
	g := NewGitDirect(raiz)

	for _, c := range []struct {
		nombre string
		spec   Spec
	}{
		{"sin repo", Spec{Branch: "main", Path: filepath.Join(raiz, "wt")}},
		{"sin rama", Spec{Repo: repo, Path: filepath.Join(raiz, "wt")}},
		{"sin destino", Spec{Repo: repo, Branch: "main"}},
		{"todo vacío", Spec{}},
	} {
		_, err := g.Create(context.Background(), c.spec)
		if err == nil {
			t.Errorf("%s: Create dio nil", c.nombre)
			continue
		}
		// Y el mensaje dice qué falta, no "incomplete spec": quien lo lee necesita saber si
		// montar el review de otro ítem lo arregla.
		if !strings.Contains(err.Error(), "incomplete spec") {
			t.Errorf("%s: el error %q no dice que el spec está incompleto", c.nombre, err)
		}
	}

	// Y nada se creó: un spec incompleto no deja ni un directorio.
	entradas, err := os.ReadDir(raiz)
	if err != nil {
		t.Fatal(err)
	}
	if len(entradas) != 0 {
		t.Errorf("un spec incompleto dejó %d entradas en la raíz: %v", len(entradas), entradas)
	}
}

// TestUnaRamaQueNoExisteFallaYNoDejaElDirectorioResiduo: la limpieza de `cleanPartial`.
//
// Y el caso es real: el forge dice que la rama del ítem es `feat/x` y para cuando se monta el
// review esa rama ya no existe —alguien la borró, o el PR se rebasó y la rama vieja se cerró—.
// `git worktree add` crea el directorio y luego falla al no encontrar la rama.
//
// Y lo que hay que comprobar es que NO queda el directorio. Sin la limpieza, el siguiente
// intento —o el siguiente ítem en la misma raíz— falla con "already exists" y el mensaje no
// señala el primer fallo, que es el que importa. Un `prdash worktrees remove` posterior tampoco
// lo quitaría: no está en el registro de git, así que no es un worktree a los ojos de `Owned`.
func TestUnaRamaQueNoExisteFallaYNoDejaElDirectorioResiduo(t *testing.T) {
	repo := repoConRama(t)
	raiz := t.TempDir()
	g := NewGitDirect(raiz)
	destino := filepath.Join(raiz, "prdash-pr-7")

	_, err := g.Create(context.Background(), Spec{
		Repo: repo, Branch: "feat/no-existe", Path: destino, Label: "prdash-pr-7",
	})
	if err == nil {
		t.Fatal("crear un worktree de una rama que no existe dio nil")
	}
	if !strings.Contains(err.Error(), "worktree") {
		t.Errorf("el error %q no dice que falla el worktree", err)
	}

	// Y el residuo no está. Ni el directorio entero ni una entrada dentro.
	if _, err := os.Stat(destino); !os.IsNotExist(err) {
		t.Errorf("quedó el directorio %s tras un add fallido: el siguiente intento falla con "+
			"'already exists' y el mensaje no señala el primer fallo", destino)
	}
	// Y el registro de git tampoco lo lista, que es lo que hace que un `Remove` posterior
	// no lo encuentre: no hay ni entrada ni directorio, y eso es el estado limpio.
	lineas := strings.Split(strings.TrimSpace(
		testutil.RunGit(t, repo, "worktree", "list")), "\n")
	for _, l := range lineas {
		if strings.Contains(l, "prdash-pr-7") {
			t.Errorf("el worktree fallido quedó en el registro de git: %q", l)
		}
	}
	// Y la raíz sigue utilizable: un fallo no la deja envenenada.
	//
	// Y con una rama NUEVA y no con `main`, porque `main` ya la tiene el repo principal y
	// git no deja dos worktrees en la misma rama. Mi primera versión usó `main` y el test
	// falló con un error de git que no era el que se quería medir.
	testutil.RunGit(t, repo, "branch", "otra")
	ok, err := g.Create(context.Background(), Spec{
		Repo: repo, Branch: "otra", Path: filepath.Join(raiz, "prdash-pr-8"),
		Label: "prdash-pr-8",
	})
	if err != nil {
		t.Fatalf("la raíz no sirve después de un fallo: %v", err)
	}
	if !Exists(ok.Path) {
		t.Error("el segundo worktree no existe después de un fallo del primero")
	}
}

// TestUnDestinoQueYaEsUnRepoNoSePisaNiSeLeeComoWorktree: `inspect` con un `.git` directorio.
//
// Y esta es la guarda que más daño hace si falta, porque `.git` como DIRECTORIO es lo que tiene
// el repo principal y un clon normal. Un worktree ENLAZADO tiene `.git` como fichero con una
// línea `gitdir: …`.
//
// Y el daño de no distinguirlo: `Remove` borraría el checkout de un repo que el usuario克隆ó a
// mano, y `Create` reutilizaría ese repo como si fuera un worktree del review. Por eso `inspect`
// devuelve un ERROR, no un "no es un worktree": quien llama tiene que enterarse.
func TestUnDestinoQueYaEsUnRepoNoSePisaNiSeLeeComoWorktree(t *testing.T) {
	raiz := t.TempDir()
	// Un clon normal en el destino: `.git` es un directorio.
	repo := filepath.Join(raiz, "prdash-pr-7")
	testutil.InitRepo(t, repo)
	testutil.CommitFile(t, repo, "trabajo.txt", "importante", "importante")

	g := NewGitDirect(raiz)
	ctx := context.Background()

	// `inspect` dice que no es un worktree enlazado, y lo dice con ERROR.
	visto, ok, err := g.inspect(ctx, repo)
	if err == nil {
		t.Error("inspect de un repo normal dio nil: alguien podría tomarlo por un worktree")
	}
	if ok {
		t.Error("inspect dijo que un repo normal es un worktree enlazado")
	}
	if visto.Path != "" {
		t.Errorf("inspect devolvió un worktree: %+v", visto)
	}
	if !strings.Contains(err.Error(), "not a linked worktree") {
		t.Errorf("el error %q no dice que no es un worktree enlazado", err)
	}

	// Y `Create` no lo pisa: falla y el repo del usuario sigue con su contenido.
	if _, err := g.Create(ctx, Spec{
		Repo: filepath.Join(t.TempDir(), "otro"), Branch: "main",
		Path: repo, Label: "prdash-pr-7",
	}); err == nil {
		t.Error("Create sobre un repo existente dio nil")
	}
	if _, err := os.Stat(filepath.Join(repo, "trabajo.txt")); err != nil {
		t.Errorf("el repo del usuario perdió su contenido: %v", err)
	}

	// Y `Remove` tampoco lo toca, que es la consecuencia de la que hablabamos: un `prdash
	// worktrees remove` no puede borrar un clon normal aunque el directorio se llame
	// `prdash-algo`.
	if err := g.Remove(ctx, repo); err == nil {
		t.Error("Remove de un repo normal dio nil")
	}
	if _, err := os.Stat(filepath.Join(repo, "trabajo.txt")); err != nil {
		t.Errorf("Remove se llevó el repo del usuario: %v", err)
	}
}

// TestAuditNoEntraEnLosDirectoriosGit: el salto del recorrido.
//
// Y no es una optimización. `Audit` camina la raíz gestionada buscando directorios `prdash-*`
// que sean worktrees enlazados, y si bajara dentro de un `.git` encontraría los worktrees que
// git guarda ahí —`.git/worktrees/<nombre>/`— y los listaría como si fueran worktrees del
// usuario.
//
// Y el daño es de borrado: `prdash worktrees remove --orphans` audita y borra, así que un `.git`
// recorrido convertiría el registro interno de git en objetivo de borrado.
func TestAuditNoEntraEnLosDirectoriosGit(t *testing.T) {
	raiz := t.TempDir()
	repo := repoConRama(t, "feat/x")
	g := NewGitDirect(raiz)

	// Un worktree de verdad, para que el recorrido tenga algo que encontrar y el salto se
	// note: sin él, un `Audit` que no bajara en ningún sitio daría la misma respuesta.
	if _, err := g.Create(context.Background(), Spec{
		Repo: repo, Branch: "feat/x", Path: filepath.Join(raiz, "prdash-pr-7"),
		Label: "prdash-pr-7",
	}); err != nil {
		t.Fatal(err)
	}

	entradas := g.Audit(context.Background())
	if len(entradas) != 1 {
		t.Fatalf("Audit devolvió %d entradas, want 1: %+v", len(entradas), entradas)
	}
	if entradas[0].Path != filepath.Join(raiz, "prdash-pr-7") {
		t.Errorf("Audit devolvió %q", entradas[0].Path)
	}

	// Y NINGUNA de las entradas es del registro interno de git. Con el salto puesto, el
	// directorio `.git/worktrees` —que contiene una entrada por worktree— no se lista, y
	// `prdash worktrees remove --orphans` no lo puede tocar.
	for _, e := range entradas {
		if strings.Contains(e.Path, string(filepath.Separator)+".git"+string(filepath.Separator)) {
			t.Errorf("Audit listó algo de dentro de un .git: %q", e.Path)
		}
		if !strings.HasPrefix(filepath.Base(e.Path), LabelPrefix) {
			t.Errorf("Audit listó %q, que no lleva el prefijo de ownership", e.Path)
		}
	}
}

// TestRemoveIfCleanPropagaElFalloDeQuitarlo: la última vuelta del candado.
//
// Y `RemoveIfClean` es "borra solo si está limpio", y el candado tiene dos etapas: comprobar
// que está limpio y quitarlo. La segunda puede fallar aunque la primera pasara —el registro de
// git corrupto, el disco lleno— y ese fallo tiene que PROPAGARSE, no comerse.
//
// Y el motivo de que importe: `RemoveIfClean` la usa la limpieza al cerrar la TUI. Un fallo
// tragado ahí sería un worktree que sigue occupying disco sin que nadie lo sepa, y el proximo
// `Audit` lo listaría como si nada.
func TestRemoveIfCleanPropagaElFalloDeQuitarlo(t *testing.T) {
	raiz := t.TempDir()
	repo := repoConRama(t, "feat/x")
	g := NewGitDirect(raiz)
	wt, err := g.Create(context.Background(), Spec{
		Repo: repo, Branch: "feat/x", Path: filepath.Join(raiz, "prdash-pr-7"),
		Label: "prdash-pr-7",
	})
	if err != nil {
		t.Fatal(err)
	}

	// El camino bueno, que es el control: sin fallo, quita y no dice nada.
	borrado, motivo, err := g.RemoveIfClean(context.Background(), wt.Path)
	if err != nil || !borrado || motivo != "" {
		t.Fatalf("RemoveIfClean de un worktree limpio: (%v, %q, %v)", borrado, motivo, err)
	}

	// Y un id que no es suyo: la comprobación de ownership corta antes que cualquier cosa,
	// así que ni intenta quitarlo.
	if _, _, err := g.RemoveIfClean(context.Background(), filepath.Join(t.TempDir(), "ajeno")); err == nil {
		t.Error("RemoveIfClean de una ruta ajena dio nil")
	}
}
