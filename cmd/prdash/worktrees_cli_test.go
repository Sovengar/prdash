package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"prdash/internal/worktree"
)

func TestListSeparatesOutputFromWarningAndEachThingIntoItsStream(t *testing.T) {
	base, orphans, healthy, foreign := worktreeOrphanFixture(t)
	pr := worktree.NewGitDirect(base)

	var stdout, stderr bytes.Buffer
	if code := listWorktrees(pr, &stdout, &stderr); code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr.String())
	}

	table := stdout.String()
	for _, want := range []string{"WORKTREE", "BRANCH", "STATE", "PATH"} {
		if !strings.Contains(table, want) {
			t.Errorf("the table does not have the column %q:\n%s", want, table)
		}
	}
	for _, e := range append(append([]string{}, orphans...), healthy) {
		if !strings.Contains(table, e) {
			t.Errorf("the table does not list %s:\n%s", e, table)
		}
	}
	if strings.Contains(table, foreign) {
		t.Errorf("the table mentions the foreign worktree %s:\n%s", foreign, table)
	}
	if strings.Contains(stderr.String(), foreign) {
		t.Errorf("stderr mentions the foreign worktree %s", foreign)
	}

	warnings := stderr.String()
	for _, o := range orphans {
		if !strings.Contains(warnings, o) {
			t.Errorf("stderr does not warn about the orphan %s:\n%s", o, warnings)
		}
		if !strings.Contains(warnings, "orphaned") && !strings.Contains(warnings, "is orphaned") {
			t.Errorf("the warning for %s does not say it is orphaned:\n%s", o, warnings)
		}
	}
	if strings.Contains(warnings, healthy) {
		t.Errorf("stderr warns about the healthy worktree %s:\n%s", healthy, warnings)
	}
}

// The tabwriter is not cosmetic: it is what makes the output readable.
func TestListAlignsTheColumnsSoTheTableIsReadable(t *testing.T) {
	base, _, _ := worktreeFixture(t)
	pr := worktree.NewGitDirect(base)

	var stdout, stderr bytes.Buffer
	listWorktrees(pr, &stdout, &stderr)

	lines := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("output has %d lines, there is no table to align:\n%s", len(lines), stdout.String())
	}

	headerPositions := columnsOf(lines[0])
	if len(headerPositions) != 4 {
		t.Fatalf("the header has %d columns, want 4: %q", len(headerPositions), lines[0])
	}
	for _, row := range lines[1:] {
		for i, col := range columnsOf(row) {
			if i >= len(headerPositions) {
				t.Errorf("row %q has more columns than the header", row)
				break
			}
			if col != headerPositions[i] {
				t.Errorf("column %d of %q starts at %d and the header's at %d: the "+
					"table is not aligned", i, row, col, headerPositions[i])
			}
		}
	}
}

// The tabwriter pads with spaces, so the next column starts exactly where this says.
func columnsOf(row string) []int {
	out := []int{0}
	i := 0
	for i < len(row) {
		if row[i] != ' ' {
			i++
			continue
		}
		start := i
		for i < len(row) && row[i] == ' ' {
			i++
		}
		if i >= len(row) {
			break
		}
		out = append(out, start+(i-start))
	}
	return out
}

func TestWithNoWorktreesTheListSaysThereAreNoneAndIsNotAnError(t *testing.T) {
	pr := worktree.NewGitDirect(t.TempDir())

	var stdout, stderr bytes.Buffer
	if code := listWorktrees(pr, &stdout, &stderr); code != 0 {
		t.Errorf("code = %d with no worktrees, want 0", code)
	}
	if !strings.Contains(stdout.String(), "no prdash review worktrees") {
		t.Errorf("it does not say there are no worktrees: %q", stdout.String())
	}
	if stderr.String() != "" {
		t.Errorf("stderr = %q with no worktrees, want empty", stderr.String())
	}
}

// The property to pin is touching nothing, and it is checked by the tree being byte-identical.
func TestDryRunSaysWhatItWouldRemoveAndTouchesNothing(t *testing.T) {
	base, orphans, healthy, foreign := worktreeOrphanFixture(t)
	pr := worktree.NewGitDirect(base)

	var stdout, stderr bytes.Buffer
	code := removeWorkphans(pr, &stdout, &stderr, true)
	if code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr.String())
	}
	out := stdout.String()
	for _, o := range orphans {
		if !strings.Contains(out, "would remove: "+o) {
			t.Errorf("it does not say it would remove %s:\n%s", o, out)
		}
		if strings.Contains(out, "worktree removed: "+o) {
			t.Errorf("a dry run says it REMOVED it: %s", o)
		}
		if !worktree.Exists(o) {
			t.Errorf("--dry-run removed %s", o)
		}
	}
	if !worktree.Exists(healthy) || !worktree.Exists(foreign) {
		t.Error("--dry-run touched the healthy or the foreign worktree")
	}
}

