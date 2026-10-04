package forge_test

import (
	"context"
	"testing"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

func TestRunActionMergeAsksTheAdapterToDeleteTheBranch(t *testing.T) {
	for _, want := range []bool{true, false} {
		item := mkItem("github", "github.com", "acme/widget", 10)
		item.State = "OPEN"
		item.HeadSHA = "9f1c0de"
		fake := &testutil.FakeAdapter{
			ForgeName:  "github",
			HostName:   "github.com",
			ItemStates: map[string]model.Item{testutil.ItemKey("acme/widget", 10): item},
		}

		out := forge.RunAction(context.Background(), fake, forge.ActionMerge, item.Ref, 10,
			forge.MergeRequest{Mode: forge.Squash, DeleteBranch: want})
		if !out.OK {
			t.Fatalf("borrar=%v: outcome = %+v", want, out)
		}
		if out.DeleteBranch != want {
			t.Errorf("outcome.DeleteBranch = %v, want %v", out.DeleteBranch, want)
		}
		if got := fake.MergeDeleteCount(want); got != 1 {
			t.Errorf("el adapter recibió %d merges con DeleteBranch=%v, want 1", got, want)
		}
	}
}

// The case that makes the re-read's pin necessary: the delete rides in the SAME command.
func TestRunActionKeepsTheMergeWhenOnlyTheDeleteFails(t *testing.T) {
	item := mkItem("github", "github.com", "acme/widget", 11)
	item.State = "OPEN"
	item.HeadSHA = "9f1c0de"
	fake := &testutil.FakeAdapter{
		ForgeName:  "github",
		HostName:   "github.com",
		ItemStates: map[string]model.Item{testutil.ItemKey("acme/widget", 11): item},
		ActionWarnings: map[string][]model.Warning{
			"merge:acme/widget#11": {{Forge: "github", Kind: "permission", Msg: "Resource not accessible by integration"}},
		},
	}
	fake.OnMerge = func(it *model.Item) { it.State = "MERGED" }

	out := forge.RunAction(context.Background(), fake, forge.ActionMerge, item.Ref, 11,
		forge.MergeRequest{Mode: forge.Squash, DeleteBranch: true})
	if !out.OK {
		t.Fatalf("un merge que salió no puede reportarse como fallido: %+v", out)
	}
	if out.Perm {
		t.Error("no debe quedar registrado como denegado: el merge sí se puede hacer")
	}
	if out.DeleteMsg == "" {
		t.Error("falta el motivo por el que la rama no se borró")
	}
}

func TestRunActionSaysNothingAboutTheBranchWithoutThePin(t *testing.T) {
	// Without a HeadSHA the real adapter refuses, and the fake copies that refusal.
	item := mkItem("github", "github.com", "acme/widget", 12)
	item.State = "OPEN"
	fake := &testutil.FakeAdapter{
		ForgeName:  "github",
		HostName:   "github.com",
		ItemStates: map[string]model.Item{testutil.ItemKey("acme/widget", 12): item},
	}

	out := forge.RunAction(context.Background(), fake, forge.ActionMerge, item.Ref, 12,
		forge.MergeRequest{Mode: forge.Squash, DeleteBranch: true})
	if out.OK {
		t.Fatalf("sin pin no puede haber merge: %+v", out)
	}
	if out.DeleteMsg != "" {
		t.Errorf("DeleteMsg = %q, want vacío: no hubo merge", out.DeleteMsg)
	}
}

// A fork PR has no branch to delete in the target repo and the forge does not complain.
func TestRunActionDoesNotBlamTheBranchOnAForkPR(t *testing.T) {
	item := mkItem("github", "github.com", "acme/widget", 13)
	item.State = "OPEN"
	item.HeadSHA = "9f1c0de"
	item.IsFork = true
	fake := &testutil.FakeAdapter{
		ForgeName:  "github",
		HostName:   "github.com",
		ItemStates: map[string]model.Item{testutil.ItemKey("acme/widget", 13): item},
		OnMerge:    func(it *model.Item) { it.State = "MERGED" },
	}

	out := forge.RunAction(context.Background(), fake, forge.ActionMerge, item.Ref, 13,
		forge.MergeRequest{Mode: forge.Squash, DeleteBranch: true})
	if !out.OK {
		t.Fatalf("outcome = %+v", out)
	}
	if out.DeleteMsg == "" {
		t.Error("un PR de fork no puede reportarse como rama borrada")
	}
}
