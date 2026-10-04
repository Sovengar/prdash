package github

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The two adapters look alike and are not interchangeable. One of the three differences is SILENT.

// The argv-vs-variable split is what is checked, because they are two different strategies.
func TestNewCaenLosDefaultsYElHostVaEnElArgvNoEnElEntorno(t *testing.T) {
	a := New("", "")
	if a.Host() != "github.com" {
		t.Errorf("sin host dio %q, want github.com", a.Host())
	}
	if a.Forge() != ForgeName {
		t.Errorf("Forge dio %q, want %q", a.Forge(), ForgeName)
	}
	if a.runner == nil {
		t.Fatal("New dejó el runner a nil")
	}
	// The prompt disabled, which is what stops gh from opening a browser to authenticate.
	if !has(a.runner.Extra, "GH_PROMPT_DISABLED=1") {
		t.Errorf("el runner no lleva GH_PROMPT_DISABLED: %v", a.runner.Extra)
	}
	// And NO host variable: on GitHub the host goes in the argv.
	for _, v := range a.runner.Extra {
		if strings.Contains(v, "HOST") {
			t.Errorf("el runner de GitHub lleva %q: el host va en el argv, que es lo unico "+
				"que `gh` acepta en todos los comandos", v)
		}
	}
	// Y el binario.
	if a.runner.Bin != "gh" {
		t.Errorf("sin binario dio %q, want gh", a.runner.Bin)
	}

	empresa := New("git.umane.example", "")
	if empresa.Host() != "git.umane.example" {
		t.Errorf("con host dio %q", empresa.Host())
	}
	// The exact argv is read from the script itself, which writes what it receives; inspecting the
	//adapter is not enough because gh accepts --hostname too.
	eco := filepath.Join(t.TempDir(), "gh-eco")
	log := filepath.Join(t.TempDir(), "argv")
	if err := os.WriteFile(eco, []byte("#!/bin/sh\necho \"$@\" > "+log+"\necho ok\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if auth := New("git.umane.example", eco).Auth(context.Background()); !auth.OK {
		t.Fatalf("sesion buena dio OK=false: %s", auth.Reason)
	}
	recibido, err := os.ReadFile(log)
	if err != nil {
		t.Fatalf("el script no llegó a ejecutarse: %v", err)
	}
	argv := strings.TrimSpace(string(recibido))
	if !strings.Contains(argv, "--hostname git.umane.example") {
		t.Errorf("la llamada no lleva el host en el argv: %q", argv)
	}
	if !strings.HasPrefix(argv, "auth status") {
		t.Errorf("la llamada no es `auth status`: %q", argv)
	}
	if cuenta(strings.Fields(argv), "git.umane.example") != 1 {
		t.Errorf("el host aparece %d veces en %q", cuenta(strings.Fields(argv), "git.umane.example"), argv)
	}

	if New("", "/opt/gh").runner.Bin != "/opt/gh" {
		t.Error("con binario propio no se usó")
	}
}

// The failure's reason is the CLI's text verbatim: it is the only thing that says why.
func TestAuthDistingueSesionYTokenMalo(t *testing.T) {
	dir := t.TempDir()

	good := script(t, dir, "gh-ok", `#!/bin/sh
echo "github.com"
echo "  ✓ Logged in to github.com account user (GH_TOKEN)"
exit 0
`)
	auth := New("github.com", good).Auth(context.Background())
	if !auth.OK {
		t.Fatalf("sesion buena dio OK=false: %s", auth.Reason)
	}
	if auth.Login != "user" {
		t.Errorf("login %q, want user", auth.Login)
	}
	if auth.Forge != ForgeName {
		t.Errorf("el estado no trae el nombre del forge: %+v", auth)
	}

	bad := script(t, dir, "gh-ko", `#!/bin/sh
echo "gh: To use GitHub CLI in a GitHub Actions workflow, set the GH_TOKEN" >&2
exit 4
`)
	auth = New("github.com", bad).Auth(context.Background())
	if auth.OK {
		t.Error("sesion mala dio OK=true")
	}
	if !strings.Contains(auth.Reason, "GH_TOKEN") {
		t.Errorf("el motivo %q no trae lo que dijo la CLI", auth.Reason)
	}
	if auth.Login != "" {
		t.Errorf("sesion mala trae login %q", auth.Login)
	}

	// A missing binary: OK=false with an error, not a panic.
	auth = New("github.com", filepath.Join(dir, "no-existe")).Auth(context.Background())
	if auth.OK {
		t.Error("un binario inexistente dio OK=true")
	}
	if strings.TrimSpace(auth.Reason) == "" {
		t.Error("un binario inexistente dio Reason vacío")
	}

	// Unexpected output with a zero exit: the session is valid and the login is empty; treating it
	// otherwise would invent a failure.
	raro := script(t, dir, "gh-raro", "#!/bin/sh\necho 'algo distinto'\nexit 0\n")
	auth = New("github.com", raro).Auth(context.Background())
	if !auth.OK {
		t.Errorf("una salida inesperada dio OK=false: %s", auth.Reason)
	}
	if auth.Login != "" {
		t.Errorf("una salida inesperada dio login %q", auth.Login)
	}
}

// The regex demands a space after "account": `gh auth status` has an "Active account: true"
//
//line with the same word.
func TestElLoginNoSeConfundeConLaLineaDeActiveAccount(t *testing.T) {
	casos := []struct {
		nombre string
		salida string
		want   string
	}{
		{"formato completo", "  ✓ Logged in to github.com account user (GH_TOKEN)", "user"},
		{"con puntos en el login", "account j.perez (GH_TOKEN)", "j.perez"},
		{"active account delante del login",
			"  Active account: true\n  ✓ Logged in to github.com account user (GH_TOKEN)", "user"},
		{"solo active account", "  Active account: true", ""},
		{"sin la palabra", "algo distinto", ""},
		{"vacio", "", ""},
		{"solo espacios", "   \n  ", ""},
		// A token glued with no space gives "user", not "user(GH_TOKEN)": the regex's character class
		//stops at the parenthesis.
		{"token pegado", "account user(GH_TOKEN)", "user"},
	}
	for _, c := range casos {
		got := loginFromAuthStatus(c.salida)
		if got != c.want {
			t.Errorf("%s: dio %q, want %q", c.nombre, got, c.want)
		}
		// It never returns "true", which is the value that would turn the viewer into a concrete
		// account.
		if got == "true" {
			t.Errorf("%s: devolvió \"true\" como login", c.nombre)
		}
	}
}

func script(t *testing.T, dir, nombre, cuerpo string) string {
	t.Helper()
	ruta := filepath.Join(dir, nombre)
	if err := os.WriteFile(ruta, []byte(cuerpo), 0o755); err != nil {
		t.Fatal(err)
	}
	return ruta
}

func has(xs []string, want string) bool { return cuenta(xs, want) > 0 }

func cuenta(xs []string, want string) int {
	n := 0
	for _, x := range xs {
		if x == want {
			n++
		}
	}
	return n
}
