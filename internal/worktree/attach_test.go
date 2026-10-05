package worktree

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"prdash/internal/herdr"
	"prdash/internal/testutil"
)

func attachFor(t *testing.T, runner *fakeRunner, spec Spec) (Worktree, error) {
	t.Helper()
	wt := Worktree{ID: spec.Path, Path: spec.Path}
	return wt, NewHerdrNative(runner, t.TempDir()).attach(context.Background(), &wt, spec)
}

func TestAttachAdoptsTheWorkspaceThatIsAlreadyOpen(t *testing.T) {
	spec := Spec{Repo: "/repos/acme", Branch: "feat/x", Path: "/wt/x", Label: "prdash/x"}
	runner := &fakeRunner{
		available: true,
		listResult: []herdr.WorktreeInfo{
			{Path: "/wt/otro", OpenWorkspaceID: "ws-otro"}, // another checkout: ignored
			{Path: spec.Path, OpenWorkspaceID: "ws-7"},
		},
		panes: map[string][]herdr.PaneInfo{
			"ws-7": {{PaneID: "ws-7:p0"}, {PaneID: "ws-7:p1"}},
		},
	}

	wt, err := attachFor(t, runner, spec)
	if err != nil {
		t.Fatalf("adopting gave an error: %v", err)
	}
	if wt.WorkspaceID != "ws-7" {
		t.Errorf("workspace=%q, want ws-7", wt.WorkspaceID)
	}
	if wt.RootPaneID != "ws-7:p0" {
		t.Errorf("base pane=%q, want ws-7:p0: it is the first one of the list", wt.RootPaneID)
	}
	if len(runner.wsCalls) != 0 {
		t.Errorf("%d new workspaces were opened (%v) when one was already open and adopting: "+
			"the review would show up as a loose, new workspace on every mount",
			len(runner.wsCalls), runner.wsCalls)
	}
}

// Checked by path.
func TestAttachDoesNotAdoptTheWorkspaceOfAnotherCheckout(t *testing.T) {
	spec := Spec{Repo: "/repos/acme", Branch: "feat/x", Path: "/wt/x", Label: "prdash/x"}
	runner := &fakeRunner{
		available: true,
		listResult: []herdr.WorktreeInfo{
			{Path: "/wt/otro", OpenWorkspaceID: "ws-otro"},
		},
		panes:     map[string][]herdr.PaneInfo{"ws-otro": {{PaneID: "ws-otro:p0"}}},
		workspace: herdr.WorkspaceInfo{WorkspaceID: "ws-nuevo", RootPaneID: "ws-nuevo:p0"},
	}

	wt, err := attachFor(t, runner, spec)
	if err != nil {
		t.Fatalf("gave an error: %v", err)
	}
	if wt.WorkspaceID != "ws-nuevo" {
		t.Errorf("workspace=%q, want ws-nuevo: another checkout's is not adopted, or the "+
			"review of PR 7 mounts in the workspace of PR 6", wt.WorkspaceID)
	}
	if len(runner.paneListArgs) != 0 {
		t.Errorf("it asked for the panes of %v, which belong to another checkout", runner.paneListArgs)
	}
}

// The case that justifies the whole check: the open_workspace_id is there, looks fine, and points
// at nothing.
func TestAttachAClosedIDIsNotAdopted(t *testing.T) {
	for _, c := range []struct {
		name  string
		panes []herdr.PaneInfo
		err   error
	}{
		{"the workspace no longer exists", nil, errors.New("workspace_not_found")},
		{"the workspace exists but has no panes", []herdr.PaneInfo{}, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			spec := Spec{Repo: "/repos/acme", Branch: "feat/x", Path: "/wt/x"}
			runner := &fakeRunner{
				available:  true,
				listResult: []herdr.WorktreeInfo{{Path: spec.Path, OpenWorkspaceID: "ws-7"}},
				panes:      map[string][]herdr.PaneInfo{"ws-7": c.panes},
				panesErr:   map[string]error{"ws-7": c.err},
				workspace:  herdr.WorkspaceInfo{WorkspaceID: "ws-nuevo", RootPaneID: "ws-nuevo:p0"},
			}

			wt, err := attachFor(t, runner, spec)
			if err != nil {
				t.Fatalf("gave an error: %v", err)
			}
			if wt.WorkspaceID != "ws-nuevo" {
				t.Errorf("workspace=%q, want ws-nuevo: an id that points at nothing is not adopted",
					wt.WorkspaceID)
			}
			if wt.RootPaneID != "ws-nuevo:p0" {
				t.Errorf("pane=%q, want the new workspace's", wt.RootPaneID)
			}
			if len(runner.wsCalls) != 1 {
				t.Errorf("%d workspaces were opened, want 1: without a base pane one has to be opened",
					len(runner.wsCalls))
			}
			if len(runner.wsCalls) == 1 {
				w := runner.wsCalls[0]
				if w.Cwd != spec.Path {
					t.Errorf("cwd=%q, want %q: the workspace has to stay at the worktree", w.Cwd, spec.Path)
				}
				if !w.NoFocus {
					t.Error("NoFocus=false: mounting a review cannot steal the TUI's focus")
				}
				if w.Label != spec.Label {
					t.Errorf("label=%q, want %q", w.Label, spec.Label)
				}
			}
		})
	}
}

