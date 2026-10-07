package tui

import (
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	toastDuration     = time.Duration(4e9)
	toastTickInterval = time.Duration(500e6)
	toastMinWidth     = 24
	toastMaxWidth     = 60
	toastFrame        = 4
	toastIconGap      = 2
	toastIconSpace    = 1
	toastMinInner     = 8
	toastMinAvailable = 8
)

type toastLevel int

const (
	toastSuccess toastLevel = iota
	toastError
	toastInfo
	toastWarning
)

type toast struct {
	message  string
	level    toastLevel
	created  time.Time
	duration time.Duration
}

type toastManager struct {
	toasts []toast
	now    func() time.Time
}

func newToastManager() *toastManager {
	return &toastManager{now: time.Now}
}

func (t *toastManager) show(message string, level toastLevel) {
	t.showFor(message, level, toastDuration)
}

func (t *toastManager) showFor(message string, level toastLevel, d time.Duration) {
	if message == "" {
		return
	}
	t.toasts = append(t.toasts, toast{message: message, level: level, created: t.now(), duration: d})
}

func (t *toastManager) replace(prev, message string, level toastLevel) {
	if message == "" {
		return
	}
	for i := len(t.toasts) - 1; i >= 0; i-- {
		if t.toasts[i].message == prev {
			t.toasts[i] = toast{message: message, level: level, created: t.now(), duration: toastDuration}
			return
		}
	}
	t.show(message, level)
}

func (t *toastManager) update() {
	now := t.now()
	alive := t.toasts[:0]
	for _, x := range t.toasts {
		if now.Sub(x.created) < x.duration {
			alive = append(alive, x)
		}
	}
	t.toasts = alive
}

func (t *toastManager) texts() []string {
	out := make([]string, 0, len(t.toasts))
	for _, x := range t.toasts {
		out = append(out, x.message)
	}
	return out
}

func (t *toastManager) last() string {
	if len(t.toasts) == 0 {
		return ""
	}
	return t.toasts[len(t.toasts)-1].message
}

func (t *toastManager) blocks(available int) []string {
	out := make([]string, 0, len(t.toasts))
	for _, x := range t.toasts {
		out = append(out, t.render(x, available))
	}
	return out
}

func (t *toastManager) render(x toast, available int) string {
	icon := toastIcon(x.level)
	width, wrapAt := toastGeometry(x.message, icon, available)
	lines := wrapText(x.message, wrapAt)

	var b strings.Builder
	for i, line := range lines {
		if i == 0 {
			b.WriteString(styleToast(x.level).Render(icon+" "+line) + "\n")
			continue
		}
		b.WriteString(styleToast(x.level).Render("  "+line) + "\n")
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(toastBorderColor(x.level))).
		Padding(0, 1).
		Width(width).
		Render(strings.TrimRight(b.String(), "\n"))
}

func toastGeometry(message, icon string, available int) (width, wrapAt int) {
	width = ansi.StringWidth(message) + ansi.StringWidth(icon) + toastIconSpace + toastFrame
	width = min(max(width, toastMinWidth), toastMaxWidth)

	if available > toastMinAvailable {
		width = min(width, available)
	}

	inner := max(width-toastFrame, toastMinInner)
	return width, inner - toastIconGap
}

func toastIcon(level toastLevel) string {
	switch level {
	case toastSuccess:
		return "✓"
	case toastError:
		return "✗"
	case toastInfo:
		return "ℹ"
	case toastWarning:
		return "⚠"
	default:
		return "•"
	}
}

func toastBorderColor(level toastLevel) string {
	switch level {
	case toastSuccess:
		return "46"
	case toastError:
		return "196"
	case toastInfo:
		return "39"
	case toastWarning:
		return "208"
	default:
		return "244"
	}
}

func styleToast(level toastLevel) lipglossStyle {
	switch level {
	case toastSuccess:
		return styleOK
	case toastError:
		return styleError
	case toastInfo:
		return styleInfo
	case toastWarning:
		return styleWarn
	default:
		return styleDim
	}
}

func wrapText(text string, max int) []string {
	if max <= 0 || ansi.StringWidth(text) <= max {
		return []string{text}
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{text}
	}
	lines := []string{words[0]}
	for _, w := range words[1:] {
		last := len(lines) - 1
		if ansi.StringWidth(lines[last])+1+ansi.StringWidth(w) <= max {
			lines[last] += " " + w
			continue
		}
		lines = append(lines, w)
	}
	return lines
}

func overlayToasts(content string, boxes []string, width int, rows []bool) string {
	if len(boxes) == 0 {
		return content
	}
	lines := strings.Split(content, "\n")
	anchor := len(lines) - 1
	for _, b := range boxes {
		block := strings.Split(b, "\n")
		bw := ansi.StringWidth(block[0])
		bh := toastBlockHeight(len(block), anchor)
		base, ok := landRow(rows, anchor, bh)
		if !ok {
			break
		}
		x := toastColumn(width, bw)
		for j := range bh {
			i := base - bh + 1 + j
			line := lines[i]
			lines[i] = ansi.Truncate(line, x, "") + block[j] + ansi.TruncateLeft(line, x+bw, "")
		}
		anchor = base - bh
	}
	return strings.Join(lines, "\n")
}

func toastBlockHeight(bh, anchor int) int { return min(bh, anchor+1) }

func toastColumn(width, bw int) int { return max(width-bw-1, 0) }

func landRow(rows []bool, anchor, bh int) (int, bool) {
	if bh <= 0 {
		return 0, false
	}
	for base := anchor; base >= bh-1; base-- {
		if admitsWarning(rows, base-bh+1, bh) {
			return base, true
		}
	}
	return 0, false
}

func admitsWarning(rows []bool, from, n int) bool {
	for i := from; i < from+n; i++ {
		if i < 0 || i >= len(rows) || !rows[i] {
			return false
		}
	}
	return true
}
