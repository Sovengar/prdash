package tui

import (
	"math/rand"
	"strings"
	"testing"
	"unicode/utf8"

	"prdash/internal/forge/model"
	"prdash/internal/inbox"
)

// mkItems crea ítems con rutas concretas, para probar el reparto del prefijo de
// ruta entre las secciones.
func mkItems(projects ...string) []model.Item {
	items := make([]model.Item, 0, len(projects))
	for i, p := range projects {
		items = append(items, mkItem("gitlab", "gitlab.com", p, "T", 100+i, ""))
	}
	return items
}

func section(kind model.Section, items ...model.Item) inbox.Section {
	return inbox.Section{Kind: kind, Items: items}
}

// TestSectionPrefix fija el prefijo de ruta común de una sección: alineado en
// fronteras "/", y sin comerse nunca el segmento final (la hoja), que es lo que
// identifica el proyecto en la celda.
func TestSectionPrefix(t *testing.T) {
	for _, tc := range []struct {
		name  string
		items []model.Item
		want  string
	}{
		{"subgrupo largo", mkItems("APPCITTI/vsocial/backend/api-gateway", "APPCITTI/vsocial/backend/web-app"), "APPCITTI/vsocial/backend"},
		{"corta en el grupo comun", mkItems("APPCITTI/vsocial/a/x", "APPCITTI/vsocial/b/y"), "APPCITTI/vsocial"},
		{"sin nada en comun", mkItems("a/one", "b/two"), ""},
		{"owner/repo de github", mkItems("acme/widget", "acme/lib"), "acme"},
		{"proyectos identicos", mkItems("g/p", "g/p"), "g"},
		{"un solo item", mkItems("APPCITTI/vsocial/backend/api"), ""},
		{"sin subgrupos", mkItems("a", "b"), ""},
		{"prefijos parciales", mkItems("APPCITTI/vs/x", "APPCITTI/vsocial/y"), "APPCITTI"},
		{"proyecto vacio", mkItems("", "g/p"), ""},
		{"misma ruta, distinto host", mkItems("g/p", "g/p"), "g"},
	} {
		if got := sectionPrefix(tc.items); got != tc.want {
			t.Errorf("%s: sectionPrefix = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestRefSuffixQuitaElPrefijoDeLaCabecera comprueba que la celda y la cabecera
// reconstruyen la referencia completa, y que un prefijo que no encaja deja la
// celda intacta en vez de mutilarla.
func TestRefSuffixQuitaElPrefijoDeLaCabecera(t *testing.T) {
	items := mkItems("APPCITTI/vsocial/backend/api-gateway", "APPCITTI/vsocial/backend/web-app")
	prefix := sectionPrefix(items)

	got := refSuffix(items[0], prefix)
	if want := "api-gateway#100"; got != want {
		t.Errorf("refSuffix = %q, want %q", got, want)
	}
	if full := prefix + "/" + got; full != refLabel(items[0]) {
		t.Errorf("%q + %q = %q, want %q", prefix, got, full, refLabel(items[0]))
	}
	if got := refSuffix(items[0], ""); got != refLabel(items[0]) {
		t.Errorf("sin prefijo: refSuffix = %q, want %q", got, refLabel(items[0]))
	}
	if got := refSuffix(items[0], "otro/grupo"); got != refLabel(items[0]) {
		t.Errorf("prefijo ajeno: refSuffix = %q, want %q", got, refLabel(items[0]))
	}
}

func TestTruncateTail(t *testing.T) {
	for _, tc := range []struct {
		s    string
		w    int
		want string
	}{
		{"short", 10, "short"},
		{"abcdefgh", 4, "…fgh"},
		{"abc", 1, "…"},
		{"abc", 0, ""},
		{"api-gateway#1234", 6, "…#1234"},
		{"api-gateway#1234", 8, "…ay#1234"},
	} {
		if got := truncateTail(tc.s, tc.w); got != tc.want {
			t.Errorf("truncateTail(%q, %d) = %q, want %q", tc.s, tc.w, got, tc.want)
		}
	}
}

// TestNewRefLayoutDimensionaITEMPorContenido comprueba que la columna se
// dimensiona al sufijo más largo de todas las secciones (no a una constante) y
// que respeta los límites.
func TestNewRefLayoutDimensionaITEMPorContenido(t *testing.T) {
	// Sufijo corto: la columna se ajusta a lo que necesita, no al tope. El ancho
	// incluye el hueco de separación, así que es el sufijo más un rune.
	lay := newRefLayout([]inbox.Section{
		section(model.SectionReview, mkItems("g/one", "g/two")...),
	})
	if got, want := lay.cols[colRefIdx].width, len("one#100")+1; got != want {
		t.Errorf("ancho de ITEM = %d, want %d (el sufijo más largo + hueco)", got, want)
	}
	if got := lay.prefixOf(model.SectionReview); got != "g" {
		t.Errorf("prefijo de review = %q, want %q", got, "g")
	}
	if got := lay.prefixOf(model.SectionAuthored); got != "" {
		t.Errorf("prefijo de una sección ausente = %q, want vacío", got)
	}

	// Sufijos larguísimos (hojas largas tras un grupo corto): se acota al tope.
	lay = newRefLayout([]inbox.Section{
		section(model.SectionReview, mkItems(
			"g/un-servicio-con-nombre-larguísimo",
			"g/otro-servicio-con-nombre-larguísimo",
		)...),
	})
	if got := lay.cols[colRefIdx].width; got != itemWidthCap {
		t.Errorf("ancho de ITEM = %d, want el tope %d", got, itemWidthCap)
	}

	// Sin ítems: el mínimo, para que "ITEM" no se solape con la columna vecina.
	lay = newRefLayout(nil)
	if got := lay.cols[colRefIdx].width; got != itemWidthMin {
		t.Errorf("ancho de ITEM sin ítems = %d, want %d", got, itemWidthMin)
	}
}

// TestItemCellsConservanElNumeroAlRecortar es el caso que motivó el cambio: con
// la ruta larga de un subgrupo, la celda tiene que seguir diciendo qué proyecto
// y qué número es, aunque haya que recortar.
func TestItemCellsConservanElNumeroAlRecortar(t *testing.T) {
	items := mkItems(
		"APPCITTI/vsocial/backend/un-servicio-con-nombre-larguísimo",
		"APPCITTI/vsocial/backend/otro-servicio-con-nombre-larguísimo",
	)
	lay := newRefLayout([]inbox.Section{section(model.SectionReview, items...)})
	ref := itemCells(items[0], model.SectionReview, "yo", lay)[colRefIdx].text

	if !strings.HasSuffix(ref, "#100") {
		t.Errorf("celda ITEM = %q, want el sufijo %q", ref, "#100")
	}
	if got := utf8.RuneCountInString(ref); got > itemWidthCap {
		t.Errorf("celda ITEM = %q (%d runes), excede el tope %d", ref, got, itemWidthCap)
	}
	if !strings.HasPrefix(ref, "…") {
		t.Errorf("celda ITEM = %q, want el recorte por la cola (prefijo %q)", ref, "…")
	}
	// Lo que se pierde por el frente es el grupo, que la cabecera ya declara.
	if got := lay.prefixOf(model.SectionReview); got != "APPCITTI/vsocial/backend" {
		t.Errorf("prefijo = %q, want el grupo que compensó el recorte", got)
	}
}

// TestListLinesMuestranElPrefijoEnLaCabecera es la prueba de integración: la
// cabecera de la sección declara el grupo común y las filas solo pintan el
// sufijo, sin truncar por ninguna de las dos.
func TestListLinesMuestranElPrefijoEnLaCabecera(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, mkItems(
		"APPCITTI/vsocial/backend/api-gateway",
		"APPCITTI/vsocial/backend/web-app",
	), false))

	var header, row string
	// La etiqueta sale de model.Section.String(): fijarla en el test lo rompe
	// cada vez que se traduce la interfaz.
	title := model.SectionReview.String()
	for _, l := range m.listLines(m.contentWidth()) {
		line := stripANSI(l.text)
		switch {
		case strings.HasPrefix(line, title):
			header = line
		case strings.Contains(line, "api-gateway#100"):
			row = line
		}
	}
	if !strings.Contains(header, "APPCITTI/vsocial/backend/") {
		t.Errorf("cabecera = %q, want el prefijo común %q", header, "APPCITTI/vsocial/backend/")
	}
	if strings.Contains(row, "APPCITTI") {
		t.Errorf("fila = %q, want solo el sufijo", row)
	}
	if !strings.Contains(row, "api-gateway#100") {
		t.Errorf("fila = %q, want %q sin truncar", row, "api-gateway#100")
	}
}

// TestListLinesSinPrefijoConservanLaRuta completa: una sección con un solo ítem no
// puede declarar prefijo común, así que la celda lleva la ruta ella sola. Si
// cabe, entera; si no, recortada por la cola, que es donde está el número.
func TestListLinesSinPrefijoConservanLaRuta(t *testing.T) {
	render := func(project string) []string {
		m := newTestModel(t, ghAdapter())
		m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, mkItems(project), false))
		lines := make([]string, 0, 8)
		for _, l := range m.listLines(m.contentWidth()) {
			lines = append(lines, stripANSI(l.text))
		}
		return lines
	}

	// Ruta corta: entra entera y sin recortes.
	for _, line := range render("g/p") {
		if strings.Contains(line, "g/p#100") {
			return
		}
	}
	t.Fatal("una ruta corta en sección de un ítem no se pintó entera")

	// Ruta larga: sin prefijo que la compense, se recorta por la cola y el
	// "#número" sigue visible. Es el límite del que el detalle es red de seguridad.
	var cell string
	for _, line := range render("APPCITTI/vsocial/backend/api-gateway") {
		if strings.HasPrefix(line, model.SectionReview.String()) && strings.Contains(line, "APPCITTI") {
			t.Errorf("cabecera = %q, want el prefijo solo si lo comparten varios ítems", line)
		}
		if strings.Contains(line, "api-gateway#100") {
			cell = line
		}
	}
	if cell == "" {
		t.Fatal("ninguna línea pintó el sufijo del ítem")
	}
	if !strings.HasPrefix(cell, "…") {
		t.Errorf("celda = %q, want el recorte por la cola", cell)
	}
}

