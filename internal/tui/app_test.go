// Tests del modelo TUI con adapters falsos: se construye el Model, se le
// envían mensajes y se inspecciona el estado y la vista, sin teatest y sin
// tocar ninguna CLI.
package tui

import (
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"prdash/internal/cache"
	"prdash/internal/config"
	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

func newTestModel(t *testing.T, adapters ...forge.Adapter) Model {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", t.TempDir()) // aísla el cache real
	m := New(config.Defaults(), adapters)
	m.width, m.height = 160, 40
	m.loading = false
	m.cachePath = "" // aísla el cache real en tests
	return m
}

func mkItem(forgeName, host, project, title string, number int, decision string) model.Item {
	it := model.NewItem(model.RepoRef{Forge: forgeName, Host: host, Project: project, Owner: "acme", Name: "widget"}, number)
	it.Title = title
	it.Author = "me"
	it.SourceBranch = "feat/x"
	it.TargetBranch = "main"
	it.URL = "https://" + host + "/" + project + "/" + strconv.Itoa(number)
	it.ReviewDecision = decision
	it.State = "OPEN"
	it.UpdatedAt = time.Now()
	return it
}

func page(cycle int, forgeName, host string, section model.Section, kind model.ReviewKind, items []model.Item, more bool) pageMsg {
	next := ""
	if more {
		next = "c1"
	}
	return pageMsg{
		cycle: cycle,
		key:   streamKey{forge: forgeName, section: section, kind: kind},
		items: items,
		next:  next,
		more:  more,
		first: true,
	}
}

func send(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	out, _ := m.Update(msg)
	return out.(Model)
}

func press(t *testing.T, m Model, key string) Model {
	t.Helper()
	km := tea.KeyPressMsg{Code: []rune(key)[0], Text: key}
	switch key {
	case "tab":
		km = tea.KeyPressMsg{Code: tea.KeyTab}
	case "up":
		km = tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		km = tea.KeyPressMsg{Code: tea.KeyDown}
	case "home":
		km = tea.KeyPressMsg{Code: tea.KeyHome}
	case "end":
		km = tea.KeyPressMsg{Code: tea.KeyEnd}
	case "pgup":
		km = tea.KeyPressMsg{Code: tea.KeyPgUp}
	case "pgdown":
		km = tea.KeyPressMsg{Code: tea.KeyPgDown}
	}
	out, _ := m.Update(km)
	return out.(Model)
}

// toastTexts devuelve los mensajes de los avisos vivos.
func toastTexts(m Model) []string { return m.toast.texts() }

// lastToast es el mensaje del último aviso lanzado.
func lastToast(m Model) string { return m.toast.last() }

// assertToast falla si ningún aviso vivo contiene want.
func assertToast(t *testing.T, m Model, want string) {
	t.Helper()
	if !strings.Contains(lastToast(m), want) {
		t.Fatalf("toast = %q, want %q (vivos: %q)", lastToast(m), want, toastTexts(m))
	}
}

func ghAdapter() *testutil.FakeAdapter {
	return &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"}
}

// TestViewShowsThreeSectionsWithBothForges cubre el escenario "el inbox
// muestra las tres secciones con datos de ambos forges".
func TestViewShowsThreeSectionsWithBothForges(t *testing.T) {
	m := newTestModel(t, ghAdapter(), &testutil.FakeAdapter{ForgeName: "gitlab", HostName: "gitlab.example.com"})

	reviewReq := mkItem("github", "github.com", "acme/lib", "Review me", 2, "REVIEW_REQUIRED")
	reviewReq.ReviewKind = model.ReviewRequested
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{mkItem("github", "github.com", "acme/widget", "Add widget", 1, "APPROVED")}, false))
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{reviewReq}, false))
	m = send(t, m, page(1, "gitlab", "gitlab.example.com", model.SectionAuthored, "", []model.Item{mkItem("gitlab", "gitlab.example.com", "grp/proj", "MR propio", 3, "APPROVED")}, false))
	m = send(t, m, page(1, "gitlab", "gitlab.example.com", model.SectionMentions, "", []model.Item{mkItem("gitlab", "gitlab.example.com", "grp/proj", "Mención", 5, "")}, false))

	view := stripANSI(m.View().Content)
	for _, want := range []string{
		"Created by me", "Review / assigned", "Mentions",
		"GH", "GLab@gitlab", // columna FORGE: abreviatura (+ host si self-hosted)
		"Add widget", "Review me", "MR propio", "Mención",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("la vista no contiene %q\n%s", want, view)
		}
	}
}

