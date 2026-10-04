// Apertura de URLs en el navegador del sistema.
package tui

import (
	"os/exec"
	"runtime"

	tea "charm.land/bubbletea/v2"
)

// browserCommand elige el abridor de URLs según la plataforma: xdg-open en
// Linux, open en macOS y el manejador de protocolo de Windows.
func browserCommand(goos, url string) (string, []string) {
	switch goos {
	case "darwin":
		return "open", []string{url}
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}
	default:
		return "xdg-open", []string{url}
	}
}

// openBrowserCmd abre una URL sin bloquear la UI. Es tolerante a la ausencia
// del binario (aviso, sin romper). No usa timeout porque el abridor es de vida
// independiente: matarlo al volver de la TUI cerraría el navegador; se lanza y
// se desliga del proceso.
//
// Y la URL vacía se trata antes de salir, porque los dos llamadores pueden
// entregarla sin querer: el `it.URL` de un ítem cuya respuesta del forge vino a
// medias, y la ruta de la imagen de una simulación que no llegó a generarse.
// Sin esta guarda, `xdg-open ""` arranca sin hacer nada útil y el aviso dice
// "abriendo " —un toast sin nada detrás, que es peor que un aviso que dice lo
// que pasó—.
func (m *Model) openBrowserCmd(url string) tea.Cmd {
	return func() tea.Msg {
		if url == "" {
			return notifyMsg{text: "no hay nada que abrir en este item", level: levelWarn}
		}
		// El seam primero: si hay doble, no se busca ni se lanza nada. Está antes del
		// `LookPath` a propósito —un doble que solo sustituye al `Start` dejaría el test
		// depending del `xdg-open` de la máquina, que en un contenedor sin escritorio no
		// existe y cambiaría lo que el test demuestra—.
		if m.openURL != nil {
			if err := m.openURL(url); err != nil {
				return notifyMsg{text: "abrir navegador: " + err.Error(), level: levelError}
			}
			return notifyMsg{text: "abriendo " + url, level: levelInfo}
		}
		bin, args := browserCommand(runtime.GOOS, url)
		if _, err := exec.LookPath(bin); err != nil {
			return notifyMsg{text: "abrir navegador: " + bin + " no encontrado", level: levelWarn}
		}
		cmd := exec.Command(bin, args...)
		if err := cmd.Start(); err != nil {
			return notifyMsg{text: "abrir navegador: " + err.Error(), level: levelError}
		}
		_ = cmd.Process.Release()
		return notifyMsg{text: "abriendo " + url, level: levelInfo}
	}
}
