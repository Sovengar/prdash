package tui

import (
	"strings"
	"testing"
	"unicode/utf8"

	"prdash/internal/forge/model"
	"prdash/internal/inbox"
)

func TestPadAndTruncate(t *testing.T) {
	if got := pad("ab", 5); got != "ab   " {
		t.Errorf("pad = %q", got)
	}
	if got := truncate("abcdef", 4); got != "abc…" {
		t.Errorf("truncate = %q", got)
	}
}

// TestRenderCellsPadsBeforeStyle verifica que el relleno va antes del estilo:
// el texto plano resultante conserva el ancho de columna.
func TestRenderCellsPadsBeforeStyle(t *testing.T) {
	cells := []cell{
		{text: "ab", style: styleForge, width: 5},
		{text: "x", style: styleRef, width: 3},
	}
	out := stripANSI(renderCells(cells, newRefLayout(nil), 100))
	if !strings.HasPrefix(out, "ab   x  ") {
		t.Errorf("renderCells = %q", out)
	}
}

// TestNingunaCeldaLlenaSuColumna cubre el pegado de columnas: el ancho de una
// columna incluye el hueco de separación, así que el texto nunca puede ocupar
// los runes completos. Sin esto, un texto que llenara la columna exacta
// pegaba con la siguiente ("…gatewayreview req").
func TestNingunaCeldaLlenaSuColumna(t *testing.T) {
	// Los casos que llenaban su columna: el título y el sufijo de ITEM (ancho
	// dinámico), y ROLE, cuyo texto crudo era del ancho exacto de su columna
	// antigua.
	it := mkItem("github", "github.com", "APPCITTI/vsocial/backend/mobile-frontend",
		strings.Repeat("t", colTitle+5), 1198, "")
	it.ReviewKind = model.ReviewRequested // sin esto roleText devuelve "-"
	lay := newRefLayout([]inbox.Section{section(model.SectionReview, mkItems("APPCITTI/vsocial/backend/mobile-frontend")...)})
	cells := itemCells(it, model.SectionReview, "yo", lay)

	// El texto CRUDO de ROLE tiene que caber con su hueco ("review req" = 10 en
	// una columna de colRole): es el caso que pegaba con STATE. El título CRUDO
	// se pasa de largo y se recorta.
	if got, want := utf8.RuneCountInString(roleText(it, "yo")), colRole-1; got != want {
		t.Fatalf("el texto de ROLE mide %d runes, want %d: el caso que pegaba ya no se está probando", got, want)
	}
	if got, want := utf8.RuneCountInString(cells[colRoleIdx].text), colRole-1; got != want {
		t.Fatalf("celda ROLE = %d runes, want %d (colRole - hueco)", got, want)
	}
	if got, want := utf8.RuneCountInString(cells[colTitleIdx].text), colTitle-1; got != want {
		t.Fatalf("celda TITLE = %d runes, want %d (colTitle - hueco)", got, want)
	}
	// Y el sufijo más largo de la columna cabe entero: el ancho lo cuenta todo.
	if got, want := utf8.RuneCountInString(cells[colRefIdx].text), textWidth(lay.cols[colRefIdx].width); got != want {
		t.Fatalf("celda ITEM = %d runes, want %d (presupuesto de la columna)", got, want)
	}
	for i, c := range cells {
		if n := utf8.RuneCountInString(c.text); n > textWidth(c.width) {
			t.Errorf("celda %d (%q) = %d runes, want <= %d (ancho %d - hueco)", i, c.text, n, textWidth(c.width), c.width)
		}
	}
}

// TestColumnasSeparadasPorUnEspacio es el mismo invariante sobre la fila
// compuesta: cada ranura de columna acaba en un espacio, así que ninguna
// columna puede pegarse con la siguiente.
func TestColumnasSeparadasPorUnEspacio(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{
		mkItem("github", "github.com", "APPCITTI/vsocial/backend/api-gateway", "api gateway", 1234, ""),
		mkItem("github", "github.com", "APPCITTI/vsocial/backend/mobile-frontend", "movil", 1198, ""),
	}, false))

	lay := newRefLayout(m.inbox.Sections)
	inner := m.contentWidth() - 2
	var row string
	for _, l := range m.listLines(m.contentWidth()) {
		if line := stripANSI(l.text); strings.Contains(line, "api-gateway#1234") {
			row = line
		}
	}
	if row == "" {
		t.Fatal("no se encontró la fila del ítem")
	}

	// El prefijo de la fila ("  " o "▸ ") mide 2 runes: la primera columna
	// empieza ahí.
	runes := []rune(row)
	off := 2
	for _, c := range lay.cols[:fitColumns(lay, inner)] {
		end := off + c.width
		if end > len(runes) {
			t.Fatalf("fila %q más corta que la columna %q", row, c.title)
		}
		if runes[end-1] != ' ' {
			t.Errorf("la columna %q acaba en %q, want un espacio de separación en %q", c.title, runes[end-1], row)
		}
		off = end
	}
}

