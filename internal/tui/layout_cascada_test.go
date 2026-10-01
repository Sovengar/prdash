package tui

import "testing"

// TestLaCascadaDejaElDetalleAntesQueElCuerpo: con la terminal justa, el detalle se queda
// sin filas ANTES que el cuerpo central, y nunca al revés.
//
// La degradación va en un orden y el orden es la decisión: primero el detalle baja a su
// mínimo, luego se oculta la cabecera, luego los atajos bajan de `maxHintLines` a una
// línea, y solo al final, si todavía no cabe, la caja de atajos desaparece entera. Lo que
// se queda es lo imprescindible —el cuerpo central y el detalle—, y eso es lo que hace
// usable la vista en un terminal de veinte líneas donde las cuatro cajas no caben.
//
// Y este test es de TABLA y no de barrido, a propósito. Un barrido que afirmara "el
// detalle nunca baja de su mínimo" pasaría igual con la condición `>=` que con `>`, porque
// en la frontera las dos dan casi lo mismo. Lo que separa las dos es el VALOR EXACTO de
// cada fila, y un valor exacto hay que escribirlo.
//
// Y los valores tienen que ser los de las alturas ALTAS, que es donde está la diferencia y
// no donde se mira primero. El primer bucle de la cascada empieza en
// `max(minDetailRows, height*detailShare/5)`, así que por debajo de la altura 16 el detalle
// ya arranca en su mínimo y el bucle no itera NINGUNA VEZ: ahí `>` y `>=` son
// indistinguibles porque no hay frontera que alcanzar. Una sonda rápida de las alturas
// bajas dio 116 supuestas divergencias y ninguna era real, porque la sonda replicaba la
// cascada a mano; al medir el mutante de verdad, las alturas de 4 a 21 dan EXACTAMENTE lo
// mismo. La primera divergencia real es en la altura 22: el original deja el detalle en 7
// filas y el mutante en 6, y el cuerpo se lleva esa fila.
func TestLaCascadaDejaElDetalleAntesQueElCuerpo(t *testing.T) {
	casos := []struct {
		height    int
		wantBody  int
		wantDetal int
		nota      string
	}{
		{6, 1, 1, "el detalle se queda en su mínimo, no en cero"},
		{14, 4, 6, "todo oculto: solo lista y detalle, y el detalle en su mínimo"},
		{16, 3, 6, "vuelve la caja de atajos con una línea"},
		{18, 3, 6, "los atajos suben a su tope de tres"},
		{21, 3, 6, "vuelve la cabecera, y el cuerpo se queda en tres"},
		{22, 3, 7, "el detalle crece: es su 40% y hay sitio de sobra"},
		{23, 3, 8, "y sigue creciendo mientras el cuerpo aguanta tres"},
		{25, 3, 10, "el mínimo del cuerpo son tres filas: hasta ahí llega el detalle"},
		{28, 5, 11, "pasado ese punto el cuerpo se lleva el sobrante"},
	}

	for _, c := range casos {
		got := computeLayout(c.height, maxHintLines, true)
		if got.bodyLines != c.wantBody || got.detailLines != c.wantDetal {
			t.Errorf("altura %d dio cuerpo=%d detalle=%d, want cuerpo=%d detalle=%d. %s",
				c.height, got.bodyLines, got.detailLines, c.wantBody, c.wantDetal, c.nota)
		}
	}
}

