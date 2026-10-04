package reporesolver

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

// Estas dos funciones son las que deciden si un montaje repite trabajo o no, y las dos se
// resumen en la misma pregunta: "¿esto ya está hecho?". Las dos respuestas equivocadas
// cuestan lo mismo —un fetch por cada montaje— y las dos son silenciosas: nada falla, solo
// se va más lento.

// TestRecordarUnaRutaVaciaNoEnsuciaLaMemoria: la negativa de `Remember`.
//
// Y la memoria es "clones ya resueltos", así que una entrada con la ruta vacía es una
// entrada que dice "ya sé dónde está este repo" sin decirlo. Un resolver que la consultara
// creería que tiene el clon y montaría el review sobre un directorio que no existe, que es
// el fallo más caro de la cadena: un worktree creado en el sitio equivocado.
//
// Y se comprueba por los dos lados, que es como se ve el efecto real: la clave no aparece,
// y una ruta de verdad sí se guarda y se lee. Sin el segundo, el test pasaría aunque
// `Remember` no guardara nunca nada.
func TestRecordarUnaRutaVaciaNoEnsuciaLaMemoria(t *testing.T) {
	memoPath := t.TempDir() + "/memo.json"
	r := New(Options{MemoPath: memoPath})
	ref := model.RepoRef{Forge: "github", Host: "github.com", Project: "o/r", Owner: "o", Name: "r"}

	// La ruta vacía no se guarda.
	r.Remember(ref, "")
	if _, ok := r.store.Route(repoKey(ref)); ok {
		t.Error("una ruta vacía se guardó en la memoria: un resolver la creería válida")
	}

	// Y una ruta de verdad, sí.
	const ruta = "/clones/o/r"
	r.Remember(ref, ruta)
	if got, ok := r.store.Route(repoKey(ref)); !ok || got != ruta {
		t.Errorf("la ruta no se guardó: (%q, %v)", got, ok)
	}

	// Y dos repos distintos no se pisan, que es lo que hace que el memo sirva con más de
	// un repo.
	otro := model.RepoRef{Forge: "github", Host: "github.com",
		Project: "grupo/sub/proy", Owner: "grupo", Name: "proy"}
	r.Remember(otro, "/clones/grupo/sub/proy")
	if got, _ := r.store.Route(repoKey(ref)); got != ruta {
		t.Errorf("guardar un proyecto pisó el otro: %q", got)
	}

	// Y SOBREVIVE a un resolver nuevo sobre el mismo fichero. Si no, la segunda ejecución
	// de la app volvería a clonar lo que ya estaba clonado, que es lo que el memo evita.
	nuevo := New(Options{MemoPath: memoPath})
	if got, ok := nuevo.store.Route(repoKey(ref)); !ok || got != ruta {
		t.Errorf("la ruta no se recuperó de un resolver nuevo: (%q, %v)", got, ok)
	}
}