// TestSectionEmptyVsError cubre "distinguir sección vacía de no se pudo
// consultar".
func TestSectionEmptyVsError(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, pageMsg{cycle: 1, key: streamKey{forge: "github", section: model.SectionReview}, warnings: []model.Warning{{Forge: "github", Section: model.SectionReview, Kind: "network", Msg: "boom"}}})

	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "could not be queried") {
		t.Errorf("la sección fallida debería decirlo\n%s", view)
	}
	if got := strings.Count(view, "(empty)"); got != 2 {
		t.Errorf("(empty) appears %d times, want 2 (review no debe decir vacío)\n%s", got, view)
	}
}

// TestDegradationKeepsOtherForges cubre "una forge caída o sin auth no vacía el
// inbox".
func TestDegradationKeepsOtherForges(t *testing.T) {
	m := newTestModel(t, ghAdapter(), &testutil.FakeAdapter{ForgeName: "gitlab", HostName: "gitlab.example.com"})
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{mkItem("github", "github.com", "acme/widget", "Sigue visible", 1, "")}, false))
	m = send(t, m, authMsg{cycle: 1, forge: "gitlab", auth: model.AuthState{Forge: "gitlab", OK: false, Reason: "401"}})
	m = send(t, m, pageMsg{cycle: 1, key: streamKey{forge: "gitlab", section: model.SectionAuthored}, warnings: []model.Warning{{Forge: "gitlab", Kind: "auth", Msg: "401"}}})

	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "Sigue visible") {
		t.Errorf("los ítems de GitHub deberían seguir visibles\n%s", view)
	}
	if !strings.Contains(view, "gitlab ✗") {
		t.Errorf("gitlab debería reportarse caído\n%s", view)
	}
}

// TestPaginationIndicator cubre el indicador "loading more…" por sección.
func TestPaginationIndicator(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{mkItem("github", "github.com", "acme/widget", "Uno", 1, "")}, true))

	if !m.sectionLoadingMore(model.SectionAuthored) {
		t.Fatal("authored debería seguir paginando")
	}
	if view := stripANSI(m.View().Content); !strings.Contains(view, "loading more…") {
		t.Errorf("falta el indicador de carga\n%s", view)
	}
}

// TestIncrementalPages: la primera página reemplaza y las siguientes acumulan.
func TestIncrementalPages(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{mkItem("github", "github.com", "acme/widget", "Uno", 1, "")}, true))

	next := page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{mkItem("github", "github.com", "acme/widget", "Dos", 2, "")}, true)
	next.first = false
	m = send(t, m, next)

	if got := len(m.sectionItems(model.SectionAuthored)); got != 2 {
		t.Fatalf("authored = %d, want 2", got)
	}

	// Una nueva primera página reemplaza la lista (refresco incremental).
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{mkItem("github", "github.com", "acme/widget", "Nuevo", 3, "")}, false))
	items := m.sectionItems(model.SectionAuthored)
	if len(items) != 1 || items[0].Title != "Nuevo" {
		t.Fatalf("authored tras refrescar = %+v", items)
	}
}

// TestManualRefreshIncrementsCycle: la tecla de refresco arranca un ciclo nuevo.
func TestManualRefreshIncrementsCycle(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	before := m.cycle
	m = press(t, m, "r")
	if m.cycle != before+1 {
		t.Fatalf("cycle = %d, want %d", m.cycle, before+1)
	}
	if !m.loading {
		t.Fatal("debería quedar en carga")
	}
}

