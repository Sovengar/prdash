package tui

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"

	"prdash/internal/forge/model"
)

func TestReviewLabelFallsBackToTheStateWhenThereIsNoDecision(t *testing.T) {
	cases := []struct {
		name string
		it   model.Item
		want string
	}{
		{
			"no decision falls back to the state",
			model.Item{State: "OPEN", ReviewDecision: ""},
			"OPEN",
		},
		{
			"approved with no review kind",
			model.Item{State: "OPEN", ReviewDecision: "APPROVED"},
			"approved",
		},
		{
			"cambios pedidos",
			model.Item{State: "OPEN", ReviewDecision: "CHANGES_REQUESTED"},
			"changes requested",
		},
		{
			"review pendiente",
			model.Item{State: "OPEN", ReviewDecision: "REVIEW_REQUIRED"},
			"review required",
		},
		{
			"review solicitado",
			model.Item{State: "OPEN", ReviewDecision: "APPROVED", ReviewKind: model.ReviewRequested},
			"approved · review requested",
		},
		{
			"assigned",
			model.Item{State: "OPEN", ReviewDecision: "APPROVED", ReviewKind: model.ReviewAssigned},
			"approved · assigned",
		},
		{
			"unknown decision as is",
			model.Item{State: "OPEN", ReviewDecision: "DISMISSED"},
			"DISMISSED",
		},
		{
			// With neither decision nor state the label is empty, which is the honest datum.
			"nothing to say",
			model.Item{},
			"",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := reviewLabel(c.it); got != c.want {
				t.Errorf("reviewLabel() = %q, want %q", got, c.want)
			}
		})
	}
}

// "no checks" and "passing (0)" are different claims.
func TestChecksDetailSeparatesTheUnknownFromTheGreen(t *testing.T) {
	cases := []struct {
		name   string
		checks model.Checks
		want   string
	}{
		{
			"no data: unknown",
			model.Checks{State: model.ChecksUnknown},
			"no checks",
		},
		{
			"zero total but with a known state: there is a count",
			model.Checks{State: model.ChecksPassing},
			"passing (0)",
		},
		{
			"all green",
			model.Checks{Total: 3, State: model.ChecksPassing},
			"passing (3)",
		},
		{
			"with failing",
			model.Checks{Total: 5, Failing: 2, State: model.ChecksFailing},
			"failing (2/5 failing, 0 pending)",
		},
		{
			"with pending",
			model.Checks{Total: 4, Pending: 1, State: model.ChecksPending},
			"pending (0/4 failing, 1 pending)",
		},
		{
			"with both",
			model.Checks{Total: 6, Failing: 1, Pending: 2, State: model.ChecksFailing},
			"failing (1/6 failing, 2 pending)",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := checksDetail(c.checks); got != c.want {
				t.Errorf("checksDetail(%+v) = %q, want %q", c.checks, got, c.want)
			}
		})
	}
}

func TestDiffDetailDistinguishesUnknownFromNoChanges(t *testing.T) {
	cases := []struct {
		name string
		d    model.DiffStat
		want string
	}{
		{"unknown", model.DiffStat{}, "unknown (forge did not report it)"},
		{"known with no changes", model.DiffStat{Known: true}, "no changes"},
		{"one file", model.DiffStat{Known: true, Additions: 1, Deletions: 0, Files: 1}, "+1 -0 (1 file)"},
		{"several", model.DiffStat{Known: true, Additions: 40, Deletions: 2, Files: 3}, "+40 -2 (3 files)"},
		{"only deletions", model.DiffStat{Known: true, Deletions: 5, Files: 1}, "+0 -5 (1 file)"},
		{"no files but with changes", model.DiffStat{Known: true, Additions: 3, Deletions: 1}, "+3 -1 (0 files)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := diffDetail(c.d); got != c.want {
				t.Errorf("diffDetail(%+v) = %q, want %q", c.d, got, c.want)
			}
		})
	}
}

