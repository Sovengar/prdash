package tool

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// `Kind` es la función que decide qué hacer con un fallo de una CLI de forge, y es la más
// importante del paquete: de ella sale la respuesta a "¿esto se reintenta?", "¿esto
// bloquea el ítem?" y "¿esto es para siempre?". Todo el vocabulario de `warning` de
// prdash —`ratelimit`, `unmergeable`, `permission`, `auth`, `notfound`— pasa por aquí.
//
// Y hay un orden en el código que no es un detalle de implementación: es una tabla de
// precedencias entre señales que se contradicen. GitHub usa 403 tanto para permiso como
// para rate limit, así que un 403 no dice nada; el texto sí. Y el 409 tampoco distingue un
// rechazo de merge de un estado que se arregla refrescando. Por eso el texto se mira
// ANTES del código HTTP, y este fichero fija ese orden porque un cambio que lo invierta no
// da ningún error: solo cambia qué ítems se reintentan y cuáles no.

// TestElTextoMandaSobreElCodigoHTTP: las dos colisiones que resuelven el orden.
//
// GitHub: 403 por permiso y 403 por rate limit. Sin mirar el texto, un rate limit se
// clasifica como permiso —que es una denegación permanente— y la TUI deja de reintentar
// un ítem que solo necesitaba esperar.
//
// Y la segunda es peor: 409 es "unmergeable" (no tiene arreglo automático) y también
// "conflict" (se arregla refrescando). Confundirlos deja un PR sin merge para siempre.
func TestElTextoMandaSobreElCodigoHTTP(t *testing.T) {
	casos := []struct {
		nombre string
		err    error
		want   string
	}{
		{
			nombre: "403 con texto de rate limit",
			// Un error con 403 Y con el texto de rate limit. Si el código mandara,
			// saldría "permission".
			err:  errors.New("gh: API rate limit exceeded for user (HTTP 403)"),
			want: "ratelimit",
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
			// Y aquí lo evidente es lo contrario de lo que parece: "Head branch was
			// modified" es UNMERGEABLE, no conflicto, aunque el 409 y la palabra
			// "modified" empujen a conflicto. Está en la lista de `isUnmergeableText` a
			// propósito, y la primera versión de este caso esperaba "conflict" —
			// la suposición fácil de que el 409 gana. Si el orden se invirtiera, el
			// texto de GitHub más común para un merge rechazado pasaría a ser un
			// conflicto reintentable, que es reintentar un merge que va a fallar.
			nombre: "head branch was modified es unmergeable, no conflicto",
			err:    errors.New("gh: Head branch was modified. Review and try the merge again. (HTTP 409)"),
			want:   "unmergeable",
		},
		{
			// El otro 409 de verdad: un conflicto de estado que sí se arregla
			// refrescando.
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
	// Y el caso sin señal ninguna: sin código y sin texto reconocible, Kind tiene que
	// devolver algo que no sea un `kind` válido. Si devolviera "" y quien lo consume
	// tratara "" como "sin problema", un error desconocido pasaría por acción correcta.
	if got := Kind(errors.New("algo que no se parece a nada")); got == "" {
		t.Error("Kind dio vacio para un error sin senal: eso se lee como no-classificado")
	}
	// Y nil sí es "", que es el otro extremo: sin error no hay nada que clasificar.
	if got := Kind(nil); got != "" {
		t.Errorf("Kind(nil) dio %q, want vacio", got)
	}
}

// TestElAutorrechazoSeMiraAntesQueElCodigo: aprobar el propio PR es un caso aparte.
//
// Y el orden importa por el mismo motivo que los otros: un 422 en GitHub es
// "custom deployment rules" o "pull request author cannot approve". Si el 422 se
// clasifica como error genérico, la TUI reintenta la aprobación indefinidamente. Con el
// texto de autorrechazo se clasifica como permiso y deja de insistir, que es lo único
// sensato: no hay nada que reintentar.
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
			// El texto solo, sin el código HTTP. Es el caso que SÍ funcionaba antes de
			// arreglar el orden, y por eso no habría detectado el fallo: gh añade
			// "(HTTP 422)" a stderr, y el test sin él se llegaba al autorrechazo.
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
	// Y el texto se mira sin distinguir mayúsculas: las CLIs no son coherentes entre sí y
	// el mensaje puede venir de un idioma distinto al del status.
	if got := Kind(errors.New("Can NOT approve your OWN pull request")); got != "selfreview" {
		t.Errorf("con mayusculas dio %q, want selfreview", got)
	}
}

// TestElMensajeDelErrorTraeQueSeEjecutoYComoTermino: el texto que ve el usuario cuando
// una CLI falla.
//
// Y son tres datos, y cada uno responde a una pregunta: qué se ejecutó, con qué código, y
// que replied el stderr. El que se omite más a menudo es el tercero, y es el que hace que
// el mensaje sea accionable: "gh: exit status 1" no dice nada; "gh: not authenticated"
// sí.
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

// TestElErrorDesenvuelveLaCausa: la cadena de `errors` tiene que estar montada.
//
// Y el motivo es el mismo que en `gitcmd`: `Kind` recibe un `error` que envuelve un
// `*exec.ExitError`, y sin `Unwrap` el `Error` sería una caja opaca. El `ExitCode` está en
// el struct, pero el `errors.As` sobre la causa es lo que permite distinguir "el proceso no
// arrancó" de "el proceso se negó", que son dos avisos distintos.
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
	// Y con la causa puesta, el mensaje la conserva: un error sin diagnóstico es peor que
	// uno con un mensaje poorer.
	if !strings.Contains(e.Error(), "m") {
		t.Errorf("el mensaje perdió el texto propio: %q", e.Error())
	}
}

