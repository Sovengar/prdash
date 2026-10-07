package tui

import (
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"prdash/internal/cache"
	"prdash/internal/config"
	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/state"
	"prdash/internal/testutil"
)

func newTestModel(t *testing.T, adapters ...forge.Adapter) Model {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", t.TempDir()) // isolates the real cache
	m := New(config.Defaults(), adapters)
	m.width, m.height = 160, 40
	m.loading = false
	m.cachePath = ""
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
	// A real forge always reports the source branch's commit, and without it the adapter refuses.
	it.HeadSHA = "head-" + project + "-" + strconv.Itoa(number)
	it.Merge = model.MergeRulesAll()
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
	out, _ := m.Update(keyMsg(t, key))
	return out.(Model)
}

func pressWithCmd(t *testing.T, m Model, key string) (Model, tea.Cmd) {
	t.Helper()
	out, cmd := m.Update(keyMsg(t, key))
	return out.(Model), cmd
}

func keyMsg(t *testing.T, key string) tea.KeyPressMsg {
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
	case "enter":
		km = tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		km = tea.KeyPressMsg{Code: tea.KeyEscape}
	case "backspace":
		km = tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "ctrl+u":
		km = tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl}
	default:
		if len(key) == 1 && unicode.IsUpper(rune(key[0])) {
			km = tea.KeyPressMsg{Code: unicode.ToLower(rune(key[0])), Mod: tea.ModShift, Text: key}
		}
	}
	return km
}

func toastTexts(m Model) []string { return m.toast.texts() }

func lastToast(m Model) string { return m.toast.last() }

func lastToastLevel(m Model) toastLevel {
	if len(m.toast.toasts) == 0 {
		return toastInfo
	}
	return m.toast.toasts[len(m.toast.toasts)-1].level
}

func assertToast(t *testing.T, m Model, want string) {
	t.Helper()
	if !strings.Contains(lastToast(m), want) {
		t.Fatalf("toast = %q, want %q (alive: %q)", lastToast(m), want, toastTexts(m))
	}
}

func ghAdapter() *testutil.FakeAdapter {
	return &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"}
}

func showSection(m Model, kind model.Section) Model {
	m.setActiveSection(kind)
	return m
}

func TestLegendCountsBothForgesAndListShowsActiveOnly(t *testing.T) {
	m := newTestModel(t, ghAdapter(), &testutil.FakeAdapter{ForgeName: "gitlab", HostName: "gitlab.example.com"})

	reviewReq := mkItem("github", "github.com", "acme/lib", "Review me", 2, "REVIEW_REQUIRED")
	reviewReq.ReviewKind = model.ReviewRequested
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{mkItem("github", "github.com", "acme/widget", "Add widget", 1, "APPROVED")}, false))
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{reviewReq}, false))
	m = send(t, m, page(1, "gitlab", "gitlab.example.com", model.SectionAuthored, "", []model.Item{mkItem("gitlab", "gitlab.example.com", "grp/proj", "MR propio", 3, "APPROVED")}, false))
	m = send(t, m, page(1, "gitlab", "gitlab.example.com", model.SectionMentions, "", []model.Item{mkItem("gitlab", "gitlab.example.com", "grp/proj", "Heads up", 5, "")}, false))

	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "Mine (2) · Assigned (1) · Mentioned (1)") {
		t.Errorf("the legend does not reflect the counts of the three sections of both forges:\n%s", view)
	}
	if !strings.Contains(view, "Review me") {
		t.Errorf("the list should show the Assigned items, the active one:\n%s", view)
	}
	for _, hidden := range []string{"Add widget", "Own MR", "Heads up"} {
		if strings.Contains(view, hidden) {
			t.Errorf("the list shows %q, which belongs to another section:\n%s", hidden, view)
		}
	}
}

func TestSectionEmptyVsError(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, pageMsg{cycle: 1, key: streamKey{forge: "github", section: model.SectionReview}, warnings: []model.Warning{{Forge: "github", Section: model.SectionReview, Kind: "network", Msg: "boom"}}})

	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "could not be queried") {
		t.Errorf("the failed section should say so\n%s", view)
	}
	if strings.Contains(view, "(empty)") {
		t.Errorf("a section with a notice should not say it is empty\n%s", view)
	}
	if !strings.Contains(view, "Assigned (0)") {
		t.Errorf("the legend should count 0 on the failed section\n%s", view)
	}
}

