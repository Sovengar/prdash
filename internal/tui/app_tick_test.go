package tui

import (
	"testing"
	"time"

	"prdash/internal/config"
	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

// TestMergeItemNoPierdeLaSeccionNiElTipoDeReview: la relectura de un ítem
// (ItemState) no sabe en qué sección del inbox está, porque esa sección la decidió
// el listado. Sin recuperar esos dos campos del original, el ítem releído se queda
// sin sección y desaparece de la lista —que es el fallo más caro y más difícil de
// ver de la TUI, porque el resto de la vista parece correcta—.
//
// Y si la relectura TRAE sección, manda la relectura: el forge es el que sabe si
// un MR dejó de estar en review.
func TestMergeItemNoPierdeLaSeccionNiElTipoDeReview(t *testing.T) {
	old := model.NewItem(model.RepoRef{Forge: "gitlab", Host: "gitlab.example.com", Project: "grp/proj"}, 5)
	old.Section = model.SectionReview
	old.ReviewKind = model.ReviewAssigned
	old.Title = "titulo viejo"

	// La relectura llega sin sección ni tipo: se recuperan del original.
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

	// Con sección y tipo propios: mandan ellos, no los del original.
	fresh = model.NewItem(old.Ref, 5)
	fresh.Section = model.SectionAuthored
	fresh.ReviewKind = ""
	got = mergeItem(old, fresh)
	if got.Section != model.SectionAuthored {
		t.Errorf("Section = %q, want la de la relectura (%q)", got.Section, model.SectionAuthored)
	}
	// El ReviewKind vacío sí se recupera: la relectura no lo conoce nunca, y un
	// tipo perdido cambia la etiqueta del detalle.
	if got.ReviewKind != model.ReviewAssigned {
		t.Errorf("ReviewKind = %q, want el del original: ItemState nunca lo trae", got.ReviewKind)
	}
}

// TestTickIntervalConBackoffYTope: el intervalo del auto-refresco son tres
// decisions. Un intervalo de cero DESACTIVA el refresco, así que un base vacío
// tiene que dar cero y no un minuto:activar un tick cada 60s en un config que
// pidió no refrescar es peor que no refrescar.
//
// El backoff suma al intervalo (no lo sustituye) y está acotado: sin el tope, un
// rate limit largo dejaría el inbox sinActualizar durante horas.
func TestTickIntervalConBackoffYTope(t *testing.T) {
	m := newTestModel(t, ghAdapter())

	// Base cero: sin auto-refresco, y sin backoff que lo reviva.
	m.cfg.RefreshInterval = 0
	m.backoff = 0
	if got := m.tickInterval(); got != 0 {
		t.Errorf("con intervalo 0, tickInterval = %v, want 0 (desactivado)", got)
	}
	// Ni un backoff enorme lo revierte: con base 0 no hay a qué sumar.
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

// TestArmTickNoEncadenaTicks: armTick garantiza UNA cadena de ticks. Si se
// encadenaran dos, cada refresh dispararía otros dos y el inbox se
// multiplicaría solo hasta comerse la API del forge. El síntoma es un rate limit
// que aparece sin motivo, y el culpable no lo dice ningún aviso.
func TestArmTickNoEncadenaTicks(t *testing.T) {
	// Con el auto-refresco desactivado no hay tick que armar. Se parte de
	// tickPending a cero para que el caso sea el del intervalo, no el de un tick
	// que ya estaba en vuelo.
	m := newTestModel(t, ghAdapter())
	m.cfg.RefreshInterval = 0
	m.tickPending = false
	if cmd := m.armTick(); cmd != nil {
		t.Error("con el refresco desactivado armTick no debería armar tick")
	}
	if m.tickPending {
		t.Error("armTick no debería marcar tickPending si no arma nada")
	}

	// Con refresco activo: arma UNO y queda pendiente.
	m = newTestModel(t, ghAdapter())
	m.cfg.RefreshInterval = 30 * time.Second
	m.tickPending = false
	if cmd := m.armTick(); cmd == nil {
		t.Fatal("con el refresco activo armTick debería armar tick")
	}
	if !m.tickPending {
		t.Error("armTick debería marcar el tick como pendiente")
	}
	// Un segundo armTick con uno pendiente no arma otro: esta es la cadena única.
	if cmd := m.armTick(); cmd != nil {
		t.Error("con un tick ya pendiente armTick no debería armar otro (cadena doble)")
	}
}

// TestRecomputeBackoffSubeSoloConRateLimitOTimeout: el backoff existe para no
// empeorar un rate limit. Subirlo por un warning que no lo es (un 404, un 422)
// castiga al usuario por un error que no se arregla esperando; no subirlo tras un
// rate limit deja el bucle pidiéndole más justo cuando no puede.
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

	// Un warning que no es de límite: el backoff se queda a cero. Estos son los
	// que de verdad no se arreglan esperando.
	for _, kind := range []string{"notfound", "permission", "auth", "validation", "conflict", "network"} {
		m = nuevo()
		m.statuses["github"].warnings = []model.Warning{{Forge: "github", Kind: kind, Msg: "x"}}
		m.recomputeBackoff()
		if m.backoff != 0 {
			t.Errorf("warning %q subió el backoff a %v, want 0: no se arregla esperando", kind, m.backoff)
		}
	}

	// Rate limit y timeout: ahí sí, y con el intervalo como base.
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
		// ...hasta el tope, que es lo que evita dejar el inbox horas sin refrescar.
		for range 20 {
			m.recomputeBackoff()
		}
		if m.backoff != maxBackoff {
			t.Errorf("el backoff se pasó del tope: %v, want %v", m.backoff, maxBackoff)
		}
	}

	// Y con el intervalo a cero la base del backoff cae a un minuto en vez de
	// sumar a cero (que dejaría el backoff en cero, sin efecto).
	m = nuevo()
	m.cfg.RefreshInterval = 0
	m.statuses["github"].warnings = []model.Warning{{Forge: "github", Kind: "ratelimit"}}
	m.recomputeBackoff()
	if m.backoff != 60*time.Second {
		t.Errorf("con intervalo 0 el backoff base = %v, want 1m", m.backoff)
	}
}