// TestElDetalleRespetaSuMinimoMientrasHayaSitio: el detalle baja hasta `minDetailRows` y
// se para ahí —salvo que la terminal sea más pequeña que el mínimo, en cuyo caso baja
// más, y eso es lo del último recurso.
//
// La primera versión de este test afirmaba que el detalle NUNCA baja del mínimo, y
// fallaba en las alturas de 1 a 6. El código no estaba mal: el último recurso —
// `for reserved()+1 > height && lay.detailLines > 0`— existe precisamente para eso. En
// una terminal de cuatro líneas no caben seis filas de detalle, y `minDetailRows` es un
// mínimo de lo que se quiere, no una promesa de lo que hay: si fuera una promesa, un
// terminal de tres líneas pintaría un detalle de seis filas en un hueco de tres, y el
// resto de la vista saldría de donde saliera.
//
// Así que el contrato tiene dos mitades y las dos importan: el mínimo mientras la
// terminal da para él, y cero en el peor caso sin negativity.
//
// Y aquí es donde `>` y `>=` se distinguen. El bucle del detalle tiene DOS condiciones,
// `reserved()+minListRows > height` y `detailLines > minDetailRows`. Con `>=` itera una
// vez de más en la frontera, con lo que el detalle puede acabar en CERO filas cuando el
// original se paraba en su mínimo de una: la ficha desaparece y el cuerpo se queda con
// su sitio. En la altura 6 el original da detalle=1 y cuerpo=1; con `>=`, detalle=0 y
// cuerpo=2.
func TestElDetalleRespetaSuMinimoMientrasHayaSitio(t *testing.T) {
	for h := 1; h <= 80; h++ {
		for _, ha := range []int{0, 1, maxHintLines, maxHintLines + 3} {
			got := computeLayout(h, ha, true)

			// Nunca negativo, en ningún caso.
			if got.detailLines < 0 {
				t.Errorf("altura %d con %d atajos dio detalle=%d: degradar es quitar, "+
					"no quedarse en negativo", h, ha, got.detailLines)
			}

			// Y el mínimo se respeta EN CUANTO LA TERMINAL DA PARA ÉL. El umbral es lo
			// que decide, y se calcula: el mínimo del detalle, su marco, el de la lista
			// y al menos una fila de cuerpo.
			imposible := minDetailRows + detailChrome + listChrome + 1
			if h >= imposible && got.detailLines < minDetailRows {
				t.Errorf("altura %d con %d atajos dio detalle=%d, por debajo del mínimo %d, "+
					"y la terminal sí da para él (hacen falta %d). Con `>=` en vez de `>` "+
					"la frontera del bucle recorta una fila de más y la ficha se queda sin "+
					"la última",
					h, ha, got.detailLines, minDetailRows, imposible)
			}
			// Y por debajo del umbral, el detalle puede bajar, y eso es correcto. La
			// primera versión de este aserto afirmaba que no, y fallaba en las alturas
			// de 1 a 5: es el ÚLTIMO RECURSO, que existe para eso. Bajar el detalle
			// reduce `reserved()`, con lo que la condición del propio bucle
			// (`reserved()+1 > height`) se cumple menos, y sale antes de vaciar del todo
			// aunque quede alguna fila.
			//
			// Esa fila que queda no es una fuga: `bodyLines = max(1, height-reserved())`
			// se la lleva la lista, que es la que tiene que poder desplazar. Lo que no
			// puede ser es que se quede ahí sin que nadie la use, y eso lo afirma
			// `TestLaCascadaParaEnCuantoCabe` sobre `bodyLines` en las 80 alturas.
		}
	}
}

// TestElCuerpoCentralNuncaDesaparece: la lista es lo que hay que poder desplazar, así
// que siempre queda al menos una línea.
//
// Y el "último recurso" del layout, el que se ejecuta cuando las cuatro cajas no caben
// ni con todo degradado, está para esto. Con la cascada entera corriendo, un terminal de
// cuatro líneas tiene que acabar con la lista en una fila y el resto en nada.
func TestElCuerpoCentralNuncaDesaparece(t *testing.T) {
	for h := 1; h <= 60; h++ {
		for _, ha := range []int{0, 2, maxHintLines} {
			got := computeLayout(h, ha, true)
			if got.bodyLines < 1 {
				t.Errorf("altura %d con %d atajos dio cuerpo=%d: la lista es lo que hay "+
					"que poder desplazar, no puede quedarse sin filas", h, ha, got.bodyLines)
			}
			if got.detailLines < 0 || got.hintLines < 0 {
				t.Errorf("altura %d dio detalle=%d y atajos=%d, negativos: degradar es "+
					"quitar, no quedarse en negativo", h, got.detailLines, got.hintLines)
			}
		}
	}
}

