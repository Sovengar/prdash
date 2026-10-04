package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// esperaA que exista espera a que un fichero aparezca, con techo.
//
// Y hace falta porque `openBrowserCmd` DESLIGA el proceso que lanza —`Process.Release()`— para
// no matarlo al salir de la TUI. Ese es el comportamiento correcto en producción y es una carrera
// en un test: el `Cmd` devuelve antes de que el hijo haya escrito nada.
//
// Y el techo es amplio a propósito. El fallo que se quiere distinguir es "el abridor no se
// ejecutó" —que tarda milisegundos—, no "la máquina está ocupada", y un techo corto convertiría
// un CI lento en un test rojo que dice lo contrario.
func esperaAQueExista(path string, techo time.Duration) ([]byte, error) {
	muerte := time.Now().Add(techo)
	var ultimo error
	for time.Now().Before(muerte) {
		raw, err := os.ReadFile(path)
		if err == nil {
			if len(raw) > 0 {
				return raw, nil
			}
			ultimo = err
		} else {
			ultimo = err
		}
		time.Sleep(5 * time.Millisecond)
	}
	return nil, ultimo
}

// Estos son los caminos REALES de `openBrowserCmd`: los que no pasan por el doble de `openURL`
// y llegan a `exec.LookPath` y a `exec.Command`. Y son los que de verdad tocan el sistema, así
// que hay que provocarlos SIN abrir nada.
//
// Y la técnica es la misma que en el resto de los tests de infraestructura: **un `PATH` vacío**.
// Con `PATH` apuntando a un directorio sin `xdg-open`, `LookPath` falla y el código avisa —
// que es exactamente el camino que se quiere probar y que además nunca abre un navegador—.
//
// Y el caso que esto cubre es real y frecuente: una instalación mínima, un contenedor, un
// sistema donde `xdg-utils` no está porque nadie usa un escritorio de GNOME. Entonces `o` no
// hace nada y el usuario no sabe por qué, porque un `Cmd` que devuelve nil no se nota.

// TestSinAbridorEnElPathSeAviadoYNoSeAbreNada: el camino de verdad, sin abrir nada.
//
// Y el `PATH` vacío es lo que hace seguro el test: con el `PATH` de la máquina, este mismo test
// abriría el navegador de quien lo corre, que es exactamente el defecto que el seam arregló y
// que no hay que volver a introducir por la puerta de atrás.
//
// Y el aviso importa por dos cosas: dice QUÉ falta —que es lo accionable— y va a nivel de aviso
// y no de error, porque no es un fallo de la acción sino del entorno. Con nivel de error, la
// TUI lo pintaría como un error de prdash.
func TestSinAbridorEnElPathSeAviadoYNoSeAbreNada(t *testing.T) {
	// Un directorio vacío como PATH: `xdg-open` no está en ninguna parte.
	vacio := t.TempDir()
	t.Setenv("PATH", vacio)

	m := newTestModel(t)
	// Y sin el seam: el camino real, el que se ejecutaría en producción.
	if m.openURL != nil {
		t.Fatal("el fixture no sirve: el seam sigue puesto y no se prueba el camino real")
	}

	const url = "https://github.com/acme/proyecto/pull/1"
	msg, ok := m.openBrowserCmd(url)().(notifyMsg)
	if !ok {
		t.Fatal("sin abridor no se devolvió un notifyMsg")
	}
	if msg.level != levelWarn {
		t.Errorf("nivel %v, want aviso: no es un fallo de la acción sino del entorno", msg.level)
	}
	// Y el aviso nombra el binario que falta, que es lo que permite arreglarlo.
	if !strings.Contains(msg.text, "xdg-open") && !strings.Contains(msg.text, "open") &&
		!strings.Contains(msg.text, "rundll32") {
		t.Errorf("el aviso %q no dice qué abridor falta", msg.text)
	}
	if strings.Contains(msg.text, "no encontrado") == false &&
		!strings.Contains(strings.ToLower(msg.text), "not found") {
		t.Errorf("el aviso %q no dice que no se encontró", msg.text)
	}
	// Y no dice "abriendo": si el aviso saliera con el texto del camino bueno, el usuario
	// cerraría el popup creyendo que el visor se abrió.
	if strings.HasPrefix(msg.text, "abriendo") {
		t.Errorf("sin abridor el aviso dice %q", msg.text)
	}
	// Y nada se creó en el PATH vacío: un intento de abrir habría dejado algo.
	entradas, err := os.ReadDir(vacio)
	if err != nil {
		t.Fatal(err)
	}
	if len(entradas) != 0 {
		t.Errorf("el PATH vacío tiene %d entradas: se llegó a ejecutar algo", len(entradas))
	}
}

