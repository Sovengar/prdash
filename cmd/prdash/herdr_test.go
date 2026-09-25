package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"prdash/internal/config"
	"prdash/internal/forge/model"
	"prdash/internal/reporesolver"
	"prdash/internal/selection"
)

// TestReviewTargetPrefersArgument comprueba que una URL explícita manda sobre
// el contexto del plugin.
func TestReviewTargetPrefersArgument(t *testing.T) {
	env := map[string]string{
		"HERDR_PLUGIN_CLICKED_URL": "https://gitlab.example.com/g/p/-/merge_requests/3",
	}
	got, ok := reviewTarget([]string{"  https://github.com/acme/widget/pull/7  "}, envGet(env))
	if !ok || got != "https://github.com/acme/widget/pull/7" {
		t.Fatalf("reviewTarget = %q ok=%v", got, ok)
	}
}

// TestReviewTargetFromClickedURLEnv cubre el link handler: la URL clicada llega
// por HERDR_PLUGIN_CLICKED_URL.
func TestReviewTargetFromClickedURLEnv(t *testing.T) {
	got, ok := reviewTarget(nil, envGet(map[string]string{
		"HERDR_PLUGIN_CLICKED_URL": "https://github.com/acme/widget/pull/9",
	}))
	if !ok || got != "https://github.com/acme/widget/pull/9" {
		t.Fatalf("reviewTarget = %q ok=%v", got, ok)
	}
}

// TestReviewTargetFromContextJSON cubre el contexto JSON del plugin cuando no
// hay env discreta.
func TestReviewTargetFromContextJSON(t *testing.T) {
	got, ok := reviewTarget(nil, envGet(map[string]string{
		"HERDR_PLUGIN_CONTEXT_JSON": `{"invocation_source":"link_click","clicked_url":"https://gitlab.example.com/g/p/-/merge_requests/11"}`,
	}))
	if !ok || got != "https://gitlab.example.com/g/p/-/merge_requests/11" {
		t.Fatalf("reviewTarget = %q ok=%v", got, ok)
	}
}

// TestReviewTargetNone informa que sin URL no se puede montar nada.
func TestReviewTargetNone(t *testing.T) {
	if got, ok := reviewTarget(nil, envGet(nil)); ok || got != "" {
		t.Fatalf("reviewTarget = %q ok=%v, quiero vacío", got, ok)
	}
	if got := clickedURLFromContext("{no json"); got != "" {
		t.Fatalf("contexto roto no debería dar URL: %q", got)
	}
}

// TestReviewItemFromURL traduce la URL al ítem mínimo del montaje.
func TestReviewItemFromURL(t *testing.T) {
	hosts := map[string]string{"github.com": "github", "gitlab.example.com": "gitlab"}
	it, ok := reviewItem("https://github.com/acme/widget/pull/42", hosts, nil)
	if !ok {
		t.Fatal("la URL de PR debería resolverse")
	}
	if it.Forge != "github" || it.Host != "github.com" || it.Ref.Project != "acme/widget" || it.Number != 42 {
		t.Fatalf("ítem = %+v", it)
	}
	if it.URL != "https://github.com/acme/widget/pull/42" {
		t.Fatalf("URL del ítem = %q", it.URL)
	}
	if _, ok := reviewItem("https://github.com/acme/widget/issues/1", hosts, nil); ok {
		t.Fatal("una URL de issue no es un review")
	}
}

// TestHostsOfMapsConfiguredHosts usa la config para etiquetar el forge de cada
// host (incluido el GitLab self-managed).
func TestHostsOfMapsConfiguredHosts(t *testing.T) {
	cfg := config.Defaults()
	hosts := hostsOf(cfg)
	if hosts["github.com"] != "github" {
		t.Fatalf("hosts = %v", hosts)
	}
	if hosts[cfg.Forges.GitLab.Host] != "gitlab" {
		t.Fatalf("hosts = %v", hosts)
	}
}

// TestToolAvailabilityIdempotent comprueba que un argv vacío se reporta como no
// disponible (pane omitido) y un binario real como disponible.
func TestToolAvailabilityIdempotent(t *testing.T) {
	if binaryAvailable(nil) {
		t.Fatal("argv vacío no está disponible")
	}
	if binaryAvailable([]string{"prdash-bin-inexistente-xyz"}) {
		t.Fatal("un binario inexistente no está disponible")
	}
	if !binaryAvailable([]string{"go"}) {
		t.Fatal("go debería estar disponible en el entorno de test")
	}
}

// TestReviewItemKeepsIdentity comprueba que la identidad del ítem es la del
// forge, host, proyecto y número de la URL, normalizando el relative URL root
// de la instancia (prefix-aware) para no duplicarlo luego al clonar.
func TestReviewItemKeepsIdentity(t *testing.T) {
	hosts := map[string]string{"gitlab.example.com": "gitlab"}
	prefixes := map[string]string{"gitlab.example.com": "git"}
	it, ok := reviewItem("https://gitlab.example.com/git/sub/proj/-/merge_requests/5", hosts, prefixes)
	if !ok {
		t.Fatal("la URL de MR debería resolverse")
	}
	want := model.With("gitlab", "gitlab.example.com", "sub/proj", 5)
	if it.ID() != want {
		t.Fatalf("ID = %+v, quiero %+v", it.ID(), want)
	}
}