// TestHTTPStatusSoloAceptaUnCodigoDeTresDigitos: leer el código HTTP de un texto.
//
// Y el detalle es que tiene que ser EXACTAMENTE de tres dígitos. Un `\d+` solto leería
// "404" de "salto 4040" o leería el "1" de un número de línea, y un código de salida
// clasificado como 404 —notfound, un ítem que ya no existe— es un ítem que la TUI deja de
// reintentar para siempre.
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
		// Un código de dos dígitos dentro de algo más largo no es un código HTTP.
		{"error 5000 al cargar", 0},
	}
	for _, c := range casos {
		if got := HTTPStatus(c.entrada); got != c.want {
			t.Errorf("HTTPStatus(%q) dio %d, want %d", c.entrada, got, c.want)
		}
	}
}

// TestKindPorTextoCuandoNoHayCodigoHTTPQueLoDiga: las ramas del texto que el código no tapa.
//
// Y estas ramas son las que la tabla de `run_test.go` no alcanza, y hay una razón concreta
// para cada una: las dos se activan por un SINÓNIMO del mismo caso que ya está probado.
//
//   - `context deadline exceeded` sí cae en `timeout`, pero por la primera condición del
//     `case`. La palabra "timed out", que es como lo dice el cliente HTTP de GitLab, no
//     aparecía en ningún test y es media clasificación.
//   - `401 Unauthorized` cae en `auth` por el código HTTP, que se resuelve antes que el
//     texto. El texto —"authentication failed", "not logged"— nunca se leía, y es lo que dice
//     `gh` cuando el token está caducado sin código HTTP en el mensaje.
//
// Y lo que se fija es que el TEXTO manda cuando el código no dice nada. Sin código, el
// clasificador tiene poco más que el fraseo, así que las sinónimos son la red de seguridad:
// una palabra menos en la lista de hoy es un mensaje que se clasifica como `network`, que es
// la clase de la que no se hace nada.
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

	// Y lo que se deja FUERA a propósito: "could not authenticate with the token" cae en
	// `network`. Escribir el test para forzar ese synónimo sería ajustar el código al test, y
	// la lista de sinónimos se amplía cuando aparece un mensaje real de un forge, no porque
	// un test lo quiera. Lo que se fija aquí es el mecanismo: el texto se lee cuando el
	// código no dice nada, y lo lee comparando PALABRAS COMPLETAS ("authentication failed"),
	// no prefijos —por eso "authenticate" a secas no llega a ser lo mismo.
	if got := Kind(errors.New("could not authenticate with the token")); got != "network" {
		t.Errorf("un synónimo que no está en la lista dio %q; si se amplía la lista, se "+
			"amplía por un mensaje real y no por un test", got)
	}

	// Y el control: los dos casos con su código HTTP siguen saliendo por el código, no por
	// el texto. Si el texto se evaluara antes, un "422 validation error" con "conflict" en el
	// cuerpo —un mensaje que GitLab escribe— se clasificaría como conflicto y se reintentaría
	// un approve que va a fallar otra vez.
	if got := Kind(errors.New("HTTP 422 Unprocessable Entity: conflict on branch")); got != "validation" {
		t.Errorf("con 422 y la palabra conflict en el cuerpo dio %q, want validation: el "+
			"código manda, y reintentar un approve inválido gasta cuota sin arreglar nada", got)
	}
}

