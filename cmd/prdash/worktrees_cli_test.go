package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"prdash/internal/worktree"
)

// Este fichero existe porque `listWorktrees` y `removeWorktreesWithin` escribían a
// `os.Stdout` y `os.Stderr` fijos, y sin un `io.Writer` por el camino la única forma de
// comprobar qué imprimen era redirigir los descriptores del proceso entero. Eso obliga a un
// test por proceso y, sobre todo, deja sin probar justo la salida del comando que **borra
// ficheros del usuario** —que es donde lo que imprime es la prueba de que borró lo que dijo y
// nada más—.
//
// Y todo aquí va contra worktrees de git de verdad, montados con `git worktree add` en
// repos de verdad. No hay dobles: el contrato de este comando es con git, y un doble
// devolvería la entrada que el test espera en vez de la que git da.

// TestListarSeparaElResultadoDelAvisoYCadaCosaEnSuCanal: la asimetría de los dos
// writers.
//
// Y es la primera razón por la que hay dos: la tabla va a stdout y el aviso de huérfano a
// stderr, porque son cosas distintas. La tabla es el resultado que un script lee; el aviso es
// la explicación de por qué una fila está marcada como `orphaned`. Un `2>/dev/null` sobre el
// listado se lleva la explicación y deja la tabla, que es lo que se quiere. Al revés se
// pierde el resultado y se queda el ruido, y el comando en un script se ve como que falló.
//
// Y el caso que lo demuestra es un huérfano de verdad —un directorio `prdash-*` sin repo
// detrás—, que es la única forma de que la columna ESTADO diga algo que no sea `ok`.
func TestListarSeparaElResultadoDelAvisoYCadaCosaEnSuCanal(t *testing.T) {
	base, orphans, healthy, foreign := worktreeOrphanFixture(t)
	pr := worktree.NewGitDirect(base)

	var stdout, stderr bytes.Buffer
	if code := listWorktrees(pr, &stdout, &stderr); code != 0 {
		t.Fatalf("código = %d, stderr = %q", code, stderr.String())
	}

	tabla := stdout.String()
	// En stdout: la tabla con las TRES entradas propias y la nada más.
	for _, quiere := range []string{"WORKTREE", "RAMA", "ESTADO", "RUTA"} {
		if !strings.Contains(tabla, quiere) {
			t.Errorf("la tabla no tiene la columna %q:\n%s", quiere, tabla)
		}
	}
	for _, e := range append(append([]string{}, orphans...), healthy) {
		if !strings.Contains(tabla, e) {
			t.Errorf("la tabla no lista %s:\n%s", e, tabla)
		}
	}
	// Y el ajeno no sale, ni en stdout ni en stderr: prdash nunca lo menciona.
	if strings.Contains(tabla, foreign) {
		t.Errorf("la tabla menciona el worktree ajeno %s:\n%s", foreign, tabla)
	}
	if strings.Contains(stderr.String(), foreign) {
		t.Errorf("stderr menciona el worktree ajeno %s", foreign)
	}

	// En stderr: solo el aviso de huérfano, y con el motivo.
	avisos := stderr.String()
	for _, o := range orphans {
		if !strings.Contains(avisos, o) {
			t.Errorf("stderr no avisa del huérfano %s:\n%s", o, avisos)
		}
		if !strings.Contains(avisos, "orphaned") && !strings.Contains(avisos, "is orphaned") {
			t.Errorf("el aviso de %s no dice que está huérfano:\n%s", o, avisos)
		}
	}
	// Y el sano NO se avisa: avisar de un worktree que está bien sería ruido que enseña a
	// ignorar el canal entero.
	if strings.Contains(avisos, healthy) {
		t.Errorf("stderr avisa del worktree sano %s:\n%s", healthy, avisos)
	}
}

