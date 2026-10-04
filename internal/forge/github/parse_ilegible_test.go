package github

import (
	"context"
	"strings"
	"testing"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
)

// One contract across all of them: what happens when the forge answers something unreadable.

func ghQueDevuelve(t *testing.T, cuerpo string) string {
	t.Helper()
	script, _ := ghQueRegistra(t, cuerpo)
	return script
}

func avisoUnico(t *testing.T, warns []model.Warning, donde string) model.Warning {
	t.Helper()
	if len(warns) != 1 {
		t.Fatalf("%s: %d avisos, want 1: %+v", donde, len(warns), warns)
	}
	return warns[0]
}

// HTML is the case chosen, because that is what a corporate proxy returns.
func TestListarConSalidaQueNoEsJSONAvisaYNoSeRompe(t *testing.T) {
	for _, c := range []struct {
		nombre string
		salida string
	}{
		{"html de un proxy", "cat <<'EOF'\n<html><body>Proxy Authentication Required</body></html>\nEOF"},
		// The truncation goes through a heredoc: with echo the script has a dangling quote and what
		//fails is the SHELL, not the parsing.
		{"json truncado", "cat <<'JSON'\n{\"data\":{\"search\":{\"nodes\":[\nJSON"},
		{"json vacio", "printf ''"},
		{"array en vez de objeto", `echo '[]'`},
		{"campo que no es una lista", `echo '{"data":{"search":{"nodes":"no soy una lista"}}}'`},
	} {
		a := New("github.com", ghQueDevuelve(t, c.salida))
		page, warns := a.List(context.Background(), forge.Query{Section: model.SectionReview})

		// No panic and no items: an empty page with a warning is "I do not know", and a page with
		// items is data.
		if len(page.Items) != 0 {
			t.Errorf("%s: devolvió %d ítems de una salida ilegible", c.nombre, len(page.Items))
		}
		w := avisoUnico(t, warns, c.nombre)
		if w.Kind != "parse" {
			t.Errorf("%s: aviso de clase %q, want parse: un dato ilegible no es un rate "+
				"limit ni un problema de permisos", c.nombre, w.Kind)
		}
		if w.Section != model.SectionReview {
			t.Errorf("%s: el aviso no lleva la sección (lleva %q)", c.nombre, w.Section)
		}
		// Y el mensaje dice algo: un aviso vacío es indistinguible de no haber avisado.
		if strings.TrimSpace(w.Msg) == "" {
			t.Errorf("%s: el aviso quedó vacío", c.nombre)
		}
	}
}

// Misclassifying here is the real damage: a parse warning must not read as "you have no work".
func TestElAvisoDeParseoNoDiceQueNoHayNada(t *testing.T) {
	a := New("github.com", ghQueDevuelve(t, `echo 'no soy json'`))
	_, warns := a.List(context.Background(), forge.Query{Section: model.SectionReview})
	w := avisoUnico(t, warns, "list")

	for _, clase := range []string{"notfound", "empty", "ok"} {
		if w.Kind == clase {
			t.Errorf("una salida ilegible se clasificó como %q, y eso hace que la TUI afirme "+
				"algo falso sobre el inbox", clase)
		}
	}
	// The message has to be recognisable as a data problem: "parse" is not in the text.
	if !strings.Contains(strings.ToLower(w.Msg), "json") &&
		!strings.Contains(strings.ToLower(w.Msg), "parse") {
		t.Logf("el aviso no menciona el formato: %q", w.Msg)
	}
}

// ItemState is more dangerous than List, because its result is what the card paints.
func TestLeerElEstadoDeUnPRConSalidaIlegibleAvisaYNoDevuelveUnItemFalso(t *testing.T) {
	a := New("github.com", ghQueDevuelve(t, `echo 'respuesta rota'`))
	it, warns := a.ItemState(context.Background(),
		model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget"}, 7)

	w := avisoUnico(t, warns, "ItemState")
	if w.Kind != "parse" {
		t.Errorf("aviso de clase %q, want parse", w.Kind)
	}
	// Y el ítem devuelto es el valor CERO, no uno a medias.
	if it.ID() != (model.ID{}) || it.Number != 0 || it.HeadSHA != "" || it.Title != "" {
		t.Errorf("devolvió un ítem a medias en vez del valor cero: %+v", it)
	}
}

// The difference with the other two: an invented comment is visible text.
func TestLaConversacionConSalidaIlegibleAvisaYNoPintaComentariosInventados(t *testing.T) {
	a := New("github.com", ghQueDevuelve(t, `echo '{{{no es json'`))
	page, warns := a.Comments(context.Background(),
		model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget"}, 7)

	w := avisoUnico(t, warns, "Comments")
	if w.Kind != "parse" {
		t.Errorf("aviso de clase %q, want parse", w.Kind)
	}
	if len(page.Comments) != 0 {
		t.Errorf("devolvió %d comentarios de una salida ilegible", len(page.Comments))
	}
	if page.Total != 0 {
		t.Errorf("Total = %d con una salida ilegible", page.Total)
	}
	// The warning carries no section: the conversation is painted on the item's card, not in a
	// column.
	if w.Section != "" {
		t.Errorf("el aviso de la conversación lleva sección %q; debería ir solo en la ficha", w.Section)
	}
}

// Both degrade to the same thing and must be told apart.
func TestUnaFallaDelBinarioYUnaSalidaIlegibleNoSeConfunden(t *testing.T) {
	q := forge.Query{Section: model.SectionReview}

	// The binary fails: the warning gets the class that matches the failure, NOT "parse".
	caido := New("github.com", ghQueDevuelve(t, "echo 'gh: no such host' >&2\nexit 1"))
	_, warns := caido.List(context.Background(), q)
	w := avisoUnico(t, warns, "binario caído")
	if w.Kind == "parse" {
		t.Error("un binario que falla se clasificó como parse: el reintento usaría el " +
			"backoff equivocado y el aviso mentiría sobre la causa")
	}
	if strings.TrimSpace(w.Msg) == "" {
		t.Error("el aviso del binario caído está vacío")
	}

	// El binario funciona y la salida es basura: parse, fijo.
	roto := New("github.com", ghQueDevuelve(t, `echo 'no soy json'`))
	_, warns = roto.List(context.Background(), q)
	if got := avisoUnico(t, warns, "salida rota").Kind; got != "parse" {
		t.Errorf("una salida ilegible dio clase %q, want parse", got)
	}
}
