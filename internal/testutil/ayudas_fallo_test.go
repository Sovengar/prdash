package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Estos helpers son la base de toda la suite, y su contrato tiene dos mitades: la mitad
// buena —preparan un repo y devuelven la salida— y la mitad que casi nadie ejercita, que es
// **abortar en voz alta cuando el fixture no sirve**.
//
// Y esa segunda mitad es la que hace que los tests fiables. Un helper que en vez de abortar
// devolviera `""` o una ruta a medias dejaría el test siguiente asserts sobre un repo vacío:
// el fixture está roto, el assert falla, y el mensaje que se lee es el del assert —que no
// señala el problema real—. Peor aún: si el assert pasa por casualidad porque el repo vacío
// cumple lo que se comprobaba, el test da verde probando NADA y nadie se entera hasta que
// otra cosa falla.
//
// Y probarlo exige un subproceso, porque `t.Fatal` mata el test que lo llama y `testing.TB`
// no sirve de doble —tiene un método privado— . La forma es la estándar: el binario de test se
// vuelve a ejecutar a sí mismo con una variable de entorno que le dice qué caso ejecutar, y
// el padre comprueba que el hijo MUERTO y que dijo por qué.
//
// Y los casos se eligen por lo que Protect each guards: correr git en el directorio equivocado,
// escribir donde no se puede, y avisar de un adapter roto. Los tres ya pasaron de verdad en
// este repo: una suite Green's dejó el repo en `core.bare=true` con un origin a un TempDir.

// casoDeFalloEnv dice al subproceso qué caso de fallo ejecutar.
const casoDeFalloEnv = "PRDASH_TESTUTIL_CASO_DE_FALLO"

// TestLosHelpersAbortanEnVozAltaEn vez de DevolverCosas: los siete caminos de `t.Fatal`.
//
// Y el nombre dice lo que se comprueba, porque "devuelve un error" y "aborta el test" son
// cosas distintas: lo primero se puede ignorar, lo segundo no.
func TestLosHelpersAbortanEnVozAltaEnVezDeDevolverCosas(t *testing.T) {
	if caso := os.Getenv(casoDeFalloEnv); caso != "" {
		ejecutaCasoDeFallo(t, caso)
		// Si el helper NO abortó, se llega aquí. Y hay que mirar `t.Failed()` en vez de dar
		// el caso por bueno a secas, porque hay dos formas de abortar y solo una mata el
		// subproceso:
		//
		//   - `t.Fatal` mata la goroutine y el subproceso sale con 1 sin llegar aquí.
		//   - `t.Error` marca el test como fallido y CONTINÚA, así que se llega a este
		//     punto con el subproceso a punto de salir con 0 si no se mira.
		//
		// `RunConformance` avisa con `t.Error` porque puede reportar varios incumplimientos de
		// una vez, y eso es lo correcto:listarlos todos en vez de parar en el primero. Sin
		// este `t.Failed()` el subproceso salía con 0 y el padre leía "el helper no abortó"
		// en un caso que sí había abortado.
		if !t.Failed() {
			t.Fatalf("el caso %q NO abortó: el helper devolvió en vez de fallar", caso)
		}
		return
	}

	for _, c := range casosDeFallo() {
		t.Run(c.nombre, func(t *testing.T) {
			salida, code := correCasoEnSubproceso(t, c.nombre)

			if code == 0 {
				t.Fatalf("el helper %s NO abortó. Un fixture roto que no aborta hace que "+
					"los tests siguientes comprueben un repo vacío —o peor, pasen por "+
					"casualidad probando nada.\n%s", c.nombre, salida)
			}
			// Y dice POR QUÉ. Un abort sin motivo es un "test failed" sin pista, que es lo
			// que hace que un fixture roto se investigue en el sitio equivocado.
			if !strings.Contains(salida, c.quiere) {
				t.Errorf("abortó pero sin decir %q:\n%s", c.quiere, salida)
			}
		})
	}
}

type casoDeFallo struct {
	nombre string
	quiere string
}

// casosDeFallo es la lista de guardas del paquete, cada una con un fragmento del mensaje que
// tiene que aparecer. La lista está junto al test y no dentro de él a propósito: es el
// inventario de "cosas que no pueden pasar en silencio", y Léala de un vistazo dice más que
// un `if` disperso por el código de producción.
func casosDeFallo() []casoDeFallo {
	return []casoDeFallo{
		{"git-falla", "exit status"},
		{"dir-inexistente", "no existe"},
		{"dir-vacio", "dir vacío"},
		{"dir-no-directorio", "no es un directorio"},
		// ENOTDIR y no EACCES: el bloqueo es un FICHERO donde el helper quiere un
		// directorio. Mi primera versión pedía "permission denied" porque el modo 000 es la
		// forma habitual de negar una escritura, y falló en las dos: el modo 000 tampoco
		// sirve si los tests corren como root, que es justo el caso de un contenedor de CI.
		// Bloquear por forma es la negación que funciona siempre y en cualquier usuario.
		{"commit-padre-bloqueado", "not a directory"},
		{"commit-destino-directorio", "is a directory"},
		{"bare-vacio", "repo real"},
		{"bare-padre-bloqueado", "not a directory"},
		{"conformance-roto", "debería reportar unsupported"},
	}
}

