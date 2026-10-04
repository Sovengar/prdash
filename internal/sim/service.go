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

// Place es dónde están los refs de un ítem: el clon local que los trajo y la
// rama local del review. No incluye el worktree del review porque la simulación no
// lo toca nunca —trabaja en un clon temporal—, y por tanto tampoco debe parecer
// que puede.
type Place struct {
	// Repo es el clon local (normal o bare) que tiene los refs.
	Repo string
	// Branch es la rama local del review.
	Branch string
}

// Locator localiza los refs de un ítem ya montado. Es un puerto y no una
// dependencia directa porque quien sabe de repos es el orquestador del review:
// sim solo necesita que le digan dónde está cada cosa.
type Locator interface {
	Locate(it model.Item) (Place, bool)
}

// Result es una simulación terminada.
type Result struct {
	// Kind es el comando que se ejecutó.
	Kind Kind
	// Path es la imagen conservada en el caché; se puede abrir con un visor.
	Path string
	// Ref es el ref contra el que se ejecutó.
	Ref string
	// Base es la rama base tal y como la publicó el forge.
	Base string
}

// keepImages es cuántas simulaciones se conservan por caché. Cada imagen son
// unas decenas de kilobytes, pero el caché no se limpia solo y un popup que se
// puede abrir con el visor hace que vale la pena conservarlas.
const keepImages = 20

// Service orquesta una simulación: localiza los refs, prepara el directorio de
// trabajo que el comando necesita, ejecuta git-sim y conserva la imagen.
type Service struct {
	// Locator dice dónde están los refs del ítem.
	Locator Locator
	// Runner ejecuta git-sim; nil usa uno por defecto.
	Runner *Runner
	// CacheDir es donde se conservan las imágenes; vacío usa DefaultCacheDir.
	CacheDir string
	// Mkdir crea los directorios que git-sim necesita para escribir; nil usa os.MkdirAll.
	//
	// Y es el tercer campo inyectado, y el último: los tres existen por la misma razón —la
	// mitad interesante de un servicio de orquestación son los fallos del entorno, y los
	// fallos del entorno no se pueden provocar en un banco de pruebas—, y los tres se
	// limitan a UNA operación cada uno.
	//
	// Y este es el caso más claro de los tres. El directorio donde git-sim escribe cuelga de
	// un `os.MkdirTemp` que acaba de salir bien, así que su `MkdirAll` solo falla cuando el
	// sistema de ficheros dice que no: `ENOSPC`, `EDQUOT`, `EIO`. Los tres son reales —un
	// `TMPDIR` en un tmpfs de 16 MB se los come con un repo grande— y los tres son imposibles
	// de montar sin privilégios. Con el campo inyectado, el test comprueba lo que de verdad
	// importa de ese error: que el aviso diga que se.prepare el render y que no lo diga como si
	// fuera un fallo de git, porque el arreglo de uno es el disco y el del otro es el remoto.
	Mkdir func(path string, perm fs.FileMode) error
	// Git ejecuta los comandos de git del staging; nil usa uno por defecto.
	//
	// Y es un campo más por la misma razón que `Runner`: un runner de git-sim que se
	// comporta bien no es la mitad interesante. La otra mitad son los fallos de git —un
	// clon con un `index.lock` de otra proceso, un ref que no se puede materializar, un
	// checkout de una rama que se borró entremedias—, y sin este campo no había forma
	// de probarlos: `Runner.Bin` permite apuntar a otro binario, así que un test levanta
	// un `git` de mentira que pasa todo menos el subcomando que quiere fallar.
	//
	// Y es la diferencia entre un fallo que se avisa y uno que se traga. Los tres errores
	// de `stage` y `materialize` son `fmt.Errorf` con el motivo dentro, que es lo que ve
	// el usuario en el popup del popup de simulación; sin poder provocarlos, nadie sabía
	// si esos `wrap` dicen algo o si son decoración.
	Git *gitcmd.Runner
}

// New construye el servicio con un runner por defecto.
func New(locator Locator) *Service {
	return &Service{Locator: locator, Runner: NewRunner(), Git: gitcmd.New()}
}