// TestForgeBadgeCoversStandardAndSelfHosted fija la etiqueta de la columna
// FORGE: abreviatura sola en el host estándar, abreviatura + primera etiqueta
// del host en uno self-hosted, sin perder nunca la identidad en el detalle.
func TestForgeBadgeCoversStandardAndSelfHosted(t *testing.T) {
	badge := func(forge, host string) string {
		it := model.NewItem(model.RepoRef{Forge: forge, Host: host, Project: "o/r"}, 1)
		return forgeBadge(it)
	}
	for _, tc := range []struct {
		forge, host, want string
	}{
		{"github", "github.com", "GH"},
		{"gitlab", "gitlab.com", "GLab"},
		{"bitbucket", "bitbucket.org", "BB"},
		{"gitlab", "umane.emeal.nttdata.com", "GLab@umane"},
		{"github", "github.corp.example.com", "GH@github"},
		{"gitlab", "gitserver", "GLab@gitserver"},
		{"github", "", "GH"},
		{"gerrit", "gerrit.example.com", "gerrit@gerrit"},
	} {
		if got := badge(tc.forge, tc.host); got != tc.want {
			t.Errorf("forgeBadge(%q, %q) = %q, want %q", tc.forge, tc.host, got, tc.want)
		}
	}

	// El detalle sigue mostrando la ruta completa.
	it := model.NewItem(model.RepoRef{Forge: "gitlab", Host: "umane.emeal.nttdata.com", Project: "g/p"}, 7)
	if got := forgeLabel(it); got != "gitlab@umane.emeal.nttdata.com" {
		t.Errorf("forgeLabel = %q", got)
	}
}

// TestForgeBadgeFitsColumn evita que un host self-hosted largo desalinee la
// tabla: la celda se recorta al ancho de columna.
func TestForgeBadgeFitsColumn(t *testing.T) {
	it := model.NewItem(model.RepoRef{Forge: "gitlab", Host: "empresaurbanisimaziyota.example.com", Project: "g/p"}, 1)
	if got := truncate(forgeBadge(it), colForge); utf8.RuneCountInString(got) != colForge {
		t.Errorf("celda forge = %q (%d runes), want %d", got, utf8.RuneCountInString(got), colForge)
	}
}

// TestNavigationMovesCursor cubre la navegación entre filas y secciones.
func TestNavigationMovesCursor(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{
		mkItem("github", "github.com", "acme/widget", "A", 1, ""),
		mkItem("github", "github.com", "acme/widget", "B", 2, ""),
	}, false))
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{
		mkItem("github", "github.com", "acme/widget", "C", 3, ""),
	}, false))

	if m.cursor != 0 {
		t.Fatalf("cursor inicial = %d", m.cursor)
	}
	m = press(t, m, "down")
	if m.cursor != 1 {
		t.Fatalf("cursor tras down = %d", m.cursor)
	}
	m = press(t, m, "tab")
	if m.cursor != 2 {
		t.Fatalf("cursor tras tab = %d, want 2 (primer ítem de review)", m.cursor)
	}
	if it, ok := m.selected(); !ok || it.Title != "C" {
		t.Fatalf("seleccionado = %+v", it)
	}
}

// TestSectionNextHonorsRebind comprueba que la tecla de section-next sale de
// [keybindings] y no de un "tab" cableado. Con el atajo hardcodeado, un
// rebind en la config se anunciaba en la barra de hints y no hacia nada: la
// tecla nueva era un fantasma.
func TestSectionNextHonorsRebind(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m.cfg.Keybindings["section-next"] = "n"
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{
		mkItem("github", "github.com", "acme/widget", "A", 1, ""),
	}, false))
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{
		mkItem("github", "github.com", "acme/widget", "C", 3, ""),
	}, false))

	m = press(t, m, "n")
	if m.cursor != 1 {
		t.Fatalf("cursor tras n = %d, want 1 (primer ítem de review)", m.cursor)
	}
	if it, ok := m.selected(); !ok || it.Title != "C" {
		t.Fatalf("seleccionado = %+v", it)
	}
	// tab ya no es la tecla de la acción: debe ser una tecla muerta y no
	// advancing de sección, para que la barra no prometa lo que no hay.
	m = press(t, m, "tab")
	if m.cursor != 1 {
		t.Fatalf("tab movió la sección aun estando rebindeada: cursor = %d, want 1", m.cursor)
	}
}

