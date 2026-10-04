package gitlab

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The runner's host is what is checked, not the struct's.
func TestNewCaenLosDefaultsYElHostSePoneEnElEntorno(t *testing.T) {
	// Sin nada.
	a := New("", "")
	if a.Host() != "gitlab.example.com" {
		t.Errorf("sin host dio %q, want el self-managed de ejemplo: un default que sea "+
			"gitlab.com mandaria las llamadas a un sitio que el usuario no configuro",
			a.Host())
	}
	if a.Forge() != ForgeName {
		t.Errorf("Forge dio %q, want %q", a.Forge(), ForgeName)
	}
	if a.runner == nil {
		t.Fatal("New dejo el runner a nil")
	}
	if got := a.runner.Extra; !contains(got, "GITLAB_HOST=gitlab.example.com") {
		t.Errorf("el runner no lleva GITLAB_HOST: %v", got)
	}
	if got := a.runner.Extra; !contains(got, "GLAB_NO_PROMPT=1") {
		t.Errorf("el runner no lleva GLAB_NO_PROMPT: sin el, glab puede pedir un token "+
			"en un proceso sin terminal y quedarse colgado: %v", got)
	}

	selfManaged := New("git.umane.example", "")
	if selfManaged.Host() != "git.umane.example" {
		t.Errorf("con host dio %q", selfManaged.Host())
	}
	if got := selfManaged.runner.Extra; !contains(got, "GITLAB_HOST=git.umane.example") {
		t.Errorf("el runner no lleva el host configurado: %v", got)
	}

	propio := New("", "/opt/glab")
	if got := propio.runner.Bin; got != "/opt/glab" {
		t.Errorf("con binario dio %q", got)
	}
	// Y el default.
	if got := New("", "").runner.Bin; got != "glab" {
		t.Errorf("sin binario dio %q, want glab", got)
	}
}

func TestAuthDistingueSesionYTokenMalo(t *testing.T) {
	dir := t.TempDir()

	// GitLab's format is "as <login>", not GitHub's "account <login>".
	good := scriptDe(t, dir, "glab-ok", `#!/bin/sh
echo "Logged in to git.umane.example as glab (GLAB_TOKEN)"
exit 0
`)
	a := New("git.umane.example", good)
	auth := a.Auth(context.Background())
	if !auth.OK {
		t.Fatalf("sesion buena dio OK=false: %s", auth.Reason)
	}
	if auth.Forge != ForgeName {
		t.Errorf("el estado no trae el nombre del forge: %+v", auth)
	}
	if auth.Login != "glab" {
		t.Errorf("login %q, want glab", auth.Login)
	}

	// The reason is the FIRST line of stderr, not the last and not all of them.
	bad := scriptDe(t, dir, "glab-ko", `#!/bin/sh
echo "401 Unauthorized" >&2
echo "detalle que no se ve" >&2
exit 1
`)
	auth = New("git.umane.example", bad).Auth(context.Background())
	if auth.OK {
		t.Error("sesion mala dio OK=true")
	}
	if strings.TrimSpace(auth.Reason) == "" {
		t.Error("sesion mala dio Reason vacío")
	}
	if !strings.Contains(auth.Reason, "401") {
		t.Errorf("el motivo %q no trae lo que dijo la CLI", auth.Reason)
	}
	if auth.Login != "" {
		t.Errorf("sesion mala trae login %q", auth.Login)
	}

	auth = New("git.umane.example", filepath.Join(dir, "no-existe")).Auth(context.Background())
	if auth.OK {
		t.Error("un binario inexistente dio OK=true")
	}
	if strings.TrimSpace(auth.Reason) == "" {
		t.Error("un binario inexistente dio Reason vacío")
	}

	// A good session with unexpected output: OK=true and an empty login, and that is the honest
	// answer.
	raro := scriptDe(t, dir, "glab-raro", "#!/bin/sh\necho 'algo distinto'\nexit 0\n")
	auth = New("git.umane.example", raro).Auth(context.Background())
	if !auth.OK {
		t.Errorf("una salida inesperada dio OK=false: %s", auth.Reason)
	}
	if auth.Login != "" {
		t.Errorf("una salida inesperada dio login %q, want vacío", auth.Login)
	}
}

// The regex points at "Logged in to <host> as <login>".
func TestElLoginSeSacaDelFormatoQueDiceGlab(t *testing.T) {
	casos := []struct {
		nombre string
		salida string
		want   string
	}{
		{"formato completo", "Logged in to git.umane.example as glab (GLAB_TOKEN)", "glab"},
		{"con puntos en el login", "as j.perez (GLAB_TOKEN)", "j.perez"},
		{"con guion en el login", "as mi-usuario (GLAB_TOKEN)", "mi-usuario"},
		{"sin la ruta del token", "Logged in to git.umane.example as glab", "glab"},
		{"active account", "  Active account: true\n  Logged in to x as glab (Y)", "glab"},
		{"sin la palabra as", "algo distinto", ""},
		{"vacio", "", ""},
		{"solo espacios", "   \n  ", ""},
		// Un paréntesis pegado al login: el corte lo deja entero.
		{"login con parentesis pegado", "as user(GLAB_TOKEN)", "user"},
	}
	for _, c := range casos {
		got := loginFromAuthStatus(c.salida)
		if got != c.want {
			t.Errorf("%s: dio %q, want %q", c.nombre, got, c.want)
		}
		// It never prints text containing the token, which is what would break the comparison.
		if strings.ContainsAny(got, "()") {
			t.Errorf("%s: el login trae parentesis: %q", c.nombre, got)
		}
	}
}

// scriptDe crea un script ejecutable y devuelve su ruta.
func scriptDe(t *testing.T, dir, nombre, cuerpo string) string {
	t.Helper()
	ruta := filepath.Join(dir, nombre)
	if err := os.WriteFile(ruta, []byte(cuerpo), 0o755); err != nil {
		t.Fatal(err)
	}
	return ruta
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
