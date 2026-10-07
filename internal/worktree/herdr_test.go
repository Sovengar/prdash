package worktree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/herdr"
	"prdash/internal/testutil"
)

var errWorkspaceProbe = errors.New("socket closed")

type fakeRunner struct {
	available    bool
	createInfo   herdr.WorktreeInfo
	createErr    error
	createCalls  []herdr.WorktreeSpec
	listResult   []herdr.WorktreeInfo
	removeCalls  []string
	removeErr    error
	workspace    herdr.WorkspaceInfo
	workspaceErr error
	wsCalls      []herdr.WorkspaceSpec
	panes        map[string][]herdr.PaneInfo
	panesErr     map[string]error
	paneListArgs []string
}

func (f *fakeRunner) Available() bool { return f.available }

func (f *fakeRunner) WorktreeCreate(_ context.Context, spec herdr.WorktreeSpec) (herdr.WorktreeInfo, error) {
	f.createCalls = append(f.createCalls, spec)
	return f.createInfo, f.createErr
}

func (f *fakeRunner) WorktreeList(context.Context, string) ([]herdr.WorktreeInfo, error) {
	return f.listResult, nil
}

func (f *fakeRunner) WorktreeRemove(_ context.Context, workspaceID string, _ bool) error {
	f.removeCalls = append(f.removeCalls, workspaceID)
	return f.removeErr
}

func (f *fakeRunner) WorkspaceCreate(_ context.Context, spec herdr.WorkspaceSpec) (herdr.WorkspaceInfo, error) {
	f.wsCalls = append(f.wsCalls, spec)
	return f.workspace, f.workspaceErr
}

func (f *fakeRunner) PaneList(_ context.Context, workspaceID string) ([]herdr.PaneInfo, error) {
	f.paneListArgs = append(f.paneListArgs, workspaceID)
	if err, ok := f.panesErr[workspaceID]; ok {
		return nil, err
	}
	return f.panes[workspaceID], nil
}

func TestSelectPicksNativeInsideHerdr(t *testing.T) {
	if _, ok := Select(&fakeRunner{available: true}, t.TempDir()).(*HerdrNative); !ok {
		t.Fatal("inside Herdr the native provisioning should be chosen")
	}
	if _, ok := Select(&fakeRunner{available: false}, t.TempDir()).(*GitDirect); !ok {
		t.Fatal("outside Herdr plain git should be chosen")
	}
	if _, ok := Select(nil, t.TempDir()).(*GitDirect); !ok {
		t.Fatal("without a Herdr client plain git should be chosen")
	}
}

