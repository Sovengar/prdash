package herdr

import (
	"strconv"
	"time"
)

// Pure, because an argv is the contract with the CLI and an inline guard can only be checked
// by running it. The rule for optional flags lives here and nowhere else: a flag with an empty value
// is NOT sent, because `--cwd ""` tells Herdr "use the empty directory".
type args []string

func (a args) with(flag, value string) args {
	if value == "" {
		return a
	}
	return append(a, flag, value)
}

func (a args) withIf(cond bool, flag string) args {
	if !cond {
		return a
	}
	return append(a, flag)
}

func worktreeCreateArgs(spec WorktreeSpec) args {
	return args{"worktree", "create"}.
		with("--cwd", spec.Cwd).
		with("--branch", spec.Branch).
		with("--path", spec.Path).
		with("--label", spec.Label).
		withIf(spec.NoFocus, "--no-focus")
}

func worktreeListArgs(cwd string) args {
	return args{"worktree", "list"}.with("--cwd", cwd)
}

func worktreeRemoveArgs(workspaceID string, force bool) args {
	return args{"worktree", "remove", "--workspace", workspaceID}.withIf(force, "--force")
}

func workspaceCreateArgs(spec WorkspaceSpec) args {
	return args{"workspace", "create"}.
		with("--cwd", spec.Cwd).
		with("--label", spec.Label).
		withIf(spec.NoFocus, "--no-focus")
}

// Opt-in for a version reason: some builds require it to close the worktree group and reject it otherwise.
func workspaceCloseArgs(workspaceID string, group bool) args {
	return args{"workspace", "close", workspaceID}.withIf(group, "--group")
}

func tabCreateArgs(spec TabSpec) args {
	return args{"tab", "create"}.
		with("--workspace", spec.WorkspaceID).
		with("--cwd", spec.Cwd).
		with("--label", spec.Label).
		withIf(spec.NoFocus, "--no-focus")
}

func splitArgs(spec SplitSpec) args {
	a := args{"pane", "split", "--pane", spec.PaneID, "--direction", spec.Direction}
	if spec.Ratio > 0 {
		a = append(a, "--ratio", strconv.FormatFloat(spec.Ratio, 'f', -1, 64))
	}
	a = a.with("--cwd", spec.Cwd)
	for _, kv := range spec.Env {
		if kv == "" {
			continue
		}
		a = append(a, "--env", kv)
	}
	return a.withIf(spec.NoFocus, "--no-focus")
}

func waitOutputArgs(paneID, match string, timeout time.Duration) args {
	a := args{"pane", "wait-output", "--match", match, paneID}
	if timeout > 0 {
		a = append(a, "--timeout", strconv.FormatInt(timeout.Milliseconds(), 10))
	}
	return a
}

func paneListArgs(workspaceID string) args {
	return args{"pane", "list"}.with("--workspace", workspaceID)
}

// The empty sound is Herdr's default, and sending an empty one is an error.
func notifyArgs(title string, opts NotifyOptions) args {
	return args{"notification", "show", title}.
		with("--body", opts.Body).
		with("--sound", opts.Sound)
}