// TestRefColInvariantes sobre rutas aleatorias: la celda es exactamente el
// recorte del sufijo que le toca, nunca queda vacía, conserva el "#número" y el
// ancho se queda dentro de los límites.
func TestRefColInvariantes(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	seg := func() string {
		return strings.Repeat(string(rune('a'+rng.Intn(26))), 1+rng.Intn(12))
	}
	// Una sección por kind, como los garantiza inbox.Build: el prefijo se indexa
	// por sección y dos secciones del mismo kind se pisarían.
	kinds := []model.Section{model.SectionAuthored, model.SectionReview, model.SectionMentions}
	for round := range 300 {
		sections := make([]inbox.Section, 1+rng.Intn(3))
		for si := range sections {
			// Los proyectos de una sección comparten grupo la mayoría de las
			// veces; a veces no, para ejercitar el prefijo vacío.
			group := seg()
			if rng.Intn(4) == 0 {
				group = seg()
			}
			for range 1 + rng.Intn(5) {
				parts := []string{group}
				for range 1 + rng.Intn(4) {
					parts = append(parts, seg())
				}
				sections[si].Kind = kinds[si]
				sections[si].Items = append(sections[si].Items,
					mkItem("gitlab", "gitlab.com", strings.Join(parts, "/"), "T", 1+rng.Intn(9999), ""))
			}
		}

		lay := newRefLayout(sections)
		w := lay.cols[colRefIdx].width
		if w < itemWidthMin || w > itemWidthCap {
			t.Fatalf("round %d: ancho de ITEM = %d, fuera de [%d, %d]", round, w, itemWidthMin, itemWidthCap)
		}
		for _, sec := range sections {
			prefix := lay.prefixOf(sec.Kind)
			for _, it := range sec.Items {
				full := refLabel(it)
				want := full
				if prefix != "" {
					want = strings.TrimPrefix(full, prefix+"/")
					if want == full {
						t.Fatalf("round %d: el prefijo %q no aplica a %q", round, prefix, full)
					}
				}
				cell := itemCells(it, sec.Kind, "yo", lay)[colRefIdx].text
				if cell == "" {
					t.Fatalf("round %d: celda vacía para %q", round, full)
				}
				// El presupuesto de texto es el ancho de la ranura menos el hueco
				// de separación: el sufijo más largo tiene que caber entero.
				cut := truncateTail(want, textWidth(w))
				if want == cut {
					if cell != want {
						t.Fatalf("round %d: celda %q, want %q", round, cell, want)
					}
					continue
				}
				// Recortada: la cola (hoja y número) tiene que salir intacta.
				if !strings.HasPrefix(cell, "…") {
					t.Fatalf("round %d: celda recortada %q, want el prefijo %q", round, cell, "…")
				}
				tail := want[max(0, len(want)-(textWidth(w)-1)):]
				if !strings.HasSuffix(cell, tail) {
					t.Fatalf("round %d: celda %q pierde la cola %q de %q", round, cell, tail, want)
				}
			}
		}
	}
}
