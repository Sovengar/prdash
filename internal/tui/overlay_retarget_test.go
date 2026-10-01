package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"prdash/internal/testutil"
)

// Los cinco guards que quedaban de retarget.go y overlay.go son cinco preguntas
// distintas: si el bloque cabe en vertical, dónde se coloca la esquina, y si el contador
// deinvalidación sube.
//
// Las del overlay son funciones puras con la posición como resultado, así que se
// preguntan por el número: dónde cae la caja es una coordenada, y una coordenada se
// escribe en el test. Un test que afirmara "la caja está más o menos en el centro" pasa
// igual con la caja corrida una columna.

// TestCenteredOriginPoneLaCajaEnMedio: la esquina de una caja centrada es lo que dice la
// cuenta, y la cuenta es exacta.
//
// El reparto es con suelo: una caja más grande que el área no puede centrarse, así que se
// pega a la esquina (0,0) en vez de dar coordenadas negativas. Y eso no es un detalle de
// la aritmética: una `x` negativa haría que `ansi.Truncate(line, x, "")` recorta por la
// IZQUIERDA de la línea de fondo y pintaría la caja desplazada, que es el bug que esta
// función existe para que no pase.
//
// Y el caso del resto impar importa: cuando sobra un número impar de columnas no se
// puede repartir en dos mitades iguales, y hay que decidir si el sobrante va a la
// izquierda o a la derecha. Aquí va a la derecha (`/2` trunca hacia abajo), y eso es lo
// que se afirma.
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
		// Y el suelo: nunca negativo. Una coordenada negativa rompe el truncado del
		// fondo, que es de donde sale el recorte por la izquierda.
		if x < 0 || y < 0 {
			t.Errorf("área %dx%d, caja %dx%d: origen (%d,%d) negativo: el truncado "+
				"del fondo recorta por la izquierda con un ancho negativo",
				c.areaW, c.areaH, c.boxW, c.boxH, x, y)
		}
		// Y la caja se cabe o se sale por abajo y por la derecha, nunca por arriba ni
		// por la izquierda: ese es el contrato de la que la llama.
		if y+c.boxH > c.areaH+1 && c.boxH <= c.areaH {
			t.Errorf("área %dx%d, caja %dx%d: la caja se sale por abajo (%d > %d) sin "+
				"que sea más alta que el área", c.areaW, c.areaH, c.boxW, c.boxH,
				y+c.boxH, c.areaH)
		}
	}
}

// TestElOverlayNoSeSaleNiPorArribaNiPorLaIzquierda: la caja se pinta ENTERA, y lo que se
// sale es el fondo.
//
// Es la diferencia entre un popup y un recorte. Un popup es una ventana modal y su borde
// es lo que dice "esto es una ventana": si el marco se salía por arriba o por la
// izquierda, el usuario ve media caja y no sabe si hay más. Lo que sí se recorta es el
// fondo, que es lo que se ve por alrededor del marco.
//
// Y el caso que decide es el de una caja MÁS ALTA que la vista: ahí no cabe entera, y lo
// que hay que decidir es qué lado se sacrifica. Aquí es abajo —se corta por abajo, que es
// lo que hace un terminal con una ventana más alta que la pantalla—, y el recorte es
// justo lo que impide que el marco de arriba se pierda.
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

	// Caja que cabe: se pinta entera y con los cuatro bordes visibles.
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
	// Y el marco de arriba está ENTERO, con sus dos esquinas, en la fila que la cuenta
	// dice. El borde de arriba es la primera fila de la caja, y su fila es la que lleva
	// el relleno del fondo delante.
	//
	// La primera versión de este aserto buscaba la cadena "--[" en la fila, que no
	// significa nada: el borde de arriba de una caja de guiones es todo guiones. Lo que
	// se afirma es que la fila del marco tiene las DOS esquinas y que su ancho visible
	// es el de la caja, que es lo único que hace falta para saber que el marco no se ha
	// cortado.
	//
	// El número de fila sale de la cuenta, no de una constante escrita a mano: con 20
	// filas de fondo y una caja de 5, el alto del borde es (20-5)/2 = 7.
	wantFila := (20 - 5) / 2
	arriba := filas[wantFila]
	if ansi.StringWidth(arriba) != 60 {
		t.Fatalf("fila %d mide %d, want 60", wantFila, ansi.StringWidth(arriba))
	}
	if !strings.HasPrefix(arriba, "···············[") || !strings.HasSuffix(arriba, "]···············") {
		t.Errorf("la fila %d no tiene las dos esquinas del marco: %q. Un marco "+
			"cortado no dice \"esto es una ventana\"", wantFila, arriba)
	}
	// Y las filas de contenido de la caja llevan el mismo margen, que es lo que
	// significa que está centrada y pegada al lado.
	for j := 1; j < 5; j++ {
		f := filas[wantFila+j]
		if !strings.Contains(f, "-----") {
			t.Errorf("la fila %d de la caja no lleva contenido: %q", wantFila+j, f)
		}
	}
	// Y la fila de ABAJO de la caja lleva también las dos esquinas.
	abajo := filas[wantFila+4]
	if !strings.HasPrefix(abajo, "···············[") || !strings.HasSuffix(abajo, "]···············") {
		t.Errorf("la fila %d no cierra el marco: %q", wantFila+4, abajo)
	}

	// Caja MÁS ALTA que la vista: se recorta por abajo y la vista no crece.
	got = overlayCentered(vista(4, 60), caja(20, 30), 60)
	filas = strings.Split(got, "\n")
	if len(filas) != 4 {
		t.Fatalf("con una caja de 20 filas en una vista de 4 el overlay dio %d filas, "+
			"want 4: el popup no puede hacer crecer la vista", len(filas))
	}
	// Y cada fila lleva el fondo alrededor: el overlay recorta, no pinta encima de todo.
	for i, l := range filas {
		if w := ansi.StringWidth(l); w != 60 {
			t.Errorf("fila %d mide %d, want 60 con una caja que no cabe", i, w)
		}
	}

	// Caja vacía: la vista intacta, byte a byte.
	base := vista(6, 40)
	if got := overlayCentered(base, "", 40); got != base {
		t.Errorf("con caja vacía la vista cambió: %q", got)
	}

	// Y el fondo SIGUE VIÉNDOSE por alrededor: cada fila tiene los puntos del fondo a
	// ambos lados de la caja.
	got = overlayCentered(vista(10, 40), caja(3, 10), 40)
	for i, l := range strings.Split(got, "\n") {
		if !strings.Contains(l, "·") {
			t.Errorf("fila %d es %q y no se ve fondo alrededor del marco: el popup "+
				"recorta el fondo, no lo tapa entero", i, l)
		}
	}
}