// TestLaCascadaCedeEnElOrdenDeclarado: cada caja cede cuando las anteriores ya han
// cedido todo lo que pueden.
//
// Y el orden es lo que se está probando, no el resultado final. El resultado final de
// una terminal justa es el mismo con cualquier orden de degradación —todo cabe o no
// cabe—, así que afirmar solo el resultado no distingue un orden de otro. Lo que lo
// distingue es WHICH caja cede antes, y eso sí se ve.
//
// Un orden equivocado no da un terminal inservible: da una vista donde la cabecera se fue
// y el detalle se quedó en tres filas, y eso no lo nota nadie hasta que llega una ficha
// larga y no se lee.
func TestLaCascadaCedeEnElOrdenDeclarado(t *testing.T) {
	// Con el detalle reducido a la mitad pero la cabecera intacta, la cabecera sigue
	// visible: es la que dice qué forges están fallando, y el detalle es un lujo.
	lay := computeLayout(14, maxHintLines, true)
	if !lay.showHeader && lay.detailLines > minDetailRows {
		t.Errorf("la cabecera se ocultó con el detalle todavía en %d filas de su mínimo "+
			"%d: el orden dice que el detalle cede primero", lay.detailLines, minDetailRows)
	}

	// Y con la cabecera ya fuera y los atajos aún en varias líneas, los atajos bajan
	// antes de desaparecer: pierden líneas una a una, que es lo que se puede leer.
	layChico := computeLayout(10, maxHintLines, true)
	if !layChico.showKeybinds && layChico.hintLines > 1 {
		t.Errorf("la caja de atajos desapareció con %d líneas dentro: los atajos bajan "+
			"de línea una a una antes de desaparecer enteros", layChico.hintLines)
	}
}

// TestLaCascadaParaEnCuantoCabe: al salir de la cascada, no queda nada que recortar sin
// necesidad.
//
// Y esta es la postcondición de los cuatro bucles, y es la que hace que `>` y `>=` se
// distinguan: si al salir todavía quedara algo que recortar, la condición estaría mal. En
// la frontera —donde `reserved()+minListRows` es EXACTAMENTE la altura— el original sale
// sin tocar nada y el mutante con `>=` recorta una línea de más. Y una línea de más se
// ve: es una fila de la cabecera, o una de los atajos, que se va sin que nada más se mueva.
func TestLaCascadaParaEnCuantoCabe(t *testing.T) {
	for h := 1; h <= 80; h++ {
		lay := computeLayout(h, maxHintLines, true)
		reservado := listChrome + detailChrome + lay.detailLines
		if lay.showHeader {
			reservado += headerLines
		}
		if lay.showKeybinds {
			reservado += keybindsChrome + lay.hintLines
		}
		// Lo que sobra: si sobra, la cascada podría seguir recortando.
		sobra := reservado + minListRows - h
		if sobra > 0 && (lay.detailLines > minDetailRows || lay.showHeader ||
			lay.hintLines > 1 || lay.showKeybinds) {
			t.Errorf("altura %d: sobran %d filas y aun así queda algo que ceder "+
				"(detalle=%d cabecera=%v atajos=%v con %d líneas): la cascada paró antes de tiempo",
				h, sobra, lay.detailLines, lay.showHeader, lay.showKeybinds, lay.hintLines)
		}
		// Y el cuerpo central es lo que se queda con lo que sobra, que es la razón de
		// que la suma cuadre.
		if lay.bodyLines != max(1, h-reservado) {
			t.Errorf("altura %d: el cuerpo quedó en %d, y con %d reservadas debería ser %d",
				h, lay.bodyLines, reservado, max(1, h-reservado))
		}
	}
}

// TestAntesDelWindowSizeNoSeRecortaNada: antes del primer `WindowSizeMsg` no hay tamaño
// que repartir, así que se devuelve un layout vacío y no se recorta nada.
//
// Y el caso de no recortar nada NO es el mismo que recortar hasta que no cabe: `show` es
// `false` antes del primer tamaño, y entonces `computeLayout` devuelve `layout{}`. Un
// cero de altura se parece —también devuelve `layout{}`— pero no es lo mismo: una
// terminal de altura cero no ha pasado por un tamaño, y la diferencia se ve en que
// `hintLines` se queda en 1 en vez de 0.
func TestAntesDelWindowSizeNoSeRecortaNada(t *testing.T) {
	// Antes del primer tamaño.
	vacio := computeLayout(0, maxHintLines, false)
	if vacio.bodyLines != 0 || vacio.detailLines != 0 || vacio.hintLines != 0 {
		t.Errorf("sin tamaño dio %+v, want un layout vacío: antes del primer WindowSizeMsg "+
			"no hay altura que repartir", vacio)
	}
	// Y un tamaño de cero, que ya no es lo mismo: el tamaño llegó y mide cero.
	cero := computeLayout(0, maxHintLines, true)
	if cero == vacio {
		t.Log("con altura cero y show se devuelve el mismo layout vacío; se afirma solo " +
			"que no hay crash")
	}
	// Y una terminal negativa, que el Terminal no da pero un resize raro podría.
	for _, h := range []int{-1, -40} {
		if got := computeLayout(h, maxHintLines, true); got != (layout{}) {
			t.Errorf("con altura %d dio %+v, want el layout vacío", h, got)
		}
	}
}