// TestAutoRefreshTick starts a refresh on tick when idle.
func TestAutoRefreshTick(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	before := m.cycle
	m = send(t, m, tickMsg{})
	if m.cycle != before+1 || !m.loading {
		t.Fatalf("tick no arrancó refresco: cycle=%d loading=%v", m.cycle, m.loading)
	}
}

// TestAutoRefreshPausedDuringAction cubre "un refresco no pisa una acción en
// curso": con una acción en curso, el tick no arranca refresco.
func TestAutoRefreshPausedDuringAction(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m.actionBusy = true
	before := m.cycle
	m = send(t, m, tickMsg{})
	if m.cycle != before {
		t.Fatalf("el tick no debería arrancar refresco con acción en curso (cycle=%d)", m.cycle)
	}
	if m.loading {
		t.Fatal("no debería quedar en carga")
	}
	if !m.actionBusy {
		t.Fatal("la acción en curso no debería cancelarse")
	}
}

// TestRefreshUpdatesOtherItemsDuringAction: aunque haya acción en curso, un
// refresco sí actualiza el resto de ítems.
func TestRefreshUpdatesOtherItemsDuringAction(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m.actionBusy = true
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{mkItem("github", "github.com", "acme/widget", "Otro", 9, "")}, false))
	if got := len(m.sectionItems(model.SectionAuthored)); got != 1 {
		t.Fatalf("el refresco debería actualizar el resto de ítems: %d", got)
	}
}

// TestDetailOpenAndBack cubre el escenario de detalle: se ve la info y se
// vuelve al inbox sin perder la selección.
func TestDetailOpenAndBack(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{
		mkItem("github", "github.com", "acme/widget", "Add widget", 1, "APPROVED"),
		mkItem("github", "github.com", "acme/widget", "Otro", 2, ""),
	}, false))
	m = press(t, m, "down") // selecciona el segundo
	cursor := m.cursor

	m = press(t, m, "enter")
	if !m.detailOpen {
		t.Fatal("el detalle debería estar abierto")
	}
	view := stripANSI(m.View().Content)
	for _, want := range []string{"Otro", "Author", "Source", "feat/x", "Target", "main", "#2", "https://"} {
		if !strings.Contains(view, want) {
			t.Errorf("el detalle no contiene %q\n%s", want, view)
		}
	}
	if m.cursor != cursor {
		t.Fatalf("abrir el detalle no debería mover el cursor")
	}

	m = press(t, m, "esc")
	if m.detailOpen {
		t.Fatal("el detalle debería cerrarse")
	}
	if m.cursor != cursor {
		t.Fatalf("volver no debería perder la selección (cursor=%d, want %d)", m.cursor, cursor)
	}
}

// TestApproveOKUpdatesNotice cubre "approve/merge desde el inbox".
func TestApproveOKUpdatesNotice(t *testing.T) {
	item := mkItem("github", "github.com", "acme/widget", "Add widget", 1, "")
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{item}, false))

	m = send(t, m, actionMsg{outcome: forge.Outcome{Kind: forge.ActionApprove, ID: item.ID(), OK: true}})
	if !strings.Contains(lastToast(m), "approve ok") {
		t.Fatalf("toast = %q", lastToast(m))
	}
	if m.actionBusy {
		t.Fatal("la acción debería haber terminado")
	}
}

