// Package bordered dibuja cajas con borde redondeado y título embebido en la
// línea superior. El contenido se recorta (ANSI-aware) al ancho interior en
// lugar de re-envolverse: el wrap rompería el alto calculado por el layout.
package bordered

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Alineación del título dentro de la línea de borde.
const (
	AlignLeft = iota
	AlignCenter
	AlignRight
)

// Rounded es el borde redondeado (╭ ╮ ╰ ╯) por defecto.
func Rounded() lipgloss.Border { return lipgloss.RoundedBorder() }

// RenderWithTitle dibuja un borde con título arriba alineado a la izquierda y
// sin leyenda inferior.
func RenderWithTitle(border lipgloss.Border, borderFg color.Color, title, content string, width int) string {
	return RenderWithTitles(border, borderFg, title, AlignLeft, "", AlignLeft, content, width)
}

// RenderWithTitles dibuja un borde con título en la línea superior y una
// leyenda opcional en la inferior, cada uno con su alineación. Un título vacío
// deja la línea completa de relleno. El ancho exterior es width (mínimo 2) y el
// interior width-2; cada línea de contenido se recorta al interior.
func RenderWithTitles(border lipgloss.Border, borderFg color.Color, topTitle string, topAlign int, bottomTitle string, bottomAlign int, content string, width int) string {
	// El suelo a dos, y no la comprobación de antes (`if width < 2 { width = 2 }`):
	// el mínimo es un máximo, no una rama. Con la condición, el borde de la condición
	// —el ancho exactamente dos— la hacía indistinguible de `width <= 2`, porque las
	// dos dejan el ancho en dos y no hay entrada que las separe. Con `max` no hay
	// comparación que mutar en la frontera: o el ancho es dos o es mayor.
	//
	// El motivo del mínimo es el mismo de antes y no cambia: por debajo de dos no
	// caben los dos bordes, y el suelo es lo que permite al layout no tener que
	// comprobar nada antes de pintar.
	width = max(2, width)

	innerWidth := width - 2 // width >= 2 → nunca negativo

	var style *ansi.Style
	if borderFg != nil {
		s := ansi.NewStyle().ForegroundColor(borderFg)
		style = &s
	}

	var b strings.Builder
	b.WriteString(borderLine(style, border.TopLeft, border.Top, border.TopRight, innerWidth, topAlign, topTitle))
	b.WriteString("\n")
	for _, line := range contentLines(style, border.Left, border.Right, content, innerWidth) {
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString(borderLine(style, border.BottomLeft, border.Bottom, border.BottomRight, innerWidth, bottomAlign, bottomTitle))
	return b.String()
}

// borderLine compone una línea de borde con el título embebido y alineado.
func borderLine(style *ansi.Style, left, fill, right string, innerWidth, align int, title string) string {
	if fill == "" {
		fill = " "
	}
	// El título se recorta SIEMPRE, sin preguntar. `ansi.Truncate` es identidad
	// visible cuando el texto ya cabe, así que la pregunta `titleWidth > innerWidth`
	// solo servía para tener una rama que mutar en la frontera —y en esa frontera, con
	// el título midiendo justo el interior, truncar y no truncar dan el mismo texto y
	// el mismo ancho—.
	//
	// Se recorta sin condición y el ancho se vuelve a MEDIR en vez de darse por bueno:
	// medir es lo que hace que el reparto del relleno de abajo sea el correcto, y
	// `titleWidth = innerWidth` a secas mentía si el truncado hubiera dejado el título
	// en menos de lo que el interior da.
	title = ansi.Truncate(title, innerWidth, "")
	titleWidth := ansi.StringWidth(title)

	remaining := innerWidth - titleWidth
	var leftPad, rightPad int
	switch align {
	case AlignRight:
		leftPad = remaining
	case AlignCenter:
		leftPad = remaining / 2
		rightPad = remaining - leftPad
	default:
		rightPad = remaining
	}

	return styled(style, left) +
		styled(style, strings.Repeat(fill, leftPad)) +
		styled(style, title) +
		styled(style, strings.Repeat(fill, rightPad)) +
		styled(style, right)
}

// contentLines recorta cada línea de contenido al ancho interior y le añade
// los bordes laterales. Contenido vacío produce una línea en blanco interior.
func contentLines(style *ansi.Style, leftChar, rightChar, content string, innerWidth int) []string {
	if leftChar == "" {
		leftChar = " "
	}
	if rightChar == "" {
		rightChar = " "
	}

	raw := strings.Split(content, "\n") // siempre >= 1 elemento
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		// Recorta y rellena SIN preguntar, por la misma razón que el título: las dos
		// preguntas —`w > innerWidth` y `pad > 0`— eran identidades en su frontera.
		// Recortar un texto que ya cabe no lo cambia, y rellenar con cero espacios no
		// añade nada. Y el relleno se puede pedir sin condiciones porque el recorte
		// anterior garantiza que la diferencia nunca es negativa: `strings.Repeat` con
		// un número negativo es un PANIC, así que la guarda no era decorativa, era lo
		// que impedía el panic. Lo que la impide ahora es el orden de las dos líneas.
		line = ansi.Truncate(line, innerWidth, "")
		line += strings.Repeat(" ", innerWidth-ansi.StringWidth(line))
		lines = append(lines, styled(style, leftChar)+line+styled(style, rightChar))
	}
	return lines
}

// styled envuelve el texto en el estilo ANSI del borde (si lo hay).
func styled(style *ansi.Style, s string) string {
	if style == nil {
		return s
	}
	return style.Styled(s)
}
