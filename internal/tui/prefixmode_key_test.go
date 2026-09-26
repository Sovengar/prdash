package tui

import (
	"strconv"
	"testing"

	"prdash/internal/forge/model"
)

// TestPrefixModeCiclaConLaTecla es el escenario central de la tecla: `p` recorre
// los tres modos y vuelve al inicio. Se pulsa de verdad, no se asigna el campo,
// porque el contrato es también que la tecla llegue al ciclo.
func TestPrefixModeCiclaConLaTecla(t *testing.T) {
	m := listModelWithItems(t, "APPCITTI/vsocial/backend/api-gateway")

	if m.prefixMode != prefixCommon {
		t.Fatalf("modo inicial = %v, want common (el valor por defecto)", m.prefixMode)
	}
	for _, want := range []prefixMode{prefixFull, prefixLeaf, prefixCommon, prefixFull} {
		m = press(t, m, "p")
		if m.prefixMode != want {
			t.Fatalf("tras pulsar p: modo = %v, want %v", m.prefixMode, want)
		}
	}
}

// TestPrefixModeSaleDeLaConfig comprueba que la tecla no está cableada: con la
// acción reasignada, la tecla nueva cicla y la vieja deja de hacerlo. Es el
// mismo guard que section-next, por el mismo motivo: una barra que anuncia una
// tecla que no hace nada es peor que no anunciarla.
func TestPrefixModeSaleDeLaConfig(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m.cfg.Keybindings["prefix-mode"] = "P"
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested,
		mkItems("acme/widget"), false))

	m = press(t, m, "P")
	if m.prefixMode != prefixFull {
		t.Fatalf("con el rebind a P, el modo = %v, want full", m.prefixMode)
	}
	antes := m.prefixMode
	m = press(t, m, "p")
	if m.prefixMode != antes {
		t.Errorf("la tecla vieja %q aún cicla el modo: %v -> %v", "p", antes, m.prefixMode)
	}
}

// TestPrefixModeNoLoReiniciaElCambioDeSeccion: el modo es global, no por sección.
// Tabular a otra sección, cuyo prefijo común es otro, no puede devolver la vista a
// common.
func TestPrefixModeNoLoReiniciaElCambioDeSeccion(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested,
		mkItems("APPCITTI/vsocial/backend/api-gateway"), false))
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "",
		ghItems("otro/proyecto"), false))

	m = press(t, m, "p")
	m = press(t, m, "p")
	if m.prefixMode != prefixLeaf {
		t.Fatalf("modo = %v, want leaf", m.prefixMode)
	}

	antes := m.activeSection
	m = press(t, m, "tab")
	if m.activeSection == antes {
		t.Fatalf("tab no cambió de sección (sigue en %v): el test no probaría nada", m.activeSection)
	}
	if m.prefixMode != prefixLeaf {
		t.Errorf("cambiar de sección reinició el modo: %v, want leaf (es global)", m.prefixMode)
	}
	// Y la vuelta completa a la sección original tampoco.
	for range 2 {
		m = press(t, m, "tab")
	}
	if m.activeSection != antes {
		t.Fatalf("no se volvió a la sección original (=%v, ahora %v): el test no probaría nada", antes, m.activeSection)
	}
	if m.prefixMode != prefixLeaf {
		t.Errorf("la vuelta completa reinició el modo: %v, want leaf", m.prefixMode)
	}
}

// TestPrefixModeNoDisparaOtrasAcciones comprueba que `p` es solo un ciclo de
// vista. Es el guard de que la tecla nueva no se cuele en ninguna otra: si `p`
// acabara en approve o en refresh, la barra sería veraz pero la vista haría cosas
// que el usuario no pidió.
func TestPrefixModeNoDisparaOtrasAcciones(t *testing.T) {
	m := listModelWithItems(t, "acme/widget")
	m.loading = false
	m.toast = newToastManager()

	m = press(t, m, "p")

	if m.mergeArmed {
		t.Error("`p` armar el merge; no es una tecla de modo de merge")
	}
	if m.mergeArmedID != (model.ID{}) {
		t.Errorf("`p` dejó un merge armado sobre %v", m.mergeArmedID)
	}
	if m.loading {
		t.Error("`p` disparó un refresco")
	}
	if m.actionBusy {
		t.Error("`p` lanzó una acción de forge")
	}
	if len(m.toast.toasts) != 0 {
		t.Errorf("`p` dejó un aviso: %v", m.toast.toasts)
	}
	if m.cursor != 0 {
		t.Errorf("`p` movió el cursor a %d, want 0", m.cursor)
	}
}

// TestPrefixModeConservaElCursorYLaVentana comprueba el efecto secundario que sí
// importa: cambiar de modo no mueve el cursor de ítem. Y como full y leaf quitan
// la línea de prefijo, la lista se acorta: sin el syncScroll del ciclo, una
// lista desplazada dejaría la fila del cursor fuera de la ventana.
func TestPrefixModeConservaElCursorYLaVentana(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	items := make([]model.Item, 0, 30)
	for i := range 30 {
		items = append(items, mkItem("github", "github.com",
			"APPCITTI/vsocial/backend/svc"+strconv.Itoa(i), "T", 100+i, ""))
	}
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, items, false))
	m.height = 12 // ventana corta: obliga a desplazar

	m = press(t, m, "end")
	antes, desplazamiento := m.cursor, m.scroll
	if antes == 0 || desplazamiento == 0 {
		t.Fatalf("el cursor (=%d) o el scroll (=%d) no se movieron: el test no probaría nada", antes, desplazamiento)
	}

	m = press(t, m, "p") // common -> full: la lista pierde la línea de prefijo

	if m.cursor != antes {
		t.Errorf("cambiar de modo movió el cursor a %d, want %d", m.cursor, antes)
	}
	linea := cursorLine(m.listLines(m.contentWidth()), m.cursor)
	ventana := m.layout().bodyLines
	if linea < m.scroll || linea >= m.scroll+ventana {
		t.Errorf("la fila del cursor (línea %d) quedó fuera de la ventana [%d, %d); scroll %d -> %d",
			linea, m.scroll, m.scroll+ventana, desplazamiento, m.scroll)
	}
}

// TestPrefixModeArrancaEnCommonSinPersistir documenta la decisión de no
// persistirlo: el modo es estado de la sesión y un modelo nuevo arranca en
// common, que es el comportamiento heredado.
func TestPrefixModeArrancaEnCommonSinPersistir(t *testing.T) {
	m := listModelWithItems(t, "acme/widget")
	m = press(t, m, "p")
	if m.prefixMode == prefixCommon {
		t.Fatal("`p` no cambió el modo: el test no probaría nada")
	}
	if nuevo := newTestModel(t, ghAdapter()); nuevo.prefixMode != prefixCommon {
		t.Errorf("un modelo nuevo arranca en %v, want common (el modo no se persiste)", nuevo.prefixMode)
	}
}