// Pure degradation, the class of thing nobody looks at.
func TestAttachWithoutAListDoesNotAdopt(t *testing.T) {
	spec := Spec{Repo: "/repos/acme", Branch: "feat/x", Path: "/wt/x", Label: "prdash/x"}
	runner := &fakeRunner{
		available:  true,
		listResult: nil,
		workspace:  herdr.WorkspaceInfo{WorkspaceID: "ws-nuevo", RootPaneID: "ws-nuevo:p0"},
	}

	wt, err := attachFor(t, runner, spec)
	if err != nil {
		t.Fatalf("without a list it should open a workspace and carry on: %v", err)
	}
	if wt.WorkspaceID != "ws-nuevo" {
		t.Errorf("workspace=%q, want ws-nuevo", wt.WorkspaceID)
	}
	if len(runner.paneListArgs) != 0 {
		t.Errorf("it asked for the panes (%v) with no list to say so", runner.paneListArgs)
	}
}

func TestAttachWithAnEmptyWorkspaceIDDoesNotAdopt(t *testing.T) {
	spec := Spec{Repo: "/repos/acme", Branch: "feat/x", Path: "/wt/x"}
	runner := &fakeRunner{
		available: true,
		listResult: []herdr.WorktreeInfo{
			{Path: spec.Path, OpenWorkspaceID: ""}, // no open workspace
		},
		workspace: herdr.WorkspaceInfo{WorkspaceID: "ws-nuevo", RootPaneID: "ws-nuevo:p0"},
	}

	wt, err := attachFor(t, runner, spec)
	if err != nil {
		t.Fatalf("gave an error: %v", err)
	}
	if wt.WorkspaceID != "ws-nuevo" {
		t.Errorf("workspace=%q, want ws-nuevo", wt.WorkspaceID)
	}
	if len(runner.paneListArgs) != 0 {
		t.Errorf("it asked for the panes of an EMPTY id (%v): Herdr cannot "+
			"answer for a workspace that does not exist", runner.paneListArgs)
	}
}

func TestAttachOnlyOpensAWorkspaceWhenEverythingFails(t *testing.T) {
	spec := Spec{Repo: "/repos/acme", Branch: "feat/x", Path: "/wt/x"}
	runner := &fakeRunner{
		available: true,
		listResult: []herdr.WorktreeInfo{
			{Path: spec.Path, OpenWorkspaceID: "ws-cerrado"},
			{Path: "/wt/otro", OpenWorkspaceID: "ws-valido"},
		},
		panes: map[string][]herdr.PaneInfo{
			"ws-cerrado": nil,
			"ws-valido":  {{PaneID: "ws-valido:p0"}},
		},
		panesErr:  map[string]error{"ws-cerrado": errors.New("workspace_not_found")},
		workspace: herdr.WorkspaceInfo{WorkspaceID: "ws-nuevo", RootPaneID: "ws-nuevo:p0"},
	}

	wt, err := attachFor(t, runner, spec)
	if err != nil {
		t.Fatalf("gave an error: %v", err)
	}
	if wt.WorkspaceID != "ws-nuevo" {
		t.Errorf("workspace=%q, want ws-nuevo", wt.WorkspaceID)
	}
	if len(runner.wsCalls) != 1 {
		t.Errorf("%d workspaces were opened, want 1: opening two leaves an orphan workspace "+
			"per mount, and that does not error, it shows up as a duplicated review", len(runner.wsCalls))
	}
	if len(runner.paneListArgs) != 1 || runner.paneListArgs[0] != "ws-cerrado" {
		t.Errorf("it asked for the panes of %v, want exactly [ws-cerrado]", runner.paneListArgs)
	}
}

