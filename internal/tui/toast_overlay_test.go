package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// vistaDePrueba arma una vista de n filas todas abiertas a avisos, con un ancho
// dado. El texto son líneas de puntos para poder ver de un vistazo qué filas ha
// tocado el aviso.
func vistaDePrueba(ancho, filas int) (string, []bool) {
	rows := make([]bool, filas)
	text := make([]string, filas)
	for i := range rows {
		rows[i] = true
		text[i] = strings.Repeat(".", ancho)
	}
	return strings.Join(text, "\n"), rows
}

// filasConLaCaja devuelve los índices de fila cuyo texto ya no es la línea de puntos,
// o sea las que el aviso ha tocado.
func filasConLaCaja(t *testing.T, antes, despues string) []int {
	t.Helper()
	a := strings.Split(antes, "\n")
	b := strings.Split(despues, "\n")
	if len(a) != len(b) {
		t.Fatalf("el overlay cambió el número de filas: %d -> %d", len(a), len(b))
	}
	var tocadas []int
	for i := range a {
		if a[i] != b[i] {
			tocadas = append(tocadas, i)
		}
	}
	return tocadas
}

// TestOverlayToastsCaeLoMasAbajoQuePuede: un aviso se apoya en las últimas filas
// disponibles, no en las primeras.
//
// Abajo es donde está lo que el usuario acaba de tocar, así que es donde un aviso
// se lee antes. Y es lo que hace que dos avisos apilados se lean de abajo arriba, en
// el orden en que aparecieron.
//
// Esta es la propiedad que desde el texto no se veía: el recorte de la superposición
// hace que la aritmética de las filas se compensara con los recortes de abajo, y el
// allowlist daba por no matable el `anchor+1` de la altura por "no observable desde el
// texto". Lo observable es la POSICIÓN de la caja, que se ve mirando qué filas han
// cambiado, no mirando el texto de la caja.
func TestOverlayToastsCaeLoMasAbajoQuePuede(t *testing.T) {
	const ancho, filas = 40, 10
	antes, rows := vistaDePrueba(ancho, filas)

	// Una caja de 3 filas, con el ancho de la caja, bordes incluidos. OJO: un
	// elemento de `boxes` es una CAJA entera, no una línea, así que las tres líneas
	// van unidas. Pasarlas sueltas sería pedir tres cajas de una línea, que se
	// apilarían igual pero en otro sitio.
	caja := "╭──────────╮\n│ aviso    │\n╰──────────╯"
	despues := overlayToasts(antes, []string{caja}, ancho, rows)

	tocadas := filasConLaCaja(t, antes, despues)
	if len(tocadas) != 3 {
		t.Fatalf("una caja de 3 filas tocó %d filas (%v), want 3: la caja se partió o se repitió",
			len(tocadas), tocadas)
	}
	// Y toca las ÚLTIMAS tres: de la 7 a la 9.
	for i, fila := range tocadas {
		if fila != filas-3+i {
			t.Errorf("la caja tocó la fila %d en la posición %d, want %d: un aviso cae lo más abajo que puede",
				fila, i, filas-3+i)
		}
	}

	// Y es ahí donde dice estar: la esquina de arriba en la primera fila que tocó y
	// la de abajo en la última. Si se invirtieran, la caja saldría del revés, que es
	// un aviso legible pero falso.
	lineas := strings.Split(despues, "\n")
	if !strings.Contains(lineas[filas-3], "╭") {
		t.Errorf("la primera fila de la caja debería llevar la esquina de arriba: %q", lineas[filas-3])
	}
	if !strings.Contains(lineas[filas-1], "╰") {
		t.Errorf("la última fila de la caja no lleva la esquina de abajo: %q", lineas[filas-1])
	}
	// Y en la de en medio va el texto del aviso, para que se vea que la caja está
	// entera y no solo sus bordes.
	if !strings.Contains(lineas[filas-2], "aviso") {
		t.Errorf("la fila %d debería llevar el texto del aviso: %q", filas-2, lineas[filas-2])
	}

	// Y queda pegada a la derecha con su columna de aire: la esquina de arriba
	// empieza en la columna del ancho de la vista menos el de la caja menos el aire.
	want := ancho - ansi.StringWidth("╭──────────╮") - 1
	if col := strings.Index(lineas[filas-3], "╭"); col != want {
		t.Errorf("la caja empieza en la columna %d, want %d: el aire de la derecha es de una columna", col, want)
	}
}