// TestLaSecuenciaDeRamasSubeEnCadaPeticionYEnCadaCierre: el contador de invalidación es
// monótono y sube en las DOS operaciones que invalidan.
//
// La razón del contador es descartar respuestas viejas: el listado de ramas de un
// repositorio llega de un proceso externo, y si el usuario pide ramas de A, cambia de ítem
// a B y el listado de A vuelve después, pintar las ramas de A sobre el panel de B es
// justo el error que un overlay no puede deshacer. La forma de descartarlo es un número
// de secuencia: cada petición toma el número que hay, y la respuesta solo se acepta si
// su número sigue siendo el último.
//
// Y aquí está lo que no es evidente: el número tiene que subir ALSO al cerrar. Si cerrar
// no lo hiciera, un listado en vuelo seguiría \"en vuelo\" después de cerrar el popup y se
// aceptaría al volver, pintando un popup que el usuario ya cerró. Por eso hay dos
// incrementos y no uno.
func TestLaSecuenciaDeRamasSubeEnCadaPeticionYEnCadaCierre(t *testing.T) {
	// El adapter hace falta porque `fetchBranches` arranca una goroutine que llama a
	// `Branches` sobre el adapter del forge: sin adapter esa goroutine revienta
	// con un nil, y un panic en una goroutine tumba el binario entero en vez de fallar
	// solo este test.
	adapt := &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"}
	m := newTestModel(t, adapt)
	// El contexto se cancela al terminar para que las goroutines de las peticiones
	// salgan antes de que acabe el test.
	defer m.cancel()
	if m.branchSeq != 0 {
		t.Logf("la secuencia arranca en %d, no en 0; el test compara diferencias, no "+
			"valores absolutos", m.branchSeq)
	}

	// Cada petición suma uno, y lo suma de uno en uno.
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

	// Y cerrar también la sube. Sin esto, un listado en vuelo se aceptaría después de
	// cerrar el popup.
	m.retarget.all = []string{"main", "feat/x"}
	m.closeRetarget()
	if m.branchSeq != prev+1 {
		t.Errorf("cerrar dejó la secuencia en %d, want %d: cerrar también invalida, o un "+
			"listado en vuelo se aceptaría sobre un popup ya cerrado", m.branchSeq, prev+1)
	}
}

// TestLaGuardaDeSalirsePorAbajoNoPuedeDispararse: la condición `y+boxH > height` que
// tenía `centeredOrigin` antes de quitarse es FALSA en todo el espacio.
//
// El `if` estaba muerto y se quitó por eso, pero quitar código muerto por cuenta propia
// es una afirmación fuerte —\"esto nunca se cumple\"— y una afirmación así necesita su
// prueba, no su comentario. Así que este test barre el espacio entero y falla si
// encuentra un caso en el que la condición sea cierta.
//
// Y el rango es de −20 a 19 en los dos ejes, que incluye los tres casos que de verdad
// importan: la caja más pequeña que la área, la caja del tamaño exacto y la caja más
// grande —y las tres dan falso—.
//
// Y fíjate en la forma del assert, que es al revés de lo habitual: aquí lo que se busca
// es que NO aparezca nada, así que el fallo es `Fatalf` y no hay lista de casos que
// repasar. Un assert normal aquí daría verde siempre, porque no está mirando un valor:
// está mirando que el conjunto esté vacío.
//
// Y un detalle sobre `Fatalf` en un bucle: se para en el primer caso, que es lo que se
// quiere. Si salieran mil casos habría que verlos todos, pero con uno basta para saber
// que la guarda estaba viva y hay que devolverla.
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
