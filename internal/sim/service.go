package sim

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"prdash/internal/forge/model"
	"prdash/internal/gitcmd"
)

type Place struct {
	Repo   string
	Branch string
	// Source is the forge ref to fetch when no review worktree exists yet, e.g. refs/pull/7/head.
	Source string
}

type Locator interface {
	Locate(it model.Item) (Place, bool)
}

type Result struct {
	Kind Kind
	Path string
	Ref  string
	Base string
}

const keepImages = 20

// simBranch is the throwaway branch the temporary clone checks out; the clone is deleted with it.
const simBranch = "prdash-sim"

type Service struct {
	Locator  Locator
	Runner   *Runner
	CacheDir string
	Mkdir    func(path string, perm fs.FileMode) error
	Git      *gitcmd.Runner

	kindsOnce sync.Once
	kinds     []Kind
}

func New(locator Locator) *Service {
	return &Service{Locator: locator, Runner: NewRunner(), Git: gitcmd.New()}
}

func (s *Service) mkdir() func(string, fs.FileMode) error {
	if s.Mkdir == nil {
		s.Mkdir = os.MkdirAll
	}
	return s.Mkdir
}

func (s *Service) gitRunner() *gitcmd.Runner {
	if s.Git == nil {
		s.Git = gitcmd.New()
	}
	return s.Git
}

func DefaultCacheDir() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "prdash", "sim"), nil
}

func (s *Service) Available() bool { return s.runner().Available() }

// Kinds probes the binary once: git-sim only learned to draw rebase in 0.4.0.
func (s *Service) Kinds() []Kind {
	s.kindsOnce.Do(func() {
		s.kinds = []Kind{KindMerge}
		ctx, cancel := context.WithTimeout(context.Background(), versionTimeout)
		defer cancel()
		if s.runner().Supports(ctx, KindRebase) {
			s.kinds = append(s.kinds, KindRebase)
		}
	})
	return s.kinds
}

func (s *Service) runner() *Runner {
	if s.Runner == nil {
		s.Runner = NewRunner()
	}
	return s.Runner
}

func (s *Service) cacheDir() (string, error) {
	if s.CacheDir != "" {
		return s.CacheDir, nil
	}
	dir, err := DefaultCacheDir()
	if err != nil {
		return "", err
	}
	s.CacheDir = dir
	return dir, nil
}