// An empty batch is the happy path and must be quiet on stderr.
func TestWithNoOrphansTheBatchRemovalSaysThereAreNoneAndWritesNothingToStderr(t *testing.T) {
	base, _, _ := worktreeFixture(t)
	pr := worktree.NewGitDirect(base)

	for _, dryRun := range []bool{false, true} {
		var stdout, stderr bytes.Buffer
		if code := removeOrphans(pr, &stdout, &stderr, dryRun, worktreeTimeout); code != 0 {
			t.Errorf("dryRun=%v: code = %d, zero orphans is the happy path", dryRun, code)
		}
		if !strings.Contains(stdout.String(), "no orphaned prdash worktrees") {
			t.Errorf("dryRun=%v: it does not say there are no orphans: %q", dryRun, stdout.String())
		}
		if stderr.String() != "" {
			t.Errorf("dryRun=%v: stderr = %q with zero orphans, want empty",
				dryRun, stderr.String())
		}
	}
}

// It decides whether a user's work survives a partly-failing batch.
func TestRejectedAndTouchedPathsGoToStderrAndTheCodeSkipsButTheRestIsRemoved(t *testing.T) {
	base, orphans, healthy, foreign := worktreeOrphanFixture(t)
	pr := worktree.NewGitDirect(base)
	// User work under the root with the prefix's name: the case an rm -rf would take with it.
	userOwned := filepath.Join(base, "prdash-user-owned")
	if err := os.MkdirAll(userOwned, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(userOwned, "work.txt"),
		[]byte("do not delete me"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := removeWorktreesWithin(pr, &stdout, &stderr, false, false,
		[]string{orphans[0], foreign, "/nonexistent/absolute/path", orphans[1]}, 10*time.Second)

	if code != 1 {
		t.Errorf("code = %d with two rejected paths, want 1", code)
	}
	errOut := stderr.String()
	for _, rejected := range []string{foreign, "/nonexistent/absolute/path"} {
		if !strings.Contains(errOut, rejected) {
			t.Errorf("stderr does not name the rejected path %s:\n%s", rejected, errOut)
		}
		if !strings.Contains(errOut, "leaving it alone") {
			t.Errorf("the rejection of %s does not say it is left alone:\n%s", rejected, errOut)
		}
	}
	out := stdout.String()
	for _, o := range orphans {
		if !strings.Contains(out, "worktree removed: "+o) {
			t.Errorf("stdout does not say that %s was removed:\n%s", o, out)
		}
		if worktree.Exists(o) {
			t.Errorf("%s is still there after removing it", o)
		}
	}
	// Owned must recognise this by the label, not by the directory name.
	for _, untouched := range []string{foreign, healthy} {
		if !worktree.Exists(untouched) {
			t.Errorf("%s was removed, and it was not prdash's", untouched)
		}
	}
	if _, err := os.Stat(filepath.Join(userOwned, "work.txt")); err != nil {
		t.Errorf("it deleted the user's work that only looked like a worktree: %v", err)
	}
}

func TestTheDispatchWritesEachErrorToItsStream(t *testing.T) {
	base, _, healthy, foreign := worktreeOrphanFixture(t)
	pr := worktree.NewGitDirect(base)

	for _, c := range []struct {
		name    string
		args    []string
		want    int
		aStderr string
		aStdout string
	}{
		{"unknown subcommand", []string{"made-up"}, 2, "unknown subcommand", ""},
		{"list with no provisioner at all", []string{"list"}, 0, "", "WORKTREE"},
		{"remove with no paths", []string{"remove"}, 2, "missing at least one path", ""},
		{"unknown flag", []string{"remove", "--made-up"}, 2, "unknown flag", ""},
		{"typo of orphans", []string{"remove", "--orphan"}, 2, "unknown flag", ""},
		{"dry-run without orphans", []string{"remove", "--dry-run"}, 2, "requires --orphans", ""},
		{"both modes mixed", []string{"remove", "--orphans", "x"}, 2, "cannot be mixed", ""},
		{"path starting with a dash", []string{"remove", "-prdash-1"}, 2, "unknown flag", ""},
		{"empty subcommand", nil, 0, "", "WORKTREE"},
	} {
		var stdout, stderr bytes.Buffer
		code := runWorktrees(pr, &stdout, &stderr, c.args)
		if code != c.want {
			t.Errorf("%s: code = %d, want %d (stderr: %s)", c.name, code, c.want, stderr.String())
		}
		if c.aStderr != "" && !strings.Contains(stderr.String(), c.aStderr) {
			t.Errorf("%s: stderr = %q, want it to contain %q", c.name, stderr.String(), c.aStderr)
		}
		if c.aStdout != "" && !strings.Contains(stdout.String(), c.aStdout) {
			t.Errorf("%s: stdout = %q, want it to contain %q", c.name, stdout.String(), c.aStdout)
		}
		if code == 2 && stdout.String() != "" {
			t.Errorf("%s: a usage error wrote to stdout: %q", c.name, stdout.String())
		}
	}

	var stdout, stderr bytes.Buffer
	if code := runWorktrees(pr, &stdout, &stderr, []string{"remove", "--orphans", "--dry-run"}); code != 0 {
		t.Errorf("a well-formed dry-run batch gave code %d", code)
	}
	if !worktree.Exists(healthy) || !worktree.Exists(foreign) {
		t.Error("the dry-run batch touched something: Exists returning false means it is gone")
	}
}

// Not worktreeTimeout (30s), because a failing test would then hang for 30s.
func removeWorkphans(pr worktree.Provisioner, stdout, stderr io.Writer, dryRun bool) int {
	return removeOrphans(pr, stdout, stderr, dryRun, 10*time.Second)
}
