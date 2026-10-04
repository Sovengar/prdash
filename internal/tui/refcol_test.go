package tui

import (
	"math/rand"
	"strings"
	"testing"
	"unicode/utf8"

	"prdash/internal/forge/model"
	"prdash/internal/inbox"
)

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

func TestNewRefLayoutDimensionaITEMPorContenido(t *testing.T) {
	lay := newRefLayout([]inbox.Section{
		section(model.SectionReview, mkItems("g/one", "g/two")...),
	}, prefixCommon)
	if got, want := lay.cols[colRefIdx].width, len("one#100")+1; got != want {
		t.Errorf("ancho de ITEM = %d, want %d (el sufijo más largo + hueco)", got, want)
	}
	if got := lay.prefixOf(model.SectionReview); got != "g" {
		t.Errorf("prefijo de review = %q, want %q", got, "g")
	}
	if got := lay.prefixOf(model.SectionAuthored); got != "" {
		t.Errorf("prefijo de una sección ausente = %q, want vacío", got)
	}

	lay = newRefLayout([]inbox.Section{
		section(model.SectionReview, mkItems(
			"g/un-servicio-con-nombre-larguísimo",
			"g/otro-servicio-con-nombre-larguísimo",
		)...),
	}, prefixCommon)
	if got := lay.cols[colRefIdx].width; got != itemWidthCap {
		t.Errorf("ancho de ITEM = %d, want el tope %d", got, itemWidthCap)
	}

	lay = newRefLayout(nil, prefixCommon)
	if got := lay.cols[colRefIdx].width; got != itemWidthMin {
		t.Errorf("ancho de ITEM sin ítems = %d, want %d", got, itemWidthMin)
	}
}

func TestItemCellsConservanElNumeroAlRecortar(t *testing.T) {
	items := mkItems(
		"APPCITTI/vsocial/backend/un-servicio-con-nombre-larguísimo",
		"APPCITTI/vsocial/backend/otro-servicio-con-nombre-larguísimo",
	)
	lay := newRefLayout([]inbox.Section{section(model.SectionReview, items...)}, prefixCommon)
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
	// What is lost from the front is the group, which the header already declares.
	if got := lay.prefixOf(model.SectionReview); got != "APPCITTI/vsocial/backend" {
		t.Errorf("prefijo = %q, want el grupo que compensó el recorte", got)
	}
}

// The integration test: with one section there is no common prefix.
func TestListLinesMuestranElPrefijoEnUnaLineaFija(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, mkItems(
		"APPCITTI/vsocial/backend/api-gateway",
		"APPCITTI/vsocial/backend/web-app",
	), false))

	var prefixLine, row string
	var all []string
	for _, l := range m.listLines(m.contentWidth()) {
		line := stripANSI(l.text)
		all = append(all, line)
		switch {
		case strings.Contains(line, "APPCITTI/vsocial/backend/"):
			prefixLine = line
		case strings.Contains(line, "api-gateway#100"):
			row = line
		}
	}
	if prefixLine == "" {
		t.Errorf("falta la línea del prefijo común:\n%s", strings.Join(all, "\n"))
	}
	if got := strings.Count(strings.Join(all, "\n"), "APPCITTI/vsocial/backend/"); got != 1 {
		t.Errorf("el prefijo aparece %d veces, want 1 (solo su línea):\n%s", got, strings.Join(all, "\n"))
	}
	if strings.Contains(row, "APPCITTI") {
		t.Errorf("fila = %q, want solo el sufijo", row)
	}
	if !strings.Contains(row, "api-gateway#100") {
		t.Errorf("fila = %q, want %q sin truncar", row, "api-gateway#100")
	}
	// No inner section header with the title and the count: the legend lives elsewhere.
	if joined := strings.Join(all, "\n"); strings.Contains(joined, "Assigned (2)") {
		t.Errorf("la lista no debería llevar cabecera de sección:\n%s", joined)
	}
}

// A section with a single item cannot declare a common prefix.
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
	if !containsSubstring(render("g/p"), "g/p#100") {
		t.Fatal("una ruta corta en sección de un ítem no se pintó entera")
	}

	// A long path: with no prefix to compensate it is clipped at the tail and the "#number"
	// survives.
	var cell string
	for _, line := range render("APPCITTI/vsocial/backend/api-gateway") {
		if strings.Contains(line, "APPCITTI") {
			t.Errorf("sin prefijo común no debe haber línea de prefijo: %q", line)
		}
		if strings.Contains(line, "api-gateway#100") {
			cell = line
		}
	}
	if cell == "" {
		t.Fatal("ninguna línea pintó el sufijo del ítem")
	}
	if !strings.Contains(cell, "…") {
		t.Errorf("celda = %q, want el recorte por la cola", cell)
	}
}

func containsSubstring(lines []string, s string) bool {
	for _, l := range lines {
		if strings.Contains(l, s) {
			return true
		}
	}
	return false
}

func TestRefColInvariantes(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	seg := func() string {
		return strings.Repeat(string(rune('a'+rng.Intn(26))), 1+rng.Intn(12))
	}
	kinds := []model.Section{model.SectionAuthored, model.SectionReview, model.SectionMentions}
	for round := range 300 {
		sections := make([]inbox.Section, 1+rng.Intn(3))
		for si := range sections {
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

		// The invariants are checked in ALL THREE modes, not just common: tail clipping is only
		//correct in common.
		for _, mode := range []prefixMode{prefixCommon, prefixFull, prefixLeaf} {
			lay := newRefLayout(sections, mode)
			w := lay.cols[colRefIdx].width
			if w < itemWidthMin || w > itemWidthCap {
				t.Fatalf("round %d en %v: ancho de ITEM = %d, fuera de [%d, %d]", round, mode, w, itemWidthMin, itemWidthCap)
			}
			if mode != prefixCommon {
				for _, sec := range sections {
					if p := lay.prefixOf(sec.Kind); p != "" {
						t.Fatalf("round %d en %v: la sección %v declara prefijo %q, want vacío", round, mode, sec.Kind, p)
					}
				}
			}
			for _, sec := range sections {
				prefix := lay.prefixOf(sec.Kind)
				for _, it := range sec.Items {
					full := refLabel(it)
					want := refCellText(it, mode, prefix)
					if want == "" {
						t.Fatalf("round %d en %v: etiqueta vacía para %q", round, mode, full)
					}
					if prefix != "" {
						if recon := strings.TrimPrefix(full, prefix+"/"); recon == full {
							t.Fatalf("round %d en %v: el prefijo %q no aplica a %q", round, mode, prefix, full)
						} else if recon != want {
							t.Fatalf("round %d en %v: celda %q, want el sufijo %q de %q", round, mode, want, recon, full)
						}
					}
					cell := itemCells(it, sec.Kind, "yo", lay)[colRefIdx].text
					if cell == "" {
						t.Fatalf("round %d en %v: celda vacía para %q", round, mode, full)
					}
					cut := truncateTail(want, textWidth(w))
					if want == cut {
						if cell != want {
							t.Fatalf("round %d en %v: celda %q, want %q", round, mode, cell, want)
						}
						continue
					}
					if !strings.HasPrefix(cell, "…") {
						t.Fatalf("round %d en %v: celda recortada %q, want el prefijo %q", round, mode, cell, "…")
					}
					tail := want[max(0, len(want)-(textWidth(w)-1)):]
					if !strings.HasSuffix(cell, tail) {
						t.Fatalf("round %d en %v: celda %q pierde la cola %q de %q", round, mode, cell, tail, want)
					}
				}
			}
		}
	}
}
