package tui

import (
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

// TestElHintNombraElModoActual comprueba extremo a extremo lo que promete la
// barra de atajos: `p` no sale a secas, sino con el modo en el que está la
// columna ITEM, para no tener que contar pulsaciones. Sin esto, el usuario
// tendría que recordar cuántas lleva.
func TestElHintNombraElModoActual(t *testing.T) {
	m := listModelWithItems(t, "APPCITTI/vsocial/backend/api-gateway")

	for _, want := range []struct {
		tecla string
		hint  string
	}{
		{"", "p prefix: common"}, // el estado inicial, sin pulsar nada
		{"p", "p prefix: full"},
		{"p", "p prefix: leaf"},
		{"p", "p prefix: common"},
	} {
		if want.tecla != "" {
			m = press(t, m, want.tecla)
		}
		bar := stripANSI(strings.Join(m.hintLines(), " "))
		if !strings.Contains(bar, want.hint) {
			t.Errorf("la barra = %q, want que contenga %q", bar, want.hint)
		}
		// Y no puede quedarse solo con la tecla a secas.
		if strings.Contains(bar, "p prefix ") {
			t.Errorf("la barra muestra la etiqueta a secas, sin el modo: %q", bar)
		}
	}
}

// TestElHintSigueAlRebindDelModo: la tecla sale de [keybindings] y el nombre del
// modo del estado. Los dos tienen que moverse a la vez, o la barra miente.
func TestElHintSigueAlRebindDelModo(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m.cfg.Keybindings["prefix-mode"] = "P"
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested,
		mkItems("acme/widget"), false))
	m = press(t, m, "P")
	m = press(t, m, "P")

	bar := stripANSI(strings.Join(m.hintLines(), " "))
	if !strings.Contains(bar, "P prefix: leaf") {
		t.Errorf("la barra = %q, want que contenga %q", bar, "P prefix: leaf")
	}
	if strings.Contains(bar, "p prefix") {
		t.Errorf("la barra sigue mostrando la tecla anterior: %q", bar)
	}
}

// TestElHintNoSeColaEnLaConfirmacionDeMerge: con el merge armado la caja de
// atajos deja de ser ayuda y pasa a ser la Confirmación. El modo no puede
// aparecer ahí: la Confirmación es la pregunta antes de una segunda tecla, y un
// `p prefix: leaf` en medio se leería como un modo de merge.
func TestElHintNoSeColaEnLaConfirmacionDeMerge(t *testing.T) {
	// El fixture de merge es el que registra los ítems en el adapter: sin él el
	// guard de acción rechaza el armado y la caja nunca pasa a Confirmación.
	f := newMergeFixture(t, mergeItems()...)
	m := f.m
	m = press(t, m, "p")
	m = press(t, m, "p") // leaf
	m = press(t, m, "m") // arma el merge: la caja pasa a ser Confirmación
	if !m.mergeArmed {
		t.Fatal("el merge no se armó: el test no probaría nada")
	}

	bar := stripANSI(strings.Join(m.hintLines(), " "))
	if strings.Contains(bar, "prefix") {
		t.Errorf("la Confirmación de merge muestra el modo de prefijo: %q", bar)
	}
	if !strings.Contains(bar, "merge") {
		t.Errorf("la caja no muestra la Confirmación: %q", bar)
	}
}
