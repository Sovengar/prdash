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

// Estas funciones son las ÚLTIMAS de la cadena de una simulación, y son las únicas que no
// hablan con git ni con un proceso externo: hablan con el sistema de ficheros. Por eso este
// fichero no lleva dobles ni adaptadores falsos —usa el disco de verdad— y por eso los
// fallos que cubre son los que solo se pueden provocar con el disco de verdad.
//
// Y la razón por la que `copyFile` existe en vez de un `cp` o un `io.Copy` a pelo es el
// `.part`: escribe a un temporal y renombra. Un lector que mira el destino —el popup, un
// `xdg-open`— nunca ve medio fichero. Sin el temporal, un corte a mitad dejaba una imagen
// truncada con el nombre definitivo, y el popup la abría como si estuviera bien.
//
// Y los errores que se comprueban abajo son los cuatro que el SO de verdad produce, y que
// verificados antes de escribir el test: crear sobre un directorio da EISDIR, leer un
// directorio da EISDIR en el `read`, crear en un padre inexistente da ENOENT, y un fichero
// en modo 000 da EACCES. Ninguno se puede provocar con un doble de `os`.

// TestCopyFileATemporalDejaElDestinoIntegroYNoElTemporal: el motivo de la función.
//
// Y el caso que lo demuestra es el de un fallo en mitad de la copia: si el destino apareciera
// a medias, el popup ofrecería una imagen rota con el nombre bueno. Lo que se comprueba es
// que después del fallo NO hay destino y NO hay temporal —porque el temporal se limpia—,
// que es lo que deja el caché en el estado en el que estaba antes de intentarlo.
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
	// Y no quedó el temporal. Esto es lo que distingue "copió" de "copió y encima dejó
	// basura": un `.part` por cada simulación acumulando en el caché de ficheros que no
	// son imágenes y que nada borra: prune solo mira los .jpg.
	if _, err := os.Stat(dst + ".part"); !os.IsNotExist(err) {
		t.Errorf("quedó el temporal %s.part", dst)
	}

	// Y sobreescribir un destino que ya existe funciona: el caché reutiliza el nombre si dos
	// simulaciones del mismo ítem caen en el mismo nanosecondo, y una copia que fallara ahí
	// sería un fallo intermitente imposible de reproducir.
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

// TestCopyFileLimpiaElTemporalCuandoLaCopiaSeCorta: el fallo de verdad.
//
// Y aquí no hace falta simular un corte: el SO lo hace por nosotros. Copiar un DIRECTORIO
// como si fuera una imagen falla con EISDIR en el `read`, que es exactamente el mismo punto
// donde fallaría un disco lleno o una lectura interrumpida —después de haber creado el
// temporal—.
//
// Y lo que importa es que el temporal se borre. Sin esa limpieza, cada simulación fallida
// dejaría un `.part` con el tamaño de medio render en un directorio que `prune` no limpia,
// y tras veinte simulaciones fallidas el caché tendría algo que ningún comando sabe quitar.
func TestCopyFileLimpiaElTemporalCuandoLaCopiaSeCorta(t *testing.T) {
	dir := t.TempDir()
	// Un directorio en lugar de una imagen: `os.Open` lo acepta —abrir un directorio es
	// legal— y el fallo llega al leer, que es lo que `io.Copy` hace. Este es el detalle que
	// hace que "el origen no existe" no valiera como caso de fallo a medio copiar.
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
		// El error concreto depende de la plataforma; lo que no puede pasar es que el
		// mensaje no diga nada de por qué, así que se registra para verlo si cambia.
		t.Logf("el error del SO no menciona EISDIR: %v", err)
	}
	// Y ni destino ni temporal.
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Error("la copia fallida dejó el destino: el popup abriría una imagen rota")
	}
	if _, err := os.Stat(dst + ".part"); !os.IsNotExist(err) {
		t.Error("la copia fallida dejó el temporal: se acumularía en el caché sin que nada lo borre")
	}
}

