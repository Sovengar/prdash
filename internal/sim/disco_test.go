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

// Los tres fallos del sistema de ficheros, a través de los tres seams del servicio.
//
// Y los tres son el mismo tipo de problema con tres capas distintas:
//
//   - `Simulate` no puede crear el directorio donde git-sim escribe. La causa es el disco:
//     `ENOSPC`, `EDQUOT`, `EIO`. El aviso tiene que decir eso y no otra cosa, porque el arreglo
//     es "libera espacio" y no "mira el remoto".
//   - `copyFile` no puede cerrar el temporal. La causa es el sistema de ficheros reportando un
//     error que el `write` no vio, y la consecuencia concreta es que el temporal se queda a
//     medias en el caché de imágenes sin que nadie lo borre.
//   - `prune` no puede preguntar la fecha de una entrada. La causa es que el fichero desapareció
//     entre el listado y el `lstat`, y la consecuencia es que la imagen más antigua se queda ahí
//     para siempre porque el conteo bajó.
//
// Y los tres se provocan por el seam de su frontera, y no montando un sistema de ficheros
// defectuoso, porque eso no se puede montar sin privilegios. Lo que sí se comprueba en cada uno
// es el camino de verdad: el aviso, la limpieza, y que la operación no se dé por buena.

// TestElDiscoSinEspacioSeAvisaComoLoQueEsYNoComoUnFalloDeGit: el `MkdirAll` del directorio de
// render.
//
// Y el caso no es inventado. El directorio cuelga de un `os.MkdirTemp` que acaba de salir bien,
// así que su `MkdirAll` solo falla cuando el sistema de ficheros dice que no puede: `ENOSPC` en
// un tmpfs pequeño —y un `TMPDIR` en un tmpfs es lo que tienen muchos contenedores de CI—,
// `EDQUOT` en un volumen con cuota, `EIO` en un disco que se está muriendo. La causa importa: el
// arreglo de `ENOSPC` es liberar espacio y el de un fallo de git es mirar el remoto.
//
// Y `ENOSPC` es el que se usa porque es el más confundible. Los avisos de git empiezan por
// `clonar <url>` o por `check out <rama>` y siempre llevan una ruta de repo; el del disco
// empieza por `prepare the render directory` y lleva la ruta del temporal. Un usuario que los
// viera iguales iría a mirar el remoto de un repo que no tiene nada que ver.
//
// Y lo segundo que se comprueba es que la simulación NO continúa y que devuelve un `Result`
// vacío: con el `Path` puesto, el popup ofrecería abrir una imagen que no se generó.
func TestElDiscoSinEspacioSeAvisaComoLoQueEsYNoComoUnFalloDeGit(t *testing.T) {
	repo, _ := simRepoMonta(t)
	it := model.NewItem(model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget"}, 7)
	it.TargetBranch = "main"

	s := newService(t, locatorFalso{ok: true, place: Place{Repo: repo, Branch: "main-origin"}},
		fakeSim(t, writeJPEG(t)))

	// Y el `mkdir` se cuenta y se mira, porque un `Mkdir` inyectado que no se llama no probaría
	// nada: el error tiene que venir del punto correcto del camino, que es DESPUÉS de clonar y de
	// materializar las dos ramas.
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
	// Y no dice nada de git, que es el error con el que más se confunde.
	for _, deGit := range []string{"clonar", "check out", "branch --quiet"} {
		if strings.Contains(msg, deGit) {
			t.Errorf("el aviso %q habla de git (%q) y el fallo fue del disco", msg, deGit)
		}
	}

	// Y el `Mkdir` por defecto sigue siendo el de producción. Con el campo puesto, un servicio
	// que se constructa a mano y no lo rellena tiene que clonar igual, y un `Mkdir` inyectado
	// que se colara en `New` dejaría al servicio real sin poder escribir.
	limpio := &Service{Locator: locatorFalso{}}
	if limpio.mkdir() == nil {
		t.Error("mkdir() no devolvió una función con un servicio construido a mano")
	}
}

// TestUnTemporalQueNoSeCierraSeBorraYNoSePublicaNada: el `Close` de la copia.
//
// Y este es el más importante de los tres, porque su consecuencia no es un aviso: es un `.part`
// a medias en el directorio del caché de imágenes, con el peso del JPEG entero.
//
// Y por qué se quedaría ahí para siempre: `prune` solo mira los ficheros que acaban en `.jpg`,
// así que un `.part` no lo cuenta ni lo borra. La limpieza de los dos fallos anteriores —la
// copia y el cierre— es lo único que evita que se acumulen, y por eso el error del cierre tiene
// que comprobar que borra y no solo que se propaga.
//
// Y el fallo se provoca con el seam: un fichero que escribe bien y cuyo `Close` falla, que es
// exactamente el estado que no se puede montar. El kernel acepta todo lo que se le pasa a
// `write` porque va a la caché de páginas, así que el único error posible aquí es del `close`, y
// el `close` solo lo decide el sistema de ficheros.
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
	// Y lo escrito llegó al fichero de verdad, que es lo que prueba que el `Copy` pasó entero y
	// que el fallo es del cierre y no de la copia.
	if !bytes.Equal(roto.escrito, contenido) {
		t.Errorf("se escribió %q, want %q", roto.escrito, contenido)
	}

	// Y el temporal no quedó. Este es el aserto que importa: un `.part` pesa lo que pesa la
	// imagen y `prune` no lo ve.
	if _, err := os.Stat(dst + ".part"); !os.IsNotExist(err) {
		t.Errorf("quedó el temporal %s: pesa lo que pesa la imagen y `prune` solo mira los "+
			"`.jpg`", dst+".part")
	}
	// Y en el destino no se publicó nada, porque el renombrado es lo último y no llegó.
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Errorf("se publicó %s sin cerrar bien el temporal", dst)
	}

	// Y el camino bueno con el mismo seam, para que lo anterior no sea "nunca publica": con un
	// cierre que funciona, el `.part` se consume en el renombrado y el destino aparece con el
	// contenido entero.
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

	// Y `creaTemporal` es el `os.Create` de verdad: sin el seam, la copia tiene que funcionar.
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

