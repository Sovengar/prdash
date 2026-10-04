package github

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// El adapter de GitHub y el de GitLab se parecen mucho y no son intercambiables. Estos
// tests fijan las TRES diferencias, porque una de ellas es silenciosa: si el adapter de
// GitHub usara el formato de GitLab para leer el login, la sesión se detectaría bien —el
// código de salida es el mismo— y el viewer saldría vacío. El inbox arrancaría sin
// saber de quién es, y la columna de rol —que decide si approve va a funcionar— saldría
// vacía en todos los ítems sin ningún aviso.
//
// La segunda diferencia es el default del host: GitHub es `github.com` —el único que
// existe— y GitLab es un self-managed de ejemplo. Un host vacío en GitHub cayendo al
// GitLab sería el peor de los dos errores.
//
// Y la tercera es cómo se pasa el host a la CLI: GitHub acepta `--hostname` en todos los
// comandos, así que lo lleva en el argv; GitLab necesita la variable `GITLAB_HOST` porque
// `glab mr` no acepta el flag.

// TestNewCaenLosDefaultsYElHostVaEnElArgvNoEnElEntorno: el cableado.
//
// Y lo del argv contra la variable es lo que se comprueba, porque son dos estrategias
// distintas por una razón que no es estética: `gh` acepta `--hostname` en todos sus
// comandos, así que el host viaja en el argv y no necesita variable; `glab` NO lo acepta en
// `glab mr`, así que su host tiene que ir en `GITLAB_HOST` o la llamada sale al GitLab
// público.
//
// Poner la variable también en GitHub no rompería nada hoy, y por eso la diferencia se
// comprueba mirando que NO está: una variable sin usar es una configuración que parece
// estar haciendo algo y no hace nada.
func TestNewCaenLosDefaultsYElHostVaEnElArgvNoEnElEntorno(t *testing.T) {
	// Sin nada: el host público, que en GitHub es el único que existe.
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
	// El prompt deshabilitado, que es lo que evita que `gh` abra un navegador para
	// autenticarse en un proceso sin terminal.
	if !has(a.runner.Extra, "GH_PROMPT_DISABLED=1") {
		t.Errorf("el runner no lleva GH_PROMPT_DISABLED: %v", a.runner.Extra)
	}
	// Y NO lleva una variable de host: en GitHub el host va en el argv.
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

	// Con host self-managed: lo lleva, y `authArgs` lo pone en el argv.
	empresa := New("git.umane.example", "")
	if empresa.Host() != "git.umane.example" {
		t.Errorf("con host dio %q", empresa.Host())
	}
	// El argv exacto se ve desde el propio script, que escribe lo que recibe en un
	// fichero. Y mirar ahí en vez de inspeccionar el adapter importa: `gh` acepta
	// `--hostname` en todos sus comandos, así que el host tiene que llegar ahí y no en una
	// variable de entorno — y si se perdiera por el camino, el adapter seguiría teniendo
	// el host en su struct y no se vería nada.
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

	// Con binario propio.
	if New("", "/opt/gh").runner.Bin != "/opt/gh" {
		t.Error("con binario propio no se usó")
	}
}

// TestAuthDistingueSesionYTokenMalo: las dos mitades de `Auth`.
//
// Y el motivo del fallo es el texto de la CLI tal cual, sin traducir: es lo único que dice
// por qué falló, y las tres razones que importan —token caducado, sin token, `gh` no
// instalado— piden cosas distintas. Un "no autenticado" de invención las taparía.
//
// Y el camino bueno trae el login, que es lo que evita una llamada extra. Un login vacío
// ahí no es un dato malo: es un formato que no se reconoció, y aun así la sesión vale.
func TestAuthDistingueSesionYTokenMalo(t *testing.T) {
	dir := t.TempDir()

	// Sesión buena, con el formato de GitHub —"account", no "as"—.
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

	// Sesión mala: OK=false y el motivo con el texto de la CLI.
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

	// Binario inexistente: OK=false con error, no un panic. Es la degradación que el
	// proyecto promete cuando `gh` no está instalado.
	auth = New("github.com", filepath.Join(dir, "no-existe")).Auth(context.Background())
	if auth.OK {
		t.Error("un binario inexistente dio OK=true")
	}
	if strings.TrimSpace(auth.Reason) == "" {
		t.Error("un binario inexistente dio Reason vacío")
	}

	// Salida inesperada con código cero: la sesión vale y el login sale vacío. Tratarlo como
	// sesión inválida dejaría el inbox entero marcado como degradado.
	raro := script(t, dir, "gh-raro", "#!/bin/sh\necho 'algo distinto'\nexit 0\n")
	auth = New("github.com", raro).Auth(context.Background())
	if !auth.OK {
		t.Errorf("una salida inesperada dio OK=false: %s", auth.Reason)
	}
	if auth.Login != "" {
		t.Errorf("una salida inesperada dio login %q", auth.Login)
	}
}

