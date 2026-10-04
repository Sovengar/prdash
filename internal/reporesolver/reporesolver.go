// Package reporesolver is the sole owner of prdash's path namespace. It never calls the forge
// API, only git, so resolving an item does not depend on credentials.
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
	// Applied symmetrically when building the clone URL and when normalising remotes, so a
	// subfolder instance resolves to the same Project as one at the host root.
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
		return "", fmt.Errorf("clonar %s: %w", url, err)
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

// Never touches a branch that already exists: the work in progress is reused.
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
	_, err := r.git.Run(ctx, repo, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil
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

// The subfolder prefix is stripped from the path so a remote at https://host/git/group/proj.git
// normalises to the same Project as one at the host root.
func ParseRemoteURL(raw string, hosts map[string]string, prefixes map[string]string) (model.RepoRef, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return model.RepoRef{}, false
	}

	// The "@" demands something before it: an SCP with an empty user is not a git remote. "://" is
	// accepted at 0 because url.Parse rejects it anyway; ">= 0" is what keeps it out of SCP.
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
		colon := strings.Index(rest, ":")
		if colon < 0 {
			return model.RepoRef{}, false
		}
		host = rest[:colon]
		path = rest[colon+1:]
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
