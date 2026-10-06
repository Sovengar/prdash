package gitcmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// fetch and clone legitimately take a while. 60s as a literal because a const decl carries no
// coverage, so `*` here would be a mutant no test can reach (ADR 0011).
const DefaultTimeout = time.Duration(60e9)

// A child that inherited our pipe descriptors keeps them open, so without this grace period the
// timeout kills the process and `cmd.Run` still does not return. Same reason and fix as in herdr.
// 250ms as a literal, same coverage argument as DefaultTimeout above.
const pipeCloseGrace = time.Duration(250e6)

type Runner struct {
	// Empty means "git" from PATH.
	Bin string
	// Non-positive means DefaultTimeout.
	Timeout time.Duration
}

func New() *Runner { return &Runner{Bin: "git", Timeout: DefaultTimeout} }

// Exit code and unwrapped cause are preserved so callers can classify a failure without matching on
// git's English message.
type Error struct {
	Args     []string
	Dir      string
	ExitCode int
	Msg      string
	Err      error
}

func (e *Error) Error() string {
	base := fmt.Sprintf("git %s: %s", strings.Join(e.Args, " "), e.Msg)
	if e.Dir != "" {
		base = fmt.Sprintf("git -C %s %s: %s", e.Dir, strings.Join(e.Args, " "), e.Msg)
	}
	if e.ExitCode != 0 {
		return fmt.Sprintf("%s (exit %d)", base, e.ExitCode)
	}
	return base
}

func (e *Error) Unwrap() error { return e.Err }

// An empty dir means the current directory. On failure it returns the partial output with the error.
func (r *Runner) Run(ctx context.Context, dir string, args ...string) (string, error) {
	bin := r.Bin
	if bin == "" {
		bin = "git"
	}
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cctx, bin, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = Env()
	// Without this the timeout cannot cut the read: see pipeCloseGrace.
	cmd.WaitDelay = pipeCloseGrace
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		msg := firstLine(strings.TrimSpace(errb.String()))
		if msg == "" {
			msg = err.Error()
		}
		gerr := &Error{Args: args, Dir: dir, Msg: msg, Err: err}
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			gerr.ExitCode = exit.ExitCode()
		}
		return out.String(), gerr
	}
	return out.String(), nil
}

// The GIT_* vars are stripped because they beat cmd.Dir: inheriting the caller's git context
// would land a UI action in a repo that is not the item's, which is the bug that deletes a branch.
func Env() []string {
	env := os.Environ()
	out := env[:0]
	for _, kv := range env {
		switch {
		case strings.HasPrefix(kv, "LC_ALL="),
			strings.HasPrefix(kv, "LANG="),
			strings.HasPrefix(kv, "LANGUAGE="),
			strings.HasPrefix(kv, "LC_MESSAGES="),
			strings.HasPrefix(kv, "GIT_DIR="),
			strings.HasPrefix(kv, "GIT_WORK_TREE="),
			strings.HasPrefix(kv, "GIT_INDEX_FILE="),
			strings.HasPrefix(kv, "GIT_COMMON_DIR="),
			strings.HasPrefix(kv, "GIT_OBJECT_DIRECTORY="),
			strings.HasPrefix(kv, "GIT_ALTERNATE_OBJECT_DIRECTORIES="),
			strings.HasPrefix(kv, "GIT_NAMESPACE="),
			strings.HasPrefix(kv, "GIT_CEILING_DIRECTORIES="),
			strings.HasPrefix(kv, "GIT_PREFIX="):
			continue
		}
		out = append(out, kv)
	}
	return append(out,
		"LC_ALL=C",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_PAGER=cat",
		"NO_COLOR=1",
	)
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