// TestListarAlineaLasColumnasParaQueLaTablaSeLea: el tabwriter, que es lo único que hay
// entre la salida y algo que un ojo pueda leer.
//
// Y no es cosmético: el listado se lee a ojo para responder "¿qué reviews tengo abiertos?",
// y sin alineado el segundo worktree desplazaría todas las columnas y comparar dos filas
// requeriría contar espacios. Es el mismo motivo por el que existe `text/tabwriter` en lugar de
// un `strings.Join` con `\t`.
//
// Y la comprobación es de estructura y no de una cadena exacta: las anchuras dependen de las
// longitudes de los labels, que dependen de las rutas del temporal. Lo que tiene que cumplirse
// es que cada columna empiece en la MISMA posición en todas las filas.
func TestListarAlineaLasColumnasParaQueLaTablaSeLea(t *testing.T) {
	base, _, _ := worktreeFixture(t)
	pr := worktree.NewGitDirect(base)

	var stdout, stderr bytes.Buffer
	listWorktrees(pr, &stdout, &stderr)

	// Filas = cabecera + las que haya.
	lineas := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
	if len(lineas) < 2 {
		t.Fatalf("salida con %d líneas, no hay tabla que alinear:\n%s", len(lineas), stdout.String())
	}

	// La posición de cada columna, sacada de la cabecera por los separadores de dos espacios
	// que deja el tabwriter.
	posicionesCabecera := columnasDe(lineas[0])
	if len(posicionesCabecera) != 4 {
		t.Fatalf("la cabecera tiene %d columnas, want 4: %q", len(posicionesCabecera), lineas[0])
	}
	for _, fila := range lineas[1:] {
		for i, col := range columnasDe(fila) {
			if i >= len(posicionesCabecera) {
				t.Errorf("la fila %q tiene más columnas que la cabecera", fila)
				break
			}
			if col != posicionesCabecera[i] {
				t.Errorf("la columna %d de %q empieza en %d y la de la cabecera en %d: la "+
					"tabla no está alineada", i, fila, col, posicionesCabecera[i])
			}
		}
	}
}

// columnasDe devuelve en qué posición de byte empieza cada columna de una fila del listado.
//
// Y el tabwriter rellena con espacios, así que la columna siguiente empieza justo después de
// la ÚLTIMA espacio de la racha, no en la primera. Mi primera versión devolvía el inicio de
// cada racha, y como la primera columna no tiene racha delante contaba una menos: la cabecera
// con cuatro columnas salía con tres y el test de alineación no comparaba nada.
//
// Y la primera columna es explícitamente el 0, que es donde empieza la fila.
func columnasDe(fila string) []int {
	out := []int{0}
	i := 0
	for i < len(fila) {
		if fila[i] != ' ' {
			i++
			continue
		}
		inicio := i
		for i < len(fila) && fila[i] == ' ' {
			i++
		}
		// Una racha de espacios que llega al final de la línea no abre columna: es el relleno
		// del último campo, y contarlo daría una columna que no existe en la fila siguiente.
		if i >= len(fila) {
			break
		}
		out = append(out, inicio+(i-inicio))
	}
	return out
}

// TestSinWorktreesElListadoDiceQueNoHayYNoEsUnError: el caso vacío.
//
// Y es un caso con conteúdo, no un `return` silencioso. `prdash worktrees list` en un repo
// donde nunca se montó una review es el caso MÁS FRECUENTE del comando, y una salida vacía se
// lee como un fallo o como un bug. Dice lo que pasa y sale con 0.
//
// Y stderr vacío es parte del contrato: si el "no hay worktrees" fuera a stderr, un `2>/dev/null`
// —que es lo que se pone para leer una tabla— lo quitaría y el usuario vería una pantalla en
// blanco sin explicación.
func TestSinWorktreesElListadoDiceQueNoHayYNoEsUnError(t *testing.T) {
	pr := worktree.NewGitDirect(t.TempDir())

	var stdout, stderr bytes.Buffer
	if code := listWorktrees(pr, &stdout, &stderr); code != 0 {
		t.Errorf("código = %d sin worktrees, want 0", code)
	}
	if !strings.Contains(stdout.String(), "no prdash review worktrees") {
		t.Errorf("no dice que no hay worktrees: %q", stdout.String())
	}
	if stderr.String() != "" {
		t.Errorf("stderr = %q sin worktrees, want vacío", stderr.String())
	}
}