// errDeCierre es el error del doble. Es un valor concreto y no un `errors.New` en cada sitio,
// para que `errors.Is` lo distinga del error de la escritura, que es otro camino.
var errDeCierre = errors.New("el sistema de ficheros no pudo cerrar el temporal")

// escrituraFalsa escribe en un fichero de verdad y falla al cerrarlo.
//
// Y escribe en un fichero de verdad porque un doble en memoria daría el error de cierre sin
// dejar nada en disco, y entonces el aserto de "el temporal no quedó" no comprobaría nada: no
// habría temporal.
type escrituraFalsa struct {
	fallaAlCerrar error

	escrito []byte
	fichero *os.File
}

// abreEn crea el fichero de verdad que hay detrás del doble.
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

// TestUnaImagenQueDesapareceAntesDelLstatNoRompeLaPoda: el `Info` de `prune`.
//
// Y la carrera es real pero no se puede forzar: `ReadDir` devuelve entradas y `Info` hace un
// `lstat`, y para que el segundo falle el fichero tiene que desaparecer entre los dos. Con
// veinte imágenes eso es una ventana de microsegundos por entrada. Así que el seam entrega una
// entrada que ya no está, que es el estado final de la carrera.
//
// Y lo que se comprueba NO es que el `continue` no reviente, que es lo que hace por
// construcción. Lo que se comprueba es que la poda sigue con el resto y que la imagen más
// antigua se borra igualmente. Y esto importa por una razón concreta: si la entrada ilegible NO
// se saltara, el conteo de la lista sería uno más y la poda borraría una imagen buena de más.
// Con el `continue` el conteo baja, la lista sigue siendo mayor que `keep` y la cola que se
// borra es la correcta.
//
// Y el error del listado entero —un caché que ya no existe, por ejemplo— va en el mismo test
// porque es el camino de al lado y se mide con el mismo seam. Un caché que no se puede leer se
// deja como está, que es lo contrario de vaciarlo a ciegas.
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

	// Y una entrada que ya no está: un `.jpg` más antiguo que la más antigua de las de verdad,
	// con la fecha que tendría, pero cuyo `lstat` falla.
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

	// Y `prune` sin seam, que es la producción: el caché de verdad se poda por el camino bueno.
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

// entradaMuerta es un `os.DirEntry` cuyo `Info` falla: el estado en el que queda una entrada de
// un `ReadDir` cuando el fichero desaparece antes del `lstat`.
type entradaMuerta struct {
	nombre string
	err    error
}

func (e entradaMuerta) Name() string               { return e.nombre }
func (e entradaMuerta) IsDir() bool                { return false }
func (e entradaMuerta) Type() fs.FileMode          { return 0 }
func (e entradaMuerta) Info() (fs.FileInfo, error) { return nil, e.err }

var _ os.DirEntry = entradaMuerta{}

// imagenFalsa escribe una imagen con la fecha que se le pide y devuelve su ruta.
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

// dosCifras rellena a la izquierda para que el orden de los nombres sea el mismo que el de las
// fechas. No es cosmetics: si `img-9` se ordenara antes que `img-10`, un fallo en la poda sería
// indistinguible de un fallo en la lista.
func dosCifras(i int) string {
	s := fmt.Sprintf("%04d", i)
	return s[len(s)-4:]
}

var _ io.Writer = &escrituraFalsa{}