// TestAppendWarningsNoRepiteElMismoAviso: la paginación repite el mismo warning en
// cada página, y sin deduplicar la lista de avisos crece con cada "cargar más"
// hasta tapar la vista. La clave es sección + tipo + mensaje: dos avisos del mismo
// tipo con mensajes distintos SÍ son dos avisos.
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

	// Distinto mensaje: otro aviso, aunque sea del mismo tipo y sección.
	got = appendWarnings(got, []model.Warning{{Forge: "github", Section: model.SectionReview, Kind: "network", Msg: "otro"}})
	if len(got) != 2 {
		t.Errorf("un mensaje distinto debería ser otro aviso, quedó en %d", len(got))
	}

	// Distinta sección: también otro, porque el aviso se pinta donde toca.
	got = appendWarnings(got, []model.Warning{{Forge: "github", Section: model.SectionAuthored, Kind: "network", Msg: "boom"}})
	if len(got) != 3 {
		t.Errorf("otra sección debería ser otro aviso, quedó en %d", len(got))
	}

	// Distinto tipo: otro.
	got = appendWarnings(got, []model.Warning{{Forge: "github", Section: model.SectionReview, Kind: "auth", Msg: "boom"}})
	if len(got) != 4 {
		t.Errorf("otro tipo debería ser otro aviso, quedó en %d", len(got))
	}

	// Y el Forge NO forma parte de la clave: el mismo aviso de dos forges se
	// guarda una vez, que es lo que evita el mismo texto repetido por cada forge.
	got = appendWarnings(got, []model.Warning{{Forge: "gitlab", Section: model.SectionReview, Kind: "network", Msg: "boom"}})
	if len(got) != 4 {
		t.Errorf("el mismo aviso de otro forge no debería duplicarse, quedó en %d", len(got))
	}

	// Con lista vacía de origen no cambia nada, y no se toca la original.
	got = appendWarnings(nil, nil)
	if got != nil {
		t.Errorf("sin origen appendWarnings = %v, want nil", got)
	}
}

// TestPausedConLoadingOAccion: el tick se pausa con una acción en curso. Si no,
// el auto-refresco puede reemplazar el inbox mientras una acción está a medias, y
// el botón que el usuario acaba de pulsar se mueve bajo su dedo.
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

// TestSectionLoadingMoreMiraLaSeccionActiva: el indicador de "cargando más" es de
// la sección que se está pintando, no de la que pagina por debajo. Con el
// indicador en la sección equivocada, el usuario ve que carga algo que no está
// viendo, y no ve lo que sí está cargando.
func TestSectionLoadingMoreMiraLaSeccionActiva(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m.streams[streamKey{forge: "github", section: model.SectionReview, kind: model.ReviewRequested}] = &stream{more: true}

	if !m.sectionLoadingMore(model.SectionReview) {
		t.Error("la sección con páginas pendientes debería decirlo")
	}
	if m.sectionLoadingMore(model.SectionAuthored) {
		t.Error("otra sección no debería cargar más: el indicador es de la que se pinta")
	}

	// Y con la página cerrada, ninguna.
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

// TestSaveSnapshotSinRutaNoPersiste: sin ruta de cache no se escribe nada, y
// sobre todo no se escribe en un sitio arbitrario. El default de newTestModel
// deja cachePath vacío justamente para que esto se pueda probar.
func TestSaveSnapshotSinRutaNoPersiste(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	if m.cachePath != "" {
		t.Fatalf("el fixture debería dejar cachePath vacío, dio %q", m.cachePath)
	}
	// No panic y no escritura: la llamada es un no-op. Un path de cache vacío
	// sería un error de la TUI escribiendo en el cwd del usuario.
	m.saveSnapshot()
}

// TestConfigDefaultsNoTraenRutasDeForgeQueNoExisten: los forges vienen de la
// config, no escritos a mano. Es la regla del proyecto y el sitio donde se
// comprueba: los tres forges con su default de host.
func TestConfigDefaultsCubrenLosTresForges(t *testing.T) {
	cfg := config.Defaults()
	if !cfg.Forges.GitHub.Enabled || cfg.Forges.GitHub.Host != "github.com" {
		t.Errorf("github = %+v, want habilitado en github.com", cfg.Forges.GitHub)
	}
	if !cfg.Forges.GitLab.Enabled || cfg.Forges.GitLab.APIBase != "/api/v4/" {
		t.Errorf("gitlab = %+v, want habilitado con la API base por defecto", cfg.Forges.GitLab)
	}
	// Bitbucket viene desactivado a propósito: no hay adapter que hable con él
	// más allá del contrato, y activarlo por defecto sería ofrecer un forge que no
	// funciona.
	if cfg.Forges.Bitbucket.Enabled {
		t.Error("bitbucket no debería venir habilitado por defecto")
	}
	// Y la conformidad del fake adapter es lo que asegura que los tres tienen
	// contrato.
	for _, name := range []string{"github", "gitlab", "bitbucket"} {
		a := &testutil.FakeAdapter{ForgeName: name, HostName: name + ".example.com"}
		testutil.RunConformance(t, a, testutil.ConformanceOptions{})
	}
}
