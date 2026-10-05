package tui

import (
	"strings"
	"testing"
	"time"

	"prdash/internal/forge/model"
)

func TestOrderBranchesLeavesTheBaseFirstAndDeduplicates(t *testing.T) {
	cases := []struct {
		name  string
		names []string
		base  string
		want  []string
	}{
		{
			"the base first even if it is the last alphabetically",
			[]string{"zebra", "main", "alfa"},
			"main",
			[]string{"main", "alfa", "zebra"},
		},
		{
			"no base, all alphabetical",
			[]string{"zebra", "alfa", "medio"},
			"",
			[]string{"alfa", "medio", "zebra"},
		},
		{
			"no base and base missing from the listing",
			[]string{"zebra", "alfa"},
			"main",
			[]string{"alfa", "zebra"},
		},
		{
			"deduplicates keeping the first appearance",
			[]string{"main", "alfa", "main", "alfa"},
			"main",
			[]string{"main", "alfa"},
		},
		{
			"discards empties and spaces",
			[]string{"main", "", "   ", "alfa", "\talfa\t"},
			"main",
			[]string{"main", "alfa"},
		},
		{
			"only the base",
			[]string{"main"},
			"main",
			[]string{"main"},
		},
		{
			"the repeated base does not repeat the row",
			[]string{"main", "main", "other"},
			"main",
			[]string{"main", "other"},
		},
		{
			"nothing",
			nil,
			"main",
			nil,
		},
		{
			"only spaces",
			[]string{"", "  "},
			"main",
			nil,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := orderBranches(c.names, c.base)
			if strings.Join(got, ",") != strings.Join(c.want, ",") {
				t.Errorf("orderBranches(%q, %q) = %q, want %q", c.names, c.base, got, c.want)
			}
			// The base, if any, goes first: the invariant the window needs to keep it in view.
			if c.base != "" && contains(got, c.base) && len(got) > 0 && got[0] != c.base {
				t.Errorf("the base %q did not come out first: %q", c.base, got)
			}
		})
	}
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func TestFilterBranchesFiltersOnTheWholeNameAndCaseInsensitively(t *testing.T) {
	all := []string{"main", "release/2.0", "fix/hunk-pane-argv", "feature/Fix-Login", "wip"}

	if got := filterBranches(all, ""); strings.Join(got, ",") != strings.Join(all, ",") {
		t.Errorf("empty filter = %q, want the whole list", got)
	}
	if got := filterBranches(all, ""); &got[0] == &all[0] {
		t.Error("filterBranches returned the input slice, not a copy")
	}
	if got := filterBranches(all, "   "); len(got) != len(all) {
		t.Errorf("spaces filter = %q, want the whole list (the empty filter is ignored)", got)
	}

	// The WHOLE name, not the last segment: "hunk" only appears in "fix/hunk-pane-argv".
	if got := filterBranches(all, "hunk"); len(got) != 1 || got[0] != "fix/hunk-pane-argv" {
		t.Errorf("filter %q = %q, want only the working branch (the filter runs over the whole name)", "hunk", got)
	}
	if got := filterBranches(all, "fix"); len(got) != 2 {
		t.Errorf("filter %q = %q, want the two containing fix ignoring case", "fix", got)
	}
	if got := filterBranches(all, "FIX"); len(got) != 2 {
		t.Errorf("filter %q = %q, want the same as in lowercase", "FIX", got)
	}
	if got := filterBranches(all, "2.0"); len(got) != 1 || got[0] != "release/2.0" {
		t.Errorf("filter %q = %q", "2.0", got)
	}
	if got := filterBranches(all, "noexiste"); len(got) != 0 {
		t.Errorf("filter with no matches = %q, want empty", got)
	}
}

// The filter shrinks the view and the cursor is clamped to it.
func TestClampRetargetCursorLeavesNoCursorOutside(t *testing.T) {
	m := newTestModel(t, ghAdapter())

	m.retarget.view = nil
	m.retarget.cursor = 7
	m.clampRetargetCursor()
	if m.retarget.cursor != 0 {
		t.Errorf("with an empty view the cursor = %d, want 0", m.retarget.cursor)
	}
	if b, ok := m.selectedBranch(); ok || b != "" {
		t.Errorf("with an empty view selectedBranch = (%q, %v), want (\"\", false)", b, ok)
	}

	view := []string{"a", "b", "c"}
	for _, c := range []int{0, 1, 2} {
		m.retarget.view, m.retarget.cursor = view, c
		m.clampRetargetCursor()
		if m.retarget.cursor != c {
			t.Errorf("cursor %d moved to %d with no reason", c, m.retarget.cursor)
		}
	}
	for _, c := range []int{3, 4, 100} {
		m.retarget.view, m.retarget.cursor = view, c
		m.clampRetargetCursor()
		if m.retarget.cursor != len(view)-1 {
			t.Errorf("cursor %d ended at %d, want %d (the last row)", c, m.retarget.cursor, len(view)-1)
		}
	}
	m.retarget.view, m.retarget.cursor = view, 2
	if b, ok := m.selectedBranch(); !ok || b != "c" {
		t.Errorf("on the last row selectedBranch = (%q, %v), want (\"c\", true)", b, ok)
	}
	m.retarget.cursor = 3
	if b, ok := m.selectedBranch(); ok || b != "" {
		t.Errorf("one row past the end gave (%q, %v), want (\"\", false)", b, ok)
	}
	m.retarget.cursor = -1
	if b, ok := m.selectedBranch(); ok || b != "" {
		t.Errorf("a negative cursor gave (%q, %v), want (\"\", false)", b, ok)
	}
}

