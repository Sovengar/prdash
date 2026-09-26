// Tests de la conversación dentro del panel de detalle: qué se pinta, cuánto, en
// qué orden y cuándo se consulta al forge.
package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

// conv es un atajo para montar una conversación de pruebas.
func conv(author, body string) model.Comment {
	return model.Comment{Author: author, Body: body}
}

// modelWithComments deja un modelo con un ítem seleccionado y su conversación ya
// resuelta, para poder pintar la ficha sin pasar por el sondeo.
func modelWithComments(t *testing.T, height int, list []model.Comment, total int) Model {
	t.Helper()
	it := mkItem("github", "github.com", "acme/widget", "Add widget", 42, "")
	gh := ghAdapter()
	gh.Conversations = map[string]forge.CommentPage{
		testutil.ItemKey("acme/widget", 42): {Comments: list, Total: total},
	}
	m := newTestModel(t, gh)
	m.width, m.height = 160, height
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{it}, false))
	return withConversation(t, m, it, forge.CommentPage{Comments: list, Total: total})
}

// withConversation resuelve la conversación de un ítem sin esperar al tick, que es
// lo que hace el sondeo cuando su goroutine publica el resultado.
func withConversation(t *testing.T, m Model, it model.Item, p forge.CommentPage) Model {
	t.Helper()
	id := it.ID()
	if _, asked := m.comments[id]; !asked {
		m.comments[id] = &commentState{} // marcada como pedida, como hace el sondeo
	}
	return send(t, m, commentsMsg{id: id, page: p})
}

// detailText es el texto plano del panel de detalle, que es lo que lee el usuario.
func detailText(t *testing.T, m Model) string {
	t.Helper()
	rows := m.layout().detailLines
	if rows <= 0 {
		t.Fatal("el layout no ha reservado filas para el detalle: falta WindowSizeMsg")
	}
	return stripANSI(strings.Join(m.detailLines(mustSelected(t, m), true, rows), "\n"))
}

func mustSelected(t *testing.T, m Model) model.Item {
	t.Helper()
	it, ok := m.selected()
	if !ok {
		t.Fatal("no hay ítem seleccionado")
	}
	return it
}

// waitFor espera a que se cumpla una condición. La consulta de comentarios va en
// una goroutine, así que contar llamadas sin esperar es medir el planificador, no
// el código.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("no se cumplió a tiempo: %s", what)
}

// detailLabels son las etiquetas de campo de la ficha. El URL va aparte porque no
// está en la rejilla, así que se comprueba por separado.
var detailLabels = []string{
	"Item:", "Forge:", "Author:", "Source:", "Target:", "Number:",
	"State:", "Checks:", "Diff:", "Updated:", "Review:", "Role:",
}

// rowWithField devuelve el índice de la primera fila que lleva `label`, o -1.
func rowWithField(rows []string, label string) int {
	for i, l := range rows {
		if strings.Contains(stripANSI(l), label) {
			return i
		}
	}
	return -1
}

