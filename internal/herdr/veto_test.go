package herdr

import (
	"context"
	"strings"
	"testing"
	"time"
)

// Herdr's CLI operations split in two for a reason that is not aesthetic: the ones that MODIFY the
//session go through the guard.

// The control case is at the end.
func TestNoMutatingOperationTouchesTheSessionOutsideHerdr(t *testing.T) {
	calls := 0
	seen := [][]string{}
	outside := &Client{
		Bin:    "must-not-run",
		getenv: func(string) string { return "" },
		execFn: func(_ context.Context, args ...string) ([]byte, []byte, error) {
			calls++
			seen = append(seen, args)
			return []byte(`{"id":"1","result":{"type":"ok"}}`), nil, nil
		},
	}
	ctx := context.Background()
	ref := Container{WorkspaceID: "w1", PaneID: "p1"}

	ops := []struct {
		name string
		call func() error
	}{
		{"WorktreeRemove", func() error { return outside.WorktreeRemove(ctx, "w1", true) }},
		{"WorkspaceCreate", func() error { _, err := outside.WorkspaceCreate(ctx, WorkspaceSpec{Label: "x"}); return err }},
		{"WorkspaceClose", func() error { return outside.WorkspaceClose(ctx, "w1", true) }},
		{"TabCreate", func() error {
			_, err := outside.TabCreate(ctx, TabSpec{WorkspaceID: "w1", Label: "t"})
			return err
		}},
		{"TabRename", func() error { return outside.TabRename(ctx, "t1", "new") }},
		{"PaneSplit", func() error {
			_, err := outside.PaneSplit(ctx, SplitSpec{PaneID: ref.PaneID, Direction: "right"})
			return err
		}},
		{"PaneRun", func() error { return outside.PaneRun(ctx, "p1", []string{"echo", "hello"}) }},
		{"PaneWaitOutput", func() error { return outside.PaneWaitOutput(ctx, "p1", "hello", time.Second) }},
		{"PaneRename", func() error { return outside.PaneRename(ctx, "p1", "title") }},
		{"PaneFocus", func() error { return outside.PaneFocus(ctx, "right") }},
		{"MountLayout", func() error { _, err := outside.MountLayout(ctx, ref, testPlan()); return err }},
	}
	// Notify does not go through the guard: it returns warnings, not an error.

	for _, op := range ops {
		if err := op.call(); err == nil {
			t.Errorf("%s: outside Herdr it gave nil, and an operation that modifies the "+
				"session has to refuse", op.name)
		}
	}
	// And none of them reached the binary. This is the assertion that matters: a veto honoured in the
	// message but not in the call is no veto.
	if len(seen) != 0 {
		t.Errorf("%d calls to the binary ran while outside Herdr: %v", len(seen), seen)
	}

	inside := &Client{
		Bin:    "herdr",
		getenv: func(k string) string { return map[string]string{"HERDR_ENV": "1"}[k] },
		execFn: func(_ context.Context, args ...string) ([]byte, []byte, error) {
			seen = append(seen, args)
			if len(args) > 0 && args[0] == "pane" && len(args) > 1 && args[1] == "split" {
				return []byte(`{"id":"1","result":{"pane":{"pane_id":"p2"}}}`), nil, nil
			}
			return []byte(`{"id":"1","result":{"type":"ok"}}`), nil, nil
		},
	}
	refInside := Container{WorkspaceID: "w1", PaneID: "p1"}
	ok := []struct {
		name string
		call func() error
	}{
		{"WorktreeRemove", func() error { return inside.WorktreeRemove(ctx, "w1", true) }},
		{"WorkspaceClose", func() error { return inside.WorkspaceClose(ctx, "w1", true) }},
		{"TabRename", func() error { return inside.TabRename(ctx, "t1", "new") }},
		{"PaneRun", func() error { return inside.PaneRun(ctx, "p1", []string{"echo", "hello"}) }},
		{"PaneFocus", func() error { return inside.PaneFocus(ctx, "right") }},
		{"PaneSplit", func() error {
			_, err := inside.PaneSplit(ctx, SplitSpec{PaneID: refInside.PaneID, Direction: "right"})
			return err
		}},
	}
	for _, op := range ok {
		if err := op.call(); err != nil {
			t.Errorf("%s: inside Herdr it gave an error: %v", op.name, err)
		}
	}
	if len(seen) == 0 {
		t.Error("inside Herdr no call was executed: the veto above is not caused by the " +
			"veto, it is caused by not being inside")
	}
	// The warning names Herdr instead of surfacing an internal error.
	err := outside.PaneRun(ctx, "p1", []string{"echo", "hello"})
	if err == nil {
		t.Error("the veto gave no error")
	} else {
		text := err.Error()
		for _, want := range []string{"herdr unavailable", "HERDR_ENV", "0.9.0"} {
			if !strings.Contains(text, want) {
				t.Errorf("the veto's warning %q does not mention %q", text, want)
			}
		}
	}
	// MountLayout outside Herdr fails with an ERROR, not only with warnings: there is no layout to
	// open.
	if _, err := outside.MountLayout(ctx, ref, testPlan()); err == nil {
		t.Error("MountLayout outside Herdr gave nil")
	}
}

