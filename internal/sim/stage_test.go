package sim

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/testutil"
)

// `stage` y `materialize` son el camino de verdad de una simulación, y lo que cuentan es
// QUÉ ramas existen en el clon de simulación y cuál queda activa. Y ese "cuál queda activa"
// es una decisión, no un detalle: de ella depende qué grafo ve el usuario.
//
//   - Merge y rebase: la ACTIVA es la base, y la que se dibuja es la del ítem. El grafo
//     que interesa es "¿qué pasa si integro esto ahí".
//   - Rebase: la activa es la del ítem. Y al revés —porque es un rebase—, el grafo es la
//     base contra la del ítem.
//
// Es la misma asimetría en el otro sentido, y es la que más cara sale si se intercambia: un
// merge dibujado al revés enseña un grafo que no corresponde a la operación que el usuario
// pidió, y no hay ningún aviso que lo diga.

// simRepoMonta un repo con la rama `main` y la rama del ítem, que es el caso normal que
// llega a `stage`.
func simRepoMonta(t *testing.T) (repo, tmp string) {
	t.Helper()
	repo = filepath.Join(t.TempDir(), "repo")
	testutil.InitRepo(t, repo)
	testutil.CommitFile(t, repo, "base.txt", "base", "base")
	testutil.SetRemote(t, repo, "origin", repo)
	testutil.CommitFile(t, repo, "feat.txt", "feat", "feat")
	// La rama del ítem sale de main, que es como llega de la review.
	if _, err := os.Stat(filepath.Join(repo, ".git")); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "branch", "feat/x")
	git(t, repo, "branch", "main-origin", "main")
	tmp = t.TempDir()
	return repo, tmp
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	return testutil.RunGit(t, dir, args...)
}

// TestStageDejaActivaLaBaseParaIntegrarYLaDelItemParaRebasar: la asimetría.
//
// Y el orden de las dos ramas es lo que hay que mirar en cada caso: `stage` materializa la
// base y la del ítem SIEMPRE, y lo único que cambia con el modo es cuál queda activa y cuál
// se dibuja.
//
// Y el caso del rebase es el que más se confunde al leer el codigo, porque el `ref` que se
// dibuja es la del item pero la activa es la base —un rebase se calcula desde la base hacia
// la rama, no al revés—.
func TestStageDejaActivaLaBaseParaIntegrarYLaDelItemParaRebasar(t *testing.T) {
	ctx := context.Background()

	// Merge: activa la base, y el ref es la del ítem.
	repo, tmp := simRepoMonta(t)
	s := New(locatorDeStage())
	path, spec, err := s.stage(ctx, Place{Repo: repo, Branch: "feat/x"}, KindMerge, "main", tmp)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if got := git(t, path, "rev-parse", "--abbrev-ref", "HEAD"); got != "main" {
		t.Errorf("merge dejó activa %q, want main: el grafo de un merge es la base con "+
			"la rama encima", got)
	}
	if spec.Ref != "feat/x" {
		t.Errorf("el ref dibujado es %q, want la rama del item", spec.Ref)
	}
	if spec.Kind != KindMerge {
		t.Errorf("el spec no lleva el modo: %+v", spec)
	}

	// Rebase: activa la del ítem, y el ref es la base.
	repo, tmp = simRepoMonta(t)
	path, spec, err = s.stage(ctx, Place{Repo: repo, Branch: "feat/x"}, KindRebase, "main", tmp)
	if err != nil {
		t.Fatalf("rebase: %v", err)
	}
	if got := git(t, path, "rev-parse", "--abbrev-ref", "HEAD"); got != "feat/x" {
		t.Errorf("rebase dejó activa %q, want la rama del item: un rebase se calcula "+
			"desde la rama hacia la base", got)
	}
	if spec.Ref != "main" {
		t.Errorf("el ref dibujado es %q, want la base", spec.Ref)
	}
}

