package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/config"
	"prdash/internal/forge"
	"prdash/internal/review/executor"
)

// `main` estaba al 23,5% porque era el único sitio donde se decidía el modo, se cargaba la
// config y se armaba la TUI. `main` no se puede llamar desde un test, así que las tres
// cosas se movieron a `parseOpts`, `run` y `wire` — y este fichero es la razón por la que
// eso no es un refactor cosmético.
//
// La prueba de que el refactor sirvió no es que `main` tenga más cobertura: es que estas
// funciones se PUEDEN llamar. Antes, para comprobar que `prdash worktrees remove --orphans`
// no se colaba con `--print` había que ejecutar el binario y leer su salida.

// TestElSubcomandoSeMiraAntesQueLosFlags: el orden, y por qué no es arbitrario.
//
// Y el caso que lo justifica es `prdash worktrees remove --orphans`. Si los flags de
// prdash se miraran antes que el subcomando, `--orphans` lo leería el `FlagSet` de prdash
// —que no lo conoce y por tanto aborta con "flag provided but not defined"—, y el borrado
// de huérfanos dejaría de funcionar por completo.
//
// Y el simétrico también está fijado: `prdash --print worktrees` NO es el subcomando,
// porque el subcomando solo se reconoce en la primera posición.
func TestElSubcomandoSeMiraAntesQueLosFlags(t *testing.T) {
	// El caso que manda: los flags del subcomando no pasan por el parser de prdash.
	got, err := parseOpts([]string{"worktrees", "remove", "--orphans", "--dry-run"})
	if err != nil {
		t.Fatalf("parseOpts de worktrees dio error: %v", err)
	}
	if got.mode != modeWorktrees {
		t.Errorf("mode = %v, want modeWorktrees", got.mode)
	}
	if got.sub != "remove" {
		t.Errorf("sub = %q, want remove", got.sub)
	}
	// Y los args son los que quedan tras el subcomando, que es lo que despacha
	// `runWorktrees`: incluye el propio subcomando en la posición 0, porque es
	// `runWorktrees` quien vuelve a mirar el subcomando para despachar.
	if len(got.args) != 3 || got.args[0] != "remove" {
		t.Errorf("args = %q, want los tres tras el subcomando", got.args)
	}

	// Y `--print` solo es un flag de prdash, nunca un subcomando.
	got, err = parseOpts([]string{"--print"})
	if err != nil {
		t.Fatalf("parseOpts de --print dio error: %v", err)
	}
	if got.mode != modePrint {
		t.Errorf("con --print dio mode %v, want modePrint", got.mode)
	}

	// Y en posición no inicial NO es subcomando: `prdash --print worktrees` es un print
	// con un argumento sobra, no un worktrees.
	got, err = parseOpts([]string{"--print", "worktrees"})
	if err != nil {
		t.Fatalf("parseOpts dio error: %v", err)
	}
	if got.mode == modeWorktrees {
		t.Error("--print worktrees se tomó como subcomando: el subcomando solo cuenta en " +
			"la primera posición")
	}
	if got.mode != modePrint {
		t.Error("--print no se reconoció cuando no es la primera posición")
	}

	// Y el subcomando por defecto: sin sub, `list`. Es lo que hace que `prdash worktrees`
	// a secas liste en vez de decir que falta el subcomando.
	got, err = parseOpts([]string{"worktrees"})
	if err != nil {
		t.Fatal(err)
	}
	if got.sub != "list" {
		t.Errorf("worktrees sin sub dio %q, want list", got.sub)
	}

	// Y con un argumento vacío explícito, que es lo que deja un `[0]=""`.
	got, err = parseOpts([]string{"worktrees", ""})
	if err != nil {
		t.Fatal(err)
	}
	if got.sub != "list" {
		t.Errorf("un sub vacío dio %q, want list", got.sub)
	}
}

