package bitbucket

import (
	"context"
	"strings"
	"testing"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

// TestConformance comprueba que el adapter inerte cumple el contrato y responde
// "no soportado" en todo, sin realizar ninguna llamada de red.
func TestConformance(t *testing.T) {
	testutil.RunConformance(t, New("bitbucket.org"), testutil.ConformanceOptions{Unsupported: true})
}

// TestAuthDiceNoImplementadoYNoNoAutenticado: el motivo importa más que el `OK`.
//
// Y los dos `OK: false` son indistinguibles por el booleano, así que la diferencia está
// enteramente en `Reason` y es la que decide qué hace el operador:
//
//   - "no autenticado" se arregla retomando el token: es un problema de credenciales.
//   - "no implementado" no se arregla de ninguna forma, porque la versión del adapter no tiene
//     esa operación. Perseguir un token válido no cambia nada.
//
// Y el motivo equivocado envía a la persona a hacer trabajo inútil, y en el caso de un forge
// que el operador cree tener pero que la versión no soporta, a revisar credenciales durante
// horas antes de descubrir que el problema es otro.
//
// Y el `Forge` viene puesto: el estado de autenticación se pinta junto al nombre del forge, y
// sin él la cabecera muestra un aviso que no se sabe de quién es.
func TestAuthDiceNoImplementadoYNoNoAutenticado(t *testing.T) {
	auth := New("bitbucket.org").Auth(context.Background())

	if auth.OK {
		t.Error("Auth dice que Bitbucket está operativo: no lo está")
	}
	if auth.Forge != "bitbucket" {
		t.Errorf("Forge = %q, want bitbucket: el estado se pinta junto al nombre del forge",
			auth.Forge)
	}
	if !strings.Contains(strings.ToLower(auth.Reason), "not implemented") {
		t.Errorf("Reason = %q, y tiene que decir que NO ESTÁ IMPLEMENTADO y no que no hay "+
			"sesión: son problemas con arreglos distintos", auth.Reason)
	}
	// Y el motivo NO puede contener lo de "autenticado", que es lo que lleva a la persona a
	// buscar el token.
	if strings.Contains(strings.ToLower(auth.Reason), "auth") {
		t.Errorf("Reason = %q menciona autenticación: mandaría a revisar el token de algo "+
			"que no tiene arreglo", auth.Reason)
	}
}

// TestLasOperacionesNoSoportadasAvisanSinSalirAConsultarElForge: `unsupported`.
//
// Y el patrón es el mismo que en los demás adapters de un forge parcial: la operación no se
// consulta, y se devuelve un aviso de clase `unsupported` con la sección pedida.
//
// Y la clase importa: `unsupported` no es un fallo ni un problema de permisos, es que esta
// versión del forge no lo tiene. Marcarlo como cualquier otra cosa haría que la TUI lo tratara
// como reintentable o que pidiera credenciales.
func TestLasOperacionesNoSoportadasAvisanSinSalirAConsultarElForge(t *testing.T) {
	// Un adapter con un binario que falla si se le llama: el aviso tiene que salir del
	// propio camino no soportado, no de una consulta que falla.
	a := New("bitbucket.org")

	for _, c := range []struct {
		nombre string
		q      forge.Query
	}{
		{"listado de review", forge.Query{Section: model.SectionReview}},
		{"listado de menciones", forge.Query{Section: model.SectionMentions}},
		{"listado de propios", forge.Query{Section: model.SectionAuthored}},
	} {
		page, warns := a.List(context.Background(), c.q)
		if len(page.Items) != 0 {
			t.Errorf("%s: %d ítems de una operación no soportada", c.nombre, len(page.Items))
		}
		if len(warns) == 0 {
			t.Errorf("%s: sin aviso: el inbox parecería vacío sin explicación", c.nombre)
			continue
		}
		if warns[0].Kind != "unsupported" {
			t.Errorf("%s: clase %q, want unsupported", c.nombre, warns[0].Kind)
		}
		if warns[0].Section != c.q.Section {
			t.Errorf("%s: el aviso no lleva la sección pedida (lleva %q)", c.nombre, warns[0].Section)
		}
	}
}
