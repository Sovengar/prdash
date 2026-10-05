package tui

import (
	"strings"
	"testing"
	"time"

	"prdash/internal/forge"
	"prdash/internal/testutil"
)

// Complementary: Valid says it cannot be done, Label returns the name as given.
func TestAMergeWithAnUnknownModeIsNotPresentedAsAKnownMode(t *testing.T) {
	for _, c := range []struct {
		mode forge.MergeMode
		want string
	}{
		{forge.MergeCommit, "merge commit"},
		{forge.Rebase, "rebase"},
		{forge.Squash, "squash"},
		{forge.MergeMode("fast-forward"), "fast-forward"},
		{forge.MergeMode(""), ""},
	} {
		if got := c.mode.Label(); got != c.want {
			t.Errorf("%q.Label() = %q, want %q", string(c.mode), got, c.want)
		}
	}

	// Valid separates what is known from what is not, which is what decides whether to warn first.
	for _, c := range []struct {
		mode forge.MergeMode
		want bool
	}{
		{forge.MergeCommit, true},
		{forge.Rebase, true},
		{forge.Squash, true},
		{forge.MergeMode("fast-forward"), false},
		{forge.MergeMode(""), false},
		{forge.MergeMode("MERGE"), false}, // uppercase is another mode, not this one
	} {
		if got := c.mode.Valid(); got != c.want {
			t.Errorf("%q.Valid() = %v, want %v", string(c.mode), got, c.want)
		}
	}

	// And the confirmation's key, the third place an unknown mode has to be handled.
	for _, c := range []struct {
		mode forge.MergeMode
		want string
	}{
		{forge.MergeCommit, "m"},
		{forge.Rebase, "r"},
		{forge.Squash, "s"},
		{forge.MergeMode("other"), "?"},
	} {
		if got := modeKey(c.mode); got != c.want {
			t.Errorf("key of %q = %q, want %q", string(c.mode), got, c.want)
		}
	}
}

// Composing on the current warning instead of stacking a second copy of the same text.
func TestReplaceSwapsTheLiveNoticeAndDoesNotDuplicateTheText(t *testing.T) {
	tm := &toastManager{now: func() time.Time { return time.Unix(0, 0) }}

	tm.show("consultando branches", toastInfo)
	tm.replace("consultando branches", "consultando branches…", toastInfo)
	if len(tm.toasts) != 1 {
		t.Fatalf("%d notices stay after replacing, want 1: the new text stacked instead "+
			"of replacing the old one and two notices of the same origin are visible", len(tm.toasts))
	}
	if tm.toasts[0].message != "consultando branches…" {
		t.Errorf("the text ended up as %q", tm.toasts[0].message)
	}

	// And with a `prev` that is not there: it stacks, it is not lost. A lost warning is the worst
	// outcome.
	tm.replace("a text that no longer exists", "another notice", toastWarning)
	if len(tm.toasts) != 2 {
		t.Errorf("a missing `prev` left %d notices, want 2: the new one stacks", len(tm.toasts))
	}

	// And an EMPTY message does not stack: an empty warning is indistinguishable from not warning.
	before := len(tm.toasts)
	tm.replace("consultando branches…", "", toastInfo)
	if len(tm.toasts) != before {
		t.Errorf("an empty message changed the number of notices: %d -> %d", before, len(tm.toasts))
	}
}

func TestTheHeaderShowsTheSpinnerFirstAndTheForgeStatusBehind(t *testing.T) {
	new := func(t *testing.T) Model {
		t.Helper()
		m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
		m.width, m.height = 120, 40
		return m
	}

	loading := new(t)
	loading.loading = true
	text := loading.headerSection().text
	if !strings.Contains(text, "refreshing") {
		t.Errorf("loading, the header does not say it is refreshing:\n%s", text)
	}
	if !strings.Contains(text, "PRDash") {
		t.Errorf("the header lost its title:\n%s", text)
	}
	spinnerIndex := strings.Index(text, "refreshing")
	statusIndex := len(text)
	for _, marca := range []string{"github", "not authenticated", "no auth"} {
		if i := strings.Index(text, marca); i >= 0 && i < statusIndex {
			statusIndex = i
		}
	}
	if statusIndex < spinnerIndex {
		t.Errorf("the forge status goes before the spinner: %q", text)
	}

	quieto := new(t)
	quieto.loading = false
	if strings.Contains(quieto.headerSection().text, "refreshing") {
		t.Errorf("with no load the header still says it is refreshing:\n%s",
			quieto.headerSection().text)
	}

	// And the narrow terminal, which is the reason for the order: the spinner survives and the forge
	// statuses go.
	narrow := new(t)
	narrow.loading = true
	narrow.width = 40
	if !strings.Contains(narrow.headerSection().text, "refreshing") {
		t.Errorf("with 40 columns the spinner disappears: %q", narrow.headerSection().text)
	}
}

// The negative case of compactCount.
func TestANegativeCountIsNotPresentedAsARoundedOne(t *testing.T) {
	for _, c := range []struct {
		n    int
		want string
	}{
		{0, "0"},
		{1, "1"},
		{999, "999"},
		{1000, "1.0k"},
		{1050, "1.1k"},
		{1500, "1.5k"},
		// 9999 rounds UP to "10k", not down to "9.9k", and that comes out as five runes.
		{9999, "10k"},
		{10000, "10k"},
		{999999, "999k"},
		{1000000, "1000k"},
		{-1, "-1"},
		{-1500, "-1500"},
	} {
		if got := compactCount(c.n); got != c.want {
			t.Errorf("compactCount(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}

// diffSpans only colours text that really is two signed digits.
func TestADiffstatThatIsNotTwoSignedDigitsIsNotPainted(t *testing.T) {
	for _, c := range []struct {
		name   string
		plain  string
		paints bool
	}{
		{"both with sign", "+12 -3", true},
		// The queue goes AFTER the deleted count, which is why "+12 files -3" does not read as a net.
		{"units at the end", "+12 -3 files", true},
		{"units in the middle", "+12 files -3", false},
		{"without the separator", "+12", false},
		{"without signs", "12 3", false},
		{"empty", "", false},
		{"text that is not a count", "no changes", false},
		{"a single sign", "+12 -", false},
		{"signo suelto", "- 3", false},
		{"empty tail", "+12 -3   ", true},
	} {
		spans := diffSpans(c.plain)
		paints := len(spans) > 0
		if paints != c.paints {
			t.Errorf("%s: paints=%v, want %v (spans=%v)", c.name, paints, c.paints, spans)
		}
		for _, s := range spans {
			if s.text == "" {
				t.Errorf("%s: there is an empty span in %v", c.name, spans)
			}
		}
	}
}
