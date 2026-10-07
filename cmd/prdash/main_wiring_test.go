package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"io"
	"prdash/internal/config"
	"prdash/internal/forge/model"
	"prdash/internal/review/plan"
	"prdash/internal/worktree"
)

// A path with a separator and a bare name are two different lookups, not one.
func TestBinaryAvailabilityDistinguishesTheThreeCases(t *testing.T) {
	for _, empty := range []string{"", "   ", "\t\n"} {
		if binaryAvailable(empty) {
			t.Errorf("binaryAvailable(%q) returned true: a pane without a command is not launched", empty)
		}
	}

	dir := t.TempDir()
	executable := filepath.Join(dir, "tool")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !binaryAvailable(executable) {
		t.Errorf("binaryAvailable(%q) returned false with the file right there", executable)
	}
	subdir := filepath.Join(dir, "asubdir")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if binaryAvailable(subdir) {
		t.Error("binaryAvailable returned true for a directory: a directory does not execute")
	}
	if binaryAvailable(filepath.Join(dir, "does-not-exist")) {
		t.Error("binaryAvailable returned true for a path that does not exist")
	}

	if !binaryAvailable("sh") {
		t.Error("binaryAvailable(\"sh\") returned false and sh is on everyone's PATH")
	}
	if binaryAvailable("definitely-not-a-real-binary") {
		t.Error("binaryAvailable returned true for a name that is not on the PATH")
	}
}

func TestHostsOfOnlyMapsWhatIsConfiguredAndInventsNoForge(t *testing.T) {
	if len(hostsOf(config.Config{})) != 0 {
		t.Errorf("without config gave %v, want an empty map", hostsOf(config.Config{}))
	}

	// The GitLab default is NOT gitlab.com: prdash's defaults point at an example self-managed GitLab.
	cfg := config.Defaults()
	hosts := hostsOf(cfg)
	if len(hosts) != 2 {
		t.Fatalf("with the two hosts it gave %d entries: %v", len(hosts), hosts)
	}
	if hosts["github.com"] != "github" {
		t.Errorf("github.com -> %q", hosts["github.com"])
	}
	if hosts[cfg.Forges.GitLab.Host] != "gitlab" {
		t.Errorf("%q -> %q, want gitlab", cfg.Forges.GitLab.Host, hosts[cfg.Forges.GitLab.Host])
	}
	for h := range hosts {
		if h == "" {
			t.Error("the map has an entry with an empty host")
		}
	}

	previous := cfg.Forges.GitLab.Host
	cfg.Forges.GitLab.Host = "git.intra.example"
	hosts = hostsOf(cfg)
	if hosts["git.intra.example"] != "gitlab" {
		t.Errorf("the self-managed host gave %q", hosts["git.intra.example"])
	}
	if _, remains := hosts[previous]; remains {
		t.Errorf("changing the host did not drop the previous one (%q is still in the map)", previous)
	}

	cfg = config.Defaults()
	cfg.Forges.GitHub.Host = "same.example"
	cfg.Forges.GitLab.Host = "same.example"
	if len(hostsOf(cfg)) != 1 {
		t.Errorf("two forges on the same host gave %d entries, want 1", len(hostsOf(cfg)))
	}
}

// Known:false with numbers arrives when the diffstat query fails and the figures default to zero.
func TestPrintDiffDistinguishesUnknownFromZero(t *testing.T) {
	cases := []struct {
		name string
		d    model.DiffStat
		want string
	}{
		{"normal known", model.DiffStat{Known: true, Additions: 12, Deletions: 3}, "+12 -3"},
		{"only additions", model.DiffStat{Known: true, Additions: 40}, "+40 -0"},
		{"only deletions", model.DiffStat{Known: true, Deletions: 7}, "+0 -7"},
		{"known zero", model.DiffStat{Known: true}, "+0 -0"},
		{"unknown", model.DiffStat{Known: false}, "-"},
		{"figures not marked as known", model.DiffStat{Additions: 5, Deletions: 5}, "-"},
		{"zero not marked", model.DiffStat{}, "-"},
	}
	for _, c := range cases {
		got := printDiff(c.d)
		if got != c.want {
			t.Errorf("%s: printDiff gave %q, want %q", c.name, got, c.want)
		}
		if strings.TrimSpace(got) == "" {
			t.Errorf("%s: printDiff returned empty text", c.name)
		}
	}
}

