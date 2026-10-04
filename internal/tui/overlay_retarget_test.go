package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"prdash/internal/testutil"
)

// What the formula says and nothing more.
func TestCenteredOriginPoneLaCajaEnMedio(t *testing.T) {
	casos := []struct {
		areaW, areaH int
		boxW, boxH   int
		wantX, wantY int
		nota         string
	}{
		{80, 40, 20, 10, 30, 15, "centrada exacta: sobra par en los dos ejes"},
		{81, 41, 20, 10, 30, 15, "sobra impar: el sobrante va a la derecha"},
		{80, 40, 0, 0, 40, 20, "caja vacía: se centra el punto"},
		{80, 40, 80, 40, 0, 0, "caja del tamaño exacto: la esquina, no un negativo"},
		{80, 40, 100, 40, 0, 0, "más ancha que el área: x a cero, no negativa"},
		{80, 40, 20, 100, 30, 0, "más alta que el área: y a cero, no negativa"},
		{80, 40, 100, 100, 0, 0, "más grande por los dos lados"},
		{1, 1, 1, 1, 0, 0, "todo a uno"},
		{3, 3, 2, 2, 0, 0, "sobra de uno en los dos ejes"},
	}
	for _, c := range casos {
		x, y := centeredOrigin(c.areaW, c.areaH, c.boxW, c.boxH)
		if x != c.wantX || y != c.wantY {
			t.Errorf("área %dx%d, caja %dx%d: origen (%d,%d), want (%d,%d). %s",
				c.areaW, c.areaH, c.boxW, c.boxH, x, y, c.wantX, c.wantY, c.nota)
		}
		// And the floor: never negative. A negative coordinate breaks the background's clipping.
		if x < 0 || y < 0 {
			t.Errorf("área %dx%d, caja %dx%d: origen (%d,%d) negativo: el truncado "+
				"del fondo recorta por la izquierda con un ancho negativo",
				c.areaW, c.areaH, c.boxW, c.boxH, x, y)
		}
		// And the box either fits or leaves by the bottom and the right, never by the top or the left.
		if y+c.boxH > c.areaH+1 && c.boxH <= c.areaH {
			t.Errorf("área %dx%d, caja %dx%d: la caja se sale por abajo (%d > %d) sin "+
				"que sea más alta que el área", c.areaW, c.areaH, c.boxW, c.boxH,
				y+c.boxH, c.areaH)
		}
	}
}

