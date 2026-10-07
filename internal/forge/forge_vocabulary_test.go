package forge

import (
	"errors"
	"strings"
	"testing"

	"prdash/internal/forge/model"
	"prdash/internal/state"
)

func TestMergeModeIsValidatedBeforeBuildingTheArgv(t *testing.T) {
	// The labels are NOT the mode names: MergeCommit is confirmed as "merge commit", what the user has to recognise.
	labels := map[MergeMode]string{
		MergeCommit: "merge commit",
		Rebase:      "rebase",
		Squash:      "squash",
	}
	for _, m := range []MergeMode{MergeCommit, Rebase, Squash} {
		if !m.Valid() {
			t.Errorf("%q: Valid() returned false for a known mode", m)
		}
		if got := m.Label(); got != labels[m] {
			t.Errorf("%q: Label() returned %q, want %q", m, got, labels[m])
		}
		if strings.TrimSpace(m.Label()) == "" {
			t.Errorf("%q: empty label", m)
		}
	}

	for _, m := range []MergeMode{"", "REBASE", "merge-commit", "fast-forward", "rebas"} {
		if m.Valid() {
			t.Errorf("%q: Valid() returned true for a mode that does not exist", m)
		}
		err := ErrUnknownMergeMode(m)
		if err == nil {
			t.Fatalf("%q: ErrUnknownMergeMode returned nil", m)
		}
		if m != "" && !strings.Contains(err.Error(), string(m)) {
			t.Errorf("the error %q does not name the mode %q", err, m)
		}
		// The valid list holds NAMES, not labels: the error says "expected merge, rebase or squash".
		for _, ok := range []MergeMode{MergeCommit, Rebase, Squash} {
			if !strings.Contains(err.Error(), string(ok)) {
				t.Errorf("the error %q does not mention the valid mode %q", err, ok)
			}
		}
	}
	if !strings.Contains(ErrUnknownMergeMode("").Error(), "expected") {
		t.Error("the error for the empty mode does not say that a list was expected")
	}
}

// "By the tail" is what distinguishes this from a `[:limit]`.
func TestKeepLastTrimsFromTheTail(t *testing.T) {
	p := CommentPage{Comments: comments(CommentLimit + 5), Total: CommentLimit + 5}
	p = p.KeepLast()
	if len(p.Comments) != CommentLimit {
		t.Fatalf("%d comments left, want %d", len(p.Comments), CommentLimit)
	}
	if p.Total != CommentLimit+5 {
		t.Errorf("the total ended up %d: it has to keep saying there are more", p.Total)
	}
	last := p.Comments[len(p.Comments)-1]
	if last.Body != "c"+itoa(CommentLimit+4) {
		t.Errorf("the last comment was not kept: %q", last.Body)
	}
	if p.Comments[0].Body == "c0" {
		t.Error("the most recent comment was lost, not the oldest one")
	}

	p = CommentPage{Comments: comments(CommentLimit), Total: CommentLimit}
	p = p.KeepLast()
	if len(p.Comments) != CommentLimit || p.Comments[0].Body != "c0" {
		t.Errorf("it trimmed at the exact limit: %d left, first %q",
			len(p.Comments), p.Comments[0])
	}

	p = CommentPage{Comments: comments(2), Total: 2}.KeepLast()
	if len(p.Comments) != 2 || p.Comments[1].Body != "c1" {
		t.Errorf("it trimmed below the limit: %+v", p.Comments)
	}

	if got := (CommentPage{}).KeepLast(); len(got.Comments) != 0 {
		t.Errorf("an empty page returned %d comments", len(got.Comments))
	}
}