// TestStageNoClonaSiNoSeSabeLaRamaDelItem: la comprobación va ANTES del clon.
//
// Y el orden es lo que se fija. Clonar para descubrir después que no hay rama del ítem
// sería trabajo tirado: un clon completo de un repo grande es de segundos y de espacio. El
// error lo dice claro, porque quien lo ve es la TUI después de pulsar una tecla y lo que
// necesita saber es qué hacer —montar el review antes de simular—.
func TestStageNoClonaSiNoSeSabeLaRamaDelItem(t *testing.T) {
	repo, tmp := simRepoMonta(t)
	tmpAntes := contarEntradas(t, tmp)

	_, _, err := New(locatorDeStage()).stage(context.Background(),
		Place{Repo: repo, Branch: ""}, KindMerge, "main", tmp)

	if err == nil {
		t.Fatal("sin rama del item dio nil")
	}
	if !strings.Contains(err.Error(), "branch") {
		t.Errorf("el error %q no dice que falta la rama", err)
	}
	if !strings.Contains(err.Error(), "review") {
		t.Errorf("el error %q no dice qué hacer: montar el review", err)
	}
	// Y no se clonó nada, que es la mitad del motivo del orden.
	if despues := contarEntradas(t, tmp); despues != tmpAntes {
		t.Errorf("clonó pese a no saber la rama: %d entradas antes, %d después", tmpAntes, despues)
	}
}

// TestMaterializeUsaLaRamaLocalSiYaExiste: el orden local antes que remoto.
//
// Y es el caso NORMAL, no el raro: `git clone` deja la rama de la que se clonó en local, así
// que la base casi siempre está ya. Y dejarla como está es lo correcto, porque la base
// local es la contra la que se comparó la review —forzarla a `origin/` compararía contra
// otra cosa y daría un grafo que no es de nadie—.
//
// Y la trampa de al revés también: si `materialize` hiciera `branch --force` sobre
// `origin/`, cada simulación perdería los commits locales de la base sin avisar.
func TestMaterializeUsaLaRamaLocalSiYaExiste(t *testing.T) {
	repo, tmp := simRepoMonta(t)
	s := New(locatorDeStage())
	path := filepath.Join(tmp, "clone")
	git(t, tmp, "clone", "--quiet", "--shared", repo, path)

	// Un commit SOLO en la base local, que es lo que un `fetch` no se traería.
	testutil.CommitFile(t, path, "local.txt", "local", "solo local")
	antes := git(t, path, "rev-parse", "main")

	if err := s.materialize(context.Background(), path, "main"); err != nil {
		t.Fatalf("materialize de una rama local: %v", err)
	}

	despues := git(t, path, "rev-parse", "main")
	if antes != despues {
		t.Errorf("materialize movió la rama local de %s a %s: un fetch no se trae el "+
			"commit local y el grafo dejaría de ser el de la review", antes, despues)
	}
	// Y el fichero sigue ahí, que es la comprobación que hace visible lo anterior.
	if _, err := os.Stat(filepath.Join(path, "local.txt")); err != nil {
		t.Errorf("materialize tiró el commit local: %v", err)
	}
}

