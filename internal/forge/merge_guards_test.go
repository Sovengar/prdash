package forge_test

import (
	"context"
	"strings"
	"testing"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/state"
	"prdash/internal/testutil"
)

// The HeadSHA that travels is the one from the re-read, not the card's.
func TestMergeActionsPassTheReReadHeadSHAAndNotTheCards(t *testing.T) {
	seen := mkItem("github", "github.com", "acme/widget", 3)
	seen.State = "OPEN"
	seen.HeadSHA = "sha-the-user-saw"

	reread := seen
	reread.HeadSHA = "fresh-sha-from-the-refresh"

	fake := stateReadingAdapter(reread)
	req := forge.MergeRequest{Mode: forge.Squash, DeleteBranch: true}
	out := forge.RunAction(context.Background(), fake, forge.ActionMerge, seen.Ref, 3, req)

	if !fake.mergeCalled {
		t.Error("the merge did not reach Merge: there is no good case to compare against")
	}
	if out.OK {
		t.Errorf("the fake does not merge and the result came out fine: %+v", out)
	}
	if out.Mode != forge.Squash || !out.DeleteBranch {
		t.Errorf("the request went out with mode %q and delete %v: they are what the user chose",
			out.Mode, out.DeleteBranch)
	}
	if !out.HasItem {
		t.Error("the result does not bring the re-read state: the TUI cannot refresh the row")
	}
	if out.Item.HeadSHA != "fresh-sha-from-the-refresh" {
		t.Errorf("the re-read state brings SHA %q: it would be applied over a stale card",
			out.Item.HeadSHA)
	}
}

// The two states that really block, and it is convenient that they are only two.
func TestAnAlreadyMergedOrClosedPRBlocksTheMergeWithoutCallingTheForge(t *testing.T) {
	for _, c := range []struct {
		name  string
		state string
	}{
		{"already merged", "MERGED"},
		{"already closed", "CLOSED"},
	} {
		it := mkItem("github", "github.com", "acme/widget", 4)
		it.State = c.state
		it.HeadSHA = "abc123"
		fake := stateReadingAdapter(it)

		out := forge.RunAction(context.Background(), fake, forge.ActionMerge, it.Ref, 4,
			forge.MergeRequest{Mode: forge.MergeCommit})

		if out.OK {
			t.Errorf("%s: the merge went through over a %q PR", c.name, c.state)
		}
		if fake.mergeCalled {
			t.Errorf("%s: Merge was called: a merge launched over a %q PR can "+
				"integrate halfway without anyone knowing", c.name, c.state)
		}
		if !out.Conflict {
			t.Errorf("%s: it was not marked as conflict (%+v): the TUI would not offer a refresh",
				c.name, out)
		}
		// The reason IS the reason: "already merged", not a generic "something failed".
		if !strings.Contains(out.Msg, "already") {
			t.Errorf("%s: the reason %q does not say that the PR is already %s", c.name, out.Msg, c.state)
		}
		if !out.HasItem || out.Item.Number != 4 {
			t.Errorf("%s: the result does not bring the re-read item", c.name)
		}
	}

	it := mkItem("github", "github.com", "acme/widget", 40)
	it.State = "DIRTY"
	it.HeadSHA = "abc123"
	fake := stateReadingAdapter(it)
	if out := forge.RunAction(context.Background(), fake, forge.ActionMerge, it.Ref, 40,
		forge.MergeRequest{Mode: forge.MergeCommit}); !fake.mergeCalled || out.OK {
		t.Errorf("a PR whose branches conflict did not reach Merge, or it went through: "+
			"called=%v out=%+v. The soft block is applied by MergeBlock, not RunAction",
			fake.mergeCalled, out)
	}
}

