package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// TestToastGeometryAcotaElAncho: el ancho de la caja sale de una estimación
// acotada por los dos límites y por el espacio libre.
//
// Lo que se afirma aquí NO es visible desde la caja pintada: una columna de más o
// de menos no se ve como una columna, se ve como "el aviso ocupa más filas", y como
// el texto se parte al ancho útil, el error se propaga al número de filas y se
// confunde con un problema de envoltura.
//
// Por eso esto vive en una función pura. Y por eso el allowlist de este repo tuvo
// que admitir estos mutantes durante meses: se estaban mirando desde el sitio
// equivocado, no desde el sitio imposible.
func TestToastGeometryAcotaElAncho(t *testing.T) {
	icon := toastIcon(toastInfo)

	for _, mensaje := range []string{"", "hola", strings.Repeat("x", 40), strings.Repeat("y", 200)} {
		for _, available := range []int{0, 5, 9, 20, 50, 100, 1000} {
			width, wrapAt := toastGeometry(mensaje, icon, available)

			// El ancho NUNCA pasa del máximo: un aviso más ancho invade la vista
			// entera y tapa lo que el usuario fue a ver.
			if width > toastMaxWidth {
				t.Errorf("mensaje de %d columnas con %d disponibles dio un ancho de %d, want <= %d",
					len(mensaje), available, width, toastMaxWidth)
			}
			// Y NUNCA pasa del hueco libre, que es la promesa del paquete entero: un
			// aviso no desborda la terminal. OJO al orden: el hueco pisa el MÍNIMO, no
			// al revés. Con 9 columnas libres la caja se hace de 9 aunque el mínimo
			// sea 24, porque una caja de 24 en una vista de 9 se sale, y salirse es
			// peor que ser ilegible.
			if available > toastMinAvailable && width > available {
				t.Errorf("con %d columnas libres la caja mide %d: desborda", available, width)
			}
			// Con hueco de sobra, el mínimo SÍ manda: una caja más estrecha que
			// toastMinWidth no tiene sitio ni para el icono.
			if available >= toastMinWidth && width < toastMinWidth {
				t.Errorf("con %d columnas libres la caja mide %d, por debajo del mínimo %d",
					available, width, toastMinWidth)
			}

			// Y el ancho de partición es EXACTAMENTE el útil menos el hueco del
			// icono, sin suelo propio. El suelo está en el ancho útil, y aquí solo se
			// descuenta: por eso el mínimo de partición que sale es 6 (8-2) y no el 4
			// que un suelo aquí prometería. Afirmar la aritmética exacta, y no "por
			// encima de un suelo", es lo que deja ver un -1 o un -3 donde la
			// partición se mueve de más.
			want := max(width-toastFrame, toastMinInner) - toastIconGap
			if wrapAt != want {
				t.Errorf("con un ancho de %d el texto se parte a %d, want %d (el útil menos el hueco del icono)",
					width, wrapAt, want)
			}
			// Y nunca por debajo de 6 columnas: por debajo el aviso sale en trozos
			// tan cortos que no dicen nada.
			if wrapAt < toastMinInner-toastIconGap {
				t.Errorf("con un ancho de %d el texto se parte a %d, por debajo de %d: sale en trozos ilegibles",
					width, wrapAt, toastMinInner-toastIconGap)
			}
		}
	}
}

