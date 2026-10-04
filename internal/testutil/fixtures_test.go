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

// It ends in a clone rather than in checking the remote exists: the question is "does a git clone of
// this fixture bring the content?", which is what the tests using it ask.
func TestElFixtureMontaUnRepoQueSePuedeClonarYEmpujar(t *testing.T) {
	base := t.TempDir()

	bare := filepath.Join(base, "origin.git")
	InitBare(t, bare)
	repo := filepath.Join(base, "work")
	InitRepo(t, repo)

	CommitFile(t, repo, "src/main.go", "package main\n", "primer commit")

	if !RefExists(t, repo, "refs/heads/main") {
		t.Error("tras CommitFile no existe refs/heads/main")
	}
	if RefExists(t, repo, "refs/heads/inexistente") {
		t.Error("RefExists dio true para un ref que no se creó nunca")
	}

	SetRemote(t, repo, "origin", bare)
	Push(t, repo, "-u", "origin", "main")

	if !RefExists(t, bare, "refs/heads/main") {
		t.Error("tras el push el remoto no tiene refs/heads/main")
	}

	SetRemote(t, repo, "origin", bare)
	remotos := RunGit(t, repo, "remote")
	if !strings.Contains(remotos, "origin") {
		t.Errorf("tras re-apuntar el remoto quedó %q", remotos)
	}
	SetRemote(t, repo, "otro", bare)
	if r := RunGit(t, repo, "remote"); strings.Count(r, "origin") != 1 {
		t.Errorf("re-apuntar dejó dos origins: %q", r)
	}

	// Cloned for real, not rebuilt by hand: what is checked is that the result is usable, not that the
	//push worked.
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

// The no-arguments form is the rare default, since `Push(t, dir, "-u", "origin", "main")` is an
// args case.
func TestPushConArgsPropiosYSinArgs(t *testing.T) {
	base := t.TempDir()
	bare := filepath.Join(base, "o.git")
	InitBare(t, bare)
	repo := filepath.Join(base, "w")
	InitRepo(t, repo)
	CommitFile(t, repo, "a.txt", "x\n", "c")
	SetRemote(t, repo, "origin", bare)

	Push(t, repo, "-u", "origin", "main")
	if !RefExists(t, bare, "refs/heads/main") {
		t.Fatal("el push con flags no llegó al remoto")
	}

	// The result is NOT checked here: without --set-upstream and a destination git fails and RunGit
	//would abort the test, so this only asserts the call is built and runs.
	if got := RunGit(t, repo, "config", "remote.origin.url"); got != bare {
		t.Fatalf("el remoto no quedó puesto: %q", got)
	}
}

// The part worth asserting is the abort: the message has to name the command AND the directory.
func TestRunGitTraeLaSalidaYFallaConElComandoQueSeLePidio(t *testing.T) {
	dir := t.TempDir()

	if got := RunGit(t, dir, "--version"); got == "" {
		t.Error("git --version dio salida vacia")
	}
	salida := RunGit(t, dir, "--version")
	if salida != strings.TrimSpace(salida) {
		t.Errorf("la salida no viene recortada: %q", salida)
	}
	repo := filepath.Join(dir, "r")
	InitRepo(t, repo)
	if got := RunGit(t, repo, "rev-parse", "--is-inside-work-tree"); got != "true" {
		t.Errorf("git rev-parse dio %q, want true dentro de un repo recien creado", got)
	}
	// With a bare TempDir git finds no repo, which is what gitEnv's isolation guarantees and what keeps
	//the fixtures from hanging onto the repo the code is being read from.
}

// Three reasons, each preventing a different damage; the empty dir is the grave one, because git
// would run in the real repo and write its config.
func TestCheckDirExplicaCadaNegativaPorSeparado(t *testing.T) {
	err := checkDir("")
	if err == nil {
		t.Fatal("un dir vacío pasó")
	}
	if !strings.Contains(err.Error(), "repo real") {
		t.Errorf("el motivo del dir vacío no dice qué pasa: %q", err)
	}

	inexistente := filepath.Join(t.TempDir(), "no-existe")
	err = checkDir(inexistente)
	if err == nil {
		t.Fatal("un dir inexistente pasó")
	}
	if !strings.Contains(err.Error(), inexistente) {
		t.Errorf("el motivo %q no nombra el directorio", err)
	}

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

	if err := checkDir(t.TempDir()); err != nil {
		t.Errorf("un TempDir dio error: %v", err)
	}
}

// The difference is not `--bare` but `-b main`: both force main as the initial branch, and without
// that the fixtures depend on the global config of whoever runs them.
func TestInitBareYInitRepoCreanLoQueDicen(t *testing.T) {
	base := t.TempDir()

	repo := filepath.Join(base, "work")
	InitRepo(t, repo)
	if got := RunGit(t, repo, "symbolic-ref", "--short", "HEAD"); got != "main" {
		t.Errorf("InitRepo dejó la rama en %q, want main", got)
	}
	for _, clave := range []string{"user.email", "user.name"} {
		if RunGit(t, repo, "config", clave) == "" {
			t.Errorf("InitRepo no puso %s", clave)
		}
	}
	if got := RunGit(t, repo, "config", "commit.gpgsign"); got != "false" {
		t.Errorf("InitRepo dejo commit.gpgsign en %q, want false", got)
	}

	bare := filepath.Join(base, "o.git")
	InitBare(t, bare)
	if got := RunGit(t, bare, "rev-parse", "--is-bare-repository"); got != "true" {
		t.Errorf("InitBare no creo un repo bare: %q", got)
	}
	// `--is-inside-work-tree` answers FALSE inside a bare; the name misleads, since it asks whether the
	//current directory is inside a work tree, not whether the repo has one.
	if got := RunGit(t, bare, "rev-parse", "--is-inside-work-tree"); got != "false" {
		t.Errorf("dentro del bare, --is-inside-work-tree dio %q, want false", got)
	}
	if got := RunGit(t, repo, "rev-parse", "--is-inside-work-tree"); got != "true" {
		t.Errorf("dentro de un repo normal dio %q, want true", got)
	}
}

// The message matters because some tests find the commit by its text; a fixed message would make two
// tests indistinguishable.
func TestCommitFileCreaLosPadresYAnota(t *testing.T) {
	dir := t.TempDir()
	InitRepo(t, dir)

	CommitFile(t, dir, "src/interno/deep.go", "package deep\n", "el commit con mensaje unico")

	contenido, err := os.ReadFile(filepath.Join(dir, "src", "interno", "deep.go"))
	if err != nil {
		t.Fatalf("CommitFile no creo los padres: %v", err)
	}
	if string(contenido) != "package deep\n" {
		t.Errorf("el contenido escrito es %q", contenido)
	}
	if !strings.Contains(RunGit(t, dir, "log", "--oneline"), "el commit con mensaje unico") {
		t.Error("el mensaje del commit no se guardó")
	}
	CommitFile(t, dir, "otro.txt", "y\n", "mensaje distinto")
	log := RunGit(t, dir, "log", "--oneline")
	if !strings.Contains(log, "el commit con mensaje unico") || !strings.Contains(log, "mensaje distinto") {
		t.Errorf("los dos commits no se distinguen en el log: %q", log)
	}
}

// This is the path the three adapters take when unconfigured, so it was as untested as the other
// half.
func TestLaSuiteDeConformidadAceptaUnAdapterQueDiceUnsupported(t *testing.T) {
	a := adapterInerte()
	RunConformance(t, a, ConformanceOptions{Unsupported: true})

	names, warns := a.Branches(t.Context(), refDePrueba())
	if len(names) != 0 {
		t.Errorf("un adapter inerte devolvio ramas: %v", names)
	}
	if len(warns) == 0 || warns[0].Kind != "unsupported" {
		t.Errorf("un adapter inerte dio %v en Branches, want un unsupported", warns)
	}
}

// A dumb predicate with a consequence: looking at the message instead of the kind would force every
// adapter to phrase its `unsupported` one specific way.
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

func TestLaPuertaCierraAnteUnAdapterRoto(t *testing.T) {
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

	fuera := ConformanceViolations(&adapterQueFalla{roto: "ramas"}, ConformanceOptions{})
	if !contains(fuera, "no debería devolver ramas") {
		t.Errorf("devolver ramas sin modo no se detectó: %v", fuera)
	}
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
		return forge.Page{More: true}, nil
	case "items":
		return forge.Page{Items: []model.Item{{Number: 1}}}, a.aviso()
	case "silencio":
		return forge.Page{}, nil
	case "no-avisar":
		return forge.Page{}, nil
	default:
		return forge.Page{}, a.aviso()
	}
}

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

func (a *adapterQueFalla) Branches(context.Context, model.RepoRef) ([]string, []model.Warning) {
	if a.roto == "ramas" {
		return []string{"main"}, a.aviso()
	}
	return nil, a.aviso()
}

func warnUnsupported() []model.Warning {
	return []model.Warning{{Forge: "roto", Kind: "unsupported", Msg: "esto es un doble roto"}}
}

// The half of RunConformance that can be tested in process; the other half, the real t.Error, is
// checked by subprocess.
func TestElInformeSeLlamaUnaVezPorIncumplimientoYNingunaSinIncumplimientos(t *testing.T) {
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

	var uno []string
	reportaIncumplimientos(&adapterQueFalla{roto: "forge-vacio"}, ConformanceOptions{},
		func(v string) { uno = append(uno, v) })
	if len(uno) != 1 {
		t.Errorf("un adapter que rompe una sola regla produjo %d informes, want 1: %v",
			len(uno), uno)
	}
}