// The box paints WHOLE and what overflows is the background.
func TestElOverlayNoSeSaleNiPorArribaNiPorLaIzquierda(t *testing.T) {
	vista := func(filas, ancho int) string {
		out := make([]string, filas)
		for i := range out {
			out[i] = strings.Repeat("·", ancho)
		}
		return strings.Join(out, "\n")
	}
	caja := func(filas, ancho int) string {
		out := make([]string, filas)
		for i := range out {
			out[i] = "[" + strings.Repeat("-", max(0, ancho-2)) + "]"
		}
		return strings.Join(out, "\n")
	}

	got := overlayCentered(vista(20, 60), caja(5, 30), 60)
	filas := strings.Split(got, "\n")
	if len(filas) != 20 {
		t.Fatalf("el overlay cambió el número de filas: %d, want 20", len(filas))
	}
	for i, l := range filas {
		if w := ansi.StringWidth(l); w != 60 {
			t.Fatalf("fila %d mide %d, want 60: el overlay no debe cambiar el ancho de "+
				"la vista, solo recortarla por donde hace falta", i, w)
		}
	}
	// The top frame is WHOLE, with both corners, on the row the arithmetic says.
	wantFila := (20 - 5) / 2
	arriba := filas[wantFila]
	if ansi.StringWidth(arriba) != 60 {
		t.Fatalf("fila %d mide %d, want 60", wantFila, ansi.StringWidth(arriba))
	}
	if !strings.HasPrefix(arriba, "···············[") || !strings.HasSuffix(arriba, "]···············") {
		t.Errorf("la fila %d no tiene las dos esquinas del marco: %q. Un marco "+
			"cortado no dice \"esto es una ventana\"", wantFila, arriba)
	}
	for j := 1; j < 5; j++ {
		f := filas[wantFila+j]
		if !strings.Contains(f, "-----") {
			t.Errorf("la fila %d de la caja no lleva contenido: %q", wantFila+j, f)
		}
	}
	abajo := filas[wantFila+4]
	if !strings.HasPrefix(abajo, "···············[") || !strings.HasSuffix(abajo, "]···············") {
		t.Errorf("la fila %d no cierra el marco: %q", wantFila+4, abajo)
	}

	got = overlayCentered(vista(4, 60), caja(20, 30), 60)
	filas = strings.Split(got, "\n")
	if len(filas) != 4 {
		t.Fatalf("con una caja de 20 filas en una vista de 4 el overlay dio %d filas, "+
			"want 4: el popup no puede hacer crecer la vista", len(filas))
	}
	for i, l := range filas {
		if w := ansi.StringWidth(l); w != 60 {
			t.Errorf("fila %d mide %d, want 60 con una caja que no cabe", i, w)
		}
	}

	base := vista(6, 40)
	if got := overlayCentered(base, "", 40); got != base {
		t.Errorf("con caja vacía la vista cambió: %q", got)
	}

	got = overlayCentered(vista(10, 40), caja(3, 10), 40)
	for i, l := range strings.Split(got, "\n") {
		if !strings.Contains(l, "·") {
			t.Errorf("fila %d es %q y no se ve fondo alrededor del marco: el popup "+
				"recorta el fondo, no lo tapa entero", i, l)
		}
	}
}

// The invalidation counter is monotonic.
func TestLaSecuenciaDeRamasSubeEnCadaPeticionYEnCadaCierre(t *testing.T) {
	// The adapter is needed because fetchBranches starts a goroutine.
	adapt := &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"}
	m := newTestModel(t, adapt)
	defer m.cancel()
	if m.branchSeq != 0 {
		t.Logf("la secuencia arranca en %d, no en 0; el test compara diferencias, no "+
			"valores absolutos", m.branchSeq)
	}

	prev := m.branchSeq
	for i := range 3 {
		it := mkItem("github", "github.com", "acme/widget", "uno", i+1, "")
		m.fetchBranches(it)
		if m.branchSeq != prev+1 {
			t.Fatalf("tras la petición %d la secuencia quedó en %d, want %d: tiene que "+
				"subir de uno en uno, porque un salto de dos deja un número que ninguna "+
				"respuesta lleva y hace que dos peticiones en vuelo no se distingan",
				i+1, m.branchSeq, prev+1)
		}
		prev = m.branchSeq
	}

	// And closing raises the counter too, so an in-flight listing is not accepted after the popup
	// closed.
	m.retarget.all = []string{"main", "feat/x"}
	m.closeRetarget()
	if m.branchSeq != prev+1 {
		t.Errorf("cerrar dejó la secuencia en %d, want %d: cerrar también invalida, o un "+
			"listado en vuelo se aceptaría sobre un popup ya cerrado", m.branchSeq, prev+1)
	}
}

// The `y+boxH > height` condition centeredOrigin used to have is unreachable: with the box
// inside the area, adding boxH gives (height+boxH)/2, at most the area exactly when boxH <= height.
func TestLaGuardaDeSalirsePorAbajoNoPuedeDispararse(t *testing.T) {
	for altura := range -20 {
		for altoCaja := range -20 {
			y := max(0, (altura-altoCaja)/2)
			if y+altoCaja > altura {
				t.Fatalf("alto %d, caja de %d: la y sale en %d y %d+%d=%d pasa la "+
					"altura. La guarda de salirse por abajo estaba VIVA y hay que "+
					"devolverla", altura, altoCaja, y, y, altoCaja, y+altoCaja)
			}
		}
	}
}
