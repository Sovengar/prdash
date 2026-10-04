package herdr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const DefaultTimeout = 30 * time.Second

// 250ms on purpose, not a second: this is a TUI, what does not answer the keyboard reads as a
// hang, and a process that has not closed its pipes in 250ms is not going to.
const pipeCloseGrace = 250 * time.Millisecond

type execFunc func(ctx context.Context, args ...string) (stdout, stderr []byte, err error)

type Client struct {
	// Empty falls back to HERDR_BIN_PATH, then "herdr".
	Bin     string
	Timeout time.Duration

	execFn execFunc
	getenv func(string) string

	versionOnce  sync.Once
	version      Version
	versionFound bool
}

func New() *Client { return &Client{Bin: defaultBin(), getenv: os.Getenv} }

// An undeterminable version does not block on drift: it is assumed compatible.
func (c *Client) Available() bool {
	if c.env("HERDR_ENV") != "1" {
		return false
	}
	v, ok := c.Version()
	if !ok {
		return true
	}
	return v.AtLeast(MinVersion)
}

func (c *Client) Version() (Version, bool) {
	c.versionOnce.Do(func() {
		out, _, err := c.run(context.Background(), "--version")
		if err != nil {
			return
		}
		c.version, c.versionFound = parseVersion(out)
	})
	return c.version, c.versionFound
}

func (c *Client) env(key string) string {
	if c.getenv != nil {
		return c.getenv(key)
	}
	return os.Getenv(key)
}

// The port never touches the session when it cannot do it with guarantees. Reads (list, version) do
// not go through here.
func (c *Client) guard(args ...string) error {
	if !c.Available() {
		return &Error{Args: args, Msg: "herdr unavailable (requires HERDR_ENV=1 and version >= " + MinVersion.String() + ")"}
	}
	return nil
}

func (c *Client) run(ctx context.Context, args ...string) ([]byte, []byte, error) {
	if c.execFn != nil {
		return c.execFn(ctx, args...)
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	bin := c.Bin
	if bin == "" {
		bin = defaultBin()
	}
	cmd := exec.CommandContext(cctx, bin, args...)
	cmd.Env = os.Environ()

	// Measured before adding it: the context kills the process but its CHILD inherits the descriptors
	// and holds the pipe, so `cmd.Run` never returns.
	cmd.WaitDelay = pipeCloseGrace

	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	return out.Bytes(), errb.Bytes(), err
}

func (c *Client) result(ctx context.Context, args ...string) ([]byte, error) {
	out, errb, err := c.run(ctx, args...)
	if err != nil {
		return nil, newError(args, err, errb)
	}
	return out, nil
}

// The message comes from ONE of two sources, never mixed: the server's `message` when stderr is
// a Herdr reply, the first stderr line otherwise. It used to start from the line and be overwritten.
func newError(args []string, err error, stderr []byte) *Error {
	e := &Error{Args: args, Err: err}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		e.Exit = exit.ExitCode()
	}
	if code, msg := parseServerError(stderr); code != "" || msg != "" {
		e.Code = code
		e.Msg = msg
	} else {
		e.Msg = firstLine(string(stderr))
	}
	if e.Msg == "" {
		e.Msg = err.Error()
	}
	return e
}

func (c *Client) WorktreeCreate(ctx context.Context, spec WorktreeSpec) (WorktreeInfo, error) {
	if err := c.guard("worktree", "create"); err != nil {
		return WorktreeInfo{}, err
	}
	out, err := c.result(ctx, worktreeCreateArgs(spec)...)
	if err != nil {
		return WorktreeInfo{}, err
	}
	return parseWorktreeCreated(out)
}

func (c *Client) WorktreeList(ctx context.Context, cwd string) ([]WorktreeInfo, error) {
	out, err := c.result(ctx, worktreeListArgs(cwd)...)
	if err != nil {
		return nil, err
	}
	list, err := parseWorktreeList(out)
	if err != nil {
		return nil, err
	}
	return list.Worktrees, nil
}

