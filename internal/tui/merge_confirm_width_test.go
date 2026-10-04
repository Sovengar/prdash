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
			t.Errorf("ancho %d: el valor del borrado no se lee entero\n%s", width, view)
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
			t.Errorf("ancho %d: el valor conmutado no aparece\n%s", width, view)
		}
		if strings.Contains(view, "yes (tab)") {
			t.Errorf("ancho %d: sigue enseñando el valor anterior\n%s", width, view)
		}
	}
}
