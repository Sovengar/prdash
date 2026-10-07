package executor

import (
	"context"
	"errors"
	"strings"
	"testing"

	"prdash/internal/cache"
	"prdash/internal/forge/model"
	"prdash/internal/herdr"
	"prdash/internal/review/plan"
	"prdash/internal/worktree"
)

type failingRecordResolver struct {
	resolverWithBare
	err     error
	records []cache.ReviewRecord
}

func (r *failingRecordResolver) RecordReview(_ model.Item, rec cache.ReviewRecord) error {
	r.records = append(r.records, rec)
	return r.err
}

type saneProvisioner struct {
	createFails bool
	created     []worktree.Spec
}

func (p *saneProvisioner) Create(_ context.Context, s worktree.Spec) (worktree.Worktree, error) {
	p.created = append(p.created, s)
	if p.createFails {
		return worktree.Worktree{}, errors.New("git worktree add: exit 128")
	}
	return worktree.Worktree{
		ID: s.Path, Label: s.Label, Path: s.Path, Branch: s.Branch, Repo: s.Repo,
	}, nil
}

func (p *saneProvisioner) List(context.Context) []worktree.Worktree { return nil }
func (p *saneProvisioner) Audit(context.Context) []worktree.Entry   { return nil }
func (p *saneProvisioner) Remove(context.Context, string) error     { return nil }
func (p *saneProvisioner) RemoveIfClean(context.Context, string) (bool, string, error) {
	return false, "", nil
}

type mountingHerdr struct{}

func (mountingHerdr) Available() bool { return true }
func (mountingHerdr) MountLayout(context.Context, herdr.Container, plan.Plan) ([]string, error) {
	return nil, nil
}
func (mountingHerdr) Notify(context.Context, string, herdr.NotifyOptions) error { return nil }

func TestIfTheReviewCannotBeRecordedItWarnsAndTheReviewStaysMounted(t *testing.T) {
	res := &failingRecordResolver{err: errors.New("no space left on device")}
	e := &Executor{Resolver: res, Worktrees: &saneProvisioner{}, Herdr: mountingHerdr{}}

	got, err := e.Mount(context.Background(), itemToMount())
	if err != nil {
		t.Fatalf("a failure to record must not abort the mount: %v", err)
	}
	if got.Worktree.Path == "" {
		t.Error("the worktree did not stay mounted after the record failure")
	}
	if !got.Herdr {
		t.Error("the layout was unmounted by a record failure: it was mounted and then thrown away")
	}
	withReason := false
	for _, w := range got.Warnings {
		if strings.Contains(w, "no space left on device") {
			withReason = true
		}
	}
	if !withReason {
		t.Errorf("no warning carries the reason for the failure: %v", got.Warnings)
	}
	// The warning has to be distinguishable from the layout's: Herdr's already came in the result.
	if len(got.Warnings) != 1 {
		t.Errorf("warnings = %v, want only the record one: the layout's are not there", got.Warnings)
	}
	// The record was ATTEMPTED: otherwise an implementation that never called RecordReview would pass.
	if len(res.records) != 1 {
		t.Fatalf("it attempted to record %d times, want 1", len(res.records))
	}
	rec := res.records[0]
	if rec.Worktree != got.Worktree.Path || rec.Branch == "" || rec.Label == "" {
		t.Errorf("the record is incomplete: %+v", rec)
	}
}

func TestIfRecordingSucceedsThereIsNoWarningNorCleanup(t *testing.T) {
	res := &failingRecordResolver{} // no error
	e := &Executor{Resolver: res, Worktrees: &saneProvisioner{}, Herdr: mountingHerdr{}}

	got, err := e.Mount(context.Background(), itemToMount())
	if err != nil {
		t.Fatalf("Mount: %v", err)
	}
	if len(got.Warnings) != 0 {
		t.Errorf("a correct record warns: %v", got.Warnings)
	}
	if len(res.records) != 1 {
		t.Errorf("it recorded %d times, want 1", len(res.records))
	}
}

// The "only if this mount created it" is the whole condition.
func TestIfTheWorktreeCannotBeCreatedTheBareIsRemovedOnlyIfThisMountCreatedIt(t *testing.T) {
	for _, c := range []struct {
		name    string
		hadBare bool
		removes bool
	}{
		{"a new clone that gets removed", false, true},
		{"a preexisting clone that is kept", true, false},
	} {
		res := &resolverWithBare{hadBare: c.hadBare}
		prov := &saneProvisioner{createFails: true}
		e := &Executor{Resolver: res, Worktrees: prov, Herdr: mountingHerdr{}}

		got, err := e.Mount(context.Background(), itemToMount())
		if err == nil {
			t.Errorf("%s: a failure to create the worktree gave nil", c.name)
			continue
		}
		if got.Worktree.Path != "" {
			t.Errorf("%s: it returned a worktree despite failing: %+v", c.name, got.Worktree)
		}
		// The error names the work that failed, which is what tells whether retrying makes sense.
		if !strings.Contains(err.Error(), "worktree") {
			t.Errorf("%s: the error %q does not say the worktree failed", c.name, err)
		}
		if got.Herdr {
			t.Errorf("%s: Herdr was mounted without a worktree", c.name)
		}
		if res.removed == 1 && !c.removes {
			t.Errorf("%s: it removed someone else's clone: it would take down the one of another "+
				"tab of the same PR", c.name)
		}
		if res.removed == 0 && c.removes {
			t.Errorf("%s: it did not remove the clone this mount created: it is left half-done", c.name)
		}
	}
}

type resolverWithBare struct {
	fakeRepoResolver
	hadBare bool
	removed int
}

func (r *resolverWithBare) HasBare(model.RepoRef) bool { return r.hadBare }

func (r *resolverWithBare) EnsureBare(context.Context, model.RepoRef) (string, error) {
	return "/clones/o/r.git", nil
}

func (r *resolverWithBare) RemoveBare(model.RepoRef) error {
	r.removed++
	return nil
}

func (r *resolverWithBare) FetchReviewRef(context.Context, string, model.Item) (string, error) {
	return "prdash/pr-7", nil
}

func (r *resolverWithBare) WorktreePath(model.RepoRef, int) string { return "/wt/prdash-pr-7" }

func itemToMount() model.Item {
	it := model.NewItem(model.RepoRef{
		Forge: "github", Host: "github.com", Project: "o/r", Owner: "o", Name: "r",
	}, 7)
	it.SourceBranch = "feat/x"
	it.TargetBranch = "main"
	it.HeadSHA = "abc123"
	return it
}
