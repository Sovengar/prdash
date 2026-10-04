package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"io"
	"prdash/internal/config"
	"prdash/internal/forge/model"
	"prdash/internal/review/plan"
	"prdash/internal/worktree"
)

// A path with a separator is looked up on disk and a bare name in the PATH, and that is because
// they are two different configurations.
func TestLaDisponibilidadDeUnBinarioDistingueLasTresCasas(t *testing.T) {
	for _, vacio := range []string{"", "   ", "\t\n"} {
		if binaryAvailable(vacio) {
			t.Errorf("binaryAvailable(%q) dio true: un pane sin comando no se lanza", vacio)
		}
	}

	dir := t.TempDir()
	ejecutable := filepath.Join(dir, "herramienta")
	if err := os.WriteFile(ejecutable, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !binaryAvailable(ejecutable) {
		t.Errorf("binaryAvailable(%q) dio false con el fichero delante", ejecutable)
	}
	subdir := filepath.Join(dir, "undir")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if binaryAvailable(subdir) {
		t.Error("binaryAvailable dio true para un directorio: un directorio no se ejecuta")
	}
	if binaryAvailable(filepath.Join(dir, "no-existe")) {
		t.Error("binaryAvailable dio true para una ruta inexistente")
	}

	if !binaryAvailable("sh") {
		t.Error("binaryAvailable(\"sh\") dio false y sh está en el PATH de todas partes")
	}
	if binaryAvailable("definitivamente-no-existe-este-binario") {
		t.Error("binaryAvailable dio true para un nombre que no está en el PATH")
	}
}

func TestHostsOfSoloMapeaLoConfiguradoYNoInventaForge(t *testing.T) {
	if len(hostsOf(config.Config{})) != 0 {
		t.Errorf("sin config dio %v, want mapa vacío", hostsOf(config.Config{}))
	}

	// The GitLab default is NOT gitlab.com: prdash's defaults point at an example self-managed GitLab.
	cfg := config.Defaults()
	hosts := hostsOf(cfg)
	if len(hosts) != 2 {
		t.Fatalf("con los dos hosts dio %d entradas: %v", len(hosts), hosts)
	}
	if hosts["github.com"] != "github" {
		t.Errorf("github.com -> %q", hosts["github.com"])
	}
	if hosts[cfg.Forges.GitLab.Host] != "gitlab" {
		t.Errorf("%q -> %q, want gitlab", cfg.Forges.GitLab.Host, hosts[cfg.Forges.GitLab.Host])
	}
	for h := range hosts {
		if h == "" {
			t.Error("el mapa tiene una entrada con host vacío")
		}
	}

	antes := cfg.Forges.GitLab.Host
	cfg.Forges.GitLab.Host = "git.intra.example"
	hosts = hostsOf(cfg)
	if hosts["git.intra.example"] != "gitlab" {
		t.Errorf("el host self-managed dio %q", hosts["git.intra.example"])
	}
	if _, sigue := hosts[antes]; sigue {
		t.Errorf("cambiar el host no quitó el anterior (%q sigue en el mapa)", antes)
	}

	cfg = config.Defaults()
	cfg.Forges.GitHub.Host = "mismo.example"
	cfg.Forges.GitLab.Host = "mismo.example"
	if len(hostsOf(cfg)) != 1 {
		t.Errorf("dos forges en el mismo host dieron %d entradas, want 1", len(hostsOf(cfg)))
	}
}

// The case that matters is Known:false WITH numbers: it arrives when the diffstat query fails and
// the figures default to zero.
func TestPrintDiffDistingueDesconocidoDeCero(t *testing.T) {
	casos := []struct {
		nombre string
		d      model.DiffStat
		want   string
	}{
		{"conocido normal", model.DiffStat{Known: true, Additions: 12, Deletions: 3}, "+12 -3"},
		{"solo añadidos", model.DiffStat{Known: true, Additions: 40}, "+40 -0"},
		{"solo borrados", model.DiffStat{Known: true, Deletions: 7}, "+0 -7"},
		{"conocido de cero", model.DiffStat{Known: true}, "+0 -0"},
		{"desconocido", model.DiffStat{Known: false}, "-"},
		{"cifras sin marcar como conocidas", model.DiffStat{Additions: 5, Deletions: 5}, "-"},
		{"cero sin marcar", model.DiffStat{}, "-"},
	}
	for _, c := range casos {
		got := printDiff(c.d)
		if got != c.want {
			t.Errorf("%s: printDiff dio %q, want %q", c.nombre, got, c.want)
		}
		if strings.TrimSpace(got) == "" {
			t.Errorf("%s: printDiff devolvió texto vacío", c.nombre)
		}
	}
}

// state has three possible values and only two are used in the listing, so the default is the
// notable one.
func TestElAuditorMarcaLoHuerfanoYDejaElRestoEnOk(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "wt")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	live := filepath.Join(dir, "live")
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatal(err)
	}

	entries := provisionerDePrueba(t, live).Audit(context.Background())
	if len(entries) != 0 {
		t.Errorf("un directorio sin worktrees dio %d entradas: %v", len(entries), entries)
	}

	entradas := provisionerDePrueba(t, repo).Audit(context.Background())
	for _, e := range entradas {
		if e.Path == "" {
			t.Errorf("una entrada sin Path: %+v", e)
		}
		if e.Orphan && e.Reason == "" {
			t.Errorf("una entrada huérfana sin motivo: %+v", e)
		}
		if !e.Orphan {
			t.Errorf("un worktree vivo marcado como huérfano: %+v", e)
		}
	}
}