func TestDegradationKeepsOtherForges(t *testing.T) {
	m := newTestModel(t, ghAdapter(), &testutil.FakeAdapter{ForgeName: "gitlab", HostName: "gitlab.example.com"})
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{mkItem("github", "github.com", "acme/widget", "Sigue visible", 1, "")}, false))
	m = send(t, m, authMsg{cycle: 1, forge: "gitlab", auth: model.AuthState{Forge: "gitlab", OK: false, Reason: "401"}})
	m = send(t, m, pageMsg{cycle: 1, key: streamKey{forge: "gitlab", section: model.SectionReview}, warnings: []model.Warning{{Forge: "gitlab", Kind: "auth", Msg: "401"}}})

	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "Sigue visible") {
		t.Errorf("the GitHub items should stay visible\n%s", view)
	}
	if !strings.Contains(view, "gitlab ✗") {
		t.Errorf("gitlab should report itself as down\n%s", view)
	}
}

func TestPaginationIndicator(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{mkItem("github", "github.com", "acme/widget", "One", 1, "")}, true))

	if !m.sectionLoadingMore(model.SectionReview) {
		t.Fatal("review, the active section, should keep paginating")
	}
	if view := stripANSI(m.View().Content); !strings.Contains(view, "loading more…") {
		t.Errorf("the active sections loading indicator is missing\n%s", view)
	}
}

func TestIncrementalPages(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{mkItem("github", "github.com", "acme/widget", "One", 1, "")}, true))

	next := page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{mkItem("github", "github.com", "acme/widget", "Two", 2, "")}, true)
	next.first = false
	m = send(t, m, next)

	if got := len(m.sectionItems(model.SectionAuthored)); got != 2 {
		t.Fatalf("authored = %d, want 2", got)
	}

	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{mkItem("github", "github.com", "acme/widget", "Nuevo", 3, "")}, false))
	items := m.sectionItems(model.SectionAuthored)
	if len(items) != 1 || items[0].Title != "Nuevo" {
		t.Fatalf("authored after refreshing = %+v", items)
	}
}

func TestManualRefreshIncrementsCycle(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	before := m.cycle
	m = press(t, m, "R")
	if m.cycle != before+1 {
		t.Fatalf("cycle = %d, want %d", m.cycle, before+1)
	}
	if !m.loading {
		t.Fatal("it should be left loading")
	}
}

func TestAutoRefreshTick(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	before := m.cycle
	m = send(t, m, tickMsg{})
	if m.cycle != before+1 || !m.loading {
		t.Fatalf("the tick did not start a refresh: cycle=%d loading=%v", m.cycle, m.loading)
	}
}

func TestAutoRefreshPausedDuringAction(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m.actionBusy = true
	before := m.cycle
	m = send(t, m, tickMsg{})
	if m.cycle != before {
		t.Fatalf("the tick should not start a refresh with an action in flight (cycle=%d)", m.cycle)
	}
	if m.loading {
		t.Fatal("it should not be left loading")
	}
	if !m.actionBusy {
		t.Fatal("the action in flight should not be cancelled")
	}
}

func TestRefreshUpdatesOtherItemsDuringAction(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m.actionBusy = true
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{mkItem("github", "github.com", "acme/widget", "Other", 9, "")}, false))
	if got := len(m.sectionItems(model.SectionAuthored)); got != 1 {
		t.Fatalf("the refresh should update the rest of the items: %d", got)
	}
}

func detailPanel(view string) string {
	lines := strings.Split(stripANSI(view), "\n")
	for i, l := range lines {
		if strings.Contains(l, "╭ acme/") {
			return strings.Join(lines[i:], "\n")
		}
	}
	return ""
}

