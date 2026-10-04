package tui

import (
	"strings"
	"testing"
)

// Three guards, three places, same shape: an exact edge that can break.

// Clipping from the top with ZERO rows leaves the detail whole.
func TestClipTopConCeroFilasDejaElDetalleEntero(t *testing.T) {
	tres := []string{"estado: abierto", "review: aprobado", "rol: autor"}

	for _, filas := range []int{-5, 0} {
		if got := clipTop(tres, filas); len(got) != len(tres) {
			t.Errorf("con %d filas el detalle quedó en %d líneas, want %d: sin sitio para "+
				"recortar no se recorta. Con el cero pasando las dos comprobaciones el "+
				"recorte devuelve un slice vacío y la ficha se queda en blanco",
				filas, len(got), len(tres))
		}
	}

	if got := clipTop(tres, 1); len(got) != 1 || got[0] != "rol: autor" {
		t.Errorf("con una fila quedó %v, want la última: el recorte es por arriba para "+
			"que el final del detalle sobreviva", got)
	}

	for _, filas := range []int{3, 4, 100} {
		if got := clipTop(tres, filas); len(got) != len(tres) {
			t.Errorf("con %d filas para %d líneas quedó en %d: no hay nada que recortar",
				filas, len(tres), len(got))
		}
	}
}

// The warning anchors to the LAST row of the view.
func TestElAvisoSePegaAbajoEnLaUltimaFila(t *testing.T) {
	const filas, ancho = 12, 40

	rows := make([]bool, filas)
	for i := range rows {
		rows[i] = true
	}
	base := make([]string, filas)
	for i := range base {
		base[i] = strings.Repeat("·", ancho)
	}

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

	ultima := lineas[filas-1]
	if !strings.Contains(ultima, "+--------------+") {
		t.Errorf("la última fila es %q y no lleva el borde del aviso: el bloque tiene "+
			"que terminar en la última fila de la vista", primeraLineaCon(ultima, 60))
	}
	if !strings.Contains(lineas[filas-3], "+--------------+") {
		t.Errorf("la antepenúltima fila es %q y no lleva el borde superior del aviso",
			primeraLineaCon(lineas[filas-3], 60))
	}
	if !strings.Contains(lineas[filas-2], "un aviso") {
		t.Errorf("la penúltima fila es %q y no lleva el texto del aviso",
			primeraLineaCon(lineas[filas-2], 60))
	}

	rowsConHueco := make([]bool, filas)
	copy(rowsConHueco, rows)
	rowsConHueco[filas-1] = false // la última fila es borde de algo
	got = overlayToasts(strings.Join(base, "\n"), []string{caja}, ancho, rowsConHueco)
	lineas = strings.Split(got, "\n")
	if strings.Contains(lineas[filas-1], "un aviso") {
		t.Errorf("el aviso se pintó en la última fila, que es borde: %q",
			primeraLineaCon(lineas[filas-1], 60))
	}
	// It moves exactly enough: the block needs THREE rows and with the last one closed only two fit.
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