func TestTheBranchCacheExpiresRightAtTheBoundary(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	key := keyOf(m.retargetItemForTest())

	m.storeBranches(key, []string{"main", "otra"})
	if got, ok := m.cachedBranches(key); !ok || len(got) != 2 {
		t.Errorf("freshly cached = (%q, %v), want the two names", got, ok)
	}

	m.branchCache[key] = branchCache{names: []string{"main"}, fetchedAt: time.Now().Add(-branchCacheTTL)}
	fresh := true
	for _, delta := range []time.Duration{-time.Second, -time.Millisecond} {
		m.branchCache[key] = branchCache{names: []string{"main"}, fetchedAt: time.Now().Add(-branchCacheTTL - delta)}
		if _, ok := m.cachedBranches(key); !ok {
			fresh = false
			t.Errorf("a cache of %v before the TTL elapsed expired: the TTL is being applied too early", -delta)
		}
	}
	_ = fresh
	for _, delta := range []time.Duration{time.Millisecond, time.Second} {
		m.branchCache[key] = branchCache{names: []string{"main"}, fetchedAt: time.Now().Add(-branchCacheTTL - delta)}
		if _, ok := m.cachedBranches(key); ok {
			t.Errorf("a cache of %v after the TTL was still valid", -delta)
		}
	}

	if got, ok := m.cachedBranches(repoKey{forge: "other", host: "other.example.com", project: "x/y"}); ok || got != nil {
		t.Errorf("an uncached repo gave (%q, %v), want (nil, false)", got, ok)
	}
	m.storeBranches(key, nil)
	if got, ok := m.cachedBranches(key); !ok || got != nil {
		t.Errorf("a repo with an empty cached list gave (%q, %v), want (nil, true): it was already asked and there was nothing", got, ok)
	}

	// The key carries forge and host: the same owner/repo in two forges are two lists.
	other := repoKey{forge: "gitlab", host: key.host, project: key.project}
	m.storeBranches(other, []string{"main", "solo-gitlab"})
	m.storeBranches(key, []string{"main", "solo-github"})
	if got, _ := m.cachedBranches(other); !contains(got, "solo-gitlab") {
		t.Errorf("the gitlab key returned %q", got)
	}
	if got, _ := m.cachedBranches(key); contains(got, "solo-gitlab") {
		t.Errorf("the github key returned gitlab branches: %q", got)
	}
}

func TestClosingThePopupInvalidatesTheInFlightListing(t *testing.T) {
	m, _ := retargetFixture(t, "main", "release/2.0")
	seqBefore := m.branchSeq

	m = press(t, m, "esc")
	if m.branchSeq == seqBefore {
		t.Fatal("closing the popup should invalidate the in-flight listing (raise the sequence)")
	}
	if m.retarget.state != retargetClosed {
		t.Fatalf("state = %v, want cerrado", m.retarget.state)
	}

	m = send(t, m, branchesMsg{seq: seqBefore, names: []string{"otra/cosa"}})
	if m.retarget.state != retargetClosed {
		t.Errorf("a listing with the sequence from before the close reopened the popup: state = %v", m.retarget.state)
	}
	if len(m.retarget.view) != 0 {
		t.Errorf("a stale listing filled the view: %q", m.retarget.view)
	}
	m = send(t, m, branchesMsg{seq: seqBefore, errMsg: "gh: Not Found"})
	if stripANSI(m.retargetOverlay2()) != "" {
		t.Errorf("a stale error painted a notice: %q", stripANSI(m.retargetOverlay2()))
	}

	open, _ := retargetFixture(t, "main", "release/2.0")
	open = send(t, open, branchesMsg{seq: open.branchSeq, names: []string{"main", "otra"}})
	if open.retarget.state != retargetChoosing {
		t.Errorf("with the current sequence and the popup open it should search: %v", open.retarget.state)
	}
	if len(open.retarget.view) != 2 {
		t.Errorf("view = %q, want the two branches", open.retarget.view)
	}
	open = press(t, open, "esc")
	open = send(t, open, branchesMsg{seq: open.branchSeq, names: []string{"new"}})
	if open.retarget.state != retargetClosed || len(open.retarget.view) != 0 {
		t.Errorf("a listing with the current sequence reopened a closed popup: %v, %q",
			open.retarget.state, open.retarget.view)
	}
}

