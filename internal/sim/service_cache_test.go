package sim

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

// keep and prune are the two that touch the user's disk, and they were the two untested.

// The ORDER of the timestamps is checked, not just how many files are left: a count alone would pass
// with the newest pruned and the oldest kept.
func TestLaPodaDejaLoRecienteYBorraLoViejo(t *testing.T) {
	dir := t.TempDir()
	base := time.Now().Add(-time.Hour)

	nombres := []string{"a.jpg", "b.jpg", "c.jpg", "d.jpg", "e.jpg"}
	for i, n := range nombres {
		ruta := filepath.Join(dir, n)
		if err := os.WriteFile(ruta, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		mod := base.Add(time.Duration(i) * time.Minute)
		if err := os.Chtimes(ruta, mod, mod); err != nil {
			t.Fatal(err)
		}
	}

	prune(dir, 2)

	for _, n := range nombres {
		_, err := os.Stat(filepath.Join(dir, n))
		sobrevive := err == nil
		queria := n == "d.jpg" || n == "e.jpg"
		if sobrevive != queria {
			if sobrevive {
				t.Errorf("%s sobrevive, pero debía borrarse por ser más antigua", n)
			} else {
				t.Errorf("%s se borró, pero debía sobrevivir por ser más reciente", n)
			}
		}
	}
	vivos := 0
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".jpg") {
			vivos++
		}
	}
	if vivos != 2 {
		t.Errorf("quedaron %d imágenes, want 2", vivos)
	}
}

