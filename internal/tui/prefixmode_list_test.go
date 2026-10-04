package tui

import (
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

func listModelWithItems(t *testing.T, projects ...string) Model {
	t.Helper()
	m := newTestModel(t, ghAdapter())
	return send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested,
		mkItems(projects...), false))
}

func listaDe(m Model) []string {
	lines := make([]string, 0, 8)
	for _, l := range m.listLines(m.contentWidth()) {
		lines = append(lines, stripANSI(l.text))
	}
	return lines
}

// The prefix line per mode: in common there is one, in the other two there is not.
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
	if got := strings.Count(joined, "APPCITTI/vsocial/backend/"); got != 1 {
		t.Errorf("el prefijo aparece %d veces, want 1:\n%s", got, joined)
	}
}

func TestFullYLeafPintanLaReferenciaCompletaOLaHoja(t *testing.T) {
	m := listModelWithItems(t, "APPCITTI/vsocial/backend/api-gateway", "APPCITTI/vsocial/backend/web-app")

	m.prefixMode = prefixFull
	full := strings.Join(listaDe(m), "\n")
	if !strings.Contains(full, "api-gateway#100") {
		t.Errorf("full no pintó la referencia del ítem:\n%s", full)
	}
	if !strings.Contains(full, "vsocial") {
		t.Errorf("full no pintó el grupo en la celda:\n%s", full)
	}

	m.prefixMode = prefixLeaf
	leaf := strings.Join(listaDe(m), "\n")
	if !strings.Contains(leaf, "api-gateway#100") {
		t.Errorf("leaf no pintó la hoja con su número:\n%s", leaf)
	}
	if strings.Contains(leaf, "vsocial") {
		t.Errorf("leaf still muestra el grupo del proyecto:\n%s", leaf)
	}
}

// Removing the prefix line returns that height to the list.
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

func TestCommonSinPrefijoComunSeVeIgualQueFull(t *testing.T) {
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
	if got := strings.Count(common, "acme"); got != 1 {
		t.Errorf("el grupo aparece %d veces, want 1:\n%s", got, common)
	}
}

// Degrading to common must not repeat the path in two places.
func TestCommonSinPrefijoComunNoRepiteLaRutaEnDosSitios(t *testing.T) {
	m := listModelWithItems(t, "acme/one", "other/one")
	m.prefixMode = prefixCommon
	for _, line := range listaDe(m) {
		if strings.TrimSpace(line) == "·" || strings.HasPrefix(strings.TrimSpace(line), "· /") {
			t.Errorf("se pintó una línea de prefijo sin contenido: %q", line)
		}
	}
}
