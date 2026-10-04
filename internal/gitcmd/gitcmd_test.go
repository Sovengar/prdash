package gitcmd

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// `Run` es la función de la que depende todo lo que hace prdash con git: worktrees, refs,
// merges, fetch. Y estaba cubierta al 68%, que es una cifra que en un paquete con una
// sola función ejecutora significa lo que dice: la mitad de lo que hace no se ha probado
// nunca, y la mitad es justo donde están los caminos que[borrar] cosas.
//
// Y el reparto de lo que faltaba no era casual. Estaba sin probar el mensaje de error —que
// es lo que el usuario ve cuando una operación falla— y la función `firstLine` entera, que
// es la que decide qué parte del stderr de git llega a pantalla. Esa función decide qué se
// le enseña a la persona, y sin test no había forma de saber qué se le enseñaba.

// TestElMensajeDeErrorTraeLoQueHaceFaltaParaArreglarlo: el mensaje de un fallo de git
// tiene que decir qué se ejecutó, dónde y cómo terminó.
//
// Y son tres datos, no uno, porque cada uno responde a una pregunta distinta de quien
// está delante de la TUI sin saber git: "¿qué ha hecho prdash?", "¿en qué repo?" y
// "¿ha sido un rechazo o un fallo?". Con solo el primero —"git rev-parse: exit status
// 128"— el mensaje es correcto y sirve de nada.
//
// Y el código de salida va aparte porque distingue dos fallos que se confunden: git
// rechazando algo (128) y git no pudiendo arrancar (127, normalmente binario ausente).
func TestElMensajeDeErrorTraeLoQueHaceFaltaParaArreglarlo(t *testing.T) {
	casos := []struct {
		nombre string
		e      *Error
		want   string
	}{
		{
			nombre: "con directorio y codigo",
			e:      &Error{Args: []string{"rev-parse", "HEAD"}, Dir: "/repos/proy", Msg: "fatal: not a git repository", ExitCode: 128},
			want:   "git -C /repos/proy rev-parse HEAD: fatal: not a git repository (exit 128)",
		},
		{
			// Sin directorio: el `git -C` desaparece entero, no sale un `-C ` vacío que
			// luego se lee como un argumento.
			nombre: "sin directorio",
			e:      &Error{Args: []string{"status"}, Msg: "no changes", ExitCode: 1},
			want:   "git status: no changes (exit 1)",
		},
		{
			// Código cero con error: es el caso de "matado por una señal", y el mensaje
			// tiene que ser legible sin el sufijo. Un "(exit 0)" después de un error es
			// ruido que hace pensar que la operación salió bien.
			nombre: "error sin codigo",
			e:      &Error{Args: []string{"log"}, Msg: "interrumpido"},
			want:   "git log: interrumpido",
		},
		{
			// Sin argumentos: se leen los nombres de `Error` en la TUI, así que la
			// estructura tiene que sobrevivir al caso degenerado.
			nombre: "sin argumentos",
			e:      &Error{Msg: "algo"},
			want:   "git : algo",
		},
		{
			nombre: "varios argumentos",
			e:      &Error{Args: []string{"worktree", "add", "-b", "b", "/r"}, Msg: "ya existe"},
			want:   "git worktree add -b b /r: ya existe",
		},
	}
	for _, c := range casos {
		if got := c.e.Error(); got != c.want {
			t.Errorf("%s: Error() dio %q, want %q", c.nombre, got, c.want)
		}
	}
}

