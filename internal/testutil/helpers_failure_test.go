package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const failureCaseEnv = "PRDASH_TESTUTIL_FAILURE_CASE"

// The seven t.Fatal paths.
func TestTheHelpersAbortLoudlyInsteadOfReturningThings(t *testing.T) {
	if c := os.Getenv(failureCaseEnv); c != "" {
		runFailureCase(t, c)
		// If the helper did NOT abort we get here, and `t.Failed()` is what has to be checked rather
		//than taking the case as good.
		if !t.Failed() {
			t.Fatalf("case %q did NOT abort: the helper returned instead of failing", c)
		}
		return
	}

	for _, c := range failureCases() {
		t.Run(c.name, func(t *testing.T) {
			output, code := runCaseInSubprocess(t, c.name)

			if code == 0 {
				t.Fatalf("helper %s did NOT abort. A broken fixture that does not abort makes "+
					"the following tests check an empty repo —or worse, pass by accident while "+
					"testing nothing.\n%s", c.name, output)
			}
			// And it says WHY: an abort with no reason is a "test failed" with no clue.
			if !strings.Contains(output, c.want) {
				t.Errorf("it aborted but without saying %q:\n%s", c.want, output)
			}
		})
	}
}

type failureCase struct {
	name string
	want string
}

func failureCases() []failureCase {
	return []failureCase{
		{"git-fails", "exit status"},
		{"missing-dir", "does not exist"},
		{"empty-dir", "empty dir"},
		{"file-instead-of-dir", "is not a directory"},
		// ENOTDIR and not EACCES: the obstruction is a FILE where the helper wants a directory.
		{"commit-parent-blocked", "not a directory"},
		{"commit-destination-directory", "is a directory"},
		{"empty-bare", "the real repo"},
		{"bare-parent-blocked", "not a directory"},
		{"broken-conformance", "should report unsupported"},
	}
}

func runFailureCase(t *testing.T, c string) {
	t.Helper()
	switch c {
	case "git-fails":
		repo := testRepo(t)
		RunGit(t, repo, "checkout", "no-such-branch")

	case "missing-dir":
		// The guard that has already passed for real: without it git would run in the process's own
		// directory.
		RunGit(t, filepath.Join(t.TempDir(), "does-not-exist"), "status")

	case "empty-dir":
		RunGit(t, "", "status")

	case "file-instead-of-dir":
		file := filepath.Join(t.TempDir(), "I-am-a-file")
		if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		RunGit(t, file, "status")

	case "commit-parent-blocked":
		repo := testRepo(t)
		blocked := filepath.Join(repo, "blocked")
		if err := os.WriteFile(blocked, []byte("I am a file"), 0o644); err != nil {
			t.Fatal(err)
		}
		CommitFile(t, repo, "blocked/uploaded.txt", "x", "cannot be done")

	case "commit-destination-directory":
		repo := testRepo(t)
		if err := os.MkdirAll(filepath.Join(repo, "I-am-a-dir"), 0o755); err != nil {
			t.Fatal(err)
		}
		CommitFile(t, repo, "I-am-a-dir", "x", "a destination that is a directory")

	case "empty-bare":
		InitBare(t, "")

	case "bare-parent-blocked":
		blocked := filepath.Join(t.TempDir(), "blocked")
		if err := os.WriteFile(blocked, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		InitBare(t, filepath.Join(blocked, "repo.git"))

	case "broken-conformance":
		// An adapter that does not support an operation has to SAY SO, not return an empty list as if
		//nothing were there.
		RunConformance(t, &failingAdapter{broken: "no-warning"},
			ConformanceOptions{Unsupported: true})
	}
}

func testRepo(t *testing.T) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "repo")
	InitRepo(t, repo)
	CommitFile(t, repo, "a.txt", "a", "a")
	return repo
}

func runCaseInSubprocess(t *testing.T, c string) (string, int) {
	t.Helper()
	cmd := exec.Command(os.Args[0],
		"-test.run=^TestTheHelpersAbortLoudlyInsteadOfReturningThings$",
		"-test.v")
	cmd.Env = append(os.Environ(), failureCaseEnv+"="+c)
	out, err := cmd.CombinedOutput()

	code := 0
	var outErr *exec.ExitError
	if err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			outErr = e
			code = e.ExitCode()
		} else {
			t.Fatalf("the subprocess could not be run: %v", err)
		}
	}
	_ = outErr
	// The failure has to be the abort's, not a panic from the child; a panic hangs the test.
	if strings.Contains(string(out), "panic:") {
		t.Errorf("the subprocess for case %s panicked instead of aborting the test:\n%s",
			c, out)
	}
	return string(out), code
}
