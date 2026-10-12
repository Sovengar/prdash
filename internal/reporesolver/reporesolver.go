package reporesolver

import (
	"context"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"prdash/internal/cache"
	"prdash/internal/forge/model"
	"prdash/internal/gitcmd"
)

type Options struct {
	Roots       []string
	CloneDir    string
	WorktreeDir string
	MemoPath    string
	Hosts       map[string]string
	Prefixes    map[string]string
	CloneURL    func(model.RepoRef) string
	ParseRemote func(string) (model.RepoRef, bool)
	Git         *gitcmd.Runner
}

type Resolver struct {
	roots       []string
	cloneDir    string
	worktreeDir string
	cloneURL    func(model.RepoRef) string
	parseRemote func(string) (model.RepoRef, bool)
	git         *gitcmd.Runner
	store       *cache.Store

	mu      sync.Mutex
	index   map[string]string
	indexed bool
}

func New(opts Options) *Resolver {
	r := &Resolver{
		roots:       append([]string(nil), opts.Roots...),
		cloneDir:    opts.CloneDir,
		worktreeDir: opts.WorktreeDir,
		git:         opts.Git,
	}
	if r.git == nil {
		r.git = gitcmd.New()
	}
	r.cloneURL = opts.CloneURL
	if r.cloneURL == nil {
		prefixes := opts.Prefixes
		r.cloneURL = func(ref model.RepoRef) string {
			return CloneURL(ref, prefixes[ref.Host])
		}
	}
	r.parseRemote = opts.ParseRemote
	if r.parseRemote == nil {
		hosts := opts.Hosts
		prefixes := opts.Prefixes
		r.parseRemote = func(raw string) (model.RepoRef, bool) {
			return ParseRemoteURL(raw, hosts, prefixes)
		}
	}
	memoPath := opts.MemoPath
	if memoPath == "" {
		if p, err := cache.MemoPath(); err == nil {
			memoPath = p
		}
	}
	r.store = cache.OpenStore(memoPath)
	return r
}

func (r *Resolver) ResolveLocal(ref model.RepoRef) (string, bool) {
	key := repoKey(ref)
	if p, ok := r.store.Route(key); ok && isRepo(p) {
		return p, true
	}

	r.mu.Lock()
	if !r.indexed {
		r.index = r.buildIndex()
		r.indexed = true
	}
	p, ok := r.index[key]
	r.mu.Unlock()
	if !ok {
		return "", false
	}
	r.Remember(ref, p)
	return p, true
}

func (r *Resolver) HasBare(ref model.RepoRef) bool { return isRepo(r.barePath(ref)) }

func (r *Resolver) EnsureBare(ctx context.Context, ref model.RepoRef) (string, error) {
	dest := r.barePath(ref)
	if isRepo(dest) {
		return dest, nil
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", fmt.Errorf("prepare the bare clone %s: %w", dest, err)
	}
	if _, err := os.Stat(dest); err == nil {
		if err := os.RemoveAll(dest); err != nil {
			return "", fmt.Errorf("clean up the incomplete bare clone %s: %w", dest, err)
		}
	}

	url := r.cloneURL(ref)
	tmp := dest + ".tmp-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if _, err := r.git.Run(ctx, "", "clone", "--bare", "--quiet", "--", url, tmp); err != nil {
		_ = os.RemoveAll(tmp)
		return "", fmt.Errorf("clone %s: %w", url, err)
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.RemoveAll(tmp)
		return "", fmt.Errorf("publish the bare clone %s: %w", dest, err)
	}
	return dest, nil
}

func (r *Resolver) RemoveBare(ref model.RepoRef) error {
	dest := r.barePath(ref)
	if _, err := os.Stat(dest); err != nil {
		return nil
	}
	return os.RemoveAll(dest)
}

