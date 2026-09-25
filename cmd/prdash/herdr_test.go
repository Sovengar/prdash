package main

import (
	"testing"

	"prdash/internal/config"
	"prdash/internal/forge/model"
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
	it, ok := reviewItem("https://github.com/acme/widget/pull/42", hosts)
	if !ok {
		t.Fatal("la URL de PR debería resolverse")
	}
	if it.Forge != "github" || it.Host != "github.com" || it.Ref.Project != "acme/widget" || it.Number != 42 {
		t.Fatalf("ítem = %+v", it)
	}
	if it.URL != "https://github.com/acme/widget/pull/42" {
		t.Fatalf("URL del ítem = %q", it.URL)
	}
	if _, ok := reviewItem("https://github.com/acme/widget/issues/1", hosts); ok {
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
// forge, host, proyecto y número de la URL.
func TestReviewItemKeepsIdentity(t *testing.T) {
	it, ok := reviewItem("https://gitlab.example.com/git/sub/proj/-/merge_requests/5", map[string]string{"gitlab.example.com": "gitlab"})
	if !ok {
		t.Fatal("la URL de MR debería resolverse")
	}
	want := model.With("gitlab", "gitlab.example.com", "git/sub/proj", 5)
	if it.ID() != want {
		t.Fatalf("ID = %+v, quiero %+v", it.ID(), want)
	}
}

func envGet(env map[string]string) func(string) string {
	return func(k string) string { return env[k] }
}