// TestBorrarEnSecoDiceQueIriaABorrarYTocanNada: `--dry-run` no es un `--orphans` que
// además avisa.
//
// Y la propiedad que hay que fijar es la de no tocar nada, y se comprueba sobre el árbol de
// ficheros y no sobre la salida: un `--dry-run` que borrara uno mismo seguiría imprimiendo
// "would remove" si el borrado viniera después. Es el mismo bug que se cuela cuando el
// `dryRun` decide el mensaje pero el bucle de borrado no lo consulta.
//
// Y el segundo caso es el que importa más: con huérfanos que se van a borrar, el texto es
// "would remove" y NO "worktree removed". Una prueba seca que imprimiera lo mismo que una
// real dejaría al usuario con la duda de si ocurrió, y en un comando que borra directorios esa
// duda es cara.
func TestBorrarEnSecoDiceQueIriaABorrarYTocanNada(t *testing.T) {
	base, orphans, healthy, foreign := worktreeOrphanFixture(t)
	pr := worktree.NewGitDirect(base)

	var stdout, stderr bytes.Buffer
	code := removeWorkphans(pr, &stdout, &stderr, true)
	if code != 0 {
		t.Fatalf("código = %d, stderr = %q", code, stderr.String())
	}
	out := stdout.String()
	for _, o := range orphans {
		if !strings.Contains(out, "would remove: "+o) {
			t.Errorf("no dice que borraría %s:\n%s", o, out)
		}
		if strings.Contains(out, "worktree removed: "+o) {
			t.Errorf("una prueba seca dice que lo BORRÓ: %s", o)
		}
		// Y lo que es lo importante: sigue ahí.
		if !worktree.Exists(o) {
			t.Errorf("--dry-run borró %s", o)
		}
	}
	// Y nada más se movió.
	if !worktree.Exists(healthy) || !worktree.Exists(foreign) {
		t.Error("--dry-run tocó el worktree sano o el ajeno")
	}
}

// TestSinHuérfanosElBorradoEnLoteDiceQueNoHayYNoEscribeEnStderr: el caso feliz del lote.
//
// Y lo de "no escribe en stderr" es el aserto que hace el test: un lote vacío es el resultado
// que un script comprueba para decidir que todo está limpio, y si escribiera un aviso en
// stderr —del tipo "nada que hacer"— un `set -e` con stderr en la salida parecería un fallo.
func TestSinHuérfanosElBorradoEnLoteDiceQueNoHayYNoEscribeEnStderr(t *testing.T) {
	base, _, _ := worktreeFixture(t) // sano + ajeno, sin huérfanos
	pr := worktree.NewGitDirect(base)

	for _, dryRun := range []bool{false, true} {
		var stdout, stderr bytes.Buffer
		if code := removeOrphans(pr, &stdout, &stderr, dryRun, worktreeTimeout); code != 0 {
			t.Errorf("dryRun=%v: código = %d, cero huérfanos es el caso feliz", dryRun, code)
		}
		if !strings.Contains(stdout.String(), "no orphaned prdash worktrees") {
			t.Errorf("dryRun=%v: no dice que no hay huérfanos: %q", dryRun, stdout.String())
		}
		if stderr.String() != "" {
			t.Errorf("dryRun=%v: stderr = %q con cero huérfanos, want vacío",
				dryRun, stderr.String())
		}
	}
}

