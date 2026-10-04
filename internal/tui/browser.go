package tui

import (
	"os/exec"
	"runtime"

	tea "charm.land/bubbletea/v2"
)

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

// Launched and detached on purpose: killing the opener on TUI exit would close the browser with it.
// The empty URL is rejected before anything else, because `xdg-open ""` starts successfully and
// the toast would say "opening" with nothing behind it.
func (m *Model) openBrowserCmd(url string) tea.Cmd {
	return func() tea.Msg {
		if url == "" {
			return notifyMsg{text: "no hay nada que abrir en este item", level: levelWarn}
		}
		// The double is checked before LookPath on purpose: a double that only replaced Start would
		// still depend on the machine having an xdg-open, which a headless CI box does not.
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
