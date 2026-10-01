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
	// No cabe en vertical: se recorta por abajo, que es lo que hace un terminal con una
	// ventana más alta que la pantalla. El recorte va con `min` y no con una pregunta
	// porque no hay pregunta que hacer: quedarse con el más corto de los dos es lo mismo
	// que preguntar cuál es más corto, sin la rama que la pregunta deja detrás.
	block = block[:min(len(block), len(lines))]
	bh := len(block)

	x, y := centeredOrigin(width, len(lines), ansi.StringWidth(block[0]), bh)

	// i no puede salirse. Arriba bh <= len(lines) por el recorte, y
	// centeredOrigin deja y <= len(lines)-bh, así que y+bh <= len(lines) y
	// j < bh da i < len(lines). El caso límite (i == len(lines)) necesitaría un
	// corte por la derecha, que es el otro eje: aquí la caja se pinta entera por
	// arriba y lo que se sale es lo que hay alrededor.
	for j := range bh {
		i := y + j
		cell := ansi.Truncate(block[j], width, "")
		cw := ansi.StringWidth(cell)
		line := lines[i]
		lines[i] = ansi.Truncate(line, x, "") + cell + ansi.TruncateLeft(line, x+cw, "")
	}
	return strings.Join(lines, "\n")
}

// centeredOrigin es la esquina superior izquierda de una caja de boxW × boxH
// centrada en un área de width × height.
//
// Vive aparte porque dos cosas la necesitan y tienen que coincidir: el overlay que
// recorta la vista y la capa de gráficos que coloca la imagen. Si cada una calculara
// su sitio, el marco y la imagen caerían en rectángulos distintos —y solo se vería
// cuando coinciden, que es lo peor que puede pasarle a un error de posición—.
//
// Las dos coordenadas son el mismo reparto con un suelo a cero, y el suelo es lo que
// hace el trabajo entero: una caja más grande que el área no puede centrarse, así que
// se pega a la esquina.
//
// Y aquí estaba el código muerto de la tanda. Eran tres líneas y un `if`:
//
//	y = max(0, (height-boxH)/2)
//	if y+boxH > height {
//		y = max(0, height-boxH)
//	}
//
// El `if` no podía activarse NUNCA. Con la caja metida en el área, la y sale a
// `(height-boxH)/2` y `(height-boxH)/2 + boxH` es `(height+boxH)/2`, que es menor o
// igual que el área justo cuando `boxH <= height`, que es el supuesto del caso. Y con
// la caja más alta, `height-boxH` es negativo, la y sale a cero por el suelo y el
// `if` la deja como estaba. O sea que el `if` solo podía activarse en el caso que el
// suelo ya cubría, y su cuerpo volvía a poner el suelo.
//
// Probado quitándolo: la suite sigue verde con la y del `if` forzada a la mitad del
// área, y eso es que el `if` no decide nada.
func centeredOrigin(width, height, boxW, boxH int) (x, y int) {
	x = max(0, (width-boxW)/2)
	y = max(0, (height-boxH)/2)
	return x, y
}
