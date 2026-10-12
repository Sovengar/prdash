package executor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/forge/model"
	"prdash/internal/herdr"
	"prdash/internal/reporesolver"
	"prdash/internal/review/plan"
	"prdash/internal/testutil"
	"prdash/internal/worktree"
)

func githubRef() model.RepoRef {
	return model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget", Owner: "acme", Name: "widget"}
}

func item(ref model.RepoRef, number int) model.Item {
	it := model.NewItem(ref, number)
	it.URL = fmt.Sprintf("https://github.com/%s/pull/%d", ref.Project, number)
	return it
}

func baseFixture(t *testing.T) (origin, repo string) {
	t.Helper()
	origin = filepath.Join(t.TempDir(), "origin.git")
	testutil.InitBare(t, origin)
	repo = filepath.Join(t.TempDir(), "repo")
	testutil.InitRepo(t, repo)
	testutil.CommitFile(t, repo, "base.txt", "base", "base")
	testutil.SetRemote(t, repo, "origin", origin)
	testutil.Push(t, repo, "-u", "origin", "main")
	return origin, repo
}

func pushPR(t *testing.T, origin string, number int, content string) {
	t.Helper()
	work := filepath.Join(t.TempDir(), "pr")
	testutil.InitRepo(t, work)
	testutil.CommitFile(t, work, fmt.Sprintf("pr-%d.txt", number), content, "pr")
	testutil.Push(t, work, origin, fmt.Sprintf("HEAD:refs/pull/%d/head", number))
}

type harness struct {
	origin   string
	ref      model.RepoRef
	cloneDir string
	wtDir    string
	resolver *reporesolver.Resolver
	pr       *worktree.GitDirect
	ex       *Executor
}

type harnessOpts struct {
	origin   string
	roots    []string
	cloneURL func(model.RepoRef) string
	herdr    HerdrPort
}

func newHarness(t *testing.T, opts harnessOpts) *harness {
	t.Helper()
	ref := githubRef()
	cloneDir := filepath.Join(t.TempDir(), "repos")
	wtDir := filepath.Join(t.TempDir(), "worktrees")
	memoPath := filepath.Join(t.TempDir(), "memo.json")

	cloneURL := opts.cloneURL
	if cloneURL == nil {
		origin := opts.origin
		cloneURL = func(model.RepoRef) string { return origin }
	}
	resolver := reporesolver.New(reporesolver.Options{
		Roots:       opts.roots,
		CloneDir:    cloneDir,
		WorktreeDir: wtDir,
		MemoPath:    memoPath,
		CloneURL:    cloneURL,
		ParseRemote: func(raw string) (model.RepoRef, bool) {
			if strings.TrimSpace(raw) == opts.origin {
				return ref, true
			}
			return model.RepoRef{}, false
		},
	})
	pr := worktree.NewGitDirect(wtDir)
	ex := &Executor{
		Resolver:  resolver,
		Worktrees: pr,
		Herdr:     opts.herdr,
		Tools: plan.Tools{
			Tuicr: plan.Tool{Argv: []string{"tuicr"}},
			Hunk:  plan.Tool{Argv: []string{"hunk"}},
			Agent: plan.Tool{Argv: []string{"opencode"}},
		},
	}
	return &harness{origin: opts.origin, ref: ref, cloneDir: cloneDir, wtDir: wtDir, resolver: resolver, pr: pr, ex: ex}
}

func TestMountFromLocalRepoRegistersActiveReview(t *testing.T) {
	origin, repo := baseFixture(t)
	pushPR(t, origin, 7, "one")
	h := newHarness(t, harnessOpts{origin: origin, roots: []string{filepath.Dir(repo)}})
	it := item(h.ref, 7)

	res, err := h.ex.Mount(context.Background(), it)
	if err != nil {
		t.Fatalf("Mount: %v", err)
	}
	if res.RepoPath != repo {
		t.Fatalf("RepoPath = %q, want the local clone %q", res.RepoPath, repo)
	}
	if res.Reused {
		t.Fatal("the first mount does not reuse")
	}
	if !worktree.Exists(res.Worktree.Path) {
		t.Fatalf("the worktree does not exist at %q", res.Worktree.Path)
	}
	if res.Branch != "prdash/pr-7" {
		t.Fatalf("branch = %q", res.Branch)
	}
	if got := testutil.RunGit(t, res.Worktree.Path, "rev-parse", "--abbrev-ref", "HEAD"); got != "prdash/pr-7" {
		t.Fatalf("worktree HEAD = %q", got)
	}
	if _, ok := h.ex.ActiveReview(it); !ok {
		t.Fatal("the review should end up registered as active")
	}
}

