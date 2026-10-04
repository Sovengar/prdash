package executor

import (
	"testing"

	"prdash/internal/cache"
	"prdash/internal/forge/model"
	"prdash/internal/reporesolver"
)

func reviewCon(worktree string) cache.ReviewRecord {
	return cache.ReviewRecord{Worktree: worktree}
}

func TestElWorktreeActivoMandaSobreLaRutaCanonica(t *testing.T) {
	// The resolver's memo has a default path that does NOT depend on WorktreeDir; without this Setenv
	//the test reads the user's memo.
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	wtDir := t.TempDir()
	ref := model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget",
		Owner: "acme", Name: "widget"}
	it := model.NewItem(ref, 7)
	it.Title = "uno"

	resolver := reporesolver.New(reporesolver.Options{WorktreeDir: wtDir})
	ex := &Executor{Resolver: resolver}

	canonica := resolver.WorktreePath(ref, 7)
	if canonica == "" {
		t.Fatal("la ruta canónica salió vacía, y sin ella el test no tiene contra qué " +
			"comparar el worktree activo")
	}
	if got := ex.worktreePath(it); got != canonica {
		t.Errorf("sin review activo dio %q, want la ruta canónica %q", got, canonica)
	}

	// An active review WITH a worktree wins, even when it is not the canonical path.
	activo := wtDir + "/movido-a-mano"
	if err := resolver.RecordReview(it, reviewCon(activo)); err != nil {
		t.Fatalf("RecordReview: %v", err)
	}
	if got := ex.worktreePath(it); got != activo {
		t.Errorf("con review activo dio %q, want el worktree del registro %q. Con la ruta "+
			"canónica el review se montaría en un sitio que no es donde está el trabajo, "+
			"y el primero se queda ocupando sitio sin revisar",
			got, activo)
	}

	// An active review WITHOUT a worktree falls back to the canonical path, which is all there is.
	if err := resolver.RecordReview(it, reviewCon("")); err != nil {
		t.Fatalf("RecordReview sin worktree: %v", err)
	}
	if got := ex.worktreePath(it); got != canonica {
		t.Errorf("con un review activo sin worktree dio %q, want la ruta canónica %q: un "+
			"registro a medio rellenar no tiene un worktree que usar, y con la condición "+
			"al revés esto devolvía la cadena vacía", got, canonica)
	}

	// An active review of ANOTHER item: this one falls back to its canonical path, because the record
	// is per item.
	otro := model.NewItem(ref, 8)
	otro.Title = "otro"
	canonica8 := resolver.WorktreePath(ref, 8)
	if canonica8 == canonica {
		t.Fatalf("los dos ítems comparten ruta canónica (%q), y con eso la comparación "+
			"no distinguiría nada", canonica8)
	}
	if got := ex.worktreePath(otro); got != canonica8 {
		t.Errorf("con review activo del 7, el ítem 8 dio %q, want su ruta canónica %q: el "+
			"registro es por ítem y no se mezcla", got, canonica8)
	}
	if canonica8 == activo {
		t.Errorf("el review activo del 7 es %q y la ruta canónica del 8 también: la "+
			"comparación no distinguiría nada", activo)
	}
}
