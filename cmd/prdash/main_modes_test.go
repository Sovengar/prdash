package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/config"
	"prdash/internal/forge"
	"prdash/internal/review/executor"
)

// With prdash's flags read first, --orphans would be eaten as a prdash flag instead of the subcommand's.
func TestTheSubcommandIsCheckedBeforeTheFlags(t *testing.T) {
	got, err := parseOpts([]string{"worktrees", "remove", "--orphans", "--dry-run"})
	if err != nil {
		t.Fatalf("parseOpts for worktrees errored: %v", err)
	}
	if got.mode != modeWorktrees {
		t.Errorf("mode = %v, want modeWorktrees", got.mode)
	}
	if got.sub != "remove" {
		t.Errorf("sub = %q, want remove", got.sub)
	}
	if len(got.args) != 3 || got.args[0] != "remove" {
		t.Errorf("args = %q, want the three after the subcommand", got.args)
	}

	got, err = parseOpts([]string{"--print"})
	if err != nil {
		t.Fatalf("parseOpts for --print errored: %v", err)
	}
	if got.mode != modePrint {
		t.Errorf("with --print it gave mode %v, want modePrint", got.mode)
	}

	got, err = parseOpts([]string{"--print", "worktrees"})
	if err != nil {
		t.Fatalf("parseOpts errored: %v", err)
	}
	if got.mode == modeWorktrees {
		t.Error("--print worktrees was taken as a subcommand: a subcommand only counts in " +
			"the first position")
	}
	if got.mode != modePrint {
		t.Error("--print was not recognised when it is not in the first position")
	}

	got, err = parseOpts([]string{"worktrees"})
	if err != nil {
		t.Fatal(err)
	}
	if got.sub != "list" {
		t.Errorf("worktrees without a sub gave %q, want list", got.sub)
	}

	got, err = parseOpts([]string{"worktrees", ""})
	if err != nil {
		t.Fatal(err)
	}
	if got.sub != "list" {
		t.Errorf("an empty sub gave %q, want list", got.sub)
	}
}

// The global flag set is a singleton that flag.Parse mutates forever, and test order is not guaranteed.
func TestTheFlagParserIsNotTheGlobalOne(t *testing.T) {
	got, err := parseOpts([]string{"--print"})
	if err != nil || got.mode != modePrint {
		t.Fatalf("--print gave %+v / %v", got, err)
	}

	got, err = parseOpts(nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.mode == modePrint {
		t.Error("a call without --print inherited the previous mode: the FlagSet is global")
	}

	for i := range 3 {
		if _, err := parseOpts([]string{"--print"}); err != nil {
			t.Fatal(err)
		}
		got, err = parseOpts([]string{"--print=false"})
		if err != nil {
			t.Fatal(err)
		}
		if got.mode == modePrint {
			t.Errorf("iteration %d leaked state between calls", i)
			break
		}
	}
}

// An error rather than being ignored, because `prdash --prnt` is almost always a typo of --print.
func TestAnUnknownFlagIsAUsageError(t *testing.T) {
	_, err := parseOpts([]string{"--prnt"})
	if err == nil {
		t.Fatal("an unknown flag gave nil: a typo of --print would silently open the TUI")
	}
	if !strings.Contains(err.Error(), "prnt") {
		t.Errorf("the error %q does not name the flag", err)
	}
	if _, err := parseOpts([]string{"--print=false"}); err != nil {
		t.Errorf("--print=false errored: %v", err)
	}
	if _, err := parseOpts([]string{"-h"}); err == nil {
		t.Error("-h gave nil; with ContinueOnError it is a parse error")
	}
}

// The default must be the TUI: a --print default would turn every flagless run into a network query.
func TestWithNoArgsTheTUIIsRequested(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"nothing", nil},
		{"empty string", []string{""}},
	} {
		got, err := parseOpts(tc.args)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got.mode != modeTUI {
			t.Errorf("%s gave mode %v, want modeTUI", tc.name, got.mode)
		}
	}
}

// A missing SetX does not break compilation: the failure only surfaces when the user presses the key.
func TestTheWiringInjectsTheFiveModelPieces(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	cfg := config.Defaults()
	ex := buildExecutor(cfg)

	model := wire(cfg, nil, ex)
	w := model.Wiring()

	if w.Mounter == nil {
		t.Error("wire did not inject the review mounter")
	}
	if w.Simulator == nil {
		t.Error("wire did not inject the simulator: the simulate action would warn about missing git-sim")
	}
	if w.Graphics == nil {
		t.Error("wire did not inject the graphics layer: the popup would come out in half-blocks")
	}
	if w.ReviewLookup == nil {
		t.Error("wire did not inject the review registry: a base change would warn about nothing")
	}
	if w.ReviewRemover == nil {
		t.Error("wire did not inject the remover: a merge would not delete the worktree")
	}

	// The four review pieces are the same executor instance, which prevents "works in tests, breaks in production".
	if m, ok := w.Mounter.(*executor.Executor); !ok || m != ex {
		t.Errorf("the mounter is not the executor it was given: %T", w.Mounter)
	}
	if _, ok := w.ReviewRemover.(*executor.Executor); !ok {
		t.Errorf("the remover is not an executor: %T", w.ReviewRemover)
	}
	if _, ok := w.ReviewLookup.(*executor.Executor); !ok {
		t.Errorf("the registry is not an executor: %T", w.ReviewLookup)
	}
}

