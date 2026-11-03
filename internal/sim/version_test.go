package sim

import (
	"context"
	"strings"
	"testing"
)

func runnerPrinting(t *testing.T, out string) *Runner {
	t.Helper()
	dir := t.TempDir()
	return &Runner{Bin: writeScript(t, dir, "git-sim", "#!/bin/sh\nprintf '%s\\n' '"+out+"'\n")}
}

func TestVersionReadsTheSemverFromTheVersionFlag(t *testing.T) {
	r := runnerPrinting(t, "git-sim version 0.4.0")
	got, err := r.Version(context.Background())
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	if got != "0.4.0" {
		t.Errorf("Version = %q, want 0.4.0", got)
	}
}

func TestVersionRejectsOutputWithoutASemver(t *testing.T) {
	for name, out := range map[string]string{
		"empty":         "",
		"prose":         "git-sim version unknown",
		"two parts":     "git-sim version 0.4",
		"non-numeric":   "git-sim version 0.x.0",
		"too many part": "git-sim version 0.4.0.1",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := runnerPrinting(t, out).Version(context.Background()); err == nil {
				t.Fatalf("Version accepted %q", out)
			}
		})
	}
}

func TestVersionFailsWhenTheBinaryFails(t *testing.T) {
	dir := t.TempDir()
	r := &Runner{Bin: writeScript(t, dir, "git-sim", "#!/bin/sh\nexit 2\n")}
	if _, err := r.Version(context.Background()); err == nil {
		t.Fatal("Version accepted a failing binary")
	}
}

// The prefix is tolerated so a `v0.4.0` tag style is still read.
func TestVersionToleratesAVPrefix(t *testing.T) {
	got, err := runnerPrinting(t, "git-sim version v0.4.0").Version(context.Background())
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	if got != "v0.4.0" {
		t.Errorf("Version = %q, want v0.4.0", got)
	}
}

func TestSupportsGatesRebaseOnTheVersion(t *testing.T) {
	cases := map[string]bool{
		"0.3.5": false,
		"0.3.9": false,
		"0.4.0": true,
		"0.4.1": true,
		"0.5.0": true,
		"1.0.0": true,
	}
	for version, want := range cases {
		got := runnerPrinting(t, "git-sim version "+version).Supports(context.Background(), KindRebase)
		if got != want {
			t.Errorf("%s: Supports(rebase) = %v, want %v", version, got, want)
		}
	}
}

func TestSupportsMergeWithoutRunningTheBinary(t *testing.T) {
	r := &Runner{Bin: "/nonexistent/git-sim"}
	if !r.Supports(context.Background(), KindMerge) {
		t.Error("Supports(merge) = false, want true: merge needs no probe")
	}
	if r.Supports(context.Background(), Kind("cherry-pick")) {
		t.Error("Supports accepted an unknown kind")
	}
}

func TestSupportsRebaseIsFalseWhenTheVersionIsUnreadable(t *testing.T) {
	if runnerPrinting(t, "git-sim version unknown").Supports(context.Background(), KindRebase) {
		t.Error("Supports(rebase) = true without a readable version")
	}
	if (&Runner{Bin: "/nonexistent/git-sim"}).Supports(context.Background(), KindRebase) {
		t.Error("Supports(rebase) = true without a binary")
	}
}

func TestVersionPartsKeepsTheFirstThreeNumbers(t *testing.T) {
	if got := versionParts("v1.2.3.4"); got != [3]int{1, 2, 3} {
		t.Errorf("versionParts = %v, want [1 2 3]", got)
	}
}

func TestServiceKindsMatchesTheBinary(t *testing.T) {
	s := &Service{Runner: runnerPrinting(t, "git-sim version 0.4.0")}
	if got := strings.Join(kindNames(s.Kinds()), ","); got != "merge,rebase" {
		t.Errorf("Kinds = %q, want merge,rebase", got)
	}
}

func TestServiceKindsFallsBackToMergeWithAnOldBinary(t *testing.T) {
	s := &Service{Runner: runnerPrinting(t, "git-sim version 0.3.5")}
	if got := strings.Join(kindNames(s.Kinds()), ","); got != "merge" {
		t.Errorf("Kinds = %q, want merge", got)
	}
	// The probe is cached: a runner that disappears does not change the answer.
	s.Runner = &Runner{Bin: "/nonexistent/git-sim"}
	if got := strings.Join(kindNames(s.Kinds()), ","); got != "merge" {
		t.Errorf("Kinds = %q after the probe, want merge", got)
	}
}

func kindNames(kinds []Kind) []string {
	out := make([]string, len(kinds))
	for i, k := range kinds {
		out[i] = string(k)
	}
	return out
}

func TestIndexErrorBecomesAnUpgradeHint(t *testing.T) {
	dir := t.TempDir()
	r := &Runner{Bin: writeScript(t, dir, "git-sim",
		"#!/bin/sh\necho 'Traceback (most recent call last):'\necho 'IndexError: list index out of range' >&2\nexit 1\n")}

	_, err := r.Render(context.Background(), dir, "/tmp/m", Spec{Kind: KindRebase, Ref: "main"})
	if err == nil {
		t.Fatal("Render did not fail")
	}
	var serr *Error
	if !asError(err, &serr) {
		t.Fatalf("err = %T, want *sim.Error", err)
	}
	if !strings.Contains(serr.Msg, "upgrade") || !strings.Contains(serr.Msg, rebaseSince) {
		t.Errorf("Msg = %q, want an upgrade hint mentioning %s", serr.Msg, rebaseSince)
	}
}

// git-sim prints "git-sim error: ..." on stdout and leaves stderr empty.
func TestAUserErrorOnStdoutReachesTheMessage(t *testing.T) {
	dir := t.TempDir()
	r := &Runner{Bin: writeScript(t, dir, "git-sim",
		"#!/bin/sh\necho \"git-sim error: 'x' is not a valid Git ref or identifier.\"\nexit 1\n")}

	_, err := r.Render(context.Background(), dir, "/tmp/m", Spec{Kind: KindMerge, Ref: "x"})
	if err == nil {
		t.Fatal("Render did not fail")
	}
	var serr *Error
	if !asError(err, &serr) {
		t.Fatalf("err = %T, want *sim.Error", err)
	}
	if !strings.Contains(serr.Msg, "not a valid Git ref") {
		t.Errorf("Msg = %q, want git-sim's stdout error", serr.Msg)
	}
}