// TestKindPorTextoParaRateLimitYConflictoCuandoNoHayCodigoHTTP: las dos ramas que la tabla
// anterior no alcanzaba.
//
// Y son las que quedan sin cubrir, y por qué: `run_test.go` llega a `ratelimit` y a `conflict`
// por el CÓDIGO HTTP o por el texto que `isRateLimitText` reconoce, así que las ramas de texto
// del `switch` nunca se ejecutan. Mi lectura de las líneas del perfil las situaba en `timeout`
// y en `ratelimit`, y el caso de `timeout` ya estaba cubierto.
//
// Y las dos son sinónimos del vocabulario que el forge USA:
//
//   - "abuse" es lo que dice la API de GitHub cuando activa la detección de abuso, y el mensaje
//     puede llegar sin el código 429 si el `gh` lo formatea.
//   - "already merged" y "already closed" son lo que responde GitHub cuando se aprueba o
//     fusiona dos veces el mismo PR. Sin código no hay manera de saberlo salvo por el texto, y
//     la clase importa: un conflicto se reintenta con un refresco, un "ya fusionado" no.
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

	// Y la asimetría que hace que clasificar bien importa: un "already merged" clasificado como
	// `network` —su clase por defecto— no reintenta; uno clasificado como `conflict` hace que la
	// TUI ofrezca refrescar. Y un "abuse" como `network` no activa el backoff largo, que es lo
	// que evita seguir golpeando una API que ya dijo que para.
	for _, msg := range []string{"already merged", "abuse detection mechanism triggered"} {
		if got := Kind(errors.New(msg)); got == "network" {
			t.Errorf("%q cayó en network, que es la clase de la que no se hace nada", msg)
		}
	}
}

// TestKindConUnCodigoSueltoSinElPatronHTTP: las dos ramas que solo se alcanzan con el codigo
// desnudo.
//
// Y `HTTPStatus` solo reconoce un código cuando viene con su patrón —"HTTP nnn" o "(HTTP nnn)"—.
// Un mensaje con un "429" suelto NO es un código HTTP para el clasificador, así que pasa de largo
// y lo decide el `switch` de texto. Medido: `HTTPStatus("429")` devuelve 0 y `Kind("429")`
// devuelve `ratelimit`.
//
// Y lo mismo con el 422, que es el caso que sí hay que fijar: `kindForHTTP` traduce 422 a
// `validation`, así que la rama de texto del 422 del `switch` solo se alcanza sin patrón HTTP.
// Es el mismo criterio para los dos y por eso van juntos.
//
// Y por qué importa el 422 suelto: es lo que llega cuando `gh` formatea el error a su manera y
// el cuerpo JSON —donde va el motivo de verdad— no se parece a nada reconocible. Sin la rama,
// un 422 caería en `network` y la TUI no lo distinguiría de un problema de red.
func TestKindConUnCodigoSueltoSinElPatronHTTP(t *testing.T) {
	for msg, want := range map[string]string{
		"429":                 "ratelimit",
		"Error: 429":          "ratelimit",
		"bare 429 here":       "ratelimit",
		"bare 422 here":       "validation",
		"api said 422, sorry": "validation",
	} {
		// Y la precondición del caso: sin patrón HTTP, `HTTPStatus` no ve un código y no puede
		// ser el que decide. Si algún día `HTTPStatus`认识 un código suelto, este test pasa a
		// estar probando otra cosa, y el aserto lo dice.
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