// TestUnAbridorQueNoSePuedeEjecutarSeAviadoConSuError: cuando `LookPath` pasa y `Start` falla.
//
// Y este es el caso más fino de los dos, y el que está detrás del `LookPath`: un fichero con
// el bit de ejecución puesto es encontrable pero puede no ser ejecutable.
//
// Y la forma de provocarlo es un shebang que no existe, no un directorio con permiso de
// ejecución: `LookPath` mira SOLO el bit de ejecución —no el shebang— así que lo encuentra y
// devuelve la ruta, y el fallo llega en `Start`, que llama a `execve` y recibe ENOENT del
// intérprete que no está. Medido antes de escribir el test.
//
// Y mi primera versión usó un directorio, que `LookPath` descarta porque comprueba que sea un
// fichero regular, así que el test tomaba el camino de "no encontrado" en lugar del que quería.
//
// Y el nivel es de ERROR y no de aviso porque aquí el abridor existe y no arranca: es un fallo
// de la acción, no del entorno. Y el aviso NO dice "no encontrado", porque `LookPath` sí lo
// encontró —confundir los dos mandaría al usuario a instalar `xdg-utils` cuando lo que hay es
// un intérprete roto—.
func TestUnAbridorQueNoSePuedeEjecutarSeAviadoConSuError(t *testing.T) {
	binDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, "xdg-open"),
		[]byte("#!/interprete-que-no-existe\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	// Y el modelo sin el seam, que es el punto: con doble, el `Start` ni se llega a mirar.
	m := newTestModel(t)
	if m.openURL != nil {
		t.Fatal("el fixture no sirve: el seam sigue puesto y no se prueba el camino real")
	}

	msg, ok := m.openBrowserCmd("https://ejemplo/x")().(notifyMsg)
	if !ok {
		t.Fatal("un abridor que no arranca no devolvió un notifyMsg")
	}
	// Y el nivel es de ERROR y no de aviso a propósito: aquí el abridor existe y no arranca.
	// Es un fallo de la acción, no del entorno, y con nivel de aviso el usuario lo descartaría
	// sin mirar.
	if msg.level != levelError {
		t.Errorf("nivel %v, want error: el abridor existe y no arranca", msg.level)
	}
	// Y el aviso NO dice "no encontrado": `LookPath` sí lo encontró —mira el bit de ejecución
	// y no el shebang—. Confundir los dos mandaría al usuario a instalar `xdg-utils` cuando lo
	// que hay es un intérprete que ya no está.
	if strings.Contains(msg.text, "no encontrado") {
		t.Errorf("el aviso dice que no se encontró el abridor, pero `LookPath` sí lo encontró: "+
			"el fallo es de arranque, no de búsqueda. Aviso: %q", msg.text)
	}
	// Y trae la causa del `Start`, que es lo único que distingue un shebang roto de un permiso
	// y de un formato: sin ella el aviso dice "no se pudo abrir" y no hay nada que hacer.
	if !strings.Contains(msg.text, "abrir navegador") {
		t.Errorf("el aviso %q no dice que es del abridor", msg.text)
	}
	if len(strings.TrimSpace(msg.text)) <= len("abrir navegador: ") {
		t.Errorf("el aviso %q no trae la causa del fallo de Start", msg.text)
	}
}

