package tool

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// Kind decides what to do with a forge CLI's failure, and the whole package hangs on it: it answers
//"is this retried", "is this a permission" and "what does the user see".

// GitHub answers 403 for a permission AND for a rate limit, so without the text a rate limit is
// classified as a permission.
func TestElTextoMandaSobreElCodigoHTTP(t *testing.T) {
	casos := []struct {
		nombre string
		err    error
		want   string
	}{
		{
			nombre: "403 con texto de rate limit",
			err:    errors.New("gh: API rate limit exceeded for user (HTTP 403)"),
			want:   "ratelimit",
		},
		{
			nombre: "429 sin texto reconocible",
			err:    errors.New("gh: something went wrong (HTTP 429)"),
			want:   "ratelimit",
		},
		{
			nombre: "403 de permiso sin texto de rate limit",
			err:    errors.New("gh: Resource not accessible by integration (HTTP 403)"),
			want:   "permission",
		},
		{
			nombre: "409 con texto de no integrable",
			err:    errors.New("gh: Pull request is not mergeable (HTTP 409)"),
			want:   "unmergeable",
		},
		{
			// The obvious reading is the wrong one: "Head branch was modified" is UNMERGEABLE, not a
			//conflict, even though 409 and the word "modified" both point at conflict.
			nombre: "head branch was modified es unmergeable, no conflicto",
			err:    errors.New("gh: Head branch was modified. Review and try the merge again. (HTTP 409)"),
			want:   "unmergeable",
		},
		{
			nombre: "409 con texto de conflicto",
			err:    errors.New("glab: conflict: source branch has been updated (HTTP 409)"),
			want:   "conflict",
		},
		{
			nombre: "404",
			err:    errors.New("gh: Not Found (HTTP 404)"),
			want:   "notfound",
		},
		{
			nombre: "401",
			err:    errors.New("gh: Bad credentials (HTTP 401)"),
			want:   "auth",
		},
	}
	for _, c := range casos {
		if got := Kind(c.err); got != c.want {
			t.Errorf("%s: Kind dio %q, want %q", c.nombre, got, c.want)
		}
	}
	if got := Kind(errors.New("algo que no se parece a nada")); got == "" {
		t.Error("Kind dio vacio para un error sin senal: eso se lee como no-classificado")
	}
	if got := Kind(nil); got != "" {
		t.Errorf("Kind(nil) dio %q, want vacio", got)
	}
}

// The order matters for the same reason as the others: a 422 in GitHub is custom deployment
// validation.
func TestElAutorrechazoSeMiraAntesQueElCodigo(t *testing.T) {
	casos := []struct {
		nombre string
		err    error
		want   string
	}{
		{
			nombre: "no puedes aprobar tu propio PR",
			err:    errors.New("gh: Can not approve your own pull request (HTTP 422)"),
			want:   "selfreview",
		},
		{
			nombre: "el texto basta, sin HTTP",
			err:    errors.New("cannot approve your own pull request"),
			want:   "selfreview",
		},
		{
			nombre: "el texto de GitLab",
			err:    errors.New("cannot approve your own merge request"),
			want:   "selfreview",
		},
	}
	for _, c := range casos {
		if got := Kind(c.err); got != c.want {
			t.Errorf("%s: Kind dio %q, want %q", c.nombre, got, c.want)
		}
	}
	if got := Kind(errors.New("Can NOT approve your OWN pull request")); got != "selfreview" {
		t.Errorf("con mayusculas dio %q, want selfreview", got)
	}
}

func TestElMensajeDelErrorTraeQueSeEjecutoYComoTermino(t *testing.T) {
	casos := []struct {
		nombre string
		e      *Error
		want   string
	}{
		{
			nombre: "con codigo",
			e:      &Error{Bin: "gh", Args: []string{"pr", "view", "1"}, Msg: "not found", ExitCode: 1},
			want:   "gh pr view 1: not found (exit 1)",
		},
		{
			nombre: "sin codigo",
			e:      &Error{Bin: "glab", Args: []string{"mr", "list"}, Msg: "interrumpido"},
			want:   "glab mr list: interrumpido",
		},
		{
			nombre: "varios argumentos",
			e:      &Error{Bin: "gh", Args: []string{"api", "-X", "POST", "/x"}, Msg: "malo"},
			want:   "gh api -X POST /x: malo",
		},
		{
			nombre: "sin argumentos",
			e:      &Error{Bin: "gh", Msg: "algo"},
			want:   "gh : algo",
		},
	}
	for _, c := range casos {
		if got := c.e.Error(); got != c.want {
			t.Errorf("%s: Error() dio %q, want %q", c.nombre, got, c.want)
		}
		if strings.TrimSpace(c.e.Error()) == "" {
			t.Errorf("%s: mensaje vacio", c.nombre)
		}
	}
}