// TestApproveOwnPulledBeforeForge: aprobar lo propio no lo admite ningún
// forge, así que la TUI lo corta antes de gastar la llamada. Lo cubre el veto
// con login conocido y, sin login, por sección propia.
func TestApproveOwnPulledBeforeForge(t *testing.T) {
	cases := []struct {
		name    string
		section model.Section
		kind    model.ReviewKind
		author  string
		login   string
	}{
		// Las listas de review siempre llegan con kind (son las dos queries que
		// emite el registry); authored y menciones van sin kind.
		{"login coincide", model.SectionAuthored, "", "Sovengar", "Sovengar"},
		{"otro autor con login", model.SectionReview, model.ReviewRequested, "otra", "Sovengar"},
		{"sin login, seccion propia", model.SectionAuthored, "", "quien sea", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			item := mkItem("github", "github.com", "acme/widget", "Add widget", 11, "")
			item.Author = tc.author
			fake := &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"}
			m := newTestModel(t, fake)
			m = send(t, m, authMsg{cycle: 1, forge: "github", auth: model.AuthState{Forge: "github", OK: true, Login: tc.login}})
			m = send(t, m, page(1, "github", "github.com", tc.section, tc.kind, []model.Item{item}, false))

			blocked := m.selfDenied[item.ID()] != ""
			m = press(t, m, "a")

			if tc.author == "otra" {
				if blocked {
					t.Fatal("un PR de otro autor no debe vetarse")
				}
				// actionBusy se fija de forma síncrona al arrancar la acción.
				if !m.actionBusy {
					t.Fatal("aprobar un PR de otro autor debería lanzar la acción")
				}
				return
			}
			if !blocked {
				t.Fatal("un PR propio debería quedar vetado")
			}
			if n := fake.ActionCallCount("approve", item.Ref, item.Number); n != 0 {
				t.Fatalf("Approve se llamó %d veces: el veto debe cortar antes del subproceso", n)
			}
			if m.actionBusy {
				t.Fatal("no debería quedar una acción en curso")
			}
			if !strings.Contains(lastToast(m), "you cannot approve your own") {
				t.Fatalf("toast = %q", lastToast(m))
			}
		})
	}
}

// TestOwnItemShowsRoleAndDetail: la fila y el detalle anticipan que approve no
// aplica, para no tener que descubrirlo fallando.
func TestOwnItemShowsRoleAndDetail(t *testing.T) {
	own := mkItem("github", "github.com", "acme/widget", "Mío", 12, "")
	own.Author = "Sovengar"
	other := mkItem("github", "github.com", "acme/widget", "De otro", 13, "")
	other.Author = "otra"
	m := newTestModel(t, ghAdapter())
	m = send(t, m, authMsg{cycle: 1, forge: "github", auth: model.AuthState{Forge: "github", OK: true, Login: "Sovengar"}})
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{own}, false))
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{other}, false))

	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "own") {
		t.Errorf("la fila de un PR propio debería marcar el rol:\n%s", view)
	}

	m = send(t, m, tea.KeyPressMsg{Code: []rune(m.cfg.KeyFor("detail"))[0], Text: m.cfg.KeyFor("detail")})
	detail := stripANSI(m.View().Content)
	if !strings.Contains(detail, "approve unavailable") {
		t.Errorf("el detalle debería explicar que approve no aplica:\n%s", detail)
	}
}

// TestSelfDenySurvivesRefresh: el veto se deriva del ítem y del login, no es
// un estado que un refresco pueda borrar. Un refresco que trae el ítem de nuevo
// lo deja igual de vetado.
func TestSelfDenySurvivesRefresh(t *testing.T) {
	item := mkItem("github", "github.com", "acme/widget", "Add widget", 14, "")
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{item}, false))
	if m.selfDenied[item.ID()] == "" {
		t.Fatal("el ítem propio debería quedar vetado")
	}
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{item}, false))
	if m.selfDenied[item.ID()] == "" {
		t.Fatal("un refresco no debe levantar el veto de aprobar lo propio")
	}
}

