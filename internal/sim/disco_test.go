package sim

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"prdash/internal/forge/model"
)

// ENOSPC on the render directory is real —a small tmpfs swallows a big repo— and the warning must not
// talk about git.
func TestElDiscoSinEspacioSeAvisaComoLoQueEsYNoComoUnFalloDeGit(t *testing.T) {
	repo, _ := simRepoMonta(t)
	it := model.NewItem(model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget"}, 7)
	it.TargetBranch = "main"

	s := newService(t, locatorFalso{ok: true, place: Place{Repo: repo, Branch: "main-origin"}},
		fakeSim(t, writeJPEG(t)))

	// The mkdir is counted and inspected, because an injected Mkdir that is never called proves
	// nothing.
	llamadas := 0
	s.Mkdir = func(path string, perm fs.FileMode) error {
		llamadas++
		if filepath.Base(path) != "media" {
			t.Errorf("se creó %q, que no es el directorio de render", path)
		}
		if perm != 0o755 {
			t.Errorf("permisos %o, want 755", perm)
		}
		return &os.PathError{Op: "mkdir", Path: path, Err: syscall.ENOSPC}
	}

	res, err := s.Simulate(context.Background(), it, KindMerge)
	if err == nil {
		t.Fatalf("sin espacio dio nil y la imagen %q", res.Path)
	}
	if llamadas != 1 {
		t.Fatalf("se intentó crear el directorio %d veces, want 1", llamadas)
	}
	if res.Path != "" {
		t.Errorf("con el disco lleno devolvió la ruta %q: el popup ofrecería abrir una imagen "+
			"que no se generó", res.Path)
	}

	msg := err.Error()
	if !strings.Contains(msg, "prepare the render directory") {
		t.Errorf("el aviso %q no dice qué se estaba preparando", msg)
	}
	if !strings.Contains(msg, "no space left") {
		t.Errorf("el aviso %q no trae la causa de ENOSPC, que es la mitad que dice cómo "+
			"arreglarlo", msg)
	}
	// And it says nothing about git, which is the error it is most confused with.
	for _, deGit := range []string{"clonar", "check out", "branch --quiet"} {
		if strings.Contains(msg, deGit) {
			t.Errorf("el aviso %q habla de git (%q) y el fallo fue del disco", msg, deGit)
		}
	}

	limpio := &Service{Locator: locatorFalso{}}
	if limpio.mkdir() == nil {
		t.Error("mkdir() no devolvió una función con un servicio construido a mano")
	}
}

// The most important of the three: its consequence is not a warning but a `.part` the size of the
// JPEG that `prune` never deletes.
func TestUnTemporalQueNoSeCierraSeBorraYNoSePublicaNada(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "destino.jpg")
	contenido := []byte("un jpeg de mentira")

	roto := &escrituraFalsa{fallaAlCerrar: errDeCierre}
	err := copiaPublicando(bytes.NewReader(contenido), dst, roto.abreEn)
	if err == nil {
		t.Fatal("un temporal que no se cierra dio nil: se publicaría medio JPEG")
	}
	if !errors.Is(err, errDeCierre) {
		t.Errorf("el error es %v, want el del cierre. Un fallo de escritura daría el del "+
			"`write` y probaría el otro camino", err)
	}
	if !bytes.Equal(roto.escrito, contenido) {
		t.Errorf("se escribió %q, want %q", roto.escrito, contenido)
	}

	// And the temporary is gone. This is the assertion that matters: a `.part` weighs as much as the
	// JPEG it was going to become.
	if _, err := os.Stat(dst + ".part"); !os.IsNotExist(err) {
		t.Errorf("quedó el temporal %s: pesa lo que pesa la imagen y `prune` solo mira los "+
			"`.jpg`", dst+".part")
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Errorf("se publicó %s sin cerrar bien el temporal", dst)
	}

	bueno := &escrituraFalsa{fallaAlCerrar: nil}
	if err := copiaPublicando(bytes.NewReader([]byte("hola")), filepath.Join(dir, "bueno.jpg"),
		bueno.abreEn); err != nil {
		t.Fatalf("con un cierre que funciona: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "bueno.jpg.part")); !os.IsNotExist(err) {
		t.Error("quedó el temporal tras un renombrado bien hecho")
	}
	got, err := os.ReadFile(filepath.Join(dir, "bueno.jpg"))
	if err != nil || string(got) != "hola" {
		t.Errorf("lo publicado es %q (%v)", got, err)
	}

	origen := filepath.Join(dir, "origen.jpg")
	if err := os.WriteFile(origen, contenido, 0o644); err != nil {
		t.Fatal(err)
	}
	directo := filepath.Join(dir, "directo.jpg")
	if err := copyFile(origen, directo); err != nil {
		t.Fatalf("copyFile sin seam: %v", err)
	}
	if _, err := os.Stat(directo); err != nil {
		t.Errorf("copyFile no publicó: %v", err)
	}
}

var errDeCierre = errors.New("el sistema de ficheros no pudo cerrar el temporal")

// It writes a real file because an in-memory double would leave nothing to assert on.
type escrituraFalsa struct {
	fallaAlCerrar error

	escrito []byte
	fichero *os.File
}

func (e *escrituraFalsa) abreEn(ruta string) (escritura, error) {
	f, err := os.Create(ruta)
	if err != nil {
		return nil, err
	}
	e.fichero = f
	return e, nil
}

func (e *escrituraFalsa) Write(p []byte) (int, error) {
	e.escrito = append(e.escrito, p...)
	if e.fichero == nil {
		return len(p), nil
	}
	return e.fichero.Write(p)
}

func (e *escrituraFalsa) Close() error {
	if e.fichero != nil {
		_ = e.fichero.Close()
		e.fichero = nil
	}
	return e.fallaAlCerrar
}

var _ escritura = &escrituraFalsa{}

// The race is real but cannot be forced, so an already-gone entry is handed over instead.
func TestUnaImagenQueDesapareceAntesDelLstatNoRompeLaPoda(t *testing.T) {
	dir := t.TempDir()
	base := time.Now().Add(-24 * time.Hour)

	base0 := imagenFalsa(t, dir, "img-00.jpg", base)
	var survivors []string
	for i := 1; i <= keepImages; i++ {
		nombre := "img-" + dosCifras(i) + ".jpg"
		cuanto := time.Duration(i) * time.Hour
		imagenFalsa(t, dir, nombre, base.Add(cuanto))
		survivors = append(survivors, filepath.Join(dir, nombre))
	}

	listado := func(string) ([]os.DirEntry, error) {
		reales, err := os.ReadDir(dir)
		if err != nil {
			return nil, err
		}
		return append(reales,
			entradaMuerta{nombre: filepath.Base(base0), err: errors.New("lstat: no such file")},
		), nil
	}

	poda(dir, keepImages, listado)

	if _, err := os.Stat(base0); !os.IsNotExist(err) {
		t.Error("la imagen más antigua no se podó")
	}
	quedan, err := filepath.Glob(filepath.Join(dir, "img-*.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	if len(quedan) != keepImages {
		t.Errorf("quedan %d imágenes, want %d: una entrada ilegible no puede hacer que se poden "+
			"de más", len(quedan), keepImages)
	}
	for _, r := range quedan {
		if !slices.Contains(survivors, r) {
			t.Errorf("quedó %s, que no estaba en la lista de las que deberían", r)
		}
	}

	// Y el listado entero fallando.
	tranquilo := t.TempDir()
	imagenFalsa(t, tranquilo, "una.jpg", base)
	poda(tranquilo, 0, func(string) ([]os.DirEntry, error) {
		return nil, &os.PathError{Op: "readdir", Path: tranquilo, Err: fs.ErrPermission}
	})
	if _, err := os.Stat(filepath.Join(tranquilo, "una.jpg")); err != nil {
		t.Errorf("un listado ilegible vació el caché: %v", err)
	}

	real := t.TempDir()
	for i := 0; i <= keepImages; i++ {
		imagenFalsa(t, real, "img-"+dosCifras(i)+".jpg", base.Add(time.Duration(i)*time.Hour))
	}
	prune(real, keepImages)
	restantes, err := filepath.Glob(filepath.Join(real, "img-*.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	if len(restantes) != keepImages {
		t.Errorf("prune sin seam dejó %d imágenes, want %d", len(restantes), keepImages)
	}
}

type entradaMuerta struct {
	nombre string
	err    error
}

func (e entradaMuerta) Name() string               { return e.nombre }
func (e entradaMuerta) IsDir() bool                { return false }
func (e entradaMuerta) Type() fs.FileMode          { return 0 }
func (e entradaMuerta) Info() (fs.FileInfo, error) { return nil, e.err }

var _ os.DirEntry = entradaMuerta{}

func imagenFalsa(t *testing.T, dir, nombre string, cuando time.Time) string {
	t.Helper()
	ruta := filepath.Join(dir, nombre)
	if err := os.WriteFile(ruta, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(ruta, cuando, cuando); err != nil {
		t.Fatal(err)
	}
	return ruta
}

func dosCifras(i int) string {
	s := fmt.Sprintf("%04d", i)
	return s[len(s)-4:]
}

var _ io.Writer = &escrituraFalsa{}