// mkdir es el `MkdirAll` efectivo.
func (s *Service) mkdir() func(string, fs.FileMode) error {
	if s.Mkdir == nil {
		s.Mkdir = os.MkdirAll
	}
	return s.Mkdir
}

// gitRunner es el runner de git efectivo.
func (s *Service) gitRunner() *gitcmd.Runner {
	if s.Git == nil {
		s.Git = gitcmd.New()
	}
	return s.Git
}

// DefaultCacheDir es el directorio donde se guardan las imágenes:
// $XDG_CACHE_HOME/prdash/sim.
func DefaultCacheDir() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "prdash", "sim"), nil
}

// Available informa si git-sim se puede ejecutar.
func (s *Service) Available() bool { return s.runner().Available() }

// runner es el runner efectivo.
func (s *Service) runner() *Runner {
	if s.Runner == nil {
		s.Runner = NewRunner()
	}
	return s.Runner
}

// cacheDir es el directorio de imágenes efectivo.
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

// Simulate renderiza la simulación de un ítem y conserva la imagen.
//
// Todo ocurre en un clon temporal de los refs del review, nunca en el clon ni en
// el worktree del usuario. No es puritanismo: git-sim necesita un HEAD que apunte
// a una rama de verdad, y la única forma de tener la base activa sin tocar el
// worktree del review —que está en la rama del ítem y es el cwd de la review— es
// tener otro directorio. El clon se hace con --shared para no copiar objetos y se
// borra al terminar, así que no deja refs, worktrees ni ramas en el repo de
// origen: si el proceso muere, lo único que queda es un directorio temporal que
// el sistema limpia solo.
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

// stage prepara el clon temporal y devuelve el directorio donde correr y el ref
// contra el que correr.
//
// Se clona con --shared (los objetos se referencian, no se copian) y luego se
// materializan las dos ramas como ramas locales del clon. Hace falta hacerlo
// porque `git clone` solo trae en local la rama de HEAD: las demás quedan como
// refs de origin/, y git-sim las acepta como argumento pero no las dibuja como
// una rama del grafo, que es justo lo que se viene a ver.
func (s *Service) stage(ctx context.Context, place Place, kind Kind, base, tmp string) (string, Spec, error) {
	// Se comprueba antes de clonar: sin la rama del ítem no hay grafo que dibujar,
	// y clonar para descubrirlo después sería trabajo tirado.
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

	// La rama que queda activa es la que git-sim toma como punto de partida: la
	// base para integrar, la del ítem para rebasar.
	active, ref := base, place.Branch
	if kind == KindRebase {
		active, ref = place.Branch, base
	}
	if _, err := s.gitRunner().Run(ctx, path, "checkout", "--quiet", active); err != nil {
		return "", Spec{}, fmt.Errorf("check out %s in the simulation clone: %w", active, err)
	}
	return path, Spec{Kind: kind, Ref: ref}, nil
}

// materialize se asegura de que exista una rama local con ese nombre, creándola
// desde el ref que exista. El orden importa: la rama local manda sobre el ref
// remoto, porque un clon de trabajo puede tener una base local que vaya por
// detrás de la del remoto y comparar contra la equivocada daría un grafo que no
// es el de nadie.
func (s *Service) materialize(ctx context.Context, dir, name string) error {
	// Si la rama ya está en local se deja como está. Es el caso normal —el clon
	// trae la rama de la que se clonó, y el repo de origen ya tiene la base— y
	// además es lo correcto: la base local es la contra la que se comparó la
	// review, así que Frontierla daría un grafo que no es el de nadie. Por eso no
	// se fuerza ni se resetea.
	if _, err := s.gitRunner().Run(ctx, dir, "rev-parse", "--verify", "--quiet", name+"^{commit}"); err == nil {
		return nil
	}
	// Si no, se crea desde el ref remoto. `git clone` solo trae en local la rama de
	// HEAD, así que esto es lo que suele pasar con la rama del ítem.
	if _, err := s.gitRunner().Run(ctx, dir, "rev-parse", "--verify", "--quiet", "origin/"+name+"^{commit}"); err != nil {
		return fmt.Errorf("the branch %s does not exist in the local clone", name)
	}
	if _, err := s.gitRunner().Run(ctx, dir, "branch", "--quiet", name, "origin/"+name); err != nil {
		return fmt.Errorf("create the branch %s in the simulation clone: %w", name, err)
	}
	return nil
}

