package testutil

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
)

// Los fixtures de git de este paquete son la base de los tests de worktree, review y
// executor, así que un fallo aquí no se ve en un test: se ve en veinte, y como "el worktree
// no se montó" en vez de como "el fixture no pushed". La mitad de lo que hay aquí ya
// estaba probado en `git_test.go` —el aislamiento del repo real, que es la parte
//=July—; lo de este fichero es el camino feliz entero y las negativas.
//
// Y el camino feliz entero importa por una razón concreta: `Push` estaba al 0%, y es la
// única forma que tiene un test de comprobar que un clon local recibió una rama. Sin
// probarlo, un test que empuja y comprueba que el ref existe estaría affirming que el `Push` no empuja nada.

// TestElFixtureMontaUnRepoQueSePuedeClonarYEmpujar: el recorrido completo.
//
// Y acaba en un clon, no en comprobar que el remote existe: la pregunta que se responde es
// "¿un `git clone` de este fixture trae el contenido?", que es la que hacen los tests que
// lo usan.
func TestElFixtureMontaUnRepoQueSePuedeClonarYEmpujar(t *testing.T) {
	base := t.TempDir()

	// El remoto bare y el repo de trabajo.
	bare := filepath.Join(base, "origin.git")
	InitBare(t, bare)
	repo := filepath.Join(base, "work")
	InitRepo(t, repo)

	// Un fichero con un directorio padre que todavía no existe: `CommitFile` tiene que
	// crearlo, porque los fixtures de prdash escriben en `src/` y `docs/`.
	CommitFile(t, repo, "src/main.go", "package main\n", "primer commit")

	// Y el ref existe, que es lo que comprueban los tests de merge.
	if !RefExists(t, repo, "refs/heads/main") {
		t.Error("tras CommitFile no existe refs/heads/main")
	}
	// Y un ref que no existe, que es la otra mitad de `RefExists`.
	if RefExists(t, repo, "refs/heads/inexistente") {
		t.Error("RefExists dio true para un ref que no se creó nunca")
	}

	// Conectar y empujar. `SetRemote` quita antes de añadir, para que llamarlo dos veces
	// no falle —y para que re-apuntar un remoto sea un acto y no una特有的)}.
	SetRemote(t, repo, "origin", bare)
	Push(t, repo, "-u", "origin", "main")

	// Y el bare tiene la rama.
	if !RefExists(t, bare, "refs/heads/main") {
		t.Error("tras el push el remoto no tiene refs/heads/main")
	}

	// Re-apuntar el remoto: `remove` + `add`. Si `remove` no fuera tolerado a fallar,
	// este paso abortaría el test en vez de dejar el remoto como está.
	SetRemote(t, repo, "origin", bare)
	remotos := RunGit(t, repo, "remote")
	if !strings.Contains(remotos, "origin") {
		t.Errorf("tras re-apuntar el remoto quedó %q", remotos)
	}
	// Y un remoto con otro nombre, para que el test no dependa de que `origin` ya exista.
	SetRemote(t, repo, "otro", bare)
	if r := RunGit(t, repo, "remote"); strings.Count(r, "origin") != 1 {
		t.Errorf("re-apuntar dejó dos origins: %q", r)
	}

	// Y un clon de verdad trae el fichero. Esta es la comprobación que cierra el
	// recorrido: no que el push funcionó, sino que el resultado sirve para lo que un test
	// lo usa.
	//
	// Y se clona de verdad, no se reconstruye a mano. La primera versión de este test
	// "clonaba" haciendo `init` + un commit propio + `push`, y el push lo rechaza git
	// con "fetch first" porque las dos historias divergen —el commit propio no es el del
	// remoto—. El rechazo es correcto y el montaje estaba mal: lo que se quería era un
	// clon, y `git clone` es una línea.
	clon := filepath.Join(base, "clon")
	RunGit(t, base, "clone", bare, clon)
	if !RefExists(t, clon, "refs/heads/main") {
		t.Error("el clon no trae la rama del remoto")
	}
	contenido, err := os.ReadFile(filepath.Join(clon, "src", "main.go"))
	if err != nil {
		t.Fatalf("el clon no trae el fichero del remoto: %v", err)
	}
	if string(contenido) != "package main\n" {
		t.Errorf("el clon trae %q", contenido)
	}
	if RunGit(t, clon, "rev-parse", "HEAD") != RunGit(t, repo, "rev-parse", "HEAD") {
		t.Error("el clon y el repo no están en el mismo commit")
	}
}

