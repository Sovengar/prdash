package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"prdash/internal/cache"
	"prdash/internal/config"
	"prdash/internal/forge/model"
	"prdash/internal/worktree"
)

type provisionerQueFallaQuitar struct {
	quitados []string
	fallaCon error
	fallaEn  map[string]bool
}

func (p *provisionerQueFallaQuitar) Create(context.Context, worktree.Spec) (worktree.Worktree, error) {
	return worktree.Worktree{}, nil
}

func (p *provisionerQueFallaQuitar) RemoveIfClean(context.Context, string) (bool, string, error) {
	return false, "", nil
}

func (p *provisionerQueFallaQuitar) List(context.Context) []worktree.Worktree { return nil }

func (p *provisionerQueFallaQuitar) Audit(context.Context) []worktree.Entry { return nil }

func (p *provisionerQueFallaQuitar) Remove(_ context.Context, id string) error {
	p.quitados = append(p.quitados, id)
	if p.fallaEn[id] {
		return p.fallaCon
	}
	return nil
}

// The exit code is the whole assertion: 0, because the session keeps going.
func TestSinForgesEnElConfigSeAviadoYLaSesionSigue(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := os.MkdirAll(filepath.Join(xdg, "prdash"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(xdg, "prdash", "config.toml"), []byte(`
[forge.github]
enabled = false

[forge.gitlab]
enabled = false

[forge.bitbucket]
enabled = false
`), 0o644); err != nil {
		t.Fatal(err)
	}

	// `--print` is the only path of `run` testable without a TUI; the other ends in
	// tea.NewProgram(...).Run(), which needs a terminal.
	var stdout, stderr bytes.Buffer
	code := run([]string{"--print"}, &stdout, &stderr)

	if code != 0 {
		t.Errorf("sin forges el código de salida es %d, want 0: la sesión tiene que arrancar "+
			"igual para que se vea el motivo", code)
	}
	if !strings.Contains(stderr.String(), "no forges") {
		t.Errorf("stderr = %q, want el aviso de que no hay forges", stderr.String())
	}
	if strings.Contains(stdout.String(), "no forges") {
		t.Errorf("el aviso salió por stdout, que es lo que lee un script: %q", stdout.String())
	}
	// `--print` promises a table, so with no adapters it prints the headers: that is what tells
	// "nothing to do" from "no forges".
	if strings.TrimSpace(stdout.String()) == "" {
		t.Error("stdout está vacío: el modo --print debe imprimir la tabla aunque no haya " +
			"nada que enseñar")
	}
}

// The same pattern as Herdr's layout and for the same reason.
func TestUnBorradoQueFallaNoTiraElRestoDelLote(t *testing.T) {
	base, owned, _ := worktreeFixture(t)
	otra := filepath.Join(base, "prdash-pr-2")
	if err := escribirComoWorktree(t, owned, otra); err != nil {
		t.Fatal(err)
	}

	prov := &provisionerQueFallaQuitar{
		fallaCon: errors.New("workspace is not empty"),
		fallaEn:  map[string]bool{otra: true},
	}
	var stdout, stderr bytes.Buffer

	code := removeWorktreesWithin(prov, &stdout, &stderr, false, false,
		[]string{owned, otra, filepath.Join(base, "prdash-pr-3")}, 5*time.Second)

	if code != 1 {
		t.Errorf("código = %d con un borrado fallido, want 1", code)
	}
	// stdout names ONLY what was deleted: a failure there would make a script count a worktree that
	// is still on disk.
	if !strings.Contains(stdout.String(), "worktree removed: "+owned) {
		t.Errorf("stdout no dice que se borró %s:\n%s", owned, stdout.String())
	}
	if strings.Contains(stdout.String(), otra) {
		t.Errorf("stdout dice que se borró %s, que sigue en disco:\n%s", otra, stdout.String())
	}
	// stderr carries the cause, which is what tells whether retrying makes sense.
	errOut := stderr.String()
	if !strings.Contains(errOut, "workspace is not empty") {
		t.Errorf("stderr no trae la causa del fallo:\n%s", errOut)
	}
	if !strings.Contains(errOut, otra) {
		t.Errorf("stderr no nombra la ruta que falló:\n%s", errOut)
	}
	// TWO removeOne calls, not three: the third does not exist and the entry guard rejects it.
	if len(prov.quitados) != 2 {
		t.Errorf("se intentó borrar %d rutas, want 2: %v", len(prov.quitados), prov.quitados)
	}
	if strings.Contains(stdout.String(), "prdash-pr-3") {
		t.Errorf("stdout menciona una ruta que ni se intentó borrar:\n%s", stdout.String())
	}
	if !strings.Contains(errOut, "prdash-pr-3") {
		t.Errorf("stderr no explica el rechazo de la tercera ruta:\n%s", errOut)
	}
}

// It copies a real worktree's .git to another path in the same repo, to get a second worktree
// without paying for a git worktree add.
func escribirComoWorktree(t *testing.T, origen, destino string) error {
	t.Helper()
	gitdir, err := os.ReadFile(filepath.Join(origen, ".git"))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(destino, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(destino, ".git"), gitdir, 0o644)
}

// A branch worth having for what it does NOT do.
func TestElSubcomandoWorktreesNoConstruyeNiAdaptersNiExecutor(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := os.MkdirAll(filepath.Join(xdg, "prdash"), 0o755); err != nil {
		t.Fatal(err)
	}
	config := "[worktree]\ndir = " + filepath.Join(t.TempDir(), "worktrees") + "\n\n" +
		"[forge.github]\nenabled = false\n\n[forge.gitlab]\nenabled = false\n"
	if err := os.WriteFile(filepath.Join(xdg, "prdash", "config.toml"),
		[]byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", t.TempDir())

	var stdout, stderr bytes.Buffer
	code := run([]string{"worktrees", "list"}, &stdout, &stderr)

	if code != 0 {
		t.Errorf("el código de salida es %d, want 0: no hay worktrees y eso no es un fallo.\n%s",
			code, stderr.String())
	}
	// It says so instead of exiting with an empty map: the subcommand is a script and its output is
	// read.
	if !strings.Contains(stdout.String()+stderr.String(), "worktree") {
		t.Errorf("la salida no menciona worktrees:\nstdout: %s\nstderr: %s",
			stdout.String(), stderr.String())
	}
	// And it did NOT complain about forges, which is half the value of the branch.
	for _, sale := range []string{"no forges", "gh:", "glab:"} {
		if strings.Contains(stderr.String(), sale) {
			t.Errorf("el subcomando worktrees menciona %q: ha construido algo que no "+
				"necesita.\nstderr: %s", sale, stderr.String())
		}
	}
}

func TestElLocalizadorDeSimulacionEncuentraElRepoDeUnReviewMontado(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheDir)

	it := model.NewItem(model.RepoRef{
		Forge: "github", Host: "github.com", Project: "acme/widget",
		Owner: "acme", Name: "widget",
	}, 7)

	loc := simLocator{ex: buildExecutor(config.Defaults())}
	if _, ok := loc.Locate(it); ok {
		t.Fatal("sin review registrado el locator dijo que sabe localizar")
	}

	memo := cache.Memo{
		Version: 1,
		Reviews: map[string]cache.ReviewRecord{
			"github/github.com/acme/widget#7": {
				Repo:     "/clones/acme/widget.git",
				Worktree: "/wt/prdash-pr-7",
				Branch:   "prdash/pr-7",
				Label:    "prdash-pr-7",
			},
		},
	}
	ruta := filepath.Join(cacheDir, cache.DirName, cache.MemoFileName)
	if err := os.MkdirAll(filepath.Dir(ruta), 0o755); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(memo)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ruta, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	loc = simLocator{ex: buildExecutor(config.Defaults())}
	place, ok := loc.Locate(it)
	if !ok {
		t.Fatal("con el review registrado en la memoria el locator dijo que no: la clave " +
			"de la memoria no coincide con la que construye el ejecutor")
	}
	if place.Repo != "/clones/acme/widget.git" {
		t.Errorf("Repo = %q, want el clon registrado", place.Repo)
	}
	if place.Branch != "prdash/pr-7" {
		t.Errorf("Branch = %q, want la rama del review", place.Branch)
	}

	// A DIFFERENT item does not get confused with this one: the record is per item.
	otro := model.NewItem(it.Ref, 8)
	if _, ok := loc.Locate(otro); ok {
		t.Error("el locator encontró el review de otro ítem: el registro es por número")
	}
}
