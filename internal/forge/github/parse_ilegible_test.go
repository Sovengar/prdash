package github

import (
	"context"
	"strings"
	"testing"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
)

// Estas ramas son todas el mismo contrato: **qué pasa cuando el forge contesta algo que no
// se puede entender**. Un `gh` que devuelve un mensaje de rate limit donde se esperaba JSON, un
// token caducado, un proxy que devuelve HTML, una versión de `gh` que cambió el formato.
//
// Y la respuesta tiene que ser un AVISO clasificado, nunca un panic y nunca un ítem a medias.
// La razón es que el aviso es lo único que el usuario ve en un inbox que por lo demás sigue
// en pie: si en vez de eso el refresco muriera, un rate limit de GitHub vaciaría la pantalla
// y parecería que prdash se ha colgado.
//
// Y la clasificación importa más que el texto: `tool.Kind(err)` distingue permiso, conflicto,
// no found y temporal, y de eso depende que la TUI decida si reintenta, si avisa de que
// faltan permisos o si deja de preguntar. Un aviso clasificado como "parse" cuando el problema
// es de permisos hace que el usuario espere un rate limit que no es eso.

// ghQueDevuelve esto es `ghQueRegistra` sin registro, para los casos en los que lo que
// importa es la salida y no los argumentos.
func ghQueDevuelve(t *testing.T, cuerpo string) string {
	t.Helper()
	script, _ := ghQueRegistra(t, cuerpo)
	return script
}

// avisoUnico extrae el único aviso de una respuesta, fallando si no hay exactamente uno.
func avisoUnico(t *testing.T, warns []model.Warning, donde string) model.Warning {
	t.Helper()
	if len(warns) != 1 {
		t.Fatalf("%s: %d avisos, want 1: %+v", donde, len(warns), warns)
	}
	return warns[0]
}

// TestListarConSalidaQueNoEsJSONAvisaYNoSeRompe: el parseo del listado.
//
// Y el caso que se elige es HTML, que es lo que devuelve un proxy corporativo o un captive
// portal: no es JSON, no es un mensaje de `gh` reconocible, y llega con código de salida 0
// porque para el proceso la respuesta fue un éxito.
//
// Y ese último detalle es el que hace el caso bueno: `gh` saliendo con 0 no significa que la
// respuesta sirva, y un runner que mirara el código de salida se tragaría el HTML como si
// fuera un inbox vacío.
func TestListarConSalidaQueNoEsJSONAvisaYNoSeRompe(t *testing.T) {
	for _, c := range []struct {
		nombre string
		salida string
	}{
		{"html de un proxy", "cat <<'EOF'\n<html><body>Proxy Authentication Required</body></html>\nEOF"},
		// El truncado va por heredoc porque con `echo` el script tiene una comilla suelta y
		// lo que falla es el SHELL, no el parseo. Mi primera versión lo puso en `echo` y el
		// aviso salió de clase "network" —que era correcto: el binario había fallado— y
		// parecía un fallo de clasificación del código bajo prueba. El aserto no distinguía
		// "la salida era ilegible" de "el script estaba roto", que son dos fixture distintas.
		{"json truncado", "cat <<'JSON'\n{\"data\":{\"search\":{\"nodes\":[\nJSON"},
		{"json vacio", "printf ''"},
		{"array en vez de objeto", `echo '[]'`},
		{"campo que no es una lista", `echo '{"data":{"search":{"nodes":"no soy una lista"}}}'`},
	} {
		a := New("github.com", ghQueDevuelve(t, c.salida))
		page, warns := a.List(context.Background(), forge.Query{Section: model.SectionReview})

		// Sin panic, y sin ítems: una página vacía con un aviso es "no lo sé"; una página con
		// ítems inventados sería "mentira".
		if len(page.Items) != 0 {
			t.Errorf("%s: devolvió %d ítems de una salida ilegible", c.nombre, len(page.Items))
		}
		w := avisoUnico(t, warns, c.nombre)
		if w.Kind != "parse" {
			t.Errorf("%s: aviso de clase %q, want parse: un dato ilegible no es un rate "+
				"limit ni un problema de permisos", c.nombre, w.Kind)
		}
		// Y el aviso lleva la sección, que es lo que permite al inbox saber a qué columna
		// atribuírlo. Sin ella, el aviso aparecería en otra parte del pantallazo.
		if w.Section != model.SectionReview {
			t.Errorf("%s: el aviso no lleva la sección (lleva %q)", c.nombre, w.Section)
		}
		// Y el mensaje dice algo: un aviso vacío es indistinguible de no haber avisado.
		if strings.TrimSpace(w.Msg) == "" {
			t.Errorf("%s: el aviso quedó vacío", c.nombre)
		}
	}
}

