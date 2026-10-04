package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"prdash/internal/cache"
	"prdash/internal/config"
	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

func TestElTickSoloSeArmaConAutoRefresh(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	conIntervalo := config.Defaults()
	conIntervalo.RefreshInterval = 2 * time.Second
	m := New(conIntervalo, nil)
	if !m.tickPending {
		t.Error("con intervalo de refresco no quedó ningun tick pendiente: no se va a consultar nada")
	}

	// Zero is the default for "off", and that is the case worth looking at.
	sinIntervalo := config.Defaults()
	sinIntervalo.RefreshInterval = 0
	m = New(sinIntervalo, nil)
	if m.tickPending {
		t.Error("sin intervalo de refresco quedó un tick pendiente: un bucle que consulta sin parar")
	}
}

// The boundary is the cursor one past the last row.
func TestSelectedNoSaleDelRango(t *testing.T) {
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	m.applySnapshot(snapshotCon(mkItem("github", "github.com", "acme/widget", "uno", 1, "")))
	m.rebuild()

	if got, ok := m.selected(); !ok || got.Number != 1 {
		t.Fatalf("con el cursor en 0, selected = %d, %v; quiero 1, true", got.Number, ok)
	}

	// A cursor one past the last row returns no item; with a `>` instead the last row would.
	for _, cursor := range []int{1, 2, 100} {
		m.cursor = cursor
		if got, ok := m.selected(); ok {
			t.Errorf("con el cursor en %d devolvió el ítem %d: está fuera de la lista de %d",
				cursor, got.Number, len(m.rows()))
		}
		got, _ := m.selected()
		if got != (model.Item{}) {
			t.Errorf("con el cursor fuera de rango devolvió %+v, want el ítem cero: "+
				"un item a medias se parece a uno bueno", got)
		}
	}

	m.applySnapshot(snapshotCon())
	m.rebuild()
	m.cursor = 0
	if _, ok := m.selected(); ok {
		t.Error("con la lista vacía devolvió un ítem")
	}
}

func TestSaveSnapshotSinRutaNoEscribeNada(t *testing.T) {
	dir := t.TempDir()
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	m.applySnapshot(snapshotCon(mkItem("github", "github.com", "acme/widget", "uno", 1, "")))
	m.rebuild()

	// Sin ruta: no escribe. Y no falla.
	m.cachePath = ""
	antes := entradasEn(t, dir)
	m.saveSnapshot()
	if despues := entradasEn(t, dir); len(despues) != len(antes) {
		t.Errorf("sin ruta de cache aparecieron ficheros: %v", despues)
	}

	// With a path it writes, and it writes THERE.
	m.cachePath = filepath.Join(dir, "snapshot.json")
	m.saveSnapshot()
	if !esperaFichero(t, m.cachePath) {
		t.Fatalf("con ruta de cache no escribió el snapshot en %s", m.cachePath)
	}
	// The file it wrote is valid JSON, which is the only thing that makes it good.
	raw, err := os.ReadFile(m.cachePath)
	if err != nil {
		t.Fatalf("no se pudo leer el snapshot escrito: %v", err)
	}
	if len(raw) == 0 {
		t.Error("el snapshot escrito está vacío")
	}
	var f cache.File
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Errorf("el snapshot escrito no es JSON valido: %v", err)
	}
	if len(f.Streams) == 0 {
		t.Error("el snapshot escrito no tiene ningun stream: no guarda lo que hay en pantalla")
	}
}

func snapshotCon(items ...model.Item) cache.File {
	return cache.File{Streams: []cache.Stream{{
		Forge:   "github",
		Host:    "github.com",
		Section: model.SectionReview,
		Kind:    model.ReviewRequested,
		Items:   items,
	}}}
}

// A stream can arrive from a forge that is no longer in the configuration.
func TestUnStreamDeUnForgeNoConfiguradoNoRevienta(t *testing.T) {
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})

	m = send(t, m, pageMsg{
		cycle:     1,
		key:       streamKey{forge: "gitlab", section: model.SectionReview},
		items:     []model.Item{mkItem("gitlab", "gitlab.com", "acme/widget", "uno", 1, "")},
		unchanged: true,
	})

	if _, ok := m.statuses["gitlab"]; ok {
		t.Error("apareció un estado para un forge que no está en la configuración")
	}
	if m.streams[streamKey{forge: "gitlab", section: model.SectionReview}] == nil {
		t.Error("el stream de un forge no configurado no se guardó")
	}
}

func TestReviewKindSoloSeEstampaEnLaSeccionDeReview(t *testing.T) {
	for _, seccion := range []model.Section{model.SectionReview, model.SectionAuthored, model.SectionMentions} {
		m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
		item := mkItem("github", "github.com", "acme/widget", "uno", 1, "")

		m = send(t, m, pageMsg{
			cycle: 1,
			key:   streamKey{forge: "github", section: seccion, kind: model.ReviewRequested},
			items: []model.Item{item},
			first: true,
		})

		got := m.streams[streamKey{forge: "github", section: seccion, kind: model.ReviewRequested}]
		if got == nil {
			t.Fatalf("sección %v: el stream no se guardó", seccion)
		}
		tiene := got.items[0].ReviewKind != ""
		quiere := seccion == model.SectionReview
		if tiene != quiere {
			t.Errorf("sección %v: ReviewKind %q presente=%v, quiere %v. "+
				"Un ReviewKind fuera de la sección de review dice que el ítem es un review en una lista donde no lo es",
				seccion, got.items[0].ReviewKind, tiene, quiere)
		}
	}
}

func esperaFichero(t *testing.T, path string) bool {
	t.Helper()
	for range 200 {
		if _, err := os.Stat(path); err == nil {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

func entradasEn(t *testing.T, dir string) []string {
	t.Helper()
	got, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var nombres []string
	for _, e := range got {
		nombres = append(nombres, e.Name())
	}
	return nombres
}

func TestElEstadoDelForgeSoloSeSellaConHost(t *testing.T) {
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})

	m.applySnapshot(cache.File{Streams: []cache.Stream{{
		Forge: "github", Host: "github.com", Section: model.SectionReview,
		Items: []model.Item{mkItem("github", "github.com", "acme/widget", "uno", 1, "")},
	}}})
	if got := m.statuses["github"].host; got != "github.com" {
		t.Errorf("con host dio %q, want github.com", got)
	}

	// An empty host does NOT erase a known one.
	m.applySnapshot(cache.File{Streams: []cache.Stream{{Forge: "github", Host: ""}}})
	if got := m.statuses["github"].host; got != "github.com" {
		t.Errorf("un snapshot sin host borró el host conocido: quedó en %q. "+
			"Un host vacío es no saberlo, no saber que no lo hay", got)
	}

	m.applySnapshot(cache.File{Streams: []cache.Stream{{Forge: "gitlab", Host: "gitlab.com"}}})
}
