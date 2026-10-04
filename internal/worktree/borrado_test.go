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

// La última tanda de guardas de `worktree`, y las dos técnicas que hacen falta aquí son cosas
// del SISTEMA y del HERDR FALSO:
//
//   - Un padre en solo lectura hace que `os.RemoveAll` falle con EACCES, que es la única forma
//     de disparar los fallos de borrado sin mocking.
//   - Un `fakeRunner` de Herdr con `createErr` y `listResult` vacíos cubre la provisión nativa.
//
// Y lo que tienen en común es que son fallos cuyo efecto es "quedó algo a medias". Un `Create`
// que falla con el destino ocupado, un `Remove` que no puede quitar el checkout, un `Audit` que
// baja dentro de un `.git` y acaba borrando el registro interno de git.

// separadorGit es lo que delata que una ruta está dentro de un `.git`. Vive en una
// variable porque `filepath.Separator` es un rune en Linux y un byte en Windows, y la
// concatenación con una cadena no compila en el segundo caso — que es justo el punto de usar
// `filepath` en vez de "/".
var separadorGit = string(filepath.Separator) + ".git" + string(filepath.Separator)

// conPadreEnSoloLectura deja el padre de `ruta` sin permiso de escritura, de modo que borrar o
// crear dentro de él falla con EACCES.
//
// Y el modo 000 en la ruta que se va a borrar no serviría: para quitar un directorio hace falta
// permiso de escritura en SU PADRE, no en él. Es el detalle que hace que un test de este tipo
// falle sin motivo aparente si se piensa al revés.
func conPadreEnSoloLectura(t *testing.T, padre string) {
	t.Helper()
	if err := os.Chmod(padre, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// Sin restaurar, el `t.TempDir()` no puede limpiar y el fallo se reporta como un error
		// de limpieza sin relación con lo que se estaba probando.
		_ = os.Chmod(padre, 0o755)
	})
}

// TestCreateFallaSiElPadreDelDestinoNoSePuedeCrearYNoDejaNada: la última guarda de `Create`.
//
// Y el caso es el de un destino cuyo padre es un fichero. Ocurre cuando alguien tiene un
// directorio `prdash-algo` en el sitio donde prdash quiere crear `prdash-algo/prdash-pr-7`, y el
// código tiene que fallar ANTES de llamar a git, no dejar que git lo diga de su manera.
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
	// Y el fichero del usuario sigue ahí: el guard falla antes de tocar nada.
	raw, err := os.ReadFile(bloqueo)
	if err != nil || string(raw) != "soy un fichero" {
		t.Errorf("el bloqueo cambió: %q %v", raw, err)
	}
}

// TestRemovePropagaElFalloDeQuitarElCheckout: el borrado que no puede borrar.
//
// Y hay dos caminos distintos que llegan al mismo `os.RemoveAll`, y los dos importan:
//
//   - El huérfano sin repo: el `.git` no declara un gitdir, así que no hay repositorio que podar
//     y solo queda borrar el checkout.
//   - El registro corrupto: el repo vive pero `git worktree remove` no quita la entrada, así que
//     se poda y se borra el residuo.
//
// Y la diferencia con el camino bueno es que ahí `RemoveAll` no falla nunca, así que sin este
// test el error se vería como "nunca pasa".
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
				// Un `.git` de trabajo que no declara gitdir: el huérfano por definición.
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
			// La raíz en solo lectura hace que quitar el checkout falle con EACCES.
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
			// Y el mensaje nombra la ruta, que es lo que se necesita para ir a buscarla a mano.
			if !strings.Contains(err.Error(), id) {
				t.Errorf("el error %q no nombra la ruta que no se pudo quitar", err)
			}
		})
	}
}

