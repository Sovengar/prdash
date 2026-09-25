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
		{"gitlab subgrupo", "https://gitlab.example.com/grupo/sub/proy.git", model.RepoRef{Forge: "gitlab", Host: "gitlab.example.com", Project: "grupo/sub/proy", Owner: "sub", Name: "proy"}, true},
		{"host desconocido", "https://bitbucket.org/acme/widget.git", model.RepoRef{}, false},
		{"ruta local", "/home/u/dev/widget", model.RepoRef{}, false},
		{"vacío", "", model.RepoRef{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ParseRemoteURL(tc.raw, hosts, nil)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, quiero %v", ok, tc.wantOK)
			}
			if ok && got != tc.want {
				t.Fatalf("ref = %+v, quiero %+v", got, tc.want)
			}
		})
	}
}

func TestParseRemoteURLStripsClonePrefix(t *testing.T) {
	hosts := map[string]string{"gitlab.example.com": "gitlab"}
	prefixes := map[string]string{"gitlab.example.com": "git"}
	want := glRef("gitlab.example.com", "grupo/sub/proy")

	cases := []struct {
		name string
		raw  string
	}{
		{"sin prefijo", "https://gitlab.example.com/grupo/sub/proy.git"},
		{"con prefijo", "https://gitlab.example.com/git/grupo/sub/proy.git"},
		{"con prefijo y barra final", "https://gitlab.example.com/git/grupo/sub/proy/"},
		{"scp con prefijo", "git@gitlab.example.com:git/grupo/sub/proy.git"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ParseRemoteURL(tc.raw, hosts, prefixes)
			if !ok {
				t.Fatalf("no parseó %q", tc.raw)
			}
			if got != want {
				t.Fatalf("ref = %+v, quiero %+v", got, want)
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
		{"github raíz", ghRef(), "", "https://github.com/acme/widget.git"},
		{"gitlab raíz", glRef("gitlab.example.com", "grupo/proy"), "", "https://gitlab.example.com/grupo/proy.git"},
		{"gitlab subcarpeta", glRef("gitlab.example.com", "grupo/proy"), "/git/", "https://gitlab.example.com/git/grupo/proy.git"},
		{"github enterprise subcarpeta", ghEnt, "git", "https://github.enterprise.com/git/acme/widget.git"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CloneURL(tc.ref, tc.prefix); got != tc.want {
				t.Fatalf("CloneURL = %q, quiero %q", got, tc.want)
			}
		})
	}
}

func TestCloneURLParseRemoteRoundTrip(t *testing.T) {
	ref := glRef("gitlab.example.com", "grupo/sub/proy")
	hosts := map[string]string{"gitlab.example.com": "gitlab"}
	prefixes := map[string]string{"gitlab.example.com": "git"}
	raw := CloneURL(ref, prefixes[ref.Host])
	got, ok := ParseRemoteURL(raw, hosts, prefixes)
	if !ok || got != ref {
		t.Fatalf("round trip = %+v, %v; quiero %+v", got, ok, ref)
	}
}

// fixture crea un bare origin y un clon local con `main` pusheada.
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

// pushReviewRef publica un commit como ref de review en el origin.
func pushReviewRef(t *testing.T, origin, srcRef string) {
	t.Helper()
	work := filepath.Join(t.TempDir(), "work")
	testutil.InitRepo(t, work)
	testutil.CommitFile(t, work, "pr.txt", "contenido del PR", "pr")
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
	// El clon local es el repo vecino del origin; apuntamos el root a su padre.
	r.roots = []string{filepath.Dir(repo)}

	got, ok := r.ResolveLocal(ref)
	if !ok || got != repo {
		t.Fatalf("ResolveLocal = %q, %v; quiero %q", got, ok, repo)
	}
}

func TestResolveLocalIndexesRemoteWithClonePrefix(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "proy")
	testutil.InitRepo(t, repo)
	testutil.CommitFile(t, repo, "base.txt", "base", "base")
	testutil.SetRemote(t, repo, "origin", "https://gitlab.example.com/git/grupo/proy.git")

	r := New(Options{
		Roots:    []string{filepath.Dir(repo)},
		MemoPath: filepath.Join(t.TempDir(), "memo.json"),
		Hosts:    map[string]string{"gitlab.example.com": "gitlab"},
		Prefixes: map[string]string{"gitlab.example.com": "git"},
	})

	got, ok := r.ResolveLocal(glRef("gitlab.example.com", "grupo/proy"))
	if !ok || got != repo {
		t.Fatalf("ResolveLocal = %q, %v; quiero %q", got, ok, repo)
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

	// Un resolver nuevo, sin roots, resuelve por la memoria persistida.
	r2 := New(Options{MemoPath: memoPath})
	got, ok := r2.ResolveLocal(ref)
	if !ok || got != repo {
		t.Fatalf("ResolveLocal por memoria = %q, %v", got, ok)
	}
}

func TestResolveLocalMissing(t *testing.T) {
	r := New(Options{Roots: []string{t.TempDir()}, MemoPath: filepath.Join(t.TempDir(), "m.json")})
	if _, ok := r.ResolveLocal(ghRef()); ok {
		t.Fatal("no debería resolver un repo inexistente")
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
		t.Fatalf("dest = %q, quiero %q", dest, want)
	}
	if !r.HasBare(ref) {
		t.Fatal("HasBare debería ser true tras clonar")
	}
	if out := testutil.RunGit(t, dest, "rev-parse", "--is-bare-repository"); out != "true" {
		t.Fatalf("el clon no es bare: %q", out)
	}
	if leftovers := glob(t, cloneDir, "*.tmp-*"); len(leftovers) != 0 {
		t.Fatalf("quedaron temporales: %v", leftovers)
	}

	again, err := r.EnsureBare(context.Background(), ref)
	if err != nil || again != dest {
		t.Fatalf("EnsureBare idempotente = %q, %v", again, err)
	}
}

// TestEnsureBareClonesWithPrefixedCloneURL comprueba de punta a punta que el
// clon bare usa la URL por defecto con el prefijo de subcarpeta del host: el
// `insteadOf` de git redirige la URL https prefijada al origin local (sin red).
func TestEnsureBareClonesWithPrefixedCloneURL(t *testing.T) {
	base := t.TempDir()
	origin := filepath.Join(base, "grupo", "proy.git")
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
	ref := glRef("gitlab.example.com", "grupo/proy")

	dest, err := r.EnsureBare(context.Background(), ref)
	if err != nil {
		t.Fatalf("EnsureBare: %v", err)
	}
	want := filepath.Join(cloneDir, "gitlab", "gitlab.example.com", "grupo", "proy")
	if dest != want {
		t.Fatalf("dest = %q, quiero %q", dest, want)
	}
	if out := testutil.RunGit(t, dest, "rev-parse", "--is-bare-repository"); out != "true" {
		t.Fatalf("el clon no es bare: %q", out)
	}
}

func TestEnsureBareFailureLeavesNoGarbage(t *testing.T) {
	cloneDir := filepath.Join(t.TempDir(), "repos")
	r := New(Options{CloneDir: cloneDir, CloneURL: func(model.RepoRef) string {
		return filepath.Join(t.TempDir(), "no-existe")
	}})

	if _, err := r.EnsureBare(context.Background(), ghRef()); err == nil {
		t.Fatal("esperaba error al clonar un remoto inexistente")
	}
	dest := filepath.Join(cloneDir, "github", "github.com", "acme", "widget")
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("no debería quedar clon bare: %v", err)
	}
	if leftovers := glob(t, cloneDir, "*.tmp-*"); len(leftovers) != 0 {
		t.Fatalf("quedaron temporales: %v", leftovers)
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
		t.Fatal("falta la rama local de review")
	}
	if _, err := os.Stat(filepath.Join(repo, "pr.txt")); err == nil {
		t.Fatal("el fetch no debe tocar el working tree")
	}
}

