package testutil

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
)

// The double records the format and the args SEPARATELY, which is what lets it check that abort
// passes the args instead of concatenating them.
type testReporter struct {
	helper  int
	fatales []string
	errors  []string
}

func (r *testReporter) Helper() { r.helper++ }

func (r *testReporter) Fatalf(format string, args ...any) {
	r.fatales = append(r.fatales, fmt.Sprintf(format, args...))
}

func (r *testReporter) Error(args ...any) {
	r.errors = append(r.errors, fmt.Sprint(args...))
}

func (r *testReporter) passed() bool { return len(r.fatales) == 0 && len(r.errors) == 0 }

// Three things in one line of code.
func TestAbortWarnsWithThePrefixAndTheWholeError(t *testing.T) {
	quiet := &testReporter{}
	abort(quiet, nil)
	if !quiet.passed() {
		t.Errorf("abort with no error warned: %v / %v", quiet.fatales, quiet.errors)
	}
	if quiet.helper == 0 {
		t.Error("abort did not call Helper: the failure would be attributed to the helper's line " +
			"and not the test's")
	}

	withFailure := &testReporter{}
	abort(withFailure, errors.New("mkdir: permission denied"))
	if len(withFailure.fatales) != 1 {
		t.Fatalf("abort gave %d warnings, want 1: %v", len(withFailure.fatales), withFailure.fatales)
	}
	want := "testutil: mkdir: permission denied"
	if withFailure.fatales[0] != want {
		t.Errorf("the warning is %q, want %q", withFailure.fatales[0], want)
	}
	if len(withFailure.errors) != 0 {
		t.Errorf("abort with a fatal error also wrote an Error: %v", withFailure.errors)
	}
}

// What is checked is that fn runs ONCE, not that it warns: an idempotent mkdir is fine but a
// git init is not.
func TestAbortWithPassesTheFunctionsErrorAndDoesNotCallItAgain(t *testing.T) {
	calls := 0
	quiet := &testReporter{}
	abortWith(quiet, func() error {
		calls++
		return nil
	})
	if !quiet.passed() {
		t.Errorf("abortWith with an fn without error warned: %v / %v", quiet.fatales, quiet.errors)
	}
	if calls != 1 {
		t.Errorf("the function ran %d times, want 1", calls)
	}

	calls = 0
	withFailure := &testReporter{}
	abortWith(withFailure, func() error {
		calls++
		return errors.New("git init: the directory exists")
	})
	if len(withFailure.fatales) != 1 {
		t.Fatalf("abortWith gave %d warnings, want 1: %v", len(withFailure.fatales), withFailure.fatales)
	}
	if !strings.Contains(withFailure.fatales[0], "testutil: git init") {
		t.Errorf("the warning %q does not bring the prefix nor the error", withFailure.fatales[0])
	}
	if calls != 1 {
		t.Errorf("the function ran %d times, want 1: a repeated `git init` finds the directory "+
			"full and the warning would be about another failure", calls)
	}
}

// "Exactly" is the word that matters: a gate that reported everything in one Error would also
// turn the test red, and a gate that reported nothing would let a broken adapter pass.
func TestTheConformanceGateRecordsOneErrorPerViolationAndDoesNotTakeThemAsGood(t *testing.T) {
	broken := &FakeAdapter{
		ForgeName: "broken",
		HostName:  "broken.example.com",
		Pages: map[FakeKey][]forge.Page{
			{Section: model.SectionReview}: {
				{Items: []model.Item{model.NewItem(model.RepoRef{Project: "acme/widget"}, 1)}},
			},
		},
	}

	for _, opts := range []ConformanceOptions{
		{Unsupported: true},
		{MissingBinary: true},
	} {
		expected := ConformanceViolations(broken, opts)
		if len(expected) == 0 {
			t.Fatalf("ConformanceViolations with %+v found nothing: the fixture does not violate "+
				"and this test would prove nothing", opts)
		}

		d := &testReporter{}
		RunConformance(d, broken, opts)

		if len(d.errors) != len(expected) {
			t.Errorf("%+v: the gate recorded %d errors and there are %d violations:\n%s",
				opts, len(d.errors), len(expected), strings.Join(d.errors, "\n"))
		}
		if len(d.fatales) != 0 {
			t.Errorf("%+v: the gate aborted the test instead of recording: %v", opts, d.fatales)
		}
		// Every violation arrives, and each ONE time.
		for _, e := range expected {
			n := 0
			for _, recorded := range d.errors {
				if recorded == e {
					n++
				}
			}
			if n != 1 {
				t.Errorf("%+v: the violation %q was recorded %d times, want 1", opts, e, n)
			}
		}
	}
}

// The half that is not the failure.
func TestTheGateStaysSilentWithACompliantAdapterAndMarksItAsAHelper(t *testing.T) {
	good := inertAdapter()
	opts := ConformanceOptions{Unsupported: true}
	if v := ConformanceViolations(good, opts); len(v) != 0 {
		t.Fatalf("the fixture does not comply: %v", v)
	}

	d := &testReporter{}
	RunConformance(d, good, opts)
	if !d.passed() {
		t.Errorf("the gate said something about a compliant adapter: %v / %v", d.fatales, d.errors)
	}
	if d.helper == 0 {
		t.Error("the gate did not call Helper: a failure would be attributed to the gate and not the adapter")
	}
}

// The violations that are not the opts'.
func TestTheGateWithANonCompliantAdapterDoesNotSkipTheForgeAndTheHost(t *testing.T) {
	nameless := &FakeAdapter{}
	v := ConformanceViolations(nameless, ConformanceOptions{Unsupported: true})
	if len(v) == 0 {
		t.Fatal("ConformanceViolations did not see that the adapter has neither forge nor host")
	}

	if len(v) < 2 || v[0] != "Forge() is empty" || v[1] != "Host() is empty" {
		t.Fatalf("the first two entries are not the forge and host ones: %v", v[:min(4, len(v))])
	}

	// The rest start with ": ", because the list formats with a.Forge() and an empty forge prints
	//empty. My first version assumed the code avoided that and the test failed: it does not.
	for _, e := range v[2:] {
		if !strings.HasPrefix(e, ": ") {
			t.Errorf("the violation %q does not carry the empty-forge slot", e)
		}
	}

	d := &testReporter{}
	RunConformance(d, nameless, ConformanceOptions{Unsupported: true})
	if len(d.errors) != len(v) {
		t.Errorf("the gate recorded %d and ConformanceViolations returned %d", len(d.errors), len(v))
	}

	if len(forge.Streams) == 0 {
		t.Fatal("forge.Streams is empty: the gate has no queries to check")
	}
	if ctx := context.Background(); ctx == nil {
		t.Fatal("nil")
	}
}
