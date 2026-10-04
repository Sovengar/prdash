package tui

import (
	"strings"
	"testing"
	"unicode/utf8"

	"prdash/internal/forge/model"
	"prdash/internal/inbox"
)

func TestRefLeafReduceLaRutaALaHoja(t *testing.T) {
	for _, tc := range []struct {
		name  string
		items []model.Item
		want  []string
	}{
		{"con subgrupo largo", mkItems("APPCITTI/vsocial/backend/api-gateway"), []string{"api-gateway#100"}},
		{"owner/repo de github", mkItems("acme/widget"), []string{"widget#100"}},
		{"sin subgrupo", mkItems("myrepo"), []string{"myrepo#100"}},
		{"varios items", mkItems("g/a/one", "g/a/two", "h/b/three"), []string{"one#100", "two#101", "three#102"}},
		{"proyecto vacío", mkItems("", "g/p"), []string{"#100", "p#101"}},
	} {
		for i, it := range tc.items {
			if got := refLeaf(it); got != tc.want[i] {
				t.Errorf("%s[%d]: refLeaf = %q, want %q", tc.name, i, got, tc.want[i])
			}
		}
	}
}

func TestPrefixModeCiclaVuelveAlOrigen(t *testing.T) {
	m := prefixCommon
	for i, want := range []prefixMode{prefixFull, prefixLeaf, prefixCommon, prefixFull} {
		m = m.next()
		if m != want {
			t.Fatalf("pulso %d: modo = %v, want %v", i+1, m, want)
		}
	}
}

func TestPrefixModeStringEsElNombreDelHint(t *testing.T) {
	for mode, want := range map[prefixMode]string{
		prefixCommon: "common",
		prefixFull:   "full",
		prefixLeaf:   "leaf",
	} {
		if got := mode.String(); got != want {
			t.Errorf("%d.String() = %q, want %q", mode, got, want)
		}
	}
}

func TestNewRefLayoutDeclaraPrefijoSoloEnCommon(t *testing.T) {
	sections := []inbox.Section{section(model.SectionReview, mkItems(
		"APPCITTI/vsocial/backend/api-gateway",
		"APPCITTI/vsocial/backend/web-app",
	)...)}
	if got := newRefLayout(sections, prefixCommon).prefixOf(model.SectionReview); got != "APPCITTI/vsocial/backend" {
		t.Errorf("prefijo en common = %q, want %q", got, "APPCITTI/vsocial/backend")
	}
	for _, mode := range []prefixMode{prefixFull, prefixLeaf} {
		if got := newRefLayout(sections, mode).prefixOf(model.SectionReview); got != "" {
			t.Errorf("prefijo en %v = %q, want vacío (no hay prefijo que declarar)", mode, got)
		}
	}
}

func TestNewRefLayoutDimensionaITEMPorModo(t *testing.T) {
	sections := []inbox.Section{section(model.SectionReview,
		mkItems("APPCITTI/vsocial/backend/api-gateway", "APPCITTI/vsocial/web-app")...)}

	for _, tc := range []struct {
		mode prefixMode
		want int
		why  string
	}{
		// "backend/api-gateway#100" = 23 runes + hueco.
		{prefixCommon, 24, "el sufijo más largo + hueco"},
		// "APPCITTI/vsocial/backend/api-gateway#100" = 40 runes + hueco, topado.
		{prefixFull, itemWidthCap, "la referencia completa + hueco, acotada al tope"},
		// "api-gateway#100" = 15 runes + hueco.
		{prefixLeaf, 16, "la hoja más larga + hueco"},
	} {
		if got := newRefLayout(sections, tc.mode).cols[colRefIdx].width; got != tc.want {
			t.Errorf("%v: ancho de ITEM = %d, want %d (%s)", tc.mode, got, tc.want, tc.why)
		}
	}
}