// TestRemoveIfCleanPropagaElFalloDeQuitarloCuandoEstaLimpio: la segunda etapa del candado.
//
// Y este es el que más se confunde con el anterior, porque `RemoveIfClean` tiene DOS etapas y
// las dos pueden fallar. La primera —¿está limpio?— se comprueba con `git status` y su fallo ya
// está cubierto. Esta es la segunda: estaba limpio y quitarlo falló.
//
// Y el motivo de que el error tenga que PROPAGARSE y no comerse: la limpieza al cerrar la TUI
// llama a esta función. Un fallo tragado ahí es un worktree que sigue ocupando disco, y el
// siguiente `Audit` lo listaría como si nada —el usuario ve su review en la lista y no hay nada
// detrás-.
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

	// Con el padre escribible: el candado pasa y quita.
	//
	// Y este caso es un HUÉRFANO de verdad, con su `.git` apuntando a un repo que no existe.
	// Mi primera versión montaba un directorio con un `.git` de trabajo y esperaba que
	// `shouldRemove` lo diera por limpio; lo que hace es lo correcto: sin repo no se puede
	// leer el estado, así que NO es limpio y no se borra. El motivo que devuelve —"could not
	// read the worktree status"— es la salvaguarda funcionando, y por eso ese camino no
	// llega a `Remove` y no sirve para probar el fallo de la segunda etapa.
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

	// Y ahora un worktree REAL —su repo vive, así que el estado se lee— con el padre en solo
	// lectura: el candado pasa porque está limpio, y quitarlo falla porque ni git ni
	// `os.RemoveAll` pueden escribir en el padre.
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
	// Y el worktree TAMBIÉN en solo lectura, porque quitar un directorio necesita permiso de
	// escritura en el propio directorio, no solo en el padre: con el padre bloqueado y el
	// worktree escribible, `git worktree remove` lo borra sin problema y el `os.RemoveAll` de
	// reserva no llega a ejecutarse. Son dos permisos y hacen falta los dos.
	//
	// Mi primera versión solo bloqueó el padre, y el test falló diciendo que el worktree se
	// quitó "a pesar del error": el error venía del `prune` y el borrado lo había hecho git.
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
	// Y sigue ahí, que es el daño del que hablaba el comentario.
	if !Exists(wt.Path) {
		t.Error("el worktree se quitó a pesar del error")
	}
}

// gEn es `NewGitDirect` con un nombre corto, porque en un test que llama cinco veces la
// repetición del nombre tapa la lectura del aserto.
func gEn(raiz string) *GitDirect { return NewGitDirect(raiz) }

// TestAuditSaltaLosDirectoriosGitAnidados: el `SkipDir` del recorrido.
//
// Y la versión de este test que había antes comprobaba lo mismo en el `.git` de un worktree, y
// no、广州 canzar nada: el `.git` de un worktree ENLAZADO es un FICHERO, y el recorrido vuelve
// antes de mirar el nombre porque la guarda de `d.IsDir()` va primero. El `SkipDir` solo se
// alcanza con un `.git` que sea un DIRECTORIO, que es lo que tiene un clon normal dentro de la
// raíz gestionada.
func TestAuditSaltaLosDirectoriosGitAnidados(t *testing.T) {
	raiz := t.TempDir()
	// Un clon normal dentro de la raíz: su `.git` es un directorio con la estructura entera.
	repo := filepath.Join(raiz, "no-es-prdash")
	testutil.InitRepo(t, repo)
	testutil.CommitFile(t, repo, "f.txt", "x", "x")

	// Y un worktree real, para que el recorrido tenga algo que encontrar: sin él, un `Audit`
	// que no bajara en ningún sitio daría la misma respuesta.
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
	// Y NADA de dentro de un `.git`. Con el salto puesto, el directorio `.git/worktrees` —que
	// tiene una entrada por worktree— no se lista, y `prdash worktrees remove --orphans` no lo
	// puede convertir en objetivo de borrado.
	for _, e := range entradas {
		if strings.Contains(e.Path, separadorGit) {
			t.Errorf("Audit listó algo de dentro de un .git: %q", e.Path)
		}
	}
	// Y `List` tampoco, que comparte el recorrido.
	for _, w := range g.List(context.Background()) {
		if strings.Contains(w.Path, separadorGit) {
			t.Errorf("List devolvió algo de dentro de un .git: %q", w.Path)
		}
	}
}

// TestLaProvisionNativaDelegaElAuditEnElEscaneoYPropagaSusFallos: la delegacion.
//
// Y `Audit`, `List` y `RemoveIfClean` de `HerdrNative` son una línea cada una y delegan en el
// escaneo de git. Delegar parece que no necesita test —es una línea— y sin embargo es
// exactamente el punto donde un error se traga: si `Audit` devolviera `nil` en vez de delegar,
// la limpieza dentro de Herdr no listaría nada y `remove --orphans` no borraría ni un huerfano,
// sin ningún aviso.
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

	// Y el fallo de Herdr al abrir un worktree se propaga con la ruta, que es lo que la TUI
	// necesita para decir "no se pudo montar ESTE review".
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

	// Y `reuse` sobre algo que no es un worktree enlazado: ni siquiera se le pregunta a Herdr,
	// porque no hay nada que reutilizar.
	if _, err := fallido.Create(context.Background(), Spec{
		Repo: repo, Branch: "feat/x", Path: filepath.Join(t.TempDir(), "no-existe"),
		Label: "prdash-pr-7",
	}); err == nil {
		t.Error("Create nativo sobre un destino vacío con Herdr que no crea dio nil")
	}
}
