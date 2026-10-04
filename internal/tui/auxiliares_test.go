package tui

import (
	"testing"

	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

// The identity is forge, host, project and number.
func TestFindItemEncuentraPorIdentidadYNoConfundeItemsDeForgesDistintos(t *testing.T) {
	gh := mkItem("github", "github.com", "acme/widget", "uno", 7, "")
	gl := mkItem("gitlab", "gitlab.example.com", "otro/widget", "otro", 7, "")
	items := []model.Item{gh, gl}

	got, ok := findItem(items, gh.ID())
	if !ok || got.ID() != gh.ID() {
		t.Errorf("findItem del primero devolvió (%v, %v)", got.Number, ok)
	}
	got, ok = findItem(items, gl.ID())
	if !ok || got.ID() != gl.ID() {
		t.Errorf("findItem del segundo devolvió (%v, %v): %+v", got.Number, ok, got)
	}

	// An id that is not there: the ZERO value and false. The zero matters because a bare model.Item{}
	// would otherwise look like a real item.
	id := gh.ID()
	id.Project = "no/existe"
	got, ok = findItem(items, id)
	if ok {
		t.Error("findItem de un id inexistente dio ok")
	}
	if got.ID() != (model.ID{}) {
		t.Errorf("findItem devolvió %+v en vez del valor cero", got.ID())
	}
	if _, ok := findItem(nil, gh.ID()); ok {
		t.Error("findItem sobre nil dio ok")
	}
}

func TestElCicloDeSeccionesCiclaYLaSeccionActivaSeRecuerda(t *testing.T) {
	// The positions map is created in New, so the nil check in setActiveSection is redundant.
	m := newTestModel(t)

	inicio := m.activeSection
	for range sectionCycle {
		m.cycleSection()
	}
	if m.activeSection != inicio {
		t.Errorf("una vuelta completa al ciclo terminó en %v, want %v", m.activeSection, inicio)
	}

	// The position is remembered.
	m2 := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	k := streamKey{forge: "github", section: model.SectionReview, kind: model.ReviewRequested}
	m2.streams[k] = &stream{}
	for _, n := range []int{1, 2, 3} {
		it := mkItem("github", "github.com", "acme/widget", "t", n, "")
		it.Section = model.SectionReview
		m2.streams[k].items = append(m2.streams[k].items, it)
	}
	m2.rebuild()
	m2.cursor = 2
	m2.scroll = 1

	m2.cycleSection() // a otra sección, sin ítems: el cursor se recorta a cero
	m2.setActiveSection(model.SectionReview)
	if m2.cursor != 2 {
		t.Errorf("al volver a review el cursor quedó en %d, want 2: la posición se recuerda "+
			"por sección y no se reinicia", m2.cursor)
	}
	// The scroll is remembered in MEMORY, but with three items in a forty-row terminal there is nothing
	//to scroll.

	m2.setActiveSection(model.SectionMentions)
	if m2.cursor != 0 || m2.scroll != 0 {
		t.Errorf("una sección nueva quedó en cursor=%d scroll=%d, want 0 y 0",
			m2.cursor, m2.scroll)
	}

	var cero Model
	cero.setActiveSection(model.SectionReview)
	if len(cero.pos) != 1 {
		t.Errorf("setActiveSection sobre un modelo de valor cero dejó %d posiciones, want 1: "+
			"escribir en un mapa nil es un PANIC, no un error visible", len(cero.pos))
	}

	m2.activeSection = "una-seccion-inventada"
	if got := m2.sectionCycleIndex(); got != 0 {
		t.Errorf("una sección fuera del ciclo dio índice %d, want 0", got)
	}
	m2.cycleSection()
}

func TestRequestCommentsRechazaLasTresNegativasYSiempreRearmaElTick(t *testing.T) {
	for _, c := range []struct {
		nombre  string
		prepara func(*testing.T) Model
	}{
		{"sin selección", func(t *testing.T) Model {
			return newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
		}},
		{"forge desconocido para el ítem", func(t *testing.T) Model {
			// The item is selected with its forge present and THEN removed from the map. My first version
			//built a gitlab-only model and tried to select a github item, so conSeleccion found nothing
			//and the code took the "no selection" guard one line above: the fixture never arrived.
			m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
			m = conSeleccion(t, m, mkItem("github", "github.com", "acme/widget", "uno", 7, ""))
			if _, ok := m.selected(); !ok {
				t.Fatal("el fixture no sirve: el ítem no quedó seleccionado")
			}
			delete(m.byForge, "github")
			return m
		}},
		{"conversación ya cargada", func(t *testing.T) Model {
			m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
			it := mkItem("github", "github.com", "acme/widget", "uno", 7, "")
			m = conSeleccion(t, m, it)
			m.comments[it.ID()] = &commentState{list: []model.Comment{{Author: "x", Body: "y"}}, ready: true}
			return m
		}},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			m := c.prepara(t)
			if cmd := m.requestComments(); cmd == nil {
				t.Error("no se rearmó el tick: la cadena se corta y el siguiente cambio de " +
					"selección no se nota")
			}
			if cmd := m.commentsCmd(); cmd == nil {
				t.Error("commentsCmd devolvió nil")
			}
		})
	}
}
