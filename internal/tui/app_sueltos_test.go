package tui

import (
	"context"
	"image"
	"strings"
	"testing"

	"prdash/internal/forge/model"
	"prdash/internal/herdr"
	"prdash/internal/testutil"
	"prdash/internal/worktree"
)

// The loose branches left in tui: one-line functions, cursor and view guards.

// The piece that makes the wiring checkable.
func TestWiringDevuelveExactamenteLoQueElModeloTienePuesto(t *testing.T) {
	completa := newTestModel(t)
	completa.mounter = &mounterFalso{}
	completa.simulator = &simuladorFalso{disponible: true}
	completa.graphics = &graphicsFalso{}
	completa.reviewLookup = &reviewLookupFalso{}
	completa.reviewRemover = &reviewRemoverFalso{}

	w := completa.Wiring()
	if w.Mounter == nil || w.Simulator == nil || w.Graphics == nil ||
		w.ReviewLookup == nil || w.ReviewRemover == nil {
		t.Errorf("Wiring perdió alguna dependencia: %+v", w)
	}
	// And they are the SAME instances, which is what makes the accessor worth having.
	if w.Simulator != completa.simulator {
		t.Error("Wiring devolvió otro Simulator: un accessor que copia no sirve para comprobar el cableado")
	}
	if w.Graphics != completa.graphics {
		t.Error("Wiring devolvió otro Graphics")
	}

	vacia := newTestModel(t)
	vacia.mounter = nil
	vacia.simulator = nil
	vacia.graphics = nil
	wv := vacia.Wiring()
	if wv.Mounter != nil || wv.Simulator != nil || wv.Graphics != nil ||
		wv.ReviewLookup != nil || wv.ReviewRemover != nil {
		t.Errorf("Wiring con el modelo vacío devolvió algo: %+v", wv)
	}
}

// Honest degradation.
func TestSinAlturaConocidaElScrollAvanzaSeisFilas(t *testing.T) {
	m := newTestModel(t)
	m.height = 0 // terminal sin tamaño todavía

	if filas := m.pageRows(); filas != 6 {
		t.Errorf("sin altura conocida pageRows = %d, want 6", filas)
	}
	m.scroll = 0
	m.scroll += m.pageRows()
	if m.scroll != 6 {
		t.Errorf("el scroll quedó en %d", m.scroll)
	}

	m2 := newTestModel(t)
	m2.height = 40
	if filas := m2.pageRows(); filas == 6 {
		t.Error("con altura conocida sigue usando el default de seis filas")
	}
}

// The end of a cycle.
func TestUnForgeQueTerminaDeCargarDejaDeEstarEnLoading(t *testing.T) {
	m := modeloConStatusesCargando(t)

	// A current github forgeDoneMsg clears github's loading and NOT gitlab's.
	salida := send(t, m, forgeDoneMsg{cycle: 3, forge: "github"})
	got := salida
	if got.statuses["github"].loading {
		t.Error("forgeDoneMsg de github no apagó su loading: el spinner de ese forge no pararía")
	}
	if !got.statuses["gitlab"].loading {
		t.Error("forgeDoneMsg de github apagó el loading de gitlab, que sigue cargando")
	}

	desconocido := send(t, m, forgeDoneMsg{cycle: 3, forge: "bitbucket"})
	if _, ok := desconocido.statuses["bitbucket"]; ok {
		t.Error("forgeDoneMsg de un forge desconocido lo creó en el mapa")
	}

	// A STALE one touches neither, and with a NEW model, because the statuses map is fresh.
	obsoleta := send(t, modeloConStatusesCargando(t), forgeDoneMsg{cycle: 1, forge: "github"})
	if !obsoleta.statuses["github"].loading || !obsoleta.statuses["gitlab"].loading {
		t.Error("un forgeDoneMsg obsoleto apagó un loading: apagaría el spinner de un forge " +
			"que sigue cargando de verdad")
	}
}

func TestAbrirEnElNavegadorSinURLSeNiegaYLoDice(t *testing.T) {
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	salida, cmd := pulsar(t, m, "o")
	got := salida
	if cmd != nil {
		t.Error("abrir sin selección devolvió comando: ejecutaría un visor sin nada")
	}
	// The warning does not separate "no selection" from "the item has no URL": one guard.
	if av := lastToast(got); !strings.Contains(av, "no URL") {
		t.Errorf("el aviso %q no dice que no hay nada que abrir", av)
	}

	m2 := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	sinURL := mkItem("github", "github.com", "acme/widget", "uno", 7, "")
	sinURL.URL = ""
	m2 = conSeleccion(t, m2, sinURL)

	salida2, cmd2 := pulsar(t, m2, "o")
	got2 := salida2
	if cmd2 != nil {
		t.Error("abrir un ítem sin URL devolvió comando")
	}
	if av := lastToast(got2); !strings.Contains(av, "no URL") {
		t.Errorf("el aviso %q no dice que el ítem no tiene URL", av)
	}
	if lastToastLevel(got2) != toastWarning {
		t.Errorf("nivel %v, want aviso", lastToastLevel(got2))
	}
}

type graphicsFalso struct{}

func (graphicsFalso) Available() bool                     { return false }
func (graphicsFalso) CellSize(context.Context) (int, int) { return 1, 2 }
func (graphicsFalso) SetImage(context.Context, string, image.Image, herdr.Placement) error {
	return nil
}
func (graphicsFalso) Clear(context.Context, string) error { return nil }

type reviewLookupFalso struct{}

func (reviewLookupFalso) ActiveReview(model.Item) (worktree.Worktree, bool) {
	return worktree.Worktree{}, false
}

type reviewRemoverFalso struct{}

func (reviewRemoverFalso) RemoveReview(context.Context, model.Item) (bool, string, error) {
	return false, "", nil
}

func modeloConStatusesCargando(t *testing.T) Model {
	t.Helper()
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	m.cycle = 3
	m.statuses = map[string]*forgeStatus{
		"github": {forge: "github", host: "github.com", loading: true},
		"gitlab": {forge: "gitlab", host: "gitlab.example.com", loading: true},
	}
	return m
}