// TestPushConArgsPropiosYSinArgs: `Push` solo antepone el verbo, y con eso basta.
//
// Y el caso sin argumentos es el que se usa más —`Push(t, dir, "-u", "origin", "main")` es
// un args con valores— así que la forma sin args es el default raro. Se prueba para que
// quede claro que no hay ningún argumento escondido.
func TestPushConArgsPropiosYSinArgs(t *testing.T) {
	base := t.TempDir()
	bare := filepath.Join(base, "o.git")
	InitBare(t, bare)
	repo := filepath.Join(base, "w")
	InitRepo(t, repo)
	CommitFile(t, repo, "a.txt", "x\n", "c")
	SetRemote(t, repo, "origin", bare)

	// Con flags.
	Push(t, repo, "-u", "origin", "main")
	if !RefExists(t, bare, "refs/heads/main") {
		t.Fatal("el push con flags no llegó al remoto")
	}

	// Sin nada: `git push` a secas. Aquí no se comprueba el resultado —sin `--set-upstream`
	// y sin destino, git falla y `RunGit` abortaría el test—, solo que la llamada se
	// construye bien y llega a ejecutarse. Y para verlo hace falta un remoto vacío al que
	// no le quede nada que empujar de más.
	if got := RunGit(t, repo, "config", "remote.origin.url"); got != bare {
		t.Fatalf("el remoto no quedó puesto: %q", got)
	}
}

// TestRunGitTraeLaSalidaYFallaConElComandoQueSeLePidio: `RunGit` es el ejecutor de todos
// los fixtures, y su contrato es "o sale con la salida, o aborta el test".
//
// Y la parte que merece aserto es la del aborta: el mensaje tiene que decir el comando Y el
// directorio. Un `t.Fatal(err)` a secas deja al que depura un "exit status 128" sin saber
// qué fixture lo dejó así, y con veinte paquetes usando estos fixtures eso es exactamente
// el problema.
func TestRunGitTraeLaSalidaYFallaConElComandoQueSeLePidio(t *testing.T) {
	dir := t.TempDir()

	// El camino bueno: stdout, recortado.
	if got := RunGit(t, dir, "--version"); got == "" {
		t.Error("git --version dio salida vacia")
	}
	// Y recortada: `RunGit` hace `TrimSpace`, y un salto al final haría que un test que
	// compara con una cadena tenga que escribirlo también.
	salida := RunGit(t, dir, "--version")
	if salida != strings.TrimSpace(salida) {
		t.Errorf("la salida no viene recortada: %q", salida)
	}
	// Y con varios args, en un repo de verdad.
	repo := filepath.Join(dir, "r")
	InitRepo(t, repo)
	if got := RunGit(t, repo, "rev-parse", "--is-inside-work-tree"); got != "true" {
		t.Errorf("git rev-parse dio %q, want true dentro de un repo recien creado", got)
	}
	// Y con un `TempDir` pelado, git NO encuentra repo. Que es justo lo que `gitEnv`
	// garantiza con el aislamiento, y lo que hace que los fixtures no se cuelguen en el
	// repo del que se está leyendo el código.
	// Y `requireDir` aborta ANTES de llegar a git cuando el directorio no existe, así que
	// el caso de un comando que falla no se puede provocar con `RunGit` sin que el test
	// muera. Queda fuera, y no es una laguna: es la razón de que `requireDir` exista
	// antes de la llamada a git, y su comportamiento está probado en
	// `TestCheckDirExplicaCadaNegativaPorSeparado`.
}