func TestMountClonesBareWhenRepoNotLocal(t *testing.T) {
	origin, _ := baseFixture(t)
	pushPR(t, origin, 8, "eight")
	h := newHarness(t, harnessOpts{origin: origin}) // no roots: no local clone
	it := item(h.ref, 8)

	res, err := h.ex.Mount(context.Background(), it)
	if err != nil {
		t.Fatalf("Mount: %v", err)
	}
	wantBare := filepath.Join(h.cloneDir, "github", "github.com", "acme", "widget")
	if res.RepoPath != wantBare {
		t.Fatalf("RepoPath = %q, want %q", res.RepoPath, wantBare)
	}
	if got := testutil.RunGit(t, wantBare, "rev-parse", "--is-bare-repository"); got != "true" {
		t.Fatalf("the clone is not bare: %q", got)
	}
	if !worktree.Exists(res.Worktree.Path) {
		t.Fatal("the worktree from the bare clone is missing")
	}
	if !strings.HasPrefix(res.Worktree.Path, h.wtDir) {
		t.Fatalf("the worktree destination must be configurable: %q", res.Worktree.Path)
	}
}

func TestMountForkFetchesReviewRef(t *testing.T) {
	origin, repo := baseFixture(t)
	pushPR(t, origin, 9, "from the fork")
	if testutil.RefExists(t, origin, "refs/heads/feature") {
		t.Fatal("the fork branch should not exist as a normal branch of origin")
	}
	h := newHarness(t, harnessOpts{origin: origin, roots: []string{filepath.Dir(repo)}})
	it := item(h.ref, 9)

	res, err := h.ex.Mount(context.Background(), it)
	if err != nil {
		t.Fatalf("Mount: %v", err)
	}
	if res.Branch != "prdash/pr-9" {
		t.Fatalf("local fork branch = %q", res.Branch)
	}
	if _, err := os.Stat(filepath.Join(res.Worktree.Path, "pr-9.txt")); err != nil {
		t.Fatalf("the worktree does not bring the fork ref's content: %v", err)
	}
	if testutil.RefExists(t, origin, "refs/heads/feature") {
		t.Fatal("no branch of origin should have been created")
	}
}

func TestMountReusesExistingWorktree(t *testing.T) {
	origin, repo := baseFixture(t)
	pushPR(t, origin, 7, "one")
	h := newHarness(t, harnessOpts{origin: origin, roots: []string{filepath.Dir(repo)}})
	it := item(h.ref, 7)
	ctx := context.Background()

	first, err := h.ex.Mount(ctx, it)
	if err != nil {
		t.Fatalf("Mount 1: %v", err)
	}
	second, err := h.ex.Mount(ctx, it)
	if err != nil {
		t.Fatalf("Mount 2: %v", err)
	}
	if !second.Reused {
		t.Fatal("the second mount should reuse")
	}
	if first.Worktree.Path != second.Worktree.Path {
		t.Fatalf("different paths: %q vs %q", first.Worktree.Path, second.Worktree.Path)
	}
	if list := h.pr.List(ctx); len(list) != 1 {
		t.Fatalf("it should not duplicate the worktree: %+v", list)
	}
}

func TestMountTwoPRsSameRepoCoexist(t *testing.T) {
	origin, repo := baseFixture(t)
	pushPR(t, origin, 1, "one")
	pushPR(t, origin, 2, "two")
	h := newHarness(t, harnessOpts{origin: origin, roots: []string{filepath.Dir(repo)}})
	ctx := context.Background()

	r1, err := h.ex.Mount(ctx, item(h.ref, 1))
	if err != nil {
		t.Fatalf("Mount 1: %v", err)
	}
	r2, err := h.ex.Mount(ctx, item(h.ref, 2))
	if err != nil {
		t.Fatalf("Mount 2: %v", err)
	}
	if r1.Worktree.Path == r2.Worktree.Path {
		t.Fatal("each item must have its own worktree at a different path")
	}
	if !worktree.Exists(r1.Worktree.Path) || !worktree.Exists(r2.Worktree.Path) {
		t.Fatal("both worktrees must coexist")
	}
	if list := h.pr.List(ctx); len(list) != 2 {
		t.Fatalf("List = %+v, want 2", list)
	}
}