// TestCopyFileNombraElFalloDeCadaPunto: los tres fallos de apertura, uno por uno.
//
// Y los tres son de apertura, no de copia, y cada uno deja el árbol como estaba:
//
//   - El origen no existe: el llamador es `keep`, que copia lo que git-sim acaba de escribir
//     en un temporal. Si ese temporal no está, el fallo es de git-sim y el mensaje tiene que
//     poder distinguirlo de un problema de permisos.
//   - El destino no se puede crear porque su padre no existe: el padre lo crea `keep` con
//     `MkdirAll` justo antes, así que solo pasa si alguien limpia el caché a mitad.
//   - El destino está en un directorio: un `.jpg` que alguien convirtió en carpeta. El
//     nombre del caché lleva forge y número, así que es raro —pero el error tiene que salir,
//     no tragarse la imagen.
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
		// El mensaje nombra el fichero implicado, que es lo que hace falta para depurar
		// sin reproducing el caso. `os` ya pone el path en su error, así que se comprueba
		// que no se lo come: el error de `keep` lo envuelve con "keep the simulation image",
		// y sin el path el mensaje sería "keep the simulation image: permission denied".
		base := c.dst
		if c.nombre == "origen que no existe" {
			base = c.src
		}
		if !strings.Contains(err.Error(), filepath.Base(base)) {
			t.Errorf("%s: el error %q no nombra el fichero implicante", c.nombre, err)
		}
		// Y ningún temporal: los tres fallos ocurren ANTES o en el primer write, pero la
		// garantía es la misma y no depende de en qué punto se caiga.
		if _, err := os.Stat(c.dst + ".part"); !os.IsNotExist(err) {
			t.Errorf("%s: dejó el temporal", c.nombre)
		}
	}
}