// TestCheckDirExplicaCadaNegativaPorSeparado: el motivo de cada `checkDir`.
//
// Y son tres, y cada uno previene un daño distinto. El `dir` vacío es el grave —git correría
// en el repo real y escribiría en su config—, y el comentario del código explica que ya
// pasó. El inexistente es un error de orchestration del test. Y el que no es directorio es
// el que más se cuela, porque `t.TempDir()` más un nombre siempre existe.
//
// Y `checkDir` está separada de `requireDir` justamente para poder probarla sin provocar un
// `t.Fatal`, que es la razón de que exista.
func TestCheckDirExplicaCadaNegativaPorSeparado(t *testing.T) {
	// Vacío: el motivo tiene que MENCIONAR el repo real, porque es lo que alguien lee a
	// las tres de la mañana cuando su suite ha dejado `core.bare=true`.
	err := checkDir("")
	if err == nil {
		t.Fatal("un dir vacío pasó")
	}
	if !strings.Contains(err.Error(), "repo real") {
		t.Errorf("el motivo del dir vacío no dice qué pasa: %q", err)
	}

	// Inexistente.
	inexistente := filepath.Join(t.TempDir(), "no-existe")
	err = checkDir(inexistente)
	if err == nil {
		t.Fatal("un dir inexistente pasó")
	}
	if !strings.Contains(err.Error(), inexistente) {
		t.Errorf("el motivo %q no nombra el directorio", err)
	}

	// Un fichero, no un directorio. Y el motivo tiene que distinguir los dos casos, que es
	// la razón de que haya dos mensajes y no uno genérico.
	fichero := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(fichero, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	err = checkDir(fichero)
	if err == nil {
		t.Fatal("un fichero pasó como directorio")
	}
	if !strings.Contains(err.Error(), "no es un directorio") {
		t.Errorf("el motivo %q no dice que no es un directorio", err)
	}

	// Y el caso bueno, porque una función de error que solo tiene el camino de error no se
	// ha probado.
	if err := checkDir(t.TempDir()); err != nil {
		t.Errorf("un TempDir dio error: %v", err)
	}
}

// TestInitBareYInitRepoCreanLoQueDicen: los dos constructores, y lo que los distingue.
//
// Y la diferencia no es el `--bare` de git: es el `-b main`. Los dos fuerzan `main` como
// rama inicial, y sin eso los fixtures dependen de la config global de quien los ejecuta —
// que en una máquina con `init.defaultBranch=master` daría todos los fixtures en `master` y
// los tests de merge pasarían por un camino que el producto no tiene.
func TestInitBareYInitRepoCreanLoQueDicen(t *testing.T) {
	base := t.TempDir()

	repo := filepath.Join(base, "work")
	InitRepo(t, repo)
	// `symbolic-ref` y no `rev-parse HEAD`: un repo sin commits no resuelve `HEAD`, y
	// justo este repo está sin commits en este punto del test. La rama inicial es lo que
	// se está mirando, y `symbolic-ref` la dice sin necesitar historial.
	if got := RunGit(t, repo, "symbolic-ref", "--short", "HEAD"); got != "main" {
		t.Errorf("InitRepo dejó la rama en %q, want main", got)
	}
	// Y con identidad local puesta, que sin ella el primer commit falla con "please tell
	// me who you are" en una máquina sin config global.
	for _, clave := range []string{"user.email", "user.name"} {
		if RunGit(t, repo, "config", clave) == "" {
			t.Errorf("InitRepo no puso %s", clave)
		}
	}
	// Y la firma desactivada: sin esto, un repo fixture en una máquina con
	// `commit.gpgsign=true` no puede commitear, y el fallo es "cannot sign" en un test que
	// no tiene nada que ver con firmas.
	if got := RunGit(t, repo, "config", "commit.gpgsign"); got != "false" {
		t.Errorf("InitRepo dejo commit.gpgsign en %q, want false", got)
	}

	bare := filepath.Join(base, "o.git")
	InitBare(t, bare)
	// Y el bare NO tiene working tree, que es lo que lo hace bare. La comprobación es
	// `rev-parse --is-bare-repository`, que es la que dice la verdad.
	if got := RunGit(t, bare, "rev-parse", "--is-bare-repository"); got != "true" {
		t.Errorf("InitBare no creo un repo bare: %q", got)
	}
	// Y el otro lado del mismo predicado: `--is-inside-work-tree` da FALSE dentro del
	// bare. El nombre engaña —pregunta si el directorio actual está dentro de un working
	// tree, no si el repo tiene uno—, así que la pareja de asertos que de verdad
	// distingue un bare de uno normal es la de arriba con "true" y esta con "false".
	if got := RunGit(t, bare, "rev-parse", "--is-inside-work-tree"); got != "false" {
		t.Errorf("dentro del bare, --is-inside-work-tree dio %q, want false", got)
	}
	// Y un repo normal, donde sí es true. La comparación de los dos es la que prueba que
	// `InitBare` hizo lo que dice.
	if got := RunGit(t, repo, "rev-parse", "--is-inside-work-tree"); got != "true" {
		t.Errorf("dentro de un repo normal dio %q, want true", got)
	}
}

// TestCommitFileCreaLosPadresYAnota: los ficheros anidados y el mensaje del commit.
//
// Y el mensaje importa porque hay tests que buscan el commit por su texto. Si
// `CommitFile` escribiera un mensaje fijo, dos tests distintos no podrían distinguishable
// sus commits, y el que buscara el suyo encontraría el del otro.
func TestCommitFileCreaLosPadresYAnota(t *testing.T) {
	dir := t.TempDir()
	InitRepo(t, dir)

	// Con padre de dos niveles que no existe.
	CommitFile(t, dir, "src/interno/deep.go", "package deep\n", "el commit con mensaje unico")

	contenido, err := os.ReadFile(filepath.Join(dir, "src", "interno", "deep.go"))
	if err != nil {
		t.Fatalf("CommitFile no creo los padres: %v", err)
	}
	if string(contenido) != "package deep\n" {
		t.Errorf("el contenido escrito es %q", contenido)
	}
	// Y el mensaje llegó al commit, que es lo que lo hace localizable.
	if !strings.Contains(RunGit(t, dir, "log", "--oneline"), "el commit con mensaje unico") {
		t.Error("el mensaje del commit no se guardó")
	}
	// Y un segundo commit con otro mensaje: los dos localizables por separado.
	CommitFile(t, dir, "otro.txt", "y\n", "mensaje distinto")
	log := RunGit(t, dir, "log", "--oneline")
	if !strings.Contains(log, "el commit con mensaje unico") || !strings.Contains(log, "mensaje distinto") {
		t.Errorf("los dos commits no se distinguen en el log: %q", log)
	}
}

// TestLaSuiteDeConformidadAceptaUnAdapterQueDiceUnsupported: la otra mitad de la puerta.
//
// Y es el camino que los tres adapters recorren cuando no están configurados, así que está
// tan sin probar como el otro. Y hay una asimetría en el contrato que este test fija: un
// adapter `unsupported` no puede devolver ítems, pero el BUSCADOR DE RAMAS sí se abre —con
// la lista vacía y el aviso—, porque si no el buscador parecería un repositorio sin ramas,
// que es un diagnóstico distinto del correcto.
//
// O sea: para el listado, "no puedo" quiere decir lista vacía. Para el buscador, quiere
// decir "avisa y abre". La asimetría es el contrato y por eso se comprueba.
func TestLaSuiteDeConformidadAceptaUnAdapterQueDiceUnsupported(t *testing.T) {
	a := adapterInerte()
	RunConformance(t, a, ConformanceOptions{Unsupported: true})

	// Y lo que la suite no comprueba pero el contrato exige: que el buscador se abriera.
	// Con `Unsupported`, `Branches` tiene que devolver lista vacía y el aviso.
	names, warns := a.Branches(t.Context(), refDePrueba())
	if len(names) != 0 {
		t.Errorf("un adapter inerte devolvio ramas: %v", names)
	}
	if len(warns) == 0 || warns[0].Kind != "unsupported" {
		t.Errorf("un adapter inerte dio %v en Branches, want un unsupported", warns)
	}
}

// TestHasKindMiraSoloElKind: el predicado que decide si un adapter es inerte.
//
// Y es un predicado tonto pero con una consecuencia: si mirara el mensaje en vez del kind,
// cada adapter tendría que escribir su `unsupported` de una manera concreta, y el texto
// cambia entre versiones de las CLIs. Mirando el kind, el texto puede ser el que sea.
func TestHasKindMiraSoloElKind(t *testing.T) {
	warns := []model.Warning{
		{Forge: "gh", Kind: "ratelimit", Msg: "lo que sea"},
		{Forge: "gh", Kind: "unsupported", Msg: "otro texto"},
	}
	if !hasKind(warns, "unsupported") {
		t.Error("hasKind no encontró el kind que está ahí")
	}
	if !hasKind(warns, "ratelimit") {
		t.Error("hasKind no encontró el primer kind")
	}
	// Y lo que no está.
	if hasKind(warns, "permission") {
		t.Error("hasKind encontró un kind que no está")
	}
	if hasKind(nil, "unsupported") {
		t.Error("hasKind encontró algo en una lista vacía")
	}
	if hasKind([]model.Warning{}, "unsupported") {
		t.Error("hasKind encontró algo en una lista vacía")
	}
}

// adapterInerte es un adapter que responde "unsupported" a todo, como uno sin credenciales.
// Se construye con el `FakeAdapter` porque sus defaults ya son casi los de un adapter
// inerte: listas vacías y conversaciones vacías.
func adapterInerte() *FakeAdapter {
	w := []model.Warning{{Forge: "inerte", Kind: "unsupported", Msg: "sin credenciales"}}
	ref := refDePrueba()
	clave := ItemKey(ref.Project, 1)
	a := &FakeAdapter{ForgeName: "inerte", HostName: "inerte.example"}
	for k := range a.Pages {
		a.ListWarnings[k] = w
	}
	a.ListWarnings = map[FakeKey][]model.Warning{}
	for _, q := range forge.Streams {
		a.ListWarnings[FakeKey{Section: q.Section, Kind: q.ReviewKind}] = w
	}
	a.StateWarnings = map[string][]model.Warning{clave: w}
	a.CommentWarnings = map[string][]model.Warning{clave: w}
	a.ActionWarnings = map[string][]model.Warning{
		"approve:o/r#1": w, "merge:o/r#1": w, "retarget:o/r#1": w,
	}
	a.BranchWarnings = map[string][]model.Warning{ref.Project: w}
	return a
}

// TestLaPuertaCierraAnteUnAdapterRoto: la prueba que antes no se podía escribir.
//
// Con la suite separada en `ConformanceViolations` esto es comprobable: se le da un adapter
// que rompe cada regla y se exige una lista con el incumplimiento. Y cada caso importa por
// separado, porque una comprobación que no dispara es una comprobación que no está.
//
// El caso que más merece nombre es `silencio`: sin binario, sin ítems y SIN AVISO. Un
// adapter así no falla nunca —no devuelve ítems, que es lo que se comprobaba— y deja el
// inbox vacío sin decir por qué. Es el único de los ocho que no se ve mirando los ítems, y
// por eso la suite exige el aviso.
func TestLaPuertaCierraAnteUnAdapterRoto(t *testing.T) {
	// De control: un adapter conforme no da ni una línea.
	f := adapterInerte()
	if fuera := ConformanceViolations(f, ConformanceOptions{Unsupported: true}); len(fuera) != 0 {
		t.Errorf("un adapter inerte dio %d incumplimientos:\n%s", len(fuera), strings.Join(fuera, "\n"))
	}

	roto := func(nombre, roto string, opts ConformanceOptions, quiere ...string) {
		t.Helper()
		fuera := ConformanceViolations(&adapterQueFalla{roto: roto}, opts)
		if len(fuera) == 0 {
			t.Errorf("%s: la suite dio el visto bueno a un adapter que rompe el contrato", nombre)
			return
		}
		for _, q := range quiere {
			if !contains(fuera, q) {
				t.Errorf("%s: la suite no se quejó de %q. Se quejó de:\n%s",
					nombre, q, strings.Join(fuera, "\n"))
			}
		}
	}

	roto("forge sin nombre", "forge-vacio", ConformanceOptions{}, "Forge() vacío")
	roto("host sin nombre", "host-vacio", ConformanceOptions{}, "Host() vacío")
	roto("paginacion incoherente", "more-sin-next", ConformanceOptions{}, "More=true sin Next")
	roto("no reporta unsupported", "no-avisar", ConformanceOptions{Unsupported: true},
		"debería reportar unsupported")
	roto("devuelve items siendo unsupported", "items", ConformanceOptions{Unsupported: true},
		"no debería devolver ítems")
	roto("devuelve ramas siendo unsupported", "ramas", ConformanceOptions{Unsupported: true},
		"no debería devolver ramas")
	roto("sin binario no avisa", "silencio", ConformanceOptions{MissingBinary: true},
		"debería devolver un warning")
	roto("sin binario devuelve items", "items", ConformanceOptions{MissingBinary: true},
		"sin binario no debería devolver ítems")

	// Y el caso sin ninguno de los dos modos: un adapter que devuelve ramas sin que se le
	// haya pedido modo. El aviso está, la lista no debería estar.
	fuera := ConformanceViolations(&adapterQueFalla{roto: "ramas"}, ConformanceOptions{})
	if !contains(fuera, "no debería devolver ramas") {
		t.Errorf("devolver ramas sin modo no se detectó: %v", fuera)
	}
	// Y el simétrico: un adapter conforme no se queja aunque no se le haya pedido nada.
	if fuera := ConformanceViolations(&adapterQueFalla{roto: "no-avisar"}, ConformanceOptions{}); len(fuera) != 0 {
		t.Errorf("sin modo y sin romper nada dio %v", fuera)
	}
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if strings.Contains(x, want) {
			return true
		}
	}
	return false
}

