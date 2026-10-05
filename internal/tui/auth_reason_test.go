package tui

import (
	"strings"
	"testing"

	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

func TestAuthReasonReachesTheScreen(t *testing.T) {
	m := newTestModel(t, &testutil.FakeAdapter{
		ForgeName: "bitbucket", HostName: "bitbucket.org",
		AuthState: model.AuthState{Forge: "bitbucket", OK: false, Reason: "not implemented in this version"},
	})
	m = send(t, m, authMsg{cycle: 1, forge: "bitbucket",
		auth: model.AuthState{Forge: "bitbucket", OK: false, Reason: "not implemented in this version"}})
	m = send(t, m, page(1, "bitbucket", "bitbucket.org", model.SectionAuthored, "",
		[]model.Item{mkItem("bitbucket", "bitbucket.org", "team/repo", "MR", 4, "")}, false))
	m = showSection(m, model.SectionAuthored)

	m = press(t, m, "a")
	toast := lastToast(m)
	if !strings.Contains(toast, "not implemented") {
		t.Errorf("toast = %q, want the adapters real reason", toast)
	}
	if strings.Contains(toast, "not authenticated") {
		t.Errorf("toast = %q: the generic label sends the person to authentication", toast)
	}
}

func TestAuthReasonFallsBackWhenEmpty(t *testing.T) {
	if got := authReason(model.AuthState{Forge: "github"}); got != "not authenticated" {
		t.Errorf("authReason = %q, want not authenticated", got)
	}
}
