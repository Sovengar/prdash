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

// El índice y el parseo de remotos son las dos mitades de "resolver dónde está este repo", y
// las dos tienen una rama que decide "este repo no sirve" — que es la mitad del trabajo, porque
// un índice con basura dentro devuelve el repo equivocado en vez de ninguno.
//
// Y las dos ramas son de entrada, no de salida: un remoto que no se puede interpretar y un
// directorio que no se puede indexar. Ninguna falla ruidosamente; las dos se descartan en
// silencio, y por eso los tests tienen que mirar lo que NO está en el índice y no lo que sí.

// TestElIndiceIgnoraLoQueNoEsUnRepoConRemotoInterpretable: el filtro del `buildIndex`.
//
// Y los tres descartes son distintos y los tres importan:
//
//   - Un directorio que no es repo: se salta, y `isGitRepo` es la comprobación que lo dice.
//   - Un repo SIN remoto `origin`: se salta. Es el caso de un clon que alguien hizo sin
//     remoto, y es legítimamente unknowable.
//   - Un repo con un remoto que no se puede interpretar: se salta. Y este es el caro, porque
//     un remoto como `/home/y/o/mis-repos` es un PATH y no una URL de forge, y un indexador que
//     lo aceptara devolvería la ruta local como si fuera la ruta canónica —que es la que se
//     pasa luego a `git clone`—.
func TestElIndiceIgnoraLoQueNoEsUnRepoConRemotoInterpretable(t *testing.T) {
	raiz := t.TempDir()

	// Un repo normal con remoto de GitHub: este SÍ se indexa.
	bueno := filepath.Join(raiz, "bueno")
	testutil.InitRepo(t, bueno)
	testutil.CommitFile(t, bueno, "a.txt", "a", "a")
	testutil.SetRemote(t, bueno, "origin", "https://github.com/acme/proyecto.git")

	// Un repo sin remoto: unknowable.
	sinRemoto := filepath.Join(raiz, "sin-remoto")
	testutil.InitRepo(t, sinRemoto)
	testutil.CommitFile(t, sinRemoto, "a.txt", "a", "a")

	// Un repo cuyo remoto es un PATH local: no es un forge.
	conPath := filepath.Join(raiz, "con-path")
	testutil.InitRepo(t, conPath)
	testutil.CommitFile(t, conPath, "a.txt", "a", "a")
	testutil.SetRemote(t, conPath, "origin", filepath.Join(raiz, "otro-lugar"))

	// Un directorio que no es repo en absoluto.
	plano := filepath.Join(raiz, "plano")
	if err := os.MkdirAll(plano, 0o755); err != nil {
		t.Fatal(err)
	}

	r := New(Options{
		Roots:    []string{raiz},
		CloneDir: filepath.Join(t.TempDir(), "clones"),
		MemoPath: filepath.Join(t.TempDir(), "memo.json"),
		// El parseo real necesita la tabla de hosts: con el mapa vacío,
		// `ParseRemoteURL` rechaza cualquier remoto y el índice queda vacío.
		Hosts: map[string]string{"github.com": "github"},
	})
	idx := r.buildIndex()

	// El bueno está, y con SU ruta.
	ref := model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/proyecto"}
	key := repoKey(ref)
	if got, ok := idx[key]; !ok {
		t.Errorf("el repo con remoto de GitHub no se indexó; el índice tiene %v", idx)
	} else if got != bueno {
		t.Errorf("el repo quedó en %q, want %q", got, bueno)
	}
	// Y los otros tres NO están, que es la parte que un test de "está el bueno" no comprueba.
	for _, no := range []string{sinRemoto, conPath, plano} {
		for k, v := range idx {
			if v == no {
				t.Errorf("%q se indexó bajo la clave %q: no es un repo con remoto interpretable", no, k)
			}
		}
	}
	// Y el índice tiene UNA entrada, no varias: un remoto que no se puede interpretar que
	// hubiera pasado añadiría una clave basura y `ResolveLocal` podría devolver esa ruta.
	if len(idx) != 1 {
		t.Errorf("el índice tiene %d entradas, want 1: %v", len(idx), idx)
	}
}