// TestElErrorDesenvuelveLaCausa: `Unwrap` es lo que permite decir "esto no ha salido
// decir que no es el error de git".
//
// Y el caso que lo justifica es `errors.As(err, &exit)`: sin `Unwrap`, un `*Error` que
// envuelve un `*exec.ExitError` no dejaría reachedlo, y la consecuencia sería que
// `ExitCode` saldría 0 en todos los fallos —porque nunca se llega a leerlo—. El código de
// salida es lo que distingue "git rechazó esto" de "git no se pudo ejecutar", y los dos
// avisos que ve el usuario son distintos.
//
// O sea: este test no prueba un método trivial. Prueba que la cadena de `errors` está
// montada, y si alguien la rompe el código de salida se queda a cero sin que nada falle.
func TestElErrorDesenvuelveLaCausa(t *testing.T) {
	// Un error que NO es de git: nil underneath.
	sinCausa := &Error{Args: []string{"x"}, Msg: "m"}
	if sinCausa.Unwrap() != nil {
		t.Error("Unwrap devolvio algo en un Error sin causa")
	}
	if errors.Unwrap(sinCausa) != nil {
		t.Error("errors.Unwrap devolvio algo en un Error sin causa")
	}

	// Con causa de git: se llega a ella por `errors.As`.
	cmd := exec.Command("sh", "-c", "exit 42")
	err := cmd.Run()
	if err == nil {
		t.Fatal("esperaba que sh -c 'exit 42' fallara")
	}
	conCausa := &Error{Args: []string{"x"}, Msg: "m", Err: err}

	if !errors.Is(conCausa, err) {
		t.Error("errors.Is no llega a la causa")
	}
	var exit *exec.ExitError
	if !errors.As(conCausa, &exit) {
		t.Fatal("errors.As no llega al *exec.ExitError: sin esto ExitCode es 0 siempre")
	}
	if exit.ExitCode() != 42 {
		t.Errorf("ExitCode = %d, want 42", exit.ExitCode())
	}
	// Y la cadena sigue siendo imprimible sin perder nada.
	if !strings.Contains(conCausa.Error(), "m") {
		t.Errorf("el mensaje perdió el texto propio: %q", conCausa.Error())
	}
}

// TestRunTraeElErrorRealDeGit: `Run` sobre un repo de verdad, fallando de verdad.
//
// Y el repo lo monta `testutil`, que es lo que permite esto sin depender de que haya git
// instalado con una config concreta. El fallo que se provoca es un comando que existe y
// se niega: así el stderr es de git y no del shell.
func TestRunTraeElErrorRealDeGit(t *testing.T) {
	dir := repoVacio(t)

	// Un comando que git rechaza. El stderr de git es lo que tiene que llegar.
	_, err := New().Run(context.Background(), dir, "cat-file", "-p", "no-existe")
	if err == nil {
		t.Fatal("git cat-file sobre un objeto inexistente dio nil")
	}
	gerr, ok := err.(*Error)
	if !ok {
		t.Fatalf("Run devolvió %T, want *Error", err)
	}
	if gerr.ExitCode == 0 {
		t.Error("ExitCode = 0 en un fallo de git: el código de salida no se leyó")
	}
	// Y el mensaje trae la primera línea de stderr, no el `err.Error()` del proceso. La
	// diferencia se ve: `err.Error()` es "exit status 128", que no explica nada.
	if strings.HasPrefix(gerr.Msg, "exit status") {
		t.Errorf("el mensaje es %q: se cogió el error del proceso en vez de su stderr", gerr.Msg)
	}
	if strings.TrimSpace(gerr.Msg) == "" {
		t.Error("el mensaje quedó vacío")
	}
	// Y una sola línea: el stderr de git puede traer varias, y el toast solo cabe una.
	if strings.Contains(gerr.Msg, "\n") {
		t.Errorf("el mensaje tiene saltos de línea: %q", gerr.Msg)
	}
	// Y el error trae contexto para el mensaje final.
	if !strings.Contains(gerr.Error(), "cat-file") {
		t.Errorf("Error() no nombra el comando: %q", gerr.Error())
	}
	if !strings.Contains(gerr.Error(), dir) {
		t.Errorf("Error() no nombra el directorio: %q", gerr.Error())
	}
}