// TestURLGetsItsOwnFullWidthRow: el URL sale de la rejilla a una fila a todo el
// ancho. En media columna se leen 40 caracteres de una URL de 80 y queda un resto
// inútil, y una URL que no se puede copiar entera no sirve para nada, que es para
// lo que está en la ficha.
func TestURLGetsItsOwnFullWidthRow(t *testing.T) {
	const long = "https://umane.emeal.nttdata.com/git/APPCTTI/vsocial/backend/vsocial-api-accions/-/merge_requests/1234"
	it := mkItem("gitlab", "gitlab.example.com", "APPCTTI/vsocial/backend/vsocial-api-accions", "MR", 1234, "")
	it.Author = "someone-else"
	it.URL = long

	gh := ghAdapter()
	gl := &testutil.FakeAdapter{ForgeName: "gitlab", HostName: "gitlab.example.com"}
	m := newTestModel(t, gh, gl)
	m.width, m.height = 160, 45
	m = send(t, m, page(1, "gitlab", "gitlab.example.com", model.SectionReview, model.ReviewRequested, []model.Item{it}, false))
	m = withConversation(t, m, it, forge.CommentPage{
		Comments: []model.Comment{conv("alice", "ok for me")}, Total: 1,
	})

	rows := m.detailLines(it, true, m.layout().detailLines)
	urlRow := rowWithField(rows, "URL:")
	if urlRow < 0 {
		t.Fatalf("no hay fila de URL:\n%s", strings.Join(rows, "\n"))
	}

	// En la fila del URL no cabe medio panel de campos al lado, así que el valor
	// tiene que haber salido entero, sin recortar.
	plain := stripANSI(rows[urlRow])
	if !strings.Contains(plain, long) {
		t.Errorf("el URL debería salir entero, no recortado:\n%s", plain)
	}
	// Y no comparte fila con ningún otro campo: eso es lo que distingue "fila propia
	// a ancho completo" de "la primera celda de la rejilla".
	for _, label := range detailLabels {
		if strings.Contains(plain, label) {
			t.Errorf("la fila del URL comparte fila con %q:\n%s", label, plain)
		}
	}
	// Sale antes de los comentarios, que van debajo de toda la ficha.
	iComment := rowWithField(rows, "Comments:")
	if iComment < 0 {
		t.Fatalf("los comentarios deberían caber a esta altura:\n%s", strings.Join(rows, "\n"))
	}
	if urlRow > iComment {
		t.Errorf("el URL (fila %d) debería ir antes que los comentarios (fila %d)", urlRow, iComment)
	}
}

// TestURLRowDoesNotCostHeight: sacar el URL de la rejilla no puede costar una fila.
// Los 13 campos ocupaban 7; los 12 que quedan más el URL a ancho completo tienen que
// seguir siendo 7. Si esto falla, la decisión de darle al URL su fila se está
// pagando con el espacio de los comentarios.
func TestURLRowDoesNotCostHeight(t *testing.T) {
	m := modelWithComments(t, 45, []model.Comment{conv("alice", "ok for me")}, 1)
	rows := m.detailLines(mustSelected(t, m), true, m.layout().detailLines)

	urlRow := rowWithField(rows, "URL:")
	if urlRow < 0 {
		t.Fatalf("no hay fila de URL:\n%s", strings.Join(rows, "\n"))
	}
	if got := len(detailLabels); got != 12 {
		t.Fatalf("el test espera 12 etiquetas de rejilla, tiene %d", got)
	}
	seen := map[string]bool{}
	for _, l := range rows {
		plain := stripANSI(l)
		for _, label := range detailLabels {
			if strings.Contains(plain, label) {
				seen[label] = true
			}
		}
	}
	for _, label := range detailLabels {
		if !seen[label] {
			t.Errorf("la ficha perdió el campo %q al darle su fila al URL:\n%s",
				label, strings.Join(rows, "\n"))
		}
	}

	// 12 campos en dos columnas son 6 filas, y el URL una más.
	gridRows := 0
	for i, l := range rows {
		if i == urlRow {
			continue
		}
		plain := stripANSI(l)
		for _, label := range detailLabels {
			if strings.Contains(plain, label) {
				gridRows++
				break
			}
		}
	}
	if gridRows != 6 {
		t.Errorf("la rejilla ocupa %d filas, want 6 (12 campos en dos columnas):\n%s",
			gridRows, strings.Join(rows, "\n"))
	}
}

// TestLastGridRowCarriesTheActionFields: al recortarse el panel, lo único que
// sobrevive es la última fila de la rejilla. Review y Role van allí a propósito,
// porque son los dos que dicen si la acción procede: una ficha recortada que no
// los enseña deja de responder a la pregunta para la que está.
func TestLastGridRowCarriesTheActionFields(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m.width, m.height = 160, 45
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{
		mkItem("github", "github.com", "acme/widget", "Add widget", 42, "CHANGES_REQUESTED"),
	}, false))
	it := mustSelected(t, m)

	// Un panel de 4 filas solo deja la última fila de la rejilla y los avisos.
	joined := stripANSI(strings.Join(m.detailLines(it, true, 4), "\n"))
	for _, want := range []string{"Review:", "changes requested", "Role:"} {
		if !strings.Contains(joined, want) {
			t.Errorf("el recorte perdió %q, que es lo que dice si la acción procede:\n%s", want, joined)
		}
	}
}

