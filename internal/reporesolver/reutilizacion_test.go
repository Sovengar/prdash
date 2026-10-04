package reporesolver

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

// Both answer the same question: "is this already done?".

// The memo holds resolved clones, so an empty path is not one of them.
func TestRecordarUnaRutaVaciaNoEnsuciaLaMemoria(t *testing.T) {
	memoPath := t.TempDir() + "/memo.json"
	r := New(Options{MemoPath: memoPath})
	ref := model.RepoRef{Forge: "github", Host: "github.com", Project: "o/r", Owner: "o", Name: "r"}

	r.Remember(ref, "")
	if _, ok := r.store.Route(repoKey(ref)); ok {
		t.Error("una ruta vacía se guardó en la memoria: un resolver la creería válida")
	}

	const ruta = "/clones/o/r"
	r.Remember(ref, ruta)
	if got, ok := r.store.Route(repoKey(ref)); !ok || got != ruta {
		t.Errorf("la ruta no se guardó: (%q, %v)", got, ok)
	}

	otro := model.RepoRef{Forge: "github", Host: "github.com",
		Project: "grupo/sub/proy", Owner: "grupo", Name: "proy"}
	r.Remember(otro, "/clones/grupo/sub/proy")
	if got, _ := r.store.Route(repoKey(ref)); got != ruta {
		t.Errorf("guardar un proyecto pisó el otro: %q", got)
	}

	// And it SURVIVES a new resolver over the same file, or the second execution would rebuild
	// everything.
	nuevo := New(Options{MemoPath: memoPath})
	if got, ok := nuevo.store.Route(repoKey(ref)); !ok || got != ruta {
		t.Errorf("la ruta no se recuperó de un resolver nuevo: (%q, %v)", got, ok)
	}
}

// A truncated history of chained commits, because a review ref is ONE line of history and not
// two branches.
func TestLaRamaDeReviewSeReutilizaYNoSeVuelveACrear(t *testing.T) {
	origin, repo := fixture(t)
	const contenido = "primera version"
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

	branch, err := r.FetchReviewRef(ctx, repo, it)
	if err != nil {
		t.Fatalf("el primer FetchReviewRef: %v", err)
	}
	if branch == "" {
		t.Fatal("FetchReviewRef devolvió una rama vacía")
	}
	head := testutil.RunGit(t, repo, "rev-parse", branch)
	esperado := testutil.RunGit(t, repo, "rev-parse", "refs/prdash/github/7")
	if head != esperado {
		t.Errorf("la rama apunta a %s y el ref a %s", head, esperado)
	}

	// The second time: it reuses. What is asserted is that the branch is NOT recreated, because
	// recreating it would lose uncommitted work.
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

	// My first version read this backwards.
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

	// The local branch stays where it was, and that is not an oversight.
	if despues := testutil.RunGit(t, repo, "rev-parse", branch); despues != antes {
		t.Errorf("la rama local se movió de %s a %s con el PR republicado: está en uso "+
			"por el worktree del review", antes, despues)
	}
}

// Failing rather than pretending: there is no such thing as a review ref without a forge to
// ask.
func TestUnForgeSinRefDeReviewConocidoNoSeAguentaLaPeticion(t *testing.T) {
	origin, repo := fixture(t)
	r := newResolver(t, origin, ghRef())

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

func it0(m model.Item) model.Item { return m }

func reviewRefDe(it model.Item) string {
	ref, ok := ReviewRef(it)
	if !ok {
		return "main"
	}
	return ref
}
