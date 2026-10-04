package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func vistaDePrueba(ancho, filas int) (string, []bool) {
	rows := make([]bool, filas)
	text := make([]string, filas)
	for i := range rows {
		rows[i] = true
		text[i] = strings.Repeat(".", ancho)
	}
	return strings.Join(text, "\n"), rows
}

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

func TestOverlayToastsCaeLoMasAbajoQuePuede(t *testing.T) {
	const ancho, filas = 40, 10
	antes, rows := vistaDePrueba(ancho, filas)

	caja := "╭──────────╮\n│ aviso    │\n╰──────────╯"
	despues := overlayToasts(antes, []string{caja}, ancho, rows)

	tocadas := filasConLaCaja(t, antes, despues)
	if len(tocadas) != 3 {
		t.Fatalf("una caja de 3 filas tocó %d filas (%v), want 3: la caja se partió o se repitió",
			len(tocadas), tocadas)
	}
	for i, fila := range tocadas {
		if fila != filas-3+i {
			t.Errorf("la caja tocó la fila %d en la posición %d, want %d: un aviso cae lo más abajo que puede",
				fila, i, filas-3+i)
		}
	}

	lineas := strings.Split(despues, "\n")
	if !strings.Contains(lineas[filas-3], "╭") {
		t.Errorf("la primera fila de la caja debería llevar la esquina de arriba: %q", lineas[filas-3])
	}
	if !strings.Contains(lineas[filas-1], "╰") {
		t.Errorf("la última fila de la caja no lleva la esquina de abajo: %q", lineas[filas-1])
	}
	if !strings.Contains(lineas[filas-2], "aviso") {
		t.Errorf("la fila %d debería llevar el texto del aviso: %q", filas-2, lineas[filas-2])
	}

	want := ancho - ansi.StringWidth("╭──────────╮") - 1
	if col := strings.Index(lineas[filas-3], "╭"); col != want {
		t.Errorf("la caja empieza en la columna %d, want %d: el aire de la derecha es de una columna", col, want)
	}
}

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

	if a := strings.Split(despues, "\n")[filas-2]; !strings.Contains(a, "A") {
		t.Errorf("la fila %d debería llevar el aviso A (el primero va abajo): %q", filas-2, a)
	}
	if b := strings.Split(despues, "\n")[filas-5]; !strings.Contains(b, "B") {
		t.Errorf("la fila %d debería llevar el aviso B (el segundo va encima): %q", filas-5, b)
	}
	if intermedia := strings.Split(despues, "\n")[filas-4]; strings.Contains(intermedia, "A") || strings.Contains(intermedia, "B") {
		t.Errorf("la fila %d entre los dos avisos tiene algo: %q", filas-4, intermedia)
	}

	for i, fila := range tocadas {
		want := filas - 6 + i
		if fila != want {
			t.Errorf("la fila tocada %d es la %d, want %d: los avisos se apilan sin huecos ni solapes",
				i, fila, want)
		}
	}
}

func TestOverlayToastsNoRompeUnMarco(t *testing.T) {
	const ancho, filas = 40, 10
	antes, rows := vistaDePrueba(ancho, filas)
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
	if len(tocadas) != 3 || tocadas[0] != filas-4-3 {
		t.Errorf("la caja se apoyó en las filas %v, want [%d, %d, %d]: lo más abajo sin pisar bordes",
			tocadas, filas-7, filas-6, filas-5)
	}
}

// landRow's boundary.
func TestOverlayToastsUnaCajaDelAltoDeLaVentana(t *testing.T) {
	const ancho, filas = 40, 10
	antes, rows := vistaDePrueba(ancho, filas)

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
	for i, fila := range tocadas {
		if fila != i {
			t.Errorf("la fila tocada %d es la %d, want %d: la caja tiene que pintar hasta arriba", i, fila, i)
		}
	}
	if !strings.Contains(strings.Split(despues, "\n")[0], "╭") {
		t.Errorf("la primera fila no lleva la esquina de arriba: la caja salió sin techo")
	}
	if !strings.Contains(strings.Split(despues, "\n")[filas-1], "╰") {
		t.Errorf("la última fila no lleva la esquina de abajo")
	}
}

// The box's width comes from its FIRST line.
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
	abiertas := func() []bool {
		r := make([]bool, filas)
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
