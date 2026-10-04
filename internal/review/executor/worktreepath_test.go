package executor

import (
	"testing"

	"prdash/internal/cache"
	"prdash/internal/forge/model"
	"prdash/internal/reporesolver"
)

// reviewCon arma un registro de review con el worktree dado. Los otros campos van vacíos a
// propósito: lo que se mira aquí es solo el worktree, y rellenarlos sería meter en el test
// cosas que no se están probando.
func reviewCon(worktree string) cache.ReviewRecord {
	return cache.ReviewRecord{Worktree: worktree}
}

// TestElWorktreeActivoMandaSobreLaRutaCanonica: si hay un review activo con worktree, se
// usa ese; si no, la ruta canónica del resolutor.
//
// Y las dos mitades importan por motivos distintos. La primera es la que evita perder
// trabajo: el worktree del review activo puede haberse movido, y si se calculara la ruta
// canónica se montaría el review en un sitio que no es donde está el trabajo. Montar dos
// veces el mismo review deja el primero ocupando sitio y sin revisar.
//
// La segunda es la del arranque: sin review activo se usa la ruta canónica, que es la del
// resolutor y su único dueño. Y tiene que ser la misma que usa `Audit`, o aparecerían
// worktrees que prdash no reconoce como suyos.
//
// Y el caso del medio es el que se olvidaba: un review activo SIN worktree. La condición es
// un `&&`, así que un registro a medio rellenar cae en la ruta canónica —y eso es lo
// correcto, porque un registro sin worktree no tiene un worktree que usar—. Con la
// condición al revés, un review activo sin worktree devolvería la cadena vacía y el
// montaje se iría a un directorio vacío.
func TestElWorktreeActivoMandaSobreLaRutaCanonica(t *testing.T) {
	// El memo del resolutor tiene una ruta por defecto que NO depende de `WorktreeDir`, y
	// sin este `Setenv` el test lee el memo de otro test de este mismo paquete: el primer
	// intento dio una ruta dentro del TempDir de `TestMountClonesBareWhenRepoNotLocal`.
	//
	// O sea que el fallo no era del aserto sino del aislamiento, y se ve como un aserto
	// que se queja de un número que no es el suyo. Un test que depende del estado que
	// dejó otro no falla donde falla la lógica.
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	wtDir := t.TempDir()
	ref := model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget",
		Owner: "acme", Name: "widget"}
	it := model.NewItem(ref, 7)
	it.Title = "uno"

	resolver := reporesolver.New(reporesolver.Options{WorktreeDir: wtDir})
	ex := &Executor{Resolver: resolver}

	// La ruta canónica, que es la del resolutor. Es la referencia contra la que se
	// compara todo lo demás.
	canonica := resolver.WorktreePath(ref, 7)
	if canonica == "" {
		t.Fatal("la ruta canónica salió vacía, y sin ella el test no tiene contra qué " +
			"comparar el worktree activo")
	}
	if got := ex.worktreePath(it); got != canonica {
		t.Errorf("sin review activo dio %q, want la ruta canónica %q", got, canonica)
	}

	// Y ahora un review activo CON worktree: ese manda, aunque no sea la ruta canónica.
	// Se usa una ruta distinta a propósito, porque si fuera la misma el test no probaría
	// nada.
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

	// Y con un review activo SIN worktree: se cae a la ruta canónica, que es lo único que
	// hay. Un registro a medio rellenar no es un worktree.
	if err := resolver.RecordReview(it, reviewCon("")); err != nil {
		t.Fatalf("RecordReview sin worktree: %v", err)
	}
	if got := ex.worktreePath(it); got != canonica {
		t.Errorf("con un review activo sin worktree dio %q, want la ruta canónica %q: un "+
			"registro a medio rellenar no tiene un worktree que usar, y con la condición "+
			"al revés esto devolvía la cadena vacía", got, canonica)
	}

	// Y con un review activo de OTRO ítem, este cae en SU ruta canónica: el registro es
	// por ítem y no se mezcla. Y la referencia es la del 8, no la del 7 — que es lo que
	// me puso la primera versión de este test.
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
