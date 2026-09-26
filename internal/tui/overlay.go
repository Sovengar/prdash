// Superposición de cajas sobre la vista.
//
// Es la misma técnica que usan los toasts (ver overlayToasts): la vista de
// fondo no se vuelve a componer, se recorta por donde hace falta el texto de la
// caja. Recortar con ansi.Truncate/TruncateLeft en vez de empalmar por índices
// conserva los códigos de color de la línea base: al revés, lo que quede a la
// derecha saldría con el color de primer plano del terminal.
package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// overlayCentered dibuja box centrada sobre content, en un viewport de width
// columnas por las líneas que tenga.
//
// A diferencia de los toasts, aquí sí se recorta lo que se salga por arriba o por
// abajo: un popup es una ventanamodal y el borde le importa. Lo que no se recorta
// es el fondo: fuera de la caja sigue viéndose la vista entera, que es el punto
// del popup.
func overlayCentered(content, box string, width int) string {
	if box == "" {
		return content
	}
	lines := strings.Split(content, "\n")
	block := strings.Split(box, "\n")
	if len(block) > len(lines) {
		// No cabe en vertical: se recorta por abajo, que es lo que hace un
		// terminal con una ventana más alta que la pantalla.
		block = block[:len(lines)]
	}
	bh := len(block)

	y := max(0, (len(lines)-bh)/2)
	x := max(0, (width-ansi.StringWidth(block[0]))/2)

	for j := range bh {
		i := y + j
		if i >= len(lines) {
			break
		}
		cell := ansi.Truncate(block[j], width, "")
		cw := ansi.StringWidth(cell)
		line := lines[i]
		lines[i] = ansi.Truncate(line, x, "") + cell + ansi.TruncateLeft(line, x+cw, "")
	}
	return strings.Join(lines, "\n")
}