func TestRefCellTextPorModo(t *testing.T) {
	conGrupo := mkItems("APPCITTI/vsocial/backend/api-gateway", "APPCITTI/vsocial/backend/web-app")
	prefix := sectionPrefix(conGrupo)
	if got, want := refCellText(conGrupo[0], prefixCommon, prefix), "api-gateway#100"; got != want {
		t.Errorf("common = %q, want %q", got, want)
	}
	if got, want := refCellText(conGrupo[0], prefixFull, ""), "APPCITTI/vsocial/backend/api-gateway#100"; got != want {
		t.Errorf("full = %q, want %q", got, want)
	}
	if got, want := refCellText(conGrupo[0], prefixLeaf, ""), "api-gateway#100"; got != want {
		t.Errorf("leaf = %q, want %q", got, want)
	}

	sinGrupo := mkItems("acme/one", "other/one")
	if p := sectionPrefix(sinGrupo); p != "" {
		t.Fatalf("el fixture no debería tener prefijo común, tiene %q", p)
	}
	celdaCommon := refCellText(sinGrupo[0], prefixCommon, "")
	celdaFull := refCellText(sinGrupo[0], prefixFull, "")
	if celdaCommon != celdaFull || celdaCommon != "acme/one#100" {
		t.Errorf("degradación: common = %q, full = %q, want ambos %q", celdaCommon, celdaFull, "acme/one#100")
	}
}

func TestFullRecortaPorLaColaYConservaElNumero(t *testing.T) {
	items := mkItems("APPCITTI/vsocial/backend/api-gateway", "APPCITTI/vsocial/web-app")
	lay := newRefLayout([]inbox.Section{section(model.SectionReview, items...)}, prefixFull)
	celda := itemCells(items[0], model.SectionReview, "yo", lay)[colRefIdx].text

	if want := "…/vsocial/backend/api-gateway#100"; celda != want {
		t.Errorf("celda ITEM = %q, want %q", celda, want)
	}
	if got := utf8.RuneCountInString(celda); got > itemWidthCap {
		t.Errorf("celda = %q (%d runes), excede el tope %d", celda, got, itemWidthCap)
	}
}

// The limit of the mode, with its real case: two repos whose leaf repeats.
func TestLeafNoDesambiguaHojasRepetidas(t *testing.T) {
	items := mkItems("acme/one", "other/one")
	lay := newRefLayout([]inbox.Section{section(model.SectionReview, items...)}, prefixLeaf)

	if got := itemCells(items[0], model.SectionReview, "yo", lay)[colRefIdx].text; got != "one#100" {
		t.Errorf("celda del primero = %q, want %q", got, "one#100")
	}
	if got := itemCells(items[1], model.SectionReview, "yo", lay)[colRefIdx].text; got != "one#101" {
		t.Errorf("celda del segundo = %q, want %q", got, "one#101")
	}
}

func TestRefCellTextNoRepiteLaRutaEnLaLinea(t *testing.T) {
	items := mkItems("acme/one", "other/one")
	lay := newRefLayout([]inbox.Section{section(model.SectionReview, items...)}, prefixCommon)
	if got := lay.prefixOf(model.SectionReview); got != "" {
		t.Fatalf("prefijo = %q, want vacío", got)
	}
	celda := itemCells(items[0], model.SectionReview, "yo", lay)[colRefIdx].text
	if got := strings.Count(celda, "acme"); got != 1 {
		t.Errorf("la celda dice %q: el grupo aparece %d veces, want 1", celda, got)
	}
}

// A real degradation of `full`.
func TestFullPierdeLaColumnaITEMEnTerminalMuyEstrecho(t *testing.T) {
	items := mkItems("APPCITTI/vsocial/backend/api-gateway", "APPCITTI/vsocial/web-app")
	segs := []inbox.Section{section(model.SectionReview, items...)}

	const umbral = 48 // colForge(14) + itemWidthCap(34)
	for _, tc := range []struct {
		mode       prefixMode
		inner      int
		quiereITEM bool
	}{
		{prefixCommon, 38, true},
		{prefixLeaf, 38, true},
		{prefixFull, 47, false},
		{prefixFull, umbral, true},
	} {
		lay := newRefLayout(segs, tc.mode)
		hay := strings.Contains(titlesDe(lay, tc.inner), "ITEM")
		if hay != tc.quiereITEM {
			t.Errorf("%v a %d columnas interiores: ITEM presente = %v, want %v (columnas: %s)",
				tc.mode, tc.inner, hay, tc.quiereITEM, titlesDe(lay, tc.inner))
		}
	}
}

func titlesDe(l refLayout, inner int) string {
	var out []string
	for _, c := range l.cols[:fitColumns(l, inner)] {
		out = append(out, c.title)
	}
	return strings.Join(out, ",")
}