// TestPruneConservaLasMasRecientesYPorFechaNoPorNombre: el criterio de `prune`.
//
// Y "por fecha" es la parte que un test de tabla noaría, porque el nombre del fichero lleva
// un `UnixNano` y haría creer que el orden del nombre es el orden de creación. En un
// directorio real el orden de `ReadDir` es alfabético, así que un prune que ordenara por
// nombre vaciaría las imágenes nuevas cuando los nombres no coincidieran con el reloj —y en
// un reloj que atrasa, que pasa.
//
// Y las fechas se ponen a mano con `os.Chtimes` porque el reloj del test no tiene resolución
// para distinguirlas: dos `WriteFile` seguidos comparten milisegundo, y el prune los trataría
// como empate.
func TestPruneConservaLasMasRecientesYPorFechaNoPorNombre(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	// Los nombres van al revés de las fechas a propósito: el fichero "9" es el MÁS
	// RECIENTE y el "1" el más antiguo, porque su fecha es base + i días. Un prune que
	// ordenara por nombre conservaría 1, 2 y 3, que son justo los tres que aquí tienen que
	// desaparecer.
	//
	// La primera versión de este test decía al revés cuál era el más reciente y por eso
	// falló pidiendo 1,2,3. El nombre del fichero lleva un UnixNano, y eso hace creer que
	// el orden del nombre es el de creación —que es lo que hay que comprobar que NO se
	// cumpla—.
	for i := 9; i >= 1; i-- {
		nombre := itoa(i) + ".jpg"
		if err := os.WriteFile(filepath.Join(dir, nombre), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		// El fichero i tiene fecha base + i días: cuanto mayor i, más reciente.
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

// TestPruneIgnoraLoQueNoEsUnaImagenYNoBorraElCacheEntero: lo que `prune` no toca.
//
// Y son tres cosas que un prune ingenuo sí borraría, y las tres viven en el mismo directorio:
//
//   - Un subdirectorio: `prune` mira la extensión, y un directorio puede llamarse
//     `backup.jpg`. Sin el `e.IsDir()`, `os.Remove` lo borra — vacio o no.
//   - Un fichero sin extensión: el `HEAD` del git-sim o un `.lock` de otro proceso.
//   - Una imagen con la extensión pero en mayúsculas: `JPEG` no es `.jpg`, y esa diferencia
//     se decide con `HasSuffix`, que es sensible a mayúsculas. Es lo que hay, no un bug.
func TestPruneIgnoraLoQueNoEsUnaImagenYNoBorraElCacheEntero(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	tocables := []string{"a.jpg", "b.jpg", "c.jpg", "d.jpg"}
	for _, n := range tocables {
		p := filepath.Join(dir, n)
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		// Todos con la MISMA fecha, que es la que los mete en la cola de la poda.
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

	// Y lo que NO es intocable aunque lo parezca: un fichero llamado exactamente `.jpg` SÍ
	// lo poda, porque `prune` decide por sufijo y `HasSuffix(".jpg", ".jpg")` es cierto. Lo
	// tenía en la lista de intocables y por eso el recuento de abajo salía 0 de 4: el
	// superviviente era el `.jpg`, que va primero en el orden de lectura.
	//
	// No es un bug y no merece arreglo: `prune` no promete mirar el nombre del fichero,
	// promete mirar lo que termina en `.jpg`, y un fichero sin nombre no es una imagen que
	// el popup pueda abrir. Lo que sí conviene es que quede escrito, porque "no borra
	// ficheros ocultos" es la suposición razonable y es falsa.
	// Y con la fecha MÁS ANTIGUA a propósito, para que el desenlace no dependa de un empate:
	// con la misma fecha que las otras cuatro, el `.jpg` ganaba el empate por orden de
	// lectura —`ReadDir` ordena por nombre y el punto va antes que las letras— y era lo que
	// sobrevivía, con las cuatro imágenes de verdad eliminadas. Eso salía como "quedan 0 de
	// 4" y lo leí como un fallo de prune cuando era un empate del fixture.
	conNombreDeExtension := filepath.Join(dir, ".jpg")
	if err := os.WriteFile(conNombreDeExtension, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	masViejo := base.AddDate(0, 0, -1)
	if err := os.Chtimes(conNombreDeExtension, masViejo, masViejo); err != nil {
		t.Fatal(err)
	}

	// Poda agresiva: solo queda una imagen.
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

	// Y el subdirectorio sigue con su contenido: `os.Remove` sobre un directorio no
	// vacío falla, y sin comprobar el error `prune` habría intentado en bucle sin bajar.
	if _, err := os.Stat(filepath.Join(dir, "backup.jpg", "contenido")); err != nil {
		t.Errorf("prune vació el subdirectorio: %v", err)
	}
}

// TestPruneConMenosDeLosQueHayQueConservarNoHaceNadaYConCeroLosBorraTodos: los dos
// extremos del número a conservar.
//
// Y `keep <= 0` es el caso que hace que el aserto "conserva n" no se pueda usar sin más: con
// `keep=0` la condición `len(files) <= keep` es falsa y se borra todo. Eso es lo que hace que
// `keepImages` sea una constante y no un valor configurable desde el config —con un `keep=0`
// en el TOML, el popup no tendría nada que abrir y no habría aviso—.
func TestPruneConMenosDeLosQueHayQueConservarNoHaceNadaYConCeroLosBorraTodos(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"a.jpg", "b.jpg"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Más de los que hay: no toca nada, ni intenta.
	prune(dir, 10)
	if len(nombresDe(t, dir)) != 2 {
		t.Errorf("prune con keep=10 quitó ficheros: %v", nombresDe(t, dir))
	}
	// Y un `keep` NEGATIVO se recorta a cero en vez de reventar. Antes de recortarlo,
	// `files[-1:]` entraba en panic: `slice bounds out of range [-1:]`. Mi primera versión de
	// este test decía que no reventaba, y lo compruebo al revés porque lo leía de memoria
	// en vez de mirarlo.
	//
	// Hoy es inalcanzable —`keepImages` es una constante—, pero una función con un `int` en
	// la firma que peta con un valor legal de ese tipo es una bomba con la etiqueta puesta.
	prune(dir, -1)
	if quedan := len(nombresDe(t, dir)); quedan != 0 {
		t.Errorf("con keep=-1 quedan %d ficheros, want 0 (negativo se lee como cero)", quedan)
	}

	// Y cero sí borra todos.
	prune(dir, 0)
	if quedan := nombresDe(t, dir); len(quedan) != 0 {
		t.Errorf("prune con keep=0 dejó %v", quedan)
	}
}

// TestPruneSobreUnDirectorioQueNoExisteNoRevienta: la negativa de `prune`.
//
// Y es una negativa y no un fallo a propósito: `prune` se llama desde `keep` DESPUÉS de un
// `MkdirAll` que ya ha tenido éxito, así que el directorio existe. El `return` temprano es la
// degradación honesta del proyecto aplicada a una función que no puede devolver error: si
// alguien limpia el caché entre el `MkdirAll` y el `prune`, lo que se pierde es la poda —que
// es una optimización— y no la imagen, que ya está copiada y devuelta.
func TestPruneSobreUnDirectorioQueNoExisteNoRevienta(t *testing.T) {
	// No hace falta esperar: lo que se comprueba es que no hay panic y que no crea nada.
	prune(filepath.Join(t.TempDir(), "no-existe"), 3)

	// Y sobre algo que no es un directorio.
	fichero := filepath.Join(t.TempDir(), "soy-un-fichero")
	if err := os.WriteFile(fichero, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	prune(fichero, 3)
	if _, err := os.Stat(fichero); err != nil {
		t.Errorf("prune se llevó un fichero que no era directorio: %v", err)
	}
}

// TestKeepCopiaAlCacheConElNombreDelItemYPodaLoQueSobra: `keep` entero, contra el disco.
//
// Y es la prueba de integración de la cola: un JPEG real escrito donde git-sim lo dejaría, un
// ítem real, el caché real, y el nombre que el popup usa para abrir. El nombre es lo que
// importa del contrato —forge, proyecto legible, número— porque es lo que reconoce `prune`
// para no borrarlo y lo que el usuario ve.
//
// Y la segunda mitad del test es la que hace que la poda no sea decorativa: se meten más de
// `keepImages` imágenes en el caché y se comprueba que `keep` deja exactamente ese número.
// `keepImages` vale 20, así que hacen falta 21 ficheros reales —y son 21 KB, no 21 renders.
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
	// Y la copia es el mismo JPEG, byte a byte. No un re-codificado: el popup la abre con un
	// visor y no con prdash, así que cualquier transformación sería una sorpresa.
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
	// Y el nombre lleva lo que hace falta para reconocerla: forge, proyecto sin barras ni
	// signos raros, número.
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

	// Y la poda: 21 imágenes más la que acaba de entrar, y quedan 20.
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
	// Y lo que NO se puede borrar es la imagen que se acaba de guardar.
	//
	// Y aquí está el fallo de mi primera versión, que era intermitente y por eso peor: miraba
	// `dst`, la imagen de la PRIMERA llamada a keep, que es la más antigua del caché para
	// cuando entra la poda. Sobrevivía por casualidad — con veintitrés ficheros escritos en el
	// mismo tictac, el orden alfabético ponía su nombre antes que los de la basura y el
	// desempate la salvaba—, y en cuanto el disco dio dos milisegundos distintos, la poda la
	// borró y el test falló.
	//
	// Es el mismo aserto-que-pasa-por-suerte de siempre: una afirmación que depende de un
	// empate del fixture es una que falla cuando el fixture cambia y no cuando el código.
	if _, err := os.Stat(ultima); err != nil {
		t.Errorf("la poda borró la imagen que se acaba de guardar: %v", err)
	}
	if ultima == dst {
		t.Error("las dos llamadas a keep dieron el mismo nombre: el UnixNano del nombre se " +
			"repite y una sobrescribiría a la otra")
	}
	// Y quedan exactamente los que hay que conservar.
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

// itemDePrueba es el ítem que se pasa a `keep`: un PR de GitHub en acme/widget.
func itemDePrueba() model.Item {
	return model.NewItem(model.RepoRef{
		Forge: "github", Host: "github.com", Project: "acme/widget",
	}, 7)
}

// nombresDe lista los ficheros de un directorio, para no repetir el os.ReadDir.
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

// contains informa si un texto está en una lista, sin arrastrar el slices de "go-cmp".
func contains(lista []string, n string) bool {
	for _, e := range lista {
		if e == n {
			return true
		}
	}
	return false
}
