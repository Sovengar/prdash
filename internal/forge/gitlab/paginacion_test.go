package gitlab

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
)

// TestConExactamenteUnaPaginaLlenaSigueHabiendoMas: si la API devuelve exactamente el
// tamaño de página, hay más.
//
// Y esto es una paginación, así que el caso de la frontera no es raro: la primera página de
// una lista de 50 menciones devuelve 50 y hay exactamente una más. O no la hay. Con un `>`
// en vez de un `>=`, esa lista se queda sin botón de "cargar más" y el usuario ve 50 MRs de
// 50 sin ninguna pista de que falten.
//
// Y el `total` de aquí no viene de una cabecera: es el número de filas que vinieron, que es
// como se hace una paginación ciega. No se puede saber si la página siguiente va vacía sin
// pedirla, y pedirla para nada es una llamada por cada refresco. La regla es "si la página
// vino llena, hay más", y "llena" es el tamaño exacto.
//
// Y los tres casos que hay que mirar son los tres: vacía, una fila menos que la página —que
// sí dice que no hay más— y exactamente una página llena, que es el borde de la condición.
func TestConExactamenteUnaPaginaLlenaSigueHabiendoMas(t *testing.T) {
	dir := t.TempDir()

	// `n` todos de la API de Todos. Solo cuentan los que son MR y mencionan a alguien,
	// pero la PAGINACIÓN mira el número de filas que vinieron, no cuántas se
	// entertaining al final: por eso hay un caso con filas que no cuentan.
	todos := func(n int, accion string) string {
		filas := make([]string, 0, n)
		for i := range n {
			filas = append(filas, `{"id":`+strconv.Itoa(i+1)+`,"action_name":"`+accion+
				`","target_type":"MergeRequest","updated_at":"2026-03-17T10:00:00Z",`+
				`"target":{"iid":`+strconv.Itoa(i+1)+`,"title":"mr `+strconv.Itoa(i+1)+
				`","web_url":"https://gitlab.com/g/p!`+strconv.Itoa(i+1)+
				`","state":"opened","source_branch":"feat/x","target_branch":"main",`+
				`"author":{"username":"alice"},`+
				`"references":{"full":"g/p!`+strconv.Itoa(i+1)+`","name":"p","namespace":"g"}}}`)
		}
		return "[" + strings.Join(filas, ",") + "]"
	}

	casos := []struct {
		n        int
		accion   string
		wantMore bool
		wantNext string
		nota     string
	}{
		{0, "mentioned", false, "", "página vacía"},
		{1, "mentioned", false, "", "una fila de una página de cincuenta"},
		{pageSize - 1, "mentioned", false, "",
			"una fila MENOS que la página: la siguiente va vacía y no se pide"},
		{pageSize, "mentioned", true, "2",
			"EXACTAMENTE una página llena: hay al menos una más por averiguar, y sin " +
				"botón el usuario ve 50 de 50 sin ninguna pista de que falten"},
		{pageSize + 1, "mentioned", true, "2", "llena y sobra"},
		{pageSize * 3, "mentioned", true, "2", "tres páginas de golpe"},
		{pageSize, "labeled", true, "2",
			"página llena de filas que NO son menciones: la paginación mira cuántas " +
				"filas vinieron, no cuántas son MR, porque el filtro es del cliente"},
	}

	for _, c := range casos {
		script := writeScript(t, dir, "glab", "#!/bin/sh\necho '"+todos(c.n, c.accion)+"'\n")
		a := New("gitlab.com", script)

		page, warns := a.todosList(context.Background(),
			forge.Query{Section: model.SectionMentions})
		if len(warns) != 0 {
			t.Errorf("%s: %d avisos: %+v", c.nota, len(warns), warns)
			continue
		}
		if page.More != c.wantMore {
			t.Errorf("%s: More=%v, want %v", c.nota, page.More, c.wantMore)
		}
		if page.Next != c.wantNext {
			t.Errorf("%s: Next=%q, want %q", c.nota, page.Next, c.wantNext)
		}
		// Y el cursor de la página siguiente avanza de uno en uno, que es lo que hace que
		// "cargar más" dos veces lleve a la misma página.
		if c.wantMore && page.Next != strconv.Itoa(2) {
			t.Errorf("%s: el cursor siguiente es %q y tiene que ser la página 2",
				c.nota, page.Next)
		}
	}

	// Y con el cursor ya en la página 3, el siguiente es el 4: el cursor es el número de
	// página y no un desplazamiento.
	script := writeScript(t, dir, "glab", "#!/bin/sh\necho '"+todos(pageSize, "mentioned")+"'\n")
	a := New("gitlab.com", script)
	page, warns := a.todosList(context.Background(),
		forge.Query{Section: model.SectionMentions, Cursor: "3"})
	if len(warns) != 0 {
		t.Fatalf("con cursor en la 3: %d avisos: %+v", len(warns), warns)
	}
	if page.Next != "4" {
		t.Errorf("con el cursor en la página 3 el siguiente es %q, want 4", page.Next)
	}
}
