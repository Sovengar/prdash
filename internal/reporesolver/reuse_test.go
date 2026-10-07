package reporesolver

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

// The memo holds resolved clones, so an empty path is not one of them.
func TestRememberingAnEmptyPathDoesNotDirtyTheMemo(t *testing.T) {
	memoPath := t.TempDir() + "/memo.json"
	r := New(Options{MemoPath: memoPath})
	ref := model.RepoRef{Forge: "github", Host: "github.com", Project: "o/r", Owner: "o", Name: "r"}

	r.Remember(ref, "")
	if _, ok := r.store.Route(repoKey(ref)); ok {
		t.Error("an empty route was saved in the memo: a resolver would believe it is valid")
	}

	const route = "/clones/o/r"
	r.Remember(ref, route)
	if got, ok := r.store.Route(repoKey(ref)); !ok || got != route {
		t.Errorf("the route was not saved: (%q, %v)", got, ok)
	}

	other := model.RepoRef{Forge: "github", Host: "github.com",
		Project: "group/sub/proj", Owner: "group", Name: "proj"}
	r.Remember(other, "/clones/group/sub/proj")
	if got, _ := r.store.Route(repoKey(ref)); got != route {
		t.Errorf("saving one project overwrote the other: %q", got)
	}

	// It SURVIVES a new resolver over the same file, or the second execution would rebuild everything.
	fresh := New(Options{MemoPath: memoPath})
	if got, ok := fresh.store.Route(repoKey(ref)); !ok || got != route {
		t.Errorf("the route was not recovered from a new resolver: (%q, %v)", got, ok)
	}
}

// A truncated history: a review ref is ONE line of history, not two branches.
func TestTheReviewBranchIsReusedAndNotRecreated(t *testing.T) {
	origin, repo := fixture(t)
	const content = "first version"
	work := filepath.Join(t.TempDir(), "pr")
	testutil.InitRepo(t, work)
	testutil.SetRemote(t, work, "origin", origin)
	testutil.CommitFile(t, work, "pr.txt", content, "revision 1")
	testutil.Push(t, work, origin, "HEAD:"+reviewRefOf(it0(model.NewItem(ghRef(), 7))))
	ref := ghRef()
	r := newResolver(t, origin, ref)
	it := model.NewItem(ref, 7)
	it.Number = 7
	ctx := context.Background()

	branch, err := r.FetchReviewRef(ctx, repo, it)
	if err != nil {
		t.Fatalf("the first FetchReviewRef: %v", err)
	}
	if branch == "" {
		t.Fatal("FetchReviewRef returned an empty branch")
	}
	head := testutil.RunGit(t, repo, "rev-parse", branch)
	expected := testutil.RunGit(t, repo, "rev-parse", "refs/prdash/github/7")
	if head != expected {
		t.Errorf("the branch points at %s and the ref at %s", head, expected)
	}

	// The branch is NOT recreated: recreating it would lose uncommitted work.
	before := testutil.RunGit(t, repo, "rev-parse", branch)
	again, err := r.FetchReviewRef(ctx, repo, it)
	if err != nil {
		t.Fatalf("the second FetchReviewRef: %v", err)
	}
	if again != branch {
		t.Errorf("the second time gave branch %q, want the same %q: recreating it loses the "+
			"pointer to the commit", again, branch)
	}
	if after := testutil.RunGit(t, repo, "rev-parse", branch); after != before {
		t.Errorf("the branch moved from %s to %s between calls with the same ref", before, after)
	}

	testutil.CommitFile(t, work, "pr.txt", content+" and something else", "revision 2")
	testutil.Push(t, work, origin, "HEAD:"+reviewRefOf(it0(model.NewItem(ghRef(), 7))))
	if _, err := r.FetchReviewRef(ctx, repo, it); err != nil {
		t.Fatal(err)
	}
	tracking := testutil.RunGit(t, repo, "rev-parse", "refs/prdash/github/7")
	remoteRef := testutil.RunGit(t, origin, "rev-parse", reviewRefOf(it0(model.NewItem(ghRef(), 7))))
	if tracking != remoteRef {
		t.Errorf("the tracking ref is at %s and the remote at %s: the fetch did not move it",
			tracking, remoteRef)
	}
	if tracking == before {
		t.Fatal("the fixture republished nothing: the tracking ref should have changed")
	}

	// The local branch stays where it was, and that is not an oversight.
	if after := testutil.RunGit(t, repo, "rev-parse", branch); after != before {
		t.Errorf("the local branch moved from %s to %s with the republished PR: it is in use "+
			"by the review's worktree", before, after)
	}
}

// Failing rather than pretending: there is no review ref without a forge to ask.
func TestAForgeWithNoKnownReviewRefDoesNotAcceptTheRequest(t *testing.T) {
	origin, repo := fixture(t)
	r := newResolver(t, origin, ghRef())

	it := model.NewItem(ghRef(), 7)
	it.Forge = "bitbucket"

	_, err := r.FetchReviewRef(context.Background(), repo, it)
	if err == nil {
		t.Fatal("a forge with no review ref gave nil: it would invent a ref and mount the base branch")
	}
	if !strings.Contains(err.Error(), "bitbucket") {
		t.Errorf("the error %q does not name the forge", err)
	}
	if !strings.Contains(err.Error(), "review ref") {
		t.Errorf("the error %q does not say the review ref is missing", err)
	}
}

func it0(m model.Item) model.Item { return m }

func reviewRefOf(it model.Item) string {
	ref, ok := ReviewRef(it)
	if !ok {
		return "main"
	}
	return ref
}