func TestTheCLIFixtureBecomesACanonicalReason(t *testing.T) {
	cases := []struct {
		name     string
		warns    []model.Warning
		wantOK   bool
		wantConf bool
		wantPerm bool
		wantMsg  string
	}{
		{"no warnings", nil, true, false, false, ""},
		{
			name:     "notfound is conflict",
			warns:    []model.Warning{{Kind: "notfound", Msg: "no such pull request"}},
			wantConf: true, wantMsg: "no such pull request",
		},
		{
			name:     "permission is permission",
			warns:    []model.Warning{{Kind: "permission", Msg: "you cannot merge"}},
			wantPerm: true, wantMsg: "you cannot merge",
		},
		{
			name:     "auth is permission",
			warns:    []model.Warning{{Kind: "auth", Msg: "not logged in"}},
			wantPerm: true, wantMsg: "not logged in",
		},
		{
			name:     "unsupported is permission",
			warns:    []model.Warning{{Kind: "unsupported", Msg: "no API"}},
			wantPerm: true, wantMsg: "no API",
		},
		{
			name:     "selfreview is permission, with the canonical reason",
			warns:    []model.Warning{{Kind: "selfreview", Msg: "you cannot review your own PR"}},
			wantPerm: true, wantMsg: state.SelfReviewReason,
		},
		{
			name:    "unmergeable is neither permission nor conflict",
			warns:   []model.Warning{{Kind: "unmergeable", Msg: "mergeable: false"}},
			wantMsg: state.UnmergeableReason,
		},
		{
			name:     "conflict is conflict",
			warns:    []model.Warning{{Kind: "conflict", Msg: "out of date"}},
			wantConf: true, wantMsg: "out of date",
		},
		{
			name:     "ratelimit is conflict",
			warns:    []model.Warning{{Kind: "ratelimit", Msg: "403 rate limited"}},
			wantConf: true, wantMsg: "403 rate limited",
		},
		{
			name:     "network is conflict",
			warns:    []model.Warning{{Kind: "network", Msg: "dial tcp: no route"}},
			wantConf: true, wantMsg: "dial tcp: no route",
		},
		{
			name:     "timeout is conflict",
			warns:    []model.Warning{{Kind: "timeout", Msg: "deadline exceeded"}},
			wantConf: true, wantMsg: "deadline exceeded",
		},
		{
			name:    "unknown kind keeps the message",
			warns:   []model.Warning{{Kind: "something-new", Msg: "whatever"}},
			wantMsg: "whatever",
		},
		{
			name:     "permission wins over conflict",
			warns:    []model.Warning{{Kind: "ratelimit", Msg: "403"}, {Kind: "permission", Msg: "no"}},
			wantPerm: true, wantMsg: "403",
		},
		{
			name:     "selfreview wins over the rest",
			warns:    []model.Warning{{Kind: "ratelimit", Msg: "403"}, {Kind: "selfreview", Msg: "x"}},
			wantPerm: true, wantMsg: state.SelfReviewReason,
		},
	}
	for _, c := range cases {
		ok, conf, perm, msg := classifyAction(c.warns)
		if ok != c.wantOK || conf != c.wantConf || perm != c.wantPerm || msg != c.wantMsg {
			t.Errorf("%s: returned (%v, %v, %v, %q), want (%v, %v, %v, %q)",
				c.name, ok, conf, perm, msg, c.wantOK, c.wantConf, c.wantPerm, c.wantMsg)
		}
		if len(c.warns) > 0 && strings.TrimSpace(msg) == "" {
			t.Errorf("%s: there are warnings and the reason ended up empty", c.name)
		}
	}
}

