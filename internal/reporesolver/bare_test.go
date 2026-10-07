package reporesolver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

// Clone-then-rename instead of cloning in place, and that is what makes a failure leave nothing.

func bareFixture(t *testing.T) (r *Resolver, ref model.RepoRef, dest, origin string) {
	t.Helper()
	origin, _ = fixture(t)
	ref = ghRef()
	cloneDir := filepath.Join(t.TempDir(), "repos")
	r = New(Options{
		Roots:    []string{filepath.Dir(origin)},
		CloneDir: cloneDir,
		MemoPath: filepath.Join(t.TempDir(), "memo.json"),
	})
	r.cloneURL = func(model.RepoRef) string { return origin }
	dest = r.barePath(ref)
	return r, ref, dest, origin
}

func TestEnsureBareReusesWhatIsThereAndDoesNotCloneAgain(t *testing.T) {
	r, ref, dest, _ := bareFixture(t)
	ctx := context.Background()

	if _, err := r.EnsureBare(ctx, ref); err != nil {
		t.Fatalf("the first EnsureBare: %v", err)
	}
	marker := filepath.Join(dest, "marcador")
	if err := os.WriteFile(marker, []byte("do not delete me"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := r.EnsureBare(ctx, ref)
	if err != nil {
		t.Fatalf("the second EnsureBare: %v", err)
	}
	if got != dest {
		t.Errorf("EnsureBare gave %q, want the canonical path %q", got, dest)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("EnsureBare re-cloned over a healthy bare: a new clone would not bring the "+
			"marker (%v)", err)
	}
	if !isRepo(dest) {
		t.Error("after the second EnsureBare the path is not a repo")
	}
}

func TestEnsureBareCleansLeftoversOfAFailedAttemptBeforeRetrying(t *testing.T) {
	r, ref, dest, _ := bareFixture(t)
	ctx := context.Background()

	if err := os.MkdirAll(filepath.Join(dest, "objects"), 0o755); err != nil {
		t.Fatal(err)
	}
	leftover := filepath.Join(dest, "objects", "incompleto")
	if err := os.WriteFile(leftover, []byte("garbage"), 0o644); err != nil {
		t.Fatal(err)
	}
	if isRepo(dest) {
		t.Fatal("the fixture is not what it claims: the leftovers would look like a repo")
	}

	got, err := r.EnsureBare(ctx, ref)
	if err != nil {
		t.Fatalf("EnsureBare with leftovers: %v", err)
	}
	if got != dest {
		t.Errorf("EnsureBare gave %q, want %q", got, dest)
	}
	if !isRepo(dest) {
		t.Error("after EnsureBare the canonical path is not a repo")
	}
	if _, err := os.Stat(leftover); err == nil {
		t.Error("the leftovers of the failed attempt are still there")
	}
}

func TestEnsureBarePropagatesTheCloneFailureAndLeavesNoGarbage(t *testing.T) {
	r, ref, dest, _ := bareFixture(t)
	r.cloneURL = func(model.RepoRef) string { return "file:///no-existe/prueba.git" }

	_, err := r.EnsureBare(context.Background(), ref)
	if err == nil {
		t.Fatal("cloning a nonexistent URL gave nil")
	}
	if !strings.Contains(err.Error(), "no-existe/prueba.git") {
		t.Errorf("the error %q does not name the URL it tried to clone", err)
	}
	if _, err := os.Stat(dest); err == nil {
		t.Error("EnsureBare left something at the canonical path after failing")
	}
	temps := glob(t, filepath.Dir(dest), "*.tmp-*")
	if len(temps) != 0 {
		t.Errorf("EnsureBare left %d temps: %v", len(temps), temps)
	}

	// The failure is REPEATABLE: the next attempt must fail for the same reason, not a different one.
	r.cloneURL = func(model.RepoRef) string { return "file:///no-existe/prueba.git" }
	_, err2 := r.EnsureBare(context.Background(), ref)
	if err2 == nil {
		t.Fatal("the second attempt gave nil")
	}
	// The messages are not identical: they carry the temporary's name, which changes.
	if !strings.Contains(err2.Error(), "no-existe/prueba.git") {
		t.Errorf("the second attempt gave %v: it fails again for another reason", err2)
	}
	if strings.Contains(err2.Error(), "already exists") {
		t.Errorf("the second attempt failed because of a previous clone left behind: %v", err2)
	}
}

// Mount calls it when the mount fails AFTERWARD.
func TestRemoveBareDeletesWhatIsThereAndToleratesWhatIsNot(t *testing.T) {
	r, ref, dest, _ := bareFixture(t)
	ctx := context.Background()

	if err := r.RemoveBare(ref); err != nil {
		t.Errorf("removing a bare that does not exist gave %v, want nil", err)
	}
	if _, err := os.Stat(dest); err == nil {
		t.Error("RemoveBare created the bare")
	}

	if _, err := r.EnsureBare(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if err := r.RemoveBare(ref); err != nil {
		t.Fatalf("RemoveBare: %v", err)
	}
	if _, err := os.Stat(dest); err == nil {
		t.Error("the bare is still on disk after removing it")
	}
	// The parent survives, which stops two mounts of the same repo from stepping on each other.
	if _, err := os.Stat(filepath.Dir(dest)); err != nil {
		t.Errorf("RemoveBare took the parent directory with it: %v", err)
	}

	// Removing it twice does not fail: the cleanup has to be idempotent.
	if err := r.RemoveBare(ref); err != nil {
		t.Errorf("the second time gave %v, want nil", err)
	}
}
