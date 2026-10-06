package gitcmd

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Three pieces, not one, because each answers a different question about a git failure.
func TestTheErrorMessageBringsWhatIsNeededToFixIt(t *testing.T) {
	cases := []struct {
		name string
		e    *Error
		want string
	}{
		{
			name: "with dir and exit code",
			e:    &Error{Args: []string{"rev-parse", "HEAD"}, Dir: "/repos/proj", Msg: "fatal: not a git repository", ExitCode: 128},
			want: "git -C /repos/proj rev-parse HEAD: fatal: not a git repository (exit 128)",
		},
		{
			name: "without dir",
			e:    &Error{Args: []string{"status"}, Msg: "no changes", ExitCode: 1},
			want: "git status: no changes (exit 1)",
		},
		{
			name: "error without exit code",
			e:    &Error{Args: []string{"log"}, Msg: "interrupted"},
			want: "git log: interrupted",
		},
		{
			name: "no arguments",
			e:    &Error{Msg: "something"},
			want: "git : something",
		},
		{
			name: "several arguments",
			e:    &Error{Args: []string{"worktree", "add", "-b", "b", "/r"}, Msg: "already exists"},
			want: "git worktree add -b b /r: already exists",
		},
	}
	for _, c := range cases {
		if got := c.e.Error(); got != c.want {
			t.Errorf("%s: Error() gave %q, want %q", c.name, got, c.want)
		}
	}
}

// Unwrap is what lets us say "this is not git's error": errors.As(err, &exitError) needs the
// chain mounted.
func TestTheErrorUnwrapsTheCause(t *testing.T) {
	withoutCause := &Error{Args: []string{"x"}, Msg: "m"}
	if withoutCause.Unwrap() != nil {
		t.Error("Unwrap returned something in an Error with no cause")
	}
	if errors.Unwrap(withoutCause) != nil {
		t.Error("errors.Unwrap returned something in an Error with no cause")
	}

	cmd := exec.Command("sh", "-c", "exit 42")
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected sh -c 'exit 42' to fail")
	}
	withCause := &Error{Args: []string{"x"}, Msg: "m", Err: err}

	if !errors.Is(withCause, err) {
		t.Error("errors.Is does not reach the cause")
	}
	var exit *exec.ExitError
	if !errors.As(withCause, &exit) {
		t.Fatal("errors.As does not reach the *exec.ExitError: without it ExitCode is always 0")
	}
	if exit.ExitCode() != 42 {
		t.Errorf("ExitCode = %d, want 42", exit.ExitCode())
	}
	if !strings.Contains(withCause.Error(), "m") {
		t.Errorf("the message lost its own text: %q", withCause.Error())
	}
}

func TestRunBringsTheRealGitError(t *testing.T) {
	dir := emptyRepo(t)

	_, err := New().Run(context.Background(), dir, "cat-file", "-p", "does-not-exist")
	if err == nil {
		t.Fatal("git cat-file on a nonexistent object gave nil")
	}
	gerr, ok := err.(*Error)
	if !ok {
		t.Fatalf("Run returned %T, want *Error", err)
	}
	if gerr.ExitCode == 0 {
		t.Error("ExitCode = 0 on a git failure: the exit code was not read")
	}
	if strings.HasPrefix(gerr.Msg, "exit status") {
		t.Errorf("the message is %q: the process error was taken instead of its stderr", gerr.Msg)
	}
	if strings.TrimSpace(gerr.Msg) == "" {
		t.Error("the message ended up empty")
	}
	if strings.Contains(gerr.Msg, "\n") {
		t.Errorf("the message has line breaks: %q", gerr.Msg)
	}
	if !strings.Contains(gerr.Error(), "cat-file") {
		t.Errorf("Error() does not name the command: %q", gerr.Error())
	}
	if !strings.Contains(gerr.Error(), dir) {
		t.Errorf("Error() does not name the directory: %q", gerr.Error())
	}
}

// Returning what was written before the failure is the right behaviour for what prdash does with
// git.
func TestRunReturnsPartialOutputWhenItFails(t *testing.T) {
	dir := t.TempDir()
	partial := filepath.Join(dir, "git-partial")
	if err := os.WriteFile(partial, []byte("#!/bin/sh\necho 'good line'\necho 'fatal: it broke' >&2\nexit 7\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	out, err := (&Runner{Bin: partial}).Run(context.Background(), dir, "log")
	if err == nil {
		t.Fatal("a binary that exits with 7 gave nil")
	}
	if !strings.Contains(out, "good line") {
		t.Errorf("the output before the failure was lost: %q", out)
	}
	gerr, ok := err.(*Error)
	if !ok {
		t.Fatalf("Run returned %T, want *Error", err)
	}
	if gerr.ExitCode != 7 {
		t.Errorf("ExitCode = %d, want 7", gerr.ExitCode)
	}
	if !strings.Contains(gerr.Msg, "it broke") {
		t.Errorf("the message does not carry the stderr: %q", gerr.Msg)
	}

	if err := os.WriteFile(partial, []byte("#!/bin/sh\nexit 7\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, err = (&Runner{Bin: partial}).Run(context.Background(), dir, "log")
	if err == nil {
		t.Fatal("a binary that exits with 7 gave nil")
	}
	if out != "" {
		t.Errorf("a binary that wrote nothing returned %q", out)
	}
	if strings.TrimSpace(gerr.Msg) == "" {
		t.Error("the message from the first case ended up empty")
	}
}

func TestTheHappyPathBringsTheWholeOutput(t *testing.T) {
	dir := emptyRepo(t)
	r := New()

	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), dir, "add", "a.txt"); err != nil {
		t.Fatalf("git add: %v", err)
	}
	out, err := r.Run(context.Background(), dir, "diff", "--cached", "--stat")
	if err != nil {
		t.Fatalf("git diff --cached: %v", err)
	}
	if !strings.Contains(out, "a.txt") {
		t.Errorf("git's output did not arrive intact: %q", out)
	}
}

// The order matters: the width trim first, then the first-line cut.
func TestFirstLineCutsWhatDoesNotFitInTheToast(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"first\nsecond\nthird", "first"},
		{"  with spaces  \nsecond", "  with spaces  "},
		{"one single line", "one single line"},
		{"", ""},
		{"\nstarts with newline", ""},
		{"with\n", "with"},
		{"trailing  \n", "trailing  "},
	}
	for _, c := range cases {
		if got := firstLine(c.input); got != c.want {
			t.Errorf("firstLine(%q) gave %q, want %q", c.input, got, c.want)
		}
		if strings.ContainsAny(firstLine(c.input), "\n") {
			t.Errorf("firstLine(%q) returned text with a newline", c.input)
		}
	}
}

