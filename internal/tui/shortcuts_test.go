package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

func TestAStreamWithNoChangesIsNotRepaginatedWholeAndSaysSo(t *testing.T) {
	for _, c := range []struct {
		name         string
		prev         streamHead
		page         forge.Page
		wantShortcut bool
	}{
		{
			name:         "full cycle and same cursor",
			prev:         streamHead{cursor: "CUR2", complete: true},
			page:         forge.Page{Next: "CUR2", More: true},
			wantShortcut: true,
		},
		{
			name:         "the previous cycle was left half done",
			prev:         streamHead{cursor: "CUR2", complete: false},
			page:         forge.Page{Next: "CUR2", More: true},
			wantShortcut: false,
		},
		{
			name:         "there is nothing left to see",
			prev:         streamHead{cursor: "CUR2", complete: true},
			page:         forge.Page{Next: "", More: false},
			wantShortcut: false,
		},
		{
			name:         "the cursor has changed",
			prev:         streamHead{cursor: "CUR1", complete: true},
			page:         forge.Page{Next: "CUR2", More: true},
			wantShortcut: false,
		},
	} {
		if got := unchangedHead(c.prev, c.page); got != c.wantShortcut {
			t.Errorf("%s: unchangedHead = %v, want %v", c.name, got, c.wantShortcut)
		}
	}

	pages := map[testutil.FakeKey][]forge.Page{}
	prev := map[streamKey]streamHead{}
	for _, q := range forge.Streams {
		pages[testutil.FakeKey{Section: q.Section, Kind: q.ReviewKind}] = []forge.Page{
			{Next: "CUR2", More: true},
			{Items: []model.Item{mkItem("github", "github.com", "acme/widget", "one", 7, "")}},
		}
		prev[streamKey{forge: "github", section: q.Section, kind: q.ReviewKind}] =
			streamHead{cursor: "CUR2", complete: true}
	}
	adapter := &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com", Pages: pages}
	events := make(chan event, 32)
	streamForge(context.Background(), context.Background(), events, adapter, 3, prev)

	var vistos []pageMsg
	for len(events) > 0 {
		e := <-events
		if p, ok := e.(pageMsg); ok {
			vistos = append(vistos, p)
		}
	}

	if len(vistos) != len(forge.Streams) {
		t.Fatalf("%d pages came out with the shortcut, want %d (one per stream): either "+
			"it paginated fully or some stream did not emit", len(vistos), len(forge.Streams))
	}
	for _, p := range vistos {
		if !p.unchanged {
			t.Errorf("a page came out without the `unchanged` mark: %+v", p)
		}
		if len(p.items) != 0 {
			t.Errorf("a page marked `unchanged` carries %d items: it would replace the "+
				"cache with a partial list", len(p.items))
		}
		if p.cycle != 3 {
			t.Errorf("the page carries cycle %d, want 3", p.cycle)
		}
	}
}

// This happens for real: an item arrives from a forge that was disabled.
func TestAnItemOfAnUnknownForgeDoesNotQueryAndDoesNotBreakTheCommentChain(t *testing.T) {
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "gitlab", HostName: "gitlab.example.com"})
	m = withSelection(t, m, mkItem("github", "github.com", "acme/widget", "one", 7, ""))

	cmd := m.requestComments()
	if cmd == nil {
		t.Fatal("the comments chain is short with an unknown forge: from here on " +
			"no selection change would fetch the conversation again")
	}
	if _, exists := m.comments[mkItem("github", "github.com", "acme/widget", "one", 7, "").ID()]; exists {
		t.Error("the item was marked as fetched without fetching it: the next tick would take " +
			"it as loaded and never ask for the conversation again")
	}
}

// The half that was missing: with an empty URL the guard warns, and this is the other half.
func TestOpeningTheBrowserWithURLReturnsACommandNotAFailureNotice(t *testing.T) {
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	it := mkItem("github", "github.com", "acme/widget", "one", 7, "")
	it.URL = "https://github.com/acme/widget/pull/7"
	m = withSelection(t, m, it)

	_, cmd := pressWithCmd(t, m, "o")
	if cmd == nil {
		t.Fatal("opening a valid URL returned no command: the popup would close as if " +
			"the browser had failed")
	}
	if av := lastToast(m); strings.Contains(av, "opening") {
		t.Errorf("there is a notice %q before even trying to open: it promises something that can fail", av)
	}
	m2 := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	m2 = withSelection(t, m2, it)
	var open string
	m2.openURL = func(u string) error { open = u; return nil }
	_, cmd2 := pressWithCmd(t, m2, "o")
	if cmd2 == nil {
		t.Fatal("opening with the seam returned no command")
	}
	if msg, ok := cmd2().(notifyMsg); !ok || !strings.Contains(msg.text, it.URL) {
		t.Errorf("the open command does not mention the items URL: %+v", msg)
	}
	if open != it.URL {
		t.Errorf("the opener received %q, want %q", open, it.URL)
	}
}

