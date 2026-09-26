package tui

import (
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

// listModelWithItems monta la sección activa de review con las rutas dadas.
func listModelWithItems(t *testing.T, projects ...string) Model {
	t.Helper()
	m := newTestModel(t, ghAdapter())
	return send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested,
		mkItems(projects...), false))
}

// listaDe count es la lista del modelo con un modo de prefijo concreto.
func listaDe(m Model) []string {
	lines := make([]string, 0, 8)
	for _, l := range m.listLines(m.contentWidth()) {
		lines = append(lines, stripANSI(l.text))
	}
	return lines
}

// TestListLinesPintanElPrefijoSoloEnCommon es el escenario de la línea de prefijo
// por modo: en common declara el grupo común de la sección, y en full y leaf no
// hay línea porque no hay prefijo que declarar.
func TestListLinesPintanElPrefijoSoloEnCommon(t *testing.T) {
	for _, mode := range []prefixMode{prefixFull, prefixLeaf} {
		m := listModelWithItems(t, "APPCITTI/vsocial/backend/api-gateway", "APPCITTI/vsocial/backend/web-app")
		m.prefixMode = mode
		for _, line := range listaDe(m) {
			if strings.Contains(line, "APPCITTI/vsocial/backend/") {
				t.Errorf("en %v se pintó una línea de prefijo: %q", mode, line)
			}
		}
	}

	m := listModelWithItems(t, "APPCITTI/vsocial/backend/api-gateway", "APPCITTI/vsocial/backend/web-app")
	joined := strings.Join(listaDe(m), "\n")
	if !strings.Contains(joined, "APPCITTI/vsocial/backend/") {
		t.Errorf("en common falta la línea de prefijo:\n%s", joined)
	}
	// El prefijo se declara una sola vez: al inicio del cuerpo, no en cada fila.
	if got := strings.Count(joined, "APPCITTI/vsocial/backend/"); got != 1 {
		t.Errorf("el prefijo aparece %d veces, want 1:\n%s", got, joined)
	}
}

// TestFullYLeafPintanLaReferenciaCompletaOLaHoja comprueba la etiqueta que ve el
// usuario en cada modo, extremo a extremo y no solo en la unidad.
func TestFullYLeafPintanLaReferenciaCompletaOLaHoja(t *testing.T) {
	m := listModelWithItems(t, "APPCITTI/vsocial/backend/api-gateway", "APPCITTI/vsocial/backend/web-app")

	m.prefixMode = prefixFull
	full := strings.Join(listaDe(m), "\n")
	if !strings.Contains(full, "api-gateway#100") {
		t.Errorf("full no pintó la referencia del ítem:\n%s", full)
	}
	// La fila lleva el grupo, no solo la hoja: es lo que distingue full de leaf.
	if !strings.Contains(full, "vsocial") {
		t.Errorf("full no pintó el grupo en la celda:\n%s", full)
	}

	m.prefixMode = prefixLeaf
	leaf := strings.Join(listaDe(m), "\n")
	if !strings.Contains(leaf, "api-gateway#100") {
		t.Errorf("leaf no pintó la hoja con su número:\n%s", leaf)
	}
	// Ni una sola vez la ruta: el modo se define por tirarla.
	if strings.Contains(leaf, "vsocial") {
		t.Errorf("leaf still muestra el grupo del proyecto:\n%s", leaf)
	}
}

// TestFullYLeafRecuperanLaLineaDelPrefijo fija el alto: quitar la línea de
// prefijo devuelve ese alto a la lista, así que en full y leaf hay una línea
// menos que en common.
func TestFullYLeafRecuperanLaLineaDelPrefijo(t *testing.T) {
	m := listModelWithItems(t, "APPCITTI/vsocial/backend/api-gateway", "APPCITTI/vsocial/backend/web-app")

	m.prefixMode = prefixCommon
	conPrefijo := len(m.listLines(m.contentWidth()))
	for _, mode := range []prefixMode{prefixFull, prefixLeaf} {
		m.prefixMode = mode
		sinPrefijo := len(m.listLines(m.contentWidth()))
		if sinPrefijo != conPrefijo-1 {
			t.Errorf("en %v la lista tiene %d líneas, want %d (una menos que en common, %d)",
				mode, sinPrefijo, conPrefijo-1, conPrefijo)
		}
	}
}

// TestCommonSinPrefijoComunSeVeIgualQueFull es la degradación: una sección sin
// nada en común no puede declarar prefijo, así que common cae a la referencia
// completa — que es exactamente full — sin inventar nada ni repetir la ruta.
func TestCommonSinPrefijoComunSeVeIgualQueFull(t *testing.T) {
	// "acme/one" y "other/one": sin nada en común y con la MISMA hoja, que es el
	// caso donde la degradación se nota (leaf no podría distinguirlos).
	m := listModelWithItems(t, "acme/one", "other/one")

	m.prefixMode = prefixCommon
	common := strings.Join(listaDe(m), "\n")
	m.prefixMode = prefixFull
	full := strings.Join(listaDe(m), "\n")

	if common != full {
		t.Errorf("sin prefijo común, common y full deberían verse igual:\ncommon:\n%s\nfull:\n%s", common, full)
	}
	if !strings.Contains(common, "acme/one#100") {
		t.Errorf("la celda no pintó la ruta completa:\n%s", common)
	}
	// El grupo aparece una sola vez por ítem, en su celda: no hay línea de prefijo
	// que lo repita.
	if got := strings.Count(common, "acme"); got != 1 {
		t.Errorf("el grupo aparece %d veces, want 1:\n%s", got, common)
	}
}

// TestCommonSinPrefijoComunNoRepiteLaRutaEnDosSitios evita el modo de fallo de
// "degradar a common pero dejar el grupo en la línea": sin prefijo no hay línea,
// y el grupo vive solo en la celda.
func TestCommonSinPrefijoComunNoRepiteLaRutaEnDosSitios(t *testing.T) {
	m := listModelWithItems(t, "acme/one", "other/one")
	m.prefixMode = prefixCommon
	for _, line := range listaDe(m) {
		// La única línea que menciona el prefijo común (vacío) sería una línea
		// "· /" o similar: aquí basta con que ninguna línea sea solo el punto.
		if strings.TrimSpace(line) == "·" || strings.HasPrefix(strings.TrimSpace(line), "· /") {
			t.Errorf("se pintó una línea de prefijo sin contenido: %q", line)
		}
	}
}
