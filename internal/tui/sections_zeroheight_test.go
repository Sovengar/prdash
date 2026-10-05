package tui

import (
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

// A zero height degrades the layout to layout{} (bodyLines 0) and that must not empty the list:
// "no reserved height" is not "no content", the whole list is painted instead of a clipped slice.
func TestZeroBodyHeightPaintsWholeList(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, numberedItems(4, "mine/repo"), false))
	m.height = 0

	lay := m.layout()
	if lay.bodyLines != 0 {
		t.Fatalf("height 0 must degrade to bodyLines 0, got %d", lay.bodyLines)
	}
	all := m.listLines(m.contentWidth())
	if len(all) == 0 {
		t.Fatal("the section needs content to prove the point")
	}

	got := stripANSI(m.listSection(lay).text)
	for _, l := range all {
		want := stripANSI(l.text)
		if !strings.Contains(got, want) {
			t.Errorf("with bodyLines 0 the list must paint whole, missing %q:\n%s", want, got)
		}
	}
}
