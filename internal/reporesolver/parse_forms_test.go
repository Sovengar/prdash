package reporesolver

import (
	"testing"

	"prdash/internal/forge/model"
)

func TestParseRemoteURLClasificaLasFormasAntesDeNormalizar(t *testing.T) {
	hosts := map[string]string{"github.com": "github", "gitlab.example.com": "gitlab"}

	casos := []struct {
		nombre string
		raw    string
		ok     bool
	}{
		{"con esquema https", "https://github.com/acme/widget.git", true},
		{"con esquema ssh", "ssh://git@github.com/acme/widget.git", true},
		{"scp", "git@github.com:acme/widget.git", true},
		{"scp con usuario de una columna", "a@github.com:acme/widget.git", true},

		// "@" en la posición 0: usuario vacío. No es un remoto de git.
		{"scp con usuario vacío", "@github.com:acme/widget.git", false},
		{"scp con solo arroba", "@", false},
		{"arroba al principio y dos puntos", "@github.com:acme/widget", false},

		{"esquema vacío con dos puntos", "://github.com:acme/widget", false},
		{"esquema vacío y arroba", "://git@github.com/acme/widget", false},
		{"solo esquema vacío", "://", false},

		// "@" sin dos puntos detrás: no es SCP, es otra cosa.
		{"arroba sin dos puntos", "git@github.com", false},
		{"arroba y barra", "git@github.com/acme/widget", false},
		{"arroba y nada", "git@", false},

		// Ni "@" ni "://": un path local o cualquier otra cosa.
		{"path local", "/home/u/dev/widget", false},
		{"path relativo", "../widget", false},
		{"solo dos puntos", "acme:widget", false},
		{"solo una palabra", "widget", false},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got, ok := ParseRemoteURL(c.raw, hosts, nil)
			if ok != c.ok {
				t.Fatalf("ParseRemoteURL(%q) dio ok=%v, quiero %v (ref %+v)", c.raw, ok, c.ok, got)
			}
			// And when it says no it does not return half a repo: a half-filled RepoRef would paint as
			// real.
			if !ok && got != (model.RepoRef{}) {
				t.Errorf("ParseRemoteURL(%q) dijo que no pero devolvió %+v", c.raw, got)
			}
		})
	}
}

// The separators being looked for can appear BEFORE where they belong.
func TestParseRemoteURLNoSeConfundeConUnSeparadorEnElSitioEquivocado(t *testing.T) {
	hosts := map[string]string{"github.com": "github"}

	// Each entry with its exact result, because what is asserted is that the separator decides.
	casos := []struct {
		raw  string
		want model.RepoRef
	}{
		// With a scheme the scheme wins, and its `@` is the user, not an SCP separator.
		{"https://git@github.com/acme/widget.git",
			model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget", Owner: "acme", Name: "widget"}},
		// An `@` inside the path is legal in a git path and is NOT a user.
		{"https://github.com/acme/wi@get.git",
			model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/wi@get", Owner: "acme", Name: "wi@get"}},
		// With a port in the host: the port stays with the host and is not read as an SCP separator.
		{"ssh://git@github.com:22/acme/widget.git",
			model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget", Owner: "acme", Name: "widget"}},
		// A `:` inside an SCP path: the separator is the FIRST one and the rest are part of the path.
		{"git@github.com:acme/wi:get.git",
			model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/wi:get", Owner: "acme", Name: "wi:get"}},
	}

	for _, c := range casos {
		got, ok := ParseRemoteURL(c.raw, hosts, nil)
		if !ok {
			t.Errorf("ParseRemoteURL(%q) dijo que no, y es una de las tres formas", c.raw)
			continue
		}
		if got != c.want {
			t.Errorf("ParseRemoteURL(%q) dio %+v, want %+v: un separador de más no es un separador de menos",
				c.raw, got, c.want)
		}
	}
}
