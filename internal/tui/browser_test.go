package tui

import (
	"errors"
	"runtime"
	"strings"
	"testing"
)

// The table is pinned per platform, not taken from the system.
func TestElAbridorSeEligePorPlataformaYNoEsElDelSistemaEntero(t *testing.T) {
	url := "https://github.com/acme/proyecto/pull/1"

	casos := []struct {
		so       string
		wantBin  string
		wantArgs []string
	}{
		{"linux", "xdg-open", []string{url}},
		{"darwin", "open", []string{url}},
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

	// And what cannot happen on any platform: an interpreter with the URL as code.
	prohibidos := []string{"sh", "bash", "zsh", "cmd", "cmd.exe", "powershell", "pwsh"}
	for _, so := range []string{"linux", "darwin", "windows", "freebsd", "plan9", ""} {
		bin, args := browserCommand(so, url)
		for _, mal := range prohibidos {
			if bin == mal {
				t.Errorf("%s: usa %q, que interpretaría la URL", so, bin)
			}
		}
		// The URL always travels as its own argument, never glued to a -c.
		for _, a := range args {
			switch a {
			case "-c", "-Command", "/c":
				t.Errorf("%s: pasa %q, que interpretaría lo que viene detrás", so, a)
			}
		}
		if len(args) == 0 {
			t.Errorf("%s: sin argumentos, no abriría nada", so)
		}
		if args[len(args)-1] != url {
			t.Errorf("%s: el ultimo argumento no es la URL: %q", so, args)
		}
	}
}

// The two boundaries of the input.
func TestUnaURLVaciaNoSeAbreYUnaConEspaciosNoSeParte(t *testing.T) {
	for _, so := range []string{"linux", "darwin", "windows"} {
		bin, args := browserCommand(so, "")
		if bin == "" {
			t.Errorf("%s: URL vacia dio binario vacio", so)
		}
		if len(args) != 1 || args[0] != "" {
			t.Logf("%s: browserCommand con URL vacia dio %q; la guarda va en el Cmd", so, args)
		}

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

	// And on the OS it is running on right now: the table has to cover it.
	if bin, args := browserCommand(runtime.GOOS, "https://x"); bin == "" || len(args) == 0 {
		t.Errorf("el SO actual (%s) no está en la tabla: bin=%q args=%q", runtime.GOOS, bin, args)
	}
}

// The three things the function does NOT do.
func TestLaURLNoSeDecoraNiSeRecorta(t *testing.T) {
	url := "https://github.com/acme/proyecto/pull/1?tab=readme#diff-123"
	_, args := browserCommand("linux", url)
	if len(args) != 1 || args[0] != url {
		t.Errorf("la URL se decoro o se recortó: %q", args)
	}

	doble := "https://https://x"
	_, args = browserCommand("linux", doble)
	if args[0] != doble {
		t.Errorf("una URL con esquema se reescribió: %q", args[0])
	}

	relativa := "/acme/proyecto/pull/1"
	_, args = browserCommand("linux", relativa)
	if args[0] != relativa {
		t.Errorf("una URL relativa se decoró: %q", args[0])
	}
	// And with an odd scheme: as given. Deciding what is valid is not this layer's job.
	raro := "file:///etc/passwd"
	bin, args := browserCommand("linux", raro)
	if args[0] != raro {
		t.Errorf("un esquema raro se reescribió: %q", args[0])
	}
	if strings.ContainsAny(bin, " \t") {
		t.Errorf("el binario trae espacios: %q", bin)
	}
}

// This is the default.
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
	// With a URL this path is not taken: the text has to carry the URL.
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
	if aviso, esAviso := conURL.(notifyMsg); !esAviso || aviso.text != "abriendo "+url {
		t.Errorf("el aviso del camino bueno es %v, want %q", conURL, "abriendo "+url)
	}
	if aviso, esAviso := conURL.(notifyMsg); esAviso && aviso.level != levelInfo {
		t.Errorf("abrir bien es información, no un error ni un aviso: %v", aviso.level)
	}

	// And with an empty URL nothing is opened, which is the other side of the guard.
	abiertas = nil
	vacia := m.openBrowserCmd("")()
	if len(abiertas) != 0 {
		t.Errorf("una URL vacía llegó al abridor: %v", abiertas)
	}
	// What comes out is the warning above, not a nil: a tea Cmd is always non-nil.
	if aviso, esAviso := vacia.(notifyMsg); !esAviso || aviso.level != levelWarn {
		t.Errorf("una URL vacía dio %v, want el aviso de warning", vacia)
	}

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
