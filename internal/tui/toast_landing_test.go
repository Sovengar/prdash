package tui

import "testing"

// TestLandRowEligeLaFilaMasBajaQueCabe: un aviso se apila lo más abajo posible,
// subiendo solo lo que no cabe.
//
// Estas dos funciones (`landRow` y `admitenAviso`) son PURAS y devuelven un valor:
// el sitio o "no cabe". No hay nada que ver ni que medir, se llaman y se comprueba
// lo que devuelven. El allowlist las daba por no matables "desde el texto
// renderizado", que era cierto y no era lo que había que mirar: hacía falta llamar a
// la función, no mirar el dibujo.
func TestLandRowEligeLaFilaMasBajaQueCabe(t *testing.T) {
	// Una ventana de 10 filas, todas abiertas a avisos.
	abiertas := func(n int) []bool {
		rows := make([]bool, n)
		for i := range rows {
			rows[i] = true
		}
		return rows
	}

	// Una caja de 3 filas cabe con la base en la fila 9, la más baja: ocupa de la 7
	// a la 9. Prefiere abajo porque abajo es donde está lo que se acaba de tocar, y
	// un aviso encima de lo que el usuario está leyendo se lee antes.
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

	// Una caja más alta que la ventana no cabe: no hay sitio y no se inventa.
	if _, ok := landRow(abiertas(3), 2, 10); ok {
		t.Error("una caja de 10 filas encontró sitio en una ventana de 3")
	}

	// Y una caja de una fila va a la fila 9, la más baja posible.
	if base, ok := landRow(abiertas(10), 9, 1); !ok || base != 9 {
		t.Errorf("una caja de 1 fila fue a la %d (ok=%v), want la 9", base, ok)
	}
}

// TestLandRowNoSeSaleDelAncla: el ancla es el límite. Un aviso que ya está en su
// sitio no puede empujar a otro por encima de donde se le dijo, o los avisos se
// subirían hasta el borde de arriba de la pantalla con el tiempo.
func TestLandRowNoSeSaleDelAncla(t *testing.T) {
	abiertas := func(n int) []bool {
		rows := make([]bool, n)
		for i := range rows {
			rows[i] = true
		}
		return rows
	}
	rows := abiertas(20)

	// Con el ancla en la 4, una caja de 3 filas solo puede ocupar de la 2 a la 4.
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
	// Y con el ancla en 1 y una caja de 3, no cabe: ahí solo hay 2 filas por encima
	// del ancla, contando la del ancla.
	if _, ok := landRow(rows, 1, 3); ok {
		t.Error("una caja de 3 filas encontró sitio con el ancla en la 1: no cabe por encima del ancla")
	}
	// Y con el ancla exactamente en la última fila que le toca, sí.
	if base, ok := landRow(rows, 2, 3); !ok || base != 2 {
		t.Errorf("con el ancla en la 2 la caja de 3 filas fue a la %d (ok=%v), want la 2", base, ok)
	}
}

