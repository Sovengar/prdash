package reporesolver

import "testing"

func hostsDePrueba() map[string]string {
	return map[string]string{"github.com": "github"}
}

// An entry starting with `://` is neither a schemed URL nor SCP, and is not normalised as if it
// were.
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

	// The good side of the same boundary: a scheme at position one or more goes through the URL branch
	//and is normalised. With `> 0` this would also work, which is why it is pinned.
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

// The condition is `at > 0`, which demands something before the @.
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