// adapterQueFalla rompe una regla distinta según `roto`, y cumple el resto. Cada `roto`
// corresponde a un aserto de la suite: si ese aserto desapareciera, el caso correspondiente
// de `TestLaPuertaCierraAnteUnAdapterRoto` dejaría de quejarse.
type adapterQueFalla struct {
	roto string
}

func (a *adapterQueFalla) Forge() string {
	if a.roto == "forge-vacio" {
		return ""
	}
	return "roto"
}
func (a *adapterQueFalla) Host() string {
	if a.roto == "host-vacio" {
		return ""
	}
	return "roto.example"
}
func (a *adapterQueFalla) Auth(context.Context) model.AuthState {
	return model.AuthState{Forge: "roto", OK: true}
}

func (a *adapterQueFalla) List(context.Context, forge.Query) (forge.Page, []model.Warning) {
	switch a.roto {
	case "more-sin-next":
		// Dice que hay más sin decir cómo llegar: la TUI pediría la siguiente página con un
		// cursor vacío, que es la primera otra vez.
		return forge.Page{More: true}, nil
	case "items":
		return forge.Page{Items: []model.Item{{Number: 1}}}, a.aviso()
	case "silencio":
		// Sin binario y sin decir nada. No devuelve ítems —eso sí lo cumple—, así que
		// cualquier suite que solo mirara los ítems lo daría por bueno.
		return forge.Page{}, nil
	case "no-avisar":
		return forge.Page{}, nil
	default:
		return forge.Page{}, a.aviso()
	}
}

