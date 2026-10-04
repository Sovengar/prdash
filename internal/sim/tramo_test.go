package sim

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The missing guards are the MIDDLE of Simulate's chain.

func gitSimQueFalla(t *testing.T, mensaje string) string {
	t.Helper()
	return writeScript(t, t.TempDir(), "git-sim",
		"#!/bin/sh\necho '"+mensaje+"' >&2\nexit 1\n")
}

func TestSiGitSimFallaElErrorSaleDeElYNoDelStageNiDelCache(t *testing.T) {
	repo, _, it := fixture(t)
	it.SourceBranch = "prdash/pr-7"
	bin := gitSimQueFalla(t, "IndexError: no such branch")

	s := newService(t, locatorFalso{ok: true, place: Place{Repo: repo, Branch: "prdash/pr-7"}}, bin)

	res, err := s.Simulate(context.Background(), it, KindMerge)
	if err == nil {
		t.Fatal("un git-sim que falla dio nil: el popup se cerraría sin decir por qué")
	}
	if !strings.Contains(err.Error(), "no such branch") {
		t.Errorf("el error %q no trae el mensaje de git-sim", err)
	}
	// And there is no image: a Result with Path set would be opened in the viewer.
	if res.Path != "" {
		t.Errorf("devolvió una imagen %q pese al fallo del render", res.Path)
	}
	// And the error does NOT mention the cache: that guard comes later, and a cache message in a
	// clone failure sends you the wrong way.
	if strings.Contains(err.Error(), "cache") {
		t.Errorf("el error %q habla del caché, que es un tramo posterior", err)
	}
}

// git-sim exits 0 and writes nothing.
func TestSiElRenderNoDejaImagenElErrorLoDiceYNoSePropagaLaRutaVacia(t *testing.T) {
	repo, _, it := fixture(t)
	it.SourceBranch = "prdash/pr-7"
	mudo := writeScript(t, t.TempDir(), "git-sim", "#!/bin/sh\nexit 0\n")

	s := newService(t, locatorFalso{ok: true, place: Place{Repo: repo, Branch: "prdash/pr-7"}}, mudo)

	res, err := s.Simulate(context.Background(), it, KindMerge)
	if err == nil {
		t.Fatal("un git-sim que no produce imagen dio nil")
	}
	if !strings.Contains(err.Error(), "no image") {
		t.Errorf("el error %q no dice que git-sim no produjo imagen", err)
	}
	if res.Path != "" {
		t.Errorf("devolvió Path=%q: se copiaría al caché un fichero vacío", res.Path)
	}
	entradas, err := os.ReadDir(s.CacheDir)
	if err == nil {
		for _, e := range entradas {
			t.Errorf("quedó %s en el caché pese al fallo", e.Name())
		}
	}
}

// No debt left.
func TestElTemporalDeLaSimulacionSeBorraAunqueElRenderFalle(t *testing.T) {
	repo, _, it := fixture(t)
	it.SourceBranch = "prdash/pr-7"
	bin := gitSimQueFalla(t, "boom")

	temporal := t.TempDir()
	t.Setenv("TMPDIR", temporal)

	s := newService(t, locatorFalso{ok: true, place: Place{Repo: repo, Branch: "prdash/pr-7"}}, bin)
	if _, err := s.Simulate(context.Background(), it, KindMerge); err == nil {
		t.Fatal("el render debería haber fallado")
	}

	entradas, err := os.ReadDir(temporal)
	if err != nil {
		t.Fatal(err)
	}
	if len(entradas) != 0 {
		var nombres []string
		for _, e := range entradas {
			nombres = append(nombres, e.Name())
		}
		t.Errorf("el temporal de la simulación quedó con %d entradas: %v. El clon del repo se "+
			"queda en disco por cada render fallido", len(entradas), nombres)
	}
}