// TestOverlayToastsApilaHaciaArriba: un segundo aviso no pisa el primero, se pone
// encima. Y van en orden inverso al de abajo.
//
// El de abajo es el primero, porque se colocó primero con el ancla abajo. El segundo
// se coloca con el ancla ya subido, así que encima. Si se pusieran encima en orden
// inverso, el usuario leería los avisos del revés.
func TestOverlayToastsApilaHaciaArriba(t *testing.T) {
	const ancho, filas = 40, 12
	antes, rows := vistaDePrueba(ancho, filas)

	cajaA := []string{"╭──╮", "│ A│", "╰──╯"}
	cajaB := []string{"╭──╮", "│ B│", "╰──╯"}

	despues := overlayToasts(antes, []string{strings.Join(cajaA, "\n"), strings.Join(cajaB, "\n")}, ancho, rows)

	tocadas := filasConLaCaja(t, antes, despues)
	if len(tocadas) != 6 {
		t.Fatalf("dos cajas de 3 filas tocaron %d filas (%v), want 6: se pisaron", len(tocadas), tocadas)
	}

	// La primera va abajo, occupying las 3 últimas: es el primer aviso y el ancla
	// estaba abajo.
	if a := strings.Split(despues, "\n")[filas-2]; !strings.Contains(a, "A") {
		t.Errorf("la fila %d debería llevar el aviso A (el primero va abajo): %q", filas-2, a)
	}
	// Y el segundo encima: 3 filas por encima del primero, sin huecos ni solapes.
	if b := strings.Split(despues, "\n")[filas-5]; !strings.Contains(b, "B") {
		t.Errorf("la fila %d debería llevar el aviso B (el segundo va encima): %q", filas-5, b)
	}
	// Y el hueco de en medio está intacto, para que se lean como dos avisos y no
	// como un bloque.
	if intermedia := strings.Split(despues, "\n")[filas-4]; strings.Contains(intermedia, "A") || strings.Contains(intermedia, "B") {
		t.Errorf("la fila %d entre los dos avisos tiene algo: %q", filas-4, intermedia)
	}

	// Y de paso, lo que se afirma aquí es que el segundo respeta el hueco: no se
	// solapa con el primero porque su ancla es la base del primero menos su altura.
	for i, fila := range tocadas {
		want := filas - 6 + i
		if fila != want {
			t.Errorf("la fila tocada %d es la %d, want %d: los avisos se apilan sin huecos ni solapes",
				i, fila, want)
		}
	}
}

// TestOverlayToastsNoRompeUnMarco: el aviso solo cae donde la vista dice que se
// puede, y si no cabe en ningún sitio no se pinta. Es la degradación que garantiza
// que un aviso nunca parte un borde.
func TestOverlayToastsNoRompeUnMarco(t *testing.T) {
	const ancho, filas = 40, 10
	antes, rows := vistaDePrueba(ancho, filas)
	// Cierro las 4 de abajo: son borde.
	for i := filas - 4; i < filas; i++ {
		rows[i] = false
	}

	caja := []string{"╭──╮", "│ A│", "╰──╯"}
	despues := overlayToasts(antes, []string{strings.Join(caja, "\n")}, ancho, rows)

	tocadas := filasConLaCaja(t, antes, despues)
	if len(tocadas) == 0 {
		t.Fatal("la caja no se pintó en ninguna parte, y había sitio de sobra arriba")
	}
	for _, fila := range tocadas {
		if !rows[fila] {
			t.Errorf("la caja tocó la fila %d, que no admite avisos: parte un marco", fila)
		}
		if fila >= filas-4 {
			t.Errorf("la caja bajó a la fila %d, que es borde: %v", fila, tocadas)
		}
	}
	// Y se pintó lo más abajo posible DENTRO de lo permitido, que es la fila 5
	// (ocupa de la 3 a la 5, justo encima de la 6 cerrada).
	if len(tocadas) != 3 || tocadas[0] != filas-4-3 {
		t.Errorf("la caja se apoyó en las filas %v, want [%d, %d, %d]: lo más abajo sin pisar bordes",
			tocadas, filas-7, filas-6, filas-5)
	}
}