// The filter clears rune by rune, not by grapheme.
func TestBackspaceDeletesOneRuneAndNoMore(t *testing.T) {
	m, _ := retargetFixture(t, "main", "release/2.0")
	m = pressFilter(t, m, "fi")

	m = press(t, m, "backspace")
	if m.retarget.query != "f" {
		t.Errorf("query = %q after a backspace, want %q", m.retarget.query, "f")
	}
	m = pressFilter(t, m, "é")
	m = press(t, m, "backspace")
	if m.retarget.query != "f" {
		t.Errorf("query = %q after deleting a multibyte rune, want %q", m.retarget.query, "f")
	}
	m = press(t, m, "backspace")
	if m.retarget.query != "" {
		t.Errorf("query = %q, want empty", m.retarget.query)
	}
	viewBefore := append([]string(nil), m.retarget.view...)
	cursorBefore := m.retarget.cursor
	allBefore := append([]string(nil), m.retarget.all...)
	m = press(t, m, "backspace")
	if m.retarget.query != "" {
		t.Errorf("backspace with an empty filter typed %q", m.retarget.query)
	}
	if m.retarget.state != retargetChoosing {
		t.Errorf("backspace with an empty filter closed the popup: %v", m.retarget.state)
	}
	if len(m.retarget.view) != len(viewBefore) {
		t.Errorf("backspace with an empty filter changed the view: %q -> %q", viewBefore, m.retarget.view)
	}
	if len(m.retarget.all) != len(allBefore) {
		t.Errorf("backspace with an empty filter changed the listing: %q -> %q", allBefore, m.retarget.all)
	}
	if got := m.retarget.view; len(got) != len(m.retarget.all) {
		t.Errorf("with an empty filter the view is %d of %d branches", len(got), len(m.retarget.all))
	}
	_ = cursorBefore
}

func TestApplyQueryReturnsToTheStartAndAlreadyCutsTheView(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m.retarget.all = []string{"main", "fix/one", "fix/two", "wip"}
	m.retarget.view = append([]string(nil), m.retarget.all...)
	m.retarget.cursor = 3
	m.retarget.win = 2

	m.retarget.query = "fix"
	m.applyQuery()

	if m.retarget.cursor != 0 {
		t.Errorf("cursor = %d after filtering, want 0: enter would apply a row that moved", m.retarget.cursor)
	}
	if m.retarget.win != 0 {
		t.Errorf("win = %d after filtering, want 0: the window reframes with the new view", m.retarget.win)
	}
	if got := m.retarget.view; len(got) != 2 || got[0] != "fix/one" {
		t.Errorf("view = %q, want the two branches of fix", got)
	}
	// The FULL listing is what clearing the filter restores, so it must not be trimmed in place.
	if len(m.retarget.all) != 4 {
		t.Errorf("the listing was clipped when filtering: %q", m.retarget.all)
	}
	m.retarget.query = "noexiste"
	m.applyQuery()
	if len(m.retarget.view) != 0 {
		t.Errorf("a filter with no matches gave view %q", m.retarget.view)
	}
	if m.retarget.cursor != 0 {
		t.Errorf("cursor = %d with an empty view, want 0", m.retarget.cursor)
	}
}

// Forge, host and project: without forge and host the same owner/repo in two forges collides.
func TestKeyOfSeparatesForgeAndHost(t *testing.T) {
	mk := func(forge, host, project string) model.Item {
		return model.NewItem(model.RepoRef{Forge: forge, Host: host, Project: project}, 1)
	}
	gh := keyOf(mk("github", "github.com", "acme/widget"))
	gl := keyOf(mk("gitlab", "gitlab.example.com", "acme/widget"))
	self := keyOf(mk("github", "github.enterprise.corp", "acme/widget"))

	if gh == gl || gh == self || gl == self {
		t.Errorf("forge and host have to enter the key: %+v %+v %+v", gh, gl, self)
	}
	if gh != keyOf(mk("github", "github.com", "acme/widget")) {
		t.Error("the same repo on the same forge gave two different keys")
	}
	if keyOf(mk("github", "github.com", "acme/widget")) == keyOf(mk("github", "github.com", "other/widget")) {
		t.Error("two different projects gave the same key")
	}
}