// ejecutaCasoDeFallo es lo que corre el subproceso. Cada rama tiene que morir; si una no
// muere, el guard del test padre lo detecta.
func ejecutaCasoDeFallo(t *testing.T, caso string) {
	t.Helper()
	switch caso {
	case "git-falla":
		// Un comando de git que existe pero falla. La salida de git va al mensaje, que es lo
		// que hace falta para depurar: un `fatal:` sin más no dice de qué comando salió.
		repo := repoDePrueba(t)
		RunGit(t, repo, "checkout", "no-existe-esta-rama")

	case "dir-inexistente":
		// La guarda que ya pasó de verdad: sin ella, git correría en el directorio del
		// proceso de test, que vive DENTRO del repo, y un `git config` escribiría en la
		// config del repo real.
		RunGit(t, filepath.Join(t.TempDir(), "no-existe"), "status")

	case "dir-vacio":
		RunGit(t, "", "status")

	case "dir-no-directorio":
		fichero := filepath.Join(t.TempDir(), "soy-un-fichero")
		if err := os.WriteFile(fichero, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		RunGit(t, fichero, "status")

	case "commit-padre-bloqueado":
		// Falla al hacer el directorio del padre del fichero, que ya existe como fichero.
		repo := repoDePrueba(t)
		bloqueo := filepath.Join(repo, "bloqueo")
		if err := os.WriteFile(bloqueo, []byte("soy un fichero"), 0o644); err != nil {
			t.Fatal(err)
		}
		CommitFile(t, repo, "bloqueo/subido.txt", "x", "no se puede")

	case "commit-destino-directorio":
		// Falla al ESCRIBIR, con el padre bien: el destino ya existe como directorio y no
		// se puede sobrescribir con un fichero. Es el otro de los dos `t.Fatal` de
		// `CommitFile`, y son Guards distintos —crear el padre y escribir el fichero—.
		repo := repoDePrueba(t)
		if err := os.MkdirAll(filepath.Join(repo, "soy-un-dir"), 0o755); err != nil {
			t.Fatal(err)
		}
		CommitFile(t, repo, "soy-un-dir", "x", "destino que es un directorio")

	case "bare-vacio":
		InitBare(t, "")

	case "bare-padre-bloqueado":
		bloqueo := filepath.Join(t.TempDir(), "bloqueo")
		if err := os.WriteFile(bloqueo, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		InitBare(t, filepath.Join(bloqueo, "repo.git"))

	case "conformance-roto":
		// Un forge que no soporta una operación tiene que DECIRLO, no devolver una lista
		// vacía como si no hubiera nada que enseñar: el usuario ve un inbox en blanco y no
		// tiene forma de saber si le faltan credenciales o si ese forge no lo soporta.
		//
		// Y el modo importa: `no-avisar` SIN modo pedido es conforme por diseño —un adapter
		// puede no avisar de lo que nadie le preguntó—, así que mi primera versión, que lo
		// usaba sin `Unsupported`, no disparaba nada y el subproceso salía con 0. Un caso de
		// fallo que no falla es peor que no tener caso: parece que la guarda está probada.
		RunConformance(t, &adapterQueFalla{roto: "no-avisar"},
			ConformanceOptions{Unsupported: true})
	}
}

// repoDePrueba es un repo git listo, para los casos que necesitan uno de verdad.
func repoDePrueba(t *testing.T) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "repo")
	InitRepo(t, repo)
	CommitFile(t, repo, "a.txt", "a", "a")
	return repo
}

// correCasoEnSubproceso re-ejecuta este binario de test con la variable de entorno puesta y
// devuelve su salida combinada y su código.
//
// Y el `-test.run` lleva el nombre del test entero y no un prefijo: el patrón es una
// expresión regular, y un prefijo corto también capturaría los subtests, que es justo lo que
// no se quiere —el hijo tiene que ejecutar UN caso y volver.
func correCasoEnSubproceso(t *testing.T, caso string) (string, int) {
	t.Helper()
	cmd := exec.Command(os.Args[0],
		"-test.run=^TestLosHelpersAbortanEnVozAltaEnVezDeDevolverCosas$",
		"-test.v")
	cmd.Env = append(os.Environ(), casoDeFalloEnv+"="+caso)
	out, err := cmd.CombinedOutput()

	codigo := 0
	var salidaErr *exec.ExitError
	if err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			salidaErr = e
			codigo = e.ExitCode()
		} else {
			// El binario ni siquiera arrancó: eso también es un fallo del caso, y hay que
			// decirlo en vez de devolver un 0 que parecería "el helper no abortó".
			t.Fatalf("no se pudo ejecutar el subproceso: %v", err)
		}
	}
	_ = salidaErr
	// Y el fallo tiene que ser el del abort, no un panic del hijo. Un panic cuelga el test
	// entero y no dice nada del helper, que es el peor de los dos desenlaces.
	if strings.Contains(string(out), "panic:") {
		t.Errorf("el subproceso del caso %s entró en panic en vez de abortar el test:\n%s",
			caso, out)
	}
	return string(out), codigo
}
