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
			t.Fatalf("delete=%v: outcome = %+v", want, out)
		}
		if out.DeleteBranch != want {
			t.Errorf("outcome.DeleteBranch = %v, want %v", out.DeleteBranch, want)
		}
		if got := fake.MergeDeleteCount(want); got != 1 {
			t.Errorf("the adapter received %d merges with DeleteBranch=%v, want 1", got, want)
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
		t.Fatalf("a merge that went through cannot be reported as failed: %+v", out)
	}
	if out.Perm {
		t.Error("it must not end up recorded as denied: the merge can be done")
	}
	if out.DeleteMsg == "" {
		t.Error("the reason the branch was not deleted is missing")
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
		t.Fatalf("without a pin there can be no merge: %+v", out)
	}
	if out.DeleteMsg != "" {
		t.Errorf("DeleteMsg = %q, want empty: there was no merge", out.DeleteMsg)
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
		t.Error("a fork PR cannot be reported as branch-deleted")
	}
}
