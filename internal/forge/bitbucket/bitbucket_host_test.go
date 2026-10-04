package bitbucket

import "testing"

// This is not a detail: the adapter is for a host that is NOT in the default config.
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

	// Nothing beyond that can be asserted: the adapter is inert, everything answers "not implemented".
}
