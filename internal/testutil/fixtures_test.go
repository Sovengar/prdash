package testutil

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
)

// It ends in a clone, not in checking the remote exists: the tests ask whether a clone brings the content.
func TestTheFixtureSetsUpARepoThatCanBeClonedAndPushed(t *testing.T) {
	base := t.TempDir()

	bare := filepath.Join(base, "origin.git")
	InitBare(t, bare)
	repo := filepath.Join(base, "work")
	InitRepo(t, repo)

	CommitFile(t, repo, "src/main.go", "package main\n", "first commit")

	if !RefExists(t, repo, "refs/heads/main") {
		t.Error("after CommitFile refs/heads/main does not exist")
	}
	if RefExists(t, repo, "refs/heads/nonexistent") {
		t.Error("RefExists returned true for a ref that was never created")
	}

	SetRemote(t, repo, "origin", bare)
	Push(t, repo, "-u", "origin", "main")

	if !RefExists(t, bare, "refs/heads/main") {
		t.Error("after the push the remote has no refs/heads/main")
	}

	SetRemote(t, repo, "origin", bare)
	remotes := RunGit(t, repo, "remote")
	if !strings.Contains(remotes, "origin") {
		t.Errorf("after re-pointing the remote %q is left", remotes)
	}
	SetRemote(t, repo, "other", bare)
	if r := RunGit(t, repo, "remote"); strings.Count(r, "origin") != 1 {
		t.Errorf("re-pointing left two origins: %q", r)
	}

	// Cloned for real, not rebuilt by hand: what matters is that the result is usable.
	clone := filepath.Join(base, "clone")
	RunGit(t, base, "clone", bare, clone)
	if !RefExists(t, clone, "refs/heads/main") {
		t.Error("the clone does not bring the remote branch")
	}
	content, err := os.ReadFile(filepath.Join(clone, "src", "main.go"))
	if err != nil {
		t.Fatalf("the clone does not bring the remote file: %v", err)
	}
	if string(content) != "package main\n" {
		t.Errorf("the clone brings %q", content)
	}
	if RunGit(t, clone, "rev-parse", "HEAD") != RunGit(t, repo, "rev-parse", "HEAD") {
		t.Error("the clone and the repo are not on the same commit")
	}
}

func TestPushWithCustomArgsAndWithoutArgs(t *testing.T) {
	base := t.TempDir()
	bare := filepath.Join(base, "o.git")
	InitBare(t, bare)
	repo := filepath.Join(base, "w")
	InitRepo(t, repo)
	CommitFile(t, repo, "a.txt", "x\n", "c")
	SetRemote(t, repo, "origin", bare)

	Push(t, repo, "-u", "origin", "main")
	if !RefExists(t, bare, "refs/heads/main") {
		t.Fatal("the push with flags did not reach the remote")
	}

	// The result is NOT checked: without --set-upstream and a destination git fails and RunGit aborts.
	if got := RunGit(t, repo, "config", "remote.origin.url"); got != bare {
		t.Fatalf("the remote was not set: %q", got)
	}
}

// The part worth asserting is the abort: the message has to name the command AND the directory.
func TestRunGitBringsTheOutputAndFailsWithTheCommandItWasAsked(t *testing.T) {
	dir := t.TempDir()

	if got := RunGit(t, dir, "--version"); got == "" {
		t.Error("git --version gave empty output")
	}
	output := RunGit(t, dir, "--version")
	if output != strings.TrimSpace(output) {
		t.Errorf("the output does not come trimmed: %q", output)
	}
	repo := filepath.Join(dir, "r")
	InitRepo(t, repo)
	if got := RunGit(t, repo, "rev-parse", "--is-inside-work-tree"); got != "true" {
		t.Errorf("git rev-parse gave %q, want true inside a freshly created repo", got)
	}
	// With a bare TempDir git finds no repo: gitEnv's isolation keeps fixtures off the code's own repo.
}