// TestToastGeometryRespetaElEspacioLibre: con hueco de sobra, la caja se estrecha
// hasta él, porque un aviso que invade la vista tapa lo que el usuario fue a ver.
//
// Y por debajo del hueco mínimo NO se intenta: la caja se queda con su ancho
// acotado y es la superposición la que la recorta. Intentar encajar en 4 columnas
// daría un aviso de 4 columnas con una letra por línea, que no informa de nada.
func TestToastGeometryRespetaElEspacioLibre(t *testing.T) {
	icon := toastIcon(toastInfo)
	mensaje := strings.Repeat("x", 30)

	// Hueco generoso: la caja se ajusta al hueco si el hueco aprieta.
	for _, available := range []int{30, 40, 50, 59} {
		width, _ := toastGeometry(mensaje, icon, available)
		if width > available {
			t.Errorf("con %d columnas libres la caja mide %d: invade la vista", available, width)
		}
	}

	// Hueco por debajo del mínimo: no se encoge. Es el mismo ancho que sin hueco
	// ninguno, porque por debajo del mínimo la decisión es la misma.
	debajo, _ := toastGeometry(mensaje, icon, 0)
	for _, available := range []int{-5, 0, 1, toastMinAvailable} {
		width, _ := toastGeometry(mensaje, icon, available)
		if width != debajo {
			t.Errorf("con %d columnas libres dio un ancho de %d y sin hueco da %d: por debajo del mínimo %d no debería cambiar",
				available, width, debajo, toastMinAvailable)
		}
	}

	// Y una columna por encima del mínimo, sí cambia: ese es el borde.
	//
	// Y el suelo aguanta incluso con la caja más estrecha que existe: el texto nunca
	// se parte por debajo de 6 columnas, que es lo que evita que un aviso salga en
	// trozos tan cortos que no dicen nada.
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

// TestToastGeometryElTextoCabeEnLaCaja: la relación entre el ancho de la caja y el
// de partición es lo que hace que el aviso no se salga.
//
// Es el mismo contrato que en los comentarios: un texto más ancho no da una caja
// más ancha, se recorta. Y aquí se ve MUY bien, porque el marco de la caja es
// visible y el texto se corta contra él.
func TestToastGeometryElTextoCabeEnLaCaja(t *testing.T) {
	icon := toastIcon(toastInfo)
	// Mensajes de la banda media, donde ni el mínimo ni el máximo mandan y la
	// aritmética se ve de verdad.
	for longitud := 0; longitud <= toastMaxWidth; longitud++ {
		// Un mensaje de MUCHAS palabras cortas, que es lo que se puede partir. Se
		// mide el texto con espacios porque una palabra suelta no se parte: es el
		// caso aparte de más abajo.
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
			// Y con el icono delante en la primera línea. OJO: el ancho del icono se
			// mide en COLUMNAS, no con len(), que contaría sus tres bytes: "ℹ" mide
			// una columna y tres bytes, y con len() el aviso parecería tres columnas
			// más ancho de lo que es.
			if ansi.StringWidth(icon)+1+len(linea) > width-toastFrame {
				t.Errorf("un mensaje de %d columnas dio una primera línea de %d más el icono en una caja de %d",
					longitud, len(linea), width)
			}
		}
		// Y la caja se pinta al ancho EXACTO que dice la geometría, con su marco.
		//
		// OJO al medir: el borde son caracteres de tres bytes, así que len() cuenta
		// bytes y una caja de 24 mide 72. Es el mismo error de bytes por columnas
		// que salió antes con el glifo del cursor, y por eso se usa el ancho visible.
		for _, linea := range strings.Split(stripANSI(renderToast(t, mensaje, 1000)), "\n") {
			if ancho := ansi.StringWidth(linea); ancho != width {
				t.Errorf("un mensaje de %d columnas dio una línea de %d en una caja de %d: la geometría y el pintado no coinciden",
					longitud, ancho, width)
			}
		}
	}

	// Y el caso que la anterior no cubre: una palabra más ancha que la caja. El
	// envoltorio parte POR PALABRAS, así que una palabra suelta no se parte: sale
	// entera y la caja la recorta. Es lo correcto, porque partir una palabra por la
	// mitad la convierte en otra palabra, y un aviso que dice otra cosa no informa de
	// nada. Lo que NO puede pasar es que la caja se salga por ello, y no se sale
	// porque la caja tiene el ancho fijo.
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

// TestToastGeometryLaEstimacionEsLaSumaDeSusPiezas: el ancho de la caja se estima
// sumando el texto, el icono, su espacio y el marco. Cada sumando es una pieza con
// nombre y la suma es el contrato.
//
// Se afirma la suma a partir del SIGNIFICADO de cada pieza, no reescribiendo la
// expresión. La diferencia importa: reescribir `+ toastIconSpace + toastFrame` en el
// test dejaría pasar un `+ 0` en cualquiera de los dos, porque los dos lados se
// equivocarían igual. Con la suma escrita desde el significado, un término que falta
// se ve.
//
// Y la banda donde la suma se ve es la media: con un mensaje corto manda el mínimo y
// con uno largo el máximo, y en los dos extremos cualquier error de un término queda
// tapado por el acotado. Por eso el recorrido es por la banda media, que es
// justamente donde la estimación decide.
func TestToastGeometryLaEstimacionEsLaSumaDeSusPiezas(t *testing.T) {
	// La banda media: anchos que no tocan ni el mínimo ni el máximo.
	for longitud := 0; longitud <= toastMaxWidth; longitud++ {
		for _, nivel := range []toastLevel{toastSuccess, toastError, toastInfo, toastWarning} {
			mensaje := strings.Repeat("z", longitud)
			icon := toastIcon(nivel)

			width, _ := toastGeometry(mensaje, icon, 1000)

			// La suma: el texto, más lo que el icono empuja (su ancho y su
			// espacio), más el marco. Un aviso con un mensaje de N columnas necesita
			// N más el hueco del icono más el marco, o el marco se come el texto.
			want := longitud + ansi.StringWidth(icon) + toastIconSpace + toastFrame
			// Y acotado a los dos límites, que es lo que hace la cadena entera.
			want = min(max(want, toastMinWidth), toastMaxWidth)
			if width != want {
				t.Errorf("un mensaje de %d columnas con icono %q dio un ancho de %d, want %d (el texto, el icono, su espacio y el marco)",
					longitud, icon, width, want)
			}
		}
	}
}

// TestToastGeometryElIconoEmpujaSegunSuAncho: el hueco de la primera línea se mide
// con el ancho real del icono, no con un 1 supuesto.
//
// Todos los iconos actuales miden una columna, así que un `1` a secas daría el mismo
// resultado hoy. Pero un icono de dos columnas —un emoji, un símbolo de doble ancho—
// se saldría del marco con el `1` fijo, y ese es el fallo que no se ve hasta que
// alguien añade el icono. La geometría se mide con ansi.StringWidth por eso, y aquí
// se comprueba con un icono inventado de dos columnas en vez de esperar a que
// aparezca uno de verdad.
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
	// Y dos iconos del MISMO ancho dan la misma caja, aunque sean glifos distintos:
	// lo que cuenta es la medida, no el carácter.
	a, _ := toastGeometry(corto, "ii", 1000)
	b, _ := toastGeometry(corto, "⚠️", 1000)
	if a != b {
		t.Errorf("dos iconos de la misma medida dieron anchos distintos: %d y %d", a, b)
	}
	// Y un icono más ancho NUNCA da una caja más estrecha: solo puede empujar.
	estrecho, _ := toastGeometry(corto, "i", 1000)
	ancho, _ := toastGeometry(corto, "✨✨", 1000)
	if ancho < estrecho {
		t.Errorf("un icono de 4 columnas dio una caja de %d y uno de 1 la dio de %d: el icono solo empuja",
			ancho, estrecho)
	}
}

