package tui

import (
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

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
		if strings.Contains(bar, "p prefix") && !strings.Contains(bar, want.hint) {
			t.Errorf("la barra muestra la etiqueta sin el modo: %q", bar)
		}
	}
}

// The same promise at the widths where the bar actually fits.
func TestElHintNombraElModoAAnchosUsables(t *testing.T) {
	for _, width := range []int{44, 56, 64, 80, 100, 160} {
		m := newTestModel(t, ghAdapter())
		m.width = width
		m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested,
			mkItems("APPCITTI/vsocial/backend/api-gateway"), false))

		for _, want := range []string{"p prefix: common", "p prefix: full", "p prefix: leaf"} {
			bar := stripANSI(strings.Join(m.hintLines(), " "))
			if !strings.Contains(bar, want) {
				t.Errorf("ancho %d: la barra = %q, want que contenga %q", width, bar, want)
				break
			}
			m = press(t, m, "p")
		}
	}
}

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

// With the merge armed the hints box stops being help.
func TestElHintNoSeColaEnLaConfirmacionDeMerge(t *testing.T) {
	// The merge fixture is what registers the items in the adapter: without it the guard would cut.
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