// TestElLoginNoSeConfundeConLaLineaDeActiveAccount: el regex de GitHub.
//
// Y el regex exige un espacio después de "account" a propósito. La salida de `gh auth status`
// tiene dos líneas que contienen la palabra: la del login —"Logged in to github.com account
// user (GH_TOKEN)"— y "Active account: true". Sin el espacio exigido, el regex cogería "true"
// como login, y el viewer sería la cadena "true": el inbox aparecería como si todo fuera
// propio y nada fuera accionable.
//
// Y el caso de la línea del login ausente es el que hace que el fallo sea silencioso: el
// login sale vacío, la sesión vale, y no hay nada que avise.
func TestElLoginNoSeConfundeConLaLineaDeActiveAccount(t *testing.T) {
	casos := []struct {
		nombre string
		salida string
		want   string
	}{
		{"formato completo", "  ✓ Logged in to github.com account user (GH_TOKEN)", "user"},
		{"con puntos en el login", "account j.perez (GH_TOKEN)", "j.perez"},
		// La línea que NO es el login, con la palabra "account" delante. Sale "user", que es
		// lo que hay después, y no "true", que es lo que hay justo antes.
		{"active account delante del login",
			"  Active account: true\n  ✓ Logged in to github.com account user (GH_TOKEN)", "user"},
		// Y el caso en el que la línea del login falta: vacío, no "true".
		{"solo active account", "  Active account: true", ""},
		{"sin la palabra", "algo distinto", ""},
		{"vacio", "", ""},
		{"solo espacios", "   \n  ", ""},
		// Y el token pegado SIN espacio detrás del login. Sale "user", no
		// "user(GH_TOKEN)", porque la clase de caracteres del regex corta en el parentesis
		// además de en el espacio. La primera versión de este caso esperaba la cadena
		// entera, que es el fallo que el regex evita: un login con el token dentro no casa
		// con ningún autor del inbox y ningún ítem aparecería como propio.
		{"token pegado", "account user(GH_TOKEN)", "user"},
	}
	for _, c := range casos {
		got := loginFromAuthStatus(c.salida)
		if got != c.want {
			t.Errorf("%s: dio %q, want %q", c.nombre, got, c.want)
		}
		// Y nunca sale "true", que es el valor que convertiría al viewer en una cadena
		// concreta y rompería la comparación con los autores.
		if got == "true" {
			t.Errorf("%s: devolvió \"true\" como login", c.nombre)
		}
	}
}

// script crea un ejecutable y devuelve su ruta.
func script(t *testing.T, dir, nombre, cuerpo string) string {
	t.Helper()
	ruta := filepath.Join(dir, nombre)
	if err := os.WriteFile(ruta, []byte(cuerpo), 0o755); err != nil {
		t.Fatal(err)
	}
	return ruta
}

// has mira si un slice de strings tiene un elemento exacto.
func has(xs []string, want string) bool { return cuenta(xs, want) > 0 }

// cuenta cuenta cuántas veces aparece un elemento exacto.
func cuenta(xs []string, want string) int {
	n := 0
	for _, x := range xs {
		if x == want {
			n++
		}
	}
	return n
}
