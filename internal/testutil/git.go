package testutil

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The suite never inherits the machine's git config. That config is global and mutable and the repos the
// tests build are real repos, so as soon as part of it reaches a clone the result depends on who ran the
// suite. Identity is the concrete case: a clone does NOT inherit user.name/user.email from its origin
// (they live in that repo's local config), so a commit inside a clone ends up with no author — the first
// prdash CI died on that.
//
// Identity goes through GIT_AUTHOR_*/GIT_COMMITTER_* instead of `git config` per repo, so it holds for
// every repo the test creates, clones included, without each new helper having to remember. Requires git
// >= 2.32; older builds ignore those variables and the tests still hold, since the identity is in the
// environment.
//
// The location GIT_* are filtered because they beat cmd.Dir: with GIT_DIR set, `git config` writes in
// THAT repo whatever directory the command runs in, so a suite launched from inside a hook would write
// the config of a stranger's repo. Same filter as gitcmd.Env() does in production, and a real repo does
// not need GIT_DIR: cmd.Dir is enough.
func gitEnv() []string {
	env := os.Environ()
	out := env[:0]
	for _, kv := range env {
		switch {
		case strings.HasPrefix(kv, "GIT_DIR="),
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
		"GIT_TERMINAL_PROMPT=0",
		"LC_ALL=C",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=prdash tests",
		"GIT_AUTHOR_EMAIL=test@prdash.local",
		"GIT_COMMITTER_NAME=prdash tests",
		"GIT_COMMITTER_EMAIL=test@prdash.local",
	)
}

// An interface rather than *testing.T because *testing.T CANNOT be DOUBLED: Fatalf ends in
// runtime.Goexit and testing.common has private fields. Without a double, proving that a helper aborts on
// a broken fixture means having a failing test, and a red test proves nothing — it reads as a failure of
// the helper. With the interface the reporting path runs in process against a double that records the
// call and RETURNS, which is what lets the test carry on and then check that it warned exactly once.
//
// Applied only where it is needed, `aborta` and `RunConformance`: widening every helper would change half
// the package's API for nothing.
type testReport interface {
	Helper()
	Fatalf(format string, args ...any)
	Error(args ...any)
}

// The only place a helper in this package calls t.Fatal, and one on purpose: with six, "what does a helper
// do when its fixture does not fit?" has six answers in the file and a test seeing two different messages
// for the same failure cannot tell two failures from one.
//
// The prefix lives here and not at each call site for the same reason: a prefix written six times is
// forgotten once, and a fixture test seeing `mkdir: permission denied` instead of `testutil: ...` cannot
// tell whether the failure is the helper's or the system's.
func aborta(t testReport, err error) {
	t.Helper()
	if err == nil {
		return
	}
	t.Fatalf("testutil: %v", err)
}

func abortaCon(t testReport, fn func() error) {
	t.Helper()
	aborta(t, fn())
}

// An empty dir would make git run in the test process's directory — the package's, which lives INSIDE
// the repo — and a fixture's `git config` or `git remote` would write to the real repo. It happened: a
// Green's suite left the repo with core.bare=true and an origin pointing at a TempDir.
func RunGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := runGit(dir, args...)
	aborta(t, err)
	return out
}

// A guard that aborts the test cannot be checked without a subprocess, because t.Fatal kills the
// goroutine and testing.TB makes a poor double (private method). With the core returning an error, the
// test asserts the reason in process; the wrappers are checked separately, by subprocess.
// The error carries the args and git's output, since without them an "exit status 128" says neither what
// failed nor what answered.
func runGit(dir string, args ...string) (string, error) {
	if err := checkDir(dir); err != nil {
		return "", fmt.Errorf("testutil: %w", err)
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = gitEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %v en %s: %w\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out)), nil
}

func checkDir(dir string) error {
	if dir == "" {
		return errors.New("dir vacío: git correría en el repo real y escribiría en su config")
	}
	fi, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("%q no existe: %w", dir, err)
	}
	if !fi.IsDir() {
		return fmt.Errorf("%q no es un directorio", dir)
	}
	return nil
}

func InitRepo(t *testing.T, dir string) {
	t.Helper()
	abortaCon(t, func() error { return creaRepoDir(dir) })
	RunGit(t, dir, "init", "-b", "main")
	RunGit(t, dir, "config", "user.email", "test@prdash.local")
	RunGit(t, dir, "config", "user.name", "prdash tests")
	RunGit(t, dir, "config", "commit.gpgsign", "false")
	desactivaAutoGC(t, dir)
}

func creaRepoDir(dir string) error {
	return os.MkdirAll(dir, 0o755)
}

// Measured: a reporesolver test doing two pushes and two fetches triggers it about one run in
// several.
func desactivaAutoGC(t *testing.T, dir string) {
	t.Helper()
	RunGit(t, dir, "config", "gc.auto", "0")
}

func CommitFile(t *testing.T, dir, name, content, msg string) {
	t.Helper()
	abortaCon(t, func() error { return commitFile(dir, name, content) })
	RunGit(t, dir, "add", "-A")
	RunGit(t, dir, "commit", "-m", msg)
}

// Two guards because they fail for different reasons: the parent may not exist (a `name` with a path)
// or something may sit where the directory should be. The second is the one that sneaks into
// fixtures, because CommitFile(repo, "blocked/sub.txt", …) with `blocked` being a file compiles,
// reads well and only fails when run.
func commitFile(dir, name, content string) error {
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("preparar el directorio de %s: %w", name, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("escribir %s: %w", path, err)
	}
	return nil
}

func InitBare(t *testing.T, dir string) {
	t.Helper()
	abortaCon(t, func() error { return initBare(dir) })
	RunGit(t, dir, "init", "--bare", "-b", "main")
	desactivaAutoGC(t, dir)
}

// The `dir == ""` guard is the one that cost the most: without it `git init --bare` runs in the test
// process's directory, which lives INSIDE the repo, and sets core.bare=true on the real repo being read.
// It happened for real.
func initBare(dir string) error {
	if dir == "" {
		return errors.New("dir vacío: `git init --bare` caería en el repo real y le pondría core.bare")
	}
	return os.MkdirAll(dir, 0o755)
}

func SetRemote(t *testing.T, dir, name, url string) {
	t.Helper()
	quitaRemote(dir, name)
	_, err := runGit(dir, "remote", "add", name, url)
	aborta(t, err)
}

// The error is ignored on purpose: a remote that did not exist is the normal case on the first call,
// and aborting would force every test to check before setting.
func quitaRemote(dir, name string) {
	rm := exec.Command("git", "remote", "remove", name)
	rm.Dir = dir
	rm.Env = gitEnv()
	_ = rm.Run() // no existía: no es un fallo
}

func Push(t *testing.T, dir string, args ...string) {
	t.Helper()
	all := append([]string{"push"}, args...)
	RunGit(t, dir, all...)
}

func RefExists(t *testing.T, dir, ref string) bool {
	t.Helper()
	aborta(t, checkDir(dir))
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", ref)
	cmd.Dir = dir
	cmd.Env = gitEnv()
	return cmd.Run() == nil
}