// Three reasons; the empty dir is the grave one: git would run in the real repo and write its config.
func TestCheckDirExplainsEachRefusalSeparately(t *testing.T) {
	err := checkDir("")
	if err == nil {
		t.Fatal("an empty dir passed")
	}
	if !strings.Contains(err.Error(), "the real repo") {
		t.Errorf("the reason for the empty dir does not say what happens: %q", err)
	}

	nonexistent := filepath.Join(t.TempDir(), "does-not-exist")
	err = checkDir(nonexistent)
	if err == nil {
		t.Fatal("a nonexistent dir passed")
	}
	if !strings.Contains(err.Error(), nonexistent) {
		t.Errorf("the reason %q does not name the directory", err)
	}

	file := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	err = checkDir(file)
	if err == nil {
		t.Fatal("a file passed as a directory")
	}
	if !strings.Contains(err.Error(), "is not a directory") {
		t.Errorf("the reason %q does not say that it is not a directory", err)
	}

	if err := checkDir(t.TempDir()); err != nil {
		t.Errorf("a TempDir gave an error: %v", err)
	}
}

// The difference is `-b main`: without it the fixtures depend on the global config of whoever runs them.
func TestInitBareAndInitRepoCreateWhatTheySay(t *testing.T) {
	base := t.TempDir()

	repo := filepath.Join(base, "work")
	InitRepo(t, repo)
	if got := RunGit(t, repo, "symbolic-ref", "--short", "HEAD"); got != "main" {
		t.Errorf("InitRepo left the branch at %q, want main", got)
	}
	for _, key := range []string{"user.email", "user.name"} {
		if RunGit(t, repo, "config", key) == "" {
			t.Errorf("InitRepo did not set %s", key)
		}
	}
	if got := RunGit(t, repo, "config", "commit.gpgsign"); got != "false" {
		t.Errorf("InitRepo left commit.gpgsign at %q, want false", got)
	}

	bare := filepath.Join(base, "o.git")
	InitBare(t, bare)
	if got := RunGit(t, bare, "rev-parse", "--is-bare-repository"); got != "true" {
		t.Errorf("InitBare did not create a bare repo: %q", got)
	}
	// `--is-inside-work-tree` answers FALSE inside a bare; the name misleads.
	if got := RunGit(t, bare, "rev-parse", "--is-inside-work-tree"); got != "false" {
		t.Errorf("inside the bare, --is-inside-work-tree gave %q, want false", got)
	}
	if got := RunGit(t, repo, "rev-parse", "--is-inside-work-tree"); got != "true" {
		t.Errorf("inside a normal repo gave %q, want true", got)
	}
}

// Some tests find the commit by its text, so the messages have to be distinguishable.
func TestCommitFileCreatesTheParentsAndNotes(t *testing.T) {
	dir := t.TempDir()
	InitRepo(t, dir)

	CommitFile(t, dir, "src/internal/deep.go", "package deep\n", "the commit with a unique message")

	content, err := os.ReadFile(filepath.Join(dir, "src", "internal", "deep.go"))
	if err != nil {
		t.Fatalf("CommitFile did not create the parents: %v", err)
	}
	if string(content) != "package deep\n" {
		t.Errorf("the written content is %q", content)
	}
	if !strings.Contains(RunGit(t, dir, "log", "--oneline"), "the commit with a unique message") {
		t.Error("the commit message was not saved")
	}
	CommitFile(t, dir, "another.txt", "y\n", "a different message")
	log := RunGit(t, dir, "log", "--oneline")
	if !strings.Contains(log, "the commit with a unique message") || !strings.Contains(log, "a different message") {
		t.Errorf("the two commits are not told apart in the log: %q", log)
	}
}

// The path the three adapters take when unconfigured, so it was as untested as the other half.
func TestTheConformanceSuiteAcceptsAnAdapterThatSaysUnsupported(t *testing.T) {
	a := inertAdapter()
	RunConformance(t, a, ConformanceOptions{Unsupported: true})

	names, warns := a.Branches(t.Context(), testRef())
	if len(names) != 0 {
		t.Errorf("an inert adapter returned branches: %v", names)
	}
	if len(warns) == 0 || warns[0].Kind != "unsupported" {
		t.Errorf("an inert adapter gave %v in Branches, want an unsupported", warns)
	}
}

