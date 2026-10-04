package reporesolver

import "testing"

// hosts de prueba: un solo host conocido, que es lo que hace falta para separar "no es un
// remoto de git" de "el host no lo conozco".
func hostsDePrueba() map[string]string {
	return map[string]string{"github.com": "github"}
}

// TestUnEsquemaAusenteNoSeCuelaPorLaRamaDeSCP: una entrada que empieza por `://` no es un
// URL con esquema ni un SCP, y no se normaliza como si fuera una de las dos.
//
// Y esto es justo lo que dice el comentario del código: "lo que no se debe es dejar que
// una entrada así se cuele por la rama de SCP, que es lo que haría un `!= 0` en lugar de
// un `>= 0`". Y el comentario lleva dos informal escrito en el sitio donde el bug estaba,
// que es donde se documenta un bug.
//
// El caso que lo distingue tiene que llevar las dos cosas: `://` en la posición cero —para
// que el índice sea exactamente el que separa `>= 0` de `> 0`— y un `@` con algo antes
// —para que la rama de SCP lo acepte si llega—. `://git@github.com:acme/widget.git` las
// tiene las dos: por la rama de URL no pasa, porque `url.Parse` rechaza un esquema
// ausente, pero por la de SCP se resuelve a `github.com/acme/widget`.
//
// Y el resultado de esa fuga no es un repo equivocado cualquiera: es un repo que existe.
// Una ruta mal pegada de ese formato acaba apuntando a un repositorio real y el montaje
// trabaja sobre él.
func TestUnEsquemaAusenteNoSeCuelaPorLaRamaDeSCP(t *testing.T) {
	casos := []struct {
		raw  string
		nota string
	}{
		{"://git@github.com:acme/widget.git", "esquema ausente y forma SCP detrás: el caso " +
			"que la condición en `>= 0` estaba puesta para cerrar"},
		{"://github.com/acme/widget", "esquema ausente, sin SCP detrás"},
		{"://git@gitlab.com:grupo/proy", "otro host, para que no sea cosa de github"},
	}
	for _, c := range casos {
		ref, ok := ParseRemoteURL(c.raw, hostsDePrueba(), nil)
		if ok {
			t.Errorf("%q se normalizó a %+v: no debería. Sin esquema no es un URL de git, "+
				"y dejarlo pasar por la rama de SCP produce un repo que EXISTE y sobre el "+
				"que se trabajaría. %s", c.raw, ref, c.nota)
		}
	}

	// Y el lado bueno del mismo borde: un esquema en la posición uno o más entra por la
	// rama de URL y sí se normaliza. Con `> 0` esto también funciona, así que no
	// distingue; lo que distingue es el caso de arriba, y por eso se afirma el de abajo
	// para que la prueba sea de las dos mitades.
	for _, raw := range []string{"https://github.com/acme/widget", "ssh://git@github.com/acme/widget.git"} {
		ref, ok := ParseRemoteURL(raw, hostsDePrueba(), nil)
		if !ok {
			t.Errorf("%q no se normalizó y debería: tiene esquema", raw)
			continue
		}
		if ref.Host != "github.com" {
			t.Errorf("%q dio host %q, want github.com", raw, ref.Host)
		}
	}
}

// TestUnSCPConElUsuarioVacioNoEsUnRemoto: `@host:owner/repo` no es un remoto de git.
//
// Y la condición es `at > 0`, con lo que se pide que haya algo ANTES del arroba. Sin esa
// condición, `@host:a/b` daría `host = "host"` y se normalizaría a un repo que probablemente
// no existe.
//
// Y el caso del `@` al principio sin `:` detrás tampoco: `@host` no tiene dos puntos, así
// que cae por la comprobación del colon y no se normaliza.
func TestUnSCPConElUsuarioVacioNoEsUnRemoto(t *testing.T) {
	casos := []string{
		"@github.com:acme/widget",
		"@github.com",
		"git@github.com",       // sin ruta detrás
		"git@github.com:acme",  // un solo segmento de ruta
		"git@github.com:acme/", // segmento vacío al final
		"git@github.com:/acme", // segmento vacío al principio
	}
	for _, raw := range casos {
		if ref, ok := ParseRemoteURL(raw, hostsDePrueba(), nil); ok {
			t.Errorf("%q se normalizó a %+v y no debería: sin usuario, sin dos puntos o con "+
				"una ruta de un solo segmento no es un repo", raw, ref)
		}
	}

	// Y el SCP de verdad sí entra, con y sin `.git`.
	for _, raw := range []string{"git@github.com:acme/widget", "git@github.com:acme/widget.git"} {
		ref, ok := ParseRemoteURL(raw, hostsDePrueba(), nil)
		if !ok {
			t.Errorf("%q no se normalizó y debería", raw)
			continue
		}
		if ref.Host != "github.com" || ref.Project != "acme/widget" {
			t.Errorf("%q dio %+v", raw, ref)
		}
	}
}
