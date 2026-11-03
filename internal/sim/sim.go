package sim

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type Kind string

const (
	KindMerge  Kind = "merge"
	KindRebase Kind = "rebase"
)

func (k Kind) String() string { return string(k) }

type Spec struct {
	Kind Kind
	Ref  string
}

const (
	DefaultBin     = "git-sim"
	DefaultTimeout = time.Duration(60e9)

	// rebaseSince is the first release whose rebase renderer replays commits instead of indexing a short SHA list.
	rebaseSince = "0.4.0"

	versionTimeout = time.Duration(5e9)
	pipeCloseGrace = time.Duration(250e6)
)

type Runner struct {
	Bin      string
	Timeout  time.Duration
	lookPath func(string) (string, error)
}

func NewRunner() *Runner { return &Runner{Bin: DefaultBin, Timeout: DefaultTimeout} }

func (r *Runner) Available() bool {
	look := r.lookPath
	if look == nil {
		look = exec.LookPath
	}
	_, err := look(r.bin())
	return err == nil
}

func (r *Runner) bin() string {
	if r.Bin == "" {
		return DefaultBin
	}
	return r.Bin
}

// Supports answers whether this git-sim can draw the kind. A version it cannot read is a no.
func (r *Runner) Supports(ctx context.Context, kind Kind) bool {
	if kind == KindMerge {
		return true
	}
	if kind != KindRebase {
		return false
	}
	version, err := r.Version(ctx)
	if err != nil {
		return false
	}
	return atLeast(version, rebaseSince)
}

// Version reads the semver from `git-sim --version`, which prints "git-sim version X.Y.Z".
func (r *Runner) Version(ctx context.Context) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, versionTimeout)
	defer cancel()

	out, err := exec.CommandContext(cctx, r.bin(), "--version").Output()
	if err != nil {
		return "", err
	}
	for _, field := range strings.Fields(string(out)) {
		if isVersion(field) {
			return field, nil
		}
	}
	return "", fmt.Errorf("sim: cannot read the git-sim version from %q", strings.TrimSpace(string(out)))
}

func isVersion(field string) bool {
	parts := strings.Split(strings.TrimPrefix(field, "v"), ".")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if _, err := strconv.Atoi(part); err != nil {
			return false
		}
	}
	return true
}

func atLeast(have, want string) bool {
	h, w := versionParts(have), versionParts(want)
	for i := range h {
		if h[i] != w[i] {
			return h[i] > w[i]
		}
	}
	return true
}

func versionParts(v string) [3]int {
	var out [3]int
	for i, part := range strings.Split(strings.TrimPrefix(v, "v"), ".") {
		if i >= len(out) {
			break
		}
		out[i], _ = strconv.Atoi(part)
	}
	return out
}

type Error struct {
	Args     []string
	Dir      string
	ExitCode int
	Msg      string
	Err      error
}

func (e *Error) Error() string {
	base := fmt.Sprintf("git-sim %s: %s", strings.Join(e.Args, " "), e.Msg)
	if e.Dir != "" {
		base = fmt.Sprintf("git-sim -C %s %s: %s", e.Dir, strings.Join(e.Args, " "), e.Msg)
	}
	if e.ExitCode != 0 {
		return fmt.Sprintf("%s (exit %d)", base, e.ExitCode)
	}
	return base
}

func (e *Error) Unwrap() error { return e.Err }

func (r *Runner) args(spec Spec, mediaDir string) []string {
	return []string{
		"--output-only-path",
		"--no-animate",
		"--img-format", "html",
		"--open-in", "local",
		"--media-dir", mediaDir,
		string(spec.Kind), spec.Ref,
	}
}

func (r *Runner) Render(ctx context.Context, workdir, mediaDir string, spec Spec) (string, error) {
	if spec.Kind != KindMerge && spec.Kind != KindRebase {
		return "", fmt.Errorf("sim: unknown kind %q", spec.Kind)
	}
	if spec.Ref == "" {
		return "", errors.New("sim: no ref to simulate against")
	}

	timeout := r.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := r.args(spec, mediaDir)
	cmd := exec.CommandContext(cctx, r.bin(), args...)
	cmd.Dir = workdir
	cmd.Env = env()
	cmd.WaitDelay = pipeCloseGrace

	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		simErr := &Error{Args: args, Dir: workdir, Msg: message(errb.String(), out.String(), err), Err: err}
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			simErr.ExitCode = exit.ExitCode()
		}
		return "", simErr
	}
	image := imagePath(out.String())
	if image == "" {
		return "", &Error{Args: args, Dir: workdir, Msg: "git-sim produced no image"}
	}
	return image, nil
}

func env() []string {
	var out []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "git_sim_") {
			continue
		}
		out = append(out, kv)
	}
	return append(out, "git_sim_auto_open=false")
}

func indexErrorLine(stderr string) string {
	for _, line := range strings.Split(stderr, "\n") {
		if s := strings.TrimSpace(line); strings.Contains(s, "IndexError") {
			return s
		}
	}
	return ""
}

func message(stderr, stdout string, err error) string {
	if line := indexErrorLine(stderr); line != "" {
		return "git-sim crashed inside its renderer (" + line + "); upgrade to " + rebaseSince + " or newer"
	}
	for _, line := range strings.Split(stderr, "\n") {
		if s := strings.TrimSpace(line); s != "" {
			return s
		}
	}
	// git-sim prints its own user errors (unknown ref, already based) on stdout, not stderr.
	for _, line := range strings.Split(stdout, "\n") {
		if s := strings.TrimSpace(line); strings.HasPrefix(s, "git-sim error") {
			return s
		}
	}
	if err != nil {
		return err.Error()
	}
	return "unknown error"
}

func imagePath(out string) string {
	lines := strings.Split(out, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if s := strings.TrimSpace(lines[i]); s != "" {
			return s
		}
	}
	return ""
}
