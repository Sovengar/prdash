package worktree

import (
	"path/filepath"
	"testing"

	"prdash/internal/herdr"
)

func emptySpec() Spec {
	return Spec{Repo: "/repos/acme", Branch: "feat/x", Path: "/wt/acme-feat-x", Label: "prdash/acme#1"}
}

func emptyInfo() herdr.WorktreeInfo {
	return herdr.WorktreeInfo{}
}

func TestComposedWithoutHerdrDataKeepsWhatTheCallerAsked(t *testing.T) {
	spec := emptySpec()
	wt := composed(spec, emptyInfo())

	if wt.Path != spec.Path || wt.ID != spec.Path {
		t.Errorf("with no Herdr data it gave path=%q id=%q, want %q for both", wt.Path, wt.ID, spec.Path)
	}
	if wt.Branch != spec.Branch {
		t.Errorf("with no Herdr data it gave branch=%q, want %q", wt.Branch, spec.Branch)
	}
	if wt.Repo != spec.Repo {
		t.Errorf("with no Herdr data it gave repo=%q, want %q", wt.Repo, spec.Repo)
	}
	if wt.Label != spec.Label {
		t.Errorf("with no Herdr data it gave label=%q, want %q", wt.Label, spec.Label)
	}
	if wt.WorkspaceID != "" || wt.RootPaneID != "" {
		t.Errorf("with no Herdr data it gave workspace=%q pane=%q, want empty",
			wt.WorkspaceID, wt.RootPaneID)
	}
}

func TestComposedHerdrWinsWhereItSaysSomething(t *testing.T) {
	spec := emptySpec()
	info := herdr.WorktreeInfo{
		Path:           "/other/sitio/moved",
		Branch:         "renamed-by-herdr",
		WorkspaceID:    "ws-1",
		RootPaneID:     "pane-1",
		WorkspaceLabel: "ws-label",
		Label:          "the-repo",
	}

	wt := composed(spec, info)

	if wt.Path != info.Path {
		t.Errorf("path=%q, want Herdr's %q: if it moved the checkout, the other one is no good",
			wt.Path, info.Path)
	}
	if wt.ID != info.Path {
		t.Errorf("id=%q, want %q: the id is the path, and if the path moved the id goes with it",
			wt.ID, info.Path)
	}
	if wt.Branch != info.Branch {
		t.Errorf("branch=%q, want Herdr's %q", wt.Branch, info.Branch)
	}
	if wt.WorkspaceID != "ws-1" || wt.RootPaneID != "pane-1" {
		t.Errorf("containers=%q/%q, want ws-1/pane-1", wt.WorkspaceID, wt.RootPaneID)
	}
	if wt.Repo != spec.Repo {
		t.Errorf("repo=%q, want %q: Herdr does not return the repo, there is nowhere to get it",
			wt.Repo, spec.Repo)
	}
}

// The label is the EXCEPTION: it is the caller's.
func TestComposedTheLabelIsTheCallersNotHerdrs(t *testing.T) {
	// Every Herdr field distinct, the only way the precedence shows: if two matched, either order would pass.
	info := herdr.WorktreeInfo{
		Path:           "/wt/moved",
		Branch:         "branch-de-herdr",
		WorkspaceID:    "ws-1",
		RootPaneID:     "pane-1",
		WorkspaceLabel: "label-of-the-workspace",
		Label:          "name-of-the-repo",
	}
	spec := emptySpec()

	wt := composed(spec, info)

	if wt.Label != spec.Label {
		t.Errorf("label=%q, want the caller's %q.\n"+
			"With Herdr's label, the worktree stops being recognisable to prdash:\n"+
			"it does not show up when listing, it is not reused and it is not removed.",
			wt.Label, spec.Label)
	}
	for _, pair := range [][2]string{
		{"from the workspace", info.WorkspaceLabel},
		{"from the worktree", info.Label},
	} {
		if pair[1] == spec.Label {
			t.Fatalf("the label %s (%q) is the same as the caller's (%q): "+
				"the test does not distinguish anything", pair[0], pair[1], spec.Label)
		}
	}
	if info.WorkspaceLabel == info.Label {
		t.Fatalf("Herdr's two labels match (%q): the test does not distinguish anything",
			info.Label)
	}
	if wt.Path != info.Path || wt.Branch != info.Branch {
		t.Errorf("the exception has spilled: path=%q branch=%q, want Herdr's",
			wt.Path, wt.Branch)
	}
}

func TestComposedWithoutCallerLabelFallsBackInThisOrder(t *testing.T) {
	t.Run("the workspace one wins", func(t *testing.T) {
		spec := emptySpec()
		spec.Label = ""
		info := herdr.WorktreeInfo{
			WorkspaceLabel: "label-of-the-workspace",
			Label:          "name-of-the-repo",
		}
		if got := composed(spec, info).Label; got != "label-of-the-workspace" {
			t.Errorf("label=%q, want the workspace's: it is the one asked for when opening it", got)
		}
	})

	t.Run("without the workspace's, the repo's", func(t *testing.T) {
		spec := emptySpec()
		spec.Label = ""
		info := herdr.WorktreeInfo{Label: "name-of-the-repo"}
		if got := composed(spec, info).Label; got != "name-of-the-repo" {
			t.Errorf("label=%q, want the repo's", got)
		}
	})

	t.Run("with neither, the path name", func(t *testing.T) {
		spec := emptySpec()
		spec.Label = ""
		if got := composed(spec, emptyInfo()).Label; got != filepath.Base(spec.Path) {
			t.Errorf("label=%q, want the path name %q", got, filepath.Base(spec.Path))
		}
	})

	// The third fallback is Herdr's PATH, not the caller's, which is where the recursion shows.
	t.Run("the last resort comes from the already resolved path", func(t *testing.T) {
		spec := emptySpec()
		spec.Label = ""
		info := herdr.WorktreeInfo{Path: "/other/moved/prdash-42"}
		got := composed(spec, info)
		if got.Label != "prdash-42" {
			t.Errorf("label=%q, want the RESOLVED path name prdash-42, not the caller's %q",
				got.Label, filepath.Base(spec.Path))
		}
		if filepath.Base(spec.Path) == "prdash-42" {
			t.Fatal("the test does not distinguish: both paths give the same name")
		}
	})
}

func TestComposedWithoutPathDoesNotInventAPathName(t *testing.T) {
	spec := emptySpec()
	spec.Label = ""
	spec.Path = ""
	wt := composed(spec, emptyInfo())

	if wt.Label == "." {
		t.Error(`label="." with an empty path: it is the path name of "", which looks like a ` +
			"chosen name and in reality they are all the same")
	}
	if wt.Label != "" {
		t.Errorf("label=%q with an empty path, want empty: there is nowhere to get a name from", wt.Label)
	}
	if wt.ID != "" || wt.Path != "" {
		t.Errorf("with a spec without a path it gave id=%q path=%q, want empty", wt.ID, wt.Path)
	}
}