func TestMountCloneFailureLeavesNoGarbage(t *testing.T) {
	h := newHarness(t, harnessOpts{
		origin: filepath.Join(t.TempDir(), "privado.git"), // does not exist / no access
		cloneURL: func(model.RepoRef) string {
			return filepath.Join(t.TempDir(), "without-permission.git")
		},
	})
	it := item(h.ref, 5)

	_, err := h.ex.Mount(context.Background(), it)
	if err == nil {
		t.Fatal("expected a clear cloning error")
	}
	bare := filepath.Join(h.cloneDir, "github", "github.com", "acme", "widget")
	if _, statErr := os.Stat(bare); !os.IsNotExist(statErr) {
		t.Fatalf("no bare clone should remain: %v", statErr)
	}
	if leftoverTemps(h.cloneDir) != 0 {
		t.Fatalf("clone temps left in %s", h.cloneDir)
	}
	if list := h.pr.List(context.Background()); len(list) != 0 {
		t.Fatalf("no worktree should remain: %+v", list)
	}
}

func TestMountFetchFailureCleansNewBare(t *testing.T) {
	origin, _ := baseFixture(t) // origin without review refs
	h := newHarness(t, harnessOpts{origin: origin})
	it := item(h.ref, 42)

	_, err := h.ex.Mount(context.Background(), it)
	if err == nil {
		t.Fatal("expected an error fetching the review ref")
	}
	bare := filepath.Join(h.cloneDir, "github", "github.com", "acme", "widget")
	if _, statErr := os.Stat(bare); !os.IsNotExist(statErr) {
		t.Fatalf("a freshly created bare clone must be cleaned up if the fetch fails: %v", statErr)
	}
	if list := h.pr.List(context.Background()); len(list) != 0 {
		t.Fatalf("no worktree should remain: %+v", list)
	}
}

func TestMountWithoutHerdrStillProvisionsWorktree(t *testing.T) {
	origin, repo := baseFixture(t)
	pushPR(t, origin, 3, "three")
	h := newHarness(t, harnessOpts{origin: origin, roots: []string{filepath.Dir(repo)}})
	h.ex.Env = plan.Env{Available: map[string]bool{"tuicr": true, "agent": true}}

	res, err := h.ex.Mount(context.Background(), item(h.ref, 3))
	if err != nil {
		t.Fatalf("Mount: %v", err)
	}
	if res.Herdr {
		t.Fatal("without Herdr no mounted layout should be reported")
	}
	if !worktree.Exists(res.Worktree.Path) {
		t.Fatal("the worktree must be mounted even without Herdr")
	}
	if res.Plan.PaneCount() != 3 {
		t.Fatalf("the missing Hunk pane should be omitted: %+v", res.Plan)
	}
}

func TestMountWithHerdrAppliesPlan(t *testing.T) {
	origin, repo := baseFixture(t)
	pushPR(t, origin, 4, "four")
	herdr := &fakeHerdr{available: true}
	h := newHarness(t, harnessOpts{origin: origin, roots: []string{filepath.Dir(repo)}, herdr: herdr})

	res, err := h.ex.Mount(context.Background(), item(h.ref, 4))
	if err != nil {
		t.Fatalf("Mount: %v", err)
	}
	if !res.Herdr || !herdr.mounted {
		t.Fatalf("Herdr should have mounted the plan: %+v", herdr)
	}
	if herdr.plan.PaneCount() != 4 {
		t.Fatalf("the mounted plan = %+v", herdr.plan)
	}
	if len(herdr.notified) == 0 {
		t.Fatal("a mounted layout should notify")
	}
}