// TestElAvisoDeParseoNoDiceQueNoHayNada: la clasificación, que es lo que decide la TUI.
//
// Y aquí está el daño real de clasificar mal. Un aviso de clase `notfound` o `empty` hace que
// el inbox muestre "no hay PRs" en vez de "no se pudo consultar", y el usuario cierra la
// sesión convencido de que le han mergeado todo. Es la clase de fallo en la que la interfaz
// afirma algo FALSO sobre el estado del mundo, y por eso el aserto es sobre la clase y no
// sobre que "haya aviso".
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
	// Y el mensaje tiene que ser reconocible como problema de datos: "parse" no está en el
	// texto, pero el texto lo dice el parseador.
	if !strings.Contains(strings.ToLower(w.Msg), "json") &&
		!strings.Contains(strings.ToLower(w.Msg), "parse") {
		t.Logf("el aviso no menciona el formato: %q", w.Msg)
	}
}

// TestLeerElEstadoDeUnPRConSalidaIlegibleAvisaYNoDevuelveUnItemFalso: `ItemState`.
//
// Y `ItemState` es más peligroso que `List` porque su resultado se aplica SOBRE el ítem que ya
// está en pantalla. Un parseo fallido que devolviera un `Item` cero dejaría un PR en blanco en
// la lista —sin título, sin autor, sin rama— y un PR con `HeadSHA` vacío es exactamente el
// fixture que hace que un approve se niegue después.
//
// Y el caso de GitLab equivalente —la MR que no aparece en la búsqueda— está en el fichero
// equivalente; los dos comparten el contrato de devolver el valor CERO junto al aviso.
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

// TestLaConversacionConSalidaIlegibleAvisaYNoPintaComentariosInventados: `Comments`.
//
// Y aquí la diferencia con los otros dos es que un comentario inventado es VISIBLE: aparece en
// la conversación con un autor y un texto que nadie escribió, y el usuario puede actuar sobre
// él. Por eso el aserto es que la lista queda vacía, no "que el total no cuadre".
//
// Y el total importa por separado: la conversation se recorta a los últimos N pero el total
// que dice el forge es lo que se enseña como "N de M". Un total a cero con comentarios
// presentes daría un "0 de 0" debajo de una conversación que sí tiene notas.
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
	// Y el aviso no lleva sección: la conversación se pinta en la ficha del ítem, no en una
	// columna del inbox, y un aviso con sección aparecería además en el inbox.
	if w.Section != "" {
		t.Errorf("el aviso de la conversación lleva sección %q; debería ir solo en la ficha", w.Section)
	}
}

// TestUnaFallaDelBinarioYUnaSalidaIlegibleNoSeConfunden: las dos clases, en la misma llamada.
//
// Y la comparación es el punto: las dos degradan a "no se pudo consultar" pero se clasifican
// distinto, y de esa clasificación depende que la TUI reintente o que avise de credenciales.
// Con el binario fallando es `tool.Kind(err)`; con el binario saliendo bien y la salida rota es
// `parse` fijo.
//
// Y el caso de GitHub que devuelve 404 con JSON de error en el cuerpo es el más delmás grave de: `gh` sale con 1, el runner lo ve como fallo y clasifica por el código, que es lo
// correcto para un 404 de verdad pero no para un rate limit que `gh` también reporta con 1.
func TestUnaFallaDelBinarioYUnaSalidaIlegibleNoSeConfunden(t *testing.T) {
	q := forge.Query{Section: model.SectionReview}

	// El binario falla: el aviso sale con la clase que le corresponda al fallo, NO con
	// "parse". Poner "parse" aquí sería decir "el forge no lo entiendo" cuando lo que pasó es
	// que no había red, y el backoff de reintento es otro.
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