// TestOverlayToastsUnaCajaDelAltoDeLaVentana: el borde de landRow.
//
// El bucle de landRow baja mientras base >= bh-1, así que una caja cabe con la base
// en la fila 0 cuando su alto es el de la ventana. Esa es la caja del alto EXACTO de
// lo que hay, y es la única donde se nota si la altura disponible lleva un +1 de más
// o de menos.
//
// Con un +1 de menos la caja se dibujaría empezando en la fila 1 y su línea de arriba
// —la esquina del marco— no se pintaría. No es un recorte de una línea: es una caja
// sin techo, que se lee como un recuadro roto.
func TestOverlayToastsUnaCajaDelAltoDeLaVentana(t *testing.T) {
	const ancho, filas = 40, 10
	antes, rows := vistaDePrueba(ancho, filas)

	// Una caja de `filas` líneas: ni una más, ni una menos.
	lineas := make([]string, filas)
	for i := range lineas {
		lineas[i] = "│" + strings.Repeat("·", ancho-2) + "│"
	}
	lineas[0] = "╭" + strings.Repeat("─", ancho-2) + "╮"
	lineas[filas-1] = "╰" + strings.Repeat("─", ancho-2) + "╯"
	caja := strings.Join(lineas, "\n")

	despues := overlayToasts(antes, []string{caja}, ancho, rows)

	tocadas := filasConLaCaja(t, antes, despues)
	if len(tocadas) != filas {
		t.Fatalf("una caja de %d filas en una ventana de %d tocó %d filas (%v), want %d: la caja no entra entera",
			filas, filas, len(tocadas), tocadas, filas)
	}
	// Y las toca TODAS, de la primera a la última. Aquí es donde se ve el +1: si la
	// altura disponible fuera una fila menos, la fila 0 se quedaría sin pintar y la
	// caja saldría sin la línea de arriba.
	for i, fila := range tocadas {
		if fila != i {
			t.Errorf("la fila tocada %d es la %d, want %d: la caja tiene que pintar hasta arriba", i, fila, i)
		}
	}
	// Y la esquina de arriba está, que es la prueba de que la caja está entera.
	if !strings.Contains(strings.Split(despues, "\n")[0], "╭") {
		t.Errorf("la primera fila no lleva la esquina de arriba: la caja salió sin techo")
	}
	if !strings.Contains(strings.Split(despues, "\n")[filas-1], "╰") {
		t.Errorf("la última fila no lleva la esquina de abajo")
	}
}

// TestOverlayToastsLaCajaEsSiempreUnRectangulo: el ancho de la caja sale de su
// PRIMERA línea, y da igual cuál se mida porque todas miden lo mismo.
//
// Eso es un invariante del PRODUCTOR, no de la superposición: `render` le pasa a
// lipgloss un ancho fijo, así que todas las líneas de la caja salen midiendo lo
// mismo. Medir la última daría el mismo número, y por eso el mutante no se ve.
//
// El invariante se afirma en el productor, que es donde está la verdad: si algún día
// una caja saliera con líneas de distinto ancho, esta aserción lo delataría antes de
// que la superposición lo notara.
func TestOverlayToastsLaCajaEsSiempreUnRectangulo(t *testing.T) {
	m := newToastManager()
	for _, nivel := range []toastLevel{toastSuccess, toastError, toastInfo, toastWarning, toastLevel(9)} {
		for _, mensaje := range []string{"corto", strings.Repeat("palabra ", 20), "con\nsaltos"} {
			for available := 0; available <= 120; available += 7 {
				lineas := strings.Split(stripANSI(m.render(toast{message: mensaje, level: nivel}, available)), "\n")
				want := ansi.StringWidth(lineas[0])
				for i, l := range lineas {
					if got := ansi.StringWidth(l); got != want {
						t.Errorf("nivel %d, mensaje de %d, %d libres: la línea %d mide %d y la primera %d: "+
							"la caja tiene que ser un rectángulo",
							nivel, len(mensaje), available, i, got, want)
						break
					}
				}
			}
		}
	}
}

// TestOverlayToastsSinSitioNoSePinta: si la vista no admite ninguna fila, el aviso no
// se pinta. Un aviso ausente no informa de nada, pero un aviso encima de un marco
// ROMPE el marco, que es peor.
func TestOverlayToastsSinSitioNoSePinta(t *testing.T) {
	const ancho, filas = 40, 6
	antes, rows := vistaDePrueba(ancho, filas)
	for i := range rows {
		rows[i] = false
	}

	caja := []string{"╭──╮", "│ A│", "╰──╯"}
	despues := overlayToasts(antes, []string{strings.Join(caja, "\n")}, ancho, rows)

	if despues != antes {
		t.Errorf("la caja se pintó en una vista que no admite ninguna fila:\n%s", despues)
	}
	// Y con más avisos que filas libres, los que sobren se descartan en orden: se
	// ve el último, que es el más reciente, y los antiguos se caen. Al revés sería
	// tirar el aviso que el usuario acaba de provocar.
	abiertas := func() []bool {
		r := make([]bool, filas)
		// Solo la fila 0 y la 4 admiten: sitio para un aviso de 1 fila cada una.
		r[0], r[4] = true, true
		return r
	}
	uno := []string{"1"}
	dos := []string{"2"}
	tres := []string{"3"}
	despues = overlayToasts(antes, []string{uno[0], dos[0], tres[0]}, ancho, abiertas())
	lineas := strings.Split(despues, "\n")
	if !strings.Contains(lineas[4], "1") {
		t.Errorf("la fila 4 no lleva el aviso 1: %q", lineas[4])
	}
	if !strings.Contains(lineas[0], "2") {
		t.Errorf("la fila 0 no lleva el aviso 2: %q", lineas[0])
	}
	if strings.Contains(lineas[0], "3") || strings.Contains(lineas[4], "3") {
		t.Error("el aviso 3 se pintó y no había sitio: debería descartarse el más viejo")
	}
}
