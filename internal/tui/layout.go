// Presupuesto de alto de la vista: qué cajas se ven y cuánto alto queda para el
// cuerpo de cada una. Es la fuente única compartida por el render, el scroll y
// los tests, igual que la tenía gitdash: la suma de las partes tiene que dar la
// altura del terminal, porque la vista se rellena a la altura exacta y cualquier
// descuadre empuja los atajos o la cabecera fuera de pantalla.
package tui

// Reparto vertical de la pantalla. El panel de detalle se queda con el 40% de
// la altura (detailShare/5) y el cuerpo central con el hueco libre.
const (
	detailShare = 2 // 2/5 = 40% de la altura
	minListRows = 3
	// minDetailRows es el mínimo del panel: por debajo el detalle se quedaría en
	// un par de líneas sin decir nada útil.
	minDetailRows = 6
	// maxHintLines acota las líneas de atajos. La barra se envuelve al ancho
	// interior, así que en un terminal estrecho son varias líneas, pero nunca más
	// de este tope: el layout recorta hints sin tener que recomputarlos.
	maxHintLines = 3
)

// layout describe las cajas visibles y el alto reservado a cada cuerpo.
// bodyLines es el del cuerpo central, que es la lista. detailLines es el del
// panel inferior con la ficha del ítem seleccionado.
type layout struct {
	showHeader   bool
	showKeybinds bool
	hintLines    int
	bodyLines    int
	detailLines  int
}

// Ajustes de las cajas: los bordes de cada una, más el contenido de la cabecera.
// El de las otras tres lo fija el layout, porque es lo que tienen que ceder.
const (
	headerContentLines = 1 // spinner y estado de los forges
	headerLines        = headerContentLines + 2
	listChrome         = 2
	detailChrome       = 2
	keybindsChrome     = 2
)

// computeLayout reparte la altura de la terminal entre las cajas.
//
// El detalle conserva su 40% mientras el cuerpo central pueda quedarse al menos
// minListRows. Si no cabe, degrada en orden: el panel de detalle baja a su
// mínimo, luego se oculta la caja de cabecera, luego los atajos bajan de
// maxHintLines a una sola línea y luego se oculta la caja de atajos entera. Ese
// recorrido deja lo imprescindible —cuerpo central y detalle— y es lo que hace
// usable la vista en un terminal de 20 líneas, donde las cuatro cajas no caben.
//
// hintAvailable son las líneas de atajos que hay realmente, ya envueltas al
// ancho interior: la caja de atajos nunca pide más de las que se van a pintar.
// show es false antes del primer WindowSizeMsg, en cuyo caso no se recorta nada
// y la vista se pinta entera.
func computeLayout(height, hintAvailable int, show bool) layout {
	if !show || height <= 0 {
		return layout{}
	}

	lay := layout{
		showHeader:   true,
		showKeybinds: true,
		hintLines:    min(maxHintLines, max(1, hintAvailable)),
	}
	// El cuerpo central es la lista: cobra sus 2 bordes.
	lay.bodyLines = height
	lay.detailLines = max(minDetailRows, height*detailShare/5)

	// reserved es todo lo que no es cuerpo central: los bordes de las cajas
	// visibles, la línea de estado de la cabecera, el panel y los atajos.
	reserved := func() int {
		n := listChrome + detailChrome + lay.detailLines
		if lay.showHeader {
			n += headerLines
		}
		if lay.showKeybinds {
			n += keybindsChrome + lay.hintLines
		}
		return n
	}

	// El cuerpo central necesita minListRows: sin él no hay ventana que
	// desplazar ni ficha que leer.
	for reserved()+minListRows > height && lay.detailLines > minDetailRows {
		lay.detailLines--
	}
	for reserved()+minListRows > height && lay.showHeader {
		lay.showHeader = false
	}
	for reserved()+minListRows > height && lay.hintLines > 1 {
		lay.hintLines--
	}
	for reserved()+minListRows > height && lay.showKeybinds {
		lay.showKeybinds = false
		lay.hintLines = 0
	}
	// Último recurso, solo en terminales diminutos: el panel se queda sin cuerpo
	// antes que dejar el cuerpo central sin una sola línea.
	for reserved()+1 > height && lay.detailLines > 0 {
		lay.detailLines--
	}

	lay.bodyLines = max(1, height-reserved())
	return lay
}
