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

func TestTheTickOnlyArmsWithAutoRefresh(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	withInterval := config.Defaults()
	withInterval.RefreshInterval = 2 * time.Second
	m := New(withInterval, nil)
	if !m.tickPending {
		t.Error("with a refresh interval no tick was left pending: nothing would ever be fetched")
	}

	// Zero is the default for "off", and that is the case worth looking at.
	withoutInterval := config.Defaults()
	withoutInterval.RefreshInterval = 0
	m = New(withoutInterval, nil)
	if m.tickPending {
		t.Error("without a refresh interval a tick was left pending: a loop that fetches forever")
	}
}

// The boundary is the cursor one past the last row.
func TestSelectedDoesNotLeaveTheRange(t *testing.T) {
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	m.applySnapshot(snapshotWith(mkItem("github", "github.com", "acme/widget", "one", 1, "")))
	m.rebuild()

	if got, ok := m.selected(); !ok || got.Number != 1 {
		t.Fatalf("with the cursor at 0, selected = %d, %v; want 1, true", got.Number, ok)
	}

	// A cursor one past the last row returns no item; with a `>` instead the last row would.
	for _, cursor := range []int{1, 2, 100} {
		m.cursor = cursor
		if got, ok := m.selected(); ok {
			t.Errorf("with the cursor at %d it returned item %d: it is outside the list of %d",
				cursor, got.Number, len(m.rows()))
		}
		got, _ := m.selected()
		if got != (model.Item{}) {
			t.Errorf("with the cursor out of range it returned %+v, want the zero item: "+
				"a half item looks like a good one", got)
		}
	}

	m.applySnapshot(snapshotWith())
	m.rebuild()
	m.cursor = 0
	if _, ok := m.selected(); ok {
		t.Error("with an empty list it returned an item")
	}
}

func TestSaveSnapshotWithoutPathWritesNothing(t *testing.T) {
	dir := t.TempDir()
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	m.applySnapshot(snapshotWith(mkItem("github", "github.com", "acme/widget", "one", 1, "")))
	m.rebuild()

	// No path: it writes nothing. And does not fail.
	m.cachePath = ""
	before := entriesIn(t, dir)
	m.saveSnapshot()
	if after := entriesIn(t, dir); len(after) != len(before) {
		t.Errorf("with no cache path files appeared: %v", after)
	}

	// With a path it writes, and it writes THERE.
	m.cachePath = filepath.Join(dir, "snapshot.json")
	m.saveSnapshot()
	if !waitFile(t, m.cachePath) {
		t.Fatalf("with a cache path it did not write the snapshot to %s", m.cachePath)
	}
	// The file it wrote is valid JSON, which is the only thing that makes it good.
	raw, err := os.ReadFile(m.cachePath)
	if err != nil {
		t.Fatalf("the written snapshot could not be read: %v", err)
	}
	if len(raw) == 0 {
		t.Error("the written snapshot is empty")
	}
	var f cache.File
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Errorf("the written snapshot is not valid JSON: %v", err)
	}
	if len(f.Streams) == 0 {
		t.Error("the written snapshot has no stream: it does not store what is on screen")
	}
}

func snapshotWith(items ...model.Item) cache.File {
	return cache.File{Streams: []cache.Stream{{
		Forge:   "github",
		Host:    "github.com",
		Section: model.SectionReview,
		Kind:    model.ReviewRequested,
		Items:   items,
	}}}
}

// A stream can arrive from a forge that is no longer in the configuration.
func TestAStreamOfAnUnconfiguredForgeDoesNotPanic(t *testing.T) {
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})

	m = send(t, m, pageMsg{
		cycle:     1,
		key:       streamKey{forge: "gitlab", section: model.SectionReview},
		items:     []model.Item{mkItem("gitlab", "gitlab.com", "acme/widget", "one", 1, "")},
		unchanged: true,
	})

	if _, ok := m.statuses["gitlab"]; ok {
		t.Error("a state appeared for a forge that is not in the config")
	}
	if m.streams[streamKey{forge: "gitlab", section: model.SectionReview}] == nil {
		t.Error("the stream of an unconfigured forge was not stored")
	}
}

func TestReviewKindIsOnlyStampedinTheReviewSection(t *testing.T) {
	for _, section := range []model.Section{model.SectionReview, model.SectionAuthored, model.SectionMentions} {
		m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
		item := mkItem("github", "github.com", "acme/widget", "one", 1, "")

		m = send(t, m, pageMsg{
			cycle: 1,
			key:   streamKey{forge: "github", section: section, kind: model.ReviewRequested},
			items: []model.Item{item},
			first: true,
		})

		got := m.streams[streamKey{forge: "github", section: section, kind: model.ReviewRequested}]
		if got == nil {
			t.Fatalf("section %v: the stream was not stored", section)
		}
		has := got.items[0].ReviewKind != ""
		wants := section == model.SectionReview
		if has != wants {
			t.Errorf("section %v: ReviewKind %q present=%v, wants %v. "+
				"A ReviewKind outside the review section says the item is a review in a list where it is not",
				section, got.items[0].ReviewKind, has, wants)
		}
	}
}

func waitFile(t *testing.T, path string) bool {
	t.Helper()
	for range 200 {
		if _, err := os.Stat(path); err == nil {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

func entriesIn(t *testing.T, dir string) []string {
	t.Helper()
	got, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range got {
		names = append(names, e.Name())
	}
	return names
}

func TestTheForgeStateIsOnlyStampedWithHost(t *testing.T) {
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})

	m.applySnapshot(cache.File{Streams: []cache.Stream{{
		Forge: "github", Host: "github.com", Section: model.SectionReview,
		Items: []model.Item{mkItem("github", "github.com", "acme/widget", "one", 1, "")},
	}}})
	if got := m.statuses["github"].host; got != "github.com" {
		t.Errorf("with host it gave %q, want github.com", got)
	}

	// An empty host does NOT erase a known one.
	m.applySnapshot(cache.File{Streams: []cache.Stream{{Forge: "github", Host: ""}}})
	if got := m.statuses["github"].host; got != "github.com" {
		t.Errorf("a snapshot without host erased the known host: it is now %q. "+
			"An empty host means not knowing it, not knowing there is none", got)
	}

	m.applySnapshot(cache.File{Streams: []cache.Stream{{Forge: "gitlab", Host: "gitlab.com"}}})
}
