// Package sim renders a simulation with git-sim. It is a renderer, not a simulator, and the picture
// is the only trace of that work, so nothing here reads its output to decide anything.
package sim

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

type Kind string

const (
	KindMerge Kind = "merge"
	// activa.
	KindRebase Kind = "rebase"
)

// git-sim.
func (k Kind) String() string { return string(k) }

// The ref is not the same branch in both cases, which is why the asker supplies it: a merge
// integrates the ref into the active branch, a rebase rebases it onto it.
type Spec struct {
	Kind Kind
	Ref  string
}

const (
	DefaultBin = "git-sim"
	// A healthy render takes a couple of seconds; the limit exists for the failure mode where git-sim hangs
	// handing the image to the desktop viewer, which never returns without a display.
	DefaultTimeout = 60 * time.Second

	// Same reason and same fix as in herdr: a child holding our pipes keeps `cmd.Run` from returning.
	pipeCloseGrace = 250 * time.Millisecond
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

// The global options go BEFORE the subcommand, whose group rejects them. `--quiet` cannot be asked
// with `--output-only-path`: the path is printed only when git-sim is not silent.
func (r *Runner) args(spec Spec, mediaDir string) []string {
	return []string{
		"--output-only-path",
		"--no-animate",
		"--media-dir", mediaDir,
		string(spec.Kind), spec.Ref,
	}
}

// The image only exists while workdir/media is still there, and its characteristic failure is not
// finishing badly but not finishing: the timeout is the only recovery.
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
	// Without this the timeout only kills the process: a render that leaves a child with the descriptors
	// open keeps `cmd.Run` waiting.
	cmd.WaitDelay = pipeCloseGrace

	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		simErr := &Error{Args: args, Dir: workdir, Msg: message(errb.String(), err), Err: err}
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

// git_sim_auto_open=false is not optional: git-sim ends by handing the image to the desktop
// viewer, which never returns without a display. It has no command-line form, only git_sim_* env vars.
func env() []string {
	out := make([]string, 0, len(os.Environ())+1)
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "git_sim_") {
			continue
		}
		out = append(out, kv)
	}
	return append(out, "git_sim_auto_open=false")
}

func message(stderr string, err error) string {
	for _, line := range strings.Split(stderr, "\n") {
		if s := strings.TrimSpace(line); s != "" {
			return s
		}
	}
	if err != nil {
		return err.Error()
	}
	return "unknown error"
}

// The last non-empty line: with --output-only-path it is the only one written, but a manim complaining
// on stdout must not displace the result.
func imagePath(out string) string {
	lines := strings.Split(out, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if s := strings.TrimSpace(lines[i]); s != "" {
			return s
		}
	}
	return ""
}