func TestHerdrNativeCreateMapsContainer(t *testing.T) {
	base := filepath.Join(t.TempDir(), "worktrees")
	dest := filepath.Join(base, "github", "github.com", "acme", "widget", "prdash-pr-7")
	runner := &fakeRunner{
		available: true,
		createInfo: herdr.WorktreeInfo{
			WorkspaceID: "w18", TabID: "w18:t1", RootPaneID: "w18:p1",
			Path: dest, Branch: "prdash/pr-7", Label: "prdash-pr-7",
		},
	}
	h := NewHerdrNative(runner, base)

	wt, err := h.Create(context.Background(), Spec{Repo: "/repo", Branch: "prdash/pr-7", Path: dest, Label: "prdash-pr-7"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if wt.WorkspaceID != "w18" || wt.RootPaneID != "w18:p1" {
		t.Fatalf("container = %+v", wt)
	}
	if wt.Path != dest || wt.Branch != "prdash/pr-7" || wt.Label != "prdash-pr-7" {
		t.Fatalf("worktree = %+v", wt)
	}
	if len(runner.createCalls) != 1 {
		t.Fatalf("createCalls = %+v", runner.createCalls)
	}
	if got := runner.createCalls[0]; !got.NoFocus || got.Cwd != "/repo" || got.Branch != "prdash/pr-7" || got.Path != dest {
		t.Fatalf("native spec = %+v", got)
	}
}

func TestHerdrNativeCreateKeepsOwnershipLabel(t *testing.T) {
	base := t.TempDir()
	dest := filepath.Join(base, "prdash-pr-7")
	runner := &fakeRunner{
		available: true,
		createInfo: herdr.WorktreeInfo{
			WorkspaceID: "w18", RootPaneID: "w18:p1",
			Path: dest, Branch: "prdash/pr-7",
			WorkspaceLabel: "prdash-pr-7", Label: "origin.git",
		},
	}
	wt, err := NewHerdrNative(runner, base).Create(context.Background(), Spec{
		Repo: "/repo", Branch: "prdash/pr-7", Path: dest, Label: "prdash-pr-7",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if wt.Label != "prdash-pr-7" {
		t.Fatalf("Label = %q, want the ownership label", wt.Label)
	}
}

func TestHerdrNativeCreateReusesExistingWorktree(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature")

	base := t.TempDir()
	dest := filepath.Join(base, "prdash-pr-1")
	if _, err := NewGitDirect(base).Create(context.Background(), Spec{Repo: repo, Branch: "feature", Path: dest, Label: "prdash-pr-1"}); err != nil {
		t.Fatalf("preparing a worktree: %v", err)
	}

	runner := &fakeRunner{
		available:  true,
		listResult: []herdr.WorktreeInfo{{Path: dest, Branch: "feature", Label: "origin.git", OpenWorkspaceID: "w19"}},
		panes:      map[string][]herdr.PaneInfo{"w19": {{PaneID: "w19:p1", WorkspaceID: "w19", TabID: "w19:t1"}}},
	}
	h := NewHerdrNative(runner, base)

	wt, err := h.Create(context.Background(), Spec{Repo: repo, Branch: "feature", Path: dest, Label: "prdash-pr-1"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(runner.createCalls) != 0 {
		t.Fatal("an existing worktree should not be created again")
	}
	if wt.WorkspaceID != "w19" || wt.RootPaneID != "w19:p1" {
		t.Fatalf("it should resolve the open workspace: %+v", wt)
	}
	if wt.Label != "prdash-pr-1" {
		t.Fatalf("the ownership label should not be overwritten with the repo name: %q", wt.Label)
	}
	if list := h.List(context.Background()); len(list) != 1 || list[0].Path != dest {
		t.Fatalf("List = %+v", list)
	}
}

// A closed workspace makes worktree list return no open_workspace_id, which detaches the review.
func TestHerdrNativeReuseAdoptsCheckoutWithNoOpenWorkspace(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature")

	base := t.TempDir()
	dest := filepath.Join(base, "prdash-pr-1")
	if _, err := NewGitDirect(base).Create(context.Background(), Spec{Repo: repo, Branch: "feature", Path: dest, Label: "prdash-pr-1"}); err != nil {
		t.Fatalf("preparing a worktree: %v", err)
	}

	runner := &fakeRunner{
		available:  true,
		listResult: []herdr.WorktreeInfo{{Path: dest, Branch: "feature", Label: "origin.git"}},
		workspace:  herdr.WorkspaceInfo{WorkspaceID: "w1C", TabID: "w1C:t1", RootPaneID: "w1C:p1"},
	}

	wt, err := NewHerdrNative(runner, base).Create(context.Background(), Spec{Repo: repo, Branch: "feature", Path: dest, Label: "prdash-pr-1"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(runner.createCalls) != 0 {
		t.Fatal("an existing worktree should not be created again")
	}
	if len(runner.wsCalls) != 1 {
		t.Fatalf("it should open a workspace for the checkout: %+v", runner.wsCalls)
	}
	if got := runner.wsCalls[0]; got.Cwd != dest || !got.NoFocus || got.Label != "prdash-pr-1" {
		t.Fatalf("workspace spec = %+v", got)
	}
	if wt.WorkspaceID != "w1C" || wt.RootPaneID != "w1C:p1" {
		t.Fatalf("container = %+v", wt)
	}
}

func TestHerdrNativeReuseFailsWhenWorkspaceCannotBeOpened(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature")

	base := t.TempDir()
	dest := filepath.Join(base, "prdash-pr-1")
	if _, err := NewGitDirect(base).Create(context.Background(), Spec{Repo: repo, Branch: "feature", Path: dest, Label: "prdash-pr-1"}); err != nil {
		t.Fatalf("preparing a worktree: %v", err)
	}

	runner := &fakeRunner{
		available:    true,
		listResult:   []herdr.WorktreeInfo{{Path: dest, Branch: "feature"}},
		workspaceErr: errWorkspaceProbe,
	}
	_, err := NewHerdrNative(runner, base).Create(context.Background(), Spec{Repo: repo, Branch: "feature", Path: dest, Label: "prdash-pr-1"})
	if err == nil {
		t.Fatal("without a workspace no worktree without a container should be returned")
	}
	if !strings.Contains(err.Error(), dest) {
		t.Fatalf("the error should name the path to clean: %v", err)
	}
}

// Herdr can return an open_workspace_id that no longer exists, pointing at nothing.
func TestHerdrNativeReuseIgnoresStaleOpenWorkspaceId(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature")

	base := t.TempDir()
	dest := filepath.Join(base, "prdash-pr-13")
	if _, err := NewGitDirect(base).Create(context.Background(), Spec{Repo: repo, Branch: "feature", Path: dest, Label: "prdash-pr-13"}); err != nil {
		t.Fatalf("preparing a worktree: %v", err)
	}

	runner := &fakeRunner{
		available:  true,
		listResult: []herdr.WorktreeInfo{{Path: dest, Branch: "feature", OpenWorkspaceID: "w1B"}},
		panesErr:   map[string]error{"w1B": errWorkspaceProbe},
		workspace:  herdr.WorkspaceInfo{WorkspaceID: "w1D", TabID: "w1D:t1", RootPaneID: "w1D:p1"},
	}

	wt, err := NewHerdrNative(runner, base).Create(context.Background(), Spec{Repo: repo, Branch: "feature", Path: dest, Label: "prdash-pr-13"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(runner.paneListArgs) == 0 {
		t.Fatal("an open_workspace_id should be checked before trusting it")
	}
	if len(runner.wsCalls) != 1 || runner.wsCalls[0].Cwd != dest {
		t.Fatalf("a stale workspace should fall back to adoption: %+v", runner.wsCalls)
	}
	if wt.WorkspaceID != "w1D" || wt.RootPaneID != "w1D:p1" {
		t.Fatalf("container = %+v", wt)
	}
}

func TestHerdrNativeReuseUsesRootPaneOfOpenWorkspace(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature")

	base := t.TempDir()
	dest := filepath.Join(base, "prdash-pr-1")
	if _, err := NewGitDirect(base).Create(context.Background(), Spec{Repo: repo, Branch: "feature", Path: dest, Label: "prdash-pr-1"}); err != nil {
		t.Fatalf("preparing a worktree: %v", err)
	}

	runner := &fakeRunner{
		available:  true,
		listResult: []herdr.WorktreeInfo{{Path: dest, Branch: "feature", OpenWorkspaceID: "w19"}},
		panes:      map[string][]herdr.PaneInfo{"w19": {{PaneID: "w19:p1", WorkspaceID: "w19", TabID: "w19:t1"}}},
	}

	wt, err := NewHerdrNative(runner, base).Create(context.Background(), Spec{Repo: repo, Branch: "feature", Path: dest, Label: "prdash-pr-1"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(runner.wsCalls) != 0 {
		t.Fatalf("a live workspace should not open another one: %+v", runner.wsCalls)
	}
	if wt.WorkspaceID != "w19" || wt.RootPaneID != "w19:p1" {
		t.Fatalf("container = %+v", wt)
	}
}

func TestHerdrNativeRemoveUsesWorkspace(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature")
	base := t.TempDir()
	dest := filepath.Join(base, "prdash-pr-1")
	if _, err := NewGitDirect(base).Create(context.Background(), Spec{Repo: repo, Branch: "feature", Path: dest, Label: "prdash-pr-1"}); err != nil {
		t.Fatalf("preparing a worktree: %v", err)
	}

	runner := &fakeRunner{
		available:  true,
		listResult: []herdr.WorktreeInfo{{Path: dest, OpenWorkspaceID: "w21"}},
	}
	h := NewHerdrNative(runner, base)
	if err := h.Remove(context.Background(), dest); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if len(runner.removeCalls) != 1 || runner.removeCalls[0] != "w21" {
		t.Fatalf("removeCalls = %v", runner.removeCalls)
	}
}

func TestHerdrNativeRemoveRefusesPathOutsideBase(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature")
	outside := filepath.Join(t.TempDir(), "prdash-pr-1")
	testutil.RunGit(t, repo, "worktree", "add", "--quiet", outside, "feature")

	runner := &fakeRunner{
		available:  true,
		listResult: []herdr.WorktreeInfo{{Path: outside, OpenWorkspaceID: "w99"}},
	}
	h := NewHerdrNative(runner, t.TempDir())
	if err := h.Remove(context.Background(), outside); err == nil {
		t.Fatal("it should not delete outside the managed root")
	}
	if len(runner.removeCalls) != 0 {
		t.Fatalf("it should not call the native client: %v", runner.removeCalls)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("the worktree outside the root should not be touched: %v", err)
	}
}

func TestHerdrNativeRemoveIfCleanRefusesPathOutsideBase(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature")
	outside := filepath.Join(t.TempDir(), "prdash-pr-1")
	testutil.RunGit(t, repo, "worktree", "add", "--quiet", outside, "feature")

	runner := &fakeRunner{
		available:  true,
		listResult: []herdr.WorktreeInfo{{Path: outside, OpenWorkspaceID: "w99"}},
	}
	h := NewHerdrNative(runner, t.TempDir())
	removed, _, err := h.RemoveIfClean(context.Background(), outside)
	if err == nil || removed {
		t.Fatalf("RemoveIfClean = (%v, _, %v), want refusal", removed, err)
	}
	if len(runner.removeCalls) != 0 {
		t.Fatalf("it should not call the native client: %v", runner.removeCalls)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("the worktree outside the root should not be touched: %v", err)
	}
}

func TestHerdrNativeReuseRejectsBranchMismatch(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature-1")
	testutil.RunGit(t, repo, "branch", "feature-2")

	base := t.TempDir()
	dest := filepath.Join(base, "prdash-pr-1")
	if _, err := NewGitDirect(base).Create(context.Background(), Spec{Repo: repo, Branch: "feature-1", Path: dest, Label: "prdash-pr-1"}); err != nil {
		t.Fatalf("preparing a worktree: %v", err)
	}

	h := NewHerdrNative(&fakeRunner{available: true}, base)
	_, err := h.Create(context.Background(), Spec{Repo: repo, Branch: "feature-2", Path: dest, Label: "prdash-pr-1"})
	if err == nil || !strings.Contains(err.Error(), "feature-1") {
		t.Fatalf("expected an error for an already-present branch, got %v", err)
	}
}

func TestHerdrNativeCreateIncompleteSpecErrors(t *testing.T) {
	h := NewHerdrNative(&fakeRunner{available: true}, t.TempDir())
	if _, err := h.Create(context.Background(), Spec{Repo: "x"}); err == nil {
		t.Fatal("an incomplete spec should fail")
	}
}

func herdrCleanWorktree(t *testing.T) (base, dest string) {
	t.Helper()
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature")
	base = t.TempDir()
	dest = filepath.Join(base, "prdash-pr-1")
	if _, err := NewGitDirect(base).Create(context.Background(), Spec{Repo: repo, Branch: "feature", Path: dest, Label: "prdash-pr-1"}); err != nil {
		t.Fatalf("preparing a worktree: %v", err)
	}
	return base, dest
}

func TestHerdrNativeRemoveIfCleanDeletesViaNativeWorkspace(t *testing.T) {
	base, dest := herdrCleanWorktree(t)
	runner := &fakeRunner{
		available:  true,
		listResult: []herdr.WorktreeInfo{{Path: dest, OpenWorkspaceID: "w21"}},
	}

	removed, reason, err := NewHerdrNative(runner, base).RemoveIfClean(context.Background(), dest)
	if err != nil || !removed || reason != "" {
		t.Fatalf("RemoveIfClean = (%v, %q, %v), want clean deletion", removed, reason, err)
	}
	if len(runner.removeCalls) != 1 || runner.removeCalls[0] != "w21" {
		t.Fatalf("removeCalls = %v, want the native workspace deletion", runner.removeCalls)
	}
}

func TestHerdrNativeRemoveIfCleanKeepsDirty(t *testing.T) {
	base, dest := herdrCleanWorktree(t)
	if err := os.WriteFile(filepath.Join(dest, "dirty.txt"), []byte("uncommitted"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{
		available:  true,
		listResult: []herdr.WorktreeInfo{{Path: dest, OpenWorkspaceID: "w21"}},
	}

	removed, reason, err := NewHerdrNative(runner, base).RemoveIfClean(context.Background(), dest)
	if err != nil || removed || reason != KeptUncommitted {
		t.Fatalf("RemoveIfClean = (%v, %q, %v), want kept for being dirty", removed, reason, err)
	}
	if len(runner.removeCalls) != 0 {
		t.Fatalf("a dirty checkout should not call the native deletion: %v", runner.removeCalls)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("the dirty worktree should not be touched: %v", err)
	}
}

func TestHerdrNativeRemoveIfCleanPropagatesRemoveError(t *testing.T) {
	base, dest := herdrCleanWorktree(t)
	runner := &fakeRunner{
		available:  true,
		listResult: []herdr.WorktreeInfo{{Path: dest, OpenWorkspaceID: "w21"}},
		removeErr:  errors.New("workspace busy"),
	}

	removed, _, err := NewHerdrNative(runner, base).RemoveIfClean(context.Background(), dest)
	if err == nil || removed {
		t.Fatalf("RemoveIfClean = (%v, _, %v), want propagated error", removed, err)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("a deletion failure should not touch the checkout: %v", err)
	}
}

// Exists true but not a worktree is the narrow band that decides between create and reuse.
func TestOnADestinationThatIsNotAWorktreeItDoesNotCreateOnTopNorAdopt(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "prdash-pr-7")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, ".git"),
		[]byte("gitdir: /no/existe\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "trabajo.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	f := &fakeRunner{available: true}
	h := NewHerdrNative(f, t.TempDir())

	wt, err := h.Create(context.Background(), Spec{
		Repo:   filepath.Join(t.TempDir(), "repo"),
		Branch: "feat/x",
		Path:   dest,
		Label:  "prdash-pr-7",
	})
	if err == nil {
		t.Fatalf("a destination that is not a worktree gave nil and a worktree: %+v", wt)
	}
	if len(f.createCalls) != 0 {
		t.Errorf("`worktree create` was called %d times over a destination that already exists",
			len(f.createCalls))
	}
	if _, err := os.Stat(filepath.Join(dest, "trabajo.txt")); err != nil {
		t.Errorf("the user's directory changed: %v", err)
	}
}

// Adopting another branch's checkout would mount the review on code nobody chose.
func TestAdoptingAWorktreeOfAnotherBranchIsRefusedAndSaid(t *testing.T) {
	repo := repoWithBranch(t)
	testutil.RunGit(t, repo, "branch", "other")

	dest := filepath.Join(t.TempDir(), "prdash-pr-7")
	testutil.RunGit(t, repo, "worktree", "add", "--quiet", dest, "other")

	h := NewHerdrNative(&fakeRunner{available: true}, t.TempDir())
	_, err := h.Create(context.Background(), Spec{
		Repo:   repo,
		Branch: "feat/x",
		Path:   dest,
		Label:  "prdash-pr-7",
	})
	if err == nil {
		t.Fatal("it adopted a worktree of another branch")
	}
	for _, want := range []string{"other", "feat/x", dest} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error %q does not say %q", err, want)
		}
	}
}

func TestRemoveOfAWorktreeWithoutAnOpenWorkspaceDelegatesToTheGitScan(t *testing.T) {
	repo := repoWithBranch(t)
	testutil.RunGit(t, repo, "branch", "feat/x")

	base := t.TempDir()
	direct := NewGitDirect(base)
	dest := filepath.Join(base, "prdash-pr-7")
	if _, err := direct.Create(context.Background(), Spec{
		Repo: repo, Branch: "feat/x", Path: dest, Label: "prdash-pr-7",
	}); err != nil {
		t.Fatal(err)
	}

	f := &fakeRunner{available: true, listResult: nil}
	h := NewHerdrNative(f, base)
	if err := h.Remove(context.Background(), dest); err != nil {
		t.Fatalf("Remove without an open workspace failed: %v", err)
	}
	if Exists(dest) {
		t.Error("the worktree is still on disk: it would stay there without anyone knowing")
	}
	if len(f.removeCalls) != 0 {
		t.Errorf("WorktreeRemove was called %d times with no open workspace", len(f.removeCalls))
	}
}

// Create already checked Exists, so this reuse branch is unreachable and must be called directly.
func TestReuseWithAGitThatIsADirectoryPropagatesTheInspectError(t *testing.T) {
	repo := repoWithBranch(t)

	dest := filepath.Join(t.TempDir(), "prdash-pr-7")
	testutil.InitRepo(t, dest)
	testutil.CommitFile(t, dest, "a.txt", "a", "a")

	h := NewHerdrNative(&fakeRunner{available: true}, t.TempDir())
	_, err := h.reuse(context.Background(), Spec{
		Repo: repo, Branch: "feat/x", Path: dest, Label: "prdash-pr-7",
	})
	if err == nil {
		t.Fatal("adopting a normal repo gave nil: the caller would receive a zeroed Worktree")
	}
	if !strings.Contains(err.Error(), "linked worktree") {
		t.Errorf("the error %q is not inspect's: reuse rewrote it and the reason is lost", err)
	}
	if !strings.Contains(err.Error(), dest) {
		t.Errorf("the error %q does not name the occupied path", err)
	}
}

// git worktree lock corrupts the record without the repo being gone: a locked entry whose directory vanishes.
func TestALockedWorktreeIsPrunedAndItsResidueDeletedWithoutTouchingTheRest(t *testing.T) {
	repo := repoWithBranch(t, "feat/x", "feat/y")

	base := t.TempDir()
	direct := NewGitDirect(base)
	ours := filepath.Join(base, "prdash-pr-7")
	if _, err := direct.Create(context.Background(), Spec{
		Repo: repo, Branch: "feat/x", Path: ours, Label: "prdash-pr-7",
	}); err != nil {
		t.Fatal(err)
	}

	testutil.RunGit(t, repo, "worktree", "lock", ours)

	foreign := filepath.Join(base, "mio")
	testutil.RunGit(t, repo, "worktree", "add", "--quiet", foreign, "feat/y")

	if err := direct.Remove(context.Background(), ours); err != nil {
		t.Fatalf("removing a locked worktree failed: %v", err)
	}

	if Exists(ours) {
		t.Errorf("%s is still on disk after a Remove that did succeed", ours)
	}
	// A locked entry stays in the record even when its directory is gone, and prune does not touch it.
	var liveEntry string
	for _, line := range strings.Split(
		testutil.RunGit(t, repo, "worktree", "list"), "\n") {
		if strings.Contains(line, "prdash-pr-7") {
			liveEntry = line
		}
	}
	if liveEntry == "" {
		t.Error("the locked worktree's entry disappeared from the record: the lock does not " +
			"prevent pruning, so something else is removing it")
	}
	if !strings.Contains(liveEntry, "locked") {
		t.Errorf("the entry that remains is not marked as locked: %q. Without that mark the "+
			"pruning would have removed it and the finding would be another one", liveEntry)
	}
	if _, err := os.Stat(ours); !os.IsNotExist(err) {
		t.Errorf("the directory %s appeared again: the entry that remains says it is there", ours)
	}

	if !Exists(foreign) {
		t.Error("the Remove took a foreign worktree that is not prdash's")
	}
	if _, err := os.Stat(filepath.Join(foreign, ".git")); err != nil {
		t.Errorf("the foreign worktree lost its .git: %v", err)
	}
}

// .git is a file (Exists true) but git cannot read the branch from it.
func TestReuseOnADestinationThatNoLongerHasGitRefusesAndDoesNotInventAWorktree(t *testing.T) {
	repo := repoWithBranch(t)
	dest := filepath.Join(t.TempDir(), "prdash-pr-7")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, ".git"),
		[]byte("gitdir: /no/existe\\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !Exists(dest) {
		t.Fatal("the fixture is no good: Exists has to give true to reach reuse")
	}

	h := NewHerdrNative(&fakeRunner{available: true}, t.TempDir())
	wt, err := h.reuse(context.Background(), Spec{
		Repo: repo, Branch: "feat/x", Path: dest, Label: "prdash-pr-7",
	})
	if err == nil {
		t.Fatalf("adopting an orphan gave nil and a worktree: %+v. With an empty branch the "+
			"item's review would mount on a checkout that is not its own", wt)
	}
	if !strings.Contains(err.Error(), dest) {
		t.Errorf("the error %q does not name the destination that is not a worktree", err)
	}
}

// reuse's guards are the negation of Create's checks, so the test jumps over Create to reach them.
func TestReuseOnADestinationWithoutGitRefuses(t *testing.T) {
	repo := repoWithBranch(t)
	dest := filepath.Join(t.TempDir(), "prdash-pr-7")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}

	h := NewHerdrNative(&fakeRunner{available: true}, t.TempDir())
	wt, err := h.reuse(context.Background(), Spec{
		Repo: repo, Branch: "feat/x", Path: dest, Label: "prdash-pr-7",
	})
	if err == nil {
		t.Fatalf("adopting a directory without .git gave nil and a worktree: %+v", wt)
	}
	if !strings.Contains(err.Error(), "linked worktree") {
		t.Errorf("the error %q does not say the destination is not a linked worktree", err)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Errorf("reuse removed the directory: %v. The guard refuses, it does not clean", err)
	}
}
