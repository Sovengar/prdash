package gitlab

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `New` es donde se decide de dónde sale el host, y por qué importa.
//
// Y la decisión es doble, y las dos mitades están en el mismo cuerpo:
//
//   - El binario vacío cae a "glab". Un runner con el binario vacío intentaría ejecutar la
//     cadena vacía, que falla con un error que no dice nada de qué CLI falta.
//   - El host vacío cae a "gitlab.example.com", NO al GitLab público. Y eso es una
//     decisión, no un default cómodo: los defaults del proyecto apuntan a un GitLab
//     self-managed, y un host vacío que se resolviera a gitlab.com mandaría las llamadas a
//     un sitio que el usuario no ha configurado.
//
// Y lo segundo —`GITLAB_HOST` como variable, no como `--hostname`— es la razón por la que el
// host tiene que fijarse aquí y no en cada llamada: las de `glab mr` no aceptan `--hostname`,
// así que la variable es el único sitio donde puede quedar.

// TestNewCaenLosDefaultsYElHostSePoneEnElEntorno: los tres valores.
//
// Y el host del runner es el que se comprueba, no el del struct: son dos copias del mismo
// dato, y si divergieran el adapter mandaría `GITLAB_HOST` de un host y `--hostname` de otro.
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
	// El runner tiene que llevar el binario y el host.
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

	// Con host self-managed.
	selfManaged := New("git.umane.example", "")
	if selfManaged.Host() != "git.umane.example" {
		t.Errorf("con host dio %q", selfManaged.Host())
	}
	if got := selfManaged.runner.Extra; !contains(got, "GITLAB_HOST=git.umane.example") {
		t.Errorf("el runner no lleva el host configurado: %v", got)
	}

	// Con binario explícito.
	propio := New("", "/opt/glab")
	if got := propio.runner.Bin; got != "/opt/glab" {
		t.Errorf("con binario dio %q", got)
	}
	// Y el default.
	if got := New("", "").runner.Bin; got != "glab" {
		t.Errorf("sin binario dio %q, want glab", got)
	}
}

// TestAuthDistingueSesionYTokenMalo: las dos mitades de `Auth`.
//
// Y lo que se comprueba con más cuidado es la del fallo: el `Reason` es el texto de la CLI,
// sin traducir ni recortar, porque es lo único que dice por qué falló. Un "no autenticado"
// de invención taparía las tres razones que importan —token caducado, sin token, `glab` no
// instalado— y las tres piden cosas distintas.
//
// Y el camino bueno trae el login, que es lo que evita una llamada extra para saber quién
// es el usuario. Un login vacío ahí no es un dato malo: es un parseo que no vio el formato
// que esperaba, y aun así la sesión vale.
func TestAuthDistingueSesionYTokenMalo(t *testing.T) {
	dir := t.TempDir()

	// Sesión buena: OK y login leído de la salida.
	// Y el formato de GitLab es "as <login>", no el "account <login>" de GitHub. Son dos
	// regex distintos y lo son a propósito: cada CLI dice lo suyo, y un regex común sería
	// un regex que no casa con ninguno de los dos formatos completos.
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

	// Sesión mala: OK=false y el motivo con texto.
	// El motivo es la PRIMERA línea de stderr, no la última ni todas: con dos líneas, lo
	// que se ve es la primera. Por eso el motivo de verifia el orden de este script, y
	// ponerlo al revés haría el test pasar por el motivo equivocado.
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
	// Y el login vacío en un fallo no molesta: lo que importa es el motivo.
	if auth.Login != "" {
		t.Errorf("sesion mala trae login %q", auth.Login)
	}

	// Binario inexistente: OK=false con el error de la CLI, no un panic. Es el caso de una
	// máquina sin `glab`, que es la degradación que el proyecto promete.
	auth = New("git.umane.example", filepath.Join(dir, "no-existe")).Auth(context.Background())
	if auth.OK {
		t.Error("un binario inexistente dio OK=true")
	}
	if strings.TrimSpace(auth.Reason) == "" {
		t.Error("un binario inexistente dio Reason vacío")
	}

	// Y sesión buena con una salida que no es la esperada: OK=true y login vacío. Es un caso
	// real —`glab` cambia el formato de `auth status` entre versiones— y tratarlo como
	// sesión inválida dejaría el inbox entero marcado como degradado.
	raro := scriptDe(t, dir, "glab-raro", "#!/bin/sh\necho 'algo distinto'\nexit 0\n")
	auth = New("git.umane.example", raro).Auth(context.Background())
	if !auth.OK {
		t.Errorf("una salida inesperada dio OK=false: %s", auth.Reason)
	}
	if auth.Login != "" {
		t.Errorf("una salida inesperada dio login %q, want vacío", auth.Login)
	}
}

// TestElLoginSeSacaDelFormatoQueDiceGlab: `loginFromAuthStatus` del adapter de GitLab.
//
// Y el regex apunta a "Logged in to <host> as <login> (<path>)". Es DISTINTO del de
// GitHub, que busca "account <login>", y lo es porque las dos CLIs dicen la cosa de forma
// distinta. La primera versión de este test usaba el formato de GitHub contra el regex de
// GitLab, y fallaba en los tres casos con formato.
//
// Y el regex exige un espacio y corta en la parenthesis, que es lo que impide que el login
// salga con el `(GLAB_TOKEN)` pegado. Un login con el token dentro no casa con ningún autor
// del inbox, así que ningún ítem aparecería como propio.
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
		// Y la línea que NO es el login, con una palabra parecida delante.
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
		// Y nunca sale un texto con el token dentro, que es lo que rompería la comparación
		// con los autores.
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

// contains mira si un slice de strings tiene un elemento exacto, para no importar slices
// solo para esto.
func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