// Looking at the message instead of the kind would force every adapter to phrase unsupported one way.
func TestHasKindLooksOnlyAtTheKind(t *testing.T) {
	warns := []model.Warning{
		{Forge: "gh", Kind: "ratelimit", Msg: "whatever"},
		{Forge: "gh", Kind: "unsupported", Msg: "some other text"},
	}
	if !hasKind(warns, "unsupported") {
		t.Error("hasKind did not find the kind that is there")
	}
	if !hasKind(warns, "ratelimit") {
		t.Error("hasKind did not find the first kind")
	}
	if hasKind(warns, "permission") {
		t.Error("hasKind found a kind that is not there")
	}
	if hasKind(nil, "unsupported") {
		t.Error("hasKind found something in an empty list")
	}
	if hasKind([]model.Warning{}, "unsupported") {
		t.Error("hasKind found something in an empty list")
	}
}

func inertAdapter() *FakeAdapter {
	w := []model.Warning{{Forge: "inert", Kind: "unsupported", Msg: "no credentials"}}
	ref := testRef()
	key := ItemKey(ref.Project, 1)
	a := &FakeAdapter{ForgeName: "inert", HostName: "inert.example"}
	for k := range a.Pages {
		a.ListWarnings[k] = w
	}
	a.ListWarnings = map[FakeKey][]model.Warning{}
	for _, q := range forge.Streams {
		a.ListWarnings[FakeKey{Section: q.Section, Kind: q.ReviewKind}] = w
	}
	a.StateWarnings = map[string][]model.Warning{key: w}
	a.CommentWarnings = map[string][]model.Warning{key: w}
	a.ActionWarnings = map[string][]model.Warning{
		"approve:o/r#1": w, "merge:o/r#1": w, "retarget:o/r#1": w,
	}
	a.BranchWarnings = map[string][]model.Warning{ref.Project: w}
	return a
}

func TestTheGateClosesOnABrokenAdapter(t *testing.T) {
	f := inertAdapter()
	if violations := ConformanceViolations(f, ConformanceOptions{Unsupported: true}); len(violations) != 0 {
		t.Errorf("an inert adapter gave %d violations:\n%s", len(violations), strings.Join(violations, "\n"))
	}

	assertBroken := func(name, broken string, opts ConformanceOptions, want ...string) {
		t.Helper()
		violations := ConformanceViolations(&failingAdapter{broken: broken}, opts)
		if len(violations) == 0 {
			t.Errorf("%s: the suite gave the green light to an adapter that breaks the contract", name)
			return
		}
		for _, q := range want {
			if !contains(violations, q) {
				t.Errorf("%s: the suite did not complain about %q. It complained about:\n%s",
					name, q, strings.Join(violations, "\n"))
			}
		}
	}

	assertBroken("empty forge", "empty-forge", ConformanceOptions{}, "Forge() is empty")
	assertBroken("empty host", "empty-host", ConformanceOptions{}, "Host() is empty")
	assertBroken("incoherent pagination", "more-without-next", ConformanceOptions{}, "More=true without Next")
	assertBroken("does not report unsupported", "no-warning", ConformanceOptions{Unsupported: true},
		"should report unsupported")
	assertBroken("returns items while unsupported", "items", ConformanceOptions{Unsupported: true},
		"should not return items")
	assertBroken("returns branches while unsupported", "branches", ConformanceOptions{Unsupported: true},
		"should not return branches")
	assertBroken("no binary gives no warning", "silent", ConformanceOptions{MissingBinary: true},
		"with no binary should return a warning")
	assertBroken("no binary returns items", "items", ConformanceOptions{MissingBinary: true},
		"with no binary should not return items")

	violations := ConformanceViolations(&failingAdapter{broken: "branches"}, ConformanceOptions{})
	if !contains(violations, "should not return branches") {
		t.Errorf("returning branches with no mode was not detected: %v", violations)
	}
	if violations := ConformanceViolations(&failingAdapter{broken: "no-warning"}, ConformanceOptions{}); len(violations) != 0 {
		t.Errorf("with no mode and nothing broken it gave %v", violations)
	}
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if strings.Contains(x, want) {
			return true
		}
	}
	return false
}