// TestConflictRefreshesItem cubre "el ítem cambió entre refresco y acción".
func TestConflictRefreshesItem(t *testing.T) {
	item := mkItem("github", "github.com", "acme/widget", "Add widget", 7, "")
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{item}, false))

	refreshed := item
	refreshed.State = "MERGED"
	m = send(t, m, actionMsg{outcome: forge.Outcome{
		Kind: forge.ActionMerge, ID: item.ID(),
		Conflict: true, Msg: "el ítem ya está mergeado",
		Item: refreshed, HasItem: true,
	}})

	if !strings.Contains(lastToast(m), "forge conflict") {
		t.Fatalf("toast = %q", lastToast(m))
	}
	items := m.sectionItems(model.SectionAuthored)
	if len(items) != 1 || items[0].State != "MERGED" {
		t.Fatalf("el ítem debería haberse refrescado: %+v", items)
	}
}

// TestPermissionRecordsDenial cubre "acción deshabilitada con motivo".
func TestPermissionRecordsDenial(t *testing.T) {
	item := mkItem("gitlab", "gitlab.example.com", "grp/proj", "MR", 4, "")
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "gitlab", HostName: "gitlab.example.com"})
	m = send(t, m, page(1, "gitlab", "gitlab.example.com", model.SectionAuthored, "", []model.Item{item}, false))

	m = send(t, m, actionMsg{outcome: forge.Outcome{Kind: forge.ActionApprove, ID: item.ID(), Perm: true, Msg: "no tienes permiso"}})
	if !strings.Contains(lastToast(m), "disabled") {
		t.Fatalf("toast = %q", lastToast(m))
	}
	if m.denied[item.ID()] == "" {
		t.Fatal("la denegación debería quedar registrada")
	}

	// Un nuevo intento no lanza acción y explica el motivo.
	m = press(t, m, "a")
	if !strings.Contains(lastToast(m), "no tienes permiso") {
		t.Fatalf("toast after retry = %q", lastToast(m))
	}
}

// TestActionDisabledWhenForgeDown: sin auth, la acción queda deshabilitada.
func TestActionDisabledWhenForgeDown(t *testing.T) {
	m := newTestModel(t, &testutil.FakeAdapter{
		ForgeName: "gitlab", HostName: "gitlab.example.com",
		AuthState: model.AuthState{Forge: "gitlab", OK: false, Reason: "401"},
	})
	m = send(t, m, authMsg{cycle: 1, forge: "gitlab", auth: model.AuthState{Forge: "gitlab", OK: false, Reason: "401"}})
	m = send(t, m, page(1, "gitlab", "gitlab.example.com", model.SectionAuthored, "", []model.Item{mkItem("gitlab", "gitlab.example.com", "grp/proj", "MR", 4, "")}, false))

	m = press(t, m, "a")
	if m.actionBusy {
		t.Fatal("no debería arrancar la acción con la forge caída")
	}
	if !strings.Contains(lastToast(m), "not authenticated") {
		t.Fatalf("toast = %q", lastToast(m))
	}
}

// TestSnapshotPaintsInstantly cubre el uso del cache al arrancar.
func TestSnapshotPaintsInstantly(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	path, err := cache.Path()
	if err != nil {
		t.Fatal(err)
	}
	it := mkItem("github", "github.com", "acme/widget", "Cacheado", 1, "")
	if err := cache.Save(path, cache.File{Streams: []cache.Stream{{
		Forge: "github", Host: "github.com", Section: model.SectionAuthored, Items: []model.Item{it},
	}}}); err != nil {
		t.Fatal(err)
	}

	m := New(config.Defaults(), []forge.Adapter{ghAdapter()})
	if got := len(m.sectionItems(model.SectionAuthored)); got != 1 {
		t.Fatalf("authored cacheado = %d, want 1", got)
	}
	if view := stripANSI(m.View().Content); !strings.Contains(view, "Cacheado") {
		t.Errorf("la vista debería pintar el cache\n%s", view)
	}
}

