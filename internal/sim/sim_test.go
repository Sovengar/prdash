package sim

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArgsPutGlobalsBeforeTheSubcommand(t *testing.T) {
	r := NewRunner()
	got := r.args(Spec{Kind: KindMerge, Ref: "prdash/pr-7"}, "/tmp/media")

	want := []string{
		"--output-only-path",
		"--no-animate",
		"--media-dir", "/tmp/media",
		"merge", "prdash/pr-7",
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("args = %v, want %v", got, want)
	}
}

// git-sim prints the image path only when not silent, so asking for both leaves the output empty.
func TestArgsNeverAskForQuietAndThePath(t *testing.T) {
	for _, kind := range []Kind{KindMerge, KindRebase} {
		argv := strings.Join(NewRunner().args(Spec{Kind: kind, Ref: "x"}, "/tmp/m"), " ")
		if strings.Contains(argv, "--quiet") {
			t.Errorf("%s: the argv asks for --quiet, which voids --output-only-path: %s", kind, argv)
		}
		if !strings.Contains(argv, "--output-only-path") {
			t.Errorf("%s: the argv does not ask for the image path: %s", kind, argv)
		}
	}
}

func TestArgsCarryTheRefOfEachKind(t *testing.T) {
	r := NewRunner()
	if got := r.args(Spec{Kind: KindRebase, Ref: "main"}, "/tmp/m"); got[len(got)-2] != "rebase" {
		t.Errorf("subcommand = %q, want rebase", got[len(got)-2])
	}
	if got := r.args(Spec{Kind: KindRebase, Ref: "main"}, "/tmp/m"); got[len(got)-1] != "main" {
		t.Errorf("ref = %q, want main", got[len(got)-1])
	}
}

// Without a display git-sim's desktop-viewer call never returns.
func TestEnvForcesNoAutoOpen(t *testing.T) {
	t.Setenv("git_sim_auto_open", "true")
	t.Setenv("git_sim_img_format", "png")

	var seen int
	for _, kv := range env() {
		switch kv {
		case "git_sim_auto_open=false":
			seen++
		case "git_sim_img_format=png":
			t.Error("env keeps a user git_sim_*; the first one wins and voids the forcing")
		}
	}
	if seen != 1 {
		t.Errorf("git_sim_auto_open=false appears %d times, want 1", seen)
	}
}

func TestImagePathTakesTheLastNonEmptyLine(t *testing.T) {
	if got := imagePath("noise\n/tmp/x.jpg\n\n"); got != "/tmp/x.jpg" {
		t.Errorf("imagePath = %q, want /tmp/x.jpg", got)
	}
	if got := imagePath("   \n"); got != "" {
		t.Errorf("imagePath = %q, want empty", got)
	}
}

func TestRenderReturnsTheImagePath(t *testing.T) {
	dir := t.TempDir()
	bin := writeScript(t, dir, "git-sim", "#!/bin/sh\necho /tmp/media/img.jpg\n")

	r := &Runner{Bin: bin}
	got, err := r.Render(context.Background(), dir, "/tmp/media", Spec{Kind: KindMerge, Ref: "feat"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got != "/tmp/media/img.jpg" {
		t.Errorf("image = %q", got)
	}
}

func TestRenderFailsWithTheStderrOfGitSim(t *testing.T) {
	dir := t.TempDir()
	bin := writeScript(t, dir, "git-sim", "#!/bin/sh\necho \"'x' is not a valid Git ref\" >&2\nexit 1\n")

	r := &Runner{Bin: bin}
	_, err := r.Render(context.Background(), dir, "/tmp/media", Spec{Kind: KindMerge, Ref: "x"})
	if err == nil {
		t.Fatal("Render did not fail")
	}
	var serr *Error
	if !asError(err, &serr) {
		t.Fatalf("err = %T, want *sim.Error", err)
	}
	if !strings.Contains(serr.Msg, "not a valid Git ref") {
		t.Errorf("Msg = %q, want git-sim's stderr", serr.Msg)
	}
	if serr.ExitCode != 1 {
		t.Errorf("ExitCode = %d, want 1", serr.ExitCode)
	}
}

func TestRenderRejectsAnUnknownKind(t *testing.T) {
	r := NewRunner()
	if _, err := r.Render(context.Background(), t.TempDir(), "/tmp/m", Spec{Kind: "cherry-pick", Ref: "x"}); err == nil {
		t.Fatal("Render accepted an unknown kind")
	}
	if _, err := r.Render(context.Background(), t.TempDir(), "/tmp/m", Spec{Kind: KindMerge}); err == nil {
		t.Fatal("Render accepted a spec without ref")
	}
}

func TestAvailableFollowsTheBinary(t *testing.T) {
	missing := &Runner{Bin: "git-sim-that-does-not-exist", lookPath: func(string) (string, error) {
		return "", os.ErrNotExist
	}}
	if missing.Available() {
		t.Error("Available = true with a missing binary")
	}

	here := &Runner{Bin: "git-sim", lookPath: func(name string) (string, error) { return "/usr/bin/" + name, nil }}
	if !here.Available() {
		t.Error("Available = false with a present binary")
	}
}

func writeScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}
