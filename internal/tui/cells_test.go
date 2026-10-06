package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"prdash/internal/forge/model"
	"prdash/internal/state"
)

// Two languages in one function, on purpose: five labels in English and two in Spanish. What is
// pinned is that neither list grows into the other.
func TestTheProblemLabelTranslatesWhatIsKnownAndWhatIsNot(t *testing.T) {
	cases := []struct {
		kind string
		want string
	}{
		{"auth", "not authenticated"},
		{"timeout", "timeout"},
		{"ratelimit", "rate limited"},
		{"parse", "unreadable response"},
		{"unsupported", "unsupported"},
		{"validation", "rejected"},
		{"network", "no connection"},
		{"some-kind", "some-kind"},
		{"", ""},
	}
	for _, c := range cases {
		got := problemLabel(c.kind)
		if got != c.want {
			t.Errorf("problemLabel(%q) gave %q, want %q", c.kind, got, c.want)
		}
		if got != strings.TrimSpace(got) {
			t.Errorf("problemLabel(%q) gave %q with edge spaces", c.kind, got)
		}
	}
	for _, kind := range []string{"auth", "timeout", "ratelimit", "parse", "unsupported", "validation", "network"} {
		if l := len(problemLabel(kind)); l > 38 {
			t.Errorf("problemLabel(%q) is %d characters, past the 38-column line budget", kind, l)
		}
	}
}

// Down, not up: a label saying 59s when a minute has passed reads as fresh data when it is not.
func TestTheAgeOfTheLastRefreshRoundsDown(t *testing.T) {
	now := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name string
		ago  time.Duration
		want string
	}{
		{"right now", time.Millisecond, "now"},
		{"59 seconds", 59 * time.Second, "59s ago"},
		{"60 seconds", 60 * time.Second, "1m ago"},
		{"90 seconds", 90 * time.Second, "1m ago"},
		{"59 minutos", 59 * time.Minute, "59m ago"},
		{"60 minutos", 60 * time.Minute, "1h ago"},
		{"25 horas", 25 * time.Hour, "25h ago"},
	}
	for _, c := range cases {
		if got := lastRefreshLabel(now.Add(-c.ago), now); got != c.want {
			t.Errorf("%s (%s): gave %q, want %q", c.name, c.ago, got, c.want)
		}
	}

	if got := lastRefreshLabel(time.Time{}, now); got != "no data" {
		t.Errorf("a zero since gave %q, want \"no data\"", got)
	}

	// The property that matters: read the label back as a duration and you get the real age within one
	// label unit.
	for _, d := range []time.Duration{
		time.Second, 30 * time.Second, 59 * time.Second,
		60 * time.Second, 90 * time.Second,
		30 * time.Minute, 59 * time.Minute, 60 * time.Minute, 90 * time.Minute,
		3 * time.Hour, 50 * time.Hour,
	} {
		got := lastRefreshLabel(now.Add(-d), now)
		parsed, err := durationOf(got)
		if err != nil {
			t.Errorf("the text %q could not be read back: %v", got, err)
			continue
		}
		// The error fits ONE label unit, not a fixed minute: at 1h30m that rounds to "1h ago" with 30
		// minutes of difference, which is rounding and not a failure.
		_, unit := numberOf(got)
		if diff := d - parsed; diff < 0 || diff >= unitDuration(unit) {
			t.Errorf("with %s the label %q stands for %s: the error does not fit one unit "+
				"of that label", d, got, parsed)
		}
	}
	var previous int
	for _, d := range []time.Duration{time.Second, 59 * time.Second, 60 * time.Second,
		59 * time.Minute, 60 * time.Minute, 3 * time.Hour} {
		_, unit := numberOf(lastRefreshLabel(now.Add(-d), now))
		if unit < previous {
			t.Errorf("with %s unit %d goes back from %d", d, unit, previous)
		}
		previous = unit
	}
}

// The CLI's text and not an invented one: an invented "could not read the branches" would hide both a
// 404 and an expired token.
func TestTheBranchNoticeUsesTheCLIsText(t *testing.T) {
	got := warnMsg([]model.Warning{{Kind: "auth", Msg: "gh: Bad credentials"}})
	if got != "gh: Bad credentials" {
		t.Errorf("warnMsg gave %q, want the untouched CLI text", got)
	}
	got = warnMsg([]model.Warning{
		{Kind: "auth", Msg: "the explanation"},
		{Kind: "network", Msg: "another one"},
	})
	if got != "the explanation" {
		t.Errorf("warnMsg gave %q, want the first one with text", got)
	}
	got = warnMsg([]model.Warning{
		{Kind: "a", Msg: "   "},
		{Kind: "b", Msg: "\t\n"},
		{Kind: "c", Msg: "the good one"},
	})
	if got != "the good one" {
		t.Errorf("warnMsg gave %q, want the one that really had text", got)
	}
	for _, warns := range [][]model.Warning{
		nil,
		{},
		{{Kind: "auth", Msg: ""}},
		{{Kind: "a", Msg: "  "}},
	} {
		if got := warnMsg(warns); strings.TrimSpace(got) == "" {
			t.Errorf("warnMsg(%v) returned empty text", warns)
		}
	}
	if got := warnMsg(nil); got != "the branches could not be read" {
		t.Errorf("with no warnings it gave %q, want the generic one", got)
	}
}

