package parse

import (
	"strings"
	"testing"
	"time"
)

// The three branches cover three shapes of input.
func TestJoinProjectNoPoneSeparadoresHuerfanos(t *testing.T) {
	casos := []struct {
		caso         string
		owner, piece string
		want         string
	}{
		{"los dos", "acme", "proy", "acme/proy"},
		{"solo owner", "acme", "", "acme"},
		{"solo nombre", "", "proy", "proy"},
		{"ninguno", "", "", ""},
		{"owner con barras", "grupo/sub", "proy", "grupo/sub/proy"},
		{"nombre con barras", "acme", "sub/proy", "acme/sub/proy"},
		{"los dos con barras", "a/b", "c/d", "a/b/c/d"},
		{"espacios", " acme ", " proy ", " acme / proy "},
	}
	for _, c := range casos {
		got := joinProject(c.owner, c.piece)
		if got != c.want {
			t.Errorf("%s: joinProject(%q, %q) dio %q, want %q", c.caso, c.owner, c.piece, got, c.want)
		}
		// The property that makes the function exist: there is never a slash at the start nor at the end.
		if strings.HasPrefix(got, "/") || strings.HasSuffix(got, "/") {
			t.Errorf("%s: dio %q, con una barra en un borde", c.caso, got)
		}
	}
}

// Splitting at the FIRST slash is the error.
func TestSplitProjectPartePorLaUltimaBarraYNoPorLaPrimera(t *testing.T) {
	casos := []struct {
		nombre string
		path   string
		wantA  string
		wantB  string
	}{
		{"una barra", "acme/proy", "acme", "proy"},
		{"dos barras", "grupo/sub/proy", "grupo/sub", "proy"},
		{"tres barras", "a/b/c/proy", "a/b/c", "proy"},
		{"sin barra", "proy", "", "proy"},
		{"barra inicial", "/grupo/proy", "grupo", "proy"},
		{"barra final", "grupo/proy/", "grupo", "proy"},
		{"barras en los dos bordes", "/grupo/proy/", "grupo", "proy"},
		{"solo barras", "///", "", ""},
		{"vacio", "", "", ""},
		// Trim removes slashes, NOT spaces, so the spaces stay inside both halves.
		{"barra con espacios alrededor", " grupo/sub /proy ", " grupo/sub ", "proy "},
	}
	for _, c := range casos {
		a, b := splitProject(c.path)
		if a != c.wantA || b != c.wantB {
			t.Errorf("%s: splitProject(%q) dio (%q, %q), want (%q, %q)",
				c.nombre, c.path, a, b, c.wantA, c.wantB)
		}
		// The right half never carries slashes: it is a project NAME, not a path.
		if strings.Contains(b, "/") {
			t.Errorf("%s: el nombre %q tiene barras", c.nombre, b)
		}
		// Both halves together rebuild the original without the middle slash.
		if a != "" && b != "" {
			if unido := joinProject(a, b); unido != strings.Trim(c.path, "/") {
				t.Errorf("%s: joinProject del resultado dio %q, no reconstruye %q",
					c.nombre, unido, strings.Trim(c.path, "/"))
			}
		}
	}
}

// What happens to an unparseable timestamp.
func TestParseTimeSoloAceptaRFC3339YElRestoDaCero(t *testing.T) {
	buenos := []string{
		"2026-04-01T12:00:00Z",
		"2026-04-01T12:00:00+02:00",
		"2026-04-01T12:00:00.123456789Z",
	}
	for _, s := range buenos {
		got := parseTime(s)
		if got.IsZero() {
			t.Errorf("parseTime(%q) dio el tiempo cero", s)
		}
		if got.Year() != 2026 {
			t.Errorf("parseTime(%q) dio el año %d", s, got.Year())
		}
	}

	malos := []struct {
		nombre string
		valor  string
	}{
		{"vacio", ""},
		{"solo espacios", "   "},
		{"solo fecha", "2026-04-01"},
		{"formato americano", "04/01/2026"},
		{"epoch en segundos", "1775044800"},
		{"epoch en milis", "1775044800000"},
		{"basura", "cuando sea"},
		{"mes invalido", "2026-13-01T12:00:00Z"},
		{"texto", "<time datetime=\"2026-04-01\"></time>"},
	}
	for _, c := range malos {
		if got := parseTime(c.valor); !got.IsZero() {
			t.Errorf("%s: parseTime(%q) dio %v, want el tiempo cero", c.nombre, c.valor, got)
		}
	}

	// The property that distinguishes it from time.Parse: the zero of a failed parse and the zero of
	//a value that is zero differ.
	if parseTime("") != parseTime("basura") {
		t.Error("el valor ausente y el ilegible dan tiempos distintos, y no deberían")
	}
	if parseTime("").After(time.Now()) {
		t.Error("el tiempo cero no es el mínimo")
	}
}

// The temptation is to split at the FIRST `!`.
func TestProjectFromRefQuitaElSufijoDelNumero(t *testing.T) {
	casos := []struct {
		nombre string
		ref    string
		want   string
	}{
		{"con numero", "grupo/proy!12", "grupo/proy"},
		{"sin numero", "grupo/proy", "grupo/proy"},
		{"con barras", "a/b/proy!3", "a/b/proy"},
		{"numero grande", "grupo/proy!1234", "grupo/proy"},
		{"vacio", "", ""},
		{"solo el signo", "!", ""},
		{"con espacios", "grupo/proy ! 12", "grupo/proy "},
	}
	for _, c := range casos {
		if got := projectFromRef(c.ref); got != c.want {
			t.Errorf("%s: projectFromRef(%q) dio %q, want %q", c.nombre, c.ref, got, c.want)
		}
	}
	// What comes out has NO `!`, because it is used as a project path.
	for _, c := range casos {
		if got := projectFromRef(c.ref); strings.Contains(got, "!") {
			t.Errorf("%s: quedo un signo de exclamacion en %q", c.nombre, got)
		}
	}
}

// The difference with the other extractor.
func TestSplitRepoURLSacaElProyectoSoloDelCaminoDeLaAPI(t *testing.T) {
	casos := []struct {
		nombre string
		url    string
		wantA  string
		wantB  string
	}{
		{"API de un repo", "https://api.github.com/repos/acme/proy", "acme", "proy"},
		{"API con subgrupos", "https://api.github.com/repos/grupo/sub/proy", "grupo/sub", "proy"},
		{"API self-hosted", "https://git.umane.example/api/v3/repos/acme/proy", "acme", "proy"},
		// A query string stays glued to the name, which is acceptable because the URL comes from us.
		{"con query", "https://api.github.com/repos/acme/proy?x=1", "acme", "proy?x=1"},
		{"url de clon", "https://github.com/acme/proy.git", "", ""},
		{"url html", "https://github.com/acme/proy", "", ""},
		// With no scheme the project still comes out: splitRepoURL only looks for the `/repos/` marker.
		{"sin esquema", "api.github.com/repos/acme/proy", "acme", "proy"},
		{"vacia", "", "", ""},
	}
	for _, c := range casos {
		a, b := splitRepoURL(c.url)
		if a != c.wantA || b != c.wantB {
			t.Errorf("%s: splitRepoURL(%q) dio (%q, %q), want (%q, %q)",
				c.nombre, c.url, a, b, c.wantA, c.wantB)
		}
	}

	// The property: with a project, the two halves recompose into a real path.
	a, b := splitRepoURL("https://api.github.com/repos/acme/proy")
	if got := joinProject(a, b); got != "acme/proy" {
		t.Errorf("el proyecto de la URL dio %q, que no se puede recomponer", got)
	}
}
