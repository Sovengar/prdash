package worktree

import (
	"context"
	"path/filepath"
	"testing"

	"prdash/internal/herdr"
	"prdash/internal/testutil"
)

// fakeRunner es un doble en memoria del subconjunto de Herdr que usa la
// provisión nativa. No toca ningún proceso.
type fakeRunner struct {
	available   bool
	createInfo  herdr.WorktreeInfo
	createErr   error
	createCalls []herdr.WorktreeSpec
	listResult  []herdr.WorktreeInfo
	removeCalls []string
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
	return nil
}

func TestSelectPicksNativeInsideHerdr(t *testing.T) {
	if _, ok := Select(&fakeRunner{available: true}, t.TempDir()).(*HerdrNative); !ok {
		t.Fatal("dentro de Herdr debería elegirse la provisión nativa")
	}
	if _, ok := Select(&fakeRunner{available: false}, t.TempDir()).(*GitDirect); !ok {
		t.Fatal("fuera de Herdr debería elegirse git directo")
	}
	if _, ok := Select(nil, t.TempDir()).(*GitDirect); !ok {
		t.Fatal("sin cliente de Herdr debería elegirse git directo")
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
		t.Fatalf("contenedor = %+v", wt)
	}
	if wt.Path != dest || wt.Branch != "prdash/pr-7" || wt.Label != "prdash-pr-7" {
		t.Fatalf("worktree = %+v", wt)
	}
	if len(runner.createCalls) != 1 {
		t.Fatalf("createCalls = %+v", runner.createCalls)
	}
	if got := runner.createCalls[0]; !got.NoFocus || got.Cwd != "/repo" || got.Branch != "prdash/pr-7" || got.Path != dest {
		t.Fatalf("spec nativo = %+v", got)
	}
}

func TestHerdrNativeCreateReusesExistingWorktree(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature")

	base := t.TempDir()
	dest := filepath.Join(base, "prdash-pr-1")
	if _, err := NewGitDirect(base).Create(context.Background(), Spec{Repo: repo, Branch: "feature", Path: dest, Label: "prdash-pr-1"}); err != nil {
		t.Fatalf("preparar worktree: %v", err)
	}

	runner := &fakeRunner{
		available:  true,
		listResult: []herdr.WorktreeInfo{{Path: dest, Branch: "feature", Label: "prdash-pr-1", OpenWorkspaceID: "w19"}},
	}
	h := NewHerdrNative(runner, base)

	wt, err := h.Create(context.Background(), Spec{Repo: repo, Branch: "feature", Path: dest, Label: "prdash-pr-1"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(runner.createCalls) != 0 {
		t.Fatal("un worktree existente no debería volver a crearse")
	}
	if wt.WorkspaceID != "w19" {
		t.Fatalf("debería resolver el workspace abierto: %+v", wt)
	}
	if list := h.List(context.Background()); len(list) != 1 || list[0].Path != dest {
		t.Fatalf("List = %+v", list)
	}
}

func TestHerdrNativeRemoveUsesWorkspace(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature")
	base := t.TempDir()
	dest := filepath.Join(base, "prdash-pr-1")
	if _, err := NewGitDirect(base).Create(context.Background(), Spec{Repo: repo, Branch: "feature", Path: dest, Label: "prdash-pr-1"}); err != nil {
		t.Fatalf("preparar worktree: %v", err)
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

func TestHerdrNativeCreateIncompleteSpecErrors(t *testing.T) {
	h := NewHerdrNative(&fakeRunner{available: true}, t.TempDir())
	if _, err := h.Create(context.Background(), Spec{Repo: "x"}); err == nil {
		t.Fatal("spec incompleto debería fallar")
	}
}
