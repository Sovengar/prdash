package executor

import (
	"testing"

	"prdash/internal/cache"
	"prdash/internal/forge/model"
	"prdash/internal/reporesolver"
)

func reviewWith(worktree string) cache.ReviewRecord {
	return cache.ReviewRecord{Worktree: worktree}
}

func TestTheActiveWorktreeWinsOverTheCanonicalPath(t *testing.T) {
	// The resolver's memo has a default path that does NOT depend on WorktreeDir; without this Setenv
	//the test reads the user's memo.
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	wtDir := t.TempDir()
	ref := model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget",
		Owner: "acme", Name: "widget"}
	it := model.NewItem(ref, 7)
	it.Title = "one"

	resolver := reporesolver.New(reporesolver.Options{WorktreeDir: wtDir})
	ex := &Executor{Resolver: resolver}

	canonical := resolver.WorktreePath(ref, 7)
	if canonical == "" {
		t.Fatal("the canonical path came back empty, and without it the test has nothing to " +
			"compare the active worktree against")
	}
	if got := ex.worktreePath(it); got != canonical {
		t.Errorf("with no active review it gave %q, want the canonical path %q", got, canonical)
	}

	// An active review WITH a worktree wins, even when it is not the canonical path.
	active := wtDir + "/movido-a-mano"
	if err := resolver.RecordReview(it, reviewWith(active)); err != nil {
		t.Fatalf("RecordReview: %v", err)
	}
	if got := ex.worktreePath(it); got != active {
		t.Errorf("with an active review it gave %q, want the record's worktree %q. With the canonical "+
			"path the review would mount somewhere that is not where the work is, and the first "+
			"one stays there taking up space unreviewed",
			got, active)
	}

	// An active review WITHOUT a worktree falls back to the canonical path, which is all there is.
	if err := resolver.RecordReview(it, reviewWith("")); err != nil {
		t.Fatalf("RecordReview without a worktree: %v", err)
	}
	if got := ex.worktreePath(it); got != canonical {
		t.Errorf("with an active review without a worktree it gave %q, want the canonical path %q: a "+
			"half-filled record has no worktree to use, and with the condition the other way "+
			"this returned the empty string", got, canonical)
	}

	// An active review of ANOTHER item: this one falls back to its canonical path, because the record
	// is per item.
	other := model.NewItem(ref, 8)
	other.Title = "other"
	canonical8 := resolver.WorktreePath(ref, 8)
	if canonical8 == canonical {
		t.Fatalf("the two items share a canonical path (%q), and with that the comparison "+
			"would not distinguish anything", canonical8)
	}
	if got := ex.worktreePath(other); got != canonical8 {
		t.Errorf("with an active review for the 7, item 8 gave %q, want its canonical path %q: the "+
			"record is per item and does not mix", got, canonical8)
	}
	if canonical8 == active {
		t.Errorf("the active review of the 7 is %q and the canonical path of the 8 is the same: the "+
			"comparison would not distinguish anything", active)
	}
}
