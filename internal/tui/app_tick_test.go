package tui

import (
	"testing"
	"time"

	"prdash/internal/config"
	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

// ItemState's re-read does not know the inbox section.
func TestMergeItemLosesNeitherTheSectionNorTheReviewKind(t *testing.T) {
	old := model.NewItem(model.RepoRef{Forge: "gitlab", Host: "gitlab.example.com", Project: "grp/proj"}, 5)
	old.Section = model.SectionReview
	old.ReviewKind = model.ReviewAssigned
	old.Title = "old title"

	// The re-read arrives with no section and no kind: both are recovered from the original.
	fresh := model.NewItem(old.Ref, 5)
	fresh.Title = "new title"
	got := mergeItem(old, fresh)
	if got.Section != model.SectionReview {
		t.Errorf("Section = %q, want %q: without it the item disappears from the list", got.Section, model.SectionReview)
	}
	if got.ReviewKind != model.ReviewAssigned {
		t.Errorf("ReviewKind = %q, want %q", got.ReviewKind, model.ReviewAssigned)
	}
	if got.Title != "new title" {
		t.Errorf("Title = %q, want the reread one", got.Title)
	}

	fresh = model.NewItem(old.Ref, 5)
	fresh.Section = model.SectionAuthored
	fresh.ReviewKind = ""
	got = mergeItem(old, fresh)
	if got.Section != model.SectionAuthored {
		t.Errorf("Section = %q, want the reread one (%q)", got.Section, model.SectionAuthored)
	}
	// An EMPTY ReviewKind IS recovered: the re-read never knows it, and a lost kind would drop the
	// item out of its column.
	if got.ReviewKind != model.ReviewAssigned {
		t.Errorf("ReviewKind = %q, want the original one: ItemState never brings it", got.ReviewKind)
	}
}

// The auto-refresh interval is three decisions.
func TestTickIntervalWithBackoffAndCap(t *testing.T) {
	m := newTestModel(t, ghAdapter())

	m.cfg.RefreshInterval = 0
	m.backoff = 0
	if got := m.tickInterval(); got != 0 {
		t.Errorf("with interval 0, tickInterval = %v, want 0 (disabled)", got)
	}
	m.backoff = 10 * time.Minute
	if got := m.tickInterval(); got != 0 {
		t.Errorf("with interval 0 and backoff %v, tickInterval = %v, want 0", 10*time.Minute, got)
	}

	// A positive base: the backoff adds up, it does not replace.
	m.cfg.RefreshInterval = 30 * time.Second
	m.backoff = 0
	if got := m.tickInterval(); got != 30*time.Second {
		t.Errorf("without backoff = %v, want 30s", got)
	}
	m.backoff = 5 * time.Second
	if got := m.tickInterval(); got != 35*time.Second {
		t.Errorf("with backoff 5s = %v, want 35s (added, not replacing)", got)
	}
}

// armTick guarantees ONE tick chain: two chains would re-refresh on every keystroke.
func TestArmTickDoesNotChainTicks(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m.cfg.RefreshInterval = 0
	m.tickPending = false
	if cmd := m.armTick(); cmd != nil {
		t.Error("with refresh disabled armTick should not arm a tick")
	}
	if m.tickPending {
		t.Error("armTick should not mark tickPending when it arms nothing")
	}

	m = newTestModel(t, ghAdapter())
	m.cfg.RefreshInterval = 30 * time.Second
	m.tickPending = false
	if cmd := m.armTick(); cmd == nil {
		t.Fatal("with refresh active armTick should arm a tick")
	}
	if !m.tickPending {
		t.Error("armTick should mark the tick as pending")
	}
	// A second armTick with one pending arms no other: this is the single chain.
	if cmd := m.armTick(); cmd != nil {
		t.Error("with a tick already pending armTick should not arm another (double chain)")
	}
}

// The backoff exists so as not to make a rate limit worse.
func TestRecomputeBackoffGrowsOnlyWithRateLimitOrTimeout(t *testing.T) {
	new := func() Model {
		m := newTestModel(t, ghAdapter())
		m.cfg.RefreshInterval = 30 * time.Second
		return m
	}

	// No warnings: back to zero.
	m := new()
	m.recomputeBackoff()
	if m.backoff != 0 {
		t.Errorf("with no warnings, backoff = %v, want 0", m.backoff)
	}

	for _, kind := range []string{"notfound", "permission", "auth", "validation", "conflict", "network"} {
		m = new()
		m.statuses["github"].warnings = []model.Warning{{Forge: "github", Kind: kind, Msg: "x"}}
		m.recomputeBackoff()
		if m.backoff != 0 {
			t.Errorf("warning %q raised the backoff to %v, want 0: waiting does not fix it", kind, m.backoff)
		}
	}

	for _, kind := range []string{"ratelimit", "timeout"} {
		m = new()
		m.statuses["github"].warnings = []model.Warning{{Forge: "github", Kind: kind, Msg: "x"}}
		m.recomputeBackoff()
		if m.backoff != 30*time.Second {
			t.Errorf("warning %q gave backoff %v, want the interval (30s) as base", kind, m.backoff)
		}
		// And repeating it doubles it...
		m.recomputeBackoff()
		if m.backoff != time.Minute {
			t.Errorf("the second %q gave %v, want the double (60s)", kind, m.backoff)
		}
		// ...up to the ceiling, which is what stops the inbox going hours without a refresh.
		for range 20 {
			m.recomputeBackoff()
		}
		if m.backoff != maxBackoff {
			t.Errorf("the backoff went past the cap: %v, want %v", m.backoff, maxBackoff)
		}
	}

	// With the interval at zero the backoff base drops to a minute instead of adding to zero.
	m = new()
	m.cfg.RefreshInterval = 0
	m.statuses["github"].warnings = []model.Warning{{Forge: "github", Kind: "ratelimit"}}
	m.recomputeBackoff()
	if m.backoff != 60*time.Second {
		t.Errorf("with interval 0 the base backoff = %v, want 1m", m.backoff)
	}
}

// Pagination repeats the same warning on every page.
func TestAppendWarningsDoesNotRepeatTheSameNotice(t *testing.T) {
	base := []model.Warning{{Forge: "github", Section: model.SectionReview, Kind: "network", Msg: "boom"}}

	got := base
	for range 3 {
		got = appendWarnings(got, []model.Warning{{Forge: "github", Section: model.SectionReview, Kind: "network", Msg: "boom"}})
	}
	if len(got) != 1 {
		t.Errorf("the same warning repeated ended at %d, want 1", len(got))
	}

	got = appendWarnings(got, []model.Warning{{Forge: "github", Section: model.SectionReview, Kind: "network", Msg: "other"}})
	if len(got) != 2 {
		t.Errorf("a different message should be another notice, ended at %d", len(got))
	}

	got = appendWarnings(got, []model.Warning{{Forge: "github", Section: model.SectionAuthored, Kind: "network", Msg: "boom"}})
	if len(got) != 3 {
		t.Errorf("another section should be another notice, ended at %d", len(got))
	}

	// Distinto tipo: other.
	got = appendWarnings(got, []model.Warning{{Forge: "github", Section: model.SectionReview, Kind: "auth", Msg: "boom"}})
	if len(got) != 4 {
		t.Errorf("another kind should be another notice, ended at %d", len(got))
	}

	// The Forge is NOT part of the key: the same warning from two forges is kept once, on
	// purpose.
	got = appendWarnings(got, []model.Warning{{Forge: "gitlab", Section: model.SectionReview, Kind: "network", Msg: "boom"}})
	if len(got) != 4 {
		t.Errorf("the same notice of another forge should not duplicate, ended at %d", len(got))
	}

	got = appendWarnings(nil, nil)
	if got != nil {
		t.Errorf("with no origin appendWarnings = %v, want nil", got)
	}
}

func TestPausedWithLoadingOrAction(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	if m.paused() {
		t.Error("with nothing in flight it should not be paused")
	}
	m.loading = true
	if !m.paused() {
		t.Error("with a load in flight it should pause")
	}
	m.loading = false
	m.actionBusy = true
	if !m.paused() {
		t.Error("with an action in flight it should pause")
	}
}

// The "loading more" indicator belongs to the section being paged.
func TestSectionLoadingMoreLooksAtTheActiveSection(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m.streams[streamKey{forge: "github", section: model.SectionReview, kind: model.ReviewRequested}] = &stream{more: true}

	if !m.sectionLoadingMore(model.SectionReview) {
		t.Error("the section with pending pages should say so")
	}
	if m.sectionLoadingMore(model.SectionAuthored) {
		t.Error("another section should not load more: the indicator belongs to the one being painted")
	}

	for k, s := range m.streams {
		s.more = false
		m.streams[k] = s
	}
	for _, kind := range []model.Section{model.SectionReview, model.SectionAuthored, model.SectionMentions} {
		if m.sectionLoadingMore(kind) {
			t.Errorf("with the page closed, %v should not load more", kind)
		}
	}
}

func TestSaveSnapshotWithoutPathPersistsNothing(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	if m.cachePath != "" {
		t.Fatalf("the fixture should leave cachePath empty, gave %q", m.cachePath)
	}
	// No panic and no write: the call is a no-op. An empty cache path would be an error.
	m.saveSnapshot()
}

func TestConfigDefaultsCoverTheThreeForges(t *testing.T) {
	cfg := config.Defaults()
	if !cfg.Forges.GitHub.Enabled || cfg.Forges.GitHub.Host != "github.com" {
		t.Errorf("github = %+v, want habilitado en github.com", cfg.Forges.GitHub)
	}
	if !cfg.Forges.GitLab.Enabled || cfg.Forges.GitLab.APIBase != "/api/v4/" {
		t.Errorf("gitlab = %+v, want enabled with the default API base", cfg.Forges.GitLab)
	}
	// Bitbucket disabled on purpose: no adapter speaks to it beyond the probe.
	if cfg.Forges.Bitbucket.Enabled {
		t.Error("bitbucket should not come enabled by default")
	}
	for _, name := range []string{"github", "gitlab", "bitbucket"} {
		a := &testutil.FakeAdapter{ForgeName: name, HostName: name + ".example.com"}
		testutil.RunConformance(t, a, testutil.ConformanceOptions{})
	}
}
