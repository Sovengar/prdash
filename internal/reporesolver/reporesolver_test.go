package reporesolver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/cache"
	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

func ghRef() model.RepoRef {
	return model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget", Owner: "acme", Name: "widget"}
}

func glRef(host, project string) model.RepoRef {
	parts := strings.Split(project, "/")
	return model.RepoRef{
		Forge:   "gitlab",
		Host:    host,
		Project: project,
		Owner:   parts[len(parts)-2],
		Name:    parts[len(parts)-1],
	}
}

func TestParseRemoteURL(t *testing.T) {
	hosts := map[string]string{"github.com": "github", "gitlab.example.com": "gitlab"}
	cases := []struct {
		name   string
		raw    string
		want   model.RepoRef
		wantOK bool
	}{
		{"scp", "git@github.com:acme/widget.git", model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget", Owner: "acme", Name: "widget"}, true},
		{"https", "https://github.com/acme/widget.git", model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget", Owner: "acme", Name: "widget"}, true},
		{"ssh", "ssh://git@github.com/acme/widget", model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget", Owner: "acme", Name: "widget"}, true},
		{"gitlab subgroup", "https://gitlab.example.com/group/sub/proj.git", model.RepoRef{Forge: "gitlab", Host: "gitlab.example.com", Project: "group/sub/proj", Owner: "sub", Name: "proj"}, true},
		{"unknown host", "https://bitbucket.org/acme/widget.git", model.RepoRef{}, false},
		{"local path", "/home/u/dev/widget", model.RepoRef{}, false},
		{"empty", "", model.RepoRef{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ParseRemoteURL(tc.raw, hosts, nil)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if ok && got != tc.want {
				t.Fatalf("ref = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestParseRemoteURLStripsClonePrefix(t *testing.T) {
	hosts := map[string]string{"gitlab.example.com": "gitlab"}
	prefixes := map[string]string{"gitlab.example.com": "git"}
	want := glRef("gitlab.example.com", "group/sub/proj")

	cases := []struct {
		name string
		raw  string
	}{
		{"without prefix", "https://gitlab.example.com/group/sub/proj.git"},
		{"with prefix", "https://gitlab.example.com/git/group/sub/proj.git"},
		{"with prefix and trailing slash", "https://gitlab.example.com/git/group/sub/proj/"},
		{"scp with prefix", "git@gitlab.example.com:git/group/sub/proj.git"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ParseRemoteURL(tc.raw, hosts, prefixes)
			if !ok {
				t.Fatalf("did not parse %q", tc.raw)
			}
			if got != want {
				t.Fatalf("ref = %+v, want %+v", got, want)
			}
		})
	}
}

func TestCloneURL(t *testing.T) {
	ghEnt := model.RepoRef{Forge: "github", Host: "github.enterprise.com", Project: "acme/widget", Owner: "acme", Name: "widget"}
	cases := []struct {
		name   string
		ref    model.RepoRef
		prefix string
		want   string
	}{
		{"github root", ghRef(), "", "https://github.com/acme/widget.git"},
		{"gitlab root", glRef("gitlab.example.com", "group/proj"), "", "https://gitlab.example.com/group/proj.git"},
		{"gitlab subfolder", glRef("gitlab.example.com", "group/proj"), "/git/", "https://gitlab.example.com/git/group/proj.git"},
		{"github enterprise subfolder", ghEnt, "git", "https://github.enterprise.com/git/acme/widget.git"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CloneURL(tc.ref, tc.prefix); got != tc.want {
				t.Fatalf("CloneURL = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCloneURLParseRemoteRoundTrip(t *testing.T) {
	ref := glRef("gitlab.example.com", "group/sub/proj")
	hosts := map[string]string{"gitlab.example.com": "gitlab"}
	prefixes := map[string]string{"gitlab.example.com": "git"}
	raw := CloneURL(ref, prefixes[ref.Host])
	got, ok := ParseRemoteURL(raw, hosts, prefixes)
	if !ok || got != ref {
		t.Fatalf("round trip = %+v, %v; want %+v", got, ok, ref)
	}
}

func fixture(t *testing.T) (origin, repo string) {
	t.Helper()
	origin = filepath.Join(t.TempDir(), "origin.git")
	testutil.InitBare(t, origin)
	repo = filepath.Join(t.TempDir(), "repo")
	testutil.InitRepo(t, repo)
	testutil.CommitFile(t, repo, "base.txt", "base", "base")
	testutil.SetRemote(t, repo, "origin", origin)
	testutil.Push(t, repo, "-u", "origin", "main")
	return origin, repo
}

func pushReviewRef(t *testing.T, origin, srcRef string) {
	t.Helper()
	work := filepath.Join(t.TempDir(), "work")
	testutil.InitRepo(t, work)
	testutil.CommitFile(t, work, "pr.txt", "PR content", "pr")
	testutil.Push(t, work, origin, "HEAD:"+srcRef)
}

func newResolver(t *testing.T, origin string, ref model.RepoRef) *Resolver {
	t.Helper()
	return New(Options{
		Roots:    []string{filepath.Dir(origin)},
		CloneDir: filepath.Join(t.TempDir(), "repos"),
		MemoPath: filepath.Join(t.TempDir(), "memo.json"),
		CloneURL: func(model.RepoRef) string { return origin },
		ParseRemote: func(raw string) (model.RepoRef, bool) {
			if strings.TrimSpace(raw) == origin {
				return ref, true
			}
			return model.RepoRef{}, false
		},
	})
}

func TestResolveLocalIndexesRoots(t *testing.T) {
	origin, repo := fixture(t)
	ref := ghRef()
	r := newResolver(t, origin, ref)
	r.roots = []string{filepath.Dir(repo)}

	got, ok := r.ResolveLocal(ref)
	if !ok || got != repo {
		t.Fatalf("ResolveLocal = %q, %v; want %q", got, ok, repo)
	}
}

func TestResolveLocalIndexesRemoteWithClonePrefix(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "proj")
	testutil.InitRepo(t, repo)
	testutil.CommitFile(t, repo, "base.txt", "base", "base")
	testutil.SetRemote(t, repo, "origin", "https://gitlab.example.com/git/group/proj.git")

	r := New(Options{
		Roots:    []string{filepath.Dir(repo)},
		MemoPath: filepath.Join(t.TempDir(), "memo.json"),
		Hosts:    map[string]string{"gitlab.example.com": "gitlab"},
		Prefixes: map[string]string{"gitlab.example.com": "git"},
	})

	got, ok := r.ResolveLocal(glRef("gitlab.example.com", "group/proj"))
	if !ok || got != repo {
		t.Fatalf("ResolveLocal = %q, %v; want %q", got, ok, repo)
	}
}

func TestResolveLocalUsesRememberedRoute(t *testing.T) {
	ref := ghRef()
	repo := filepath.Join(t.TempDir(), "repo")
	testutil.InitRepo(t, repo)
	testutil.CommitFile(t, repo, "base.txt", "base", "base")

	memoPath := filepath.Join(t.TempDir(), "memo.json")
	r := New(Options{MemoPath: memoPath})
	r.Remember(ref, repo)

	r2 := New(Options{MemoPath: memoPath})
	got, ok := r2.ResolveLocal(ref)
	if !ok || got != repo {
		t.Fatalf("ResolveLocal from the memo = %q, %v", got, ok)
	}
}

func TestResolveLocalMissing(t *testing.T) {
	r := New(Options{Roots: []string{t.TempDir()}, MemoPath: filepath.Join(t.TempDir(), "m.json")})
	if _, ok := r.ResolveLocal(ghRef()); ok {
		t.Fatal("it should not resolve a nonexistent repo")
	}
}

func TestEnsureBareClonesAndIsIdempotent(t *testing.T) {
	origin, _ := fixture(t)
	ref := ghRef()
	cloneDir := filepath.Join(t.TempDir(), "repos")
	r := New(Options{CloneDir: cloneDir, CloneURL: func(model.RepoRef) string { return origin }})

	dest, err := r.EnsureBare(context.Background(), ref)
	if err != nil {
		t.Fatalf("EnsureBare: %v", err)
	}
	want := filepath.Join(cloneDir, "github", "github.com", "acme", "widget")
	if dest != want {
		t.Fatalf("dest = %q, want %q", dest, want)
	}
	if !r.HasBare(ref) {
		t.Fatal("HasBare should be true after cloning")
	}
	if out := testutil.RunGit(t, dest, "rev-parse", "--is-bare-repository"); out != "true" {
		t.Fatalf("the clone is not bare: %q", out)
	}
	if leftovers := glob(t, cloneDir, "*.tmp-*"); len(leftovers) != 0 {
		t.Fatalf("temps left: %v", leftovers)
	}

	again, err := r.EnsureBare(context.Background(), ref)
	if err != nil || again != dest {
		t.Fatalf("idempotent EnsureBare = %q, %v", again, err)
	}
}

func TestEnsureBareClonesWithPrefixedCloneURL(t *testing.T) {
	base := t.TempDir()
	origin := filepath.Join(base, "group", "proj.git")
	testutil.InitBare(t, origin)

	gitConfig := filepath.Join(t.TempDir(), "gitconfig")
	rewrite := fmt.Sprintf("[url \"file://%s/\"]\n\tinsteadOf = https://gitlab.example.com/git/\n", base)
	if err := os.WriteFile(gitConfig, []byte(rewrite), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", gitConfig)

	cloneDir := filepath.Join(t.TempDir(), "repos")
	r := New(Options{
		CloneDir: cloneDir,
		Hosts:    map[string]string{"gitlab.example.com": "gitlab"},
		Prefixes: map[string]string{"gitlab.example.com": "git"},
	})
	ref := glRef("gitlab.example.com", "group/proj")

	dest, err := r.EnsureBare(context.Background(), ref)
	if err != nil {
		t.Fatalf("EnsureBare: %v", err)
	}
	want := filepath.Join(cloneDir, "gitlab", "gitlab.example.com", "group", "proj")
	if dest != want {
		t.Fatalf("dest = %q, want %q", dest, want)
	}
	if out := testutil.RunGit(t, dest, "rev-parse", "--is-bare-repository"); out != "true" {
		t.Fatalf("the clone is not bare: %q", out)
	}
}

func TestEnsureBareFailureLeavesNoGarbage(t *testing.T) {
	cloneDir := filepath.Join(t.TempDir(), "repos")
	r := New(Options{CloneDir: cloneDir, CloneURL: func(model.RepoRef) string {
		return filepath.Join(t.TempDir(), "no-existe")
	}})

	if _, err := r.EnsureBare(context.Background(), ghRef()); err == nil {
		t.Fatal("expected an error cloning a nonexistent remote")
	}
	dest := filepath.Join(cloneDir, "github", "github.com", "acme", "widget")
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("no bare clone should remain: %v", err)
	}
	if leftovers := glob(t, cloneDir, "*.tmp-*"); len(leftovers) != 0 {
		t.Fatalf("temps left: %v", leftovers)
	}
}

func TestFetchReviewRefGitHub(t *testing.T) {
	origin, repo := fixture(t)
	pushReviewRef(t, origin, "refs/pull/7/head")
	r := New(Options{CloneDir: t.TempDir()})

	it := model.NewItem(ghRef(), 7)
	branch, err := r.FetchReviewRef(context.Background(), repo, it)
	if err != nil {
		t.Fatalf("FetchReviewRef: %v", err)
	}
	if branch != "prdash/pr-7" {
		t.Fatalf("branch = %q", branch)
	}
	if !testutil.RefExists(t, repo, "refs/heads/prdash/pr-7") {
		t.Fatal("the local review branch is missing")
	}
	if _, err := os.Stat(filepath.Join(repo, "pr.txt")); err == nil {
		t.Fatal("the fetch must not touch the working tree")
	}
}

func TestFetchReviewRefGitLab(t *testing.T) {
	origin, repo := fixture(t)
	pushReviewRef(t, origin, "refs/merge-requests/3/head")
	r := New(Options{CloneDir: t.TempDir()})

	ref := model.RepoRef{Forge: "gitlab", Host: "gitlab.example.com", Project: "group/proj", Owner: "group", Name: "proj"}
	it := model.NewItem(ref, 3)
	branch, err := r.FetchReviewRef(context.Background(), repo, it)
	if err != nil {
		t.Fatalf("FetchReviewRef GL: %v", err)
	}
	if branch != "prdash/pr-3" || !testutil.RefExists(t, repo, "refs/heads/prdash/pr-3") {
		t.Fatalf("GL branch = %q", branch)
	}
}

func TestFetchReviewRefMissingRefErrors(t *testing.T) {
	_, repo := fixture(t)
	r := New(Options{CloneDir: t.TempDir()})
	if _, err := r.FetchReviewRef(context.Background(), repo, model.NewItem(ghRef(), 99)); err == nil {
		t.Fatal("expected an error with a nonexistent review ref")
	}
}

func TestFetchReviewRefUnknownForgeErrors(t *testing.T) {
	ref := model.RepoRef{Forge: "bitbucket", Host: "bitbucket.org", Project: "acme/widget"}
	r := New(Options{CloneDir: t.TempDir()})
	if _, err := r.FetchReviewRef(context.Background(), t.TempDir(), model.NewItem(ref, 1)); err == nil {
		t.Fatal("expected an error for a forge with no review ref")
	}
}

func TestWorktreePathIsOwnedHere(t *testing.T) {
	r := New(Options{WorktreeDir: "/data/worktrees"})
	got := r.WorktreePath(ghRef(), 7)
	want := filepath.Join("/data/worktrees", "github", "github.com", "acme", "widget", "prdash-pr-7")
	if got != want {
		t.Fatalf("WorktreePath = %q, want %q", got, want)
	}
}

func TestRecordReviewPersists(t *testing.T) {
	memoPath := filepath.Join(t.TempDir(), "memo.json")
	r := New(Options{MemoPath: memoPath})
	it := model.NewItem(ghRef(), 7)
	rec := cache.ReviewRecord{Repo: "/r", Worktree: "/w", Branch: "prdash/pr-7", Label: "prdash-pr-7"}
	if err := r.RecordReview(it, rec); err != nil {
		t.Fatalf("RecordReview: %v", err)
	}
	r2 := New(Options{MemoPath: memoPath})
	got, ok := r2.ActiveReview(it.ID())
	if !ok || got != rec {
		t.Fatalf("ActiveReview = %+v, %v", got, ok)
	}
}

func TestForgetReviewRemovesRecord(t *testing.T) {
	memoPath := filepath.Join(t.TempDir(), "memo.json")
	r := New(Options{MemoPath: memoPath})
	it := model.NewItem(ghRef(), 7)
	if err := r.RecordReview(it, cache.ReviewRecord{Worktree: "/w", Label: "prdash-pr-7"}); err != nil {
		t.Fatalf("RecordReview: %v", err)
	}
	if err := r.ForgetReview(it); err != nil {
		t.Fatalf("ForgetReview: %v", err)
	}
	if _, ok := r.ActiveReview(it.ID()); ok {
		t.Fatal("ActiveReview should not report a forgotten review")
	}
	r2 := New(Options{MemoPath: memoPath})
	if _, ok := r2.ActiveReview(it.ID()); ok {
		t.Fatal("the forgetting should persist in the memo")
	}
}

func TestDeleteReviewRefsRemovesBranchAndTrackingRef(t *testing.T) {
	origin, repo := fixture(t)
	pushReviewRef(t, origin, "refs/pull/7/head")
	r := New(Options{CloneDir: t.TempDir()})
	it := model.NewItem(ghRef(), 7)
	ctx := context.Background()
	if _, err := r.FetchReviewRef(ctx, repo, it); err != nil {
		t.Fatalf("FetchReviewRef: %v", err)
	}

	deleted, warnings := r.DeleteReviewRefs(ctx, repo, it)
	if !deleted {
		t.Fatal("it should report a deletion")
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if testutil.RefExists(t, repo, "refs/heads/prdash/pr-7") {
		t.Error("the review branch survived")
	}
	if testutil.RefExists(t, repo, "refs/prdash/github/7") {
		t.Error("the tracking ref survived")
	}
}

func TestDeleteReviewRefsWithoutRefsIsANoop(t *testing.T) {
	_, repo := fixture(t)
	r := New(Options{CloneDir: t.TempDir()})

	deleted, warnings := r.DeleteReviewRefs(context.Background(), repo, model.NewItem(ghRef(), 7))
	if deleted || len(warnings) != 0 {
		t.Fatalf("DeleteReviewRefs = (%v, %v), want a silent no-op", deleted, warnings)
	}
}

// A branch a worktree holds cannot be deleted: git refuses, and the refusal
// must be reported instead of swallowed.
func TestDeleteReviewRefsReportsABranchHeldByAWorktree(t *testing.T) {
	origin, repo := fixture(t)
	pushReviewRef(t, origin, "refs/pull/7/head")
	r := New(Options{CloneDir: t.TempDir()})
	it := model.NewItem(ghRef(), 7)
	ctx := context.Background()
	if _, err := r.FetchReviewRef(ctx, repo, it); err != nil {
		t.Fatalf("FetchReviewRef: %v", err)
	}
	wt := filepath.Join(t.TempDir(), "wt-pr7")
	testutil.RunGit(t, repo, "worktree", "add", "--quiet", wt, "prdash/pr-7")

	deleted, warnings := r.DeleteReviewRefs(ctx, repo, it)
	if deleted {
		t.Error("it claimed the branch was deleted while it was checked out")
	}
	if len(warnings) == 0 {
		t.Error("the refusal should be reported as a warning")
	}
	if !testutil.RefExists(t, repo, "refs/heads/prdash/pr-7") {
		t.Error("the branch was deleted despite being checked out")
	}

	testutil.RunGit(t, repo, "worktree", "remove", "--force", wt)
	deleted, warnings = r.DeleteReviewRefs(ctx, repo, it)
	if !deleted || len(warnings) != 0 {
		t.Fatalf("after the worktree is gone DeleteReviewRefs = (%v, %v), want a clean deletion", deleted, warnings)
	}
}

func TestDeleteLocalBranchRemovesWhenNotCheckedOut(t *testing.T) {
	origin, repo := fixture(t)
	ref := ghRef()
	r := newResolver(t, origin, ref)
	r.roots = []string{filepath.Dir(repo)}
	testutil.RunGit(t, repo, "branch", "feat/x")

	deleted, warning := r.DeleteLocalBranch(context.Background(), ref, "feat/x")
	if !deleted || warning != "" {
		t.Fatalf("DeleteLocalBranch = (%v, %q), want a silent deletion", deleted, warning)
	}
	if testutil.RefExists(t, repo, "refs/heads/feat/x") {
		t.Error("the local branch survived")
	}
}

func TestDeleteLocalBranchSkipsTheCheckedOutBranch(t *testing.T) {
	origin, repo := fixture(t)
	ref := ghRef()
	r := newResolver(t, origin, ref)
	r.roots = []string{filepath.Dir(repo)}
	testutil.RunGit(t, repo, "checkout", "-b", "feat/x")

	deleted, warning := r.DeleteLocalBranch(context.Background(), ref, "feat/x")
	if deleted || warning != "" {
		t.Fatalf("DeleteLocalBranch = (%v, %q), want it left alone", deleted, warning)
	}
	if !testutil.RefExists(t, repo, "refs/heads/feat/x") {
		t.Error("the checked-out branch was deleted")
	}
}

func TestDeleteLocalBranchWithoutABranchOrACloneIsANoop(t *testing.T) {
	origin, repo := fixture(t)
	ref := ghRef()
	r := newResolver(t, origin, ref)
	r.roots = []string{filepath.Dir(repo)}

	if deleted, warning := r.DeleteLocalBranch(context.Background(), ref, "feat/absent"); deleted || warning != "" {
		t.Errorf("a missing branch = (%v, %q), want a no-op", deleted, warning)
	}
	unresolvable := glRef("gitlab.example.com", "group/proj")
	if deleted, warning := r.DeleteLocalBranch(context.Background(), unresolvable, "feat/x"); deleted || warning != "" {
		t.Errorf("an unresolvable repo = (%v, %q), want a no-op", deleted, warning)
	}
	if deleted, warning := r.DeleteLocalBranch(context.Background(), ref, "   "); deleted || warning != "" {
		t.Errorf("an empty branch = (%v, %q), want a no-op", deleted, warning)
	}
}

func TestDeleteReviewRefsWithoutARepoIsANoop(t *testing.T) {
	r := New(Options{CloneDir: t.TempDir()})
	it := model.NewItem(ghRef(), 7)
	for _, repo := range []string{"", t.TempDir()} {
		deleted, warnings := r.DeleteReviewRefs(context.Background(), repo, it)
		if deleted || len(warnings) != 0 {
			t.Fatalf("DeleteReviewRefs(%q) = (%v, %v), want a silent no-op", repo, deleted, warnings)
		}
	}
}

// A lock next to the tracking ref is the only way git refuses an update-ref
// without a broken git: the refusal must surface as a warning.
func TestDeleteReviewRefsReportsAFailedTrackingRefDeletion(t *testing.T) {
	origin, repo := fixture(t)
	pushReviewRef(t, origin, "refs/pull/7/head")
	r := New(Options{CloneDir: t.TempDir()})
	it := model.NewItem(ghRef(), 7)
	ctx := context.Background()
	if _, err := r.FetchReviewRef(ctx, repo, it); err != nil {
		t.Fatalf("FetchReviewRef: %v", err)
	}
	lockRef(t, repo, "refs/prdash/github/7")

	deleted, warnings := r.DeleteReviewRefs(ctx, repo, it)
	if !deleted {
		t.Error("the branch was deletable and should be reported deleted")
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "refs/prdash/github/7") {
		t.Fatalf("warnings = %v, want the tracking ref refusal", warnings)
	}
	if !testutil.RefExists(t, repo, "refs/prdash/github/7") {
		t.Error("the tracking ref disappeared despite the refusal")
	}
}

// Same lock, on the branch itself: the deletion fails and must say so instead of
// reporting a deletion that did not happen.
func TestDeleteLocalBranchReportsAGitRefusal(t *testing.T) {
	origin, repo := fixture(t)
	ref := ghRef()
	r := newResolver(t, origin, ref)
	r.roots = []string{filepath.Dir(repo)}
	testutil.RunGit(t, repo, "branch", "feat/x")
	lockRef(t, repo, "refs/heads/feat/x")

	deleted, warning := r.DeleteLocalBranch(context.Background(), ref, "feat/x")
	if deleted {
		t.Error("it claimed a deletion git refused")
	}
	if !strings.Contains(warning, "feat/x") {
		t.Fatalf("warning = %q, want the local branch refusal", warning)
	}
	if !testutil.RefExists(t, repo, "refs/heads/feat/x") {
		t.Error("the branch disappeared despite the refusal")
	}
}

func TestCheckedOutIsFalseWhenTheWorktreeListFails(t *testing.T) {
	r := New(Options{MemoPath: filepath.Join(t.TempDir(), "memo.json")})
	if r.checkedOut(context.Background(), t.TempDir(), "main") {
		t.Error("a failing worktree list must not be read as checked out")
	}
}

// lockRef drops the `.lock` git itself would create: its presence makes the ref
// update fail the same way a concurrent git would.
func lockRef(t *testing.T, repo, ref string) {
	t.Helper()
	path := filepath.Join(repo, ".git", filepath.FromSlash(ref)+".lock")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

func glob(t *testing.T, dir, pattern string) []string {
	t.Helper()
	var out []string
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if ok, _ := filepath.Match(pattern, d.Name()); ok {
			out = append(out, path)
		}
		return nil
	})
	return out
}