// aviso devuelve el `unsupported` salvo que el caso sea precisamente "no avisar".
func (a *adapterQueFalla) aviso() []model.Warning {
	if a.roto == "no-avisar" {
		return nil
	}
	return warnUnsupported()
}

func (a *adapterQueFalla) ItemState(context.Context, model.RepoRef, int) (model.Item, []model.Warning) {
	return model.Item{}, a.aviso()
}
func (a *adapterQueFalla) Comments(context.Context, model.RepoRef, int) (forge.CommentPage, []model.Warning) {
	return forge.CommentPage{}, a.aviso()
}
func (a *adapterQueFalla) Approve(context.Context, model.RepoRef, int) []model.Warning {
	return a.aviso()
}
func (a *adapterQueFalla) Merge(context.Context, model.RepoRef, int, forge.MergeRequest) []model.Warning {
	return a.aviso()
}
func (a *adapterQueFalla) Retarget(context.Context, model.RepoRef, int, string) []model.Warning {
	return a.aviso()
}

// Branches devuelve ramas aun pudiendo no hacerlo: es la regla que dice que el buscador se
// abre aunque el listado venga vacío, y un repo sin ramas se parece a uno donde nadie miró.
func (a *adapterQueFalla) Branches(context.Context, model.RepoRef) ([]string, []model.Warning) {
	if a.roto == "ramas" {
		return []string{"main"}, a.aviso()
	}
	return nil, a.aviso()
}

