package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// What is asserted here is NOT visible from the painted box: a column more or less does not show.
func TestToastGeometryAcotaElAncho(t *testing.T) {
	icon := toastIcon(toastInfo)

	for _, mensaje := range []string{"", "hola", strings.Repeat("x", 40), strings.Repeat("y", 200)} {
		for _, available := range []int{0, 5, 9, 20, 50, 100, 1000} {
			width, wrapAt := toastGeometry(mensaje, icon, available)

			if width > toastMaxWidth {
				t.Errorf("mensaje de %d columnas con %d disponibles dio un ancho de %d, want <= %d",
					len(mensaje), available, width, toastMaxWidth)
			}
			// The free space OVERRIDES the minimum, not the other way round: with 9 free columns the box is 9 even
			//if the minimum is larger.
			if available > toastMinAvailable && width > available {
				t.Errorf("con %d columnas libres la caja mide %d: desborda", available, width)
			}
			if available >= toastMinWidth && width < toastMinWidth {
				t.Errorf("con %d columnas libres la caja mide %d, por debajo del mínimo %d",
					available, width, toastMinWidth)
			}

			// The wrap width is EXACTLY the usable width minus the icon gap, with no floor of its own, which is
			//why the minimum that comes out is 6 (8-2) and not 4.
			want := max(width-toastFrame, toastMinInner) - toastIconGap
			if wrapAt != want {
				t.Errorf("con un ancho de %d el texto se parte a %d, want %d (el útil menos el hueco del icono)",
					width, wrapAt, want)
			}
			if wrapAt < toastMinInner-toastIconGap {
				t.Errorf("con un ancho de %d el texto se parte a %d, por debajo de %d: sale en trozos ilegibles",
					width, wrapAt, toastMinInner-toastIconGap)
			}
		}
	}
}

// A warning that covers the view hides what the user came to look at.
func TestToastGeometryRespetaElEspacioLibre(t *testing.T) {
	icon := toastIcon(toastInfo)
	mensaje := strings.Repeat("x", 30)

	for _, available := range []int{30, 40, 50, 59} {
		width, _ := toastGeometry(mensaje, icon, available)
		if width > available {
			t.Errorf("con %d columnas libres la caja mide %d: invade la vista", available, width)
		}
	}

	debajo, _ := toastGeometry(mensaje, icon, 0)
	for _, available := range []int{-5, 0, 1, toastMinAvailable} {
		width, _ := toastGeometry(mensaje, icon, available)
		if width != debajo {
			t.Errorf("con %d columnas libres dio un ancho de %d y sin hueco da %d: por debajo del mínimo %d no debería cambiar",
				available, width, debajo, toastMinAvailable)
		}
	}

	// The floor holds even with the narrowest box there is: the text never wraps below 6 columns.
	for available := -5; available <= toastMinWidth; available++ {
		if _, wrapAt := toastGeometry(mensaje, icon, available); wrapAt < toastMinInner-toastIconGap {
			t.Errorf("con %d columnas libres el texto se parte a %d, want >= %d",
				available, wrapAt, toastMinInner-toastIconGap)
		}
	}

	justo, _ := toastGeometry(mensaje, icon, toastMinAvailable+1)
	if justo == debajo {
		t.Errorf("con %d columnas libres dio el mismo ancho que sin hueco: el umbral no está donde dice",
			toastMinAvailable+1)
	}
}

func TestToastGeometryElTextoCabeEnLaCaja(t *testing.T) {
	icon := toastIcon(toastInfo)
	for longitud := 0; longitud <= toastMaxWidth; longitud++ {
		mensaje := ""
		for i := 0; i < longitud; i++ {
			if i > 0 {
				mensaje += " "
			}
			mensaje += "z"
		}
		width, wrapAt := toastGeometry(mensaje, icon, 1000)

		for _, linea := range wrapText(mensaje, wrapAt) {
			if len(linea) > wrapAt {
				t.Errorf("un mensaje de %d columnas dio una línea de %d partiéndose a %d",
					longitud, len(linea), wrapAt)
			}
			// The icon's width is measured in COLUMNS, not len(), which would count its three bytes.
			if ansi.StringWidth(icon)+1+len(linea) > width-toastFrame {
				t.Errorf("un mensaje de %d columnas dio una primera línea de %d más el icono en una caja de %d",
					longitud, len(linea), width)
			}
		}
		// The box is painted at exactly the width the geometry says. The border is three-byte characters,
		//so len() counts bytes and a 24-wide box measures 72.
		for _, linea := range strings.Split(stripANSI(renderToast(t, mensaje, 1000)), "\n") {
			if ancho := ansi.StringWidth(linea); ancho != width {
				t.Errorf("un mensaje de %d columnas dio una línea de %d en una caja de %d: la geometría y el pintado no coinciden",
					longitud, ancho, width)
			}
		}
	}

	// The wrapper breaks BY WORDS, so a single word wider than the box comes out whole and the box
	//clips it, which is right: breaking a word across lines misreads.
	for longitud := toastMaxWidth; longitud < toastMaxWidth+20; longitud++ {
		mensaje := strings.Repeat("z", longitud)
		width, _ := toastGeometry(mensaje, icon, 1000)
		lineas := wrapText(mensaje, max(width-toastFrame, toastMinInner)-toastIconGap)
		if len(lineas) != 1 || len(lineas[0]) != longitud {
			t.Errorf("una palabra de %d columnas se partió en %d trozos: una palabra no se parte",
				longitud, len(lineas))
		}
		for _, linea := range strings.Split(stripANSI(renderToast(t, mensaje, 1000)), "\n") {
			if ancho := ansi.StringWidth(linea); ancho != width {
				t.Errorf("una palabra de %d columnas dio una línea de %d en una caja de %d: la caja se salió",
					longitud, ancho, width)
			}
		}
	}
}

