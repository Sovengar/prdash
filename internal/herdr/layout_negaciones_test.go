package herdr

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"prdash/internal/review/plan"
)

func TestStaleWorkspaceFailsAndDoesNotFakeAMount(t *testing.T) {
	f := newFakeLayout()
	if _, err := f.client().MountLayout(context.Background(),
		Container{WorkspaceID: "w18"}, testPlan()); err != nil {
		t.Fatalf("with the workspace alive the mount failed: %v", err)
	}

	failing := newFakeLayout()
	base := failing.respond
	failing.respond = func(args []string) ([]byte, []byte, error) {
		if len(args) > 2 && args[0] == "pane" && args[1] == "list" {
			return nil, nil, fmt.Errorf("workspace w1B not found")
		}
		return base(args)
	}
	warns, err := failing.client().MountLayout(context.Background(),
		Container{WorkspaceID: "w1B"}, testPlan())
	if err == nil {
		t.Fatalf("a dead workspace did not mount silently (warnings=%v)", warns)
	}
	if !strings.Contains(err.Error(), "w1B") {
		t.Errorf("the error should name the workspace so one knows which is stale, "+
			"and it says %q", err.Error())
	}
	if err.Error() == "" || strings.Contains(err.Error(), "<nil>") {
		t.Errorf("the error wraps a nil: %q. With the inverted condition the healthy "+
			"path returns an error with a nil inside, which is what the mutant looks like",
			err.Error())
	}
	if failing.called("workspace", "create") {
		t.Error("it should not create a replacement workspace: the mount has to fail, " +
			"not redo the review somewhere else")
	}
}

// The temptation here is to take the first match.
func TestTabIsLookedUpByThePaneThatWasPassed(t *testing.T) {
	// A workspace with SEVERAL panes where the requested one is not the first; the fixture has at
	//least three and the request goes to the last on purpose.
	multi := `{"id":"cli:pane:list","result":{"type":"pane_list","panes":[
	  {"pane_id":"w18:p1","workspace_id":"w18","tab_id":"w18:t1","cwd":"/repo","label":"shell"},
	  {"pane_id":"w18:p2","workspace_id":"w18","tab_id":"w18:t2","cwd":"/repo","label":"vi"},
	  {"pane_id":"w18:p3","workspace_id":"w18","tab_id":"w18:t3","cwd":"/repo","label":"prdash"}
	]}}`
	f := newFakeLayout()
	base := f.respond
	f.respond = func(args []string) ([]byte, []byte, error) {
		if len(args) > 2 && args[0] == "pane" && args[1] == "list" {
			return []byte(multi), nil, nil
		}
		return base(args)
	}
	c := f.client()

	ctx := context.Background()
	panes, err := c.PaneList(ctx, "w18")
	if err != nil {
		t.Fatalf("PaneList: %v", err)
	}
	if len(panes) < 3 {
		t.Fatalf("the fixture has %d panes, and the test needs at least 3 so that "+
			"the request is NOT the first", len(panes))
	}
	requested := panes[len(panes)-1].PaneID

	got := c.tabOf(ctx, panes[0].WorkspaceID, requested)
	if got == "" {
		t.Fatalf("the tab of pane %q came back empty: it exists in the list", requested)
	}
	want := panes[len(panes)-1].TabID
	if want == panes[0].TabID {
		t.Fatalf("the panes of the multi fixture share a tab (%q), and then comparing "+
			"the id wrongly would not be distinguishable", want)
	}
	if got != want {
		t.Errorf("pane %q gave tab %q, want %q. The tab is found by COMPARING the id: "+
			"returning the first pane's renames the wrong tab", requested, got, want)
	}

	// A pane that is not in the list has no tab, which is what makes the mount skip it.
	if got := c.tabOf(ctx, panes[0].WorkspaceID, "w18:no-existe"); got != "" {
		t.Errorf("a pane that is not in the list gave tab %q, want empty", got)
	}
}

// The trap is in what the plan does NOT set.
func TestPlanDirectionIsRespected(t *testing.T) {
	cases := []struct {
		dir  string
		want string
		note string
	}{
		{plan.DirRight, plan.DirRight, "the explicit right"},
		{plan.DirDown, plan.DirDown, "explicit down: it is the diff editor's one, and it " +
			"is misread if put on the right"},
		{"", plan.DirRight, "saying nothing: the right, which is how a diff is read " +
			"next to what comments it"},
	}
	for _, c := range cases {
		if got := direction(plan.Pane{Dir: c.dir}); got != c.want {
			t.Errorf("direction %q gave %q, want %q. %s", c.dir, got, c.want, c.note)
		}
	}
}