func (c *Client) WorktreeRemove(ctx context.Context, workspaceID string, force bool) error {
	if err := c.guard("worktree", "remove"); err != nil {
		return err
	}
	_, err := c.result(ctx, worktreeRemoveArgs(workspaceID, force)...)
	return err
}

func (c *Client) WorkspaceCreate(ctx context.Context, spec WorkspaceSpec) (WorkspaceInfo, error) {
	if err := c.guard("workspace", "create"); err != nil {
		return WorkspaceInfo{}, err
	}
	out, err := c.result(ctx, workspaceCreateArgs(spec)...)
	if err != nil {
		return WorkspaceInfo{}, err
	}
	return parseWorkspaceCreated(out)
}

// worktrees vinculados (algunas versiones lo exigen).
func (c *Client) WorkspaceClose(ctx context.Context, workspaceID string, group bool) error {
	if err := c.guard("workspace", "close"); err != nil {
		return err
	}
	_, err := c.result(ctx, workspaceCloseArgs(workspaceID, group)...)
	return err
}

func (c *Client) TabCreate(ctx context.Context, spec TabSpec) (TabInfo, error) {
	if err := c.guard("tab", "create"); err != nil {
		return TabInfo{}, err
	}
	out, err := c.result(ctx, tabCreateArgs(spec)...)
	if err != nil {
		return TabInfo{}, err
	}
	return parseTabCreated(out)
}

// The only way to name the tab the container already created (the worktree's root pane):
// `tab create --label` only applies to tabs its caller creates.
func (c *Client) TabRename(ctx context.Context, tabID, label string) error {
	if err := c.guard("tab", "rename"); err != nil {
		return err
	}
	_, err := c.result(ctx, "tab", "rename", tabID, label)
	return err
}

func (c *Client) PaneSplit(ctx context.Context, spec SplitSpec) (PaneInfo, error) {
	if err := c.guard("pane", "split"); err != nil {
		return PaneInfo{}, err
	}
	out, err := c.result(ctx, splitArgs(spec)...)
	if err != nil {
		return PaneInfo{}, err
	}
	return parsePaneSplit(out)
}

// Fire-and-forget: the command plus Enter is sent to the pane's shell and nothing waits. The argv
// is joined into one quoted shell line so an argument with spaces does not break.
func (c *Client) PaneRun(ctx context.Context, paneID string, argv []string) error {
	if len(argv) == 0 {
		return fmt.Errorf("pane run: empty argv")
	}
	if err := c.guard("pane", "run"); err != nil {
		return err
	}
	_, err := c.result(ctx, "pane", "run", paneID, strings.Join(argv, " "))
	return err
}

func (c *Client) PaneWaitOutput(ctx context.Context, paneID, match string, timeout time.Duration) error {
	if err := c.guard("pane", "wait-output"); err != nil {
		return err
	}
	_, err := c.result(ctx, waitOutputArgs(paneID, match, timeout)...)
	return err
}

func (c *Client) PaneRename(ctx context.Context, paneID, label string) error {
	if err := c.guard("pane", "rename"); err != nil {
		return err
	}
	_, err := c.result(ctx, "pane", "rename", paneID, label)
	return err
}

func (c *Client) PaneFocus(ctx context.Context, direction string) error {
	if direction == "" {
		direction = "right"
	}
	if err := c.guard("pane", "focus"); err != nil {
		return err
	}
	_, err := c.result(ctx, "pane", "focus", "--direction", direction)
	return err
}

func (c *Client) PaneList(ctx context.Context, workspaceID string) ([]PaneInfo, error) {
	out, err := c.result(ctx, paneListArgs(workspaceID)...)
	if err != nil {
		return nil, err
	}
	return parsePaneList(out)
}

func (c *Client) Notify(ctx context.Context, title string, opts NotifyOptions) error {
	if err := c.guard("notification", "show"); err != nil {
		return err
	}
	_, err := c.result(ctx, notifyArgs(title, opts)...)
	return err
}
