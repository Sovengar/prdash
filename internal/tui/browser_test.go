package tui

import (
	"errors"
	"runtime"
	"strings"
	"testing"
)

// `browserCommand` decide qué se ejecuta para abrir una URL, y es una función pura con una
// tabla detrás. Lo que se fija es lo que evita el peor resultado posible: ejecutar algo que
// no es un abridor.
//
// El peor caso no es "no abre el navegador", es "ejecuta otra cosa". Una URL viene del
// forge, y una URL con un esquema raro —`file://`, o algo que la CLI interprete como
// opción— pasada a un abridor que la pase a un shell, es ejecución de código. Por eso el
// ejecutable y sus argumentos se eligen aquí y no en el `Cmd`.
//
// Y la diferencia entre plataformas importa en la forma de los argumentos, que no es la
// misma: en Windows el abridor no recibe la URL como un argumento normal sino embebida en
// `/c start`. Un `exec.Command(bin, url)` en Windows no abre nada.

// TestElAbridorSeEligePorPlataformaYNoEsElDelSistemaEntero: la tabla.
//
// Y lo que se fija por plataforma es la FORMA, no solo el nombre. Y el caso de un SO
// desconocido: cae a algo, y lo que no puede hacer es devolver el sistema entero —un
// `exec.Command("sh", "-c", url)` sería ejecución de código con una URL del forge—.
func TestElAbridorSeEligePorPlataformaYNoEsElDelSistemaEntero(t *testing.T) {
	url := "https://github.com/acme/proyecto/pull/1"

	casos := []struct {
		so       string
		wantBin  string
		wantArgs []string
	}{
		{"linux", "xdg-open", []string{url}},
		{"darwin", "open", []string{url}},
		// Y Windows: el abridor no acepta la URL como argumento suelto.
		{"windows", "rundll32", []string{"url.dll,FileProtocolHandler", url}},
	}
	for _, c := range casos {
		bin, args := browserCommand(c.so, url)
		if bin != c.wantBin {
			t.Errorf("%s: dio el binario %q, want %q", c.so, bin, c.wantBin)
		}
		if len(args) != len(c.wantArgs) {
			t.Fatalf("%s: dio %d argumentos %q, want %d %q", c.so, len(args), args,
				len(c.wantArgs), c.wantArgs)
		}
		for i := range c.wantArgs {
			if args[i] != c.wantArgs[i] {
				t.Errorf("%s: argv[%d] = %q, want %q (completo %q)", c.so, i, args[i],
					c.wantArgs[i], args)
			}
		}
	}

	// Y lo que no puede pasar en ninguna plataforma: un intérprete con la URL como código.
	prohibidos := []string{"sh", "bash", "zsh", "cmd", "cmd.exe", "powershell", "pwsh"}
	for _, so := range []string{"linux", "darwin", "windows", "freebsd", "plan9", ""} {
		bin, args := browserCommand(so, url)
		for _, mal := range prohibidos {
			if bin == mal {
				t.Errorf("%s: usa %q, que interpretaría la URL", so, bin)
			}
		}
		// Y la URL viaja siempre como argumento propio, nunca pegada a un `-c`.
		for _, a := range args {
			switch a {
			case "-c", "-Command", "/c":
				t.Errorf("%s: pasa %q, que interpretaría lo que viene detrás", so, a)
			}
		}
		if len(args) == 0 {
			t.Errorf("%s: sin argumentos, no abriría nada", so)
		}
		// Y el último argumento es la URL en todas partes, que es la forma en que el
		// abridor la espera.
		if args[len(args)-1] != url {
			t.Errorf("%s: el ultimo argumento no es la URL: %q", so, args)
		}
	}
}