func TestReadOnlyIsNotVetoed(t *testing.T) {
	calls := 0
	seen := [][]string{}
	a := &Client{
		Bin:    "herdr",
		getenv: func(string) string { return "" }, // outside Herdr on purpose
		execFn: func(_ context.Context, args ...string) ([]byte, []byte, error) {
			calls++
			seen = append(seen, args)
			return []byte(`{"result":{"source":{"repo_root":"/r"},"worktrees":[
				{"path":"/r","branch":"main","label":"et","open_workspace_id":"w1",
				 "is_linked_worktree":false},
				{"path":"/r/.worktrees/wt-1","branch":"feat/x","label":"prdash-pr-1",
				 "open_workspace_id":"w2","is_linked_worktree":true}
			]}}`), nil, nil
		},
	}

	list, err := a.WorktreeList(context.Background(), "/r")
	if err != nil {
		t.Fatalf("a read gave an error: %v", err)
	}
	if calls == 0 {
		t.Error("the read never reached the binary: the veto swallowed the reads too")
	}
	if len(list) != 2 {
		t.Fatalf("the list gave %d entries: %+v", len(list), list)
	}
	if list[0].Path != "/r" || list[0].Branch != "main" {
		t.Errorf("the first entry was read wrong: %+v", list[0])
	}
	if !list[1].IsLinkedWorktree || list[1].OpenWorkspaceID != "w2" {
		t.Errorf("the second entry lost the flags: %+v", list[1])
	}
	if list[0].IsLinkedWorktree {
		t.Error("the main repo appeared as a linked worktree")
	}

	broken := &Client{
		Bin:    "herdr",
		getenv: func(string) string { return "" },
		execFn: func(context.Context, ...string) ([]byte, []byte, error) {
			return nil, []byte("fatal: no"), &Error{Args: []string{"x"}, Msg: "no", Exit: 1}
		},
	}
	if _, err := broken.WorktreeList(context.Background(), "/r"); err == nil {
		t.Error("a read that fails gave nil")
	}
	empty := &Client{
		Bin: "herdr", getenv: func(string) string { return "" },
		execFn: func(context.Context, ...string) ([]byte, []byte, error) {
			return []byte(`{"result":{"source":{},"worktrees":[]}}`), nil, nil
		},
	}
	emptyList, err := empty.WorktreeList(context.Background(), "/r")
	if err != nil {
		t.Errorf("an empty list gave an error: %v", err)
	}
	if len(emptyList) != 0 {
		t.Errorf("an empty list gave %d entries", len(emptyList))
	}
	_ = seen
}