// renderToast pinta un aviso con un ancho dado, para poder mirar la caja entera.
func renderToast(t *testing.T, mensaje string, available int) string {
	t.Helper()
	m := newToastManager()
	return m.render(toast{message: mensaje, level: toastInfo}, available)
}

// TestToastColumnVaPegadaALaDerechaConAire: la caja se ancla a la derecha, con UNA
// columna de aire entre ella y el borde.
//
// El aire es lo que separa "esto es un aviso encima de la vista" de "esto es un
// borde más". Sin él, la última columna de la caja es la última de la vista y el
// aviso se lee como parte del marco.
func TestToastColumnVaPegadaALaDerechaConAire(t *testing.T) {
	for width := 0; width <= 200; width += 1 {
		for _, bw := range []int{1, 5, 24, 60, 100, 200} {
			x := toastColumn(width, bw)

			// Nunca negativa: una columna negativa es un índice que se sale, y en
			// ansi.Truncate eso no es un error visible, es un recorte en la
			// posición equivocada.
			if x < 0 {
				t.Errorf("toastColumn(%d, %d) = %d: una columna negativa se sale", width, bw, x)
			}
			// Y la caja no se sale por la derecha CUANDO CABE: acaba en la columna
			// width-2 y deja UNA de aire. Pegada al borde se leería como parte del
			// marco, que es justo lo que un aviso no es.
			//
			// Solo se afirma cuando cabe. Con una caja más ancha que la vista no hay
			// sitio donde no se salga, y el comportamiento correcto es salirse y
			// dejar que la superposición recorte: esconder el aviso entero sería
			// peor que enseñarlo cortado.
			if bw <= width-1 && x+bw > width-1 {
				t.Errorf("toastColumn(%d, %d) = %d: la caja acaba en la columna %d y la vista tiene %d, se sale por la derecha",
					width, bw, x, x+bw, width)
			}
			// Con sitio de sobra, va pegada a la derecha con ese aire: ni más
			// (parecería que falta algo) ni menos (parecería marco).
			if bw < width-1 && x != width-bw-1 {
				t.Errorf("toastColumn(%d, %d) = %d, want %d: el aire de la derecha es de una columna",
					width, bw, x, width-bw-1)
			}
			// Cuando NO hay sitio para el aire —la caja es tan ancha que se
			// saltaría el borde—, va a 0. Es lo menos malo: al menos se ve por
			// dónde empieza, y la superposición recorta por la derecha.
			if bw >= width-1 && x != 0 {
				t.Errorf("toastColumn(%d, %d) = %d sin sitio para el aire: debería ir a 0", width, bw, x)
			}
			// Y la aritmética exacta, para que un -1 o un -2 en el aire se vean.
			if x != max(width-bw-1, 0) {
				t.Errorf("toastColumn(%d, %d) = %d, want %d", width, bw, x, max(width-bw-1, 0))
			}
		}
	}
}

