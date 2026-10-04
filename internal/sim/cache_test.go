package sim

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"prdash/internal/forge/model"
)

// The case that proves it is a failure midway: a half-written destination would offer the popup a broken
// image under the right name.
func TestCopyFileATemporalDejaElDestinoIntegroYNoElTemporal(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "origen.jpg")
	if err := os.WriteFile(src, []byte("una imagen cualquiera"), 0o644); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "destino.jpg")

	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("leer el destino: %v", err)
	}
	if string(got) != "una imagen cualquiera" {
		t.Errorf("el destino tiene %q, want el contenido de origen", got)
	}
	if _, err := os.Stat(dst + ".part"); !os.IsNotExist(err) {
		t.Errorf("quedó el temporal %s.part", dst)
	}

	if err := os.WriteFile(src, []byte("segunda version"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(src, dst); err != nil {
		t.Fatalf("sobreescribir el destino: %v", err)
	}
	if got, _ := os.ReadFile(dst); string(got) != "segunda version" {
		t.Errorf("tras la segunda copia hay %q, want la segunda version", got)
	}
}

// No simulated cut needed: copying a DIRECTORY as if it were an image fails with EISDIR on the read.
func TestCopyFileLimpiaElTemporalCuandoLaCopiaSeCorta(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "esto-es-un-directorio.jpg")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "destino.jpg")

	err := copyFile(src, dst)
	if err == nil {
		t.Fatal("copiar un directorio dio nil")
	}
	if !errors.Is(err, fs.ErrInvalid) && !strings.Contains(err.Error(), "is a directory") {
		t.Logf("el error del SO no menciona EISDIR: %v", err)
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Error("la copia fallida dejó el destino: el popup abriría una imagen rota")
	}
	if _, err := os.Stat(dst + ".part"); !os.IsNotExist(err) {
		t.Error("la copia fallida dejó el temporal: se acumularía en el caché sin que nada lo borre")
	}
}

func TestCopyFileNombraElFalloDeCadaPunto(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "origen.jpg")
	if err := os.WriteFile(src, []byte("contenido"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		nombre   string
		src, dst string
	}{
		{"origen que no existe", filepath.Join(dir, "no-existe.jpg"),
			filepath.Join(dir, "d1.jpg")},
		{"padre del destino inexistente", src, filepath.Join(dir, "no-existe", "d2.jpg")},
		{"destino que es un directorio", src, dir},
	} {
		err := copyFile(c.src, c.dst)
		if err == nil {
			t.Errorf("%s: copyFile dio nil", c.nombre)
			continue
		}
		// The message names the file involved. os already puts the path in its error, so this checks that
		//keep's wrap does not swallow it.
		base := c.dst
		if c.nombre == "origen que no existe" {
			base = c.src
		}
		if !strings.Contains(err.Error(), filepath.Base(base)) {
			t.Errorf("%s: el error %q no nombra el fichero implicante", c.nombre, err)
		}
		if _, err := os.Stat(c.dst + ".part"); !os.IsNotExist(err) {
			t.Errorf("%s: dejó el temporal", c.nombre)
		}
	}
}