// TestUnaURLVaciaNoSeAbreYUnaConEspaciosNoSeParte: los dos bordes de la entrada.
//
// Y el caso de los espacios es el que protege de verdad: si la URL se partiera por
// espacios, "https://github.com/acme/mi proyecto" se abriría como dos argumentos, el
// abridor abriría el primero y el segundo se trataría como un archivo o un error. Y una
// URL de fork de GitHub trae ramas con espacios, así que no es un caso inventado.
func TestUnaURLVaciaNoSeAbreYUnaConEspaciosNoSeParte(t *testing.T) {
	// Y la URL vacía: `browserCommand` la pasa tal cual, porque no es su trabajo
	// decidir si una URL merece abrirse —eso lo hace el `Cmd` que compone el aviso—. La
	// guarda está un nivel más arriba y tiene su propio test más abajo.
	for _, so := range []string{"linux", "darwin", "windows"} {
		bin, args := browserCommand(so, "")
		if bin == "" {
			t.Errorf("%s: URL vacia dio binario vacio", so)
		}
		if len(args) != 1 || args[0] != "" {
			t.Logf("%s: browserCommand con URL vacia dio %q; la guarda va en el Cmd", so, args)
		}

		// Con espacios: un solo argumento, entero.
		conEspacios := "https://github.com/acme/proyecto/tree/mi rama"
		_, args = browserCommand(so, conEspacios)
		encontrada := 0
		for _, a := range args {
			if a == conEspacios {
				encontrada++
			}
		}
		if encontrada != 1 {
			t.Errorf("%s: una URL con espacios dio %q, y tiene que ir en un argumento "+
				"entero", so, args)
		}
	}

	// Y en el SO donde se ejecuta ahora mismo: la tabla tiene que cubrirlo, o el comando
	// devolvería un binario vacío y no abriría nada.
	if bin, args := browserCommand(runtime.GOOS, "https://x"); bin == "" || len(args) == 0 {
		t.Errorf("el SO actual (%s) no está en la tabla: bin=%q args=%q", runtime.GOOS, bin, args)
	}
}

// TestLaURLNoSeDecoraNiSeRecorta: lo que NO hace la función.
//
// Y son tres cosas que un ayudante de "abrir en el navegador" hace a menudo y que aquí no
// occan, porque cada una rompe casos reales:
//
//   - Prefijar con `https://` si falta. Una URL que ya trae su esquema —GitHub la manda
//     completa— se convertiría en `https://https://…`, que ningún abridor abre.
//   - Recortar los parámetros de tracking. Una URL con `?tab=readme` es una deepened link
//     que lleva a una sección concreta, y recortarla deja al usuario en la página entera
//     sin saber por qué.
//   - Validarla como URL. Una URL relativa —que es lo que puede dar un forge mal
//     configurado— no es un error de la función: es un dato raro que el abridor va a
//     rechazar con su propio mensaje, que es mejor que uno inventado aquí.
func TestLaURLNoSeDecoraNiSeRecorta(t *testing.T) {
	url := "https://github.com/acme/proyecto/pull/1?tab=readme#diff-123"
	_, args := browserCommand("linux", url)
	if len(args) != 1 || args[0] != url {
		t.Errorf("la URL se decoro o se recortó: %q", args)
	}

	// Una URL ya con esquema, sin duplicar.
	doble := "https://https://x"
	_, args = browserCommand("linux", doble)
	if args[0] != doble {
		t.Errorf("una URL con esquema se reescribió: %q", args[0])
	}

	// Y una relativa: se pasa tal cual, sin prefixar ni validar.
	relativa := "/acme/proyecto/pull/1"
	_, args = browserCommand("linux", relativa)
	if args[0] != relativa {
		t.Errorf("una URL relativa se decoró: %q", args[0])
	}
	// Y con un esquema raro: también tal cual. No es función de esta capa decidir qué
	// esquemas son válidos; lo que no puede hacer es ejecutar un intérprete para averiguarlo.
	raro := "file:///etc/passwd"
	bin, args := browserCommand("linux", raro)
	if args[0] != raro {
		t.Errorf("un esquema raro se reescribió: %q", args[0])
	}
	if strings.ContainsAny(bin, " \t") {
		t.Errorf("el binario trae espacios: %q", bin)
	}
}