// Two forms of the same datum because of width: forgeLabel lives in the detail, where the whole path
// fits; forgeBadge in the column, where it does not.
func TestTheForgeIsIdentifiedTwoWaysAndBothFit(t *testing.T) {
	cases := []struct {
		name  string
		it    model.Item
		larga string
		short string
	}{
		{
			name:  "public github",
			it:    model.Item{Forge: "github", Host: "github.com"},
			larga: "github@github.com",
			short: "GH",
		},
		{
			name:  "public gitlab",
			it:    model.Item{Forge: "gitlab", Host: "gitlab.com"},
			larga: "gitlab@gitlab.com",
			short: "GLab",
		},
		{
			name:  "public bitbucket",
			it:    model.Item{Forge: "bitbucket", Host: "bitbucket.org"},
			larga: "bitbucket@bitbucket.org",
			short: "BB",
		},
		{
			name:  "gitlab self-managed",
			it:    model.Item{Forge: "gitlab", Host: "git.umane.example"},
			larga: "gitlab@git.umane.example",
			short: "GLab@git",
		},
		{
			name:  "host without dots",
			it:    model.Item{Forge: "gitlab", Host: "localhost"},
			larga: "gitlab@localhost",
			short: "GLab@localhost",
		},
		{
			// An unknown forge uses the FULL name: "PHab" would be an invention resembling the other
			// abbreviations and meaning nothing.
			name:  "unknown forge",
			it:    model.Item{Forge: "phabricator", Host: "phab.example"},
			larga: "phabricator@phab.example",
			short: "phabricator@phab",
		},
		{
			name:  "no host",
			it:    model.Item{Forge: "github"},
			larga: "github",
			short: "GH",
		},
		{
			name:  "empty forge",
			it:    model.Item{},
			larga: "",
			short: "",
		},
		{
			name:  "public host in uppercase",
			it:    model.Item{Forge: "github", Host: "GitHub.COM"},
			larga: "github@GitHub.COM",
			short: "GH",
		},
	}
	for _, c := range cases {
		if got := forgeLabel(c.it); got != c.larga {
			t.Errorf("%s: forgeLabel gave %q, want %q", c.name, got, c.larga)
		}
		if got := forgeBadge(c.it); got != c.short {
			t.Errorf("%s: forgeBadge gave %q, want %q", c.name, got, c.short)
		}
		if len(c.short) > len(c.larga) {
			t.Errorf("%s: the short label %q is longer than the long one %q", c.name, c.short, c.larga)
		}
		if got := forgeBadge(c.it); got != strings.TrimSpace(got) {
			t.Errorf("%s: forgeBadge gave %q with spaces", c.name, got)
		}
	}
}

func TestTheUsersRoleDistinguishesOwnFromReview(t *testing.T) {
	viewer := "me"

	if got := roleText(model.Item{ReviewKind: model.ReviewRequested}, viewer); got != "review req" {
		t.Errorf("review requested gave %q", got)
	}
	if got := roleText(model.Item{ReviewKind: model.ReviewAssigned}, viewer); got != "assigned" {
		t.Errorf("review assigned gave %q", got)
	}
	// An owned item with a pending review stays "review req": the forge says a review is waiting, and
	// the viewer is who has to look at it, author or not.
	ownWithReview := model.Item{ReviewKind: model.ReviewRequested, Author: viewer}
	if got := roleText(ownWithReview, viewer); got != "review req" {
		t.Errorf("a requested review on an own item gave %q, want review req", got)
	}

	withoutReview := model.Item{Author: "someone", Title: "something"}
	if got := roleText(withoutReview, viewer); got != "-" {
		t.Errorf("someone else's item with no review gave %q, want -", got)
	}
	if got := roleText(model.Item{Author: viewer, Title: "something"}, viewer); got != "own" {
		t.Errorf("an own item gave %q, want own", got)
	}
	if got := roleText(withoutReview, ""); got == "" {
		t.Error("with an empty viewer the label came out empty")
	}
	if got := roleText(model.Item{Section: model.SectionAuthored, Author: "someone"}, ""); got != "own" {
		t.Errorf("an item created with an empty viewer gave %q, want own", got)
	}
	for _, kind := range []model.ReviewKind{model.ReviewRequested, model.ReviewAssigned, ""} {
		if got := roleText(model.Item{ReviewKind: kind}, viewer); got != strings.TrimSpace(got) {
			t.Errorf("roleText gave %q with spaces for kind %q", got, kind)
		}
	}
}