// TestAllocateGivesTheLeftoverToWhoseNeedsIt: cuando no caben los comentarios
// enteros, todos reciben una fila —para que los cinco estén, que es lo pedido— y
// el sobrante va a quien más tiene que perder. Sin esto, el reparto a ciegas daba la
// misma cuota a todos y un comentario de seis párrafos quedaba en "the timeout is
// 30x too high…" al lado de cuatro de una línea, con una fila sin usar.
func TestAllocateGivesTheLeftoverToWhoseNeedsIt(t *testing.T) {
	cases := []struct {
		name   string
		need   []int
		budget int
		want   []int
	}{
		{"caben enteros", []int{1, 4, 1, 1, 1}, 10, []int{1, 4, 1, 1, 1}},
		{"sobra una fila", []int{1, 4, 1, 1, 1}, 8, []int{1, 4, 1, 1, 1}},
		{"falta una fila", []int{1, 4, 1, 1, 1}, 7, []int{1, 3, 1, 1, 1}},
		{"todos largos", []int{4, 4, 4, 4, 4}, 11, []int{3, 2, 2, 2, 2}},
		{"uno larguísimo", []int{4, 1, 1, 1, 1}, 6, []int{2, 1, 1, 1, 1}},
		{"sin presupuesto", []int{1, 4}, 1, []int{1, 1}},
		{"uno solo", []int{4}, 1, []int{1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := allocate(tc.need, tc.budget)
			if len(got) != len(tc.want) {
				t.Fatalf("allocate devolvió %d cuotas, want %d", len(got), len(tc.want))
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("cuota %d = %d, want %d (reparto completo: %v)", i, got[i], tc.want[i], got)
				}
			}
			// Nadie recibe más de lo que necesita, y la suma no pasa del presupuesto
			// cuando hay presupuesto para todos. Con menos filas que comentarios no
			// caben todos, y eso lo recorta quien compone el bloque.
			sum := 0
			for i, n := range got {
				if n > tc.need[i] {
					t.Errorf("la cuota %d (%d) supera lo que necesita (%d)", i, n, tc.need[i])
				}
				if n < 1 {
					t.Errorf("la cuota %d es 0: el comentario desaparecería", i)
				}
				sum += n
			}
			if tc.budget >= len(tc.need) && sum > tc.budget {
				t.Errorf("el reparto suma %d y el presupuesto es %d", sum, tc.budget)
			}
		})
	}
}

// TestAllocateIsDeterministic: el mismo reparto tiene que salir igual cada vez. Si
// dependiera del recorrido de un mapa, el mismo PR se pintaría distinto en cada
// refresco y la ficha bailaría sin que nada hubiera cambiado.
func TestAllocateIsDeterministic(t *testing.T) {
	first := allocate([]int{2, 2, 2}, 4)
	if first[0] != 2 || first[1] != 1 || first[2] != 1 {
		t.Errorf("con una fila sobrante se la lleva siempre el primero: %v", first)
	}
	for range 50 {
		got := allocate([]int{2, 2, 2}, 4)
		for i := range got {
			if got[i] != first[i] {
				t.Fatalf("allocate no es determinista: %v != %v", got, first)
			}
		}
	}
}

// TestLongCommentKeepsItsRowsWhenOthersAreShort: el caso que motiva el reparto. Con
// cuatro comentarios de una línea y uno de varios párrafos, el largo no puede
// quedarse en su primera frase mientras sobran filas.
func TestLongCommentKeepsItsRowsWhenOthersAreShort(t *testing.T) {
	m := modelWithComments(t, 45, []model.Comment{
		conv("alice", "one liner"),
		conv("bob", "the timeout is 30x too high\n\nI would put it at 2s like the rest of the endpoints, "+
			"otherwise the pool exhausts and the requests die in the queue without ever logging"),
		conv("carol", "one liner"),
		conv("dave", "one liner"),
		conv("erin", "one liner"),
	}, 5)
	detail := detailText(t, m)
	if !strings.Contains(detail, "I would put it at 2s") {
		t.Errorf("el comentario largo debería ocupar más de su primera línea:\n%s", detail)
	}
	for _, want := range []string{"alice:", "carol:", "dave:", "erin:"} {
		if !strings.Contains(detail, want) {
			t.Errorf("el reparto de filas no debería sacrificar a %q:\n%s", want, detail)
		}
	}
}