func (s *Service) Simulate(ctx context.Context, it model.Item, kind Kind) (Result, error) {
	if s.Locator == nil {
		return Result{}, errors.New("no local repository is known for this item")
	}
	place, ok := s.Locator.Locate(it)
	if !ok || place.Repo == "" {
		return Result{}, errors.New("no local clone is known for this item: clone it or mount the review first")
	}

	base := strings.TrimSpace(it.TargetBranch)
	if base == "" {
		return Result{}, fmt.Errorf("the forge reports no target branch for %s#%d", it.Ref.Project, it.Number)
	}

	tmp, err := os.MkdirTemp("", "prdash-sim-")
	if err != nil {
		return Result{}, fmt.Errorf("prepare the simulation directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	workdir, spec, err := s.stage(ctx, place, kind, base, tmp)
	if err != nil {
		return Result{}, err
	}

	media := filepath.Join(tmp, "media")
	if err := s.mkdir()(media, 0o755); err != nil {
		return Result{}, fmt.Errorf("prepare the render directory: %w", err)
	}
	image, err := s.runner().Render(ctx, workdir, media, spec)
	if err != nil {
		return Result{}, err
	}
	path, err := s.keep(image, it, kind)
	if err != nil {
		return Result{}, err
	}
	return Result{Kind: kind, Path: path, Ref: spec.Ref, Base: base}, nil
}

func (s *Service) stage(ctx context.Context, place Place, kind Kind, base, tmp string) (string, Spec, error) {
	if place.Branch == "" && place.Source == "" {
		return "", Spec{}, errors.New("the review branch is unknown; mount the review first")
	}

	path := filepath.Join(tmp, "clone")
	if _, err := s.gitRunner().Run(ctx, "", "clone", "--quiet", "--shared", place.Repo, path); err != nil {
		return "", Spec{}, fmt.Errorf("clone the local refs of %s: %w", place.Repo, err)
	}

	branch := place.Branch
	if branch == "" {
		branch = simBranch
		if err := s.fetchReview(ctx, place, path); err != nil {
			return "", Spec{}, err
		}
	}

	if err := s.materialize(ctx, path, branch); err != nil {
		return "", Spec{}, err
	}
	if err := s.materialize(ctx, path, base); err != nil {
		return "", Spec{}, err
	}

	active, ref := base, branch
	if kind == KindRebase {
		active, ref = branch, base
	}
	if _, err := s.gitRunner().Run(ctx, path, "checkout", "--quiet", active); err != nil {
		return "", Spec{}, fmt.Errorf("check out %s in the simulation clone: %w", active, err)
	}
	return path, Spec{Kind: kind, Ref: ref}, nil
}

// fetchReview brings the forge ref into the throwaway clone, so simulating never writes refs to the user's clone.
func (s *Service) fetchReview(ctx context.Context, place Place, dir string) error {
	raw, err := s.gitRunner().Run(ctx, place.Repo, "remote", "get-url", "origin")
	url := strings.TrimSpace(raw)
	if err != nil || url == "" {
		return fmt.Errorf("the local clone %s has no origin remote to fetch %s from", place.Repo, place.Source)
	}
	if _, err := s.gitRunner().Run(ctx, dir, "fetch", "--no-tags", "--quiet", url, "+"+place.Source+":refs/heads/"+simBranch); err != nil {
		return fmt.Errorf("fetch %s from the origin of %s: %w", place.Source, place.Repo, err)
	}
	return nil
}

func (s *Service) materialize(ctx context.Context, dir, name string) error {
	if _, err := s.gitRunner().Run(ctx, dir, "rev-parse", "--verify", "--quiet", name+"^{commit}"); err == nil {
		return nil
	}
	if _, err := s.gitRunner().Run(ctx, dir, "rev-parse", "--verify", "--quiet", "origin/"+name+"^{commit}"); err != nil {
		return fmt.Errorf("the branch %s does not exist in the local clone", name)
	}
	if _, err := s.gitRunner().Run(ctx, dir, "branch", "--quiet", name, "origin/"+name); err != nil {
		return fmt.Errorf("create the branch %s in the simulation clone: %w", name, err)
	}
	return nil
}

func (s *Service) keep(image string, it model.Item, kind Kind) (string, error) {
	dir, err := s.cacheDir()
	if err != nil {
		return "", fmt.Errorf("locate the simulation cache: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create the simulation cache: %w", err)
	}
	ext := filepath.Ext(image)
	if ext == "" {
		ext = ".html"
	}
	name := fmt.Sprintf("%s-%s-%d-%d%s", it.Forge, slug(it.Ref.Project), it.Number, time.Now().UnixNano(), ext)
	dst := filepath.Join(dir, name)
	if err := copyFile(image, dst); err != nil {
		return "", fmt.Errorf("keep the simulation image: %w", err)
	}
	prune(dir, keepImages)
	return dst, nil
}

type writer interface {
	io.Writer
	io.Closer
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	return copyPublishing(in, dst, createTemp)
}

func createTemp(ruta string) (writer, error) { return os.Create(ruta) }

func copyPublishing(in io.Reader, dst string, crea func(string) (writer, error)) error {
	tmp := dst + ".part"
	out, err := crea(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func prune(dir string, keep int) {
	pruneWith(dir, keep, os.ReadDir)
}

func pruneWith(dir string, keep int, listing func(string) ([]os.DirEntry, error)) {
	keep = max(keep, 0)
	entries, err := listing(dir)
	if err != nil {
		return
	}
	type aged struct {
		path string
		mod  time.Time
	}
	files := make([]aged, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jpg") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, aged{path: filepath.Join(dir, e.Name()), mod: info.ModTime()})
	}
	for i := 1; i < len(files); i++ {
		for j := i; j > 0 && files[j].mod.After(files[j-1].mod); j-- {
			files[j], files[j-1] = files[j-1], files[j]
		}
	}
	files = files[min(keep, len(files)):]
	for _, f := range files {
		_ = os.Remove(f.path)
	}
}

func slug(project string) string {
	out := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		if r == '-' || r == '_' || r == '.' {
			return r
		}
		return '-'
	}, project)
	return strings.Trim(out, "-")
}