// keep copia la imagen generada al caché y poda lo que sobra. La copia es
// necesaria porque la original vive en un directorio temporal que se borra al
// terminar el render, y el popup ofrece abrirla en un visor.
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

// escritura es lo que `copyFile` necesita del fichero de destino: escribir y cerrar.
//
// Y es una interfaz porque el error de `Close` es el ÚNICO error de la copia que no es de
// disco, y no hay forma de provocarlo de otra manera. Todo lo que se escribe pasa por la caché
// de páginas del kernel, así que `io.Copy` termina sin error aunque el disco se llame después:
// el fallo, si lo hay, lo devuelve el `close()`, y eso lo decide el sistema de ficheros. Ni un
// `/dev/full` lo hace —ese falla en `write`, que es un camino distinto—, ni un fichero en NFS, ni
// nada que se pueda montar en un banco de pruebas.
//
// Y con la interfaz el test entrega un fichero cuyo `Close` falla, que es exactamente el
// estado que hay que comprobar: que el temporal se borra y que el error se propaga sin haber
// publicado un `.part` a medias.
type escritura interface {
	io.Writer
	io.Closer
}

// copyFile copia de origen a destino de forma atómica-ish: primero a un temporal
// y luego renombrado, para que un lector simultáneo no encuentre medio JPEG.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	return copiaPublicando(in, dst, creaTemporal)
}

// creaTemporal es el `os.Create` de producción, con la firma de `escritura` porque
// `*os.File` la cumple.
func creaTemporal(ruta string) (escritura, error) { return os.Create(ruta) }

// copiaPublicando copia `in` a un temporal junto a `dst` y lo publica con un renombrado.
//
// Y el `crea` va inyectado, y solo el `crea`, porque es la única parte de esta función que
// depende del sistema de ficheros de una forma que el banco de pruebas no puede imitar. El
// renombrado y el borrado se dejan con `os`, y se pueden probar de verdad: un `dst` que ya es
// un directorio con contenido hace fallar el `rename` sin tocar una sola línea de este código.
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
	// El renombrado también limpia. Los dos fallos anteriores ya lo hacen y este no, y la
	// asimetría se nota: el temporal es un `.part` en el directorio del caché, y `prune` solo
	// mira los `.jpg`, así que un `.part` que se queda ahí no lo borra nadie. El caso que lo
	// provoca es `dst` siendo un directorio que ya existe —`crea(tmp)` tiene éxito, porque
	// `tmp` es otro nombre, y el rename falla con EEXIST—.
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// prune deja solo las n imágenes más recientes por fecha de modificación.
//
// Y el listado va inyectado por el mismo motivo que el `crea` de la copia, y con el mismo
// alcance mínimo: `DirEntry.Info` es un `lstat`, y solo falla si el fichero desaparece entre el
// `ReadDir` y el `lstat`. Es una carrera, y las carreras no se fuerzan —la ventana es de
// microsegundos por entrada y la probabilidad por ejecución es bajísima—, así que lo que se
// hace es entregar una entrada que ya no está.
//
// Y lo que se comprueba con ella no es el `continue` en sí, que es lo obvio, sino que la poda
// sigue con el resto: una entrada ilegible no puede hacer que la imagen más antigua se quede
// ahí para siempre porque el conteo bajó.
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
	// Ordena de más reciente a más antigua y borra la cola.
	for i := 1; i < len(files); i++ {
		for j := i; j > 0 && files[j].mod.After(files[j-1].mod); j-- {
			files[j], files[j-1] = files[j-1], files[j]
		}
	}
	for _, f := range files[keep:] {
		_ = os.Remove(f.path)
	}
}

// slug reduce una ruta de proyecto a algo legible en un nombre de fichero.
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
