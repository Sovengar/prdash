package tui

import (
	"strings"
	"testing"
	"time"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

func modelWithNewCycle(t *testing.T) Model {
	t.Helper()
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	m.cycle = 1
	m.beginRefresh()
	return m
}

func TestAStaleCycleMessageIsDiscardedButRearmsTheTimer(t *testing.T) {
	cases := []struct {
		name string
		old  interface{}
	}{
		{"authMsg", authMsg{cycle: 0, forge: "github", auth: model.AuthState{OK: false}}},
		{"pageMsg", pageMsg{cycle: 0, key: streamKey{forge: "github"}, items: nil}},
		{"forgeDoneMsg", forgeDoneMsg{cycle: 0, forge: "github"}},
		{"refreshDoneMsg", refreshDoneMsg{cycle: 0}},
		{"actionMsg", actionMsg{cycle: 0}},
	}

	for _, c := range cases {
		m := modelWithNewCycle(t)
		m.loading = true
		before := m.loading

		output, cmd := m.Update(c.old)

		got := output.(Model)
		if got.loading != before {
			t.Errorf("%s: a stale message touched `loading` (%v -> %v)", c.name, before, got.loading)
		}
		// The pump is re-armed: withPump always returns a non-nil Cmd, and it is the only thing that does.
		if cmd == nil {
			t.Errorf("%s: a stale message returns nil and kills the event timer", c.name)
		}
	}
}

// The other side, so the table above is not worth "everything is re-armed".
func TestACurrentMessageDoesWhatItShould(t *testing.T) {
	m := modelWithNewCycle(t)
	m.loading = true
	m.lastRefresh = time.Time{}

	output, cmd := m.Update(refreshDoneMsg{cycle: m.cycle})
	got := output.(Model)

	if got.loading {
		t.Error("a current refreshDoneMsg left `loading` true: the spinner would never stop")
	}
	if got.lastRefresh.IsZero() {
		t.Error("a current refreshDoneMsg did not record lastRefresh: the border would not say when")
	}
	if cmd == nil {
		t.Error("a current refreshDoneMsg does not re-arm the tick: the refresh dies after one")
	}

	m = modelWithNewCycle(t)
	m.statuses = map[string]*forgeStatus{"github": {forge: "github", host: "github.com"}}
	autenticado := model.AuthState{Forge: "github", OK: true, Login: "me"}
	output, _ = m.Update(authMsg{cycle: m.cycle, forge: "github", auth: autenticado})
	if !output.(Model).statuses["github"].auth.OK {
		t.Error("a current authMsg did not write the auth state: the forge would stay " +
			"marked as degraded")
	}

	m = modelWithNewCycle(t)
	m.statuses = map[string]*forgeStatus{}
	if _, cmd := m.Update(authMsg{cycle: m.cycle, forge: "deshabilitado",
		auth: model.AuthState{OK: true}}); cmd == nil {
		t.Error("an authMsg from an unknown forge does not re-arm the timer")
	}
}

func TestTheModelsMessagesDoNotRearmTheTimer(t *testing.T) {
	m := modelWithNewCycle(t)
	output, cmd := m.Update(notifyMsg{text: "something happened", level: levelWarn})
	if cmd != nil {
		t.Error("notifyMsg returns a Cmd: every notice would leave a goroutine waiting")
	}
	got := output.(Model)
	if strings.TrimSpace(lastToast(got)) == "" {
		t.Error("notifyMsg did not show the notice")
	}

	m = modelWithNewCycle(t)
	if _, cmd := m.Update(reviewCleanupMsg{}); cmd != nil {
		t.Error("reviewCleanupMsg returns a Cmd: it consumes no reader and must not re-arm")
	}
}

func TestTheTickFiresOnUnpausingAndNotWhilePaused(t *testing.T) {
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	// paused() is `loading || actionBusy`; there is no flag of its own.
	m.loading = true
	m.tickPending = true

	output, cmd := m.Update(tickMsg{})
	got := output.(Model)

	// My first version asserted the opposite, having read the name instead of the code.
	if !got.tickPending {
		t.Error("a paused tick left none pending: on resume there would be no next one")
	}
	if cmd == nil {
		t.Error("the paused tick does not re-arm: on resume there would be no next tick")
	}
	if got.cycle != m.cycle {
		t.Errorf("a paused tick started a cycle: cycle %d -> %d", m.cycle, got.cycle)
	}

	m2 := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	m2.tickPending = true
	before := m2.cycle
	out2, _ := m2.Update(tickMsg{})
	if out2.(Model).cycle == before {
		t.Error("an unpaused tick did not start a cycle: the refresh would never happen")
	}
}

// Only a merge names its strategy: it can be rebase, squash or merge commit, and the user has to know which one runs.
func TestTheActionProgressNoticeNamesTheMergeModeAndNothingElseDoes(t *testing.T) {
	cases := []struct {
		kind forge.ActionKind
		mode forge.MergeMode
		want string
	}{
		{forge.ActionMerge, forge.Rebase, "merge (rebase) in progress\u2026"},
		{forge.ActionMerge, forge.Squash, "merge (squash) in progress\u2026"},
		{forge.ActionMerge, forge.MergeCommit, "merge (merge commit) in progress\u2026"},
		{forge.ActionApprove, forge.Rebase, "approve in progress\u2026"},
		{forge.ActionRetarget, forge.Squash, "retarget in progress\u2026"},
	}
	for _, c := range cases {
		if got := actionProgressNotice(c.kind, c.mode); got != c.want {
			t.Errorf("actionProgressNotice(%q, %q) = %q, want %q", c.kind, c.mode, got, c.want)
		}
	}
}