// TestCommentsShowBelowTheFields: los comentarios van DEBAJO de la ficha, y la
// ficha se lee en dos columnas. Es lo que hace que quepan: en una columna la ficha
// ocupa 16 de las 18 líneas que da el 40% de un terminal de 45, y no cabría ni
// uno.
func TestCommentsShowBelowTheFields(t *testing.T) {
	m := modelWithComments(t, 45, []model.Comment{
		conv("alice", "please add a test for the retry path"),
		conv("bob", "the timeout is 30x too high"),
	}, 2)

	detail := detailText(t, m)
	iField := strings.Index(detail, "State:")
	iComment := strings.Index(detail, "Comments:")
	if iField < 0 || iComment < 0 {
		t.Fatalf("faltan bloques en el detalle:\n%s", detail)
	}
	if iComment < iField {
		t.Errorf("los comentarios deberían ir después de la ficha:\n%s", detail)
	}
	for _, want := range []string{"alice:", "please add a test for the retry path", "bob:", "the timeout is 30x too high"} {
		if !strings.Contains(detail, want) {
			t.Errorf("falta %q en el detalle:\n%s", want, detail)
		}
	}
}

// TestFieldsStayInTwoColumns: la rejilla dejó de ser el modo de emergencia para
// terminales bajos. Si volviera a la columna única en un terminal alto, los
// comentarios no tendrían sitio y esta feature no existiría.
func TestFieldsStayInTwoColumns(t *testing.T) {
	m := modelWithComments(t, 60, []model.Comment{conv("alice", "one")}, 1)
	lines := m.detailLines(mustSelected(t, m), true, m.layout().detailLines)
	joined := stripANSI(strings.Join(lines, "\n"))

	// La rejilla empareja los campos por posición: (Item|Forge) y (Author|Source).
	// Que Item y Forge caigan en la MISMA línea es lo que prueba que la rejilla está
	// activa; en columna una quedaría cada uno en su propia fila.
	if !strings.Contains(joined, "Item:") {
		t.Fatalf("no está la fila de la rejilla:\n%s", joined)
	}
	row := ""
	for _, l := range strings.Split(joined, "\n") {
		if strings.Contains(l, "Item:") {
			row = l
			break
		}
	}
	if !strings.Contains(row, "Forge:") {
		t.Errorf("Item y Forge deberían ir en la misma fila de la rejilla, no apilados:\n%s", row)
	}
	// En columna única los 13 campos ocuparían 13 filas; en rejilla, 7.
	if fields := strings.Count(joined, ":"); fields < 13 {
		t.Errorf("se perdieron campos en la rejilla (%d etiquetas):\n%s", fields, joined)
	}
	if rows := strings.Count(joined, "Item:"); rows != 1 {
		t.Errorf("Item debería salir una vez, no %d", rows)
	}
}

// TestCommentsCapAtFive: la ficha enseña como mucho CommentLimit, aunque la
// consulta trajera más.
func TestCommentsCapAtFive(t *testing.T) {
	var many []model.Comment
	for i := 0; i < 9; i++ {
		many = append(many, conv(fmt.Sprintf("u%d", i), fmt.Sprintf("body %d", i)))
	}
	m := modelWithComments(t, 60, many, 9)
	detail := detailText(t, m)
	for i := 0; i < 9; i++ {
		body := fmt.Sprintf("body %d", i)
		want := i < forge.CommentLimit
		if got := strings.Contains(detail, body); got != want {
			t.Errorf("%q presente=%v, want %v (solo los %d primeros)", body, got, want, forge.CommentLimit)
		}
	}
}

// TestCommentsAnnounceThereAreMore: el total es lo que dice que la ficha se está
// perdiendo conversación, que es el momento de abrir el PR. Sin él, cinco
// comentarios se leerían como cinco comentarios y nadie iría a mirar los otros.
func TestCommentsAnnounceThereAreMore(t *testing.T) {
	m := modelWithComments(t, 45, []model.Comment{
		conv("alice", "one"), conv("bob", "two"), conv("carol", "three"),
		conv("dave", "four"), conv("erin", "five"),
	}, 23)
	detail := detailText(t, m)
	if !strings.Contains(detail, "5 of 23") {
		t.Errorf("con 23 comentarios la ficha debería decir cuáles enseña:\n%s", detail)
	}
}