// TestElIndiceNoEntraEnLosGitNiEnLosWorktrees: el salto del recorrido.
//
// Y son dos cosas que se parecen y no son lo mismo:
//
//   - `.git` es el directorio de metadatos de un repo. Entrar ahí encuentra `.git/worktrees/`,
//     que es el REGISTRO INTERNO de git, y un indexador que lo listara acabaría resolviendo
//     "el repo de este PR" a la entrada interna del worktree de otro PR.
//   - Un marcador de worktree es un directorio con un `.git` FICHERO. Es el worktree de verdad,
//     y su repo es el MISMO que el del repo principal, así que no es una entrada más del
//     índice.
func TestElIndiceNoEntraEnLosGitNiEnLosWorktrees(t *testing.T) {
	raiz := t.TempDir()
	repo := filepath.Join(raiz, "repo")
	testutil.InitRepo(t, repo)
	testutil.CommitFile(t, repo, "a.txt", "a", "a")
	testutil.SetRemote(t, repo, "origin", "https://github.com/acme/proyecto.git")

	// Un worktree del repo dentro de la misma raíz. Es un directorio más con un `.git` fichero.
	wt := filepath.Join(raiz, "mi-worktree")
	testutil.RunGit(t, repo, "branch", "feat/x")
	testutil.RunGit(t, repo, "worktree", "add", "--quiet", wt, "feat/x")

	r := New(Options{
		Roots:    []string{raiz},
		CloneDir: filepath.Join(t.TempDir(), "clones"),
		MemoPath: filepath.Join(t.TempDir(), "memo.json"),
		// El parseo real necesita la tabla de hosts: con el mapa vacío,
		// `ParseRemoteURL` rechaza cualquier remoto y el índice queda vacío.
		Hosts: map[string]string{"github.com": "github"},
	})
	idx := r.buildIndex()

	// La entrada es la del repo principal, y no la del worktree: son el mismo repo y el
	// worktree además vive en una ruta que `git clone` no entendería como origen.
	for k, v := range idx {
		if strings.Contains(v, string(filepath.Separator)+"mi-worktree") {
			t.Errorf("el worktree se indexó como repo propio bajo %q: %q", k, v)
		}
	}
	ref := model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/proyecto"}
	if got := idx[repoKey(ref)]; got != repo {
		t.Errorf("el índice resolvió a %q, want el repo principal %q", got, repo)
	}
	// Y lo que importa para el daño: NADA dentro de `.git`.
	for _, v := range idx {
		if strings.Contains(v, string(filepath.Separator)+".git"+string(filepath.Separator)) {
			t.Errorf("el índice tiene una entrada de dentro de un .git: %q", v)
		}
	}
}

// TestUnaRaizInaccesibleNoRompeElIndice: `filepath.Abs` que falla.
//
// Y la forma de provokedarlo es real y no un doble: `filepath.Abs` de una ruta RELATIVA llama a
// `os.Getwd`, y `Getwd` falla cuando el directorio de trabajo ya no existe —un repo que se
// borró mientras prdash estaba abierto, o un `cd` a un directorio temporal que `t.TempDir()`
// ya limpió—.
//
// Y lo que importa es que el fallo de UNA raíz no tire el índice entero: hay más raíces, y
// perderlas todas por una que desapareció sería un fallo que se ve como "prdash no encuentra
// ningún repo" sin explicación.
func TestUnaRaizInaccesibleNoRompeElIndice(t *testing.T) {
	// Un directorio que se borra: es el cwd que `Getwd` no puede resolver.
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

	// Una raíz RELATIVA cuyo cwd se invalida. Y hay que hacer el cambio dentro de un
	// subtest, porque `os.Chdir` es global al proceso y `t.Chdir` lo restaura.
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
			// El parseo real necesita la tabla de hosts: con el mapa vacío,
			// `ParseRemoteURL` rechaza cualquier remoto y el índice queda vacío.
			Hosts: map[string]string{"github.com": "github"},
		})
		// Con el cwd todavía vivo, la Abs de la raíz relativa falla por no existir el
		// directorio, que es el mismo camino de error que un cwd muerto.
		idx := r.buildIndex()
		ref := model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/proyecto"}
		if got := idx[repoKey(ref)]; got != bueno {
			t.Errorf("una raíz inválida se llevó el índice entero: resolvió a %q, want %q",
				got, bueno)
		}
	})

	// Y una raíz que NO existe en absoluto, que es el caso que se da cuando una de las raíces
	// del config se borró. No es un error: la config puede listar raíces de máquinas y
	// máquinas que ya no están.
	r := New(Options{
		Roots:    []string{filepath.Join(t.TempDir(), "no-existe"), bueno},
		CloneDir: filepath.Join(t.TempDir(), "clones"),
		MemoPath: filepath.Join(t.TempDir(), "memo.json"),
		// El parseo real necesita la tabla de hosts: con el mapa vacío,
		// `ParseRemoteURL` rechaza cualquier remoto y el índice queda vacío.
		Hosts: map[string]string{"github.com": "github"},
	})
	idx := r.buildIndex()
	ref := model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/proyecto"}
	if got := idx[repoKey(ref)]; got != bueno {
		t.Errorf("una raíz inexistente se llevó el índice entero: resolvió a %q, want %q", got, bueno)
	}
}