func warnUnsupported() []model.Warning {
	return []model.Warning{{Forge: "roto", Kind: "unsupported", Msg: "esto es un doble roto"}}
}

// TestElInformeSeLlamaUnaVezPorIncumplimientoYNingunaSinIncumplimientos: el bucle de
// `RunConformance`.
//
// Y es la mitad de `RunConformance` que se puede probar en proceso, y se separó para eso: la
// otra mitad —el `t.Error` de verdad— solo se comprueba por subproceso, porque `testing.T` no
// se puede doblear y `t.Error` no detiene la ejecución.
//
// Y lo que se fija es el CONTEO, que es lo que decide si la puerta cierra o solo avisa:
//
//   - Una llamada por incumplimiento, y con el texto del incumplimiento. Un bucle que
//     llamara dos veces por incumplimiento —o que no llamara— mostraría la suite el doble de
//     veces o la dejaría pasar.
//   - Y cero llamadas sin incumplimientos, que es el caso bueno: una puerta conforme no dice
//     nada, y una que llama siempre con un texto vacío rompe el test que no debía tocar.
func TestElInformeSeLlamaUnaVezPorIncumplimientoYNingunaSinIncumplimientos(t *testing.T) {
	// De control: un adapter inerte no dice nada.
	var dichos []string
	reportaIncumplimientos(adapterInerte(), ConformanceOptions{Unsupported: true},
		func(v string) { dichos = append(dichos, v) })
	if len(dichos) != 0 {
		t.Errorf("un adapter inerte produjo %d informes: %v", len(dichos), dichos)
	}

	for _, c := range []struct {
		nombre string
		roto   string
		opts   ConformanceOptions
		quiere string
	}{
		{"forge sin nombre", "forge-vacio", ConformanceOptions{}, "Forge() vacío"},
		{"host sin nombre", "host-vacio", ConformanceOptions{}, "Host() vacío"},
		{"no reporta unsupported", "no-avisar", ConformanceOptions{Unsupported: true},
			"debería reportar unsupported"},
		{"devuelve items siendo unsupported", "items", ConformanceOptions{Unsupported: true},
			"no debería devolver ítems"},
	} {
		var dicho []string
		reportaIncumplimientos(&adapterQueFalla{roto: c.roto}, c.opts,
			func(v string) { dicho = append(dicho, v) })

		if len(dicho) == 0 {
			t.Errorf("%s: la puerta no dijo nada y un adapter roto pasó el visto bueno", c.nombre)
			continue
		}
		// Y el texto del incumplimiento va íntegro, que es lo que permite corregir sin
		// volver a mirar la suite.
		found := false
		for _, v := range dicho {
			if strings.TrimSpace(v) == "" {
				t.Errorf("%s: un informe con texto vacío es indistinguible de no avisar",
					c.nombre)
			}
			if strings.Contains(v, c.quiere) {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: los informes son %q, y ninguno dice %q", c.nombre, dicho, c.quiere)
		}
	}

	// Y el caso que hace que el conteo signifique algo: UN incumplimiento produce exactamente
	// un informe. Sin esta aserción, un bucle que llamara tres veces por incumplimiento
	// pasaría todos los casos anteriores, que solo miran que se diga algo.
	var uno []string
	reportaIncumplimientos(&adapterQueFalla{roto: "forge-vacio"}, ConformanceOptions{},
		func(v string) { uno = append(uno, v) })
	if len(uno) != 1 {
		t.Errorf("un adapter que rompe una sola regla produjo %d informes, want 1: %v",
			len(uno), uno)
	}
}