// The most important thing Env does.
func TestTheGitEnvironmentDoesNotInheritTheShellContext(t *testing.T) {
	hostile := map[string]string{
		"GIT_DIR":                          "/other/repo/.git",
		"GIT_WORK_TREE":                    "/other/repo",
		"GIT_INDEX_FILE":                   "/other/index",
		"GIT_COMMON_DIR":                   "/other/common",
		"GIT_OBJECT_DIRECTORY":             "/other/objs",
		"GIT_ALTERNATE_OBJECT_DIRECTORIES": "/other/alt",
		"GIT_NAMESPACE":                    "other",
		"GIT_CEILING_DIRECTORIES":          "/other/ceiling",
		"GIT_PREFIX":                       "src/",
		"LC_ALL":                           "es_ES.UTF-8",
		"LANG":                             "es_ES.UTF-8",
		"LANGUAGE":                         "es",
		"LC_MESSAGES":                      "es_ES.UTF-8",
	}
	for k, v := range hostile {
		t.Setenv(k, v)
	}

	env := Env()
	seen := map[string]string{}
	for _, kv := range env {
		if i := strings.IndexByte(kv, '='); i > 0 {
			seen[kv[:i]] = kv[i+1:]
		}
	}

	for k := range hostile {
		if _, stillPresent := seen[k]; stillPresent {
			if k == "LC_ALL" || k == "LANG" {
				continue
			}
			t.Errorf("%s is still in the environment with the value %q: git would operate in "+
				"that context instead of the one it is asked for", k, seen[k])
		}
	}

	for k, want := range map[string]string{
		"LC_ALL":              "C",
		"GIT_TERMINAL_PROMPT": "0",
		"GIT_PAGER":           "cat",
		"NO_COLOR":            "1",
	} {
		if seen[k] != want {
			t.Errorf("%s = %q, want %q", k, seen[k], want)
		}
	}
	if _, stillPresent := seen["LANG"]; stillPresent {
		t.Error("LANG is still present: it competes with the LC_ALL that is set")
	}

	t.Setenv("PRDASH_TEST_QUE_SI_SE_CONSERVA", "value")
	seen = map[string]string{}
	for _, kv := range Env() {
		if i := strings.IndexByte(kv, '='); i > 0 {
			seen[kv[:i]] = kv[i+1:]
		}
	}
	if seen["PRDASH_TEST_QUE_SI_SE_CONSERVA"] != "value" {
		t.Error("Env dropped a variable it should not have: the environment is over-filtered")
	}
}

func TestTheRunTimeoutActuallyCuts(t *testing.T) {
	dir := t.TempDir()
	hung := filepath.Join(dir, "git-hung")
	script := "#!/bin/sh\nsh -c 'sleep 5' &\nsleep 5\n"
	if err := os.WriteFile(hung, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	r := &Runner{Bin: hung, Timeout: 50 * time.Millisecond}
	start := time.Now()
	_, err := r.Run(context.Background(), dir, "status")
	elapsed := time.Since(start)

	if err == nil {
		t.Error("a hung git gave nil")
	}
	if elapsed > 2*time.Second {
		t.Errorf("Run took %s with a 50ms timeout: the context does not cut the call, it only "+
			"kills the process", elapsed)
	}

	short := filepath.Join(dir, "git-short")
	if err := os.WriteFile(short, []byte("#!/bin/sh\nsleep 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	r = &Runner{Bin: short}
	start = time.Now()
	if _, err := r.Run(context.Background(), dir, "status"); err != nil {
		t.Errorf("with the default floor and a healthy binary it errored: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 900*time.Millisecond {
		t.Errorf("with an empty Timeout it took %s: a short timeout was applied instead of the floor",
			elapsed)
	}
}

// A name and not a resolved path, so the Runner normalises it.
func TestTheDefaultBinaryIsGitAndNotTheEmptyField(t *testing.T) {
	dir := emptyRepo(t)
	if _, err := (&Runner{}).Run(context.Background(), dir, "rev-parse", "--git-dir"); err != nil {
		t.Fatalf("without Bin, Run failed: %v", err)
	}
	if got := New().Bin; got != "git" {
		t.Errorf("New().Bin = %q, want git", got)
	}
	if got := New().Timeout; got != DefaultTimeout {
		t.Errorf("New().Timeout = %v, want DefaultTimeout", got)
	}
}

func emptyRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	r := New()
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "t@example.com"},
		{"config", "user.name", "Test"},
	} {
		if _, err := r.Run(context.Background(), dir, args...); err != nil {
			t.Fatalf("git %s: %v", strings.Join(args, " "), err)
		}
	}
	return dir
}
