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

func (m *Model) openBrowserCmd(url string) tea.Cmd {
	return func() tea.Msg {
		if url == "" {
			return notifyMsg{text: "nothing to open in this item", level: levelWarn}
		}
		if m.openURL != nil {
			if err := m.openURL(url); err != nil {
				return notifyMsg{text: "open browser: " + err.Error(), level: levelError}
			}
			return notifyMsg{text: "opening " + url, level: levelInfo}
		}
		bin, args := browserCommand(runtime.GOOS, url)
		if _, err := exec.LookPath(bin); err != nil {
			return notifyMsg{text: "open browser: " + bin + " not found", level: levelWarn}
		}
		cmd := exec.Command(bin, args...)
		if err := cmd.Start(); err != nil {
			return notifyMsg{text: "open browser: " + err.Error(), level: levelError}
		}
		_ = cmd.Process.Release()
		return notifyMsg{text: "opening " + url, level: levelInfo}
	}
}