func TestFetchReviewRefGitLab(t *testing.T) {
	origin, repo := fixture(t)
	pushReviewRef(t, origin, "refs/merge-requests/3/head")
	r := New(Options{CloneDir: t.TempDir()})

	ref := model.RepoRef{Forge: "gitlab", Host: "gitlab.example.com", Project: "grupo/proy", Owner: "grupo", Name: "proy"}
	it := model.NewItem(ref, 3)
	branch, err := r.FetchReviewRef(context.Background(), repo, it)
	if err != nil {
		t.Fatalf("FetchReviewRef GL: %v", err)
	}
	if branch != "prdash/pr-3" || !testutil.RefExists(t, repo, "refs/heads/prdash/pr-3") {
		t.Fatalf("rama GL = %q", branch)
	}
}

func TestFetchReviewRefMissingRefErrors(t *testing.T) {
	_, repo := fixture(t)
	r := New(Options{CloneDir: t.TempDir()})
	if _, err := r.FetchReviewRef(context.Background(), repo, model.NewItem(ghRef(), 99)); err == nil {
		t.Fatal("esperaba error con un ref de review inexistente")
	}
}

func TestFetchReviewRefUnknownForgeErrors(t *testing.T) {
	ref := model.RepoRef{Forge: "bitbucket", Host: "bitbucket.org", Project: "acme/widget"}
	r := New(Options{CloneDir: t.TempDir()})
	if _, err := r.FetchReviewRef(context.Background(), t.TempDir(), model.NewItem(ref, 1)); err == nil {
		t.Fatal("esperaba error para un forge sin ref de review")
	}
}

func TestWorktreePathIsOwnedHere(t *testing.T) {
	r := New(Options{WorktreeDir: "/data/worktrees"})
	got := r.WorktreePath(ghRef(), 7)
	want := filepath.Join("/data/worktrees", "github", "github.com", "acme", "widget", "prdash-pr-7")
	if got != want {
		t.Fatalf("WorktreePath = %q, quiero %q", got, want)
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