// TestLandRowSubePorEncimaDeLoQueNoAdmite: si las filas de abajo no admiten
// avisos, la caja sube hasta encontrar un hueco, y si no hay ninguno, no se pinta.
//
// Esta es la degradación que hace que un aviso NUNCA rompa un marco: cae por
// dentro de alguna caja, nunca encima de un borde. Un aviso encima de un borde lo
// parte, y un marco partido se lee como un fallo de dibujo.
func TestLandRowSubePorEncimaDeLoQueNoAdmite(t *testing.T) {
	// De 10 filas, las 3 de abajo (7, 8, 9) están cerradas: son borde.
	rows := make([]bool, 10)
	for i := range rows {
		rows[i] = true
	}
	rows[7], rows[8], rows[9] = false, false, false

	// Una caja de 3 filas no cabe abajo, así que sube: de la 4 a la 6.
	base, ok := landRow(rows, 9, 3)
	if !ok {
		t.Fatal("una caja de 3 filas no encontró sitio con las 3 de abajo cerradas, y las 7 de arriba están libres")
	}
	if base != 6 {
		t.Errorf("la caja se apoyó en la fila %d, want 6 (justo encima de las cerradas)", base)
	}
	// Y NINGUNA de las filas que ocupa está cerrada. Es la promesa.
	for i := base - 3 + 1; i <= base; i++ {
		if !rows[i] {
			t.Errorf("la caja ocupa la fila %d, que no admite avisos: parte un marco", i)
		}
	}

	// Con un hueco de UNA fila entre medias, la caja también lo encuentra, aunque
	// tenga que subir más: mejor un aviso un poco más arriba que ninguno.
	rows2 := make([]bool, 10)
	for i := range rows2 {
		rows2[i] = true
	}
	rows2[8], rows2[9] = false, false
	if base, ok := landRow(rows2, 9, 2); !ok || base != 7 {
		t.Errorf("con un hueco de 2 filas abajo la caja de 2 fue a la %d (ok=%v), want la 7", base, ok)
	}

	// Y si no cabe en ninguna parte, no se pinta: es preferible un aviso ausente a
	// un aviso encima de un marco.
	ninguna := make([]bool, 5)
	if _, ok := landRow(ninguna, 4, 3); ok {
		t.Error("una caja de 3 filas encontró sitio en una ventana donde no admits ninguna fila")
	}
}

// TestAdmitenAvisoEsTodoONada: las n filas tienen que admitir TODAS. Una sola
// cerrada basta para que el sitio no valga, porque un aviso necesita su altura
// entera: medio aviso encima de un borde parte el marco igual que entero.
func TestAdmitenAvisoEsTodoONada(t *testing.T) {
	abiertas := func(n int) []bool {
		rows := make([]bool, n)
		for i := range rows {
			rows[i] = true
		}
		return rows
	}

	// Adentro del rango y todo abierto: vale.
	if !admitenAviso(abiertas(10), 0, 10) {
		t.Error("diez filas abiertas no se admitting themselves: la función está rota")
	}
	// Una fila cerrada en medio: no vale, y da igual dónde esté.
	for _, cerrada := range []int{0, 3, 6, 9} {
		rows := abiertas(10)
		rows[cerrada] = false
		if admitenAviso(rows, 0, 10) {
			t.Errorf("con la fila %d cerrada, el bloque entero se admitió: medio aviso parte el marco igual que entero", cerrada)
		}
	}
	// Fuera del rango: no vale. Una fila que no existe no es interior de nada.
	if admitenAviso(abiertas(10), 8, 5) {
		t.Error("un bloque que se sale por abajo se admitió: la fila 12 no existe")
	}
	if admitenAviso(abiertas(10), -3, 2) {
		t.Error("un bloque con un índice negativo se admitió")
	}
	// Y en el borde exacto: termina justo en la última fila, vale; se pasa de ella,
	// no.
	if !admitenAviso(abiertas(10), 8, 2) {
		t.Error("un bloque que termina justo en la última fila no se admitió")
	}
	if admitenAviso(abiertas(10), 9, 2) {
		t.Error("un bloque que termina una fila más allá de la última se admitió")
	}
	// Y cero filas: no hay nada que admitir, pero tampoco nada que pintar. La función
	// lo dice que sí porque no hay nada que se oponga; quien decide que no se pinta
	// es landRow, con su bh <= 0.
	if !admitenAviso(abiertas(10), 5, 0) {
		t.Error("un bloque de cero filas debería admitirse: no hay nada que se oponga")
	}
	// Y un slice vacío: menos filas que las pedidas, así que no.
	if admitenAviso(nil, 0, 1) {
		t.Error("un bloque sobre un slice vacío se admitió")
	}
}

// TestLandRowConUnaCajaDeAlturaCero: una caja de cero filas no se pinta. No hay
// nada que colocar, y colocar "algo" en la fila del ancla dejaría un hueco de una
// línea sin dibujo, que se lee como un fallo.
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
	// Y sobre un ancla que no existe tampoco.
	if _, ok := landRow(rows, -1, 3); ok {
		t.Error("una caja encontró sitio con un ancla negativo")
	}
}
