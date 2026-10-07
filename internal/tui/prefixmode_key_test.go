package tui

import (
	"strconv"
	"testing"

	"prdash/internal/forge/model"
)

func TestPrefixModeCyclesWithTheKey(t *testing.T) {
	m := listModelWithItems(t, "APPCITTI/vsocial/backend/api-gateway")

	if m.prefixMode != prefixCommon {
		t.Fatalf("initial mode = %v, want common (the default value)", m.prefixMode)
	}
	for _, want := range []prefixMode{prefixFull, prefixLeaf, prefixCommon, prefixFull} {
		m = press(t, m, "p")
		if m.prefixMode != want {
			t.Fatalf("after pressing p: mode = %v, want %v", m.prefixMode, want)
		}
	}
}

func TestPrefixModeComesFromTheConfig(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m.cfg.Keybindings["prefix-mode"] = "P"
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested,
		mkItems("acme/widget"), false))

	m = press(t, m, "P")
	if m.prefixMode != prefixFull {
		t.Fatalf("with the rebind to P, the mode = %v, want full", m.prefixMode)
	}
	before := m.prefixMode
	m = press(t, m, "p")
	if m.prefixMode != before {
		t.Errorf("the old key %q still cycles the mode: %v -> %v", "p", before, m.prefixMode)
	}
}

func TestPrefixModeIsNotResetByTheSectionChange(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested,
		mkItems("APPCITTI/vsocial/backend/api-gateway"), false))
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "",
		ghItems("other/project"), false))

	m = press(t, m, "p")
	m = press(t, m, "p")
	if m.prefixMode != prefixLeaf {
		t.Fatalf("mode = %v, want leaf", m.prefixMode)
	}

	before := m.activeSection
	m = press(t, m, "tab")
	if m.activeSection == before {
		t.Fatalf("tab did not change section (still at %v): the test would prove nothing", m.activeSection)
	}
	if m.prefixMode != prefixLeaf {
		t.Errorf("changing section reset the mode: %v, want leaf (it is global)", m.prefixMode)
	}
	for range 2 {
		m = press(t, m, "tab")
	}
	if m.activeSection != before {
		t.Fatalf("it did not return to the original section (=%v, now %v): the test would prove nothing", before, m.activeSection)
	}
	if m.prefixMode != prefixLeaf {
		t.Errorf("the full turn reset the mode: %v, want leaf", m.prefixMode)
	}
}

func TestPrefixModeFiresNoOtherActions(t *testing.T) {
	m := listModelWithItems(t, "acme/widget")
	m.loading = false
	m.toast = newToastManager()

	m = press(t, m, "p")

	if m.mergeArmed {
		t.Error("`p` armed the merge; it is not a merge-mode key")
	}
	if m.mergeArmedID != (model.ID{}) {
		t.Errorf("`p` left a merge armed on %v", m.mergeArmedID)
	}
	if m.loading {
		t.Error("`p` fired a refresh")
	}
	if m.actionBusy {
		t.Error("`p` launched a forge action")
	}
	if len(m.toast.toasts) != 0 {
		t.Errorf("`p` left a notice: %v", m.toast.toasts)
	}
	if m.cursor != 0 {
		t.Errorf("`p` moved the cursor to %d, want 0", m.cursor)
	}
}

func TestPrefixModeKeepsTheCursorAndTheWindow(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	items := make([]model.Item, 0, 30)
	for i := range 30 {
		items = append(items, mkItem("github", "github.com",
			"APPCITTI/vsocial/backend/svc"+strconv.Itoa(i), "T", 100+i, ""))
	}
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, items, false))
	m.height = 12

	m = press(t, m, "end")
	before, offset := m.cursor, m.scroll
	if before == 0 || offset == 0 {
		t.Fatalf("the cursor (=%d) or the scroll (=%d) did not move: the test would prove nothing", before, offset)
	}

	m = press(t, m, "p")

	if m.cursor != before {
		t.Errorf("changing mode moved the cursor to %d, want %d", m.cursor, before)
	}
	line := cursorLine(m.listLines(m.contentWidth()), m.cursor)
	window := m.layout().bodyLines
	if line < m.scroll || line >= m.scroll+window {
		t.Errorf("the cursor row (line %d) ended outside the window [%d, %d); scroll %d -> %d",
			line, m.scroll, m.scroll+window, offset, m.scroll)
	}
}

func TestPrefixModeStartsInCommonWithoutPersisting(t *testing.T) {
	m := listModelWithItems(t, "acme/widget")
	m = press(t, m, "p")
	if m.prefixMode == prefixCommon {
		t.Fatal("`p` did not change the mode: the test would prove nothing")
	}
	if new := newTestModel(t, ghAdapter()); new.prefixMode != prefixCommon {
		t.Errorf("a new model starts at %v, want common (the mode is not persisted)", new.prefixMode)
	}
}