func TestDetailPanelFollowsCursor(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{
		mkItem("github", "github.com", "acme/widget", "Add widget", 1, "APPROVED"),
		mkItem("github", "github.com", "acme/widget", "Other", 2, ""),
	}, false))

	panel := detailPanel(m.View().Content)
	if panel == "" {
		t.Fatalf("there is no detail box in the view:\n%s", m.View().Content)
	}
	for _, want := range []string{"Add widget", "Author", "Source", "feat/x", "Target", "main", "#1"} {
		if !strings.Contains(panel, want) {
			t.Errorf("the panel does not contain %q\n%s", want, panel)
		}
	}

	m = press(t, m, "down")
	panel = detailPanel(m.View().Content)
	if !strings.Contains(panel, "Other") || !strings.Contains(panel, "#2") {
		t.Errorf("the panel did not follow the cursor\n%s", panel)
	}
	if strings.Contains(panel, "#1") || strings.Contains(panel, "Add widget") {
		t.Errorf("the panel still shows the previous item\n%s", panel)
	}
}

func TestApproveOKUpdatesNotice(t *testing.T) {
	item := mkItem("github", "github.com", "acme/widget", "Add widget", 1, "")
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{item}, false))

	m = send(t, m, actionMsg{outcome: forge.Outcome{Kind: forge.ActionApprove, ID: item.ID(), OK: true}})
	if !strings.Contains(lastToast(m), "approve ok") {
		t.Fatalf("toast = %q", lastToast(m))
	}
	if m.actionBusy {
		t.Fatal("the action should have finished")
	}
}

func TestApproveOwnPulledBeforeForge(t *testing.T) {
	cases := []struct {
		name    string
		section model.Section
		kind    model.ReviewKind
		author  string
		login   string
	}{
		{"login coincide", model.SectionAuthored, "", "Sovengar", "Sovengar"},
		{"another author with login", model.SectionReview, model.ReviewRequested, "someone", "Sovengar"},
		{"no login, own section", model.SectionAuthored, "", "whoever", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			item := mkItem("github", "github.com", "acme/widget", "Add widget", 11, "")
			item.Author = tc.author
			fake := &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"}
			m := newTestModel(t, fake)
			m = send(t, m, authMsg{cycle: 1, forge: "github", auth: model.AuthState{Forge: "github", OK: true, Login: tc.login}})
			m = send(t, m, page(1, "github", "github.com", tc.section, tc.kind, []model.Item{item}, false))
			m = showSection(m, tc.section)

			blocked := m.selfDenied[item.ID()] != ""
			m = press(t, m, "a")

			if tc.author == "someone" {
				if blocked {
					t.Fatal("a PR by another author must not be vetoed")
				}
				if !m.actionBusy {
					t.Fatal("approving a PR by another author should launch the action")
				}
				return
			}
			if !blocked {
				t.Fatal("a PR of my own should stay vetoed")
			}
			if n := fake.ActionCallCount("approve", item.Ref, item.Number); n != 0 {
				t.Fatalf("Approve was called %d times: the veto must cut before the subprocess", n)
			}
			if m.actionBusy {
				t.Fatal("no action should be left in flight")
			}
			if !strings.Contains(lastToast(m), "you cannot approve your own") {
				t.Fatalf("toast = %q", lastToast(m))
			}
		})
	}
}

func TestOwnItemShowsRoleAndDetail(t *testing.T) {
	own := mkItem("github", "github.com", "acme/widget", "Mine", 12, "")
	own.Author = "Sovengar"
	other := mkItem("github", "github.com", "acme/widget", "From another", 13, "")
	other.Author = "someone"
	m := newTestModel(t, ghAdapter())
	m = send(t, m, authMsg{cycle: 1, forge: "github", auth: model.AuthState{Forge: "github", OK: true, Login: "Sovengar"}})
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{own}, false))
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{other}, false))
	m = showSection(m, model.SectionAuthored)

	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "own") {
		t.Errorf("the row of my own PR should mark the role:\n%s", view)
	}

	// The veto does not take rows of the card.
	if view := stripANSI(m.View().Content); strings.Contains(view, "approve unavailable") {
		t.Errorf("the veto on approving my own should not take a row of the card:\n%s", view)
	}

	m = press(t, m, "a")
	assertToast(t, m, state.SelfReviewReason)
}