// The ORDER of the checks is what matters and it is subtle: `notfound` wins.
func TestCheckBeforeActionVetoesWhatCannotBeDone(t *testing.T) {
	out := checkBeforeAction(model.Item{Number: 1}, []model.Warning{{Kind: "notfound", Msg: "404"}})
	if out == nil {
		t.Fatal("a vanished item passed the pre-check")
	}
	if !out.Conflict {
		t.Error("a vanished item was not classified as conflict")
	}
	if out.Perm {
		t.Error("a vanished item was classified as permission: the TUI would leave it without " +
			"merge forever, while what actually happened is that it no longer exists")
	}
	if !strings.Contains(out.Msg, "no longer exists") {
		t.Errorf("the reason %q does not say that the item vanished", out.Msg)
	}
	if out.HasItem {
		t.Error("it brought back a re-read item that should not exist")
	}

	out = checkBeforeAction(model.Item{Number: 1}, []model.Warning{{Kind: "permission", Msg: "no"}})
	if out == nil || !out.Perm || out.Conflict {
		t.Errorf("no permission returned %+v, want Perm", out)
	}

	out = checkBeforeAction(model.Item{Number: 7}, []model.Warning{{Kind: "weird", Msg: "who knows"}})
	if out == nil || !out.Conflict {
		t.Fatalf("an unclassified warning returned %+v, want conflict", out)
	}
	if !out.HasItem || out.Item.Number != 7 {
		t.Errorf("it had the re-read item, did not bring it back: %+v", out.Item)
	}

	out = checkBeforeAction(model.Item{Number: 1}, []model.Warning{{Kind: "weird"}})
	if out == nil || out.Msg == "" {
		t.Fatalf("a warning without a message returned %+v, want a reason", out)
	}
	if !strings.Contains(out.Msg, "re-read") {
		t.Errorf("the filler reason %q does not say that it could not be re-read", out.Msg)
	}

	out = checkBeforeAction(model.Item{}, nil)
	if out == nil || !out.Conflict {
		t.Fatalf("an item that could not be read returned %+v, want conflict", out)
	}
	if out.HasItem {
		t.Error("it brought back an item that could not be read")
	}

	out = checkBeforeAction(model.Item{Number: 3, State: "MERGED"}, nil)
	if out == nil || !out.Conflict {
		t.Fatalf("an already merged item returned %+v, want conflict", out)
	}
	if out.Msg == "" {
		t.Error("the state veto returned an empty reason")
	}
	if !out.HasItem {
		t.Error("the state veto did not bring back the item, which the TUI needs to paint it")
	}

	if out := checkBeforeAction(model.Item{Number: 3, State: "OPEN", ReviewDecision: "APPROVED"}, nil); out != nil {
		t.Errorf("an actionable item returned %+v, want nil", out)
	}
}

func TestFirstMsgTakesTheFirstOneAndInventsNothing(t *testing.T) {
	cases := []struct {
		warns []model.Warning
		want  string
	}{
		{nil, ""},
		{[]model.Warning{}, ""},
		{[]model.Warning{{Kind: "a", Msg: "first"}, {Kind: "b", Msg: "second"}}, "first"},
		{[]model.Warning{{Kind: "a"}}, ""},
	}
	for _, c := range cases {
		if got := firstMsg(c.warns); got != c.want {
			t.Errorf("firstMsg(%+v) returned %q, want %q", c.warns, got, c.want)
		}
	}
}

func TestAMergeReasonIsNotDeclaredTwoWays(t *testing.T) {
	one := ErrUnknownMergeMode("nope")
	other := ErrUnknownMergeMode("nope")
	if one.Error() != other.Error() {
		t.Errorf("two calls to the same error gave different texts: %q and %q",
			one, other)
	}
	joined := errors.Join(other)
	if !strings.Contains(joined.Error(), "nope") {
		t.Errorf("the error lost when grouping it: %v", joined)
	}
}

func comments(n int) []model.Comment {
	out := make([]model.Comment, n)
	for i := range out {
		out[i] = model.Comment{Author: "a" + itoa(i), Body: "c" + itoa(i)}
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// Complementary to Valid: Label returns the name as given, never disguising an unknown mode as a known one.
func TestTheNameOfAnUnknownModeIsReturnedAsIsAndNotDisguisedAsAKnownOne(t *testing.T) {
	known := map[MergeMode]bool{MergeCommit: true, Rebase: true, Squash: true}

	for _, m := range []MergeMode{
		MergeMode("fast-forward"), MergeMode(""), MergeMode("MERGE"), MergeMode(" squash"),
		MergeMode("rebasea"),
	} {
		got := m.Label()
		if got != string(m) {
			t.Errorf("mode %q gave label %q: it has to be returned as is, or the "+
				"warning does not say what to fix", m, got)
		}
		for kn := range known {
			if got == kn.Label() && string(m) != string(kn) {
				t.Errorf("mode %q gave label %q, which belongs to a known mode",
					m, got)
			}
		}
		if m.Valid() {
			t.Errorf("mode %q gave Valid() = true: it would be accepted in `gh pr merge` without a "+
				"flag and would open an interactive prompt that leaves the TUI hanging", m)
		}
	}

	for _, m := range []MergeMode{MergeCommit, Rebase, Squash} {
		if !m.Valid() || m.Label() != testLabels[m] {
			t.Errorf("known mode %q changed: Valid=%v Label=%q", m, m.Valid(), m.Label())
		}
	}
}

var testLabels = map[MergeMode]string{
	MergeCommit: "merge commit",
	Rebase:      "rebase",
	Squash:      "squash",
}