// TestElParserDeFlagsNoEsElGlobal: el seam, y la razón de existir.
//
// Y lo que se fija no es solo que el parseo funciona: es que dos llamadas NO se
// contaminan. El `flag` global es un singletón que `flag.Parse()` modifica para siempre,
// así que un test que lo usara dejaría el estado alterado para el resto de la suite —y el
// orden de los tests dentro de un binario no está garantizado, así que el fallo aparecería
// en otro test y en otra máquina—.
func TestElParserDeFlagsNoEsElGlobal(t *testing.T) {
	// Con el flag puesto.
	got, err := parseOpts([]string{"--print"})
	if err != nil || got.mode != modePrint {
		t.Fatalf("--print dio %+v / %v", got, err)
	}

	// Y sin él, DESPUÉS: si el estado global fuera el mismo, el `true` de antes se
	// quedaría pegado.
	got, err = parseOpts(nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.mode == modePrint {
		t.Error("una llamada sin --print heredó el modo de la anterior: el FlagSet es global")
	}

	// Y en cualquier orden, que es lo que un `t.Run` en paralelo destrozaría.
	for i := range 3 {
		if _, err := parseOpts([]string{"--print"}); err != nil {
			t.Fatal(err)
		}
		got, err = parseOpts([]string{"--print=false"})
		if err != nil {
			t.Fatal(err)
		}
		if got.mode == modePrint {
			t.Errorf("la repetición %d filtró estado entre llamadas", i)
			break
		}
	}
}

// TestUnFlagDesconocidoEsUnErrorDeUso: la negativa.
//
// Y el motivo de que sea un ERROR y no un "lo ignoro" está en que `prdash --prnt` es casi
// siempre una errata de `--print`. Sin el error, el programa abriría la TUI —que es lo que
// hace sin el flag— y el usuario creería que el flag no hace nada.
func TestUnFlagDesconocidoEsUnErrorDeUso(t *testing.T) {
	_, err := parseOpts([]string{"--prnt"})
	if err == nil {
		t.Fatal("un flag desconocido dio nil: la errata de --print abriría la TUI en silencio")
	}
	// Y el error menciona el flag, que es lo que hay que corregir.
	if !strings.Contains(err.Error(), "prnt") {
		t.Errorf("el error %q no nombra el flag", err)
	}
	// Y `--print=false` sí es válido: un flag booleano con valor explícito.
	if _, err := parseOpts([]string{"--print=false"}); err != nil {
		t.Errorf("--print=false dio error: %v", err)
	}
	// Y `-h` es un error de parseo con `ContinueOnError`, no un modo. Es el
	// comportamiento estándar de `flag`, y fijarlo evita que alguien lo trate como "el
	// modo ayuda" y construya un camino que nunca se recorre.
	if _, err := parseOpts([]string{"-h"}); err == nil {
		t.Error("-h dio nil; con ContinueOnError es un error de parseo")
	}
}

// TestSinArgsSePideLaTui: el default.
//
// Y el default tiene que ser la TUI. Un default de `--print` convertiría cada invocación
// sin flag en una consulta de red, que es lo contrario de lo que quiere alguien que abre el
// programa para ver su inbox.
func TestSinArgsSePideLaTui(t *testing.T) {
	for _, caso := range []struct {
		nombre string
		args   []string
	}{
		{"nada", nil},
		{"cadena vacia", []string{""}},
	} {
		got, err := parseOpts(caso.args)
		if err != nil {
			t.Fatalf("%s: %v", caso.nombre, err)
		}
		if got.mode != modeTUI {
			t.Errorf("%s dio mode %v, want modeTUI", caso.nombre, got.mode)
		}
	}
}

// TestElCableadoInyectaLasCincoPiezasDelModelo: `wire`.
//
// Y son cinco `SetX` seguidos, y uno que falte NO rompe la compilación: el modelo arranca
// igual y el fallo sale solo cuando el usuario pulsa la tecla correspondiente, con un aviso
// que dice "falta la dependencia" —que es un diagnóstico, no un fallo, y que no dice cuál
// de las cinco falta—.
//
// Y lo que más importa del cableado es que las cuatro piezas que hablan de reviews usan LA
// MISMA instancia del ejecutor. Cuatro instancias distintas funcionan en los tests —cada
// una tiene su propia memoria— y fallan en producción, cuando un merge monta un review que
// el simulador no reconoce y el aviso de base desfasada no aparece.
func TestElCableadoInyectaLasCincoPiezasDelModelo(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	cfg := config.Defaults()
	ex := buildExecutor(cfg)

	model := wire(cfg, nil, ex)
	w := model.Wiring()

	if w.Mounter == nil {
		t.Error("wire no inyectó el montador de reviews")
	}
	if w.Simulator == nil {
		t.Error("wire no inyectó el simulador: la acción de simular avisaría de que falta git-sim")
	}
	if w.Graphics == nil {
		t.Error("wire no inyectó la capa de gráficos: el popup saldría en half-blocks")
	}
	// Y el registro y el borrador de reviews, que son los dos que hacen que el merge avise
	// de la base desfasada y que borre el worktree. Sin ellos el merge funciona igual y no
	// avisa ni borra, que es el fallo silencioso.
	if w.ReviewLookup == nil {
		t.Error("wire no inyectó el registro de reviews: un cambio de base no avisaría de nada")
	}
	if w.ReviewRemover == nil {
		t.Error("wire no inyectó el removedor: un merge no borraría el worktree")
	}

	// Y las cuatro piezas de review son LA MISMA instancia del ejecutor, que es el aserto
	// que no se puede hacer comparando campos privados y que evita el fallo de "funciona
	// en los tests, falla en producción".
	//
	// Se comprueba con el tipo del mounter, que es el ejecutor: si `wire` hubiera
	// construido un segundo ejecutor para el simulador, el registro de reviews y el
	// simulador no verían el mismo review.
	if m, ok := w.Mounter.(*executor.Executor); !ok || m != ex {
		t.Errorf("el montador no es el ejecutor que se le pasó: %T", w.Mounter)
	}
	if _, ok := w.ReviewRemover.(*executor.Executor); !ok {
		t.Errorf("el removedor no es un ejecutor: %T", w.ReviewRemover)
	}
	if _, ok := w.ReviewLookup.(*executor.Executor); !ok {
		t.Errorf("el registro no es un ejecutor: %T", w.ReviewLookup)
	}
}

// TestBuildAdaptersRespetaLoQueEstaHabilitado: la parte del cableado que decide qué forges
// se hablan.
//
// Y los tres casos, porque los tres son distintos y el último es el que se confunde:
//
//   - Deshabilitado: no sale. Un forge deshabilitado que se consulta igual aparece en la
//     cabecera del inbox como degradado, y el usuario ve un aviso que no puede arreglar
//     porque no hay forma de desactivarlo desde el config.
//   - Solo uno habilitado: solo ese.
//   - Bitbucket: sale aunque no esté operativo, porque el comentario del código lo dice y
//     es lo correcto — un `bitbucket.enabled` con el token caducado tiene que verse como
//     degradado, no desaparecer.
func TestBuildAdaptersRespetaLoQueEstaHabilitado(t *testing.T) {
	// Ninguno.
	cfg := config.Defaults()
	cfg.Forges.GitHub.Enabled = false
	cfg.Forges.GitLab.Enabled = false
	cfg.Forges.Bitbucket.Enabled = false
	if got := buildAdapters(cfg); len(got) != 0 {
		t.Errorf("con los tres deshabilitados dio %d adapters", len(got))
	}

	// Solo GitHub.
	cfg = config.Defaults()
	cfg.Forges.GitLab.Enabled = false
	cfg.Forges.Bitbucket.Enabled = false
	got := buildAdapters(cfg)
	if len(got) != 1 || got[0].Forge() != "github" {
		t.Errorf("solo github dio %v", nombres(got))
	}
	// Y el HOST sale de la config, no de un valor fijo: un GitHub Enterprise es un host
	// distinto y las llamadas tienen que ir ahí.
	if got[0].Host() != cfg.Forges.GitHub.Host {
		t.Errorf("el host del adapter es %q, want %q", got[0].Host(), cfg.Forges.GitHub.Host)
	}

	// Los tres: el orden es github, gitlab, bitbucket, y eso es lo que fija el orden de
	// las secciones del inbox. Un orden distinto no rompe nada visible, pero mueve las
	// columnas de sitio.
	cfg = config.Defaults()
	cfg.Forges.Bitbucket.Enabled = true
	got = buildAdapters(cfg)
	want := []string{"github", "gitlab", "bitbucket"}
	gotNombres := nombres(got)
	if len(gotNombres) != len(want) {
		t.Fatalf("los tres dieron %v, want %v", gotNombres, want)
	}
	for i := range want {
		if gotNombres[i] != want[i] {
			t.Errorf("el orden de los adapters es %v, want %v", gotNombres, want)
			break
		}
	}
	// Y el de GitLab con su host self-managed, que es el que trae la config por defecto y
	// que no es gitlab.com.
	if got[1].Host() != cfg.Forges.GitLab.Host {
		t.Errorf("el host de gitlab es %q, want %q", got[1].Host(), cfg.Forges.GitLab.Host)
	}
	// Y bitbucket con el host público, que es el único que hay.
	if got[2].Host() != "bitbucket.org" {
		t.Errorf("el host de bitbucket es %q, want bitbucket.org", got[2].Host())
	}
}

// TestRunDevuelveElCodigoDeUsoSinEjecutarNada: el camino de error de `run`.
//
// Y el código es 2, no 1: "uso incorrecto" y "falló" son cosas distintas y un script que
// llama a prdash las distingue.
//
// Y stdout vacío: un uso incorrecto no imprime nada, para que un script que captura la
// salida no procese basura.
func TestRunDevuelveElCodigoDeUsoSinEjecutarNada(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	var stdout, stderr bytes.Buffer
	code := run([]string{"--no-existe"}, &stdout, &stderr)

	if code != 2 {
		t.Errorf("un flag desconocido devolvió %d, want 2 (uso incorrecto)", code)
	}
	if !strings.Contains(stderr.String(), "no-existe") {
		t.Errorf("stderr = %q, no nombra el flag", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("un uso incorrecto imprimio %q", stdout.String())
	}
}

// TestRunSinForgesAvanzaAvisaYNoAbreLaTui: el camino de "no hay nada configurado".
//
// Y no es fatal: con `--print` se sale con 0 y con el aviso. Abortar dejaría al usuario con
// un programa que no explica nada, y el aviso en stderr es lo que dice por qué el inbox
// viene vacío.
func TestRunSinForgesAvanzaAvisaYNoAbreLaTui(t *testing.T) {
	// Un config en blanco: sin forges habilitados.
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, config.DirName), 0o755); err != nil {
		t.Fatal(err)
	}
	// Y la clave es `[forge...]` en singular, que es lo que lee el parser. La primera
	// versión de este test usó `[forges...]` y el config se ignoraba entero: salía el
	// aviso de "no forges enabled" en el sitio EQUIVOCADO de la salida —en stdout— en vez
	// del de stderr, y parecía un fallo de `run`.
	toml := "[forge.github]\nenabled = false\n\n[forge.gitlab]\nenabled = false\n" +
		"\n[forge.bitbucket]\nenabled = false\n"
	if err := os.WriteFile(filepath.Join(dir, config.DirName, config.FileName), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}

	// Con `--print` se comprueba entero, sin abrir la TUI — que en un test sería un
	// cuelgue esperando teclado.
	var stdout, stderr bytes.Buffer
	code := run([]string{"--print"}, &stdout, &stderr)

	if code != 0 {
		t.Errorf("con --print y sin forges devolvió %d, want 0", code)
	}
	if !strings.Contains(stderr.String(), "no forges enabled") {
		t.Errorf("stderr = %q, want el aviso de que no hay forges", stderr.String())
	}
	// Y stdout con las TRES cabeceras de sección en cero. `inbox.Build` con cero
	// resultados sí produce secciones, y eso es lo que un script que parsea la salida
	// espera: si no imprimiera nada, no podría distinguir "no tienes nada" de "el
	// programa falló antes de imprimir".
	//
	// Y las tres con su nombre largo, que es la misma cadena que pinta la TUI y la
	// razón por la que el modo texto se usa para comprobar el pipeline contra la vista.
	for _, quiere := range []string{"Created by me (0)", "Review / assigned (0)", "Mentions (0)"} {
		if !strings.Contains(stdout.String(), quiere) {
			t.Errorf("stdout no trae %q: %q", quiere, stdout.String())
		}
	}
	if strings.Contains(stderr.String(), "abort") {
		t.Errorf("stderr = %q dice que abortó", stderr.String())
	}
}

