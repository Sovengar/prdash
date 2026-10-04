package tui

import "testing"

func TestLandRowEligeLaFilaMasBajaQueCabe(t *testing.T) {
	abiertas := func(n int) []bool {
		rows := make([]bool, n)
		for i := range rows {
			rows[i] = true
		}
		return rows
	}

	base, ok := landRow(abiertas(10), 9, 3)
	if !ok {
		t.Fatal("una caja de 3 filas en una ventana de 10 no encontró sitio, y sobra hueco")
	}
	if base != 9 {
		t.Errorf("una caja de 3 filas en una ventana de 10 se apoyó en la fila %d, want 9 (la más baja)", base)
	}
	if desde := base - 3 + 1; desde != 7 {
		t.Errorf("con la base en la %d ocupa desde la %d, want 7", base, desde)
	}

	if _, ok := landRow(abiertas(3), 2, 10); ok {
		t.Error("una caja de 10 filas encontró sitio en una ventana de 3")
	}

	if base, ok := landRow(abiertas(10), 9, 1); !ok || base != 9 {
		t.Errorf("una caja de 1 fila fue a la %d (ok=%v), want la 9", base, ok)
	}
}

// The anchor is the limit: a warning already in place cannot move again.
func TestLandRowNoSeSaleDelAncla(t *testing.T) {
	abiertas := func(n int) []bool {
		rows := make([]bool, n)
		for i := range rows {
			rows[i] = true
		}
		return rows
	}
	rows := abiertas(20)

	base, ok := landRow(rows, 4, 3)
	if !ok {
		t.Fatal("una caja de 3 filas con el ancla en la 4 no encontró sitio, y caben de sobra")
	}
	if base > 4 {
		t.Errorf("con el ancla en la 4 la caja se apoyó en la %d: se pasó del ancla", base)
	}
	if base != 4 {
		t.Errorf("con el ancla en la 4 y sitio de sobra la caja se apoyó en la %d, want 4", base)
	}
	if _, ok := landRow(rows, 1, 3); ok {
		t.Error("una caja de 3 filas encontró sitio con el ancla en la 1: no cabe por encima del ancla")
	}
	if base, ok := landRow(rows, 2, 3); !ok || base != 2 {
		t.Errorf("con el ancla en la 2 la caja de 3 filas fue a la %d (ok=%v), want la 2", base, ok)
	}
}

func TestLandRowSubePorEncimaDeLoQueNoAdmite(t *testing.T) {
	rows := make([]bool, 10)
	for i := range rows {
		rows[i] = true
	}
	rows[7], rows[8], rows[9] = false, false, false

	base, ok := landRow(rows, 9, 3)
	if !ok {
		t.Fatal("una caja de 3 filas no encontró sitio con las 3 de abajo cerradas, y las 7 de arriba están libres")
	}
	if base != 6 {
		t.Errorf("la caja se apoyó en la fila %d, want 6 (justo encima de las cerradas)", base)
	}
	for i := base - 3 + 1; i <= base; i++ {
		if !rows[i] {
			t.Errorf("la caja ocupa la fila %d, que no admite avisos: parte un marco", i)
		}
	}

	rows2 := make([]bool, 10)
	for i := range rows2 {
		rows2[i] = true
	}
	rows2[8], rows2[9] = false, false
	if base, ok := landRow(rows2, 9, 2); !ok || base != 7 {
		t.Errorf("con un hueco de 2 filas abajo la caja de 2 fue a la %d (ok=%v), want la 7", base, ok)
	}

	ninguna := make([]bool, 5)
	if _, ok := landRow(ninguna, 4, 3); ok {
		t.Error("una caja de 3 filas encontró sitio en una ventana donde no admits ninguna fila")
	}
}

// ALL the rows have to admit it: one closed row is enough to refuse.
func TestAdmitenAvisoEsTodoONada(t *testing.T) {
	abiertas := func(n int) []bool {
		rows := make([]bool, n)
		for i := range rows {
			rows[i] = true
		}
		return rows
	}

	if !admitenAviso(abiertas(10), 0, 10) {
		t.Error("diez filas abiertas no se admitting themselves: la función está rota")
	}
	for _, cerrada := range []int{0, 3, 6, 9} {
		rows := abiertas(10)
		rows[cerrada] = false
		if admitenAviso(rows, 0, 10) {
			t.Errorf("con la fila %d cerrada, el bloque entero se admitió: medio aviso parte el marco igual que entero", cerrada)
		}
	}
	if admitenAviso(abiertas(10), 8, 5) {
		t.Error("un bloque que se sale por abajo se admitió: la fila 12 no existe")
	}
	if admitenAviso(abiertas(10), -3, 2) {
		t.Error("un bloque con un índice negativo se admitió")
	}
	if !admitenAviso(abiertas(10), 8, 2) {
		t.Error("un bloque que termina justo en la última fila no se admitió")
	}
	if admitenAviso(abiertas(10), 9, 2) {
		t.Error("un bloque que termina una fila más allá de la última se admitió")
	}
	if !admitenAviso(abiertas(10), 5, 0) {
		t.Error("un bloque de cero filas debería admitirse: no hay nada que se oponga")
	}
	if admitenAviso(nil, 0, 1) {
		t.Error("un bloque sobre un slice vacío se admitió")
	}
}

func TestLandRowConUnaCajaDeAlturaCero(t *testing.T) {
	rows := make([]bool, 10)
	for i := range rows {
		rows[i] = true
	}
	for _, bh := range []int{0, -1, -10} {
		if base, ok := landRow(rows, 9, bh); ok {
			t.Errorf("una caja de %d filas se colocó en la %d: no hay nada que pintar", bh, base)
		}
	}
	if _, ok := landRow(rows, -1, 3); ok {
		t.Error("una caja encontró sitio con un ancla negativo")
	}
}
