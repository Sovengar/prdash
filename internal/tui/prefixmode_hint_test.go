package tui

import (
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

func TestTheHintNamesTheCurrentMode(t *testing.T) {
	m := listModelWithItems(t, "APPCITTI/vsocial/backend/api-gateway")

	for _, want := range []struct {
		key  string
		hint string
	}{
		{"", "p prefix: common"}, // the initial state, without pressing anything
		{"p", "p prefix: full"},
		{"p", "p prefix: leaf"},
		{"p", "p prefix: common"},
	} {
		if want.key != "" {
			m = press(t, m, want.key)
		}
		bar := stripANSI(strings.Join(m.hintLines(), " "))
		if !strings.Contains(bar, want.hint) {
			t.Errorf("the bar = %q, want it to contain %q", bar, want.hint)
		}
		if strings.Contains(bar, "p prefix") && !strings.Contains(bar, want.hint) {
			t.Errorf("the bar shows the label without the mode: %q", bar)
		}
	}
}

// The same promise at the widths where the bar actually fits.
func TestTheHintNamesTheModeAtUsableWidths(t *testing.T) {
	for _, width := range []int{44, 56, 64, 80, 100, 160} {
		m := newTestModel(t, ghAdapter())
		m.width = width
		m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested,
			mkItems("APPCITTI/vsocial/backend/api-gateway"), false))

		for _, want := range []string{"p prefix: common", "p prefix: full", "p prefix: leaf"} {
			bar := stripANSI(strings.Join(m.hintLines(), " "))
			if !strings.Contains(bar, want) {
				t.Errorf("width %d: the bar = %q, want it to contain %q", width, bar, want)
				break
			}
			m = press(t, m, "p")
		}
	}
}

func TestTheHintFollowsTheModeRebind(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m.cfg.Keybindings["prefix-mode"] = "P"
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested,
		mkItems("acme/widget"), false))
	m = press(t, m, "P")
	m = press(t, m, "P")

	bar := stripANSI(strings.Join(m.hintLines(), " "))
	if !strings.Contains(bar, "P prefix: leaf") {
		t.Errorf("the bar = %q, want it to contain %q", bar, "P prefix: leaf")
	}
	if strings.Contains(bar, "p prefix") {
		t.Errorf("the bar still shows the previous key: %q", bar)
	}
}

// With the merge armed the hints box stops being help.
func TestTheHintDoesNotQueueInTheMergeConfirmation(t *testing.T) {
	// The merge fixture is what registers the items in the adapter: without it the guard would cut.
	f := newMergeFixture(t, mergeItems()...)
	m := f.m
	m = press(t, m, "p")
	m = press(t, m, "p") // leaf
	m = press(t, m, "m") // arms the merge: the box becomes the Confirmation
	if !m.mergeArmed {
		t.Fatal("the merge did not arm: the test would prove nothing")
	}

	bar := stripANSI(strings.Join(m.hintLines(), " "))
	if strings.Contains(bar, "prefix") {
		t.Errorf("the merge Confirmation shows the prefix mode: %q", bar)
	}
	if !strings.Contains(bar, "merge") {
		t.Errorf("the box does not show the Confirmation: %q", bar)
	}
}
