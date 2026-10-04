package bitbucket

import "testing"

// TestElHostPorDefectoSoloSeAplicaSinHost: el host por defecto es el de Bitbucket, y un
// host propio se respeta.
//
// Y esto no es un detalle: el adapter es para un host, y el host decide dónde se busca cada
// cosa —la API, la URL del PR, el remoto del worktree—. Con la condición al revés, un host
// propio se sustituye por `bitbucket.org` y el adapter entero opera contra el sitio
// equivocado: las llamadas irían a una instancia donde ese repositorio no existe, y el
// error sería "no encontrado" de un repositorio que sí existe.
//
// Y el caso que lo distingue tiene que llevar las dos mitades: el host vacío, que es lo
// que el suelo cubre, y un host propio, que es lo que la condición tiene que dejar pasar.
func TestElHostPorDefectoSoloSeAplicaSinHost(t *testing.T) {
	casos := []struct {
		entrada, want string
		nota          string
	}{
		{"", "bitbucket.org", "sin host: el de Bitbucket, que es el único sitio al que " +
			"habla este adapter"},
		{"bb.ejemplo.com", "bb.ejemplo.com", "host propio: Bitbucket self-hosted, que " +
			"es justamente para lo que existe el parámetro"},
		{"bitbucket.org", "bitbucket.org", "el de por defecto escrito a mano: da igual"},
		{"192.168.1.10:7990", "192.168.1.10:7990", "host con puerto, que es lo que " +
			"pasa en una instancia detrás de un proxy"},
	}
	for _, c := range casos {
		if got := New(c.entrada).Host(); got != c.want {
			t.Errorf("New(%q).Host() = %q, want %q. %s", c.entrada, got, c.want, c.nota)
		}
	}

	// Y no se puede afirmar nada más allá, porque este adapter es inerte: no tiene
	// repoURL ni ItemState que usen el host, todo devuelve un "no implementado". El host
	// se propaga por el constructor y no se usa más, con lo que `Host()` es toda la
	// superficie observable. Decir lo contrario sería inventar un aserto que no puede
	// fallar.
}