// TestToastBlockHeightNuncaPasaDeLoQueQueda: una caja no se pinta más alta que el
// hueco que hay.
//
// El +1 es el borde que importa: la última fila (índice anchor) es la más baja que
// existe, así que desde ella caben anchor+1 filas contando las de arriba. Sin ese
// +1, una caja de 3 filas en una ventana de 3 no encontraría sitio nunca, que es
// el peor caso posible: en la ventana hecha a su medida no se pinta.
func TestToastBlockHeightNuncaPasaDeLoQueQueda(t *testing.T) {
	// anchor arranca en 0 a propósito: viene de len(lines)-1 sobre un
	// strings.Split, que siempre devuelve al menos una línea, así que anchor nunca
	// es negativo. Probarlo por debajo sería inventar un caso que el producto no
	// puede tener, y un suelo parataparlo sería ruido.
	for anchor := 0; anchor <= 50; anchor++ {
		for bh := 0; bh <= 50; bh++ {
			got := toastBlockHeight(bh, anchor)

			// Nunca más alta que el hueco: es lo que evita que la caja se salga por
			// arriba y se coma el borde de otro marco.
			if got > anchor+1 {
				t.Errorf("toastBlockHeight(%d, %d) = %d con hueco para %d: se sale por arriba",
					bh, anchor, got, anchor+1)
			}
			// Y nunca más alta que la caja misma: no hay filas de relleno que
			// invente.
			if got > bh {
				t.Errorf("toastBlockHeight(%d, %d) = %d: inventó filas", bh, anchor, got)
			}
			// Y la aritmética exacta, que es lo que separa el +1 del 0.
			if got != min(bh, anchor+1) {
				t.Errorf("toastBlockHeight(%d, %d) = %d, want %d", bh, anchor, got, min(bh, anchor+1))
			}
		}
	}
	// Y el caso que de verdad importa, dicho explícito: una caja del alto exacto de
	// la ventana cabe, con la base en la última fila.
	ventana := 3
	if got := toastBlockHeight(ventana, ventana-1); got != ventana {
		t.Errorf("una caja de %d filas en una ventana de %d quedó en %d: no cabe justo, que es el caso que debe caber",
			ventana, ventana, got)
	}
}