// TestLaRamaDeReviewSeReutilizaYNoSeVuelveACrear: el otro lado de `FetchReviewRef`.
//
// Y la razón de la comprobación es de trabajo, no de corrección: `git fetch` del ref de
// review es una llamada de red por montaje, y es la que más tarda. Si además se creara la
// rama local cada vez, el montaje de un PR ya montado costaría dos operaciones en vez de
// una.
//
// Y el orden importa: el fetch va ANTES de mirar si la rama existe. Al revés, un resolver
// nuevo sobre un repo ya montado crearía una rama que apunta al ref viejo — porque el fetch
// es lo que la actualiza— y el review se montaría sobre un commit que nadie miró, que es
// exactamente el fallo que el pin de head SHA evita en el merge y que aquí sería el mismo
// Applied al principio del montaje.
func TestLaRamaDeReviewSeReutilizaYNoSeVuelveACrear(t *testing.T) {
	origin, repo := fixture(t)
	const contenido = "primera version"
	// Un ÚNICO repo de trabajo con dos commits encadenados, porque un ref de review es una
	// línea de historia y no dos ramas: dos commits sin relación se rechaza al empujar por
	// no ser fast-forward, y el fetch del código fuerza el ref con `+` precisamente porque
	// el forge lo mueve como quiere.
	work := filepath.Join(t.TempDir(), "pr")
	testutil.InitRepo(t, work)
	testutil.SetRemote(t, work, "origin", origin)
	testutil.CommitFile(t, work, "pr.txt", contenido, "revision 1")
	testutil.Push(t, work, origin, "HEAD:"+reviewRefDe(it0(model.NewItem(ghRef(), 7))))
	ref := ghRef()
	r := newResolver(t, origin, ref)
	it := model.NewItem(ref, 7)
	it.Number = 7
	ctx := context.Background()

	// La primera vez: crea la rama.
	branch, err := r.FetchReviewRef(ctx, repo, it)
	if err != nil {
		t.Fatalf("el primer FetchReviewRef: %v", err)
	}
	if branch == "" {
		t.Fatal("FetchReviewRef devolvió una rama vacía")
	}
	// Y la rama existe y apunta al commit del ref de review.
	head := testutil.RunGit(t, repo, "rev-parse", branch)
	esperado := testutil.RunGit(t, repo, "rev-parse", "refs/prdash/github/7")
	if head != esperado {
		t.Errorf("la rama apunta a %s y el ref a %s", head, esperado)
	}

	// La segunda vez: reutiliza. Y lo que se comprueba es que la rama NO se recrea —que
	// perdería el puntero al commit— y que el fetch sí ocurrió, que es lo que la
	// mantiene al día.
	antes := testutil.RunGit(t, repo, "rev-parse", branch)
	otra, err := r.FetchReviewRef(ctx, repo, it)
	if err != nil {
		t.Fatalf("el segundo FetchReviewRef: %v", err)
	}
	if otra != branch {
		t.Errorf("la segunda vez dio la rama %q, want la misma %q: recrearla pierde el "+
			"puntero al commit", otra, branch)
	}
	if despues := testutil.RunGit(t, repo, "rev-parse", branch); despues != antes {
		t.Errorf("la rama se movió de %s a %s entre llamadas con el mismo ref", antes, despues)
	}

	// Y aquí está el caso que mi primera versión leía al revés, y es el que da sentido a
	// todo lo demás: con el PR republicado, el ref de seguimiento SÍ se mueve y la rama
	// local NO.
	// El segundo commit, sobre el primero. Con contenido distinto a propósito: con el
	// mismo contenido, el mismo mensaje y la misma fecha, git da el mismo SHA y el ref no
	// se movería — que fue el primer rojo de este test.
	testutil.CommitFile(t, work, "pr.txt", contenido+" y algo mas", "revision 2")
	testutil.Push(t, work, origin, "HEAD:"+reviewRefDe(it0(model.NewItem(ghRef(), 7))))
	if _, err := r.FetchReviewRef(ctx, repo, it); err != nil {
		t.Fatal(err)
	}
	seguimiento := testutil.RunGit(t, repo, "rev-parse", "refs/prdash/github/7")
	refRemoto := testutil.RunGit(t, origin, "rev-parse", reviewRefDe(it0(model.NewItem(ghRef(), 7))))
	if seguimiento != refRemoto {
		t.Errorf("el ref de seguimiento está en %s y el remoto en %s: el fetch no lo movió",
			seguimiento, refRemoto)
	}
	if seguimiento == antes {
		t.Fatal("el fixture no republicó nada: el ref de seguimiento debería haber cambiado")
	}

	// Y la rama local se queda donde estaba. NO es un olvido: es lo que dice el
	// comentario de la función —"nunca toca una rama que ya exista"—, y es lo correcto
	// porque esa rama está en uso en el worktree del review en curso. Moverla mientras está
	// en uso dejaría el review a medio camino, y git la rechazaría igualmente.
	//
	// La consecuencia —que el review se monte sobre un commit viejo si el PR se actualizó
	// entre montajes— es exactamente lo que el pin de head SHA del merge detecta. Esa es
	// la cadena: aquí se prefiere no tocar nada y no romper el worktree, y la deriva se
	// avisa en el merge, que es donde importa.
	if despues := testutil.RunGit(t, repo, "rev-parse", branch); despues != antes {
		t.Errorf("la rama local se movió de %s a %s con el PR republicado: está en uso "+
			"por el worktree del review", antes, despues)
	}
}

// TestUnForgeSinRefDeReviewConocidoNoSeAguantaLaPeticion: la negativa de `FetchReviewRef`.
//
// Y el motivo de fallar en vez de inventar un ref es el del pin: un ref inventado —
// `refs/heads/main` cuando no se sabe cuál es— haría que el montaje trajera la rama base en
// vez del PR, y el usuario vería un review del commit equivocado sin ningún aviso. El
// error tiene que nombrar el forge, porque el destino de la acción depende de él.
func TestUnForgeSinRefDeReviewConocidoNoSeAguentaLaPeticion(t *testing.T) {
	origin, repo := fixture(t)
	r := newResolver(t, origin, ghRef())

	// Un forge sin ref conocido. Los tres que hay en prdash lo tienen, así que esto necesita
	// uno inventado — que es justo lo que pasa si se añade un forge sin escribir su ref—.
	it := model.NewItem(ghRef(), 7)
	it.Forge = "bitbucket"

	_, err := r.FetchReviewRef(context.Background(), repo, it)
	if err == nil {
		t.Fatal("un forge sin ref de review dio nil: inventaría un ref y montaría la rama base")
	}
	if !strings.Contains(err.Error(), "bitbucket") {
		t.Errorf("el error %q no nombra el forge", err)
	}
	if !strings.Contains(err.Error(), "review ref") {
		t.Errorf("el error %q no dice que falta el ref de review", err)
	}
}

// it0 es un Item vacío del forge que usa el fixture.
func it0(m model.Item) model.Item { return m }

// reviewRefDe devuelve el ref de review de un ítem, o "main" si no lo tiene —que no
// debería pasar con los forges de prdash—.
func reviewRefDe(it model.Item) string {
	ref, ok := ReviewRef(it)
	if !ok {
		return "main"
	}
	return ref
}