// An unauthenticated forge and an unimplemented one are not the same repair.
func TestAuthReasonDoesNotConfuseTheTwoThingsFixedDifferently(t *testing.T) {
	if got := authReason(model.AuthState{}); got != "not authenticated" {
		t.Errorf("with no reason = %q, want \"not authenticated\"", got)
	}
	for _, reason := range []string{
		"GH_TOKEN is not set",
		"not implemented: Bitbucket has no approvals",
		"token expirado",
	} {
		if got := authReason(model.AuthState{Reason: reason}); got != reason {
			t.Errorf("authReason(%q) = %q, want the adapters reason", reason, got)
		}
	}
}

func TestDetailWarningsWarnByForgeAndByActionReason(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	it := mkItem("github", "github.com", "acme/widget", "One", 1, "")

	if got := m.detailWarnings(it); len(got) != 0 {
		t.Errorf("nothing to warn about: %v", got)
	}

	m.statuses["github"].auth = model.AuthState{Forge: "github", OK: false, Reason: "GH_TOKEN is not valid"}
	got := m.detailWarnings(it)
	if len(got) != 2 {
		t.Fatalf("notice = %q, want 2 (separator + notice)", got)
	}
	if got[0] != "" {
		t.Errorf("the first line of the notice should be an empty separator, gave %q", got[0])
	}
	if !strings.Contains(stripANSI(got[1]), "GH_TOKEN is not valid") || !strings.Contains(stripANSI(got[1]), "github") {
		t.Errorf("the notice should name the forge and the reason: %q", stripANSI(got[1]))
	}

	m.denied[it.ID()] = "the branch conflicts with main"
	got = m.detailWarnings(it)
	if len(got) != 2 {
		t.Fatalf("notice = %q, want 2 (the action rules)", got)
	}
	if s := stripANSI(got[1]); !strings.Contains(s, "the branch conflicts with main") {
		t.Errorf("the notice should be the actions reason: %q", s)
	}
	if s := stripANSI(got[1]); strings.Contains(s, "GH_TOKEN is not valid") {
		t.Errorf("with an action reason the forges one should not come out: %q", s)
	}
}

// The cut has to show the END.
func TestClipTopClipsFromTheTopAndLeavesNoGaps(t *testing.T) {
	lines := []string{"one", "two", "three", "four"}

	if got := clipTop(lines, 4); len(got) != 4 {
		t.Errorf("with 4 rows for 4 lines = %d, want 4", len(got))
	}
	if got := clipTop(lines, 10); len(got) != 4 {
		t.Errorf("with spare room = %d, want 4 (it does not pad)", len(got))
	}
	if got := clipTop(lines, 2); len(got) != 2 {
		t.Fatalf("with 2 rows for 4 lines = %d, want 2", len(got))
	}
	// With no rows there is no clipping possible, so the entry comes back whole: it is a
	// passthrough.
	for _, rows := range []int{0, -1} {
		if got := clipTop(lines, rows); len(got) != len(lines) {
			t.Errorf("rows=%d returned %d lines, want the %d intact (no clipping possible)", rows, len(got), len(lines))
		}
	}
	if got := clipTop(nil, 3); len(got) != 0 {
		t.Errorf("with no lines it returned %d", len(got))
	}
	if got := clipTop([]string{"x"}, 5); len(got) != 1 {
		t.Errorf("one line with 5 rows = %d, want 1", len(got))
	}
}

