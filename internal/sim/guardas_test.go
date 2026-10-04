package sim

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

// INFRASTRUCTURE failures: no disk, no HOME, repo gone.

// A file where a directory was expected denies a write without needing permissions.
func bloqueaConUnFichero(t *testing.T, ruta string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(ruta), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ruta, []byte("bloqueo"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// os.UserCacheDir fails with neither XDG_CACHE_HOME nor HOME.
func TestSinCacheDirNoSePuedeSimularYElErrorLoDice(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("HOME", "")

	if _, err := DefaultCacheDir(); err == nil {
		t.Fatal("DefaultCacheDir sin HOME ni XDG_CACHE_HOME dio nil")
	}

	s := &Service{}
	if _, err := s.cacheDir(); err == nil {
		t.Error("cacheDir sin CacheDir ni entorno dio nil")
	}

	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	dir, err := DefaultCacheDir()
	if err != nil {
		t.Fatalf("DefaultCacheDir con entorno: %v", err)
	}
	if filepath.Base(dir) != "sim" {
		t.Errorf("DefaultCacheDir dio %q, want .../prdash/sim", dir)
	}
}

// The one that fires in a container.
func TestSinTMPDIRNoSePreparaElDirectorioDeLaSimulacion(t *testing.T) {
	bloqueaConUnFichero(t, filepath.Join(t.TempDir(), "tmp-bloqueado"))
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "tmp-bloqueado"))

	repo, review, it := fixture(t)
	it.SourceBranch = "prdash/pr-7"
	s := newService(t, locatorFalso{ok: true, place: Place{Repo: repo, Branch: "prdash/pr-7"}},
		fakeSim(t, writeJPEG(t)))

	_, err := s.Simulate(context.Background(), it, KindMerge)
	if err == nil {
		t.Fatal("sin TMPDIR utilizable se simuló igualmente")
	}
	if !strings.Contains(err.Error(), "simulation directory") {
		t.Errorf("el error %q no dice que falla el directorio de la simulación: hay tres "+
			"temporales en el camino y saber cuál es dice si hay que esperar", err)
	}
	_ = review
}

// The ORDER is the point: the clone is what fails, not the checkout.
func TestUnRepoQueYaNoEstaFallaAlClonarYNoAlCheckout(t *testing.T) {
	// The repo Locator gives does not exist, which is stronger than deleting it afterwards.
	s := newService(t, locatorFalso{ok: true, place: Place{
		Repo: filepath.Join(t.TempDir(), "repo-que-no-existe"), Branch: "prdash/pr-7",
	}}, fakeSim(t, writeJPEG(t)))

	_, _, err := s.stage(context.Background(), s.locatorPlace(), KindMerge, "main", t.TempDir())
	if err == nil {
		t.Fatal("se clonó un repo que no existe")
	}
	if !strings.Contains(err.Error(), "clone") {
		t.Errorf("el error %q no dice que falla el clon", err)
	}
	if !strings.Contains(err.Error(), "repo-que-no-existe") {
		t.Errorf("el error %q no nombra el repo que no se pudo clonar", err)
	}
}

// The asymmetry with the clone error.
func TestUnaRamaQueNoEstaEnElClonFallaAlMaterializar(t *testing.T) {
	repo, _ := simRepoMonta(t)

	for _, c := range []struct {
		nombre  string
		base    string
		delItem string
		falta   string
	}{
		// In both cases ONE of the two is missing and the other is fine, so the error has to name the one that
		//failed.
		{"falta la rama del ítem", "main", "feat/inexistente", "feat/inexistente"},
		{"falta la base", "base/inexistente", "feat/x", "base/inexistente"},
	} {
		_, _, err := New(locatorDeStage()).stage(context.Background(),
			Place{Repo: repo, Branch: c.delItem}, KindMerge, c.base, t.TempDir())
		if err == nil {
			t.Errorf("%s: pasó sin la rama", c.nombre)
			continue
		}
		if !strings.Contains(err.Error(), c.falta) {
			t.Errorf("%s: el error %q no nombra la ref que falta (%q)",
				c.nombre, err, c.falta)
		}
	}

	// The good case, or "always fails" would prove nothing about which guard fired.
	if _, _, err := New(locatorDeStage()).stage(context.Background(),
		Place{Repo: repo, Branch: "feat/x"}, KindMerge, "main", t.TempDir()); err != nil {
		t.Errorf("una rama que sí existe dio error: %v", err)
	}
}

func TestKeepPropagaElFalloDeCrearElCacheYNoDejaNada(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "cache-bloqueado")
	bloqueaConUnFichero(t, cache)

	s := &Service{CacheDir: cache}
	_, err := s.keep(writeJPEG(t), itemConRama("prdash/pr-7"), KindMerge)
	if err == nil {
		t.Fatal("un caché que no se puede crear dio nil")
	}
	if !strings.Contains(err.Error(), "simulation cache") {
		t.Errorf("el error %q no dice que falla el caché", err)
	}
	info, statErr := os.Stat(cache)
	if statErr == nil && info.IsDir() {
		t.Error("se creó el caché a medias: el siguiente intento lo encontraría existente")
	}
}

// A failure that slips through easily.
func TestGitSimQueNoProduceImagenLoDice(t *testing.T) {
	mudo := writeScript(t, t.TempDir(), "git-sim", "#!/bin/sh\nexit 0\n")

	r := &Runner{Bin: mudo}
	_, err := r.Render(context.Background(), t.TempDir(), t.TempDir(), Spec{Kind: KindMerge, Ref: "main"})
	if err == nil {
		t.Fatal("un git-sim que no produce imagen dio nil")
	}
	if !strings.Contains(err.Error(), "no image") {
		t.Errorf("el error %q no dice que git-sim no produjo imagen", err)
	}
	// The binary's name is in there: an error without the command that caused it sends you looking.
	if !strings.Contains(err.Error(), "git-sim") {
		t.Errorf("el error %q no nombra git-sim", err)
	}
}

// The chain end to end, with no mocks.
func TestSimulatePropagaElFalloDelStageYDiceQueVinoDeAhí(t *testing.T) {
	noExiste := filepath.Join(t.TempDir(), "repo-que-no-existe")
	s := newService(t, locatorFalso{ok: true, place: Place{Repo: noExiste, Branch: "prdash/pr-7"}},
		fakeSim(t, writeJPEG(t)))

	res, err := s.Simulate(context.Background(), itemConRama("prdash/pr-7"), KindMerge)
	if err == nil {
		t.Fatal("Simulate con un repo inexistente dio nil")
	}
	// And the empty result: a Result with Path set and a missing image would be opened.
	if res.Path != "" {
		t.Errorf("devolvió una imagen %q pese al fallo", res.Path)
	}
	if !strings.Contains(err.Error(), noExiste) {
		t.Errorf("el error %q no nombra el repo que falló", err)
	}
}

func (s *Service) locatorPlace() Place {
	if l, ok := s.Locator.(locatorFalso); ok {
		return l.place
	}
	return Place{}
}

func itemConRama(branch string) model.Item {
	it := model.NewItem(model.RepoRef{
		Forge: "github", Host: "github.com", Project: "acme/widget",
	}, 7)
	it.SourceBranch = branch
	it.TargetBranch = "main"
	return it
}
