package tui

import (
	"strings"
	"testing"
)

// Tres guards de tres sitios, y los tres tienen la misma forma: un borde exacto que se
// puede mover un punto sin que se note.
//
// Lo que los distingue del resto es que en los tres el valor de la frontera NO es un caso
// raro, es un caso de producción. Cero filas de hueco, la última fila de la vista y dos
// comentarios con la misma atención pasan todos en cada ejecución.

// TestClipTopConCeroFilasDejaElDetalleEntero: el recorte por arriba con CERO filas no
// recorta.
//
// Y el cero es un caso de producción, no un borde teórico: el detalle se recorta contra
// las filas que sobran en la ficha, y en un terminal corto sobran cero. Con la condición
// escrita como `< 0` en vez de `<= 0`, el cero pasa las dos comprobaciones y el recorte
// hace `lines[len(lines):]`, que es un slice VACÍO. La ficha se queda sin nada en vez de
// quedarse entera.
//
// Y la asimetría es lo que hace que el caso tenga sentido: con más filas de las que hay,
// el detalle se devuelve entero, y con CERO filas también. Cero filas no significa "sin
// detalle", significa "no hay sitio para recortar". Si cero significara "sin detalle", un
// terminal pequeño dejaría la ficha en blanco justo cuando el usuario la necesita entera.
func TestClipTopConCeroFilasDejaElDetalleEntero(t *testing.T) {
	tres := []string{"estado: abierto", "review: aprobado", "rol: autor"}

	// Cero filas, y filas negativas: el detalle entero.
	for _, filas := range []int{-5, 0} {
		if got := clipTop(tres, filas); len(got) != len(tres) {
			t.Errorf("con %d filas el detalle quedó en %d líneas, want %d: sin sitio para "+
				"recortar no se recorta. Con el cero pasando las dos comprobaciones el "+
				"recorte devuelve un slice vacío y la ficha se queda en blanco",
				filas, len(got), len(tres))
		}
	}

	// Con una fila sí recorta, y por el final: el estado, el review y el rol son lo que
	// dice si la acción procede, y es lo que tiene que sobrevivir.
	if got := clipTop(tres, 1); len(got) != 1 || got[0] != "rol: autor" {
		t.Errorf("con una fila quedó %v, want la última: el recorte es por arriba para "+
			"que el final del detalle sobreviva", got)
	}

	// Y con un número de filas igual o mayor que las que hay, no se toca. Ese es el
	// otro borde, y es identidad: recortar por el número entero de elementos a esa
	// longitud no es recortar.
	//
	// Y el 2 no está en la lista a propósito: con tres líneas y dos filas SÍ recorta,
	// porque hay algo que recortar. La primera versión de esta lista empezaba en 2 y
	// falló por eso.
	for _, filas := range []int{3, 4, 100} {
		if got := clipTop(tres, filas); len(got) != len(tres) {
			t.Errorf("con %d filas para %d líneas quedó en %d: no hay nada que recortar",
				filas, len(tres), len(got))
		}
	}
}

// TestElAvisoSePegaAbajoEnLaUltimaFila: el aviso se ancla en la última fila de la vista,
// y de ahí para arriba.
//
// Y el `-1` de `anchor` es lo que dice "la última fila". Con `+1` el ancla está una fila
// más abajo de la última, y el índice sale del rango: un PANIC. Con `-2` está una más
// arriba, que no revienta y por eso es el peligroso: el aviso aparece una fila más arriba
// de donde le toca y no se ve ningún fallo, solo un aviso flotando.
//
// Y la pregunta no es "dónde acaba el aviso" sino "qué fila es la última de la vista",
// porque un aviso pegado al fondo de un hueco entre cajas se lee como parte de la caja de
// abajo. De ahí el `admitenAviso`: el bloque solo aterriza si todas sus filas son interior
// de alguna caja.
//
// El caso que mira es el de la vista alta, que es donde la diferencia se ve, y el de un
// hueco que no llega al fondo, que es donde el anclaje decide si el aviso se pinta.
func TestElAvisoSePegaAbajoEnLaUltimaFila(t *testing.T) {
	const filas, ancho = 12, 40

	// La vista: filas que admiten aviso, con la última incluida.
	rows := make([]bool, filas)
	for i := range rows {
		rows[i] = true
	}
	base := make([]string, filas)
	for i := range base {
		base[i] = strings.Repeat("·", ancho)
	}

	// Una caja de tres filas, que es el alto mínimo con contenido.
	caja := strings.Join([]string{
		"+--------------+",
		"| un aviso     |",
		"+--------------+",
	}, "\n")

	got := overlayToasts(strings.Join(base, "\n"), []string{caja}, ancho, rows)
	lineas := strings.Split(got, "\n")

	if len(lineas) != filas {
		t.Fatalf("la superposición cambió el número de filas: %d, want %d. El aviso se "+
			"pinta encima recortando el fondo, no añadiendo filas", len(lineas), filas)
	}

	// La última fila de la vista es donde acaba el bloque, y la de arriba es donde
	// empieza. Ese es el anclaje: pegado al fondo, no flotando.
	ultima := lineas[filas-1]
	if !strings.Contains(ultima, "+--------------+") {
		t.Errorf("la última fila es %q y no lleva el borde del aviso: el bloque tiene "+
			"que terminar en la última fila de la vista", primeraLineaCon(ultima, 60))
	}
	if !strings.Contains(lineas[filas-3], "+--------------+") {
		t.Errorf("la antepenúltima fila es %q y no lleva el borde superior del aviso",
			primeraLineaCon(lineas[filas-3], 60))
	}
	// Y la fila de EN MEDIO lleva el contenido, no un borde.
	if !strings.Contains(lineas[filas-2], "un aviso") {
		t.Errorf("la penúltima fila es %q y no lleva el texto del aviso",
			primeraLineaCon(lineas[filas-2], 60))
	}

	// Y con un hueco al fondo que NO admite aviso, el bloque sube hasta el hueco y no se
	// sale por debajo. Esto es lo que decide el anclaje cuando el fondo no llega al
	// borde: pintar en la última fila invadiría un borde, y un aviso que se come un
	// borde se lee como parte del marco.
	rowsConHueco := make([]bool, filas)
	copy(rowsConHueco, rows)
	rowsConHueco[filas-1] = false // la última fila es borde de algo
	got = overlayToasts(strings.Join(base, "\n"), []string{caja}, ancho, rowsConHueco)
	lineas = strings.Split(got, "\n")
	if strings.Contains(lineas[filas-1], "un aviso") {
		t.Errorf("el aviso se pintó en la última fila, que es borde: %q",
			primeraLineaCon(lineas[filas-1], 60))
	}
	//
	// Y sube lo justo: el bloque necesita TRES filas, con la última cerrada solo caben
	// en la fila 10 y hacia arriba, así que el texto cae en la fila 9 — la tercera desde
	// abajo, no la cuarta. La primera versión miraba la cuarta y falló.
	if !strings.Contains(lineas[filas-3], "un aviso") {
		t.Errorf("con la última fila cerrada el aviso debería subir una fila y quedar en "+
			"la tercera desde abajo, y quedó %q en esa fila",
			primeraLineaCon(lineas[filas-3], 60))
	}
	if strings.Contains(lineas[filas-2], "un aviso") {
		t.Errorf("el aviso se quedó pegado al fondo con la última fila cerrada: %q",
			primeraLineaCon(lineas[filas-2], 60))
	}
}