// TestUnaURLVaciaSeDiceQueNoHayNadaYNoSeEjecutaNada: la guarda del `Cmd`.
//
// Y este es el defecto que la prueba de arriba destapó. Los dos llamadores de
// `openBrowserCmd` pueden entregar una URL vacía sin querer —el `it.URL` de un ítem cuya
// respuesta del forge vino a medias, y la ruta de la imagen de una simulación que no llegó
// a generarse— y sin guarda el `Cmd` ejecutaba `xdg-open ""`, que arranca sin hacer nada
// útil, y devolvía el aviso "abriendo ". Un toast sin nada detrás.
//
// Y el aviso tiene que decir lo que pasó: "no hay nada que abrir", no un texto vacío. Un
// aviso vacío es indistinguible de que no se tambourineó.
//
// Y esto no se puede probar mirando el `Cmd` sin ejecutarlo, porque `tea.Cmd` es una
// función: se devuelve y se llama. El aserto es sobre el `notifyMsg` que sale.
func TestUnaURLVaciaSeDiceQueNoHayNadaYNoSeEjecutaNada(t *testing.T) {
	m := newTestModel(t)
	msg, ok := m.openBrowserCmd("")().(notifyMsg)
	if !ok {
		t.Fatal("el comando con URL vacia no devolvió un notifyMsg")
	}
	if msg.level != levelWarn {
		t.Errorf("nivel %v, want aviso: abrir una URL vacía no es un error", msg.level)
	}
	if strings.TrimSpace(msg.text) == "" {
		t.Error("el aviso quedó vacío, que es indistinguible de no haber avisado")
	}
	if strings.HasPrefix(msg.text, "abriendo ") {
		t.Errorf("el aviso dice %q, que es el texto del camino bueno con una URL detrás", msg.text)
	}
	// Y con URL no se toma este camino: el texto tiene que traer la URL.
	//
	// Y aquí hay un `openURL` inyectado, y no es un detalle. La versión anterior de este test
	// ejecutaba el Cmd de verdad —`m.openBrowserCmd(url)()`— para comprobar que con URL no
	// salía el aviso de vacío, y eso hacía `exec.Command("xdg-open", url).Start()`: la suite
	// abría el navegador REAL de quien la corría, en su escritorio, con una URL de prueba.
	// Cada `make test` lo hacía, y en una máquina de CI con xdg-open abre una ventana que
	// nadie mira.
	//
	// El aserto no necesita el navegador para nada: solo necesita que el camino bueno se tome.
	// Con el seam se sigue comprobando eso y además se comprueba que la URL es la que viaja,
	// que antes no se comprobaba porque mirar el `Cmd` sin llamarlo no era posible.
	abiertas := []string{}
	m.openURL = func(url string) error {
		abiertas = append(abiertas, url)
		return nil
	}
	const url = "https://github.com/acme/proyecto/pull/1"
	conURL := m.openBrowserCmd(url)()
	if aviso, esAviso := conURL.(notifyMsg); esAviso &&
		strings.HasPrefix(aviso.text, "no hay nada") {
		t.Errorf("con URL dio el aviso de vacío: %q", aviso.text)
	}
	if len(abiertas) != 1 || abiertas[0] != url {
		t.Errorf("se abrieron %v, want exactamente [%s]", abiertas, url)
	}
	// Y el aviso del camino bueno trae la URL, que es lo que se enseña.
	if aviso, esAviso := conURL.(notifyMsg); !esAviso || aviso.text != "abriendo "+url {
		t.Errorf("el aviso del camino bueno es %v, want %q", conURL, "abriendo "+url)
	}
	if aviso, esAviso := conURL.(notifyMsg); esAviso && aviso.level != levelInfo {
		t.Errorf("abrir bien es información, no un error ni un aviso: %v", aviso.level)
	}

	// Y con URL vacía NO se abre nada, que es el otro lado de la guarda. Antes esto tampoco
	// se comprobaba, y es el aserto que importa: un `xdg-open ""` por cada popup de
	// simulación fallida es un proceso desperdiciado y un toast sin nada detrás.
	abiertas = nil
	vacia := m.openBrowserCmd("")()
	if len(abiertas) != 0 {
		t.Errorf("una URL vacía llegó al abridor: %v", abiertas)
	}
	// Y lo que sale es el aviso de warning de arriba, no un nil: un Cmd de tea siempre
	// devuelve un mensaje o nil, y nil aquí significaría que no se avisó de nada.
	if aviso, esAviso := vacia.(notifyMsg); !esAviso || aviso.level != levelWarn {
		t.Errorf("una URL vacía dio %v, want el aviso de warning", vacia)
	}

	// Y un abridor que falla avisa del fallo con su mensaje, que es lo que se necesita para
	// saber que la URL no se abrió.
	m.openURL = func(string) error { return errors.New("no hay display") }
	msg, ok = m.openBrowserCmd(url)().(notifyMsg)
	if !ok {
		t.Fatal("un abridor que falló no devolvió un notifyMsg")
	}
	if !strings.Contains(msg.text, "no hay display") {
		t.Errorf("el aviso no trae el fallo del abridor: %q", msg.text)
	}
	if msg.level != levelError {
		t.Errorf("abrir y fallar es un error (nivel %v), no un aviso: el usuario pulsing o "+
			"merece saber que no se abrió nada", msg.level)
	}
}
