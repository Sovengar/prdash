package forge_test

import (
	"context"
	"strings"
	"testing"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

func retargetFixture(t *testing.T, it model.Item) *testutil.FakeAdapter {
	t.Helper()
	a := &testutil.FakeAdapter{
		ForgeName:  it.Forge,
		HostName:   it.Host,
		ItemStates: map[string]model.Item{},
	}
	a.ItemStates[testutil.ItemKey(it.Ref.Project, it.Number)] = it
	return a
}

func retargetItem(state string) model.Item {
	it := model.NewItem(model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget", Owner: "acme", Name: "widget"}, 7)
	it.Title = "Add widget"
	it.SourceBranch = "feat/x"
	it.TargetBranch = "main"
	it.State = state
	return it
}

// The path is the same as the other actions —re-read, check, act, re-read—.
func TestRunRetargetChangesTheBaseAndReReads(t *testing.T) {
	it := retargetItem("OPEN")
	a := retargetFixture(t, it)

	out := forge.RunRetarget(context.Background(), a, it.Ref, it.Number, "release/2.0")
	if !out.OK {
		t.Fatalf("RunRetarget = %+v, want OK", out)
	}
	if a.RetargetCount() != 1 || a.Retargets[0] != "release/2.0" {
		t.Errorf("the adapter received %v, want one call with release/2.0", a.Retargets)
	}
	if out.Base != "release/2.0" {
		t.Errorf("Outcome.Base = %q, want the requested branch for the warning", out.Base)
	}
	if !out.HasItem {
		t.Error("Outcome does not bring the re-read item: the view would keep the old base")
	}
}

func TestRunRetargetRefusesWithoutABranch(t *testing.T) {
	it := retargetItem("OPEN")
	a := retargetFixture(t, it)

	out := forge.RunRetarget(context.Background(), a, it.Ref, it.Number, "   ")
	if !out.Perm {
		t.Errorf("Outcome = %+v, want Perm: an empty base flag leaves the PR with no base", out)
	}
	if out.Msg != forge.ErrMissingBaseBranch.Error() {
		t.Errorf("Msg = %q, want the canonical reason", out.Msg)
	}
	if out.HasItem {
		t.Error("it re-read the item for nothing: with no branch there is no action to save")
	}
	if a.RetargetCount() != 0 {
		t.Errorf("it called the adapter (%v) with no branch to send", a.Retargets)
	}
}

func TestRunRetargetRespectsTheOpenGuard(t *testing.T) {
	for _, state := range []string{"MERGED", "CLOSED"} {
		it := retargetItem(state)
		a := retargetFixture(t, it)

		out := forge.RunRetarget(context.Background(), a, it.Ref, it.Number, "release/2.0")
		if !out.Conflict {
			t.Errorf("RunRetarget on %s = %+v, want conflict", state, out)
		}
		if a.RetargetCount() != 0 {
			t.Errorf("it called the adapter on a %s item", state)
		}
	}
}

func TestRunRetargetClassifiesTheForgesPermission(t *testing.T) {
	it := retargetItem("OPEN")
	a := retargetFixture(t, it)
	a.ActionWarnings = map[string][]model.Warning{
		"retarget:" + testutil.ItemKey(it.Ref.Project, it.Number): {
			{Forge: "github", Kind: "permission", Msg: "must have push access"},
		},
	}

	out := forge.RunRetarget(context.Background(), a, it.Ref, it.Number, "release/2.0")
	if !out.Perm {
		t.Errorf("Outcome = %+v, want Perm for lack of push", out)
	}
	if !strings.Contains(out.Msg, "push access") {
		t.Errorf("Msg = %q, want the forge's reason", out.Msg)
	}
}