func TestMountPassesNativeContainerToLayout(t *testing.T) {
	origin, repo := baseFixture(t)
	pushPR(t, origin, 6, "six")
	herdr := &fakeHerdr{available: true}
	h := newHarness(t, harnessOpts{origin: origin, roots: []string{filepath.Dir(repo)}, herdr: herdr})
	h.ex.Worktrees = &fakeProvisioner{wt: worktree.Worktree{
		ID: "wt", Label: "prdash-pr-6", Path: "/tmp/wt-prdash-pr-6",
		Branch: "prdash/pr-6", Repo: repo, WorkspaceID: "w30", RootPaneID: "w30:p1",
	}}

	if _, err := h.ex.Mount(context.Background(), item(h.ref, 6)); err != nil {
		t.Fatalf("Mount: %v", err)
	}
	if herdr.container.WorkspaceID != "w30" || herdr.container.PaneID != "w30:p1" {
		t.Fatalf("container = %+v", herdr.container)
	}
}

type fakeProvisioner struct {
	wt worktree.Worktree

	removeIfCleanRemoved bool
	removeIfCleanReason  string
	removeIfCleanErr     error
	removeIfCleanID      string
}

func (f *fakeProvisioner) Create(context.Context, worktree.Spec) (worktree.Worktree, error) {
	return f.wt, nil
}
func (f *fakeProvisioner) Remove(context.Context, string) error { return nil }
func (f *fakeProvisioner) RemoveIfClean(_ context.Context, id string) (bool, string, error) {
	f.removeIfCleanID = id
	return f.removeIfCleanRemoved, f.removeIfCleanReason, f.removeIfCleanErr
}
func (f *fakeProvisioner) List(context.Context) []worktree.Worktree { return nil }
func (f *fakeProvisioner) Audit(context.Context) []worktree.Entry   { return nil }

type fakeHerdr struct {
	available bool
	mounted   bool
	container herdr.Container
	plan      plan.Plan
	notified  []string
}

func (f *fakeHerdr) Available() bool { return f.available }

func (f *fakeHerdr) MountLayout(_ context.Context, c herdr.Container, pl plan.Plan) ([]string, error) {
	f.mounted = true
	f.container = c
	f.plan = pl
	return nil, nil
}

func (f *fakeHerdr) Notify(_ context.Context, title string, _ herdr.NotifyOptions) error {
	f.notified = append(f.notified, title)
	return nil
}

func leftoverTemps(dir string) int {
	n := 0
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.Contains(d.Name(), ".tmp-") {
			n++
		}
		return nil
	})
	return n
}

func mountReview(t *testing.T, number int) (*harness, model.Item) {
	t.Helper()
	origin, repo := baseFixture(t)
	pushPR(t, origin, number, "content")
	h := newHarness(t, harnessOpts{origin: origin, roots: []string{filepath.Dir(repo)}})
	it := item(h.ref, number)
	if _, err := h.ex.Mount(context.Background(), it); err != nil {
		t.Fatalf("Mount: %v", err)
	}
	return h, it
}

func TestCleanupAfterMergeRemovesCleanAndForgets(t *testing.T) {
	h, it := mountReview(t, 5)
	fake := &fakeProvisioner{removeIfCleanRemoved: true}
	h.ex.Worktrees = fake

	res, err := h.ex.CleanupAfterMerge(context.Background(), it)
	if err != nil || !res.WorktreeRemoved || res.WorktreeReason != "" {
		t.Fatalf("CleanupAfterMerge = %+v, %v; want the worktree deleted with no reason", res, err)
	}
	if want := h.resolver.WorktreePath(it.Ref, it.Number); fake.removeIfCleanID != want {
		t.Fatalf("RemoveIfClean received %q, want %q", fake.removeIfCleanID, want)
	}
	if _, ok := h.ex.ActiveReview(it); ok {
		t.Fatal("a real deletion should forget the review record")
	}
}

func TestCleanupAfterMergeKeepsAndDoesNotForget(t *testing.T) {
	h, it := mountReview(t, 5)
	h.ex.Worktrees = &fakeProvisioner{removeIfCleanReason: worktree.KeptUncommitted}

	res, err := h.ex.CleanupAfterMerge(context.Background(), it)
	if err != nil || res.WorktreeRemoved || res.WorktreeReason != worktree.KeptUncommitted {
		t.Fatalf("CleanupAfterMerge = %+v, %v; want kept for being dirty", res, err)
	}
	if _, ok := h.ex.ActiveReview(it); !ok {
		t.Fatal("keeping the worktree must not forget the record")
	}
}

