package herdr

import "testing"

// Fixtures de la forma real de la CLI: todo va envuelto en {"id","result"}.

const fixtureWorktreeCreated = `{
  "id": "cli:worktree:create",
  "result": {
    "type": "worktree_created",
    "workspace": {"workspace_id": "w18", "label": "prdash-pr-7"},
    "tab": {"tab_id": "w18:t1"},
    "root_pane": {"pane_id": "w18:p1"},
    "worktree": {
      "path": "/home/u/.herdr/worktrees/prdash/feat-x",
      "branch": "prdash/pr-7",
      "label": "prdash-pr-7",
      "is_linked_worktree": true,
      "is_prunable": false,
      "open_workspace_id": "w18"
    }
  }
}`

const fixtureWorktreeList = `{
  "id": "cli:worktree:list",
  "result": {
    "type": "worktree_list",
    "source": {
      "repo_key": "/repo/.git",
      "repo_name": "repo",
      "repo_root": "/repo",
      "source_checkout_path": "/repo",
      "source_workspace_id": "w17"
    },
    "worktrees": [
      {"path": "/repo", "branch": "main", "label": "repo", "is_linked_worktree": false, "is_prunable": false, "open_workspace_id": "w17"},
      {"path": "/repo.wt/prdash-pr-7", "branch": "prdash/pr-7", "label": "prdash-pr-7", "is_linked_worktree": true, "is_prunable": false, "open_workspace_id": ""}
    ]
  }
}`

const fixtureWorkspaceCreated = `{
  "id": "cli:workspace:create",
  "result": {
    "type": "workspace_created",
    "workspace": {"workspace_id": "w20", "label": "review"},
    "tab": {"tab_id": "w20:t1"},
    "root_pane": {"pane_id": "w20:p1"}
  }
}`

const fixtureTabCreated = `{
  "id": "cli:tab:create",
  "result": {
    "type": "tab_created",
    "tab": {"tab_id": "w20:t2"},
    "root_pane": {"pane_id": "w20:p5"}
  }
}`

const fixturePaneSplit = `{
  "id": "cli:pane:split",
  "result": {
    "type": "pane_info",
    "pane": {"pane_id": "w18:p2", "workspace_id": "w18", "tab_id": "w18:t1", "cwd": "/repo.wt/prdash-pr-7", "label": ""}
  }
}`

const fixturePaneList = `{
  "id": "cli:pane:list",
  "result": {
    "type": "pane_list",
    "panes": [
      {"pane_id": "w18:p1", "workspace_id": "w18", "tab_id": "w18:t1", "cwd": "/repo.wt/prdash-pr-7", "label": "TUICR"},
      {"pane_id": "w18:p2", "workspace_id": "w18", "tab_id": "w18:t1", "cwd": "/repo.wt/prdash-pr-7", "label": "Hunk"}
    ]
  }
}`

const fixtureNotification = `{
  "id": "cli:notification:show",
  "result": {"type": "notification_show", "shown": true, "reason": ""}
}`

func TestParseWorktreeCreated(t *testing.T) {
	info, err := parseWorktreeCreated([]byte(fixtureWorktreeCreated))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if info.WorkspaceID != "w18" || info.TabID != "w18:t1" || info.RootPaneID != "w18:p1" {
		t.Fatalf("contenedor = %+v", info)
	}
	if info.Path != "/home/u/.herdr/worktrees/prdash/feat-x" || info.Branch != "prdash/pr-7" || info.Label != "prdash-pr-7" {
		t.Fatalf("worktree = %+v", info)
	}
	if !info.IsLinkedWorktree || info.OpenWorkspaceID != "w18" {
		t.Fatalf("flags = %+v", info)
	}
}

func TestParseWorktreeList(t *testing.T) {
	list, err := parseWorktreeList([]byte(fixtureWorktreeList))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if list.RepoRoot != "/repo" || len(list.Worktrees) != 2 {
		t.Fatalf("list = %+v", list)
	}
	if list.Worktrees[1].Path != "/repo.wt/prdash-pr-7" || !list.Worktrees[1].IsLinkedWorktree {
		t.Fatalf("worktree[1] = %+v", list.Worktrees[1])
	}
}

func TestParseWorkspaceAndTabAndPane(t *testing.T) {
	ws, err := parseWorkspaceCreated([]byte(fixtureWorkspaceCreated))
	if err != nil || ws.WorkspaceID != "w20" || ws.RootPaneID != "w20:p1" {
		t.Fatalf("workspace = %+v, err=%v", ws, err)
	}
	tab, err := parseTabCreated([]byte(fixtureTabCreated))
	if err != nil || tab.TabID != "w20:t2" || tab.RootPaneID != "w20:p5" {
		t.Fatalf("tab = %+v, err=%v", tab, err)
	}
	pane, err := parsePaneSplit([]byte(fixturePaneSplit))
	if err != nil || pane.PaneID != "w18:p2" || pane.Cwd != "/repo.wt/prdash-pr-7" {
		t.Fatalf("pane = %+v, err=%v", pane, err)
	}
	panes, err := parsePaneList([]byte(fixturePaneList))
	if err != nil || len(panes) != 2 || panes[1].Label != "Hunk" {
		t.Fatalf("panes = %+v, err=%v", panes, err)
	}
}

func TestParseNotification(t *testing.T) {
	shown, reason, err := parseNotification([]byte(fixtureNotification))
	if err != nil || !shown || reason != "" {
		t.Fatalf("notification shown=%v reason=%q err=%v", shown, reason, err)
	}
}

func TestParseVersion(t *testing.T) {
	v, ok := parseVersion([]byte("herdr 0.9.1-preview.2026-09-21-0ff0f27e2226\n"))
	if !ok || v.Major != 0 || v.Minor != 9 || v.Patch != 1 {
		t.Fatalf("version = %+v ok=%v", v, ok)
	}
	if !v.AtLeast(MinVersion) {
		t.Fatalf("%s debería ser >= %s", v, MinVersion)
	}
	if (Version{Major: 0, Minor: 8, Patch: 2}).AtLeast(MinVersion) {
		t.Fatal("0.8.2 no debería alcanzar el mínimo")
	}
	if _, ok := parseVersion([]byte("nope")); ok {
		t.Fatal("sin semver no debería haber versión")
	}
}

func TestParseMalformedResult(t *testing.T) {
	if _, err := parseWorktreeCreated([]byte("{")); err == nil {
		t.Fatal("una respuesta ilegible debería fallar, no entrar en pánico")
	}
	if _, err := parsePaneSplit([]byte(`{"id":"x"}`)); err == nil {
		t.Fatal("un sobre sin .result útil debería fallar")
	}
}

func TestParseServerErrorVariants(t *testing.T) {
	code, msg := parseServerError([]byte(`{"error":{"code":"worktree_create_failed","message":"no such branch"}}`))
	if code != "worktree_create_failed" || msg != "no such branch" {
		t.Fatalf("variant error = %q %q", code, msg)
	}
	code, msg = parseServerError([]byte(`{"code":"x","message":"y"}`))
	if code != "x" || msg != "y" {
		t.Fatalf("variant flat = %q %q", code, msg)
	}
	if code, msg := parseServerError([]byte("no json")); code != "" || msg != "" {
		t.Fatalf("texto sin json = %q %q", code, msg)
	}
}