// TestRunDevuelveLaSalidaParcialCuandoFalla: lo que se escribió antes del fallo, se
// devuelve.
//
// Y es el comportamiento correcto para lo que prdash hace con git: un `git log` que
// escribe veinte commits y falla en el veintiuno ha dado información útil, y tirar la
// salida deja al usuario con un error y nada que mirar. Además, `out` y el error van
// juntos, así que quien los recibe puede usar lo uno o lo otro.
//
// Y para probarlo hace falta un binario que escriba y LUEGO falle, porque un comando de
// git real no hace eso: o sale limpio o no imprime nada. La primera versión de este test
// buscaba ese comando y no existe, así que la conclusión "no se puede probar" era falsa —
// lo que no se puede esprovocar con git— y la forma de probarlo es un binario falso que
// haga las dos cosas, que es justo el caso que `Run` tiene que sostener.
func TestRunDevuelveLaSalidaParcialCuandoFalla(t *testing.T) {
	dir := t.TempDir()
	parcial := filepath.Join(dir, "git-parcial")
	if err := os.WriteFile(parcial, []byte("#!/bin/sh\necho 'linea buena'\necho 'fatal: se rompio' >&2\nexit 7\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	out, err := (&Runner{Bin: parcial}).Run(context.Background(), dir, "log")
	if err == nil {
		t.Fatal("un binario que sale con 7 dio nil")
	}
	// La salida.good line se conserva.
	if !strings.Contains(out, "linea buena") {
		t.Errorf("la salida anterior al fallo se perdió: %q", out)
	}
	gerr, ok := err.(*Error)
	if !ok {
		t.Fatalf("Run devolvió %T, want *Error", err)
	}
	if gerr.ExitCode != 7 {
		t.Errorf("ExitCode = %d, want 7", gerr.ExitCode)
	}
	if !strings.Contains(gerr.Msg, "se rompio") {
		t.Errorf("el mensaje no trae el stderr: %q", gerr.Msg)
	}

	// Y el caso de "no escribió nada": la salida vacía con error no es un acierto. Con
	// `echo ""` en vez de una línea buena, `out` sale vacía y el error sigue estar.
	if err := os.WriteFile(parcial, []byte("#!/bin/sh\nexit 7\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, err = (&Runner{Bin: parcial}).Run(context.Background(), dir, "log")
	if err == nil {
		t.Fatal("un binario que sale con 7 dio nil")
	}
	if out != "" {
		t.Errorf("un binario que no escribió devolvió %q", out)
	}
	// Y el mensaje, al no haber stderr, cae al error del proceso. Es feo pero es lo que
	// hay, y lo importante es que no quede vacío.
	if strings.TrimSpace(gerr.Msg) == "" {
		t.Error("el mensaje del primer caso quedó vacío")
	}
}

// TestElCaminoFelizTraeLaSalidaEntera: el contraste, para que el test anterior no_valga
// por el caso de que `Run` devuelva siempre vacío.
func TestElCaminoFelizTraeLaSalidaEntera(t *testing.T) {
	dir := repoVacio(t)
	r := New()

	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hola\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), dir, "add", "a.txt"); err != nil {
		t.Fatalf("git add: %v", err)
	}
	out, err := r.Run(context.Background(), dir, "diff", "--cached", "--stat")
	if err != nil {
		t.Fatalf("git diff --cached: %v", err)
	}
	if !strings.Contains(out, "a.txt") {
		t.Errorf("la salida de git no llegó intacta: %q", out)
	}
}

// TestFirstLineCortaLoQueNoCabeEnElToast: qué parte de un stderr multilínea ve el usuario.
//
// Y el orden importa: primero el recorte de la derecha, luego el corte de la primera
// línea. Al revés, un stderr que empieza con un espacio deja el toast empezando por un
// espacio y el texto descentrado.
//
// Y el caso sin salto de línea es el que devuelve la cadena entera, que es lo que pasa con
// el stderr de una sola línea —que es el de la mayoría de errores de git—.
func TestFirstLineCortaLoQueNoCabeEnElToast(t *testing.T) {
	casos := []struct {
		entrada string
		want    string
	}{
		{"primera\nsegunda\ntercera", "primera"},
		{"  con espacios  \nsegunda", "  con espacios  "},
		{"una sola linea", "una sola linea"},
		{"", ""},
		{"\nempieza con salto", ""},
		{"con\n", "con"},
		{"trailing  \n", "trailing  "},
	}
	for _, c := range casos {
		if got := firstLine(c.entrada); got != c.want {
			t.Errorf("firstLine(%q) dio %q, want %q", c.entrada, got, c.want)
		}
		// Y nunca sale más de una línea, que es la propiedad entera de la función.
		if strings.ContainsAny(firstLine(c.entrada), "\n") {
			t.Errorf("firstLine(%q) devolvio texto con salto", c.entrada)
		}
	}
}

// TestElEntornoDeGitNoHeredaelContextoDelShell: `Env` quita las GIT_* de localización y
// las del contexto de git, y pone las suyas.
//
// Y esto es lo más importante que hace el paquete, y por eso la comprobación no es de
// contenido sino de AUSENCIA. Con GIT_DIR o GIT_WORK_TREE en el entorno, git opera en
// ESE repo y da igual el `-C` que se le pase. prdash elige el repo de cada ítem por su
// cuenta, así que heredar el contexto de git de quien lo lanzó haría que una operación de
// la TUI cayera en otro repo —que es el fallo que borra la rama equivocada—.
//
// Y el `LC_ALL=C` va por la razón contraria: sin él, un usuario con el locale en español
// recibe los errores de git traducidos, y los mensajes que prdash compara con texto fijo
// dejan de casar.
func TestElEntornoDeGitNoHeredaelContextoDelShell(t *testing.T) {
	// Montar un entorno hostil: cada variable que tiene que desaparecer.
	hostiles := map[string]string{
		"GIT_DIR":                          "/otro/repo/.git",
		"GIT_WORK_TREE":                    "/otro/repo",
		"GIT_INDEX_FILE":                   "/otro/index",
		"GIT_COMMON_DIR":                   "/otro/common",
		"GIT_OBJECT_DIRECTORY":             "/otro/objs",
		"GIT_ALTERNATE_OBJECT_DIRECTORIES": "/otro/alt",
		"GIT_NAMESPACE":                    "otro",
		"GIT_CEILING_DIRECTORIES":          "/otro/techo",
		"GIT_PREFIX":                       "src/",
		"LC_ALL":                           "es_ES.UTF-8",
		"LANG":                             "es_ES.UTF-8",
		"LANGUAGE":                         "es",
		"LC_MESSAGES":                      "es_ES.UTF-8",
	}
	for k, v := range hostiles {
		t.Setenv(k, v)
	}

	env := Env()
	vistos := map[string]string{}
	for _, kv := range env {
		if i := strings.IndexByte(kv, '='); i > 0 {
			vistos[kv[:i]] = kv[i+1:]
		}
	}

	for k := range hostiles {
		if _, sigue := vistos[k]; sigue {
			// LC_ALL y LANG tienen que estar, pero con el valor forzado. El resto no
			// puede estar.
			if k == "LC_ALL" || k == "LANG" {
				continue
			}
			t.Errorf("%s sigue en el entorno con el valor %q: git operaría en ese "+
				"contexto en vez del que se le pide", k, vistos[k])
		}
	}

	// Y lo que se pone, con los valores exactos.
	for k, want := range map[string]string{
		"LC_ALL":              "C",
		"GIT_TERMINAL_PROMPT": "0",
		"GIT_PAGER":           "cat",
		"NO_COLOR":            "1",
	} {
		if vistos[k] != want {
			t.Errorf("%s = %q, want %q", k, vistos[k], want)
		}
	}
	// Y LANG se quita pero LC_ALL se queda: `LC_ALL` gana sobre `LANG`, y como se pone
	// explícitamente a C, da igual lo que diga LANG. Por eso se quita LANG —para que no
	// haya dos variables compitiendo— y no hace falta ponerla.
	if _, sigue := vistos["LANG"]; sigue {
		t.Error("LANG sigue presente: compite con el LC_ALL que se pone")
	}

	// Y una variable que no tiene nada que ver se conserva. Si `Env` filtrara de más, un
	// PATH vacío o un HOME equivocado rompería git de una forma difícil de ver.
	t.Setenv("PRDASH_TEST_QUE_SI_SE_CONSERVA", "valor")
	vistos = map[string]string{}
	for _, kv := range Env() {
		if i := strings.IndexByte(kv, '='); i > 0 {
			vistos[kv[:i]] = kv[i+1:]
		}
	}
	if vistos["PRDASH_TEST_QUE_SI_SE_CONSERVA"] != "valor" {
		t.Error("Env tiró una variable que no debía: el entorno se filtra de más")
	}
}

// TestElTimeoutDeRunCortaDeVerdad: como en herdr, el timeout tiene que cortar y no solo
// matar al proceso.
//
// Y aquí el hijo que hereda los descriptores es aún más fácil que en el caso de herdr,
// porque git los lanza de serie: un `git fetch` abre un proceso de transporte, y un
// `git commit` lanza un hook. Un hook que se quede con el stdout abierto es un caso real,
// no uno inventado, y sin `WaitDelay` la TUI se queda esperando al hook.
//
// Y el suelo por defecto se comprueba en la dirección contraria al primer caso, por lo
// mismo que en herdr: con el suelo de 60s y un binario que duerme un segundo, lo
// correcto es que tarde un segundo.
func TestElTimeoutDeRunCortaDeVerdad(t *testing.T) {
	dir := t.TempDir()
	// Un "git" que se cuelga dejando un hijo vivo con los descriptores abiertos: el
	// patrón que hacía que `cmd.Run()` no volviera.
	colgado := filepath.Join(dir, "git-colgado")
	script := "#!/bin/sh\nsh -c 'sleep 5' &\nsleep 5\n"
	if err := os.WriteFile(colgado, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	r := &Runner{Bin: colgado, Timeout: 50 * time.Millisecond}
	inicio := time.Now()
	_, err := r.Run(context.Background(), dir, "status")
	elapsed := time.Since(inicio)

	if err == nil {
		t.Error("un git colgado dio nil")
	}
	if elapsed > 2*time.Second {
		t.Errorf("Run tardó %s con un timeout de 50ms: el contexto no corta la llamada, "+
			"solo mata al proceso", elapsed)
	}

	// Y con el Timeout vacío se aplica el suelo por defecto, que no es el timeout del
	// caso anterior.
	corto := filepath.Join(dir, "git-corto")
	if err := os.WriteFile(corto, []byte("#!/bin/sh\nsleep 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	r = &Runner{Bin: corto}
	inicio = time.Now()
	if _, err := r.Run(context.Background(), dir, "status"); err != nil {
		t.Errorf("con el suelo por defecto y un binario sano dio error: %v", err)
	}
	if elapsed := time.Since(inicio); elapsed < 900*time.Millisecond {
		t.Errorf("con Timeout vacío tardó %s: se aplicó un timeout corto en vez del suelo",
			elapsed)
	}
}

// TestElBinarioPorDefectoEsGitYNoElCampoVacio: sin `Bin` puesto se usa "git".
//
// Y el motivo de que sea un nombre y no una ruta resuelta está en que el Runner lo
// normalize al construirse en `New`; aquí solo se comprueba que el camino vacío no rompe.
func TestElBinarioPorDefectoEsGitYNoElCampoVacio(t *testing.T) {
	dir := repoVacio(t)
	// Sin Bin: sale "git". Con un repo de verdad detrás, la operación tiene que funcionar,
	// así que si no llegara al binario real fallaría.
	if _, err := (&Runner{}).Run(context.Background(), dir, "rev-parse", "--git-dir"); err != nil {
		t.Fatalf("sin Bin, Run falló: %v", err)
	}
	// Y `New` lo pone explícito.
	if got := New().Bin; got != "git" {
		t.Errorf("New().Bin = %q, want git", got)
	}
	// Y con un Timeout ya puesto, que es lo que hace `New`.
	if got := New().Timeout; got != DefaultTimeout {
		t.Errorf("New().Timeout = %v, want DefaultTimeout", got)
	}
}

// repoVacio crea un repo git sin commits bajo t.TempDir().
func repoVacio(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	r := New()
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "t@example.com"},
		{"config", "user.name", "Test"},
	} {
		if _, err := r.Run(context.Background(), dir, args...); err != nil {
			t.Fatalf("git %s: %v", strings.Join(args, " "), err)
		}
	}
	return dir
}
