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
func (m *Model) openBrowserCmd(url string) tea.Cmd {
	return func() tea.Msg {
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