// TestUnConfigIlegibleAvisaPeroNoAborta: la regla del proyecto.
//
// "Config nunca aborta": un config ausente o malformado degrada a defaults con un aviso. Y
// es una regla, no una cortesía —una TUI que no arranca porque el fichero de config tiene
// una coma de menos es peor que una TUI que arranca con los atajos por defecto—.
//
// Y el aviso va al stderr del LLAMADOR, que es lo que hace testeable este camino: con un
// `os.Stderr` fijo, el aviso iría a la salida del proceso y el test no podría verlo.
func TestUnConfigIlegibleAvisaPeroNoAborta(t *testing.T) {
	// Y el directorio es el de CONFIG, no el de cache. La primera versión fijó
	// `XDG_CACHE_HOME` —el otro, el de la cache de comentarios— y escribía el
	// `config.toml` ahí, de modo que `config.Load()` leía los defaults y no había ningún
	// aviso que ver. Un test que no falla por lo que dice que comprueba.
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	if err := os.MkdirAll(filepath.Join(dir, config.DirName), 0o755); err != nil {
		t.Fatal(err)
	}
	// TOML roto de forma inequívoca: un array donde tocaba un valor. La primera versión
	// usó una tabla sin cerrar, que algunos parsers aceptan como una clave con punto, y el
	// aviso no salía por una razón que no tenía que ver con el TOML roto.
	roto := "enabled = [[[\n"
	if err := os.WriteFile(filepath.Join(dir, config.DirName, config.FileName), []byte(roto), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"--print"}, &stdout, &stderr)

	if code != 0 {
		t.Errorf("un config roto devolvió %d, want 0: config nunca aborta", code)
	}
	// Y avisó. Con los defaults, que traen GitHub y GitLab habilitados, así que hay
	// adapters y no sale el aviso de "no forges".
	if !strings.Contains(stderr.String(), "config") {
		t.Errorf("stderr = %q, want el aviso del config", stderr.String())
	}
	if strings.Contains(stderr.String(), "no forges enabled") {
		t.Errorf("con un config roto salio el aviso de forges: %q", stderr.String())
	}
}

// nombres extrae la lista de forges de unos adapters, para que los fallos se lean.
func nombres(as []forge.Adapter) []string {
	out := make([]string, len(as))
	for i, a := range as {
		out[i] = a.Forge()
	}
	return out
}