// TestPerForgeUpdateIndicator cubre el indicador "última actualización" por
// forge: una forge lenta no debe mentir sobre el resto.
func TestPerForgeUpdateIndicator(t *testing.T) {
	m := newTestModel(t, ghAdapter(), &testutil.FakeAdapter{ForgeName: "gitlab", HostName: "gitlab.example.com"})
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{mkItem("github", "github.com", "acme/widget", "Uno", 1, "")}, false))

	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "github ✓ now") {
		t.Errorf("github debería mostrar su propia hora\n%s", view)
	}
	if !strings.Contains(view, "gitlab ✓ no data") {
		t.Errorf("gitlab no debería heredar la hora de github\n%s", view)
	}
}

// TestUnsupportedForgeShowsReason cubre "Bitbucket está presente pero no
// operativo": la sección dice que no se pudo consultar por no soportado.
func TestUnsupportedForgeShowsReason(t *testing.T) {
	m := newTestModel(t, &testutil.FakeAdapter{
		ForgeName: "bitbucket", HostName: "bitbucket.org",
		AuthState: model.AuthState{Forge: "bitbucket", OK: false, Reason: "no soportado en esta versión"},
	})
	m = send(t, m, authMsg{cycle: 1, forge: "bitbucket", auth: model.AuthState{Forge: "bitbucket", OK: false, Reason: "no soportado en esta versión"}})
	m = send(t, m, pageMsg{cycle: 1, key: streamKey{forge: "bitbucket", section: model.SectionAuthored}, warnings: []model.Warning{{Forge: "bitbucket", Section: model.SectionAuthored, Kind: "unsupported", Msg: "no soportado"}}})

	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "no soportado") {
		t.Errorf("la sección debería decir que no está soportado\n%s", view)
	}
	if strings.Contains(view, "bitbucket ✓") {
		t.Errorf("bitbucket no debería reportarse operativo\n%s", view)
	}
}

// TestUnchangedHead fija la decisión del refresco incremental por cursor.
func TestUnchangedHead(t *testing.T) {
	cases := []struct {
		name string
		prev streamHead
		page forge.Page
		want bool
	}{
		{"cabecera igual", streamHead{cursor: "c1", complete: true}, forge.Page{Next: "c1", More: true}, true},
		{"no completo aún", streamHead{cursor: "c1", complete: false}, forge.Page{Next: "c1", More: true}, false},
		{"cabecera distinta", streamHead{cursor: "c1", complete: true}, forge.Page{Next: "c2", More: true}, false},
		{"una sola página", streamHead{cursor: "", complete: true}, forge.Page{Next: "", More: false}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := unchangedHead(c.prev, c.page); got != c.want {
				t.Fatalf("unchangedHead = %v, want %v", got, c.want)
			}
		})
	}
}

// TestIncrementalUnchangedKeepsItems cubre "refresco incremental": si la
// cabecera no cambió, se conserva lo ya cargado y no se vuelve a paginar.
func TestIncrementalUnchangedKeepsItems(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(m.cycle, "github", "github.com", model.SectionAuthored, "", []model.Item{mkItem("github", "github.com", "acme/widget", "A", 1, "")}, true))
	cont := page(m.cycle, "github", "github.com", model.SectionAuthored, "", []model.Item{mkItem("github", "github.com", "acme/widget", "B", 2, "")}, false)
	cont.first = false
	m = send(t, m, cont)

	if got := len(m.sectionItems(model.SectionAuthored)); got != 2 {
		t.Fatalf("authored = %d, want 2", got)
	}
	if m.sectionLoadingMore(model.SectionAuthored) {
		t.Fatal("no debería quedar paginación pendiente")
	}

	// Nuevo ciclo: la cabecera no cambió.
	m = send(t, m, pageMsg{cycle: m.cycle, key: streamKey{forge: "github", section: model.SectionAuthored}, unchanged: true})
	if got := len(m.sectionItems(model.SectionAuthored)); got != 2 {
		t.Fatalf("el refresco sin cambios debería conservar los ítems: %d", got)
	}
}

