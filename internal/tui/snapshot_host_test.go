package tui

import (
	"testing"

	"prdash/internal/cache"
	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

// Each stream is saved with its forge's host.
func TestElSnapshotGuardaElHostDeCadaForge(t *testing.T) {
	adapt := &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"}
	m := newTestModel(t, adapt)

	if st := m.statuses["github"]; st == nil {
		t.Fatal("el adapter registrado no tiene estado, y el test necesita uno con host")
	}

	m.applySnapshot(snapshotCon(mkItem("github", "github.com", "acme/widget", "uno", 1, "")))
	m.rebuild()

	f := m.snapshot()
	if len(f.Streams) == 0 {
		t.Fatal("el snapshot salió sin streams, y el test necesita al menos uno")
	}

	vistos := map[string]bool{}
	for _, s := range f.Streams {
		vistos[s.Forge] = true
		if s.Forge == "github" && s.Host == "" {
			t.Errorf("el stream de github se guardó sin host. El host es lo que separa "+
				"dos streams del mismo forge en dos instancias, y sin él la apertura "+
				"siguiente no puede saber cuál es cuál: %+v", s)
		}
		if s.Forge == "github" && s.Host != "github.com" {
			t.Errorf("el stream de github se guardó con host %q, want github.com", s.Host)
		}
	}
	if !vistos["github"] {
		t.Error("el snapshot no tiene stream de github")
	}

	m.applySnapshot(cache.File{Streams: []cache.Stream{{
		Forge:   "gitlab",
		Host:    "gitlab.com",
		Section: "review",
		Kind:    "requested",
		Items:   nil,
	}}})
	m.rebuild()

	f = m.snapshot()
	conHost, sinEstado := 0, 0
	for _, s := range f.Streams {
		if s.Forge == "github" && s.Host == "github.com" {
			conHost++
		}
		if s.Forge == "gitlab" && s.Host == "" {
			sinEstado++
		}
	}
	if conHost == 0 {
		t.Error("después de meter un forge sin estado, el stream de github perdió su host")
	}
	if sinEstado == 0 {
		t.Error("el stream de un forge sin estado no llegó al snapshot: perderlo entero " +
			"sería peor que guardarlo sin host")
	}
}

// The snapshot comes out sorted by forge.
func TestElSnapshotVaOrdenadoPorForgeSeccionYTipo(t *testing.T) {
	adapt := &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"}
	m := newTestModel(t, adapt)

	// The keys are written in an order that is NOT the sorted one, and there are SEVERAL kinds.
	claves := []struct{ forge, section, kind string }{
		{"github", "authored", "commented"},
		{"gitlab", "review", "assigned"},
		{"github", "mentions", "requested"},
		{"github", "authored", "requested"},
		{"gitlab", "authored", "approved"},
		{"github", "authored", "approved"},
		{"bitbucket", "review", "assigned"},
		{"github", "review", "commented"},
		{"github", "review", "requested"},
		{"gitlab", "review", "requested"},
		{"gitlab", "review", "approved"},
		{"github", "review", "approved"},
		{"gitlab", "authored", "requested"},
		{"gitlab", "review", "commented"},
		{"github", "authored", "assigned"},
	}
	for _, c := range claves {
		m.applySnapshot(cache.File{Streams: []cache.Stream{{
			Forge: c.forge, Section: model.Section(c.section), Kind: model.ReviewKind(c.kind),
		}}})
	}
	m.rebuild()

	want := []string{
		"bitbucket/review/assigned",
		"github/authored/approved",
		"github/authored/assigned",
		"github/authored/commented",
		"github/authored/requested",
		"github/mentions/requested",
		"github/review/approved",
		"github/review/commented",
		"github/review/requested",
		"gitlab/authored/approved",
		"gitlab/authored/requested",
		"gitlab/review/approved",
		"gitlab/review/assigned",
		"gitlab/review/commented",
		"gitlab/review/requested",
	}

	f := m.snapshot()
	if len(f.Streams) != len(want) {
		t.Fatalf("el snapshot tiene %d streams, want %d", len(f.Streams), len(want))
	}
	for i, s := range f.Streams {
		got := s.Forge + "/" + string(s.Section) + "/" + string(s.Kind)
		if got != want[i] {
			t.Errorf("stream %d es %q, want %q. El snapshot se ordena por forge, luego "+
				"sección y luego tipo, y el orden es el de las letras: se compara entre "+
				"ejecuciones para saber si el inbox ha cambiado, y un orden invertido "+
				"produce un «ha cambiado» cada vez que se abre",
				i, got, want[i])
		}
	}
}