// "By date" is what a table test would miss: the filename carries a UnixNano and makes it look like
// name order is date order.
func TestPruneConservaLasMasRecientesYPorFechaNoPorNombre(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	// The names run opposite to the dates on purpose, so a prune sorting by name would keep exactly the
	//three it must not.
	for i := 9; i >= 1; i-- {
		nombre := itoa(i) + ".jpg"
		if err := os.WriteFile(filepath.Join(dir, nombre), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		cuando := base.AddDate(0, 0, i)
		if err := os.Chtimes(filepath.Join(dir, nombre), cuando, cuando); err != nil {
			t.Fatal(err)
		}
	}

	prune(dir, 3)

	restantes := nombresDe(t, dir)
	quiere := []string{"9.jpg", "8.jpg", "7.jpg"}
	if len(restantes) != len(quiere) {
		t.Fatalf("quedan %v, want %v", restantes, quiere)
	}
	for _, n := range quiere {
		if !contains(restantes, n) {
			t.Errorf("%s no se conservó: quedan %v", n, restantes)
		}
	}
	for _, n := range []string{"1.jpg", "2.jpg", "3.jpg", "4.jpg", "5.jpg", "6.jpg"} {
		if contains(restantes, n) {
			t.Errorf("%s se conservó y es de las más antiguas: quedan %v", n, restantes)
		}
	}
}

func TestPruneIgnoraLoQueNoEsUnaImagenYNoBorraElCacheEntero(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	tocables := []string{"a.jpg", "b.jpg", "c.jpg", "d.jpg"}
	for _, n := range tocables {
		p := filepath.Join(dir, n)
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, base, base); err != nil {
			t.Fatal(err)
		}
	}
	intocables := map[string]string{
		"backup.jpg":     "directorio",
		"notas.txt":      "texto",
		"HEAD":           "fichero",
		"candidata.JPEG": "imagen en mayusculas",
	}
	for n, clase := range intocables {
		p := filepath.Join(dir, n)
		var err error
		if clase == "directorio" {
			err = os.MkdirAll(filepath.Join(p, "contenido"), 0o755)
		} else {
			err = os.WriteFile(p, []byte("x"), 0o644)
		}
		if err != nil {
			t.Fatalf("preparar %s: %v", n, err)
		}
		if err := os.Chtimes(p, base, base); err != nil {
			t.Fatal(err)
		}
	}

	// A file named exactly `.jpg` IS pruned, because prune decides by suffix and HasSuffix(".jpg",".jpg")
	//is true. It was on the untouchable list, which is what broke the count below.
	conNombreDeExtension := filepath.Join(dir, ".jpg")
	if err := os.WriteFile(conNombreDeExtension, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	masViejo := base.AddDate(0, 0, -1)
	if err := os.Chtimes(conNombreDeExtension, masViejo, masViejo); err != nil {
		t.Fatal(err)
	}

	prune(dir, 1)

	if _, err := os.Stat(conNombreDeExtension); err == nil {
		t.Errorf("el fichero %q sobrevivió a la poda: prune decide por sufijo", ".jpg")
	}
	for n := range intocables {
		if _, err := os.Stat(filepath.Join(dir, n)); err != nil {
			t.Errorf("prune borró %s (%s): %v", n, intocables[n], err)
		}
	}
	quedan := 0
	for _, n := range tocables {
		if _, err := os.Stat(filepath.Join(dir, n)); err == nil {
			quedan++
		}
	}
	if quedan != 1 {
		t.Errorf("quedan %d imágenes de 4, want 1", quedan)
	}

	if _, err := os.Stat(filepath.Join(dir, "backup.jpg", "contenido")); err != nil {
		t.Errorf("prune vació el subdirectorio: %v", err)
	}
}

func TestPruneConMenosDeLosQueHayQueConservarNoHaceNadaYConCeroLosBorraTodos(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"a.jpg", "b.jpg"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	prune(dir, 10)
	if len(nombresDe(t, dir)) != 2 {
		t.Errorf("prune con keep=10 quitó ficheros: %v", nombresDe(t, dir))
	}
	// A NEGATIVE keep is clamped to zero instead of panicking: `files[-1:]` gave
	//`slice bounds out of range [-1:]`. My first version of this test asserted it did not panic
	//and I now check it the other way round.
	prune(dir, -1)
	if quedan := len(nombresDe(t, dir)); quedan != 0 {
		t.Errorf("con keep=-1 quedan %d ficheros, want 0 (negativo se lee como cero)", quedan)
	}

	prune(dir, 0)
	if quedan := nombresDe(t, dir); len(quedan) != 0 {
		t.Errorf("prune con keep=0 dejó %v", quedan)
	}
}

func TestPruneSobreUnDirectorioQueNoExisteNoRevienta(t *testing.T) {
	prune(filepath.Join(t.TempDir(), "no-existe"), 3)

	fichero := filepath.Join(t.TempDir(), "soy-un-fichero")
	if err := os.WriteFile(fichero, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	prune(fichero, 3)
	if _, err := os.Stat(fichero); err != nil {
		t.Errorf("prune se llevó un fichero que no era directorio: %v", err)
	}
}

func TestKeepCopiaAlCacheConElNombreDelItemYPodaLoQueSobra(t *testing.T) {
	cache := t.TempDir()
	origen := writeJPEG(t)

	s := &Service{CacheDir: cache}
	it := itemDePrueba()
	it.Number = 42

	dst, err := s.keep(origen, it, KindMerge)
	if err != nil {
		t.Fatalf("keep: %v", err)
	}
	a, err := os.ReadFile(origen)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("leer la copia del caché: %v", err)
	}
	if len(a) != len(b) {
		t.Fatalf("la copia pesa %d y el original %d", len(b), len(a))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("la copia difiere del original en el byte %d", i)
		}
	}
	nombre := filepath.Base(dst)
	if !strings.HasPrefix(nombre, "github-") {
		t.Errorf("el nombre %q no empieza por el forge", nombre)
	}
	if !strings.Contains(nombre, "-42-") {
		t.Errorf("el nombre %q no lleva el número del ítem", nombre)
	}
	if strings.ContainsAny(nombre, "/ #") {
		t.Errorf("el nombre %q lleva caracteres que un visor no abarca", nombre)
	}

	for i := 0; i < keepImages+1; i++ {
		p := filepath.Join(cache, "github-extra-"+itoa(i)+"-1.jpg")
		if err := os.WriteFile(p, []byte("basura"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ultima, err := s.keep(origen, it, KindMerge)
	if err != nil {
		t.Fatal(err)
	}
	// The image just saved cannot be deleted. My first version looked at `dst`, the image from the FIRST
	//keep call, which is the oldest, and failed intermittently.
	if _, err := os.Stat(ultima); err != nil {
		t.Errorf("la poda borró la imagen que se acaba de guardar: %v", err)
	}
	if ultima == dst {
		t.Error("las dos llamadas a keep dieron el mismo nombre: el UnixNano del nombre se " +
			"repite y una sobrescribiría a la otra")
	}
	quedan := 0
	for _, n := range nombresDe(t, cache) {
		if strings.HasSuffix(n, ".jpg") {
			quedan++
		}
	}
	if quedan != keepImages {
		t.Errorf("quedan %d imágenes tras la poda, want %d", quedan, keepImages)
	}
}

func itemDePrueba() model.Item {
	return model.NewItem(model.RepoRef{
		Forge: "github", Host: "github.com", Project: "acme/widget",
	}, 7)
}

func nombresDe(t *testing.T, dir string) []string {
	t.Helper()
	entradas, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("listar %s: %v", dir, err)
	}
	salida := make([]string, 0, len(entradas))
	for _, e := range entradas {
		salida = append(salida, e.Name())
	}
	return salida
}

func contains(lista []string, n string) bool {
	for _, e := range lista {
		if e == n {
			return true
		}
	}
	return false
}