// state has three values and only two are listed, so the default is the notable one.
func TestTheAuditorMarksOrphansAndLeavesTheRestOk(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "wt")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	live := filepath.Join(dir, "live")
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatal(err)
	}

	entries := testProvisioner(t, live).Audit(context.Background())
	if len(entries) != 0 {
		t.Errorf("a directory without worktrees gave %d entries: %v", len(entries), entries)
	}

	repoEntries := testProvisioner(t, repo).Audit(context.Background())
	for _, e := range repoEntries {
		if e.Path == "" {
			t.Errorf("an entry without a Path: %+v", e)
		}
		if e.Orphan && e.Reason == "" {
			t.Errorf("an orphan entry without a reason: %+v", e)
		}
		if !e.Orphan {
			t.Errorf("a live worktree marked as orphan: %+v", e)
		}
	}
}

// This is the dangerous half of the command, so what is tested is the negative.
func TestRemovalRefusesWhatIsNotAnOwnedWorktree(t *testing.T) {
	dir := t.TempDir()
	foreign := filepath.Join(dir, "other-repo")
	if err := os.MkdirAll(foreign, 0o755); err != nil {
		t.Fatal(err)
	}

	pr := testProvisioner(t, dir)

	code := removeWorktreesWithin(pr, io.Discard, io.Discard, false, false, []string{foreign}, time.Second)
	if code == 0 {
		t.Error("removing someone else's repo returned 0")
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Errorf("the foreign repo is gone: %v", err)
	}

	if code := removeWorktreesWithin(pr, io.Discard, io.Discard, false, false, []string{filepath.Join(dir, "nothing")}, time.Second); code == 0 {
		t.Error("removing a nonexistent path returned 0")
	}

	before, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if code := removeWorktreesWithin(pr, io.Discard, io.Discard, false, false, []string{"absolutely-does-not-exist"}, time.Second); code == 0 {
		t.Error("a nonexistent relative path returned 0")
	}
	after, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Errorf("the directory entry count changed from %d to %d on a refusal",
			len(before), len(after))
	}

	if code := removeWorktreesWithin(pr, io.Discard, io.Discard, false, true, []string{foreign}, time.Second); code == 0 {
		t.Error("dry-run on a foreign repo returned 0: dry-run should not refuse, " +
			"but warn and touch nothing")
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Errorf("the dry-run deleted the foreign repo: %v", err)
	}
}

// A slow item must not spend the following ones' budget, not merely have a deadline.
func TestTheBudgetIsPerItemAndNotGlobal(t *testing.T) {
	pr := testProvisioner(t, t.TempDir())

	if code := removeWorktreesWithin(pr, io.Discard, io.Discard, false, false, []string{filepath.Join(t.TempDir(), "nothing")},
		time.Millisecond); code == 0 {
		t.Error("a nonexistent path with a tiny budget returned 0")
	}

	start := time.Now()
	removeWorktreesWithin(pr, io.Discard, io.Discard, true, true, nil, 2*time.Second)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("an orphan dry-run took %s with nothing to delete", elapsed)
	}
}

// Wiring is where a new config field ends up half-wired.
func TestTheExecutorBuildUsesTheConfig(t *testing.T) {
	// Emptying tools.agent does not leave it empty: ToolArgs falls back to the default name.
	cfg := config.Defaults()
	cfg.Tools.Tuicr = "/opt/tuicr --dark"
	cfg.Tools.Hunk = "hunk"
	cfg.Tools.Agent = ""

	tools := plan.Tools{
		Tuicr:  paneTool(cfg, "tuicr"),
		Hunk:   paneTool(cfg, "hunk"),
		Agent:  paneTool(cfg, "agent"),
		Editor: paneTool(cfg, "editor"),
	}

	if got := tools.Binary(plan.KindTuicr); got != "/opt/tuicr" {
		t.Errorf("the tuicr binary came out %q", got)
	}
	if got := tools.Binary(plan.KindHunk); got != "hunk" {
		t.Errorf("the hunk binary came out %q", got)
	}
	if got := tools.Binary(plan.KindAgent); got != "opencode" {
		t.Errorf("with tools.agent empty it gave %q, want the default name", got)
	}
	if got := tools.Binary(plan.Kind("nonexistent")); got != "" {
		t.Errorf("a nonexistent tool gave %q, want empty", got)
	}
	if binaryAvailable(tools.Binary(plan.Kind("nonexistent"))) {
		t.Error("an empty argv declared itself available")
	}

	ex := buildExecutor(config.Config{})
	if ex == nil {
		t.Fatal("buildExecutor returned nil")
	}
	if ex.Resolver == nil || ex.Worktrees == nil || ex.Herdr == nil {
		t.Errorf("the executor has a nil piece: %+v", ex)
	}
	svc := buildSimulator(config.Config{}, ex)
	if svc == nil {
		t.Fatal("buildSimulator returned nil")
	}
	loc := simLocator{ex: ex}
	if _, ok := loc.Locate(model.Item{}); ok {
		t.Error("the locator found a clone with no mounted review")
	}
}

func testProvisioner(t *testing.T, dir string) worktree.Provisioner {
	t.Helper()
	return worktree.Select(nil, dir)
}