func (r *Resolver) FetchReviewRef(ctx context.Context, repo string, it model.Item) (string, error) {
	src, ok := ReviewRef(it)
	if !ok {
		return "", fmt.Errorf("forge %q has no known review ref", it.Forge)
	}
	branch := ReviewBranch(it.Number)
	track := fmt.Sprintf("refs/prdash/%s/%d", it.Forge, it.Number)

	if _, err := r.git.Run(ctx, repo, "fetch", "--no-tags", "origin", "+"+src+":"+track); err != nil {
		return "", fmt.Errorf("fetch %s from %s: %w", src, it.Ref.Project, err)
	}
	if r.branchExists(ctx, repo, branch) {
		return branch, nil
	}
	if _, err := r.git.Run(ctx, repo, "branch", branch, track); err != nil {
		return "", fmt.Errorf("create the local branch %s: %w", branch, err)
	}
	return branch, nil
}

// DeleteReviewRefs removes the review branch and its tracking ref that
// FetchReviewRef created in repo. Missing refs are not an error: the review may
// never have been mounted. A ref still checked out in a worktree makes git
// refuse, and that refusal is reported so the caller does not claim a deletion
// that did not happen.
func (r *Resolver) DeleteReviewRefs(ctx context.Context, repo string, it model.Item) (bool, []string) {
	if repo == "" || !isRepo(repo) {
		return false, nil
	}
	branchDeleted := false
	var warnings []string
	branch := ReviewBranch(it.Number)
	if r.refExists(ctx, repo, "refs/heads/"+branch) {
		if _, err := r.git.Run(ctx, repo, "branch", "-D", branch); err != nil {
			warnings = append(warnings, "could not delete the review branch "+branch+": "+err.Error())
		} else {
			branchDeleted = true
		}
	}
	// The tracking ref is bookkeeping: deleting it does not mean the branch is
	// gone, so it never turns into a "review branch deleted" claim.
	track := fmt.Sprintf("refs/prdash/%s/%d", it.Forge, it.Number)
	if r.refExists(ctx, repo, track) {
		if _, err := r.git.Run(ctx, repo, "update-ref", "-d", track); err != nil {
			warnings = append(warnings, "could not delete the review ref "+track+": "+err.Error())
		}
	}
	return branchDeleted, warnings
}

// DeleteLocalBranch removes branch from the clone ResolveLocal maps to ref, but
// only when no worktree holds it: deleting a checked-out branch is refused by
// git and would surprise the user anyway. No local clone, an unknown branch and
// a checked-out branch are all silent no-ops — prdash does not own the user's
// clone and never touches a branch it cannot confidently identify.
//
// The branch is a forge-supplied string used as an argument to a destructive
// command, so it is validated as a ref before anything else, and the clone's
// default branch (origin/HEAD) is never a candidate even when checked out
// nowhere: a PR whose head is named like the default would otherwise delete the
// branch the whole clone is built around.
func (r *Resolver) DeleteLocalBranch(ctx context.Context, ref model.RepoRef, branch string) (bool, string) {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return false, ""
	}
	repo, ok := r.ResolveLocal(ref)
	if !ok || !isRepo(repo) {
		return false, ""
	}
	if !r.validBranch(ctx, repo, branch) {
		return false, ""
	}
	// Nothing to delete is decided before the default-branch guard: a branch
	// that is not there cannot be the default one either.
	if !r.refExists(ctx, repo, "refs/heads/"+branch) {
		return false, ""
	}
	isDefault, err := r.isDefaultBranch(ctx, repo, branch)
	if err != nil {
		return false, "could not read the local default branch; left " + branch + " alone: " + err.Error()
	}
	if isDefault {
		return false, "left the local default branch " + branch + " alone"
	}
	// An unverifiable checkout is a reason to keep, not to proceed: the branch
	// stays and the notice says why.
	held, err := r.checkedOut(ctx, repo, branch)
	if err != nil {
		return false, "could not check whether " + branch + " is checked out: " + err.Error()
	}
	if held {
		return false, ""
	}
	if _, err := r.git.Run(ctx, repo, "branch", "-D", branch); err != nil {
		return false, "could not delete the local branch " + branch + ": " + err.Error()
	}
	return true, ""
}