// TestBackoffOnRateLimit cubre el backoff del auto-refresco ante límite de
// peticiones.
func TestBackoffOnRateLimit(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	base := m.tickInterval()

	m.statuses["github"].warnings = []model.Warning{{Forge: "github", Kind: "ratelimit", Msg: "429"}}
	m.recomputeBackoff()
	if m.backoff <= 0 {
		t.Fatal("debería aplicar backoff ante rate limit")
	}
	if m.tickInterval() <= base {
		t.Fatalf("tickInterval = %v, debería superar %v", m.tickInterval(), base)
	}

	m.statuses["github"].warnings = nil
	m.recomputeBackoff()
	if m.backoff != 0 {
		t.Fatalf("backoff = %v, debería resetearse", m.backoff)
	}
}

// TestManualOnlyRefreshHasNoTick: con intervalo 0, no hay auto-refresco.
func TestManualOnlyRefreshHasNoTick(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m.cfg.RefreshInterval = 0
	if m.tickCmd() != nil {
		t.Fatal("con intervalo 0 no debería programarse tick")
	}
	if m.tickInterval() != 0 {
		t.Fatalf("tickInterval = %v, want 0", m.tickInterval())
	}
}

// TestCurrentCycleDrainsLoading cubre H1 con ciclo único (M-1): el ciclo
// vigente siempre emite su refreshDone, que baja loading y rearma el tick.
func TestCurrentCycleDrainsLoading(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = press(t, m, "r")
	if !m.loading {
		t.Fatal("el refresco debe quedar en carga")
	}
	m = send(t, m, refreshDoneMsg{cycle: m.cycle})
	if m.loading {
		t.Fatal("el ciclo vigente debe bajar loading")
	}
	if !m.tickPending {
		t.Fatal("debe rearmar el tick")
	}
	before := m.cycle
	m = send(t, m, tickMsg{})
	if m.cycle == before {
		t.Fatalf("el tick debe volver a refrescar (cycle=%d)", m.cycle)
	}
}

// TestRefreshDoesNotOverlap cubre M-1: no se solapan ciclos.
func TestRefreshDoesNotOverlap(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = press(t, m, "r")
	cycle := m.cycle
	m = press(t, m, "r") // con el ciclo en vuelo
	if m.cycle != cycle {
		t.Fatalf("no debe arrancar un ciclo solapado (cycle=%d, want %d)", m.cycle, cycle)
	}
}

// TestObsoleteRefreshDoneIsInert cubre M-1: un refreshDone obsoleto no baja
// loading ni arma un tick (solo rearma la bomba).
func TestObsoleteRefreshDoneIsInert(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m.loading = true
	m.tickPending = false
	m = send(t, m, refreshDoneMsg{cycle: m.cycle - 1})
	if !m.loading {
		t.Fatal("un refreshDone obsoleto no debe bajar loading")
	}
	if m.tickPending {
		t.Fatal("un refreshDone obsoleto no debe armar tick")
	}
}

// TestArmTickSingleChain cubre M-1: una sola cadena de ticks.
func TestArmTickSingleChain(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m.tickPending = false
	if m.armTick() == nil {
		t.Fatal("sin tick pendiente debería armar uno")
	}
	if m.armTick() != nil {
		t.Fatal("con tick pendiente no debería armar otro")
	}
}

// TestSingleChannelReader cubre M-2: ticks y refrescos locales no añaden
// lectores del canal (invariante: exactamente uno).
func TestSingleChannelReader(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	if m.readers != 1 {
		t.Fatalf("lectores iniciales = %d, want 1", m.readers)
	}

	for i := 0; i < 5; i++ {
		m = send(t, m, tickMsg{})
		m = send(t, m, refreshDoneMsg{cycle: m.cycle})
	}
	m = press(t, m, "r")
	m = send(t, m, refreshDoneMsg{cycle: m.cycle})

	// Un evento del canal consume un lector y rearma exactamente uno.
	m = send(t, m, authMsg{cycle: m.cycle, forge: "github", auth: model.AuthState{Forge: "github", OK: true}})

	if m.readers != 1 {
		t.Fatalf("lectores = %d, want 1 (no deben acumularse)", m.readers)
	}
}

