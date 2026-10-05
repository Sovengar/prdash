// Toasts: transient warnings drawn over the view at the bottom right, self-destructing after their
// TTL. Same model as dbx's ToastManager, with an injectable clock and a width clipped to the view.
package tui

import (
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	toastDuration     = 4 * time.Second
	toastTickInterval = 500 * time.Millisecond
	toastMinWidth     = 24
	toastMaxWidth     = 60
	// lipgloss Width() is the CONTENT width, so this is what has to be subtracted to know how much
	// text fits.
	toastFrame = 4
	// Only on the first line, which is why the text wraps at the usable width minus this. The icon's
	// width is measured, so a wider icon tomorrow still lets the text fit.
	toastIconGap = 2
	// Named apart from toastIconGap because they are different things: the icon's width, which depends
	// on the level, and a fixed gap. Summed into a single 2, a wide icon would push nothing.
	toastIconSpace = 1
	// Below 8 columns of text nothing is readable, and an unreadable warning says nothing.
	toastMinInner = 8
	// Below this the overlay would clip the box on the right, so it is more honest not to pretend it
	// fits.
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

// Not finding one (it expired or another replaced it) stacks the new one, which is what lets a
// warning be composed on instead of duplicated.
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
	// All three numbers come from toastGeometry, which is its own function because inside the
	// painting a miscalculated width looks like "the warning takes more rows", not like a bad width.
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
	// The width is fixed so every line of the block measures the same and the overlay does not
	// misalign the view.
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(toastBorderColor(x.level))).
		Padding(0, 1).
		Width(width).
		Render(strings.TrimRight(b.String(), "\n"))
}

// The width is an ESTIMATE, deliberately: the wrapper guarantees the text fits, and an
// over-estimate wraps into more lines, which beats a frame clipping a word in half.
func toastGeometry(message, icon string, available int) (width, wrapAt int) {
	// Every term is a named piece and the sum is the contract: a missing or extra term makes the warning
	// overflow or narrow, and from the overlaid text you cannot tell that from the overlay clipping it.
	width = ansi.StringWidth(message) + ansi.StringWidth(icon) + toastIconSpace + toastFrame
	width = min(max(width, toastMinWidth), toastMaxWidth)

	// Below this the box is not even attempted: a box wider than the column it lands in gets clipped on
	// the right, and such a narrow column does not fit a minimum-width warning.
	if available > toastMinAvailable {
		width = min(width, available)
	}

	// No floor on the wrap width, and that is a conclusion: inner >= 8, so inner - toastIconGap >= 6,
	// and a floor of 4 could never bind. A floor that cannot bind invites tests through it.
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

// Only on the interior rows marked in `rows`: a warning on a border breaks a frame. When no
// free interior is left the rest are dropped, because an unreadable warning says nothing.
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

// The cap is anchor+1: the last row is the anchor, so anchor+1 rows fit from it upwards. No zero
// floor, because a floor that cannot bind is noise that hides the +1 that does matter.
func toastBlockHeight(bh, anchor int) int { return min(bh, anchor+1) }

// Pinned to the right with ONE column of air: a warning reaching the last column reads as part
// of the frame. Its own function because the overlay cannot show a one-column overflow.
func toastColumn(width, bw int) int { return max(width-bw-1, 0) }

// The loop does NOT clamp the anchor: the only caller passes len(lines)-1 over rows built
// alongside, and the anchor goes DOWN, so a clamp could never fire and hid the step upwards.
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

// A row that does not exist counts as not admitting one: not painting is better than painting
// too much.
func admitsWarning(rows []bool, from, n int) bool {
	for i := from; i < from+n; i++ {
		if i < 0 || i >= len(rows) || !rows[i] {
			return false
		}
	}
	return true
}