// The detail is a two-column card and its geometry is the whole point.
func TestTheDetailFieldsAreNeitherSteppedOnNorEaten(t *testing.T) {
	long := detailField{key: "checks", value: strings.Repeat("x", 200)}

	for _, inner := range []int{30, 40, 60, 80, 120} {
		cell := max(24, (inner-detailGap)/2)
		line, w := detailCell(long, cell)
		if w > cell {
			t.Errorf("inner %d (cell %d): the field measures %d columns, want <= %d: %q",
				inner, cell, w, cell, ansi.Strip(line))
		}
		want := labelWidth + utf8.RuneCountInString(stripANSI(line)) - labelWidth
		if w != want {
			t.Errorf("inner %d: the returned width is %d but the value measures %d: the padding would be computed wrong",
				inner, w, want)
		}
	}

	// The cell never drops below 24 however narrow the interior, because below that the label does
	// not fit.
	for _, inner := range []int{10, 20, 28, 30} {
		if cell := max(24, (inner-detailGap)/2); cell != 24 {
			t.Errorf("inner %d gave a cell of %d columns, want the minimum of 24", inner, cell)
		}
	}

	// A grid row never exceeds the interior, which is what keeps the two columns apart.
	for _, inner := range []int{10, 28, 30, 40} {
		rows := detailGrid([]detailField{long, long}, inner)
		if len(rows) != 1 {
			t.Fatalf("inner %d: %d rows, want 1", inner, len(rows))
		}
		want := 2*24 + detailGap
		if w := ansi.StringWidth(rows[0]); w != want {
			t.Errorf("inner %d: the row measures %d columns, want %d (two cells of 24 plus the gap of %d): %q",
				inner, w, want, detailGap, ansi.Strip(rows[0]))
		}
	}
	for _, inner := range []int{60, 80, 120, 200} {
		for _, line := range detailGrid([]detailField{long, long, long}, inner) {
			if w := ansi.StringWidth(line); w > inner {
				t.Errorf("inner %d: the row measures %d columns, want <= %d: %q", inner, w, inner, ansi.Strip(line))
			}
		}
	}

	for _, inner := range []int{20, 38, 60, 160} {
		line := fullWidthField(detailField{key: "url", value: "https://gitlab.example.com/grp/proj/-/merge_requests/1"}, inner)
		if w := ansi.StringWidth(line); w > inner {
			t.Errorf("inner %d: the full width field measures %d columns, want <= %d: %q",
				inner, w, inner, ansi.Strip(line))
		}
	}
	// And with a 1-column interior it does not panic: the max(1, ...) prevents a negative.
	for _, inner := range []int{0, 1, 2} {
		line := fullWidthField(detailField{key: "u", value: "value"}, inner)
		if w := ansi.StringWidth(line); w != labelWidth+1 {
			t.Errorf("inner %d: the minimum field measures %d columns, want %d (label + 1)",
				inner, w, labelWidth+1)
		}
	}
}

// The age decides whether an item looks freshly touched or abandoned.
func TestRelTimeDoesNotInventANegativeNumber(t *testing.T) {
	for _, tc := range []struct {
		ago  time.Duration
		want string
	}{
		{0, "now"},
		{30 * time.Second, "now"},
		{59 * time.Second, "now"},
		{61 * time.Second, "1m"},
		{59 * time.Minute, "59m"},
		{61 * time.Minute, "1h"},
		{23 * time.Hour, "23h"},
		{25 * time.Hour, "1d"},
		{6 * 24 * time.Hour, "6d"},
		{30 * 24 * time.Hour, "30d"},
	} {
		got := relativeTime(time.Now().Add(-tc.ago))
		if got != tc.want {
			t.Errorf("relativeTime(ago %v) = %q, want %q", tc.ago, got, tc.want)
		}
	}
	// A date in the future (a skewed clock or a bad fixture) cannot give a negative age.
	if got := relativeTime(time.Now().Add(24 * time.Hour)); strings.HasPrefix(got, "-") {
		t.Errorf("a future date gave %q, which reads as a negative number", got)
	}
	// And the zero date is a dash, not a "-20000d": the forge did not send it.
	if got := relativeTime(time.Time{}); got != "-" {
		t.Errorf("a date with no data gave %q, want \"-\"", got)
	}
}

func TestOrDashLeavesNoFieldEmpty(t *testing.T) {
	if got := orDash(""); got != "-" {
		t.Errorf("orDash(\"\") = %q, want \"-\"", got)
	}
	if got := orDash("value"); got != "value" {
		t.Errorf("orDash(\"value\") = %q, want the value intact", got)
	}
	if got := orDash(" "); got != " " {
		t.Errorf("orDash(\" \") = %q, want the space intact", got)
	}
}
