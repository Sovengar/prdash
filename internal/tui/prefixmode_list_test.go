package tui

import (
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

func listModelWithItems(t *testing.T, projects ...string) Model {
	t.Helper()
	m := newTestModel(t, ghAdapter())
	return send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested,
		mkItems(projects...), false))
}

func listOf(m Model) []string {
	lines := make([]string, 0, 8)
	for _, l := range m.listLines(m.contentWidth()) {
		lines = append(lines, stripANSI(l.text))
	}
	return lines
}

// The prefix line per mode: in common there is one, in the other two there is not.
func TestListLinesPaintThePrefixOnlyInCommon(t *testing.T) {
	for _, mode := range []prefixMode{prefixFull, prefixLeaf} {
		m := listModelWithItems(t, "APPCITTI/vsocial/backend/api-gateway", "APPCITTI/vsocial/backend/web-app")
		m.prefixMode = mode
		for _, line := range listOf(m) {
			if strings.Contains(line, "APPCITTI/vsocial/backend/") {
				t.Errorf("in %v a prefix line was painted: %q", mode, line)
			}
		}
	}

	m := listModelWithItems(t, "APPCITTI/vsocial/backend/api-gateway", "APPCITTI/vsocial/backend/web-app")
	joined := strings.Join(listOf(m), "\n")
	if !strings.Contains(joined, "APPCITTI/vsocial/backend/") {
		t.Errorf("in common the prefix line is missing:\n%s", joined)
	}
	if got := strings.Count(joined, "APPCITTI/vsocial/backend/"); got != 1 {
		t.Errorf("the prefix appears %d times, want 1:\n%s", got, joined)
	}
}

func TestFullAndLeafPaintTheWholeReferenceOrTheLeaf(t *testing.T) {
	m := listModelWithItems(t, "APPCITTI/vsocial/backend/api-gateway", "APPCITTI/vsocial/backend/web-app")

	m.prefixMode = prefixFull
	full := strings.Join(listOf(m), "\n")
	if !strings.Contains(full, "api-gateway#100") {
		t.Errorf("full did not paint the item reference:\n%s", full)
	}
	if !strings.Contains(full, "vsocial") {
		t.Errorf("full did not paint the group in the cell:\n%s", full)
	}

	m.prefixMode = prefixLeaf
	leaf := strings.Join(listOf(m), "\n")
	if !strings.Contains(leaf, "api-gateway#100") {
		t.Errorf("leaf did not paint the sheet with its number:\n%s", leaf)
	}
	if strings.Contains(leaf, "vsocial") {
		t.Errorf("leaf still shows the projects group:\n%s", leaf)
	}
}

// Removing the prefix line returns that height to the list.
func TestFullAndLeafRecoverThePrefixLine(t *testing.T) {
	m := listModelWithItems(t, "APPCITTI/vsocial/backend/api-gateway", "APPCITTI/vsocial/backend/web-app")

	m.prefixMode = prefixCommon
	withPrefix := len(m.listLines(m.contentWidth()))
	for _, mode := range []prefixMode{prefixFull, prefixLeaf} {
		m.prefixMode = mode
		withoutPrefix := len(m.listLines(m.contentWidth()))
		if withoutPrefix != withPrefix-1 {
			t.Errorf("in %v the list has %d lines, want %d (one less than in common, %d)",
				mode, withoutPrefix, withPrefix-1, withPrefix)
		}
	}
}

func TestCommonWithNoCommonPrefixLooksLikeFull(t *testing.T) {
	m := listModelWithItems(t, "acme/one", "other/one")

	m.prefixMode = prefixCommon
	common := strings.Join(listOf(m), "\n")
	m.prefixMode = prefixFull
	full := strings.Join(listOf(m), "\n")

	if common != full {
		t.Errorf("without a common prefix, common and full should look the same:\ncommon:\n%s\nfull:\n%s", common, full)
	}
	if !strings.Contains(common, "acme/one#100") {
		t.Errorf("the cell did not paint the full path:\n%s", common)
	}
	if got := strings.Count(common, "acme"); got != 1 {
		t.Errorf("the group appears %d times, want 1:\n%s", got, common)
	}
}

// Degrading to common must not repeat the path in two places.
func TestCommonWithNoCommonPrefixDoesNotRepeatThePathTwice(t *testing.T) {
	m := listModelWithItems(t, "acme/one", "other/one")
	m.prefixMode = prefixCommon
	for _, line := range listOf(m) {
		if strings.TrimSpace(line) == "·" || strings.HasPrefix(strings.TrimSpace(line), "· /") {
			t.Errorf("a prefix line was painted without content: %q", line)
		}
	}
}