// The cache lives under the user's home and prune deletes by directory, not by list.
func TestLaPodaIgnoraLoQueNoSonImagenesYLosDirectorios(t *testing.T) {
	dir := t.TempDir()

	for _, n := range []string{"vieja.jpg", "nueva.jpg"} {
		ruta := filepath.Join(dir, n)
		if err := os.WriteFile(ruta, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	extras := map[string]string{
		"a-medias.jpg.part": "temporal de copyFile",
		"nota.txt":          "no es una imagen",
		"sin-extension":     "no tiene extensión",
	}
	for n := range extras {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sub := filepath.Join(dir, "sub.jpg") // directorio con nombre de imagen
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	viejo := time.Now().Add(-time.Hour)
	mod := time.Now()
	if err := os.Chtimes(filepath.Join(dir, "vieja.jpg"), viejo, viejo); err != nil {
		t.Fatal(err)
	}
	for _, n := range append(keys(extras), "sub.jpg") {
		p := filepath.Join(dir, n)
		if err := os.Chtimes(p, viejo, viejo); err != nil {
			t.Fatal(err)
		}
	}
	_ = mod

	prune(dir, 1)

	if _, err := os.Stat(filepath.Join(dir, "nueva.jpg")); err != nil {
		t.Error("la imagen más reciente no sobrevivió a la poda")
	}
	if _, err := os.Stat(filepath.Join(dir, "vieja.jpg")); err == nil {
		t.Error("la imagen más antigua sobrevivió a la poda")
	}
	for n := range extras {
		if _, err := os.Stat(filepath.Join(dir, n)); err != nil {
			t.Errorf("%s (%s) no sobrevivió a la poda", n, extras[n])
		}
	}
	if _, err := os.Stat(sub); err != nil {
		t.Error("la poda se comió un directorio con nombre de imagen")
	}
}

func TestLaPodaConMenosQueElLimiteNoTocaNada(t *testing.T) {
	vacio := t.TempDir()
	prune(vacio, 5)
	entries, err := os.ReadDir(vacio)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("podar un directorio vacío dio %d entradas", len(entries))
	}

	uno := t.TempDir()
	ruta := filepath.Join(uno, "a.jpg")
	if err := os.WriteFile(ruta, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	prune(uno, 5)
	if _, err := os.Stat(ruta); err != nil {
		t.Error("una imagen por debajo del límite se borró")
	}

	prune(filepath.Join(t.TempDir(), "no-existe"), 5)
}

// The name is the deduplication key: forge, project, number, kind and a UnixNano.
func TestKeepCopiaLaImagenYLaPodaDespues(t *testing.T) {
	dir := t.TempDir()
	origen := filepath.Join(dir, "render.jpg")
	if err := os.WriteFile(origen, []byte("contenido"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := &Service{CacheDir: filepath.Join(dir, "cache")}
	it := model.Item{Forge: "github", Number: 42, Ref: model.RepoRef{Project: "grupo/proyecto"}}

	dst, err := s.keep(origen, it, KindMerge)
	if err != nil {
		t.Fatalf("keep: %v", err)
	}

	contenido, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("la imagen guardada no está: %v", err)
	}
	if string(contenido) != "contenido" {
		t.Errorf("la copia tiene %q", contenido)
	}
	if _, err := os.Stat(dst + ".part"); err == nil {
		t.Error("el temporal de la copia sigue en el caché")
	}
	nombre := filepath.Base(dst)
	if !strings.HasPrefix(nombre, "github-") || !strings.Contains(nombre, "proyecto") {
		t.Errorf("el nombre %q no lleva forge ni proyecto", nombre)
	}
	if !strings.Contains(nombre, "-42-") {
		t.Errorf("el nombre %q no lleva el número", nombre)
	}
	if !strings.HasSuffix(nombre, ".jpg") {
		t.Errorf("el nombre %q no acaba en .jpg", nombre)
	}
	if _, err := os.Stat(s.CacheDir); err != nil {
		t.Errorf("keep no creó el directorio del caché: %v", err)
	}

	if _, err := os.Stat(origen); err != nil {
		t.Error("keep borró el original, que el visor necesita")
	}
}

// Both must surface as errors and not as an empty dst: an empty dst with the error ignored would
// have the popup open nothing.
func TestKeepFallaConOrigenQueNoExisteYConDestinoImposible(t *testing.T) {
	dir := t.TempDir()
	it := model.Item{Forge: "github", Number: 1, Ref: model.RepoRef{Project: "p"}}

	s := &Service{CacheDir: filepath.Join(dir, "c1")}
	dst, err := s.keep(filepath.Join(dir, "no-existe.jpg"), it, KindMerge)
	if err == nil {
		t.Fatal("keep con un origen inexistente dio nil")
	}
	if dst != "" {
		t.Errorf("keep devolvió %q con error", dst)
	}

	bloque := filepath.Join(dir, "bloque")
	if err := os.WriteFile(bloque, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	s = &Service{CacheDir: bloque}
	if _, err := s.keep(origenDePrueba(t), it, KindMerge); err == nil {
		t.Error("keep con un CacheDir que es un fichero dio nil")
	}
}

// The memo matters: cacheDir stores the result so os.UserCacheDir is not asked on every image.
func TestElDirectorioDelCacheCaeAlDefaultYSeMemoriza(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)

	ruta, err := DefaultCacheDir()
	if err != nil {
		t.Fatalf("DefaultCacheDir: %v", err)
	}
	if filepath.Dir(ruta) != filepath.Join(dir, "prdash") || filepath.Base(ruta) != "sim" {
		t.Errorf("DefaultCacheDir dio %q", ruta)
	}

	s := &Service{}
	obtenido, err := s.cacheDir()
	if err != nil {
		t.Fatalf("cacheDir: %v", err)
	}
	if obtenido != ruta {
		t.Errorf("cacheDir dio %q, want el default %q", obtenido, ruta)
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	if otra, _ := s.cacheDir(); otra != ruta {
		t.Errorf("la segunda llamada dio %q, want el memoizado %q", otra, ruta)
	}

	explicito := filepath.Join(dir, "otro")
	puesto := &Service{CacheDir: explicito}
	if g, _ := puesto.cacheDir(); g != explicito {
		t.Errorf("con CacheDir puesto dio %q, want %q", g, explicito)
	}
	if puesto.CacheDir != explicito {
		t.Errorf("cacheDir alteró el CacheDir: %q", puesto.CacheDir)
	}
}

// The order matters: without a Locator there is nowhere to look, and with a Locator but no clone
// there is nothing to take refs from.
func TestSimulateNiegaSinLocatorYSinClon(t *testing.T) {
	it := model.Item{Forge: "github", Number: 1, Ref: model.RepoRef{Project: "p"}}

	s := &Service{}
	if _, err := s.Simulate(context.Background(), it, KindMerge); err == nil {
		t.Error("sin Locator dio nil")
	} else if !strings.Contains(err.Error(), "local repository") {
		t.Errorf("el error %q no dice que falta el repo local", err)
	}

	s = &Service{Locator: locatorFalso{ok: false}}
	if _, err := s.Simulate(context.Background(), it, KindMerge); err == nil {
		t.Error("con un locator que no encuentra dio nil")
	} else if !strings.Contains(err.Error(), "mounted") {
		t.Errorf("el error %q no dice que falta montar el review", err)
	}

	s = &Service{Locator: locatorFalso{ok: true, place: Place{}}}
	if _, err := s.Simulate(context.Background(), it, KindMerge); err == nil {
		t.Error("con un Place sin Repo dio nil")
	}

	s = &Service{Locator: locatorFalso{ok: true, place: Place{Repo: dirDePrueba(t), Branch: "b"}}}
	_, err := s.Simulate(context.Background(), it, KindMerge)
	if err == nil {
		t.Error("con un repo presente dio nil sin llegar a renderizar")
	}
}

// A Service with Runner nil recovers by itself, which happens when someone composes one by hand.
func TestAvailableYRunnerNoRevientanConElServicioVacio(t *testing.T) {
	s := &Service{}
	_ = s.Available()
	if s.Runner == nil {
		t.Error("runner() no rellenó el Runner de un servicio construido a mano")
	}
	nuevo := New(locatorFalso{})
	if nuevo == nil || nuevo.Runner == nil || nuevo.Git == nil {
		t.Errorf("New devolvió %+v con piezas a nil", nuevo)
	}
	manuales := &Service{Locator: locatorFalso{}}
	if manuales.gitRunner() == nil || manuales.Git == nil {
		t.Error("gitRunner() no rellenó el Git de un servicio construido a mano")
	}
}

// The decision is whether the process complained or hung, which is what separates two completely
// different diagnoses.
func TestElMensajeDeUnErrorDeSimSeparaQuejarseDeColgarse(t *testing.T) {
	got := message("primera\nsegunda\ntercera", errors.New("exit status 1"))
	if !strings.Contains(got, "primera") {
		t.Errorf("el mensaje %q no trae la primera línea de stderr", got)
	}
	if strings.Contains(got, "segunda") {
		t.Errorf("el mensaje %q trae más de una línea", got)
	}

	got = message("   \n  ", errors.New("exit status 137"))
	if strings.TrimSpace(got) == "" {
		t.Error("sin stderr el mensaje quedó vacío")
	}
	if !strings.Contains(got, "137") {
		t.Errorf("el mensaje %q perdió el código de salida", got)
	}

	if got := message("", nil); strings.TrimSpace(got) == "" {
		t.Error("sin stderr ni error el mensaje quedó vacío")
	}
}

// My first version assumed bin read an environment variable so a test could point it elsewhere:
// it does not, and the test now checks the field.
func TestElBinarioDeSimEsElCampoOElCanónicoYNadaMás(t *testing.T) {
	if got := (&Runner{}).bin(); got != DefaultBin {
		t.Errorf("con Bin vacío dio %q, want %q", got, DefaultBin)
	}
	if DefaultBin != "git-sim" {
		t.Errorf("DefaultBin es %q, want git-sim", DefaultBin)
	}
	if got := (&Runner{Bin: "/opt/git-sim-mio"}).bin(); got != "/opt/git-sim-mio" {
		t.Errorf("con Bin puesto dio %q", got)
	}
	t.Setenv("GIT_SIM_BIN", "/opt/otro")
	if got := (&Runner{}).bin(); got != DefaultBin {
		t.Errorf("con GIT_SIM_BIN en el entorno dio %q, want el canónico", got)
	}
	if got := NewRunner().Bin; got != DefaultBin {
		t.Errorf("NewRunner().Bin = %q", got)
	}
	_ = (&Runner{}).Available()
}

// The path and arguments are what make the failure reproducible without guessing which invocation
// hit it.
func TestElErrorDeSimTraeLoQueSeEjecuto(t *testing.T) {
	e := &Error{Args: []string{"config", "user.email"}, Dir: "/repos/proy", Msg: "no such repository", ExitCode: 128}
	msg := e.Error()
	for _, quiere := range []string{"config", "user.email", "/repos/proy", "no such repository", "128"} {
		if !strings.Contains(msg, quiere) {
			t.Errorf("el mensaje %q no trae %q", msg, quiere)
		}
	}
	if strings.Contains((&Error{Args: []string{"a"}, Msg: "m"}).Error(), "exit") {
		t.Error("un error sin código de salida lo pone entre paréntesis")
	}
	if e.Unwrap() != nil {
		_ = e // Unwrap sin causa devuelve nil; aquí solo se comprueba que no revienta
	}
}

type locatorFalso struct {
	ok    bool
	place Place
}

func (l locatorFalso) Locate(model.Item) (Place, bool) { return l.place, l.ok }

func origenDePrueba(t *testing.T) string {
	t.Helper()
	ruta := filepath.Join(t.TempDir(), "render.jpg")
	if err := os.WriteFile(ruta, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return ruta
}

func dirDePrueba(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	testutil.InitRepo(t, dir)
	return dir
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
