package sim

import (
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

// stubLocator devuelve un Place fijo.
type stubLocator struct {
	place Place
	ok    bool
}

func (l stubLocator) Locate(model.Item) (Place, bool) { return l.place, l.ok }

// fixture arma un repo con una rama base y una de la PR, más un clon bare que
// hace de repo local, y devuelve un ítem que apunta a él.
func fixture(t *testing.T) (repo, review string, it model.Item) {
	t.Helper()
	dir := t.TempDir()

	src := filepath.Join(dir, "src")
	testutil.InitRepo(t, src)
	testutil.CommitFile(t, src, "f.txt", "base", "base")
	testutil.RunGit(t, src, "branch", "-M", "main")
	testutil.RunGit(t, src, "checkout", "-b", "prdash/pr-7")
	testutil.CommitFile(t, src, "f.txt", "pr", "pr")

	// El clon bare es el "repo local" que guarda los refs, igual que el que
	// provisiona el resolutor; los refs se replican a mano porque el test no
	// necesita un remoto.
	repo = filepath.Join(dir, "remote.git")
	testutil.InitBare(t, repo)
	testutil.RunGit(t, src, "remote", "add", "origin", repo)
	testutil.Push(t, src, "origin", "main", "prdash/pr-7")

	// El worktree del review es un checkout real de la rama de la PR. src vuelve
	// a la base antes, porque una rama no puede estar en dos worktrees a la vez.
	review = filepath.Join(dir, "review")
	testutil.RunGit(t, src, "checkout", "--quiet", "main")
	testutil.RunGit(t, src, "worktree", "add", "--quiet", review, "prdash/pr-7")

	it = model.NewItem(model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget"}, 7)
	it.SourceBranch = "feat/x"
	it.TargetBranch = "main"
	return repo, review, it
}

// fakeSim escribe un git-sim que copia una imagen real a su --media-dir e
// imprime la ruta, que es el contrato que el runner espera.
func fakeSim(t *testing.T, srcJPEG string) string {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\n" +
		"media=\"\"\n" +
		"while [ $# -gt 0 ]; do\n" +
		"  if [ \"$1\" = \"--media-dir\" ]; then media=\"$2\"; fi\n" +
		"  shift\n" +
		"done\n" +
		"mkdir -p \"$media/images\"\n" +
		"cp " + srcJPEG + " \"$media/images/out.jpg\"\n" +
		"echo \"$media/images/out.jpg\"\n"
	return writeScript(t, dir, "git-sim", script)
}

// writeJPEG crea un JPEG diminuto y devuelve su ruta.
func writeJPEG(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "src.jpg")
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := range 8 {
		for x := range 8 {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x * 30), G: uint8(y * 30), B: 200, A: 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := jpeg.Encode(f, img, nil); err != nil {
		t.Fatal(err)
	}
	return path
}

func newService(t *testing.T, loc Locator, bin string) *Service {
	t.Helper()
	svc := New(loc)
	svc.Runner = &Runner{Bin: bin}
	svc.CacheDir = filepath.Join(t.TempDir(), "cache")
	return svc
}

// TestSimulateLeavesNoTraceInTheLocalRepo: la simulación necesita un HEAD
// enganchado a una rama, y la única forma de tenerlo sin tocar el worktree del
// review es otro directorio. Lo que no puede ser es que ese directorio se convierta
// en deuda: ni un worktree registrado, ni una rama nueva, ni un cambio sin
// commitear en el repo del usuario.
func TestSimulateLeavesNoTraceInTheLocalRepo(t *testing.T) {
	repo, review, it := fixture(t)
	before := testutil.RunGit(t, repo, "branch", "--format=%(refname)")
	svc := newService(t, stubLocator{place: Place{Repo: repo, Branch: "prdash/pr-7"}, ok: true}, fakeSim(t, writeJPEG(t)))

	res, err := svc.Simulate(context.Background(), it, KindMerge)
	if err != nil {
		t.Fatalf("Simulate: %v", err)
	}
	if res.Kind != KindMerge || res.Ref != "prdash/pr-7" || res.Base != "main" {
		t.Errorf("Result = %+v", res)
	}
	if _, err := os.Stat(res.Path); err != nil {
		t.Errorf("la imagen no se conservó en %s: %v", res.Path, err)
	}

	if after := testutil.RunGit(t, repo, "branch", "--format=%(refname)"); after != before {
		t.Errorf("el repo ganó o perdió ramas:\n%s\nwant\n%s", after, before)
	}
	if n := strings.Count(testutil.RunGit(t, repo, "worktree", "list"), "\n"); n != 0 {
		t.Errorf("quedan %d worktrees en el repo", n)
	}
	// El worktree del review sigue siendo suyo, en su rama y limpio.
	if got := strings.TrimSpace(testutil.RunGit(t, review, "rev-parse", "--abbrev-ref", "HEAD")); got != "prdash/pr-7" {
		t.Errorf("el worktree del review quedó en %q", got)
	}
	if out := testutil.RunGit(t, review, "status", "--porcelain"); strings.TrimSpace(out) != "" {
		t.Errorf("el render dejó cambios sin commitear en el worktree del review:\n%s", out)
	}
}

// TestRebaseRunsFromTheItemBranch: el rebase parte de la rama del ítem, así que
// es la que debe quedar activa en el clon de la simulación, y el ref contra el
// que corre es la base. Invertirlo dibujaría el grafo al revés.
func TestRebaseRunsFromTheItemBranch(t *testing.T) {
	repo, _, it := fixture(t)
	// Un runner que registra el directorio y el comando con el que se llamó.
	dir := t.TempDir()
	bin := writeScript(t, dir, "git-sim", "#!/bin/sh\n"+
		"echo \"$PWD|$*|$(git rev-parse --abbrev-ref HEAD)\" > "+filepath.Join(dir, "cwd")+"\n"+
		"media=\"\"\n"+
		"while [ $# -gt 0 ]; do if [ \"$1\" = \"--media-dir\" ]; then media=\"$2\"; fi; shift; done\n"+
		"mkdir -p \"$media/images\" && cp "+writeJPEG(t)+" \"$media/images/out.jpg\" && echo \"$media/images/out.jpg\"\n")

	svc := newService(t, stubLocator{place: Place{Repo: repo, Branch: "prdash/pr-7"}, ok: true}, bin)
	res, err := svc.Simulate(context.Background(), it, KindRebase)
	if err != nil {
		t.Fatalf("Simulate: %v", err)
	}
	if res.Ref != "main" {
		t.Errorf("Ref = %q, want main: el rebase se hace contra la base", res.Ref)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "cwd"))
	if err != nil {
		t.Fatal(err)
	}
	cwd, rest, _ := strings.Cut(strings.TrimSpace(string(raw)), "|")
	if cwd == repo {
		t.Fatal("el render corrió en el repo de origen en vez del clon temporal")
	}
	args, head, _ := strings.Cut(rest, "|")
	if !strings.HasSuffix(args, " rebase main") {
		t.Errorf("argv = %q, want un rebase contra main", args)
	}
	// La rama activa tiene que ser la del ítem: de ahí es de donde se rebasa, y
	// git-sim dibuja desde donde está HEAD.
	if head != "prdash/pr-7" {
		t.Errorf("HEAD = %q, want prdash/pr-7", head)
	}
}

// TestSimulateNeedsAMountedReview: sin review montado no hay refs locales, y la
// simulación lo dice en vez de clonar o adivinar una ruta.
func TestSimulateNeedsAMountedReview(t *testing.T) {
	_, _, it := fixture(t)
	svc := newService(t, stubLocator{}, fakeSim(t, writeJPEG(t)))

	_, err := svc.Simulate(context.Background(), it, KindMerge)
	if err == nil || !strings.Contains(err.Error(), "mounted") {
		t.Fatalf("err = %v, want un aviso de que falta montar la review", err)
	}
}

// TestSimulateNeedsATargetBranch: sin base no hay contra qué comparar, y un
// destino inventado sería una simulación de otra cosa.
func TestSimulateNeedsATargetBranch(t *testing.T) {
	repo, _, it := fixture(t)
	it.TargetBranch = ""
	svc := newService(t, stubLocator{place: Place{Repo: repo, Branch: "prdash/pr-7"}, ok: true}, fakeSim(t, writeJPEG(t)))

	if _, err := svc.Simulate(context.Background(), it, KindMerge); err == nil {
		t.Fatal("Simulate aceptó un ítem sin rama base")
	}
}

// TestSimulateFailsWhenTheBaseIsNotInTheClone: una base que no está en el clon no
// se puede comparar, y decirlo es mejor que renderizar un grafo vacío. El aviso
// tiene que nombrar la base que falta: sin ella, "no se pudo simular" obligaría a
// ir a mirar a mano qué rama era.
func TestSimulateFailsWhenTheBaseIsNotInTheClone(t *testing.T) {
	repo, _, it := fixture(t)
	it.TargetBranch = "release/9"
	svc := newService(t, stubLocator{place: Place{Repo: repo, Branch: "prdash/pr-7"}, ok: true}, fakeSim(t, writeJPEG(t)))

	_, err := svc.Simulate(context.Background(), it, KindMerge)
	if err == nil || !strings.Contains(err.Error(), "release/9") {
		t.Fatalf("err = %v, want que nombre la base que no encuentra", err)
	}
}

// TestMaterializeFallsBackToTheRemoteRef: un clon con la base solo como ref
// remoto sigue siendo utilizable, porque lo que importa es comparar contra el
// mismo commit que tiene el destino. Es el caso real cuando el resolutor apuntó
// al clon local del usuario y la rama se borró allí.
func TestMaterializeFallsBackToTheRemoteRef(t *testing.T) {
	repo, _, it := fixture(t)
	clone := filepath.Join(t.TempDir(), "clone")
	testutil.RunGit(t, t.TempDir(), "clone", "--quiet", repo, clone)
	// La rama base desaparece del clon, pero sigue estando en el remoto.
	testutil.RunGit(t, clone, "update-ref", "-d", "refs/heads/main")

	svc := newService(t, stubLocator{place: Place{Repo: clone}, ok: true}, fakeSim(t, writeJPEG(t)))
	if err := svc.materialize(context.Background(), clone, it.TargetBranch); err != nil {
		t.Fatalf("materialize: %v", err)
	}
	if !testutil.RefExists(t, clone, "refs/heads/main") {
		t.Error("materialize no creó la rama local a partir del ref remoto")
	}
}

// TestMaterializePrefersTheLocalBranch: si el clon tiene la base local, es esa la
// que hay que comparar, aunque el remoto vaya por delante. Usar la del remoto
// dibujaría un grafo que no es el de la review.
func TestMaterializePrefersTheLocalBranch(t *testing.T) {
	repo, _, it := fixture(t)
	clone := filepath.Join(t.TempDir(), "clone")
	testutil.RunGit(t, t.TempDir(), "clone", "--quiet", repo, clone)
	// Se avanza la base solo en el clon local: ahora local y remoto divergen.
	testutil.RunGit(t, clone, "checkout", "--quiet", "main")
	testutil.CommitFile(t, clone, "solo-local.txt", "x", "avance local")

	localSHA := strings.TrimSpace(testutil.RunGit(t, clone, "rev-parse", "main"))
	remoteSHA := strings.TrimSpace(testutil.RunGit(t, repo, "rev-parse", "main"))
	if localSHA == remoteSHA {
		t.Skip("el clon y el remoto apuntan al mismo commit; no hay divergencia que probar")
	}

	svc := newService(t, stubLocator{place: Place{Repo: clone, Branch: "prdash/pr-7"}, ok: true}, fakeSim(t, writeJPEG(t)))
	if err := svc.materialize(context.Background(), clone, it.TargetBranch); err != nil {
		t.Fatalf("materialize: %v", err)
	}
	if got := strings.TrimSpace(testutil.RunGit(t, clone, "rev-parse", "main")); got != localSHA {
		t.Errorf("main = %s, want la local %s (el remoto es %s)", got, localSHA, remoteSHA)
	}
}

// TestPruneKeepsTheNewest: el caché no se limpia solo y un popup que se puede
// abrir con el visor hace que valga la pena conservar las imágenes, pero no
// todas para siempre.
func TestPruneKeepsTheNewest(t *testing.T) {
	dir := t.TempDir()
	for i, name := range []string{"a.jpg", "b.jpg", "c.jpg"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		stamp := timeAt(int64(i))
		_ = os.Chtimes(path, stamp, stamp)
	}
	if err := os.WriteFile(filepath.Join(dir, "notimage.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	prune(dir, 2)

	if _, err := os.Stat(filepath.Join(dir, "a.jpg")); !os.IsNotExist(err) {
		t.Error("no podó la imagen más antigua")
	}
	if _, err := os.Stat(filepath.Join(dir, "c.jpg")); err != nil {
		t.Errorf("podó una imagen reciente: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "notimage.txt")); err != nil {
		t.Errorf("tocó un fichero que no es una imagen: %v", err)
	}
}

// timeAt construye una marca de tiempo para agear ficheros en el test.
func timeAt(offset int64) time.Time {
	return time.Unix(1_700_000_000+offset*60, 0)
}
