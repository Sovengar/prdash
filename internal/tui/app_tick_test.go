package tui

import (
	"testing"
	"time"

	"prdash/internal/config"
	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

// ItemState's re-read does not know the inbox section.
func TestMergeItemNoPierdeLaSeccionNiElTipoDeReview(t *testing.T) {
	old := model.NewItem(model.RepoRef{Forge: "gitlab", Host: "gitlab.example.com", Project: "grp/proj"}, 5)
	old.Section = model.SectionReview
	old.ReviewKind = model.ReviewAssigned
	old.Title = "titulo viejo"

	// The re-read arrives with no section and no kind: both are recovered from the original.
	fresh := model.NewItem(old.Ref, 5)
	fresh.Title = "titulo nuevo"
	got := mergeItem(old, fresh)
	if got.Section != model.SectionReview {
		t.Errorf("Section = %q, want %q: sin esto el ítem desaparece de la lista", got.Section, model.SectionReview)
	}
	if got.ReviewKind != model.ReviewAssigned {
		t.Errorf("ReviewKind = %q, want %q", got.ReviewKind, model.ReviewAssigned)
	}
	// Y el resto de la relectura manda: es estado más fresco.
	if got.Title != "titulo nuevo" {
		t.Errorf("Title = %q, want el de la relectura", got.Title)
	}

	fresh = model.NewItem(old.Ref, 5)
	fresh.Section = model.SectionAuthored
	fresh.ReviewKind = ""
	got = mergeItem(old, fresh)
	if got.Section != model.SectionAuthored {
		t.Errorf("Section = %q, want la de la relectura (%q)", got.Section, model.SectionAuthored)
	}
	// An EMPTY ReviewKind IS recovered: the re-read never knows it, and a lost kind would drop the
	// item out of its column.
	if got.ReviewKind != model.ReviewAssigned {
		t.Errorf("ReviewKind = %q, want el del original: ItemState nunca lo trae", got.ReviewKind)
	}
}

// The auto-refresh interval is three decisions.
func TestTickIntervalConBackoffYTope(t *testing.T) {
	m := newTestModel(t, ghAdapter())

	m.cfg.RefreshInterval = 0
	m.backoff = 0
	if got := m.tickInterval(); got != 0 {
		t.Errorf("con intervalo 0, tickInterval = %v, want 0 (desactivado)", got)
	}
	m.backoff = 10 * time.Minute
	if got := m.tickInterval(); got != 0 {
		t.Errorf("con intervalo 0 y backoff %v, tickInterval = %v, want 0", 10*time.Minute, got)
	}

	// Base positiva: el backoff se suma, no sustituye.
	m.cfg.RefreshInterval = 30 * time.Second
	m.backoff = 0
	if got := m.tickInterval(); got != 30*time.Second {
		t.Errorf("sin backoff = %v, want 30s", got)
	}
	m.backoff = 5 * time.Second
	if got := m.tickInterval(); got != 35*time.Second {
		t.Errorf("con backoff 5s = %v, want 35s (suma, no sustituye)", got)
	}
}

// armTick guarantees ONE tick chain: two chains would re-refresh on every keystroke.
func TestArmTickNoEncadenaTicks(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m.cfg.RefreshInterval = 0
	m.tickPending = false
	if cmd := m.armTick(); cmd != nil {
		t.Error("con el refresco desactivado armTick no debería armar tick")
	}
	if m.tickPending {
		t.Error("armTick no debería marcar tickPending si no arma nada")
	}

	m = newTestModel(t, ghAdapter())
	m.cfg.RefreshInterval = 30 * time.Second
	m.tickPending = false
	if cmd := m.armTick(); cmd == nil {
		t.Fatal("con el refresco activo armTick debería armar tick")
	}
	if !m.tickPending {
		t.Error("armTick debería marcar el tick como pendiente")
	}
	// A second armTick with one pending arms no other: this is the single chain.
	if cmd := m.armTick(); cmd != nil {
		t.Error("con un tick ya pendiente armTick no debería armar otro (cadena doble)")
	}
}

// The backoff exists so as not to make a rate limit worse.
func TestRecomputeBackoffSubeSoloConRateLimitOTimeout(t *testing.T) {
	nuevo := func() Model {
		m := newTestModel(t, ghAdapter())
		m.cfg.RefreshInterval = 30 * time.Second
		return m
	}

	// Sin warnings: a cero.
	m := nuevo()
	m.recomputeBackoff()
	if m.backoff != 0 {
		t.Errorf("sin warnings, backoff = %v, want 0", m.backoff)
	}

	for _, kind := range []string{"notfound", "permission", "auth", "validation", "conflict", "network"} {
		m = nuevo()
		m.statuses["github"].warnings = []model.Warning{{Forge: "github", Kind: kind, Msg: "x"}}
		m.recomputeBackoff()
		if m.backoff != 0 {
			t.Errorf("warning %q subió el backoff a %v, want 0: no se arregla esperando", kind, m.backoff)
		}
	}

	for _, kind := range []string{"ratelimit", "timeout"} {
		m = nuevo()
		m.statuses["github"].warnings = []model.Warning{{Forge: "github", Kind: kind, Msg: "x"}}
		m.recomputeBackoff()
		if m.backoff != 30*time.Second {
			t.Errorf("warning %q dio backoff %v, want el intervalo (30s) como base", kind, m.backoff)
		}
		// Y al repetirlo se duplica...
		m.recomputeBackoff()
		if m.backoff != time.Minute {
			t.Errorf("el segundo %q dio %v, want el doble (60s)", kind, m.backoff)
		}
		// ...up to the ceiling, which is what stops the inbox going hours without a refresh.
		for range 20 {
			m.recomputeBackoff()
		}
		if m.backoff != maxBackoff {
			t.Errorf("el backoff se pasó del tope: %v, want %v", m.backoff, maxBackoff)
		}
	}

	// With the interval at zero the backoff base drops to a minute instead of adding to zero.
	m = nuevo()
	m.cfg.RefreshInterval = 0
	m.statuses["github"].warnings = []model.Warning{{Forge: "github", Kind: "ratelimit"}}
	m.recomputeBackoff()
	if m.backoff != 60*time.Second {
		t.Errorf("con intervalo 0 el backoff base = %v, want 1m", m.backoff)
	}
}

// Pagination repeats the same warning on every page.
func TestAppendWarningsNoRepiteElMismoAviso(t *testing.T) {
	base := []model.Warning{{Forge: "github", Section: model.SectionReview, Kind: "network", Msg: "boom"}}

	// El mismo aviso tres veces: uno solo.
	got := base
	for range 3 {
		got = appendWarnings(got, []model.Warning{{Forge: "github", Section: model.SectionReview, Kind: "network", Msg: "boom"}})
	}
	if len(got) != 1 {
		t.Errorf("el mismo aviso repetido quedó en %d, want 1", len(got))
	}

	got = appendWarnings(got, []model.Warning{{Forge: "github", Section: model.SectionReview, Kind: "network", Msg: "otro"}})
	if len(got) != 2 {
		t.Errorf("un mensaje distinto debería ser otro aviso, quedó en %d", len(got))
	}

	got = appendWarnings(got, []model.Warning{{Forge: "github", Section: model.SectionAuthored, Kind: "network", Msg: "boom"}})
	if len(got) != 3 {
		t.Errorf("otra sección debería ser otro aviso, quedó en %d", len(got))
	}

	// Distinto tipo: otro.
	got = appendWarnings(got, []model.Warning{{Forge: "github", Section: model.SectionReview, Kind: "auth", Msg: "boom"}})
	if len(got) != 4 {
		t.Errorf("otro tipo debería ser otro aviso, quedó en %d", len(got))
	}

	// The Forge is NOT part of the key: the same warning from two forges is kept once, on
	// purpose.
	got = appendWarnings(got, []model.Warning{{Forge: "gitlab", Section: model.SectionReview, Kind: "network", Msg: "boom"}})
	if len(got) != 4 {
		t.Errorf("el mismo aviso de otro forge no debería duplicarse, quedó en %d", len(got))
	}

	got = appendWarnings(nil, nil)
	if got != nil {
		t.Errorf("sin origen appendWarnings = %v, want nil", got)
	}
}

func TestPausedConLoadingOAccion(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	if m.paused() {
		t.Error("sin nada en curso no debería estar en pausa")
	}
	m.loading = true
	if !m.paused() {
		t.Error("con una carga en curso debería pausar")
	}
	m.loading = false
	m.actionBusy = true
	if !m.paused() {
		t.Error("con una acción en curso debería pausar")
	}
}

// The "loading more" indicator belongs to the section being paged.
func TestSectionLoadingMoreMiraLaSeccionActiva(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m.streams[streamKey{forge: "github", section: model.SectionReview, kind: model.ReviewRequested}] = &stream{more: true}

	if !m.sectionLoadingMore(model.SectionReview) {
		t.Error("la sección con páginas pendientes debería decirlo")
	}
	if m.sectionLoadingMore(model.SectionAuthored) {
		t.Error("otra sección no debería cargar más: el indicador es de la que se pinta")
	}

	for k, s := range m.streams {
		s.more = false
		m.streams[k] = s
	}
	for _, kind := range []model.Section{model.SectionReview, model.SectionAuthored, model.SectionMentions} {
		if m.sectionLoadingMore(kind) {
			t.Errorf("con la página cerrada, %v no debería cargar más", kind)
		}
	}
}

func TestSaveSnapshotSinRutaNoPersiste(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	if m.cachePath != "" {
		t.Fatalf("el fixture debería dejar cachePath vacío, dio %q", m.cachePath)
	}
	// No panic and no write: the call is a no-op. An empty cache path would be an error.
	m.saveSnapshot()
}

func TestConfigDefaultsCubrenLosTresForges(t *testing.T) {
	cfg := config.Defaults()
	if !cfg.Forges.GitHub.Enabled || cfg.Forges.GitHub.Host != "github.com" {
		t.Errorf("github = %+v, want habilitado en github.com", cfg.Forges.GitHub)
	}
	if !cfg.Forges.GitLab.Enabled || cfg.Forges.GitLab.APIBase != "/api/v4/" {
		t.Errorf("gitlab = %+v, want habilitado con la API base por defecto", cfg.Forges.GitLab)
	}
	// Bitbucket disabled on purpose: no adapter speaks to it beyond the probe.
	if cfg.Forges.Bitbucket.Enabled {
		t.Error("bitbucket no debería venir habilitado por defecto")
	}
	for _, name := range []string{"github", "gitlab", "bitbucket"} {
		a := &testutil.FakeAdapter{ForgeName: name, HostName: name + ".example.com"}
		testutil.RunConformance(t, a, testutil.ConformanceOptions{})
	}
}
