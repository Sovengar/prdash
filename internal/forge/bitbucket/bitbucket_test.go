package bitbucket

import (
	"context"
	"strings"
	"testing"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

func TestConformance(t *testing.T) {
	testutil.RunConformance(t, New("bitbucket.org"), testutil.ConformanceOptions{Unsupported: true})
}

// The two OK:false are indistinguishable by the boolean, so the REASON is what decides what the
// operator does.
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
	// The reason must NOT contain the word "authenticated": that is what sends the user off to
	// configure auth instead of looking at the project.
	if strings.Contains(strings.ToLower(auth.Reason), "auth") {
		t.Errorf("Reason = %q menciona autenticación: mandaría a revisar el token de algo "+
			"que no tiene arreglo", auth.Reason)
	}
}

// Same pattern as the other partially-implemented forges.
func TestLasOperacionesNoSoportadasAvisanSinSalirAConsultarElForge(t *testing.T) {
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