func (r *Resolver) Remember(ref model.RepoRef, path string) {
	if path == "" {
		return
	}
	r.store.SetRoute(repoKey(ref), path)
}

func (r *Resolver) RecordReview(it model.Item, rec cache.ReviewRecord) error {
	r.store.SetReview(itemKey(it.ID()), rec)
	return nil
}

func (r *Resolver) ActiveReview(id model.ID) (cache.ReviewRecord, bool) {
	return r.store.Review(itemKey(id))
}

func (r *Resolver) ForgetReview(it model.Item) error {
	r.store.DeleteReview(itemKey(it.ID()))
	return nil
}

func (r *Resolver) WorktreePath(ref model.RepoRef, number int) string {
	return filepath.Join(
		r.worktreeDir,
		ref.Forge,
		ref.Host,
		filepath.FromSlash(ref.Project),
		fmt.Sprintf("prdash-pr-%d", number),
	)
}

func (r *Resolver) barePath(ref model.RepoRef) string {
	return filepath.Join(r.cloneDir, ref.Forge, ref.Host, filepath.FromSlash(ref.Project))
}

func (r *Resolver) branchExists(ctx context.Context, repo, branch string) bool {
	return r.refExists(ctx, repo, "refs/heads/"+branch)
}

func (r *Resolver) refExists(ctx context.Context, repo, ref string) bool {
	_, err := r.git.Run(ctx, repo, "show-ref", "--verify", "--quiet", ref)
	return err == nil
}

// `worktree list` is the only view that sees every checkout: HEAD alone misses
// the linked worktrees of the same clone, and one of those is exactly the
// review worktree this cleanup is racing.
func (r *Resolver) checkedOut(ctx context.Context, repo, branch string) (bool, error) {
	out, err := r.git.Run(ctx, repo, "worktree", "list", "--porcelain")
	if err != nil {
		return false, err
	}
	want := "branch refs/heads/" + branch
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == want {
			return true, nil
		}
	}
	return false, nil
}

// check-ref-format and not a pattern of our own: it is git's own rule for what
// a branch name may be, and it rejects the globs show-ref would match and the
// option-looking names branch -D would read as switches.
func (r *Resolver) validBranch(ctx context.Context, repo, branch string) bool {
	_, err := r.git.Run(ctx, repo, "check-ref-format", "--branch", branch)
	return err == nil
}

