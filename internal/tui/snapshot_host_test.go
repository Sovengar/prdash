package tui

import (
	"testing"

	"prdash/internal/cache"
	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

func TestTheSnapshotStoresEachForgesHost(t *testing.T) {
	adapt := &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"}
	m := newTestModel(t, adapt)

	if st := m.statuses["github"]; st == nil {
		t.Fatal("the registered adapter has no state, and the test needs one with a host")
	}

	m.applySnapshot(snapshotWith(mkItem("github", "github.com", "acme/widget", "one", 1, "")))
	m.rebuild()

	f := m.snapshot()
	if len(f.Streams) == 0 {
		t.Fatal("the snapshot came out with no streams, and the test needs at least one")
	}

	vistos := map[string]bool{}
	for _, s := range f.Streams {
		vistos[s.Forge] = true
		if s.Forge == "github" && s.Host == "" {
			t.Errorf("the github stream was stored without host. The host is what separates "+
				"two streams of the same forge in two instances, and without it the next "+
				"open cannot tell which is which: %+v", s)
		}
		if s.Forge == "github" && s.Host != "github.com" {
			t.Errorf("the github stream was stored with host %q, want github.com", s.Host)
		}
	}
	if !vistos["github"] {
		t.Error("the snapshot has no github stream")
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
	withHost, noState := 0, 0
	for _, s := range f.Streams {
		if s.Forge == "github" && s.Host == "github.com" {
			withHost++
		}
		if s.Forge == "gitlab" && s.Host == "" {
			noState++
		}
	}
	if withHost == 0 {
		t.Error("after inserting a forge with no state, the github stream lost its host")
	}
	if noState == 0 {
		t.Error("the stream of a forge with no state did not reach the snapshot: losing it " +
			"entirely would be worse than storing it without host")
	}
}

func TestTheSnapshotIsOrderedByForgeSectionAndKind(t *testing.T) {
	adapt := &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"}
	m := newTestModel(t, adapt)

	// The keys are written in an order that is NOT the sorted one, and there are SEVERAL kinds.
	keys := []struct{ forge, section, kind string }{
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
	for _, c := range keys {
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
		t.Fatalf("the snapshot has %d streams, want %d", len(f.Streams), len(want))
	}
	for i, s := range f.Streams {
		got := s.Forge + "/" + string(s.Section) + "/" + string(s.Kind)
		if got != want[i] {
			t.Errorf("stream %d is %q, want %q. The snapshot is ordered by forge, then "+
				"section and then kind, and the order is alphabetical: it is compared across "+
				"runs to know whether the inbox changed, and an inverted order "+
				"produces a \u201cchanged\u201d every time it opens",
				i, got, want[i])
		}
	}
}