// The test documenting a bug: an unknown action returned OK:true without doing anything.
func TestAnActionThatIsNeitherApproveNorMergeIsNotDispatchedAndDoesNotReadTheItem(t *testing.T) {
	for _, kind := range []forge.ActionKind{
		forge.ActionRetarget, forge.ActionKind("invented"), forge.ActionKind(""),
	} {
		it := mkItem("github", "github.com", "acme/widget", 8)
		it.State = "OPEN"
		it.HeadSHA = "abc123"
		fake := stateReadingAdapter(it)
		fake.stateReads = 0

		out := forge.RunAction(context.Background(), fake, kind, it.Ref, 8,
			forge.MergeRequest{Mode: forge.MergeCommit})

		if out.OK {
			t.Errorf("action %q came out fine without being done: the header's warning would say "+
				"the operation worked", kind)
		}
		if fake.mergeCalled {
			t.Errorf("action %q reached Merge", kind)
		}
		if fake.stateReads != 0 {
			t.Errorf("action %q re-read the item %d times: an impossible action must not "+
				"spend a trip to the forge", kind, fake.stateReads)
		}
		// The reason is canonical and NAMES the action, so whoever debugs does not have to guess.
		if !strings.Contains(out.Msg, "does not implement") {
			t.Errorf("action %q gave the reason %q, which does not say it is not implemented", kind, out.Msg)
		}
		if !strings.Contains(out.Msg, string(kind)) {
			t.Errorf("action %q gave the reason %q, which does not name it", kind, out.Msg)
		}
		// Neither a conflict ("the item changed, refresh") nor a permission.
		if out.Conflict || out.Perm || out.Unmergeable {
			t.Errorf("action %q was classified as %+v: it is neither a conflict nor a permission, "+
				"it is a failure of whoever called", kind, out)
		}
		if out.Kind != kind {
			t.Errorf("the result Kind is %q, want %q", out.Kind, kind)
		}
		if out.ID.Project != "acme/widget" || out.ID.Number != 8 {
			t.Errorf("the result ID is %+v: the warning could not be attributed to the item", out.ID)
		}
	}

	it := mkItem("github", "github.com", "acme/widget", 9)
	it.State = "OPEN"
	good := stateReadingAdapter(it)
	good.stateReads = 0
	forge.RunAction(context.Background(), good, forge.ActionApprove, it.Ref, 9,
		forge.MergeRequest{})
	if good.stateReads == 0 {
		t.Error("with approve the item was not re-read: the guard is not what cut, and the inbox " +
			"state would stay unrefreshed")
	}
}

// A veto that does not depend on the forge: nobody can approve their own.
func TestApprovingYourOwnIsRefusedWithAReason(t *testing.T) {
	it := mkItem("github", "github.com", "acme/widget", 7)
	it.State = "OPEN"
	it.HeadSHA = "abc123"
	it.Author = "myself"
	fake := stateReadingAdapter(it)
	fake.approveVeto = true

	out := forge.RunAction(context.Background(), fake, forge.ActionApprove, it.Ref, 7,
		forge.MergeRequest{})
	if out.OK {
		t.Fatal("it approved its own PR")
	}
	if strings.TrimSpace(out.Msg) == "" {
		t.Fatal("with no reason: the user sees that the key does nothing")
	}
	// The reason is the CANONICAL one, not the CLI's text, which changes between versions.
	if out.Msg != state.SelfReviewReason {
		t.Errorf("the reason is %q, want the canonical %q", out.Msg, state.SelfReviewReason)
	}
	// Classified as PERMISSION and not as a conflict, which stops the TUI from asking for a refresh that fixes nothing.
	if !out.Perm {
		t.Errorf("the veto came out as %+v: without the permission mark the TUI would retry it "+
			"on every refresh", out)
	}
	if out.Conflict {
		t.Error("the veto additionally came out as conflict: two kinds at once and the TUI does " +
			"not know which to offer")
	}
}

// It embeds the project's FakeAdapter, so the real methods still work and only merges are recorded.
type mergeRecordingAdapter struct {
	testutil.FakeAdapter
	mergeCalled bool
	stateReads  int
	approveVeto bool
}

func (a *mergeRecordingAdapter) Approve(
	_ context.Context, _ model.RepoRef, _ int,
) []model.Warning {
	if a.approveVeto {
		return []model.Warning{{Forge: a.ForgeName, Kind: "selfreview", Msg: "you cannot approve"}}
	}
	return a.FakeAdapter.Approve(context.Background(), model.RepoRef{}, 0)
}

func (a *mergeRecordingAdapter) ItemState(
	ctx context.Context, ref model.RepoRef, number int,
) (model.Item, []model.Warning) {
	a.stateReads++
	return a.FakeAdapter.ItemState(ctx, ref, number)
}

func (a *mergeRecordingAdapter) Merge(
	_ context.Context, _ model.RepoRef, _ int, _ forge.MergeRequest,
) []model.Warning {
	a.mergeCalled = true
	return []model.Warning{{Forge: a.ForgeName, Kind: "denied", Msg: "the fake does not merge"}}
}

func stateReadingAdapter(it model.Item) *mergeRecordingAdapter {
	return &mergeRecordingAdapter{
		FakeAdapter: testutil.FakeAdapter{
			ForgeName:  it.Forge,
			HostName:   it.Host,
			ItemStates: map[string]model.Item{testutil.ItemKey(it.Ref.Project, it.Number): it},
		},
	}
}
