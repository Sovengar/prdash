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

// `main` was at 23.5% because it was the only place that decided the mode, loaded the config and
//built the TUI, and `main` cannot be called from a test.

// The case that forces it: `prdash worktrees remove --orphans`. With prdash's flags read first,
// --orphans is eaten as a prdash flag instead of the subcommand's.
func TestElSubcomandoSeMiraAntesQueLosFlags(t *testing.T) {
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
	if len(got.args) != 3 || got.args[0] != "remove" {
		t.Errorf("args = %q, want los tres tras el subcomando", got.args)
	}

	got, err = parseOpts([]string{"--print"})
	if err != nil {
		t.Fatalf("parseOpts de --print dio error: %v", err)
	}
	if got.mode != modePrint {
		t.Errorf("con --print dio mode %v, want modePrint", got.mode)
	}

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

	got, err = parseOpts([]string{"worktrees"})
	if err != nil {
		t.Fatal(err)
	}
	if got.sub != "list" {
		t.Errorf("worktrees sin sub dio %q, want list", got.sub)
	}

	got, err = parseOpts([]string{"worktrees", ""})
	if err != nil {
		t.Fatal(err)
	}
	if got.sub != "list" {
		t.Errorf("un sub vacío dio %q, want list", got.sub)
	}
}

// Not only that parsing works but that two calls do NOT contaminate each other: the global flag set
// is a singleton that flag.Parse mutates forever, and test order is not guaranteed.
func TestElParserDeFlagsNoEsElGlobal(t *testing.T) {
	got, err := parseOpts([]string{"--print"})
	if err != nil || got.mode != modePrint {
		t.Fatalf("--print dio %+v / %v", got, err)
	}

	got, err = parseOpts(nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.mode == modePrint {
		t.Error("una llamada sin --print heredó el modo de la anterior: el FlagSet es global")
	}

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

// An error rather than being ignored, because `prdash --prnt` is almost always a typo of --print.
func TestUnFlagDesconocidoEsUnErrorDeUso(t *testing.T) {
	_, err := parseOpts([]string{"--prnt"})
	if err == nil {
		t.Fatal("un flag desconocido dio nil: la errata de --print abriría la TUI en silencio")
	}
	if !strings.Contains(err.Error(), "prnt") {
		t.Errorf("el error %q no nombra el flag", err)
	}
	if _, err := parseOpts([]string{"--print=false"}); err != nil {
		t.Errorf("--print=false dio error: %v", err)
	}
	if _, err := parseOpts([]string{"-h"}); err == nil {
		t.Error("-h dio nil; con ContinueOnError es un error de parseo")
	}
}

// The default must be the TUI: a --print default would turn every flagless invocation into a
// network query.
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

// Five SetX calls in a row, and a missing one does NOT break compilation: the model comes up
// anyway and the failure only surfaces when the user presses the key.
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
	if w.ReviewLookup == nil {
		t.Error("wire no inyectó el registro de reviews: un cambio de base no avisaría de nada")
	}
	if w.ReviewRemover == nil {
		t.Error("wire no inyectó el removedor: un merge no borraría el worktree")
	}

	// The four review pieces are the SAME executor instance, which cannot be asserted by comparing
	//private fields and prevents "works in the tests, breaks in production".
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

func TestBuildAdaptersRespetaLoQueEstaHabilitado(t *testing.T) {
	cfg := config.Defaults()
	cfg.Forges.GitHub.Enabled = false
	cfg.Forges.GitLab.Enabled = false
	cfg.Forges.Bitbucket.Enabled = false
	if got := buildAdapters(cfg); len(got) != 0 {
		t.Errorf("con los tres deshabilitados dio %d adapters", len(got))
	}

	cfg = config.Defaults()
	cfg.Forges.GitLab.Enabled = false
	cfg.Forges.Bitbucket.Enabled = false
	got := buildAdapters(cfg)
	if len(got) != 1 || got[0].Forge() != "github" {
		t.Errorf("solo github dio %v", nombres(got))
	}
	if got[0].Host() != cfg.Forges.GitHub.Host {
		t.Errorf("el host del adapter es %q, want %q", got[0].Host(), cfg.Forges.GitHub.Host)
	}

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
	if got[1].Host() != cfg.Forges.GitLab.Host {
		t.Errorf("el host de gitlab es %q, want %q", got[1].Host(), cfg.Forges.GitLab.Host)
	}
	if got[2].Host() != "bitbucket.org" {
		t.Errorf("el host de bitbucket es %q, want bitbucket.org", got[2].Host())
	}
}

// The code is 2, not 1: "wrong usage" and "it failed" are different things and a script calling
// prdash needs to tell them apart.
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

func TestRunSinForgesAvanzaAvisaYNoAbreLaTui(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, config.DirName), 0o755); err != nil {
		t.Fatal(err)
	}
	// The key is `[forge...]` singular, which is what the parser reads. My first version used
	//`[forges...]` and the whole config was ignored.
	toml := "[forge.github]\nenabled = false\n\n[forge.gitlab]\nenabled = false\n" +
		"\n[forge.bitbucket]\nenabled = false\n"
	if err := os.WriteFile(filepath.Join(dir, config.DirName, config.FileName), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"--print"}, &stdout, &stderr)

	if code != 0 {
		t.Errorf("con --print y sin forges devolvió %d, want 0", code)
	}
	if !strings.Contains(stderr.String(), "no forges enabled") {
		t.Errorf("stderr = %q, want el aviso de que no hay forges", stderr.String())
	}
	for _, quiere := range []string{"Created by me (0)", "Review / assigned (0)", "Mentions (0)"} {
		if !strings.Contains(stdout.String(), quiere) {
			t.Errorf("stdout no trae %q: %q", quiere, stdout.String())
		}
	}
	if strings.Contains(stderr.String(), "abort") {
		t.Errorf("stderr = %q dice que abortó", stderr.String())
	}
}

func TestUnConfigIlegibleAvisaPeroNoAborta(t *testing.T) {
	// The directory is the CONFIG one, not the cache one. The first version pinned XDG_CACHE_HOME and
	//wrote config.toml there, so the config was never found.
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	if err := os.MkdirAll(filepath.Join(dir, config.DirName), 0o755); err != nil {
		t.Fatal(err)
	}
	roto := "enabled = [[[\n"
	if err := os.WriteFile(filepath.Join(dir, config.DirName, config.FileName), []byte(roto), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"--print"}, &stdout, &stderr)

	if code != 0 {
		t.Errorf("un config roto devolvió %d, want 0: config nunca aborta", code)
	}
	if !strings.Contains(stderr.String(), "config") {
		t.Errorf("stderr = %q, want el aviso del config", stderr.String())
	}
	if strings.Contains(stderr.String(), "no forges enabled") {
		t.Errorf("con un config roto salio el aviso de forges: %q", stderr.String())
	}
}

func nombres(as []forge.Adapter) []string {
	out := make([]string, len(as))
	for i, a := range as {
		out[i] = a.Forge()
	}
	return out
}