// isDefaultBranch reports whether branch is the clone's default branch. An
// unreadable default is an error, never a silent "no": failing open would let
// the deletion take the branch the whole clone is built around. A bare clone has
// no remotes/origin/HEAD (EnsureBare clones --bare), but its own HEAD is the
// default branch it was cloned with, so that is the one fallback.
func (r *Resolver) isDefaultBranch(ctx context.Context, repo, branch string) (bool, error) {
	out, err := r.git.Run(ctx, repo, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD")
	if err != nil {
		if !r.isBare(ctx, repo) {
			return false, err
		}
		out, err = r.git.Run(ctx, repo, "symbolic-ref", "--quiet", "--short", "HEAD")
		if err != nil {
			return false, err
		}
	}
	return defaultName(out) == branch, nil
}

func (r *Resolver) isBare(ctx context.Context, repo string) bool {
	out, err := r.git.Run(ctx, repo, "rev-parse", "--is-bare-repository")
	return err == nil && strings.TrimSpace(out) == "true"
}

func defaultName(out string) string {
	name := strings.TrimSpace(out)
	name = strings.TrimPrefix(name, "refs/remotes/")
	name = strings.TrimPrefix(name, "refs/heads/")
	return strings.TrimPrefix(name, "origin/")
}

func (r *Resolver) buildIndex() map[string]string {
	idx := map[string]string{}
	ctx := context.Background()
	for _, root := range r.roots {
		abs, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		_ = filepath.WalkDir(abs, func(path string, d fs.DirEntry, err error) error {
			if err != nil || !d.IsDir() {
				return nil
			}
			if path != abs && strings.HasPrefix(d.Name(), ".") {
				return fs.SkipDir
			}
			if d.Name() == ".git" || isWorktreeMarker(path) {
				return fs.SkipDir
			}
			if !isGitRepo(path) {
				return nil
			}
			raw, err := r.git.Run(ctx, path, "remote", "get-url", "origin")
			if err != nil || raw == "" {
				return nil
			}
			ref, ok := r.parseRemote(raw)
			if !ok {
				return nil
			}
			key := repoKey(ref)
			if _, exists := idx[key]; !exists {
				idx[key] = path
			}
			return nil
		})
	}
	return idx
}

func ReviewBranch(number int) string { return fmt.Sprintf("prdash/pr-%d", number) }

func ReviewRef(it model.Item) (string, bool) {
	switch it.Forge {
	case "github":
		return fmt.Sprintf("refs/pull/%d/head", it.Number), true
	case "gitlab":
		return fmt.Sprintf("refs/merge-requests/%d/head", it.Number), true
	default:
		return "", false
	}
}

func CloneURL(ref model.RepoRef, prefix string) string {
	project := strings.TrimSuffix(ref.Project, ".git")
	host := strings.Trim(ref.Host, "/")
	base := strings.Trim(prefix, "/")
	if base == "" {
		return fmt.Sprintf("https://%s/%s.git", host, project)
	}
	return fmt.Sprintf("https://%s/%s/%s.git", host, base, project)
}

func ParseRemoteURL(raw string, hosts map[string]string, prefixes map[string]string) (model.RepoRef, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return model.RepoRef{}, false
	}

	var host, path string
	if i := strings.Index(raw, "://"); i >= 0 {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			return model.RepoRef{}, false
		}
		host = u.Hostname()
		path = u.Path
	} else if at := strings.Index(raw, "@"); at > 0 {
		rest := raw[at+1:]
		var ok bool
		host, path, ok = strings.Cut(rest, ":")
		if !ok {
			return model.RepoRef{}, false
		}
	} else {
		return model.RepoRef{}, false
	}

	path = strings.Trim(strings.TrimSuffix(strings.TrimSpace(path), ".git"), "/")
	if prefix := strings.Trim(prefixes[host], "/"); prefix != "" {
		path = strings.TrimPrefix(path, prefix+"/")
	}
	parts := strings.Split(path, "/")
	if host == "" || len(parts) < 2 {
		return model.RepoRef{}, false
	}
	for _, p := range parts {
		if p == "" {
			return model.RepoRef{}, false
		}
	}
	forge, ok := hosts[host]
	if !ok || forge == "" {
		return model.RepoRef{}, false
	}
	return model.RepoRef{
		Forge:   forge,
		Host:    host,
		Project: strings.Join(parts, "/"),
		Owner:   parts[len(parts)-2],
		Name:    parts[len(parts)-1],
	}, true
}

func repoKey(ref model.RepoRef) string {
	return ref.Forge + "/" + ref.Host + "/" + ref.Project
}

func itemKey(id model.ID) string {
	return id.Forge + "/" + id.Host + "/" + id.Project + "#" + strconv.Itoa(id.Number)
}

func isRepo(path string) bool {
	if _, err := os.Lstat(filepath.Join(path, ".git")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(path, "HEAD")); err != nil {
		return false
	}
	info, err := os.Stat(filepath.Join(path, "objects"))
	return err == nil && info.IsDir()
}

func isGitRepo(path string) bool {
	info, err := os.Lstat(filepath.Join(path, ".git"))
	return err == nil && info.IsDir()
}

func isWorktreeMarker(path string) bool {
	info, err := os.Lstat(filepath.Join(path, ".git"))
	return err == nil && !info.IsDir()
}
