// Package herdr is the only coupling point with Herdr: it reads HERDR_ENV, invokes the binary and
// parses its JSON. The rest of the program talks to the Port and never touches Herdr. Outside Herdr, or
// below the minimum version, the port reports unavailable and no mutating operation runs. All Herdr
// output is treated as data: server errors arrive as JSON on stderr and become a typed Error, never a
// panic.
package herdr

import (
	"context"
	"fmt"
	"os"
	"time"

	"prdash/internal/review/plan"
)

var MinVersion = Version{Major: 0, Minor: 9, Patch: 0}

type Version struct {
	Major int
	Minor int
	Patch int
	Raw   string
}

func (v Version) AtLeast(min Version) bool {
	switch {
	case v.Major != min.Major:
		return v.Major > min.Major
	case v.Minor != min.Minor:
		return v.Minor > min.Minor
	default:
		return v.Patch >= min.Patch
	}
}

func (v Version) String() string { return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch) }

func InHerdr() bool { return os.Getenv("HERDR_ENV") == "1" }

type WorktreeSpec struct {
	Cwd     string
	Branch  string
	Path    string
	Label   string
	NoFocus bool
}

type WorktreeInfo struct {
	WorkspaceID      string
	WorkspaceLabel   string
	TabID            string
	RootPaneID       string
	Path             string
	Branch           string
	Label            string
	OpenWorkspaceID  string
	IsLinkedWorktree bool
}

type WorkspaceSpec struct {
	Cwd     string
	Label   string
	NoFocus bool
}

type WorkspaceInfo struct {
	WorkspaceID string
	TabID       string
	RootPaneID  string
}

type TabSpec struct {
	WorkspaceID string
	Cwd         string
	Label       string
	NoFocus     bool
}

type TabInfo struct {
	TabID      string
	RootPaneID string
}

type SplitSpec struct {
	PaneID    string
	Direction string
	Ratio     float64
	Cwd       string
	Env       []string
	NoFocus   bool
}

// PaneInfo identifica un pane.
type PaneInfo struct {
	PaneID      string
	WorkspaceID string
	TabID       string
	Cwd         string
	Label       string
}

type NotifyOptions struct {
	Body  string
	Sound string // none|done|request (vacío = default de Herdr)
}

// Both ids come together because the native provisioning delivers them paired and the layout needs
// them paired; with neither, the layout creates the workspace and takes both from there.
type Container struct {
	WorkspaceID string
	PaneID      string
}

type Error struct {
	Args []string
	Exit int
	Code string
	Msg  string
	Err  error
}

func (e *Error) Error() string {
	base := fmt.Sprintf("herdr %v: %s", e.Args, e.Msg)
	if e.Code != "" {
		base = fmt.Sprintf("%s [%s]", base, e.Code)
	}
	if e.Exit != 0 {
		return fmt.Sprintf("%s (exit %d)", base, e.Exit)
	}
	return base
}

func (e *Error) Unwrap() error { return e.Err }

type Port interface {
	Available() bool
	Version() (Version, bool)

	WorktreeCreate(ctx context.Context, spec WorktreeSpec) (WorktreeInfo, error)
	WorktreeList(ctx context.Context, cwd string) ([]WorktreeInfo, error)
	WorktreeRemove(ctx context.Context, workspaceID string, force bool) error

	WorkspaceCreate(ctx context.Context, spec WorkspaceSpec) (WorkspaceInfo, error)
	WorkspaceClose(ctx context.Context, workspaceID string, group bool) error
	TabCreate(ctx context.Context, spec TabSpec) (TabInfo, error)
	TabRename(ctx context.Context, tabID, label string) error

	PaneSplit(ctx context.Context, spec SplitSpec) (PaneInfo, error)
	PaneRun(ctx context.Context, paneID string, argv []string) error
	PaneWaitOutput(ctx context.Context, paneID, match string, timeout time.Duration) error
	PaneRename(ctx context.Context, paneID, label string) error
	PaneFocus(ctx context.Context, direction string) error
	PaneList(ctx context.Context, workspaceID string) ([]PaneInfo, error)

	Notify(ctx context.Context, title string, opts NotifyOptions) error

	MountLayout(ctx context.Context, container Container, pl plan.Plan) ([]string, error)
}

func defaultBin() string {
	if p := os.Getenv("HERDR_BIN_PATH"); p != "" {
		return p
	}
	return "herdr"
}