func TestBuildAdaptersRespectsWhatIsEnabled(t *testing.T) {
	cfg := config.Defaults()
	cfg.Forges.GitHub.Enabled = false
	cfg.Forges.GitLab.Enabled = false
	cfg.Forges.Bitbucket.Enabled = false
	if got := buildAdapters(cfg); len(got) != 0 {
		t.Errorf("with the three disabled it gave %d adapters", len(got))
	}

	cfg = config.Defaults()
	cfg.Forges.GitLab.Enabled = false
	cfg.Forges.Bitbucket.Enabled = false
	got := buildAdapters(cfg)
	if len(got) != 1 || got[0].Forge() != "github" {
		t.Errorf("only github gave %v", adapterNames(got))
	}
	if got[0].Host() != cfg.Forges.GitHub.Host {
		t.Errorf("the adapter's host is %q, want %q", got[0].Host(), cfg.Forges.GitHub.Host)
	}

	cfg = config.Defaults()
	cfg.Forges.Bitbucket.Enabled = true
	got = buildAdapters(cfg)
	want := []string{"github", "gitlab", "bitbucket"}
	gotNames := adapterNames(got)
	if len(gotNames) != len(want) {
		t.Fatalf("the three gave %v, want %v", gotNames, want)
	}
	for i := range want {
		if gotNames[i] != want[i] {
			t.Errorf("the adapter order is %v, want %v", gotNames, want)
			break
		}
	}
	if got[1].Host() != cfg.Forges.GitLab.Host {
		t.Errorf("the gitlab host is %q, want %q", got[1].Host(), cfg.Forges.GitLab.Host)
	}
	if got[2].Host() != "bitbucket.org" {
		t.Errorf("the bitbucket host is %q, want bitbucket.org", got[2].Host())
	}
}

// The code is 2, not 1: a script must tell "wrong usage" from "it failed".
func TestRunReturnsTheUsageCodeWithoutDoingAnything(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	var stdout, stderr bytes.Buffer
	code := run([]string{"--no-exist"}, &stdout, &stderr)

	if code != 2 {
		t.Errorf("an unknown flag returned %d, want 2 (wrong usage)", code)
	}
	if !strings.Contains(stderr.String(), "no-exist") {
		t.Errorf("stderr = %q, it does not name the flag", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("a wrong usage printed %q", stdout.String())
	}
}

func TestRunWithoutForgesProceedsWithWarningAndDoesNotOpenTheTUI(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, config.DirName), 0o755); err != nil {
		t.Fatal(err)
	}
	// The key is [forge...] singular: [forges...] is silently ignored by the parser.
	toml := "[forge.github]\nenabled = false\n\n[forge.gitlab]\nenabled = false\n" +
		"\n[forge.bitbucket]\nenabled = false\n"
	if err := os.WriteFile(filepath.Join(dir, config.DirName, config.FileName), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"--print"}, &stdout, &stderr)

	if code != 0 {
		t.Errorf("with --print and no forges it returned %d, want 0", code)
	}
	if !strings.Contains(stderr.String(), "no forges enabled") {
		t.Errorf("stderr = %q, want the warning that there are no forges", stderr.String())
	}
	for _, want := range []string{"Created by me (0)", "Review / assigned (0)", "Mentions (0)"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout does not carry %q: %q", want, stdout.String())
		}
	}
	if strings.Contains(stderr.String(), "abort") {
		t.Errorf("stderr = %q says that it aborted", stderr.String())
	}
}

func TestAnUnreadableConfigWarnsButDoesNotAbort(t *testing.T) {
	// The directory is the config one: pinning XDG_CACHE_HOME leaves config.toml never found.
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	if err := os.MkdirAll(filepath.Join(dir, config.DirName), 0o755); err != nil {
		t.Fatal(err)
	}
	broken := "enabled = [[[\n"
	if err := os.WriteFile(filepath.Join(dir, config.DirName, config.FileName), []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"--print"}, &stdout, &stderr)

	if code != 0 {
		t.Errorf("a broken config returned %d, want 0: config never aborts", code)
	}
	if !strings.Contains(stderr.String(), "config") {
		t.Errorf("stderr = %q, want the config warning", stderr.String())
	}
	if strings.Contains(stderr.String(), "no forges enabled") {
		t.Errorf("with a broken config the forges warning still came out: %q", stderr.String())
	}
}

func adapterNames(as []forge.Adapter) []string {
	out := make([]string, len(as))
	for i, a := range as {
		out[i] = a.Forge()
	}
	return out
}