func TestToastGeometryLaEstimacionEsLaSumaDeSusPiezas(t *testing.T) {
	for longitud := 0; longitud <= toastMaxWidth; longitud++ {
		for _, nivel := range []toastLevel{toastSuccess, toastError, toastInfo, toastWarning} {
			mensaje := strings.Repeat("z", longitud)
			icon := toastIcon(nivel)

			width, _ := toastGeometry(mensaje, icon, 1000)

			want := longitud + ansi.StringWidth(icon) + toastIconSpace + toastFrame
			want = min(max(want, toastMinWidth), toastMaxWidth)
			if width != want {
				t.Errorf("un mensaje de %d columnas con icono %q dio un ancho de %d, want %d (el texto, el icono, su espacio y el marco)",
					longitud, icon, width, want)
			}
		}
	}
}

// Every current icon is one column, so a hardcoded 1 would give the same answer today and break
// tomorrow.
func TestToastGeometryElIconoEmpujaSegunSuAncho(t *testing.T) {
	corto := strings.Repeat("z", 30)
	for _, icon := range []string{"i", "ii", "iii", "✨", "⚠️"} {
		want := 30 + ansi.StringWidth(icon) + toastIconSpace + toastFrame
		want = min(max(want, toastMinWidth), toastMaxWidth)
		if width, _ := toastGeometry(corto, icon, 1000); width != want {
			t.Errorf("con un icono de %d columnas dio un ancho de %d, want %d: el icono no empujó",
				ansi.StringWidth(icon), width, want)
		}
	}
	a, _ := toastGeometry(corto, "ii", 1000)
	b, _ := toastGeometry(corto, "⚠️", 1000)
	if a != b {
		t.Errorf("dos iconos de la misma medida dieron anchos distintos: %d y %d", a, b)
	}
	estrecho, _ := toastGeometry(corto, "i", 1000)
	ancho, _ := toastGeometry(corto, "✨✨", 1000)
	if ancho < estrecho {
		t.Errorf("un icono de 4 columnas dio una caja de %d y uno de 1 la dio de %d: el icono solo empuja",
			ancho, estrecho)
	}
}

func renderToast(t *testing.T, mensaje string, available int) string {
	t.Helper()
	m := newToastManager()
	return m.render(toast{message: mensaje, level: toastInfo}, available)
}

// The air is what separates "a warning on top of the view" from "one more border".
func TestToastColumnVaPegadaALaDerechaConAire(t *testing.T) {
	for width := 0; width <= 200; width += 1 {
		for _, bw := range []int{1, 5, 24, 60, 100, 200} {
			x := toastColumn(width, bw)

			if x < 0 {
				t.Errorf("toastColumn(%d, %d) = %d: una columna negativa se sale", width, bw, x)
			}
			// Only asserted when it fits.
			if bw <= width-1 && x+bw > width-1 {
				t.Errorf("toastColumn(%d, %d) = %d: la caja acaba en la columna %d y la vista tiene %d, se sale por la derecha",
					width, bw, x, x+bw, width)
			}
			if bw < width-1 && x != width-bw-1 {
				t.Errorf("toastColumn(%d, %d) = %d, want %d: el aire de la derecha es de una columna",
					width, bw, x, width-bw-1)
			}
			if bw >= width-1 && x != 0 {
				t.Errorf("toastColumn(%d, %d) = %d sin sitio para el aire: debería ir a 0", width, bw, x)
			}
			if x != max(width-bw-1, 0) {
				t.Errorf("toastColumn(%d, %d) = %d, want %d", width, bw, x, max(width-bw-1, 0))
			}
		}
	}
}

// The +1 is the edge that matters: the last row (index anchor) is the lowest there is, so anchor+1
// rows fit from it counting up.
func TestToastBlockHeightNuncaPasaDeLoQueQueda(t *testing.T) {
	// anchor starts at 0 on purpose: it comes from len(lines)-1 over a strings.Split, which always
	//returns at least one line, so anchor is never negative.
	for anchor := 0; anchor <= 50; anchor++ {
		for bh := 0; bh <= 50; bh++ {
			got := toastBlockHeight(bh, anchor)

			if got > anchor+1 {
				t.Errorf("toastBlockHeight(%d, %d) = %d con hueco para %d: se sale por arriba",
					bh, anchor, got, anchor+1)
			}
			if got > bh {
				t.Errorf("toastBlockHeight(%d, %d) = %d: inventó filas", bh, anchor, got)
			}
			if got != min(bh, anchor+1) {
				t.Errorf("toastBlockHeight(%d, %d) = %d, want %d", bh, anchor, got, min(bh, anchor+1))
			}
		}
	}
	ventana := 3
	if got := toastBlockHeight(ventana, ventana-1); got != ventana {
		t.Errorf("una caja de %d filas en una ventana de %d quedó en %d: no cabe justo, que es el caso que debe caber",
			ventana, ventana, got)
	}
}