// TestAuthoredOnlyShowsOwnItems comprueba que la sección authored solo contiene
// lo que abrí yo.
func TestAuthoredOnlyShowsOwnItems(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{mkItem("github", "github.com", "acme/widget", "Mío", 1, "")}, false))
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{mkItem("github", "github.com", "acme/lib", "Ajeno", 2, "")}, false))

	authored := m.sectionItems(model.SectionAuthored)
	if len(authored) != 1 || authored[0].Title != "Mío" {
		t.Fatalf("authored = %+v", authored)
	}
}

func TestInitReturnsCmd(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	if m.Init() == nil {
		t.Fatal("Init debería devolver un Cmd")
	}
}

// TestCompactCount fija la regla de abreviación del diffstat. El objetivo es que
// el peor caso quepa en la columna: "+9.9k -9.9k" son 11 runes, y por eso a
// partir de cinco dígitos se redondea a "12k" en vez de "12.3k".
func TestCompactCount(t *testing.T) {
	// El salto de décimas a enteros llega al redondear al alza, no al llegar a
	// cinco dígitos: 9499 sigue siendo "9.5k" y solo 9950 se va a "10k".
	cases := map[int]string{
		0: "0", 7: "7", 999: "999",
		1000: "1.0k", 1234: "1.2k", 9499: "9.5k", 9949: "9.9k",
		9950: "10k", 9999: "10k", 10000: "10k", 12345: "12k", 999999: "999k",
	}
	for in, want := range cases {
		if got := compactCount(in); got != want {
			t.Errorf("compactCount(%d) = %q, want %q", in, got, want)
		}
	}
	if got := utf8.RuneCountInString(diffColumnText(model.DiffStat{Additions: 9999, Deletions: 9999, Known: true})); got > textWidth(colDiff) {
		t.Errorf("el peor caso ocupa %d runes y la columna da %d", got, textWidth(colDiff))
	}
}

// TestDiffColumnTextUnknownIsDash: un diffstat que el forge no reportó se marca
// "-", igual que los checks sin datos. Un "0 -0" parecería un PR vacío.
func TestDiffColumnTextUnknownIsDash(t *testing.T) {
	if got := diffColumnText(model.DiffStat{}); got != "-" {
		t.Errorf("diffColumnText sin datos = %q, want %q", got, "-")
	}
	if got := diffColumnText(model.DiffStat{Known: true}); got != "+0 -0" {
		t.Errorf("diffColumnText de un PR vacío = %q, want %q (conocido, cero líneas)", got, "+0 -0")
	}
	if got := diffColumnText(model.DiffStat{Additions: 381, Deletions: 36, Known: true}); got != "+381 -36" {
		t.Errorf("diffColumnText = %q, want %q", got, "+381 -36")
	}
}

// TestDiffColumnAppearsOnlyOnWideTerminals fija la prioridad de la columna DIFF:
// va la última, así que a 124 con rutas largas no cabe y se omite sin que se
// pierda nada (el detalle la trae). Con terminal ancha aparece.
func TestDiffColumnAppearsOnlyOnWideTerminals(t *testing.T) {
	item := mkItem("github", "github.com", "APPCITTI/vsocial/backend/vsocial-api-actuacions", "fix", 1015, "")
	item.Diff = model.DiffStat{Additions: 42, Deletions: 1, Files: 2, Known: true}

	render := func(width int) (header, row string) {
		m := newTestModel(t, ghAdapter())
		m.width, m.height = width, 40
		m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{item}, false))
		lay := newRefLayout(m.inbox.Sections)
		header = stripANSI(headerLine(lay, m.contentWidth()-2))
		for _, l := range m.listLines(m.contentWidth()) {
			if line := stripANSI(l.text); strings.Contains(line, "vsocial-api-actuacions#1015") {
				row = line
			}
		}
		return header, row
	}

	_, narrow := render(124)
	if strings.Contains(narrow, "+42 -1") {
		t.Errorf("a 124 la columna DIFF no debería caber, pero la fila la trae:\n%s", narrow)
	}
	header, wide := render(200)
	if !strings.Contains(wide, "+42 -1") {
		t.Errorf("a 200 la fila debería traer el diffstat:\n%s", wide)
	}
	if !strings.Contains(header, "DIFF") {
		t.Errorf("a 200 la cabecera debería traer la columna DIFF:\n%s", header)
	}
}