var _ = time.Second

// A tea.Cmd checked only with "not nil" checks nothing: the content of the message is the whole point.
func TestTheTickChainReturnsItsMessageAndNotJustANonNil(t *testing.T) {
	m := newTestModel(t)
	cmd := m.commentsCmd()
	if cmd == nil {
		t.Fatal("commentsCmd returned nil: the conversation chain is short")
	}
	if msg, ok := cmd().(commentsTickMsg); !ok {
		t.Errorf("commentsCmd returned %T, want commentsTickMsg", msg)
	}

	m2 := newTestModel(t)
	m2.cfg.RefreshInterval = time.Millisecond
	tick := m2.tickCmd()
	if tick == nil {
		t.Fatal("tickCmd returned nil with a valid interval")
	}
	if msg, ok := tick().(tickMsg); !ok {
		t.Errorf("tickCmd returned %T, want tickMsg", msg)
	}
}

func TestTheUpArrowAlsoMovesTheBranchSelectorsCursor(t *testing.T) {
	m := modelInRetarget(t, retargetChoosing)
	if before := pressModel(t, m, "down").retarget.cursor; pressModel(t, pressModel(t, m, "down"), "up").retarget.cursor == before {
		t.Error("with an empty filter, up did not undo the down movement")
	}

	m2 := modelInRetarget(t, retargetChoosing)
	m2.retarget.query = "re"
	afterK := pressModel(t, m2, "k")
	if afterK.retarget.cursor != 0 {
		t.Errorf("with a filter typed, k moved the cursor: it would type and navigate at once")
	}
	m2.retarget.cursor = 2
	sube := pressModel(t, m2, "up")
	if sube.retarget.cursor != 1 {
		t.Errorf("with a filter typed, up left the cursor at %d, want 1: navigation cannot "+
			"depend on what has been typed", sube.retarget.cursor)
	}
	if sube.retarget.query != "re" {
		t.Errorf("up altered the filter: it is %q", sube.retarget.query)
	}
}

func TestABranchListingWithWarningsPastesThemIntoTheMessageInsteadOfDroppingThem(t *testing.T) {
	ref := model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget"}
	a := &testutil.FakeAdapter{
		ForgeName:   "github",
		HostName:    "github.com",
		BranchLists: map[string][]string{ref.Project: {"main", "feat/x"}},
		BranchWarnings: map[string][]model.Warning{
			ref.Project: {{Kind: "permission", Msg: "no permission to read refs"}},
		},
	}
	m := newTestModel(t, a)
	m.retarget.state = retargetListing
	m.retarget.item = model.NewItem(ref, 7)
	m.events = make(chan event, 4)

	m.fetchBranches(m.retarget.item)

	varSaw := false
	for waiting := time.Now().Add(2 * time.Second); ; {
		select {
		case e := <-m.events:
			msg, ok := e.(branchesMsg)
			if !ok {
				continue
			}
			varSaw = true
			if len(msg.names) != 2 {
				t.Errorf("%d branches arrived, want 2: the warnings must not kill the listing",
					len(msg.names))
			}
			if !strings.Contains(msg.errMsg, "no permission") {
				t.Errorf("the forges warning did not reach the message: errMsg=%q", msg.errMsg)
			}
			if msg.seq != m.branchSeq {
				t.Errorf("the message carries seq=%d and the model is at %d: without that a stale "+
					"listing would be accepted", msg.seq, m.branchSeq)
			}
		default:
			if varSaw || !time.Now().Before(waiting) {
				goto comprobado
			}
			time.Sleep(2 * time.Millisecond)
		}
	}
comprobado:
	if !varSaw {
		t.Fatal("no branchesMsg arrived through the events channel")
	}
}