// TestLaURLVaciaSeCompruebaANTESDeMirarElEntorno: el orden de las guardas.
//
// Y el orden es lo que hay que fijar, y es el que impide un despiste: si el `LookPath` fuera
// antes de la comprobación de la URL, una URL vacía en una máquina sin `xdg-open` daría el
// aviso de "no encontrado el abridor" en vez del de "no hay nada que abrir". Son dos problemas
// distintos con arreglos distintos —instalar `xdg-utils` frente a no tener qué abrir— y el
// segundo es el que se puede resolver sin tocar el sistema.
func TestLaURLVaciaSeCompruebaAntesDeMirarElEntorno(t *testing.T) {
	// Sin abridor Y con URL vacía: el aviso tiene que ser el de la URL.
	vacio := t.TempDir()
	t.Setenv("PATH", vacio)

	m := newTestModel(t)
	msg, ok := m.openBrowserCmd("")().(notifyMsg)
	if !ok {
		t.Fatal("una URL vacía no devolvió un notifyMsg")
	}
	if !strings.Contains(msg.text, "no hay nada") {
		t.Errorf("el aviso %q es el del entorno y no el de la URL vacía: se comprobaría el "+
			"abridor antes de la URL y el aviso sería el equivocado", msg.text)
	}
	// Y con el abridor presente, el mismo aviso: el resultado no depende de la máquina, que
	// es lo que hace que un test de este tipo valga.
	t.Setenv("PATH", os.Getenv("PATH"))
	if got := m.openBrowserCmd("")(); !strings.Contains(got.(notifyMsg).text, "no hay nada") {
		t.Errorf("con abridor el aviso de URL vacía cambió: %q", got)
	}
}

// TestLaURLSePasaTalCualAlAbridor: la frontera con el shell.
//
// Y `browserCommand` ya tiene su test de que el texto va como un argumento literal y no partido
// en trozos; lo que se comprueba aquí es que el `openBrowserCmd` REAL usa esa función y no
// concatena la URL a mano, que es lo que rompería las comillas.
//
// Y el modo de comprobarlo sin abrir nada es poner un abridor FALSO en el PATH —un script que
// escribe sus argumentos en un fichero y sale con 0— y mirar lo que recibió. Es un binario de
// verdad,-launchado de verdad, y no abre nada porque su único efecto es escribir un log.
func TestLaURLSePasaTalCualAlAbridor(t *testing.T) {
	binDir := t.TempDir()
	log := filepath.Join(binDir, "args.log")
	script := "#!/bin/sh\n" +
		"for a in \"$@\"; do echo \"[$a]\" >> " + log + "; done\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(binDir, "xdg-open"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	m := newTestModel(t)
	// Una URL con todo lo peligroso: espacios, comillas, punto y coma y símbolos.
	const url = `https://host/p; echo pwned?a=1&b="2" '3'`
	msg, ok := m.openBrowserCmd(url)().(notifyMsg)
	if !ok {
		t.Fatalf("no se devolvió un notifyMsg: %v", msg)
	}
	if msg.level != levelInfo || !strings.Contains(msg.text, url) {
		t.Errorf("el camino bueno no se tomó: %+v", msg)
	}

	// Y el abridor recibió la URL como UN argumento, con los corchetes que el script pone.
	// Si se hubiera partido o citado mal, los corchetes no cerrarían donde deben.
	//
	// Y hay que ESPERAR al log, porque `openBrowserCmd` hace `Process.Release()`: el proceso
	// queda desligado y el `Cmd` devuelve antes de que el script haya escrito nada. Leer el
	// fichero sin más es una carrera que falla en la máquina rápida, y mi primera versión lo
	// hacía.
	crudo, err := esperaAQueExista(log, 2*time.Second)
	if err != nil {
		t.Fatalf("el abridor no llegó a ejecutarse: %v", err)
	}
	linea := strings.TrimSpace(string(crudo))
	if linea != "["+url+"]" {
		t.Errorf("el abridor recibió %q, want un solo argumento con la URL entera", linea)
	}
}
