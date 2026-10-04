package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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

// The REAL paths of openBrowserCmd: the ones that do not go through the openURL double.

// The PATH is emptied, so nothing is opened.
func TestSinAbridorEnElPathSeAviadoYNoSeAbreNada(t *testing.T) {
	vacio := t.TempDir()
	t.Setenv("PATH", vacio)

	m := newTestModel(t)
	// And without the seam: the real path, the one production would run.
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
	if !strings.Contains(msg.text, "xdg-open") && !strings.Contains(msg.text, "open") &&
		!strings.Contains(msg.text, "rundll32") {
		t.Errorf("el aviso %q no dice qué abridor falta", msg.text)
	}
	if strings.Contains(msg.text, "no encontrado") == false &&
		!strings.Contains(strings.ToLower(msg.text), "not found") {
		t.Errorf("el aviso %q no dice que no se encontró", msg.text)
	}
	// And it does not say "opening": with the good path's text the user would think it opened.
	if strings.HasPrefix(msg.text, "abriendo") {
		t.Errorf("sin abridor el aviso dice %q", msg.text)
	}
	entradas, err := os.ReadDir(vacio)
	if err != nil {
		t.Fatal(err)
	}
	if len(entradas) != 0 {
		t.Errorf("el PATH vacío tiene %d entradas: se llegó a ejecutar algo", len(entradas))
	}
}

// LookPath looks at the execute bit and NOT the shebang, so the warning must not say "not found":
// the user would go install xdg-utils when the interpreter is what is missing.
func TestUnAbridorQueNoSePuedeEjecutarSeAviadoConSuError(t *testing.T) {
	binDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, "xdg-open"),
		[]byte("#!/interprete-que-no-existe\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	m := newTestModel(t)
	if m.openURL != nil {
		t.Fatal("el fixture no sirve: el seam sigue puesto y no se prueba el camino real")
	}

	msg, ok := m.openBrowserCmd("https://ejemplo/x")().(notifyMsg)
	if !ok {
		t.Fatal("un abridor que no arranca no devolvió un notifyMsg")
	}
	// The level is ERROR and not a warning on purpose: the opener exists and does not start.
	if msg.level != levelError {
		t.Errorf("nivel %v, want error: el abridor existe y no arranca", msg.level)
	}
	// The warning does NOT say "not found": LookPath DID find it —it looks at the execute bit— and
	// the failure arrives in Start.
	if strings.Contains(msg.text, "no encontrado") {
		t.Errorf("el aviso dice que no se encontró el abridor, pero `LookPath` sí lo encontró: "+
			"el fallo es de arranque, no de búsqueda. Aviso: %q", msg.text)
	}
	// And it carries Start's cause, which is the only thing that tells a broken shebang from a
	// permission problem.
	if !strings.Contains(msg.text, "abrir navegador") {
		t.Errorf("el aviso %q no dice que es del abridor", msg.text)
	}
	if len(strings.TrimSpace(msg.text)) <= len("abrir navegador: ") {
		t.Errorf("el aviso %q no trae la causa del fallo de Start", msg.text)
	}
}

// With the guards in the other order, an empty URL on a machine with no opener would report the
// environment instead of having nothing to open.
func TestLaURLVaciaSeCompruebaAntesDeMirarElEntorno(t *testing.T) {
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
	// And with the opener present, the same warning: the result does not depend on the machine.
	t.Setenv("PATH", os.Getenv("PATH"))
	if got := m.openBrowserCmd("")(); !strings.Contains(got.(notifyMsg).text, "no hay nada") {
		t.Errorf("con abridor el aviso de URL vacía cambió: %q", got)
	}
}

// The boundary with the shell.
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
	const url = `https://host/p; echo pwned?a=1&b="2" '3'`
	msg, ok := m.openBrowserCmd(url)().(notifyMsg)
	if !ok {
		t.Fatalf("no se devolvió un notifyMsg: %v", msg)
	}
	if msg.level != levelInfo || !strings.Contains(msg.text, url) {
		t.Errorf("el camino bueno no se tomó: %+v", msg)
	}

	// The opener got the URL as ONE argument, with the brackets the script adds.
	crudo, err := esperaAQueExista(log, 2*time.Second)
	if err != nil {
		t.Fatalf("el abridor no llegó a ejecutarse: %v", err)
	}
	linea := strings.TrimSpace(string(crudo))
	if linea != "["+url+"]" {
		t.Errorf("el abridor recibió %q, want un solo argumento con la URL entera", linea)
	}
}