// TestUnRemotoConUnaParteVaciaNoSeConvierteEnUnRepo: la guarda de `parseRemote`.
//
// Y la forma de llegar a ella es una barra doble, que es lo que produce un remoto mal escrito
// a mano o un `.gitmodules` con una rutavacío: `https://github.com//proyecto`.
//
// Y el daño de no comprobarlo es silencioso: sin la guarda, `parts` tendría un elemento vacío y
// `Project` sería "//proyecto", que es una clave de índice VÁLIDA. Prdash buscaría ese repo,
// no lo encontraría, y el fallo aparecería como "no se pudo resolver" en vez de "el remoto está
// mal escrito".
func TestUnRemotoConUnaParteVaciaNoSeConvierteEnUnRepo(t *testing.T) {
	for _, remote := range []string{
		// Barra doble DENTRO de la ruta, que es la guarda que se quiere probar.
		"https://github.com//proyecto.git",
		"https://github.com/acme//proyecto.git",
		"https://github.com///.git",
		"git@github.com:acme//proyecto.git",
		// Y los que ya se rechazan antes, como control de que la guarda no es lo único.
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

	// Y el camino bueno, porque si no "todo se rechaza" contaría como prueba.
	//
	// Y aquí van TRES formas que mi primera versión ponía en la lista de rechazados y que se
	// aceptan, con razón. No son un fallo: son la forma normal de escribir un remoto, y
	// excluirlas habría dejado el test probando una regla que no existe.
	//
	//   - `…/proyecto//.git`: el doble slash ANTES del `.git` es un artefacto real —se
	//     concatena una ruta con un sufijo vacío— y el recorte lo deja en `acme/proyecto`.
	//     La guarda rechaza una barra doble en el MEDIO, que sí indica un hueco.
	//   - `git@…`: la forma SCP. Es el remoto por defecto de `git clone` cuando no hay
	//     protocolo, así que es el camino más común de todos.
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
	// Y Owner y Name salen de las DOS ÚLTIMAS partes, no de las dos primeras: con un grupo
	// intermedio, `Owner` es el grupo y no el dueño real. Es lo que usa `splitProject` en
	// GitHub para armar `gh -R`, así que un error aquí manda las acciones al repo equivocado.
	if ref.Owner != "grupo" || ref.Name != "proyecto" {
		t.Errorf("Owner/Name = %s/%s, want grupo/proyecto", ref.Owner, ref.Name)
	}
}

// parseRemoteDePrueba delega en el parseo REAL del paquete —`ParseRemoteURL` con la misma
// tabla de hosts que usa producción— para no probar una copia.
//
// Y una copia de la función con los mismos casos daría verde con la función rota, que es el
// peor tipo de test que existe: el test pasa y no está probando nada del código.
//
// Y la tabla de hosts es obligatoria: `ParseRemoteURL` rechaza cualquier host que no esté en
// ella, así que un resolver sin `Hosts` no interpretaría ni un remoto de GitHub. Mi primera
// versión usaba un resolver vacío y todos los casos salían rechazados —incluido el del camino
// bueno, que es el que avisa de que el fallo es del fixture y no del código—.
func parseRemoteDePrueba(raw string) (model.RepoRef, bool) {
	return ParseRemoteURL(raw, hostsDePrueba(), nil)
}

// TestFetchReviewRefPropagaElFalloDelFetchConSuRef: el fallo de red del montaje.
//
// Y el mensaje tiene que decir QUÉ ref se intentó traer, porque hay dos refs en juego y
// confundirlos hace que el diagnóstico dé a la rama equivocada: el `src` es el del forge —
// `refs/pull/7/head` en GitHub— y el `track` es el local que se crea a partir de él.
//
// Y el caso es real: el remoto se cae, o el ref ya no existe porque alguien borró la rama del PR, que es lo que pasa cuando se cierra el PR desde GitHub con la rama de detrás.
func TestFetchReviewRefPropagaElFalloDelFetchConSuRef(t *testing.T) {
	origin, repo := fixture(t)
	r := newResolver(t, origin, ghRef())
	it := model.NewItem(ghRef(), 7)
	it.Number = 7

	// El ref de review no existe en el origin: el clon tiene main pero no el del PR.
	_, err := r.FetchReviewRef(context.Background(), repo, it)
	if err == nil {
		t.Fatal("traer un ref que no existe dio nil")
	}
	if !strings.Contains(err.Error(), "fetch") {
		t.Errorf("el error %q no dice que falla el fetch", err)
	}
	// Y nombra el repo, que es lo que se necesita para saber si es un remoto caído o un ref
	// que ya no está.
	if !strings.Contains(err.Error(), "acme") {
		t.Errorf("el error %q no nombra el proyecto del que seerró", err)
	}
	// Y no se creó ninguna rama local a medias. La guarda va ANTES del `git branch`, y una
	// rama local apuntando a un ref inexistente haría que `Create` la REUTILIZARA —su rama
	// coincide con la pedida— y montara un review vacío sin avisar de nada.
	for _, ref := range []string{"prdash/pr-7", "refs/prdash/github/7"} {
		if testutil.RefExists(t, repo, ref) {
			t.Errorf("el fetch fallido dejó %s en el repo: Create lo reutilizaría y el "+
				"review se montaría vacío", ref)
		}
	}
}
