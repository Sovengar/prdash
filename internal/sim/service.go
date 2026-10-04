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
	"time"

	"prdash/internal/forge/model"
	"prdash/internal/gitcmd"
)

// It does not include the review worktree because the simulation never touches it — it works in a
// temporary clone — and so it should not look like it can.
type Place struct {
	Repo   string
	Branch string
}

// A port rather than a direct dependency because the review orchestrator is what knows about repos.
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

type Service struct {
	Locator  Locator
	Runner   *Runner
	CacheDir string
	// The third injected field: the three exist because environment failures cannot be staged in a
	// test. This one only fails when the filesystem says so (ENOSPC, EDQUOT, EIO) — real, not stageable.
	Mkdir func(path string, perm fs.FileMode) error
	// git-sim behaving is not the interesting half; git failing is, and without this field there was
	// no way to provoke it. Runner.Bin stands up a fake git that passes everything but one subcommand.
	Git *gitcmd.Runner
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

// $XDG_CACHE_HOME/prdash/sim.
func DefaultCacheDir() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "prdash", "sim"), nil
}

func (s *Service) Available() bool { return s.runner().Available() }

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

// Everything happens in a temporary clone: git-sim needs a HEAD on a real branch and the
// review's worktree is the review's cwd. Cloned with --shared and deleted, so a crash leaves nothing.
func (s *Service) Simulate(ctx context.Context, it model.Item, kind Kind) (Result, error) {
	if s.Locator == nil {
		return Result{}, errors.New("no local repository is known for this item")
	}
	place, ok := s.Locator.Locate(it)
	if !ok || place.Repo == "" {
		return Result{}, errors.New("the review must be mounted first: the local refs do not exist yet")
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

// `--shared` (objects are referenced, not copied) and both branches then materialised as local
// branches: `git clone` only brings HEAD's branch into local and the rest stay as origin/ refs, which
// git-sim accepts as an argument but does not draw in the graph.
func (s *Service) stage(ctx context.Context, place Place, kind Kind, base, tmp string) (string, Spec, error) {
	if place.Branch == "" {
		return "", Spec{}, errors.New("the review branch is unknown; mount the review first")
	}

	path := filepath.Join(tmp, "clone")
	if _, err := s.gitRunner().Run(ctx, "", "clone", "--quiet", "--shared", place.Repo, path); err != nil {
		return "", Spec{}, fmt.Errorf("clone the local refs of %s: %w", place.Repo, err)
	}

	if err := s.materialize(ctx, path, place.Branch); err != nil {
		return "", Spec{}, err
	}
	if err := s.materialize(ctx, path, base); err != nil {
		return "", Spec{}, err
	}

	active, ref := base, place.Branch
	if kind == KindRebase {
		active, ref = place.Branch, base
	}
	if _, err := s.gitRunner().Run(ctx, path, "checkout", "--quiet", active); err != nil {
		return "", Spec{}, fmt.Errorf("check out %s in the simulation clone: %w", active, err)
	}
	return path, Spec{Kind: kind, Ref: ref}, nil
}

// The local branch wins over the remote ref, because a work clone can have a base behind the remote's
// and comparing against the wrong one gives a graph that is nobody's.
func (s *Service) materialize(ctx context.Context, dir, name string) error {
	// An already-local branch is left alone. That is the normal case, and it is also correct: the local
	// base is the one the review was compared against.
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
	name := fmt.Sprintf("%s-%s-%d-%d.jpg", it.Forge, slug(it.Ref.Project), it.Number, time.Now().UnixNano())
	dst := filepath.Join(dir, name)
	if err := copyFile(image, dst); err != nil {
		return "", fmt.Errorf("keep the simulation image: %w", err)
	}
	prune(dir, keepImages)
	return dst, nil
}

// An interface because Close is the only copy error that is not a disk error and cannot be
// provoked: everything written goes through the page cache, so io.Copy finishes even if the disk fills.
type escritura interface {
	io.Writer
	io.Closer
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	return copiaPublicando(in, dst, creaTemporal)
}

func creaTemporal(ruta string) (escritura, error) { return os.Create(ruta) }

// Only the creator is injected, because it is the only part that depends on the filesystem in a way
// the test cannot imitate. The rename and the delete stay on os and are testable for real: a `dst` that
// is already a non-empty directory fails the rename without touching a line of this function.
func copiaPublicando(in io.Reader, dst string, crea func(string) (escritura, error)) error {
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
	// The rename cleans up too, and the asymmetry shows: the temporary is a `.part` in the cache directory
	// and `prune` only looks at `.jpg`, so a `.part` left behind is deleted by nobody.
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// The listing is injected because DirEntry.Info only fails if the file vanishes between ReadDir
// and the lstat, and races cannot be forced. What matters is that the prune CARRIES ON past a dead entry.
func prune(dir string, keep int) {
	poda(dir, keep, os.ReadDir)
}

func poda(dir string, keep int, listado func(string) ([]os.DirEntry, error)) {
	if keep < 0 {
		keep = 0
	}
	entries, err := listado(dir)
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
	if len(files) <= keep {
		return
	}
	for i := 1; i < len(files); i++ {
		for j := i; j > 0 && files[j].mod.After(files[j-1].mod); j-- {
			files[j], files[j-1] = files[j-1], files[j]
		}
	}
	for _, f := range files[keep:] {
		_ = os.Remove(f.path)
	}
}

func slug(project string) string {
	out := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '-' || r == '_' || r == '.':
			return r
		default:
			return '-'
		}
	}, project)
	return strings.Trim(out, "-")
}