// Same reason as in gitcmd: Kind receives an error wrapping an *exec.ExitError.
func TestElErrorDesenvuelveLaCausa(t *testing.T) {
	if (&Error{Bin: "gh"}).Unwrap() != nil {
		t.Error("Unwrap devolvio algo sin causa")
	}
	if errors.Unwrap(&Error{Bin: "gh"}) != nil {
		t.Error("errors.Unwrap devolvio algo sin causa")
	}

	cmd := exec.Command("sh", "-c", "exit 9")
	err := cmd.Run()
	if err == nil {
		t.Fatal("esperaba que sh -c 'exit 9' fallara")
	}
	e := &Error{Bin: "gh", Args: []string{"x"}, Msg: "m", Err: err}
	if !errors.Is(e, err) {
		t.Error("errors.Is no llega a la causa")
	}
	var exit *exec.ExitError
	if !errors.As(e, &exit) {
		t.Fatal("errors.As no llega al *exec.ExitError")
	}
	if exit.ExitCode() != 9 {
		t.Errorf("ExitCode = %d, want 9", exit.ExitCode())
	}
	if !strings.Contains(e.Error(), "m") {
		t.Errorf("el mensaje perdió el texto propio: %q", e.Error())
	}
}

// EXACTLY three digits: a bare `\d+` would read "4040" or a version number as a status.
func TestHTTPStatusSoloAceptaUnCodigoDeTresDigitos(t *testing.T) {
	casos := []struct {
		entrada string
		want    int
	}{
		{"gh: Not Found (HTTP 404)", 404},
		{"HTTP 429", 429},
		{"(HTTP 503)", 503},
		{"sin código", 0},
		{"HTTP 40", 0},
		{"HTTP 4040", 0},
		{"", 0},
		{"error 5000 al cargar", 0},
	}
	for _, c := range casos {
		if got := HTTPStatus(c.entrada); got != c.want {
			t.Errorf("HTTPStatus(%q) dio %d, want %d", c.entrada, got, c.want)
		}
	}
}

// These are the branches run_test.go's table cannot reach, and the reason is specific.
func TestKindPorTextoCuandoNoHayCodigoHTTPQueLoDiga(t *testing.T) {
	for msg, want := range map[string]string{
		"timed out after 30s":                "timeout",
		"request timed out":                  "timeout",
		"authentication failed":              "auth",
		"not logged in: run `gh auth login`": "auth",
		"HTTP request timed out (no code)":   "timeout",
	} {
		if got := Kind(errors.New(msg)); got != want {
			t.Errorf("Kind(%q) = %q, want %q", msg, got, want)
		}
	}

	// Deliberately left out: "could not authenticate with the token" falls into `network`. Writing
	//the test to force that synonym would be fitting the code to the test.
	if got := Kind(errors.New("could not authenticate with the token")); got != "network" {
		t.Errorf("un synónimo que no está en la lista dio %q; si se amplía la lista, se "+
			"amplía por un mensaje real y no por un test", got)
	}

	// The control: the two cases that DO carry an HTTP code still go through the code. If the text ran
	//first, a "422 validation error" containing "conflict" would be misread.
	if got := Kind(errors.New("HTTP 422 Unprocessable Entity: conflict on branch")); got != "validation" {
		t.Errorf("con 422 y la palabra conflict en el cuerpo dio %q, want validation: el "+
			"código manda, y reintentar un approve inválido gasta cuota sin arreglar nada", got)
	}
}

func TestKindPorTextoParaRateLimitYConflictoCuandoNoHayCodigoHTTP(t *testing.T) {
	for msg, want := range map[string]string{
		"abuse detection mechanism triggered":   "ratelimit",
		"You have triggered an abuse detection": "ratelimit",
		"API rate limit exceeded for user":      "ratelimit",
		"already merged":                        "conflict",
		"already closed":                        "conflict",
		"Pull Request is already merged":        "conflict",
		"Merge conflict detected":               "conflict",
	} {
		if got := Kind(errors.New(msg)); got != want {
			t.Errorf("Kind(%q) = %q, want %q", msg, got, want)
		}
	}

	// The asymmetry is why classifying well matters: an "already merged" classed as `network` does not
	//retry; classed as `conflict` it does.
	for _, msg := range []string{"already merged", "abuse detection mechanism triggered"} {
		if got := Kind(errors.New(msg)); got == "network" {
			t.Errorf("%q cayó en network, que es la clase de la que no se hace nada", msg)
		}
	}
}

// HTTPStatus only recognises a code when it comes with its pattern —"HTTP 404" — so these two
// branches are only reachable with the bare code.
func TestKindConUnCodigoSueltoSinElPatronHTTP(t *testing.T) {
	for msg, want := range map[string]string{
		"429":                 "ratelimit",
		"Error: 429":          "ratelimit",
		"bare 429 here":       "ratelimit",
		"bare 422 here":       "validation",
		"api said 422, sorry": "validation",
	} {
		if code := HTTPStatus(msg); code != 0 {
			t.Errorf("el fixture %q SÍ tiene un código HTTP (%d): ya no prueba el switch de texto",
				msg, code)
			continue
		}
		if got := Kind(errors.New(msg)); got != want {
			t.Errorf("Kind(%q) = %q, want %q", msg, got, want)
		}
	}
}