// This is the dangerous half of the command, so what is tested is the negative.
func TestElBorradoRechazaLoQueNoEsWorktreePropio(t *testing.T) {
	dir := t.TempDir()
	ajeno := filepath.Join(dir, "otro-repo")
	if err := os.MkdirAll(ajeno, 0o755); err != nil {
		t.Fatal(err)
	}

	pr := provisionerDePrueba(t, dir)

	code := removeWorktreesWithin(pr, io.Discard, io.Discard, false, false, []string{ajeno}, time.Second)
	if code == 0 {
		t.Error("borrar un repo ajeno devolvió 0")
	}
	if _, err := os.Stat(ajeno); err != nil {
		t.Errorf("el repo ajeno no está: %v", err)
	}

	if code := removeWorktreesWithin(pr, io.Discard, io.Discard, false, false, []string{filepath.Join(dir, "nada")}, time.Second); code == 0 {
		t.Error("borrar una ruta inexistente devolvió 0")
	}

	antes, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if code := removeWorktreesWithin(pr, io.Discard, io.Discard, false, false, []string{"no-existe-en-absoluto"}, time.Second); code == 0 {
		t.Error("una ruta relativa inexistente devolvió 0")
	}
	despues, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(antes) != len(despues) {
		t.Errorf("el número de entradas del directorio cambió de %d a %d con un rechazo",
			len(antes), len(despues))
	}

	if code := removeWorktreesWithin(pr, io.Discard, io.Discard, false, true, []string{ajeno}, time.Second); code == 0 {
		t.Error("dry-run sobre un repo ajeno devolvió 0: dry-run no debería rechazar, " +
			"sino avisar y no tocar")
	}
	if _, err := os.Stat(ajeno); err != nil {
		t.Errorf("el dry-run borró el repo ajeno: %v", err)
	}
}

// The point is not that each delete has a deadline (the global one already did that) but that a slow
// item cannot spend the following ones'.
func TestElPresupuestoEsPorItemYNoGlobal(t *testing.T) {
	pr := provisionerDePrueba(t, t.TempDir())

	if code := removeWorktreesWithin(pr, io.Discard, io.Discard, false, false, []string{filepath.Join(t.TempDir(), "nada")},
		time.Millisecond); code == 0 {
		t.Error("una ruta inexistente con presupuesto minúsculo devolvió 0")
	}

	inicio := time.Now()
	removeWorktreesWithin(pr, io.Discard, io.Discard, true, true, nil, 2*time.Second)
	if elapsed := time.Since(inicio); elapsed > time.Second {
		t.Errorf("un dry-run de huérfanos tardó %s con nada que borrar", elapsed)
	}
}

// Wiring is where a new config field ends up half-wired.
func TestElBuildDelEjecutorUsaLaConfig(t *testing.T) {
	// Emptying `tools.agent` does NOT leave it empty: ToolArgs falls back to the tool's default name,
	//"opencode".
	cfg := config.Defaults()
	cfg.Tools.Tuicr = "/opt/tuicr --dark"
	cfg.Tools.Hunk = "hunk"
	cfg.Tools.Agent = ""

	tools := plan.Tools{
		Tuicr:  paneTool(cfg, "tuicr"),
		Hunk:   paneTool(cfg, "hunk"),
		Agent:  paneTool(cfg, "agent"),
		Editor: paneTool(cfg, "editor"),
	}

	if got := tools.Binary(plan.KindTuicr); got != "/opt/tuicr" {
		t.Errorf("el binario de tuicr salió %q", got)
	}
	if got := tools.Binary(plan.KindHunk); got != "hunk" {
		t.Errorf("el binario de hunk salió %q", got)
	}
	if got := tools.Binary(plan.KindAgent); got != "opencode" {
		t.Errorf("con tools.agent vacío dio %q, want el nombre por defecto", got)
	}
	if got := tools.Binary(plan.Kind("inventada")); got != "" {
		t.Errorf("una herramienta inexistente dio %q, want vacío", got)
	}
	if binaryAvailable(tools.Binary(plan.Kind("inventada"))) {
		t.Error("un argv vacío se declaró disponible")
	}

	ex := buildExecutor(config.Config{})
	if ex == nil {
		t.Fatal("buildExecutor devolvió nil")
	}
	if ex.Resolver == nil || ex.Worktrees == nil || ex.Herdr == nil {
		t.Errorf("el ejecutor tiene una pieza a nil: %+v", ex)
	}
	svc := buildSimulator(config.Config{}, ex)
	if svc == nil {
		t.Fatal("buildSimulator devolvió nil")
	}
	loc := simLocator{ex: ex}
	if _, ok := loc.Locate(model.Item{}); ok {
		t.Error("el locator encontró un clon sin review montado")
	}
}

func provisionerDePrueba(t *testing.T, dir string) worktree.Provisioner {
	t.Helper()
	return worktree.Select(nil, dir)
}
