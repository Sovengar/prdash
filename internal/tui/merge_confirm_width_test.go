package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestMergeConfirmKeepsTheDeleteValueWhenItWraps(t *testing.T) {
	for _, width := range []int{40, 50, 60, 76, 100, 160} {
		f := newMergeFixture(t, mergeItems()...)
		m := send(t, f.m, tea.WindowSizeMsg{Width: width, Height: 40})
		m = press(t, m, "m")

		view := stripANSI(m.View().Content)
		if !strings.Contains(view, "yes (tab)") {
			t.Errorf("width %d: the delete value is not fully readable\n%s", width, view)
		}
	}
}

// The toggle has to show the NEW value, not the old one.
func TestMergeConfirmKeepsBothDeleteValuesVisible(t *testing.T) {
	for _, width := range []int{50, 76, 120} {
		f := newMergeFixture(t, mergeItems()...)
		m := send(t, f.m, tea.WindowSizeMsg{Width: width, Height: 40})
		m = press(t, m, "m")
		m = press(t, m, "tab")

		view := stripANSI(m.View().Content)
		if !strings.Contains(view, "no (tab)") {
			t.Errorf("width %d: the toggled value does not appear\n%s", width, view)
		}
		if strings.Contains(view, "yes (tab)") {
			t.Errorf("width %d: it still shows the previous value\n%s", width, view)
		}
	}
}