// TestDiffSpansColoreaSoloCifras: la columna DIFF lleva dos colores en la misma
// celda —verde lo que se añade, rojo lo que se quita— y es notación de diff, no
// un juicio. Lo que no es un recuento con signo se queda plano: colorear un
// "unknown" o un "no changes" inventaría una cifra que no está.
func TestDiffSpansColoreaSoloCifras(t *testing.T) {
	cases := []struct {
		plain string
		want  []string // texto de cada tramo; nil = sin tramos (todo plano)
	}{
		{"+381 -36", []string{"+381", " ", "-36"}},
		{"+1.2k -6.7k", []string{"+1.2k", " ", "-6.7k"}},
		{"+381 -36 (11 files)", []string{"+381", " ", "-36", " (11 files)"}},
		{"+0 -0", []string{"+0", " ", "-0"}},
		{"-", nil},
		{"no changes", nil},
		{"unknown (forge did not report it)", nil},
		{"+381", nil}, // truncado: sin el lado de las eliminaciones
		{"+abc -def", nil},
		{"381 36", nil}, // sin signo no es un diffstat
	}
	for _, c := range cases {
		spans := diffSpans(c.plain)
		if c.want == nil {
			if spans != nil {
				t.Errorf("diffSpans(%q) = %+v, want nil (sin color)", c.plain, spans)
			}
			continue
		}
		var got []string
		for _, s := range spans {
			got = append(got, s.text)
		}
		if len(got) != len(c.want) {
			t.Errorf("diffSpans(%q) = %q, want %q", c.plain, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("diffSpans(%q)[%d] = %q, want %q", c.plain, i, got[i], c.want[i])
			}
		}
	}
}

// TestDiffSpansSideColores fija qué color va con cada lado: verde al añadido,
// rojo al quitado. Si se intercambian, el diffstat diría lo contrario de lo que
// pasó.
func TestDiffSpansSideColores(t *testing.T) {
	spans := diffSpans("+381 -36")
	if len(spans) != 3 {
		t.Fatalf("spans = %d, want 3", len(spans))
	}
	if got := spans[0].style.Render("x"); got != styleDiffAdd.Render("x") {
		t.Errorf("lo añadido debería ir en styleDiffAdd: %q", got)
	}
	if got := spans[2].style.Render("x"); got != styleDiffDel.Render("x") {
		t.Errorf("lo quitado debería ir en styleDiffDel: %q", got)
	}
}

// TestRenderCellSpansKeepWidth: una celda con varios tramos debe ocupar
// exactamente el ancho de su columna, igual que una de un solo estilo. Si el
// relleno se midiera sobre texto ya coloreado, la tabla bailaría.
func TestRenderCellSpansKeepWidth(t *testing.T) {
	c := diffCell(model.DiffStat{Additions: 1589, Deletions: 474, Files: 23, Known: true}, colDiff)
	out := renderCell(c)
	if got := utf8.RuneCountInString(stripANSI(out)); got != colDiff {
		t.Errorf("la celda ocupa %d runes, want %d: %q", got, colDiff, stripANSI(out))
	}
	if got := strings.TrimRight(stripANSI(out), " "); got != "+1.6k -474" {
		t.Errorf("celda = %q, want %q", got, "+1.6k -474")
	}
	if !strings.Contains(out, "\x1b[") {
		t.Errorf("la celda debería traer color: %q", out)
	}
	// La celda desconocida no lleva color de cifra: solo el gris de "no hay dato".
	unknown := renderCell(diffCell(model.DiffStat{}, colDiff))
	if got := strings.TrimRight(stripANSI(unknown), " "); got != "-" {
		t.Errorf("celda desconocida = %q, want %q", got, "-")
	}
	if got := utf8.RuneCountInString(stripANSI(unknown)); got != colDiff {
		t.Errorf("la celda desconocida ocupa %d runes, want %d", got, colDiff)
	}
}

// TestStyleDiffTextNoAlteraElAncho: styleDiffText solo añade códigos ANSI; el
// texto plano tiene que quedar idéntico, que es lo que permite medir y recortar
// antes de colorear.
func TestStyleDiffTextNoAlteraElAncho(t *testing.T) {
	for _, plain := range []string{"+381 -36", "+381 -36 (11 files)", "-", "no changes", "unknown (forge did not report it)"} {
		if got := stripANSI(styleDiffText(plain)); got != plain {
			t.Errorf("styleDiffText(%q) en plano = %q, want idéntico", plain, got)
		}
	}
}