// TestRutasRechazadasYTocadasVanAStderrYElCodigoDeSaltaPeroElRestoSeBorra: el lote
// parcial.
//
// Y este es el caso más importante del comando, porque es el que decide si un error borra lo
// que no debía. La respuesta es no: cada ruta se comprueba y se rechaza SOLO, y el código de
// salida sale a 1 para que un script se entere, pero el resto del lote se procesa igual.
//
// Y lo que se comprueba es lo de las dos mitades, porque una sola no basta:
//
//   - La ruta rechazada sigue ahí. Es lo que impide que `remove` sea un `rm` con pasos.
//   - La ruta buena sí se borró. Es lo que impide que un rechazo tumbe el lote entero.
//
// Y el motivo de que el fallo no sea fatal es la misma razón por la que la lista se imprime
// entera: un usuario que pasa diez rutas y una está mal escrita quiere las otras nueve
// borradas, y volver a intentarlo todo porque una falla obliga a recordar cuáles eran.
func TestRutasRechazadasYTocadasVanAStderrYElCodigoDeSaltaPeroElRestoSeBorra(t *testing.T) {
	base, orphans, healthy, foreign := worktreeOrphanFixture(t)
	pr := worktree.NewGitDirect(base)
	// Un trabajo del usuario que NO es de prdash pero está bajo la raíz y con el nombre
	// del prefijo: el caso que un `rm -rf` se llevaría por delante. Es un directorio suelto
	// con un fichero dentro, sin `.git`, que es como se ve algo que el usuario tiene en su
	// home y que se parece a un review.
	ajeno := filepath.Join(base, "prdash-mio-del-usuario")
	if err := os.MkdirAll(ajeno, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ajeno, "trabajo.txt"),
		[]byte("no me borres"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := removeWorktreesWithin(pr, &stdout, &stderr, false, false,
		[]string{orphans[0], foreign, "/no/existe/en/absoluto", orphans[1]}, 10*time.Second)

	// El código de salida avisa de que algo no se borró.
	if code != 1 {
		t.Errorf("código = %d con dos rutas rechazadas, want 1", code)
	}
	// Los rechazos van a stderr, uno por línea, cada uno nombrando lo que se dejó intacto.
	errOut := stderr.String()
	for _, rechazada := range []string{foreign, "/no/existe/en/absoluto"} {
		if !strings.Contains(errOut, rechazada) {
			t.Errorf("stderr no nombra la ruta rechazada %s:\n%s", rechazada, errOut)
		}
		if !strings.Contains(errOut, "leaving it alone") {
			t.Errorf("el rechazo de %s no dice que se la deja intacta:\n%s", rechazada, errOut)
		}
	}
	// Y lo que se borró va a stdout, que es el resultado que se consulta después.
	out := stdout.String()
	for _, o := range orphans {
		if !strings.Contains(out, "worktree removed: "+o) {
			t.Errorf("stdout no dice que se borró %s:\n%s", o, out)
		}
		if worktree.Exists(o) {
			t.Errorf("%s sigue ahí después de borrarlo", o)
		}
	}
	// Lo ajeno a prdash sigue intacto. Y el nombre "parecido a prdash" es el caso que
	// `Owned` tiene que reconhecer por la etiqueta y no por el nombre del directorio.
	//
	// Y `Exists` en true significa que SIGUE ahí, que es lo que se comprueba aquí. Mi
	// primera versión afirmaba lo contrario —`if Exists entonces "se borró"`— y fallaba
	// marcando como borrados justo los dos worktrees que el comando deja intactos. Es el
	// mismo error de lectura que ya me costó un test en `worktree`: un nombre de función que
	// devuelve true cuando la cosa está bien invites a escribir la rama al revés.
	for _, intacto := range []string{foreign, healthy} {
		if !worktree.Exists(intacto) {
			t.Errorf("%s se borró, y no era suyo", intacto)
		}
	}
	if _, err := os.Stat(filepath.Join(ajeno, "trabajo.txt")); err != nil {
		t.Errorf("borró el trabajo del usuario que solo se parecia a un worktree: %v", err)
	}
}

// TestElDespachoEscribeCadaErrorEnSuCanal: el mapa completo subcomando → destino → código.
//
// Y es un test de tabla sobre `runWorktrees` entero, y lo que compra es que un error de uso
// NUNCA acabe en stdout. Si lo hiciera, `prdash worktrees remove --bogus > log` dejaría el
// mensaje de error en `log` y el script creería que la operación salió bien.
//
// Y el código de salida es el otro eje: 2 para uso incorrecto —que es lo que la convención de
// Unix llama error de uso y lo que un wrapper distingue de un fallo de la operación— y 0 o 1
// para lo queDepends de los worktrees.
func TestElDespachoEscribeCadaErrorEnSuCanal(t *testing.T) {
	base, _, healthy, foreign := worktreeOrphanFixture(t)
	pr := worktree.NewGitDirect(base)

	for _, c := range []struct {
		nombre  string
		args    []string
		want    int
		aStderr string
		aStdout string
	}{
		{"subcomando desconocido", []string{"inventado"}, 2, "unknown subcommand", ""},
		{"list sin provisioner de nada", []string{"list"}, 0, "", "WORKTREE"},
		{"remove sin rutas", []string{"remove"}, 2, "missing at least one path", ""},
		{"flag desconocido", []string{"remove", "--inventado"}, 2, "unknown flag", ""},
		{"typo de orphans", []string{"remove", "--orphan"}, 2, "unknown flag", ""},
		{"dry-run sin orphans", []string{"remove", "--dry-run"}, 2, "requires --orphans", ""},
		{"los dos modos mezclados", []string{"remove", "--orphans", "x"}, 2, "cannot be mixed", ""},
		{"ruta con guion inicial", []string{"remove", "-prdash-1"}, 2, "unknown flag", ""},
		{"subcomando vacío", nil, 0, "", "WORKTREE"},
	} {
		var stdout, stderr bytes.Buffer
		code := runWorktrees(pr, &stdout, &stderr, c.args)
		if code != c.want {
			t.Errorf("%s: código = %d, want %d (stderr: %s)", c.nombre, code, c.want, stderr.String())
		}
		if c.aStderr != "" && !strings.Contains(stderr.String(), c.aStderr) {
			t.Errorf("%s: stderr = %q, want que contenga %q", c.nombre, stderr.String(), c.aStderr)
		}
		if c.aStdout != "" && !strings.Contains(stdout.String(), c.aStdout) {
			t.Errorf("%s: stdout = %q, want que contenga %q", c.nombre, stdout.String(), c.aStdout)
		}
		// Y el canal de cada cosa: un error de uso a stdout rompe el `> log`.
		if code == 2 && stdout.String() != "" {
			t.Errorf("%s: un error de uso escribió en stdout: %q", c.nombre, stdout.String())
		}
	}

	// Y un caso de cada lado de "borró / no borró", para que la tabla de arriba no valiera por
	// comprobar solo errores de uso.
	var stdout, stderr bytes.Buffer
	if code := runWorktrees(pr, &stdout, &stderr, []string{"remove", "--orphans", "--dry-run"}); code != 0 {
		t.Errorf("un lote en seco bien formado dio código %d", code)
	}
	if !worktree.Exists(healthy) || !worktree.Exists(foreign) {
		t.Error("el lote en seco tocó algo: Exists en false significa que ya no está")
	}
}

// removeWorkphans es `removeOrphans` con un presupuesto que no depende del reloj del test.
// La razón de no usar `worktreeTimeout` —30 s— es que un test que falla se queda 30 s
// esperando en vez de fallar al momento, y uno que pasa tarde sin motivo enseña a ignorar el
// tiempo de la suite.
func removeWorkphans(pr worktree.Provisioner, stdout, stderr io.Writer, dryRun bool) int {
	return removeOrphans(pr, stdout, stderr, dryRun, 10*time.Second)
}