func TestSelfDenySurvivesRefresh(t *testing.T) {
	item := mkItem("github", "github.com", "acme/widget", "Add widget", 14, "")
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{item}, false))
	m = showSection(m, model.SectionAuthored)
	if m.selfDenied[item.ID()] == "" {
		t.Fatal("my own item should stay vetoed")
	}
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{item}, false))
	if m.selfDenied[item.ID()] == "" {
		t.Fatal("a refresh must not lift the veto on approving my own")
	}
}

func TestConflictRefreshesItem(t *testing.T) {
	item := mkItem("github", "github.com", "acme/widget", "Add widget", 7, "")
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{item}, false))

	refreshed := item
	refreshed.State = "MERGED"
	m = send(t, m, actionMsg{outcome: forge.Outcome{
		Kind: forge.ActionMerge, ID: item.ID(),
		Conflict: true, Msg: "the item is already merged",
		Item: refreshed, HasItem: true,
	}})

	if !strings.Contains(lastToast(m), "forge conflict") {
		t.Fatalf("toast = %q", lastToast(m))
	}
	items := m.sectionItems(model.SectionAuthored)
	if len(items) != 1 || items[0].State != "MERGED" {
		t.Fatalf("the item should have been refreshed: %+v", items)
	}
}

func TestPermissionRecordsDenial(t *testing.T) {
	item := mkItem("gitlab", "gitlab.example.com", "grp/proj", "MR", 4, "")
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "gitlab", HostName: "gitlab.example.com"})
	m = send(t, m, page(1, "gitlab", "gitlab.example.com", model.SectionAuthored, "", []model.Item{item}, false))
	m = showSection(m, model.SectionAuthored)

	m = send(t, m, actionMsg{outcome: forge.Outcome{Kind: forge.ActionApprove, ID: item.ID(), Perm: true, Msg: "no tienes permiso"}})
	if !strings.Contains(lastToast(m), "disabled") {
		t.Fatalf("toast = %q", lastToast(m))
	}
	if m.denied[item.ID()] == "" {
		t.Fatal("the denial should be recorded")
	}

	m = press(t, m, "a")
	if !strings.Contains(lastToast(m), "no tienes permiso") {
		t.Fatalf("toast after retry = %q", lastToast(m))
	}
}

func TestActionDisabledWhenForgeDown(t *testing.T) {
	m := newTestModel(t, &testutil.FakeAdapter{
		ForgeName: "gitlab", HostName: "gitlab.example.com",
		AuthState: model.AuthState{Forge: "gitlab", OK: false, Reason: "401"},
	})
	m = send(t, m, authMsg{cycle: 1, forge: "gitlab", auth: model.AuthState{Forge: "gitlab", OK: false, Reason: "401"}})
	m = send(t, m, page(1, "gitlab", "gitlab.example.com", model.SectionAuthored, "", []model.Item{mkItem("gitlab", "gitlab.example.com", "grp/proj", "MR", 4, "")}, false))
	m = showSection(m, model.SectionAuthored)

	m = press(t, m, "a")
	if m.actionBusy {
		t.Fatal("the action should not start with the forge down")
	}
	toast := lastToast(m)
	if !strings.Contains(toast, "401") {
		t.Errorf("toast = %q, want the adapters reason", toast)
	}
	if strings.Contains(toast, "not authenticated") {
		t.Errorf("toast = %q: the generic label covers the real reason", toast)
	}
}

func TestSnapshotPaintsInstantly(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	path, err := cache.Path()
	if err != nil {
		t.Fatal(err)
	}
	it := mkItem("github", "github.com", "acme/widget", "Cached", 1, "")
	if err := cache.Save(path, cache.File{Streams: []cache.Stream{{
		Forge: "github", Host: "github.com", Section: model.SectionReview, Kind: model.ReviewRequested, Items: []model.Item{it},
	}}}); err != nil {
		t.Fatal(err)
	}

	m := New(config.Defaults(), []forge.Adapter{ghAdapter()})
	if got := len(m.sectionItems(model.SectionReview)); got != 1 {
		t.Fatalf("cached review = %d, want 1", got)
	}
	if view := stripANSI(m.View().Content); !strings.Contains(view, "Cached") {
		t.Errorf("the view should paint the cache\n%s", view)
	}
}