func TestCleanupAfterMergeWithoutActiveReviewIsNoop(t *testing.T) {
	origin, repo := baseFixture(t)
	h := newHarness(t, harnessOpts{origin: origin, roots: []string{filepath.Dir(repo)}})
	fake := &fakeProvisioner{removeIfCleanRemoved: true}
	h.ex.Worktrees = fake

	res, err := h.ex.CleanupAfterMerge(context.Background(), item(h.ref, 5))
	if err != nil || res.WorktreeRemoved || res.WorktreeReason != "" || len(res.Deleted) != 0 || len(res.Warnings) != 0 {
		t.Fatalf("CleanupAfterMerge = %+v, %v; want no-op", res, err)
	}
	if fake.removeIfCleanID != "" {
		t.Fatal("with no mounted review the provisioner should not be called")
	}
}

func TestCleanupAfterMergePropagatesError(t *testing.T) {
	h, it := mountReview(t, 5)
	h.ex.Worktrees = &fakeProvisioner{removeIfCleanErr: errors.New("boom")}

	res, err := h.ex.CleanupAfterMerge(context.Background(), it)
	if err == nil || res.WorktreeRemoved {
		t.Fatalf("CleanupAfterMerge = (%+v, %v), want propagated error", res, err)
	}
}

func TestCleanupAfterMergeDeletesReviewAndLocalBranches(t *testing.T) {
	origin, repo := baseFixture(t)
	pushPR(t, origin, 7, "seven")
	testutil.RunGit(t, repo, "branch", "feat/x")
	h := newHarness(t, harnessOpts{origin: origin, roots: []string{filepath.Dir(repo)}})
	it := item(h.ref, 7)
	it.SourceBranch = "feat/x"
	ctx := context.Background()
	if _, err := h.ex.Mount(ctx, it); err != nil {
		t.Fatalf("Mount: %v", err)
	}

	res, err := h.ex.CleanupAfterMerge(ctx, it)
	if err != nil {
		t.Fatalf("CleanupAfterMerge: %v", err)
	}
	if !res.WorktreeRemoved {
		t.Fatalf("the worktree should be removed: %+v", res)
	}
	if testutil.RefExists(t, repo, "refs/heads/prdash/pr-7") {
		t.Error("the review branch survived in the local clone")
	}
	if testutil.RefExists(t, repo, "refs/prdash/github/7") {
		t.Error("the tracking ref survived in the local clone")
	}
	if testutil.RefExists(t, repo, "refs/heads/feat/x") {
		t.Error("the PR source branch survived in the local clone")
	}
	if got := strings.Join(res.Deleted, ","); got != "review branch,local branch" {
		t.Errorf("Deleted = %q, want both branches", got)
	}
	if len(res.Warnings) != 0 {
		t.Errorf("no warning expected, got %v", res.Warnings)
	}
}

func TestCleanupAfterMergeDeletesReviewBranchFromTheBareClone(t *testing.T) {
	origin, _ := baseFixture(t)
	pushPR(t, origin, 8, "eight")
	h := newHarness(t, harnessOpts{origin: origin}) // no roots: the bare clone is used
	it := item(h.ref, 8)
	ctx := context.Background()
	res, err := h.ex.Mount(ctx, it)
	if err != nil {
		t.Fatalf("Mount: %v", err)
	}

	cleaned, err := h.ex.CleanupAfterMerge(ctx, it)
	if err != nil {
		t.Fatalf("CleanupAfterMerge: %v", err)
	}
	if testutil.RefExists(t, res.RepoPath, "refs/heads/prdash/pr-8") {
		t.Error("the review branch survived in the bare clone")
	}
	if testutil.RefExists(t, res.RepoPath, "refs/prdash/github/8") {
		t.Error("the tracking ref survived in the bare clone")
	}
	if len(cleaned.Warnings) != 0 {
		t.Errorf("no warning expected, got %v", cleaned.Warnings)
	}
}