// TestDetailReflectsActionUpdate cubre H2: el detalle se deriva del estado vivo.
func TestDetailReflectsActionUpdate(t *testing.T) {
	item := mkItem("github", "github.com", "acme/widget", "Add widget", 1, "")
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{item}, false))
	m = press(t, m, "enter")

	refreshed := item
	refreshed.State = "MERGED"
	m = send(t, m, actionMsg{cycle: m.cycle, outcome: forge.Outcome{
		Kind: forge.ActionMerge, ID: item.ID(), Conflict: true, Msg: "ya mergeado",
		Item: refreshed, HasItem: true,
	}})

	if view := stripANSI(m.View().Content); !strings.Contains(view, "merged") {
		t.Errorf("el detalle debería reflejar el estado nuevo\n%s", view)
	}
}

// TestStaleActionAppliesReread cubre M6: el ítem releído es el estado más
// reciente y se aplica aunque el ciclo haya avanzado (no se revierte).
func TestStaleActionAppliesReread(t *testing.T) {
	item := mkItem("github", "github.com", "acme/widget", "Add widget", 1, "")
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{item}, false))

	newer := item
	newer.State = "MERGED"
	m = send(t, m, actionMsg{cycle: m.cycle - 1, outcome: forge.Outcome{
		Kind: forge.ActionMerge, ID: item.ID(), Item: newer, HasItem: true, OK: true,
	}})

	items := m.sectionItems(model.SectionAuthored)
	if len(items) != 1 || items[0].State != "MERGED" {
		t.Fatalf("el estado releído debería aplicarse: %+v", items)
	}
}

// TestRefreshClearsDenied cubre M4: un refresco exitoso limpia la denegación.
func TestRefreshClearsDenied(t *testing.T) {
	item := mkItem("gitlab", "gitlab.example.com", "grp/proj", "MR", 4, "")
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "gitlab", HostName: "gitlab.example.com"})
	m.denied[item.ID()] = "no tienes permiso"

	m = send(t, m, page(1, "gitlab", "gitlab.example.com", model.SectionAuthored, "", []model.Item{item}, false))
	if _, ok := m.denied[item.ID()]; ok {
		t.Fatal("el refresco exitoso debería limpiar la denegación")
	}
}

// TestDegradedDoesNotComplete cubre M3: un fallback degradado no marca el
// stream como completo (no se congela en el refresco incremental).
func TestDegradedDoesNotComplete(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, pageMsg{
		cycle: m.cycle,
		key:   streamKey{forge: "github", section: model.SectionAuthored},
		items: []model.Item{mkItem("github", "github.com", "acme/widget", "X", 1, "")},
		first: true,
		warnings: []model.Warning{
			{Forge: "github", Section: model.SectionAuthored, Kind: "degraded", Msg: "datos parciales vía REST"},
		},
	})
	if m.streams[streamKey{forge: "github", section: model.SectionAuthored}].complete {
		t.Fatal("un fallback degradado no debe marcarse como completo")
	}
	if view := stripANSI(m.View().Content); !strings.Contains(view, "datos parciales") {
		t.Errorf("la sección debería avisar de datos parciales\n%s", view)
	}
}

// TestBrowserCommand cubre M1: el abridor se elige por plataforma.
func TestBrowserCommand(t *testing.T) {
	cases := map[string]string{
		"linux":   "xdg-open",
		"darwin":  "open",
		"windows": "rundll32",
	}
	for goos, want := range cases {
		if bin, args := browserCommand(goos, "http://x/y"); bin != want || args[len(args)-1] != "http://x/y" {
			t.Errorf("browserCommand(%s) = %s %v", goos, bin, args)
		}
	}
}

func TestOpenBrowserWithoutURL(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = press(t, m, "o")
	if !strings.Contains(lastToast(m), "no URL") {
		t.Fatalf("toast = %q", lastToast(m))
	}
}