func TestPerForgeUpdateIndicator(t *testing.T) {
	m := newTestModel(t, ghAdapter(), &testutil.FakeAdapter{ForgeName: "gitlab", HostName: "gitlab.example.com"})
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{mkItem("github", "github.com", "acme/widget", "One", 1, "")}, false))

	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "github ✓ now") {
		t.Errorf("github should show its own time\n%s", view)
	}
	if !strings.Contains(view, "gitlab ✓ no data") {
		t.Errorf("gitlab should not inherit githubs time\n%s", view)
	}
}

func TestUnsupportedForgeShowsReason(t *testing.T) {
	m := newTestModel(t, &testutil.FakeAdapter{
		ForgeName: "bitbucket", HostName: "bitbucket.org",
		AuthState: model.AuthState{Forge: "bitbucket", OK: false, Reason: "unsupported in this version"},
	})
	m = send(t, m, authMsg{cycle: 1, forge: "bitbucket", auth: model.AuthState{Forge: "bitbucket", OK: false, Reason: "unsupported in this version"}})
	m = send(t, m, pageMsg{cycle: 1, key: streamKey{forge: "bitbucket", section: model.SectionReview}, warnings: []model.Warning{{Forge: "bitbucket", Section: model.SectionReview, Kind: "unsupported", Msg: "unsupported"}}})

	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "unsupported") {
		t.Errorf("the section should say it is not supported\n%s", view)
	}
	if strings.Contains(view, "bitbucket ✓") {
		t.Errorf("bitbucket should not report itself as operational\n%s", view)
	}
}

func TestUnchangedHead(t *testing.T) {
	cases := []struct {
		name string
		prev streamHead
		page forge.Page
		want bool
	}{
		{"same header", streamHead{cursor: "c1", complete: true}, forge.Page{Next: "c1", More: true}, true},
		{"not complete yet", streamHead{cursor: "c1", complete: false}, forge.Page{Next: "c1", More: true}, false},
		{"header distinta", streamHead{cursor: "c1", complete: true}, forge.Page{Next: "c2", More: true}, false},
		{"a single page", streamHead{cursor: "", complete: true}, forge.Page{Next: "", More: false}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := unchangedHead(c.prev, c.page); got != c.want {
				t.Fatalf("unchangedHead = %v, want %v", got, c.want)
			}
		})
	}
}

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
		t.Fatal("no pagination should be left pending")
	}

	m = send(t, m, pageMsg{cycle: m.cycle, key: streamKey{forge: "github", section: model.SectionAuthored}, unchanged: true})
	if got := len(m.sectionItems(model.SectionAuthored)); got != 2 {
		t.Fatalf("the refresh with no changes should keep the items: %d", got)
	}
}

func TestBackoffOnRateLimit(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	base := m.tickInterval()

	m.statuses["github"].warnings = []model.Warning{{Forge: "github", Kind: "ratelimit", Msg: "429"}}
	m.recomputeBackoff()
	if m.backoff <= 0 {
		t.Fatal("it should apply backoff on rate limit")
	}
	if m.tickInterval() <= base {
		t.Fatalf("tickInterval = %v, should exceed %v", m.tickInterval(), base)
	}

	m.statuses["github"].warnings = nil
	m.recomputeBackoff()
	if m.backoff != 0 {
		t.Fatalf("backoff = %v, should reset", m.backoff)
	}
}

func TestManualOnlyRefreshHasNoTick(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m.cfg.RefreshInterval = 0
	if m.tickCmd() != nil {
		t.Fatal("with interval 0 no tick should be scheduled")
	}
	if m.tickInterval() != 0 {
		t.Fatalf("tickInterval = %v, want 0", m.tickInterval())
	}
}