// TestCommentsCountOnlyWhenItMatters: con todo a la vista no hay nada que avisar
// y el recuento solo sería ruido.
func TestCommentsCountOnlyWhenItMatters(t *testing.T) {
	m := modelWithComments(t, 45, []model.Comment{conv("alice", "one"), conv("bob", "two")}, 2)
	detail := detailText(t, m)
	if strings.Contains(detail, "of 2") {
		t.Errorf("no hay nada que avisar si se ve todo:\n%s", detail)
	}
	if !strings.Contains(detail, "Comments:") {
		t.Errorf("debería seguir estando la etiqueta:\n%s", detail)
	}
}

// TestCommentHeadlineSkipsBotBoilerplate: los bots de GitHub abren con un
// comentario HTML invisible. Si se picturara, la ficha enseñaría metadata
// invisible en vez de lo que dijo la persona.
func TestCommentHeadlineSkipsBotBoilerplate(t *testing.T) {
	m := modelWithComments(t, 45, []model.Comment{
		conv("ssf-bot", "<!-- ssf: origin=acme/widget#553 -->\n\nattached the agent to the session"),
	}, 1)
	detail := detailText(t, m)
	if !strings.Contains(detail, "attached the agent to the session") {
		t.Errorf("no se pintó el texto real del comentario:\n%s", detail)
	}
	if strings.Contains(detail, "ssf: origin") {
		t.Errorf("se pintó el boilerplate del bot:\n%s", detail)
	}
}

// TestCommentBodySplitsOverAvailableRows: en un terminal alto sobra sitio y el
// cuerpo se lee entero; el autor va en la primera fila y el resto alineado debajo
// para que se lea como un bloque.
func TestCommentBodySplitsOverAvailableRows(t *testing.T) {
	body := "el reintento no tiene backoff y por eso el servicio se cae cuando " +
		"la dependencia tarda, y ademas el timeout esta a treinta veces lo que " +
		"deberia, asi que el pool se agota y las peticiones mueren en cola sin " +
		"llegar a loguear nada"
	m := modelWithComments(t, 60, []model.Comment{conv("alice", body)}, 1)
	detail := detailText(t, m)

	if !strings.Contains(detail, "el reintento no tiene backoff") {
		t.Fatalf("no se pintó el comentario:\n%s", detail)
	}
	if strings.Count(detail, "alice:") != 1 {
		t.Errorf("el autor debería salir una sola vez:\n%s", detail)
	}
	// Con mucho sitio el cuerpo debe ocupar más de una fila.
	rows := 0
	for _, l := range strings.Split(detail, "\n") {
		if strings.Contains(l, "backoff") || strings.Contains(l, "dependencia") ||
			strings.Contains(l, "treinta veces") || strings.Contains(l, "agota") {
			rows++
		}
	}
	if rows < 2 {
		t.Errorf("con un terminal alto el cuerpo debería ocupar varias filas, ocupó %d:\n%s", rows, detail)
	}
	// Y ninguna fila debe rebasar el ancho: lo que no cabe se corta con "…", no se
	// desborda sobre el borde de la caja.
	for _, l := range strings.Split(detail, "\n") {
		if n := len([]rune(l)); n > m.contentWidth() {
			t.Errorf("fila de %d runes, más que el ancho útil %d:\n%q", n, m.contentWidth(), l)
		}
	}
}

// TestCommentCutIsMarked: lo que no cabe de un comentario se anuncia con "…". Una
// fila que para en mitad de una frase se lee como si el comentario fuera corto, y
// esa es justo la decisión que se pierde. Hace falta un terminal alto para que el
// bloque entre (a 24 filas la rejilla de campos ya llena el panel) y, aun así, un
// cuerpo que no cabe en las filas que le tocan.
func TestCommentCutIsMarked(t *testing.T) {
	long := strings.Repeat("palabra ", 400)
	m := modelWithComments(t, 45, []model.Comment{conv("alice", long)}, 1)
	detail := detailText(t, m)
	if !strings.Contains(detail, "Comments:") {
		t.Fatalf("a 45 filas los comentarios deberían caber:\n%s", detail)
	}
	if !strings.Contains(detail, "…") {
		t.Errorf("un comentario que no cabe debería acabar en …:\n%s", detail)
	}
	// Y el recorte no puede desbordar el ancho de la caja: eso rompería el borde.
	for _, l := range strings.Split(detail, "\n") {
		if n := len([]rune(l)); n > m.contentWidth() {
			t.Errorf("fila de %d runes, más que el ancho útil %d:\n%q", n, m.contentWidth(), l)
		}
	}
}