// The `break`: as soon as the checkout appears with an id that does not hold, the rest of the
// list is not read.
func TestAttachABadIDStopsTheSearch(t *testing.T) {
	spec := Spec{Repo: "/repos/acme", Branch: "feat/x", Path: "/wt/x"}
	runner := &fakeRunner{
		available: true,
		listResult: []herdr.WorktreeInfo{
			{Path: spec.Path, OpenWorkspaceID: "ws-cerrado"},
			{Path: spec.Path, OpenWorkspaceID: "ws-vivo"},
		},
		panes: map[string][]herdr.PaneInfo{
			"ws-cerrado": nil,
			"ws-vivo":    {{PaneID: "ws-vivo:p0"}},
		},
		panesErr:  map[string]error{"ws-cerrado": errors.New("workspace_not_found")},
		workspace: herdr.WorkspaceInfo{WorkspaceID: "ws-nuevo", RootPaneID: "ws-nuevo:p0"},
	}

	wt, err := attachFor(t, runner, spec)
	if err != nil {
		t.Fatalf("gave an error: %v", err)
	}
	if wt.WorkspaceID != "ws-nuevo" {
		t.Errorf("workspace=%q, want ws-nuevo: as soon as the checkout appears with an id that "+
			"does not hold, the rest of the list is not read. Without that the id of the second "+
			"entry would be adopted, which belongs to another repo and would open the review in a "+
			"foreign workspace",
			wt.WorkspaceID)
	}
	if len(runner.paneListArgs) != 1 || runner.paneListArgs[0] != "ws-cerrado" {
		t.Errorf("it asked for the panes of %v, want exactly [ws-cerrado]", runner.paneListArgs)
	}
}

// On REUSING an existing checkout the label is the caller's, not the one the checkout had.
func TestReuseTheCallersLabelOverwritesTheCheckouts(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature")

	base := t.TempDir()
	dest := filepath.Join(base, "prdash-pr-1")
	if _, err := NewGitDirect(base).Create(context.Background(),
		Spec{Repo: repo, Branch: "feature", Path: dest, Label: "etiqueta-VIEJA"}); err != nil {
		t.Fatalf("preparing a worktree: %v", err)
	}

	runner := &fakeRunner{
		available: true,
		listResult: []herdr.WorktreeInfo{
			{Path: dest, Branch: "feature", Label: "la-del-repo-del-nativo"},
		},
		workspace: herdr.WorkspaceInfo{WorkspaceID: "w1", RootPaneID: "w1:p1"},
	}

	wt, err := NewHerdrNative(runner, base).Create(context.Background(),
		Spec{Repo: repo, Branch: "feature", Path: dest, Label: "prdash/acme#12"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if wt.Label != "prdash/acme#12" {
		t.Errorf("label=%q, want the caller's prdash/acme#12: if the checkout's or the native's "+
			"wins, the worktree comes back with the name of the previous session and "+
			"prdash stops recognising it", wt.Label)
	}
	if len(runner.wsCalls) != 1 || runner.wsCalls[0].Label != "prdash/acme#12" {
		t.Errorf("the workspace was opened with label %q, want the caller's",
			labelOf(runner.wsCalls))
	}
}

// Without a caller label the checkout's is kept, which is the DIRECTORY NAME, because that is what
// Herdr reports.
func TestReuseWithoutACallerLabelKeepsTheCheckouts(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature")

	base := t.TempDir()
	dest := filepath.Join(base, "el-pr-7-del-repo-acme")
	if _, err := NewGitDirect(base).Create(context.Background(),
		Spec{Repo: repo, Branch: "feature", Path: dest, Label: "lo-que-pase"}); err != nil {
		t.Fatalf("preparing a worktree: %v", err)
	}

	runner := &fakeRunner{
		available: true,
		listResult: []herdr.WorktreeInfo{
			{Path: dest, Branch: "feature", Label: "acme-widget"},
		},
		workspace: herdr.WorkspaceInfo{WorkspaceID: "w1", RootPaneID: "w1:p1"},
	}

	wt, err := NewHerdrNative(runner, base).Create(context.Background(),
		Spec{Repo: repo, Branch: "feature", Path: dest})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if wt.Label != "el-pr-7-del-repo-acme" {
		t.Errorf("label=%q, want the checkout's name el-pr-7-del-repo-acme", wt.Label)
	}
	if wt.Label == "acme-widget" {
		t.Error("the label is the repo name the native reports: all the " +
			"worktrees of that repo would be called the same and stop being distinguishable")
	}
}

func labelOf(calls []herdr.WorkspaceSpec) string {
	if len(calls) == 0 {
		return "<no workspace was opened>"
	}
	return calls[0].Label
}