func TestCurrentCycleDrainsLoading(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = press(t, m, "R")
	if !m.loading {
		t.Fatal("the refresh must be left loading")
	}
	m = send(t, m, refreshDoneMsg{cycle: m.cycle})
	if m.loading {
		t.Fatal("the current cycle must lower loading")
	}
	if !m.tickPending {
		t.Fatal("it must rearm the tick")
	}
	before := m.cycle
	m = send(t, m, tickMsg{})
	if m.cycle == before {
		t.Fatalf("the tick must refresh again (cycle=%d)", m.cycle)
	}
}

// Covers M-1: cycles must not overlap.
func TestRefreshDoesNotOverlap(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = press(t, m, "R")
	cycle := m.cycle
	m = press(t, m, "R") // with the cycle in flight
	if m.cycle != cycle {
		t.Fatalf("an overlapping cycle must not start (cycle=%d, want %d)", m.cycle, cycle)
	}
}

func TestObsoleteRefreshDoneIsInert(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m.loading = true
	m.tickPending = false
	m = send(t, m, refreshDoneMsg{cycle: m.cycle - 1})
	if !m.loading {
		t.Fatal("a stale refreshDone must not lower loading")
	}
	if m.tickPending {
		t.Fatal("a stale refreshDone must not arm a tick")
	}
}

func TestArmTickSingleChain(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m.tickPending = false
	if m.armTick() == nil {
		t.Fatal("with no pending tick it should arm one")
	}
	if m.armTick() != nil {
		t.Fatal("with a pending tick it should not arm another")
	}
}

func TestSingleChannelReader(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	if m.readers != 1 {
		t.Fatalf("initial readers = %d, want 1", m.readers)
	}

	for i := 0; i < 5; i++ {
		m = send(t, m, tickMsg{})
		m = send(t, m, refreshDoneMsg{cycle: m.cycle})
	}
	m = press(t, m, "R")
	m = send(t, m, refreshDoneMsg{cycle: m.cycle})

	// An event from the channel consumes a reader and re-arms exactly one.
	m = send(t, m, authMsg{cycle: m.cycle, forge: "github", auth: model.AuthState{Forge: "github", OK: true}})

	if m.readers != 1 {
		t.Fatalf("readers = %d, want 1 (they must not accumulate)", m.readers)
	}
}

func TestDetailReflectsActionUpdate(t *testing.T) {
	item := mkItem("github", "github.com", "acme/widget", "Add widget", 1, "")
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{item}, false))
	m = showSection(m, model.SectionAuthored)

	refreshed := item
	refreshed.State = "MERGED"
	m = send(t, m, actionMsg{cycle: m.cycle, outcome: forge.Outcome{
		Kind: forge.ActionMerge, ID: item.ID(), Conflict: true, Msg: "already merged",
		Item: refreshed, HasItem: true,
	}})

	if view := stripANSI(m.View().Content); !strings.Contains(view, "merged") {
		t.Errorf("the detail should reflect the new state\n%s", view)
	}
}

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
		t.Fatalf("the reread state should apply: %+v", items)
	}
}

func TestRefreshClearsDenied(t *testing.T) {
	item := mkItem("gitlab", "gitlab.example.com", "grp/proj", "MR", 4, "")
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "gitlab", HostName: "gitlab.example.com"})
	m.denied[item.ID()] = "no tienes permiso"

	m = send(t, m, page(1, "gitlab", "gitlab.example.com", model.SectionAuthored, "", []model.Item{item}, false))
	if _, ok := m.denied[item.ID()]; ok {
		t.Fatal("a successful refresh should clear the denial")
	}
}

func TestDegradedDoesNotComplete(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, pageMsg{
		cycle: m.cycle,
		key:   streamKey{forge: "github", section: model.SectionReview, kind: model.ReviewRequested},
		items: []model.Item{mkItem("github", "github.com", "acme/widget", "X", 1, "")},
		first: true,
		warnings: []model.Warning{
			{Forge: "github", Section: model.SectionReview, Kind: "degraded", Msg: "partial data via REST"},
		},
	})
	if m.streams[streamKey{forge: "github", section: model.SectionReview, kind: model.ReviewRequested}].complete {
		t.Fatal("a degraded fallback must not be marked as complete")
	}
	if view := stripANSI(m.View().Content); !strings.Contains(view, "partial data") {
		t.Errorf("the section should warn about partial data\n%s", view)
	}
}

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