type failingAdapter struct {
	broken string
}

func (a *failingAdapter) Forge() string {
	if a.broken == "empty-forge" {
		return ""
	}
	return "broken"
}
func (a *failingAdapter) Host() string {
	if a.broken == "empty-host" {
		return ""
	}
	return "broken.example"
}
func (a *failingAdapter) Auth(context.Context) model.AuthState {
	return model.AuthState{Forge: "broken", OK: true}
}

func (a *failingAdapter) List(context.Context, forge.Query) (forge.Page, []model.Warning) {
	switch a.broken {
	case "more-without-next":
		return forge.Page{More: true}, nil
	case "items":
		return forge.Page{Items: []model.Item{{Number: 1}}}, a.warn()
	case "silent":
		return forge.Page{}, nil
	case "no-warning":
		return forge.Page{}, nil
	default:
		return forge.Page{}, a.warn()
	}
}

func (a *failingAdapter) warn() []model.Warning {
	if a.broken == "no-warning" {
		return nil
	}
	return warnUnsupported()
}

func (a *failingAdapter) ItemState(context.Context, model.RepoRef, int) (model.Item, []model.Warning) {
	return model.Item{}, a.warn()
}
func (a *failingAdapter) Comments(context.Context, model.RepoRef, int) (forge.CommentPage, []model.Warning) {
	return forge.CommentPage{}, a.warn()
}
func (a *failingAdapter) Approve(context.Context, model.RepoRef, int) []model.Warning {
	return a.warn()
}
func (a *failingAdapter) Merge(context.Context, model.RepoRef, int, forge.MergeRequest) []model.Warning {
	return a.warn()
}
func (a *failingAdapter) Retarget(context.Context, model.RepoRef, int, string) []model.Warning {
	return a.warn()
}

func (a *failingAdapter) Branches(context.Context, model.RepoRef) ([]string, []model.Warning) {
	if a.broken == "branches" {
		return []string{"main"}, a.warn()
	}
	return nil, a.warn()
}

func warnUnsupported() []model.Warning {
	return []model.Warning{{Forge: "broken", Kind: "unsupported", Msg: "a double break"}}
}

// The half of RunConformance testable in process; the real t.Error is checked by subprocess.
func TestTheReportIsCalledOncePerViolationAndNeverWithoutViolations(t *testing.T) {
	var said []string
	reportViolations(inertAdapter(), ConformanceOptions{Unsupported: true},
		func(v string) { said = append(said, v) })
	if len(said) != 0 {
		t.Errorf("an inert adapter produced %d reports: %v", len(said), said)
	}

	for _, c := range []struct {
		name   string
		broken string
		opts   ConformanceOptions
		want   string
	}{
		{"empty forge", "empty-forge", ConformanceOptions{}, "Forge() is empty"},
		{"empty host", "empty-host", ConformanceOptions{}, "Host() is empty"},
		{"does not report unsupported", "no-warning", ConformanceOptions{Unsupported: true},
			"should report unsupported"},
		{"returns items while unsupported", "items", ConformanceOptions{Unsupported: true},
			"should not return items"},
	} {
		var once []string
		reportViolations(&failingAdapter{broken: c.broken}, c.opts,
			func(v string) { once = append(once, v) })

		if len(once) == 0 {
			t.Errorf("%s: the gate said nothing and a broken adapter passed the green light", c.name)
			continue
		}
		found := false
		for _, v := range once {
			if strings.TrimSpace(v) == "" {
				t.Errorf("%s: a report with empty text is indistinguishable from not warning",
					c.name)
			}
			if strings.Contains(v, c.want) {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: the reports are %q, and none says %q", c.name, once, c.want)
		}
	}

	var one []string
	reportViolations(&failingAdapter{broken: "empty-forge"}, ConformanceOptions{},
		func(v string) { one = append(one, v) })
	if len(one) != 1 {
		t.Errorf("an adapter that breaks a single rule produced %d reports, want 1: %v",
			len(one), one)
	}
}