// TestCommentsAreFirstToGo: cuando el panel es diminuto, los comentarios se caen
// antes que la ficha. Son lo único que se puede volver a pedir en un instante y lo
// único que no estaba ahí antes de existir esta sección; su ausencia se nota menos
// que la de un campo, y un campo que se va no vuelve.
func TestCommentsAreFirstToGo(t *testing.T) {
	list := []model.Comment{conv("alice", "one"), conv("bob", "two"), conv("carol", "three")}
	gh := ghAdapter()
	gh.Conversations = map[string]forge.CommentPage{testutil.ItemKey("acme/widget", 42): {Comments: list, Total: 3}}
	m := newTestModel(t, gh)
	m.width, m.height = 160, 24
	it := mkItem("github", "github.com", "acme/widget", "Add widget", 42, "")
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{it}, false))
	m = withConversation(t, m, it, forge.CommentPage{Comments: list, Total: 3})

	// A 9 filas (el 40% de 24) la rejilla de campos ya llena el panel: los
	// comentarios no caben y la ficha entera se conserva.
	detail := stripANSI(strings.Join(m.detailLines(it, true, 9), "\n"))
	if strings.Contains(detail, "alice:") {
		t.Errorf("a 9 filas los comentarios deberían caerse:\n%s", detail)
	}
	for _, want := range []string{"Add widget", "Item:", "State:", "Role:"} {
		if !strings.Contains(detail, want) {
			t.Errorf("la ficha no debería perder %q al caer los comentarios:\n%s", want, detail)
		}
	}
}

// TestDetailAlwaysFitsThePanel: el bloque de comentarios nunca hace que la caja
// se pase de su alto. Es la invariante que sostiene el layout: la suma de las cajas
// tiene que dar la altura del terminal.
func TestDetailAlwaysFitsThePanel(t *testing.T) {
	for _, height := range []int{20, 24, 30, 40, 45, 60, 80} {
		t.Run(fmt.Sprintf("alto %d", height), func(t *testing.T) {
			var list []model.Comment
			for i := 0; i < 12; i++ {
				list = append(list, conv(fmt.Sprintf("u%d", i), strings.Repeat("texto largo ", 30)))
			}
			m := modelWithComments(t, height, list, 12)
			rows := m.layout().detailLines
			it := mustSelected(t, m)
			// Con y sin avisos de acción, que son filas que también compiten.
			for _, denied := range []bool{false, true} {
				if denied {
					m.denied[it.ID()] = "no permission"
				}
				lines := m.detailLines(it, true, rows)
				if len(lines) > rows {
					t.Fatalf("el detalle devolvió %d filas para un panel de %d (denied=%v):\n%s",
						len(lines), rows, denied, strings.Join(lines, "\n"))
				}
			}
		})
	}
}

// TestCommentsStates: los tres estados tienen que decir cosas distintas. "No hay
// comentarios" y "todavía no lo he preguntado" con la misma línea harían que un
// ítem sin consultar pareciera un PR sin conversación.
func TestCommentsStates(t *testing.T) {
	it := mkItem("github", "github.com", "acme/widget", "Add widget", 42, "")

	cases := []struct {
		name string
		set  func(m *Model)
		want string
		not  string
	}{
		{
			// En vuelo es una entrada marcada y sin resolver, no la ausencia de
			// entrada: esa es la diferencia entre "lo estoy preguntando" y "no lo he
			// preguntado", y por eso tienen que ser dos casos y no uno.
			name: "en vuelo",
			set: func(m *Model) {
				m.comments[it.ID()] = &commentState{}
			},
			want: "loading",
		},
		{
			name: "sin preguntar",
			set:  func(m *Model) {},
			not:  "loading",
		},
		{
			name: "sin comentarios",
			set: func(m *Model) {
				m.comments[it.ID()] = &commentState{ready: true}
			},
			want: "none", not: "loading",
		},
		{
			name: "fallo de lectura",
			set: func(m *Model) {
				m.comments[it.ID()] = &commentState{ready: true, err: "rate limited"}
			},
			want: "rate limited", not: "loading",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestModel(t, ghAdapter())
			m.width, m.height = 160, 45
			m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{it}, false))
			tc.set(&m)
			detail := stripANSI(strings.Join(m.detailLines(it, true, m.layout().detailLines), "\n"))
			if !strings.Contains(detail, tc.want) {
				t.Errorf("el panel debería decir %q:\n%s", tc.want, detail)
			}
			if tc.not != "" && strings.Contains(detail, tc.not) {
				t.Errorf("el panel no debería decir %q:\n%s", tc.not, detail)
			}
		})
	}
}