// TestMaterializeCreaLaRamaDelItemDesdeElRemoto: el caso que hace falta.
//
// Y es el caso habitual para la rama del ítem: `git clone` solo trae en local la rama de
// HEAD, así que la rama de la review hay que crearla desde `origin/`. Y si no existe ni en
// local ni en el remoto, el error lo dice —porque simular sin la rama dibujaría un grafo de
// otra cosa, y eso es peor que no simular.
func TestMaterializeCreaLaRamaDelItemDesdeElRemoto(t *testing.T) {
	repo, tmp := simRepoMonta(t)
	s := New(locatorDeStage())
	path := filepath.Join(tmp, "clone")
	git(t, tmp, "clone", "--quiet", "--shared", repo, path)

	// La rama del ítem no está en local —el clon se hizo desde main— pero sí en origin.
	if tiene := git(t, path, "branch", "--list", "feat/x"); strings.TrimSpace(tiene) != "" {
		t.Fatalf("feat/x ya estaba en local, y este test necesita que no: %q", tiene)
	}
	if err := s.materialize(context.Background(), path, "feat/x"); err != nil {
		t.Fatalf("materialize de una rama remota: %v", err)
	}
	// Y ahora sí existe en local, apuntando al mismo sitio que en el remoto.
	local := git(t, path, "rev-parse", "feat/x")
	remoto := git(t, path, "rev-parse", "origin/feat/x")
	if local != remoto {
		t.Errorf("la rama creada apunta a %s y el remoto a %s", local, remoto)
	}

	// Y una rama que no existe en ninguna parte: error claro, sin crear nada.
	err := s.materialize(context.Background(), path, "no-existe")
	if err == nil {
		t.Fatal("una rama inexistente dio nil")
	}
	if !strings.Contains(err.Error(), "no-existe") {
		t.Errorf("el error %q no nombra la rama", err)
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("el error %q no dice que no existe", err)
	}
	// Y el mensaje dice dónde mirar: el clon local, que es donde se buscó.
	if !strings.Contains(err.Error(), "local clone") {
		t.Errorf("el error %q no dice dónde se buscó", err)
	}
}

// TestElClonDeSimulacionEsTemporalYShared: las dos propiedades del clon.
//
// Y `--shared` no es una optimización: es lo que hace que el clon no copie objetos. Un
// clon completo de un repo de páginas de historial pesa gigas, y la simulación se pide cada
// vez que el usuario abre el popup.
//
// Y `--quiet` evita que git progresse por stdout, que es lo que se parsea después: una
// línea de progreso del clon sería JSON inválido.
func TestElClonDeSimulacionEsTemporalYShared(t *testing.T) {
	repo, tmp := simRepoMonta(t)
	s := New(locatorDeStage())

	path, _, err := s.stage(context.Background(),
		Place{Repo: repo, Branch: "feat/x"}, KindMerge, "main", tmp)
	if err != nil {
		t.Fatal(err)
	}

	// El clon existe Y está en el temporal, no en el repo del usuario.
	if filepath.Dir(path) != tmp {
		t.Errorf("el clon quedó en %q, want dentro de %q: el clon de simulación no puede "+
			"vivir junto al repo del usuario", filepath.Dir(path), tmp)
	}
	// Y no dejó nada en el repo de origen: ni refs nuevas, ni ramas, ni worktrees.
	ramas := git(t, repo, "branch", "--list")
	if strings.Contains(ramas, "sim") || strings.Contains(ramas, "tmp") {
		t.Errorf("la simulación dejó ramas en el repo de origen: %q", ramas)
	}
	// Y el repo de origen sigue teniendo UN solo worktree —el suyo—, porque un clon de
	// simulación no se registra como worktree. Si lo hiciera, cada simulación añadiría
	// una fila al `git worktree list` del repo del usuario.
	// Y son LÍNEAS, no el total: `git worktree list` no pone newline al final y
	// `RunGit` recorta, así que contar `\n` daría 0 líneas para un repo con un
	// worktree. La primera version de esta comprobación contaba saltos y daba -1, que
	// es el sintoma de estar midiendo la cosa equivocada.
	lineas := strings.Split(strings.TrimSpace(git(t, repo, "worktree", "list")), "\n")
	if len(lineas) != 1 {
		t.Errorf("la simulación dejó worktrees en el repo de origen: %d", len(lineas)-1)
	}
}

// locatorDeStage es un Locator que no se usa: `stage` no lo llama, pero `New` lo exige
// para armar el servicio con su runner de git. Un `&Service{}` a pelo tiene el runner a
// nil y `stage` revienta al clonar —que es el modo de fallo que da menos pistas—.
func locatorDeStage() Locator { return locatorFalso{} }

// contarEntradas cuenta las entradas de un directorio, para comprobar que no se clona.
func contarEntradas(t *testing.T, dir string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	return len(entries)
}