// Failing carries the count and passing carries none, on purpose: a big number is what makes a red
// cell worth reading.
func TestTheChecksAreSummarisedByTheCountAndTheTrafficLight(t *testing.T) {
	cases := []struct {
		name    string
		checks  model.Checks
		wantTxt string
	}{
		{"failing with count", model.Checks{State: model.ChecksFailing, Failing: 3, Total: 12}, "✗3"},
		{"failing of one", model.Checks{State: model.ChecksFailing, Failing: 1}, "✗1"},
		{"pending with count", model.Checks{State: model.ChecksPending, Pending: 2}, "…2"},
		{"passing without count", model.Checks{State: model.ChecksPassing, Total: 12}, "✓"},
		{"unknown", model.Checks{State: model.ChecksUnknown}, "-"},
		{"empty state", model.Checks{}, "-"},
		{
			name:    "figures without state",
			checks:  model.Checks{Failing: 3, Total: 9},
			wantTxt: "-",
		},
	}
	render := func(cs model.Checks) string { return styleChecks(cs).Render("x") }
	for _, c := range cases {
		if got := checksText(c.checks); got != c.wantTxt {
			t.Errorf("%s: checksText gave %q, want %q", c.name, got, c.wantTxt)
		}
		if got := checksText(c.checks); strings.TrimSpace(got) == "" {
			t.Errorf("%s: checksText returned empty", c.name)
		}
		// Compared as rendered rather than as a style, because lipgloss.Style carries the colour.
		if render(c.checks) != render(model.Checks{State: c.checks.State}) {
			t.Errorf("%s: the style depends on the state alone", c.name)
		}
	}

	vistos := map[string]model.CheckState{}
	for _, state := range []model.CheckState{
		model.ChecksFailing, model.ChecksPending, model.ChecksPassing, model.ChecksUnknown,
	} {
		painted := render(model.Checks{State: state})
		if other, seen := vistos[painted]; seen {
			t.Errorf("states %q and %q paint the same (%q)", state, other, painted)
		}
		vistos[painted] = state
	}
}

// The reason matters as much as the veto: an empty one produces a toast with the prefix and nothing.
func TestTheActionableStateGivesReasonsThatSayWhatToDo(t *testing.T) {
	cases := []struct {
		name  string
		state string
		veto  bool
	}{
		{"merged", "MERGED", true},
		{"closed", "CLOSED", true},
		{"open", "OPEN", false},
		{"blank", "", false},
		{"merged in lowercase", "merged", true},
		{"closed in lowercase", "closed", true},
	}
	for _, c := range cases {
		ok, reason := state.Actionable(model.Item{Number: 1, State: c.state})
		if ok == c.veto {
			t.Errorf("%s: Actionable gave ok=%v, want %v", c.name, ok, !c.veto)
		}
		if !ok && strings.TrimSpace(reason) == "" {
			t.Errorf("%s: vetoed without giving a reason", c.name)
		}
		if ok && reason != "" {
			t.Errorf("%s: did not veto but gave reason %q", c.name, reason)
		}
	}
}

// Number and unit are NOT separated by a space: "30s ago" is one field with the count glued to the
// letter.
func numberOf(text string) (int, int) {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return 0, 0
	}
	n, unit, _ := partsOf(fields[0])
	return n, unit
}

func durationOf(text string) (time.Duration, error) {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return 0, errors.New("empty text")
	}
	n, unit, had := partsOf(fields[0])
	if !had {
		if fields[0] == "now" || fields[0] == "no" {
			return 0, nil
		}
		return 0, errors.New("no number")
	}
	switch unit {
	case 1:
		return time.Duration(n) * time.Second, nil
	case 2:
		return time.Duration(n) * time.Minute, nil
	case 3:
		return time.Duration(n) * time.Hour, nil
	}
	return 0, errors.New("unknown unit")
}

func unitDuration(unit int) time.Duration {
	switch unit {
	case 1:
		return time.Second
	case 2:
		return time.Minute
	case 3:
		return time.Hour
	}
	return 0
}

// The third value tells "0" from "no number": "0s ago" is data and "now" has no count. Without the
// distinction a "now" would read as zero seconds.
func partsOf(campo string) (n, unit int, hadNumber bool) {
	i := 0
	for i < len(campo) && campo[i] >= '0' && campo[i] <= '9' {
		n = n*10 + int(campo[i]-'0')
		i++
	}
	if i == 0 {
		return 0, 0, false
	}
	switch {
	case i < len(campo) && campo[i] == 's':
		unit = 1
	case i < len(campo) && campo[i] == 'm':
		unit = 2
	case i < len(campo) && campo[i] == 'h':
		unit = 3
	}
	return n, unit, true
}
