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

type testReport interface {
	Helper()
	Fatalf(format string, args ...any)
	Error(args ...any)
}

func abort(t testReport, err error) {
	t.Helper()
	if err == nil {
		return
	}
	t.Fatalf("testutil: %v", err)
}

func abortWith(t testReport, fn func() error) {
	t.Helper()
	abort(t, fn())
}

func RunGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := runGit(dir, args...)
	abort(t, err)
	return out
}

func runGit(dir string, args ...string) (string, error) {
	if err := checkDir(dir); err != nil {
		return "", fmt.Errorf("testutil: %w", err)
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = gitEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %v in %s: %w\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out)), nil
}

func checkDir(dir string) error {
	if dir == "" {
		return errors.New("empty dir: git would run in the real repo and write to its config")
	}
	fi, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("%q does not exist: %w", dir, err)
	}
	if !fi.IsDir() {
		return fmt.Errorf("%q is not a directory", dir)
	}
	return nil
}

func InitRepo(t *testing.T, dir string) {
	t.Helper()
	abortWith(t, func() error { return createRepoDir(dir) })
	RunGit(t, dir, "init", "-b", "main")
	RunGit(t, dir, "config", "user.email", "test@prdash.local")
	RunGit(t, dir, "config", "user.name", "prdash tests")
	RunGit(t, dir, "config", "commit.gpgsign", "false")
	disableAutoGC(t, dir)
}

func createRepoDir(dir string) error {
	return os.MkdirAll(dir, 0o755)
}

func disableAutoGC(t *testing.T, dir string) {
	t.Helper()
	RunGit(t, dir, "config", "gc.auto", "0")
}

func CommitFile(t *testing.T, dir, name, content, msg string) {
	t.Helper()
	abortWith(t, func() error { return commitFile(dir, name, content) })
	RunGit(t, dir, "add", "-A")
	RunGit(t, dir, "commit", "-m", msg)
}

func commitFile(dir, name, content string) error {
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("preparing the %s directory: %w", name, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

func InitBare(t *testing.T, dir string) {
	t.Helper()
	abortWith(t, func() error { return initBare(dir) })
	RunGit(t, dir, "init", "--bare", "-b", "main")
	disableAutoGC(t, dir)
}

func initBare(dir string) error {
	if dir == "" {
		return errors.New("empty dir: `git init --bare` would land in the real repo and set core.bare on it")
	}
	return os.MkdirAll(dir, 0o755)
}

func SetRemote(t *testing.T, dir, name, url string) {
	t.Helper()
	removeRemote(dir, name)
	_, err := runGit(dir, "remote", "add", name, url)
	abort(t, err)
}

func removeRemote(dir, name string) {
	rm := exec.Command("git", "remote", "remove", name)
	rm.Dir = dir
	rm.Env = gitEnv()
	_ = rm.Run() // it did not exist: not a failure
}

func Push(t *testing.T, dir string, args ...string) {
	t.Helper()
	all := append([]string{"push"}, args...)
	RunGit(t, dir, all...)
}

func RefExists(t *testing.T, dir, ref string) bool {
	t.Helper()
	abort(t, checkDir(dir))
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", ref)
	cmd.Dir = dir
	cmd.Env = gitEnv()
	return cmd.Run() == nil
}