// TestReviewURLToCloneURLChain cubre el HIGH de punta a punta: la cadena
// link handler → resolver no debe duplicar el prefijo. Un host con subcarpeta
// produce la URL de clon con un único "/git/"; un host raíz/GitHub no cambia.
func TestReviewURLToCloneURLChain(t *testing.T) {
	gitlabCfg := config.Config{Forges: config.Forges{
		GitLab: config.GitLabConfig{Host: "gitlab.example.com", APIBase: "/git/api/v4/"},
	}}
	hosts := hostsOf(gitlabCfg)
	prefixes := clonePrefixesOf(gitlabCfg)

	cases := []struct {
		name string
		url  string
		want string
	}{
		{
			"gitlab subcarpeta",
			"https://gitlab.example.com/git/sub/proj/-/merge_requests/5",
			"https://gitlab.example.com/git/sub/proj.git",
		},
		{
			"github raíz",
			"https://github.com/acme/widget/pull/9",
			"https://github.com/acme/widget.git",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			it, ok := reviewItem(tc.url, hosts, prefixes)
			if !ok {
				t.Fatalf("no resolvió %q", tc.url)
			}
			got := reporesolver.CloneURL(it.Ref, prefixes[it.Host])
			if got != tc.want {
				t.Fatalf("CloneURL = %q, quiero %q", got, tc.want)
			}
		})
	}
}

// TestMountFromTargetFallsBackToSelection comprueba que la acción invocada sin
// URL (keybinding) monta el ítem que la TUI dejó seleccionado.
func TestMountFromTargetFallsBackToSelection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "selection.json")
	it := model.NewItem(model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget", Owner: "acme", Name: "widget"}, 7)
	if err := selection.Save(path, selection.FromItem(it, time.Now())); err != nil {
		t.Fatal(err)
	}

	var got model.Item
	code := mountFromTarget(config.Defaults(), nil, envGet(nil),
		func() (string, error) { return path, nil },
		func(_ config.Config, mounted model.Item) int { got = mounted; return 0 })

	if code != 0 {
		t.Fatalf("code = %d", code)
	}
	if got.ID() != it.ID() {
		t.Fatalf("montado = %+v, quiero %+v", got.ID(), it.ID())
	}
}

// TestMountFromTargetPrefersURL comprueba que una URL explícita sigue mandando
// sobre la selección persistida.
func TestMountFromTargetPrefersURL(t *testing.T) {
	var got model.Item
	code := mountFromTarget(config.Defaults(), []string{"https://github.com/acme/widget/pull/9"}, envGet(nil),
		func() (string, error) { return filepath.Join(t.TempDir(), "nope.json"), nil },
		func(_ config.Config, mounted model.Item) int { got = mounted; return 0 })

	if code != 0 || got.Number != 9 {
		t.Fatalf("code=%d item=%+v", code, got.ID())
	}
}

// TestMountFromTargetWithoutSelectionFailsClean comprueba que sin selección no
// se monta nada y no hay side effects.
func TestMountFromTargetWithoutSelectionFailsClean(t *testing.T) {
	called := false
	code := mountFromTarget(config.Defaults(), nil, envGet(nil),
		func() (string, error) { return filepath.Join(t.TempDir(), "nope.json"), nil },
		func(config.Config, model.Item) int { called = true; return 0 })

	if code == 0 {
		t.Fatal("sin selección debería fallar")
	}
	if called {
		t.Fatal("no debería montarse nada sin selección")
	}
}

// TestSelectedReviewRejectsStaleCorruptAndIncomplete cubre los estados que la
// acción debe rechazar con error claro.
func TestSelectedReviewRejectsStaleCorruptAndIncomplete(t *testing.T) {
	dir := t.TempDir()
	it := model.NewItem(model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget"}, 7)

	stale := filepath.Join(dir, "stale.json")
	if err := selection.Save(stale, selection.FromItem(it, time.Now().Add(-2*selection.MaxAge))); err != nil {
		t.Fatal(err)
	}
	if _, err := selectedReview(func() (string, error) { return stale, nil }, time.Now()); err == nil {
		t.Fatal("una selección obsoleta debería fallar")
	}

	corrupt := filepath.Join(dir, "corrupt.json")
	if err := os.WriteFile(corrupt, []byte("{no json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := selectedReview(func() (string, error) { return corrupt, nil }, time.Now()); err == nil {
		t.Fatal("una selección corrupta debería fallar")
	}

	incomplete := filepath.Join(dir, "incomplete.json")
	if err := os.WriteFile(incomplete, []byte(`{"forge":"github","host":"github.com"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := selectedReview(func() (string, error) { return incomplete, nil }, time.Now()); err == nil {
		t.Fatal("una selección incompleta debería fallar")
	}
}

func envGet(env map[string]string) func(string) string {
	return func(k string) string { return env[k] }
}