// An already-absent worktree holds nothing, so the branch is still fair game.
func TestCleanupAfterMergeDeletesTheBranchWhenTheWorktreeIsAlreadyGone(t *testing.T) {
	origin, repo := baseFixture(t)
	pushPR(t, origin, 7, "seven")
	h := newHarness(t, harnessOpts{origin: origin, roots: []string{filepath.Dir(repo)}})
	it := item(h.ref, 7)
	ctx := context.Background()
	res, err := h.ex.Mount(ctx, it)
	if err != nil {
		t.Fatalf("Mount: %v", err)
	}
	if err := h.pr.Remove(ctx, res.Worktree.Path); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	cleaned, err := h.ex.CleanupAfterMerge(ctx, it)
	if err != nil {
		t.Fatalf("CleanupAfterMerge: %v", err)
	}
	if cleaned.WorktreeRemoved {
		t.Error("the worktree was already gone; it should not claim to have removed it")
	}
	if testutil.RefExists(t, repo, "refs/heads/prdash/pr-7") {
		t.Error("the review branch survived despite nothing holding it")
	}
	if len(cleaned.Deleted) == 0 {
		t.Errorf("the deletion should be reported: %+v", cleaned)
	}
}

// The worktree is deleted first on purpose: a branch a worktree holds cannot be
// deleted, and pretending otherwise would leave it behind silently.
func TestCleanupAfterMergeKeepsTheBranchWhenTheWorktreeStaysDirty(t *testing.T) {
	origin, repo := baseFixture(t)
	pushPR(t, origin, 7, "seven")
	h := newHarness(t, harnessOpts{origin: origin, roots: []string{filepath.Dir(repo)}})
	it := item(h.ref, 7)
	ctx := context.Background()
	res, err := h.ex.Mount(ctx, it)
	if err != nil {
		t.Fatalf("Mount: %v", err)
	}
	if err := os.WriteFile(filepath.Join(res.Worktree.Path, "dirty.txt"), []byte("wip"), 0o644); err != nil {
		t.Fatal(err)
	}

	cleaned, err := h.ex.CleanupAfterMerge(ctx, it)
	if err != nil {
		t.Fatalf("CleanupAfterMerge: %v", err)
	}
	if cleaned.WorktreeRemoved || cleaned.WorktreeReason != worktree.KeptUncommitted {
		t.Fatalf("the dirty worktree should be kept: %+v", cleaned)
	}
	if !testutil.RefExists(t, repo, "refs/heads/prdash/pr-7") {
		t.Error("the review branch was deleted while its worktree still held it")
	}
	if len(cleaned.Deleted) != 0 {
		t.Errorf("nothing should be reported deleted, got %v", cleaned.Deleted)
	}
}

// A branch git refuses to delete is a warning on the merge, never a failure: the
// merge already happened and the cleanup cannot undo it. A lock next to the ref
// is what makes git refuse without a broken git.
func TestCleanupAfterMergeWarnsWhenTheLocalBranchCannotBeDeleted(t *testing.T) {
	origin, repo := baseFixture(t)
	pushPR(t, origin, 7, "seven")
	testutil.RunGit(t, repo, "branch", "feat/x")
	lockRef(t, repo, "refs/heads/feat/x")
	h := newHarness(t, harnessOpts{origin: origin, roots: []string{filepath.Dir(repo)}})
	it := item(h.ref, 7)
	it.SourceBranch = "feat/x"
	ctx := context.Background()
	if _, err := h.ex.Mount(ctx, it); err != nil {
		t.Fatalf("Mount: %v", err)
	}

	res, err := h.ex.CleanupAfterMerge(ctx, it)
	if err != nil {
		t.Fatalf("CleanupAfterMerge: %v", err)
	}
	if !res.WorktreeRemoved {
		t.Fatalf("a branch refusal must not undo the worktree removal: %+v", res)
	}
	found := false
	for _, w := range res.Warnings {
		if strings.Contains(w, "feat/x") {
			found = true
		}
	}
	if !found {
		t.Fatalf("warnings = %v, want the local branch refusal", res.Warnings)
	}
}

func lockRef(t *testing.T, repo, ref string) {
	t.Helper()
	path := filepath.Join(repo, ".git", filepath.FromSlash(ref)+".lock")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
}