// NewGraphics was at 0% because every test constructed it differently.
func TestGraphicsClientIsBuiltAndReadsTheEnvironment(t *testing.T) {
	// The socket variable is HERDR_SOCKET_PATH, not HERDR_SOCK.
	t.Setenv("HERDR_PANE_ID", "p9")
	t.Setenv("HERDR_SOCKET_PATH", "/tmp/herdr.sock")
	g := NewGraphics()
	if g.getenv == nil {
		t.Fatal("NewGraphics left getenv as nil")
	}
	if got := g.env("HERDR_PANE_ID"); got != "p9" {
		t.Errorf("env gave %q, want p9", got)
	}
	if got := g.socket(); got != "/tmp/herdr.sock" {
		t.Errorf("socket gave %q, want the HERDR_SOCKET_PATH one", got)
	}
	if got := g.pane(); got != "p9" {
		t.Errorf("pane gave %q, want p9", got)
	}
	direct := &Graphics{Socket: "/tmp/other.sock", PaneID: "p0"}
	if got := direct.socket(); got != "/tmp/other.sock" {
		t.Errorf("with Socket set it gave %q", got)
	}
	if got := direct.pane(); got != "p0" {
		t.Errorf("with PaneID set it gave %q", got)
	}

	t.Setenv("HERDR_PANE_ID", "")
	t.Setenv("HERDR_SOCKET_PATH", "")
	g = NewGraphics()
	if got := g.env("HERDR_PANE_ID"); got != "" {
		t.Errorf("without the variable it gave %q, want empty", got)
	}
	if haveGraphicsTarget(g.socket(), g.pane()) {
		t.Error("without socket or pane it gave a valid target")
	}

	// A Graphics with a nil getenv falls back to os.Getenv instead of panicking.
	gNil := &Graphics{}
	t.Setenv("HERDR_PANE_ID", "p-with-nil")
	if got := gNil.env("HERDR_PANE_ID"); got != "p-with-nil" {
		t.Errorf("with getenv nil it gave %q, want the real environment value", got)
	}
}

// Same case in the normal client.
func TestClientAlsoFallsBackToOsGetenvWithoutGetenv(t *testing.T) {
	cNil := &Client{Bin: "herdr"}
	t.Setenv("HERDR_ENV", "1")
	if got := cNil.env("HERDR_ENV"); got != "1" {
		t.Errorf("with getenv nil it gave %q, want 1", got)
	}
	// And without the variable.
	t.Setenv("HERDR_ENV", "")
	if got := cNil.env("HERDR_ENV"); got != "" {
		t.Errorf("without the variable it gave %q", got)
	}
	own := &Client{getenv: func(k string) string {
		if k == "HERDR_ENV" {
			return "1"
		}
		return ""
	}}
	if got := own.env("HERDR_ENV"); got != "1" {
		t.Errorf("with its own getenv it gave %q", got)
	}
	t.Setenv("HERDR_ENV", "")
	if got := own.env("HERDR_ENV"); got != "1" {
		t.Errorf("after emptying the variable it gave %q: the own getenv wins", got)
	}
}

// The caching matters, not the version: Available queries on every call.
func TestVersionIsQueriedOnceAndCached(t *testing.T) {
	calls := 0
	a := &Client{
		Bin:    "herdr",
		getenv: func(k string) string { return map[string]string{"HERDR_ENV": "1"}[k] },
		execFn: func(_ context.Context, args ...string) ([]byte, []byte, error) {
			calls++
			if len(args) == 1 && args[0] == "--version" {
				return []byte("herdr 0.9.1-preview.7\n"), nil, nil
			}
			return []byte(`{"id":"1","result":{"type":"ok"}}`), nil, nil
		},
	}

	v, ok := a.Version()
	if !ok {
		t.Fatal("Version gave false with a valid output")
	}
	if v.Major != 0 || v.Minor != 9 || v.Patch != 1 {
		t.Errorf("Version gave %d.%d.%d, want 0.9.1", v.Major, v.Minor, v.Patch)
	}
	for i := 0; i < 5; i++ {
		a.Version()
		a.Available()
	}
	if calls != 1 {
		t.Errorf("the version was queried %d times, want 1: Available calls it on every "+
			"operation and without caching every mount would be several subprocess calls",
			calls)
	}

	// With --version failing it is not blocked: that is the development build, where every operation
	// would fail.
	broken := &Client{
		Bin:    "herdr",
		getenv: func(k string) string { return map[string]string{"HERDR_ENV": "1"}[k] },
		execFn: func(context.Context, ...string) ([]byte, []byte, error) {
			return nil, nil, &Error{Args: []string{"--version"}, Msg: "no such flag", Exit: 1}
		},
	}
	if _, ok := broken.Version(); ok {
		t.Error("Version gave true with a --version that fails")
	}
	if !broken.Available() {
		t.Error("Available vetoed on drift when the version cannot be determined: " +
			"that leaves prdash without Herdr on a development build")
	}
}