// TestCommentFailIsNotCachedAsEmpty: un fallo deja el estado en error y se dice, no
// una lista vacía. Si se cacheara como "sin comentarios", un rate limit se
// convertiría en la mentira de que el PR no tiene conversación.
func TestCommentFailIsNotCachedAsEmpty(t *testing.T) {
	it := mkItem("github", "github.com", "acme/widget", "Add widget", 42, "")
	gh := ghAdapter()
	gh.CommentWarnings = map[string][]model.Warning{
		testutil.ItemKey("acme/widget", 42): {{Kind: "ratelimit", Msg: "API rate limit exceeded"}},
	}
	m := newTestModel(t, gh)
	m.width, m.height = 160, 45
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{it}, false))
	m = send(t, m, commentsMsg{id: it.ID(), err: "API rate limit exceeded"})

	st := m.comments[it.ID()]
	if st == nil || !st.ready || st.err == "" {
		t.Fatalf("el fallo debería quedar registrado como error: %+v", st)
	}
	if strings.Contains(detailText(t, m), "none") {
		t.Errorf("un fallo no puede pintarse como \"no hay comentarios\":\n%s", detailText(t, m))
	}
}

// TestCommentsFetchedOnceForTheSelectedItem: la conversación se pide al llegar el
// cursor al ítem y se cachea. Preguntarla en cada render convertiría la navegación
// en una ráfaga de subprocesos.
func TestCommentsFetchedOnceForTheSelectedItem(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m.width, m.height = 160, 45
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{
		mkItem("github", "github.com", "acme/widget", "One", 1, ""),
		mkItem("github", "github.com", "acme/widget", "Two", 2, ""),
	}, false))
	gh := m.byForge["github"].(*testutil.FakeAdapter)

	// Tres ticks seguidos sin respuesta: el primero abre la consulta y los otros
	// dos no deben abrir nada más, porque el ítem ya está marcado como pedido. Por
	// eso se espera a la primera llamada antes de contar: si no, se estaría
	// midiendo el planificador.
	for range 3 {
		m.requestComments()
	}
	waitFor(t, "la primera consulta de comentarios", func() bool { return gh.CommentCallCount() >= 1 })
	if got := gh.CommentCallCount(); got != 1 {
		t.Fatalf("Comments llamado %d veces, want 1 (los ticks no pueden duplicar la consulta)", got)
	}

	// Llega la respuesta y el siguiente tick ya no pregunta de nuevo.
	m = send(t, m, commentsMsg{id: mustSelected(t, m).ID(), page: forge.CommentPage{
		Comments: []model.Comment{conv("alice", "hi")}, Total: 1,
	}})
	m.requestComments()
	if got := gh.CommentCallCount(); got != 1 {
		t.Errorf("Comments llamado %d veces tras cachear, want 1", got)
	}
}