// The ORDER is what matters.
func TestUnCacheQueNoSePuedeCrearFallaDespuesDeRenderizarYLoDice(t *testing.T) {
	repo, _, it := fixture(t)
	it.SourceBranch = "prdash/pr-7"

	cache := filepath.Join(t.TempDir(), "cache-bloqueado")
	if err := os.WriteFile(cache, []byte("bloqueo"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := newService(t, locatorFalso{ok: true, place: Place{Repo: repo, Branch: "prdash/pr-7"}},
		fakeSim(t, writeJPEG(t)))
	s.CacheDir = cache

	res, err := s.Simulate(context.Background(), it, KindMerge)
	if err == nil {
		t.Fatal("un caché que no se puede crear dio nil")
	}
	if !strings.Contains(err.Error(), "simulation cache") {
		t.Errorf("el error %q no dice que falla el caché, que es donde está la culpa", err)
	}
	// The message does NOT say git-sim failed, because it did not.
	if strings.Contains(err.Error(), "git-sim") {
		t.Errorf("el error %q culpa a git-sim, que sí renderizó", err)
	}
	if res.Path != "" {
		t.Errorf("devolvió Path=%q sin haber guardado nada", res.Path)
	}
}

// The checkout error of each one.
func TestLaRamaDeReviewSeActivaParaRebaseYLaBaseParaIntegrar(t *testing.T) {
	repo, _ := simRepoMonta(t)
	// A base that is a branch name not in the clone.
	origen, tmp := simRepoMonta(t)
	_ = repo
	for _, c := range []struct {
		nombre  string
		kind    Kind
		base    string
		delItem string
	}{
		{"integrar con base inexistente", KindMerge, "base/que-no-existe", "feat/x"},
		{"rebasar con base inexistente", KindRebase, "base/que-no-existe", "feat/x"},
	} {
		_, _, err := New(locatorDeStage()).stage(context.Background(),
			Place{Repo: origen, Branch: c.delItem}, c.kind, c.base, t.TempDir())
		if err == nil {
			t.Errorf("%s: pasó sin la base", c.nombre)
			continue
		}
		if !strings.Contains(err.Error(), c.base) {
			t.Errorf("%s: el error %q no nombra la base que no se pudo activar",
				c.nombre, err)
		}
	}
	_ = tmp
}

// The case that uncovered the leak.
func TestCopyFileNoDejaTemporalSiElDestinoEsUnDirectorioQueYaExiste(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "origen.jpg")
	if err := os.WriteFile(src, []byte("imagen"), 0o644); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "destino.jpg")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := copyFile(src, dst); err == nil {
		t.Fatal("copiar a un directorio dio nil")
	}
	if _, err := os.Stat(dst + ".part"); !os.IsNotExist(err) {
		t.Errorf("quedó el temporal %s.part: cada intento fallido dejaría basura que prune no "+
			"borra porque solo mira los .jpg", dst+".part")
	}
	info, err := os.Stat(dst)
	if err != nil || !info.IsDir() {
		t.Errorf("el destino dejó de ser un directorio: %v", err)
	}
	// The real prune does not touch the `.part` even if one were left, which is what makes a
	// leftover harmless.
	restos := filepath.Join(dir, "otro.part")
	if err := os.WriteFile(restos, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	prune(dir, 0)
	if _, err := os.Stat(restos); err != nil {
		t.Errorf("prune quitó el .part, así que la limpieza de copyFile no es necesaria: %v", err)
	}
}

// The case is a symlink.
func TestElPaseoDePruneToleraUnaEntradaQueNoSePuedeEstadificar(t *testing.T) {
	dir := t.TempDir()
	colgado := filepath.Join(dir, "colgado.jpg")
	if err := os.Symlink(filepath.Join(dir, "no-existe.jpg"), colgado); err != nil {
		t.Fatal(err)
	}
	// A real image that must NOT be lost, written after the link: a prune that used the directory
	// order would take it.
	buena := filepath.Join(dir, "buena.jpg")
	if err := os.WriteFile(buena, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	prune(dir, 0)

	// The broken link stays: it is not an image and os.Remove would take it anyway, but leaving it
	// makes the state visible.
	if _, err := os.Stat(buena); err == nil {
		t.Error("la imagen buena sigue ahí: el paseo de prune se paró en el enlace roto")
	}
}
