// Toasts: avisos transitorios que se dibujan encima de la vista, abajo a la
// derecha, y se autodestruyen pasado su TTL.
//
// Mismo modelo que el ToastManager de dbx (pila de avisos con caducidad, tick
// propio y overlay en la esquina), con dos diferencias: el reloj es inyectable
// para poder testear la expiración sin dormir, y el ancho se recorta al ancho
// útil de la vista para no desbordar nunca laterminal.
package tui

import (
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	// toastDuration es el TTL por defecto de un aviso.
	toastDuration = 4 * time.Second
	// toastTickInterval es la resolución con la que caducan los avisos.
	toastTickInterval = 500 * time.Millisecond
	// toastMinWidth/toastMaxWidth acotan el ancho de la caja.
	toastMinWidth = 24
	toastMaxWidth = 60
)

// toastLevel es la gravedad de un aviso y decide icono y color.
type toastLevel int

const (
	toastSuccess toastLevel = iota
	toastError
	toastInfo
	toastWarning
)

// toast es un aviso con su caducidad.
type toast struct {
	message  string
	level    toastLevel
	created  time.Time
	duration time.Duration
}

// toastManager es la pila de avisos vivos.
type toastManager struct {
	toasts []toast
	// now es el reloj: inyectable para que los tests no dependan del tiempo.
	now func() time.Time
}

// newToastManager construye la pila con el reloj real.
func newToastManager() *toastManager {
	return &toastManager{now: time.Now}
}

// show añade un aviso con el TTL por defecto.
func (t *toastManager) show(message string, level toastLevel) {
	t.showFor(message, level, toastDuration)
}

// showFor añade un aviso con TTL propio.
func (t *toastManager) showFor(message string, level toastLevel, d time.Duration) {
	if message == "" {
		return
	}
	t.toasts = append(t.toasts, toast{message: message, level: level, created: t.now(), duration: d})
}

// update poda los avisos caducados. Es lo que llama el tick.
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

// texts devuelve los mensajes vivos, para los tests.
func (t *toastManager) texts() []string {
	out := make([]string, 0, len(t.toasts))
	for _, x := range t.toasts {
		out = append(out, x.message)
	}
	return out
}

// last devuelve el mensaje del último aviso, o "" si no hay ninguno.
func (t *toastManager) last() string {
	if len(t.toasts) == 0 {
		return ""
	}
	return t.toasts[len(t.toasts)-1].message
}

// blocks dibuja cada aviso vivo como una caja; es lo que se superpone.
func (t *toastManager) blocks(available int) []string {
	out := make([]string, 0, len(t.toasts))
	for _, x := range t.toasts {
		out = append(out, t.render(x, available))
	}
	return out
}

// render compone la caja de un aviso: icono, texto envuelto y borde.
func (t *toastManager) render(x toast, available int) string {
	icon := toastIcon(x.level)
	// Ancho total de la caja: el texto, el icono y el hueco de borde+padding,
	// acotado por los límites y por el espacio libre. En lipgloss Width() es el
	// ancho final (incluye borde y padding), de ahí el +5 y el -4 de abajo.
	width := ansi.StringWidth(x.message) + ansi.StringWidth(icon) + 5
	width = min(max(width, toastMinWidth), toastMaxWidth)
	if available > 8 {
		width = min(width, available)
	}
	// El texto se envuelve al ancho útil: menos borde y padding, y menos el icono
	// y su espacio en la primera línea.
	inner := max(width-4, 8)
	lines := wrapText(x.message, max(inner-2, 4))

	var b strings.Builder
	for i, line := range lines {
		if i == 0 {
			b.WriteString(styleToast(x.level).Render(icon+" "+line) + "\n")
			continue
		}
		b.WriteString(styleToast(x.level).Render("  "+line) + "\n")
	}
	// lipgloss añade el borde y el padding; el ancho se fija para que todas las
	// líneas del bloque midan lo mismo y el overlay no desalinee la vista.
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(toastBorderColor(x.level))).
		Padding(0, 1).
		Width(width).
		Render(strings.TrimRight(b.String(), "\n"))
}

// toastIcon es el glifo del nivel.
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

// toastBorderColor es el color del borde según el nivel.
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

// styleToast elige el estilo del texto del aviso.
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

// wrapText parte el texto en líneas de como mucho max columnas, por palabras.
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

// overlayToasts superpone los avisos abajo a la derecha de `content`. Se
// recortan por la derecha con ansi.Truncate/TruncateLeft para conservar los
// códigos de color de la línea base: al revés que un simple replace, el texto de
// debajo no se ensucia ni se desalinea.
//
// Solo pinta sobre las filas marcadas en `rows`: el interior de las cajas. Un
// aviso nunca cae sobre un borde, así que ningún marco se rompe por tener un
// aviso encima —que es lo que pasaba con el borde inferior del detalle—. Cada
// aviso busca el hueco más bajo que le quepa, y los siguientes se apilan por
// encima del anterior; si ya no queda interior libre, los que sobren no se
// pintan, porque un aviso ilegible no informa de nada.
func overlayToasts(content string, boxes []string, width int, rows []bool) string {
	if len(boxes) == 0 {
		return content
	}
	lines := strings.Split(content, "\n")
	anchor := len(lines) - 1
	for _, b := range boxes {
		block := strings.Split(b, "\n")
		bw := ansi.StringWidth(block[0])
		bh := min(len(block), anchor+1) // nunca más alto que lo que queda
		base, ok := landRow(rows, anchor, bh)
		if !ok {
			break
		}
		x := max(width-bw-1, 0)
		for j := range bh {
			i := base - bh + 1 + j
			line := lines[i]
			lines[i] = ansi.Truncate(line, x, "") + block[j] + ansi.TruncateLeft(line, x+bw, "")
		}
		anchor = base - bh
	}
	return strings.Join(lines, "\n")
}

// landRow devuelve la fila más baja, sin pasar de anchor, en la que cabe un
// bloque de bh filas que admiten avisos. Devuelve false si no cabe en ninguna.
func landRow(rows []bool, anchor, bh int) (int, bool) {
	if bh <= 0 {
		return 0, false
	}
	for base := min(anchor, len(rows)-1); base >= bh-1; base-- {
		if admitenAviso(rows, base-bh+1, bh) {
			return base, true
		}
	}
	return 0, false
}

// admitenAviso indica si las n filas que empiezan en from son todas interior de
// alguna caja. Una fila que no existe cuenta como que no: es preferible no
// pintar a pintar de más.
func admitenAviso(rows []bool, from, n int) bool {
	for i := from; i < from+n; i++ {
		if i < 0 || i >= len(rows) || !rows[i] {
			return false
		}
	}
	return true
}