// TestCommentsNotAskedWithoutSession: sin sesión la consulta solo puede fallar, y
// la cabecera ya lo dice. Gastar un subproceso en volver a fallar no le informa de
// nada nuevo al usuario.
func TestCommentsNotAskedWithoutSession(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m.width, m.height = 160, 45
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{
		mkItem("github", "github.com", "acme/widget", "One", 1, ""),
	}, false))
	// El estado de sesión no lo fija New(): lo deja optimista en true hasta que
	// llega el authMsg del refresco. Por eso hay que inyectarlo aquí, que es como
	// está en pantalla cuando la cabecera avisa de que no hay sesión.
	m.statuses["github"].auth = model.AuthState{Forge: "github", OK: false, Reason: "bad token"}
	gh := m.byForge["github"].(*testutil.FakeAdapter)

	m.requestComments()
	if got := gh.CommentCallCount(); got != 0 {
		t.Errorf("Comments llamado %d veces sin sesión, want 0", got)
	}
	// Y la ficha no se queda fingiendo que los está cargando: si no se pregunta,
	// no hay nada en vuelo que pintar.
	if _, marked := m.comments[mustSelected(t, m).ID()]; marked {
		t.Error("no se debería marcar como pedida una consulta que no se lanza")
	}
	if detail := detailText(t, m); strings.Contains(detail, "loading") {
		t.Errorf("sin consulta en vuelo no debería decir loading:\n%s", detail)
	}
}

// TestCommentsInvalidatedByAction: una acción puede escribir en la conversación
// (un approve deja nota de review), así que lo cacheado deja de ser verdad y hay
// que volver a preguntarlo.
func TestCommentsInvalidatedByAction(t *testing.T) {
	it := mkItem("github", "github.com", "acme/widget", "Add widget", 42, "")
	gh := ghAdapter()
	m := newTestModel(t, gh)
	m.width, m.height = 160, 45
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{it}, false))
	m = send(t, m, commentsMsg{id: it.ID(), page: forge.CommentPage{
		Comments: []model.Comment{conv("alice", "hi")}, Total: 1,
	}})
	if _, ok := m.comments[it.ID()]; !ok {
		t.Fatal("la conversación debería estar cacheada antes de la acción")
	}

	m.applyAction(forge.Outcome{Kind: forge.ActionApprove, ID: it.ID(), OK: true, Item: it, HasItem: true}, m.cycle)
	if _, ok := m.comments[it.ID()]; ok {
		t.Error("una acción debería invalidar la conversación cacheada del ítem")
	}
	// Y el siguiente tick vuelve a preguntar por ella.
	_ = m.requestComments()
	waitFor(t, "la consulta tras invalidar", func() bool { return gh.CommentCallCount() >= 1 })
	if got := gh.CommentCallCount(); got != 1 {
		t.Errorf("Comments llamado %d veces tras invalidar, want 1", got)
	}
}

// TestCommentsSurviveRefresh: el inbox se recarga cada minuto y la conversación no
// cambia a ese ritmo. Repreguntarla en cada ciclo solo haría parpadear la ficha,
// así que la cache sobrevive al refresco.
func TestCommentsSurviveRefresh(t *testing.T) {
	it := mkItem("github", "github.com", "acme/widget", "Add widget", 42, "")
	m := newTestModel(t, ghAdapter())
	m.width, m.height = 160, 45
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{it}, false))
	m = send(t, m, commentsMsg{id: it.ID(), page: forge.CommentPage{
		Comments: []model.Comment{conv("alice", "hi")}, Total: 1,
	}})

	m.cycle = 2
	m = send(t, m, page(2, "github", "github.com", model.SectionAuthored, "", []model.Item{it}, false))
	if _, ok := m.comments[it.ID()]; !ok {
		t.Error("el refresco del inbox no debería tirar la conversación cacheada")
	}
}

// TestCommentTotalNeverBelowShown: un total menor que lo mostrado se leería como
// que la ficha enseña comentarios que no existen.
func TestCommentTotalNeverBelowShown(t *testing.T) {
	it := mkItem("github", "github.com", "acme/widget", "Add widget", 42, "")
	m := newTestModel(t, ghAdapter())
	m.width, m.height = 160, 45
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{it}, false))
	m = send(t, m, commentsMsg{id: it.ID(), page: forge.CommentPage{
		Comments: []model.Comment{conv("a", "1"), conv("b", "2"), conv("c", "3")}, Total: 1,
	}})

	detail := detailText(t, m)
	if strings.Contains(detail, "of 1") || strings.Contains(detail, "3 of 1") {
		t.Errorf("un total por debajo de lo mostrado no debe pintarse:\n%s", detail)
	}
	if st := m.comments[it.ID()]; st.total < 3 {
		t.Errorf("total = %d, want >= 3 (lo mostrado)", st.total)
	}
}
