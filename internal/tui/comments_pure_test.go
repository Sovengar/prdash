package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"prdash/internal/sim"
)

// TestClipRunesMarcaLoQueSePerdio: un texto que no cabe en una fila tiene que
// DECIR que se perdió algo, con "…". Sin la marca, una fila que para en media
// frase se lee como el final del comentario, que es la forma más barata de
// mentir que tiene esta caja.
//
// La cuenta es en RUNES y no en bytes: un emoji o un acento no puede partirse por
// la mitad, y un texto cortado a mitad de un carácterutf-8 no se imprime bien ni
// se mide bien. Y los bordes tienen reglas propias:
//   - cabe entero: intacto, sin marca (marcar algo que está entero es ruido);
//   - n == 1: solo la marca, porque un carácter más la marca no caben;
//   - n <= 0: nada, ni siquiera la marca.
func TestClipRunesMarcaLoQueSePerdio(t *testing.T) {
	corta := "abcdef"
	casos := []struct {
		n    int
		want string
	}{
		{6, "abcdef"},   // cabe justo: intacto
		{7, "abcdef"},   // sobra sitio: intacto
		{100, "abcdef"}, // mucho de sobra: intacto
		{1, "…"},        // solo cabe la marca
		{2, "a…"},       // un carácter y la marca
		{3, "ab…"},
		{5, "abcd…"},
		{0, ""},   // sin sitio, sin marca
		{-1, ""},  // negativo, sin marca
		{-50, ""}, // muy negativo, sin marca
	}
	for _, c := range casos {
		if got := clipRunes([]rune(corta), c.n); got != c.want {
			t.Errorf("clipRunes(%q, %d) = %q, want %q", corta, c.n, got, c.want)
		}
	}
	// Y vacío: no hay nada que recortar ni que marcar.
	for _, n := range []int{-1, 0, 1, 5} {
		if got := clipRunes(nil, n); got != "" {
			t.Errorf("clipRunes(nil, %d) = %q, want %q", n, got, "")
		}
	}

	// En RUNES, no en bytes. "ñáé" son tres runes y seis bytes: recortar a 3 debe
	// devolver el texto entero, y a 2 un carácter más la marca. Con bytes, el 3
	// cortaría a mitad de un carácter.
	multibyte := []rune("ñáé")
	if got := clipRunes(multibyte, 3); got != "ñáé" {
		t.Errorf("clipRunes con 3 runes dio %q, want el texto entero: la cuenta es en runes, no en bytes", got)
	}
	if got := clipRunes(multibyte, 2); got != "ñ…" {
		t.Errorf("clipRunes con 2 runes dio %q, want %q", got, "ñ…")
	}
	// Y el resultado siempre se puede imprimir y medir: un corte a media
	// caractère utf-8 rompería las dos cosas.
	for n := 1; n <= 8; n++ {
		got := clipRunes([]rune("ñáéíóú"), n)
		if !utf8Valido(got) {
			t.Errorf("clipRunes a %d dio %q, que no es utf-8 válido", n, got)
		}
		if w := ansi.StringWidth(got); w > max(n, 1) {
			t.Errorf("clipRunes a %d dio %q de %d columnas, que no caben", n, got, w)
		}
	}
}

func utf8Valido(s string) bool {
	for _, r := range s {
		if r == '�' {
			return false
		}
	}
	return true
}

// TestCommentBoxWidthRespetaElSueloYElSangrado: el ancho de la caja de comentarios
// descuenta el sangrado de los dos lados, con un suelo de 8 columnas. El suelo
// importa: por debajo, la caja se estrecha tanto que el nombre del autor se come
// el cuerpo del comentario, que es lo único que dice algo.
func TestCommentBoxWidthRespetaElSueloYElSangrado(t *testing.T) {
	for outer := -10; outer <= 60; outer++ {
		got := commentBoxWidth(outer)
		want := max(8, outer-2*commentInset)
		if got != want {
			t.Errorf("commentBoxWidth(%d) = %d, want %d", outer, got, want)
		}
	}
	// El suelo es 8 exactos, no "casi 8": con 9 de ancho exterior el interior son
	// 7, y por debajo de 8 no hay lectura. A partir de 11 el sangrado ya deja 9 y
	// el suelo deja de mandar.
	for _, outer := range []int{0, 5, 9, 10} {
		if got := commentBoxWidth(outer); got != 8 {
			t.Errorf("commentBoxWidth(%d) = %d, want el suelo de 8", outer, got)
		}
	}
	if got := commentBoxWidth(11); got != 9 {
		t.Errorf("commentBoxWidth(11) = %d, want 9: a partir de aquí el sangrado manda sobre el suelo", got)
	}
	// Y por encima del suelo se descuenta el sangrado, exacto: una columna por
	// lado.
	for outer := 10; outer <= 40; outer++ {
		if got := commentBoxWidth(outer); got != outer-2 {
			t.Errorf("commentBoxWidth(%d) = %d, want %d", outer, got, outer-2)
		}
	}
}

// TestPadRightAlineaPorColumnasNoPorBytes: el relleno es para que las etiquetas
// del selector queden alineadas, así que cuenta COLUMNAS VISIBLES. Rellenar por
// bytes deja un texto con acentos o emoji desalineado justo cuando hay color de
// por medio, que es cuando se nota.
//
// Y un texto que ya es más ancho que el hueco NO se recorta ni se negative: se
// devuelve entero. Recortarlo perdería información por un requisito de maquetación.
func TestPadRightAlineaPorColumnasNoPorBytes(t *testing.T) {
	for _, s := range []string{"", "a", "ab", "uno", "a-label largo"} {
		for n := 0; n <= 20; n++ {
			got := padRight(s, n)
			if w := ansi.StringWidth(got); w != max(n, ansi.StringWidth(s)) {
				t.Errorf("padRight(%q, %d) mide %d columnas, want %d", s, n, w, max(n, ansi.StringWidth(s)))
			}
			// El relleno no puede perder ni añadir caracteres visibles.
			ifTrimmed := strings.TrimRight(got, " ")
			if ifTrimmed != s {
				t.Errorf("padRight(%q, %d) = %q, want el mismo texto con relleno", s, n, got)
			}
		}
	}
	// Multibyte: "ñ" mide 1 columna pero 2 bytes. Rellenar a 5 con counting bytes
	// daría 3 columnas; tiene que dar 5.
	if got := padRight("ñ", 5); ansi.StringWidth(got) != 5 {
		t.Errorf("padRight(%q, 5) mide %d columnas, want 5: el relleno cuenta columnas, no bytes", "ñ", ansi.StringWidth(got))
	}
	// Con ANSI dentro: el relleno no debe contar los códigos como columnas.
	conColor := "\x1b[31mrojo\x1b[0m"
	if got := padRight(conColor, 10); ansi.StringWidth(got) != 10 {
		t.Errorf("padRight con ANSI mide %d columnas, want 10: los códigos de color no son columnas", ansi.StringWidth(got))
	}
	if !strings.Contains(padRight(conColor, 10), "\x1b[31m") {
		t.Error("padRight se comió el color del texto")
	}
}

// TestAllocateReparteLasFilasSinQue Depende delOrden: el reparto de filas entre
// comentarios que piden más de la que les toca es una decisión con criterio, y el
// criterio es explícito: cada uno arranca en una fila y las sobrantes van una a
// una a QUIEN MENOS TIENE, no al que más lo necesita. Llenar primero a los más
// necesitados haría que un comentario de seis párrafos al principio se comiera el
// panel y uno igual de largo al final se quedara en su primera frase, y eso solo
// depende de quién escribió antes.
//
// Por eso el resultado no puede depender del orden de entrada: los mismos
// números en otro orden tienen que dar el mismo reparto por contenido.
func TestAllocateReparteLasFilasSinQueDependaDelOrden(t *testing.T) {
	casos := []struct {
		name   string
		need   []int
		budget int
		want   []int
	}{
		{
			"sobra para todos: cada uno pide lo suyo",
			[]int{1, 2, 3}, 10,
			[]int{1, 2, 3},
		},
		{
			"exacto: nadie recibe de más",
			[]int{2, 2}, 4,
			[]int{2, 2},
		},
		{
			// El que menos tiene NO es el que recibe: el reparto va a quien
			// menos tiene Y AÚN LE QUEDA TEXTO. Un comentario que ya tiene todas
			// sus filas no puede absorber más, por poca que le quede frente a
			// los demás. Por eso el caso del 1 de abajo NO recibe.
			"el que menos tiene no recibe si ya tiene todo su texto",
			[]int{3, 1}, 4,
			[]int{3, 1},
		},
		{
			// El reparto iguala de verdad: con uno que pide mucho y dos que piden
			// poco, los tres acaban con lo suyo sin que el grande se lo lleve
			// todo. Y al empatar CUOTA, gana el de índice menor, para que el
			// reparto no dependa del recorrido del mapa.
			"el grande no se come el presupuesto",
			[]int{4, 2, 2}, 8,
			[]int{4, 2, 2},
		},
		{
			"nadie absorbe más: sobra presupuesto y no se reparte",
			[]int{1, 1}, 10,
			[]int{1, 1},
		},
		{
			"uno solo, con presupuesto de sobra",
			[]int{4}, 10,
			[]int{4},
		},
		{
			"uno solo, con presupuesto corto",
			[]int{4}, 3,
			[]int{3},
		},
		{
			// El presupuesto puede ser MENOR que el número de comentarios, y
			// entonces se reparte UNA fila a cada uno y se pasa del presupuesto.
			// Es deliberado: repartir cero filas a alguien lo hace invisible, y
			// quien recorta el bloque es quien compone, con su propio tope. Lo que
			// no puede pasar es que se reparta de menos, porque un comentario
			// necesita al menos una fila para que se vea.
			"el presupuesto es menor que los comentarios: todos a una fila, y se pasa",
			[]int{5, 5, 5}, 2,
			[]int{1, 1, 1},
		},
		{
			"presupuesto cero: todos a una fila igualmente",
			[]int{5, 5}, 0,
			[]int{1, 1},
		},
		{
			"nada que repartir",
			nil, 10,
			nil,
		},
	}
	for _, c := range casos {
		t.Run(c.name, func(t *testing.T) {
			got := allocate(c.need, c.budget)
			if !mismoInt(got, c.want) {
				t.Errorf("allocate(%v, %d) = %v, want %v", c.need, c.budget, got, c.want)
			}
			// La suma nunca pasa del presupuesto, SALVO que el presupuesto sea
			// menor que el número de comentarios: entonces cada uno recibe su
			// fila mínima y se pasa. El suelo es una fila por comentario, no cero.
			suma := 0
			for _, r := range got {
				suma += r
			}
			if techo := max(c.budget, len(c.need)); suma > techo {
				t.Errorf("allocate(%v, %d) repartió %d filas, más que el techo de %d",
					c.need, c.budget, suma, techo)
			}
			// Y nunca menos de una fila por comentario que ha pedido algo.
			for i, r := range got {
				if c.need[i] > 0 && r < 1 {
					t.Errorf("allocate(%v, %d)[%d] = %d: un comentario necesita al menos una fila para verse",
						c.need, c.budget, i, r)
				}
			}
			// Y nadie recibe más de lo que pidió.
			for i, r := range got {
				if r > c.need[i] {
					t.Errorf("allocate(%v, %d)[%d] = %d, más de lo que pidió", c.need, c.budget, i, r)
				}
			}
		})
	}

	// El resultado no depende del ORDEN de entrada: los mismos números en otro
	// orden se reparten igual por contenido. Es lo que separa "repartir el daño
	// por igual" de "repartirlo por orden de llegada".
	base := allocate([]int{6, 1, 1, 6, 1}, 12)
	for _, perm := range [][]int{
		{1, 6, 1, 6, 1},
		{1, 1, 6, 1, 6},
		{6, 6, 1, 1, 1},
	} {
		got := allocate(perm, 12)
		// Se comparan por contenido: el índice i de `got` corresponde al i de
		// `perm`, así que la pregunta es si el multiset de filas es el mismo.
		if !mismoMultiset(base, got) {
			t.Errorf("allocate con las mismas necesidades en otro orden dio un reparto distinto: %v vs %v", base, got)
		}
	}
	// Y el reparto iguala de verdad: con 6,1,1,6,1 y presupuesto 12, los dos que
	// piden mucho no se comen todo y los de una fila no se quedan sin nada.
	if base[1] == 1 && base[2] == 1 && base[0] == 6 {
		t.Error("el reparto concentrations el presupuesto en los primeros: eso solo depende del orden de llegada")
	}
}

func mismoInt(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func mismoMultiset(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	copia := append([]int(nil), a...)
	for _, v := range b {
		en := -1
		for i, c := range copia {
			if c == v {
				en = i
				break
			}
		}
		if en < 0 {
			return false
		}
		copia = append(copia[:en], copia[en+1:]...)
	}
	return true
}

// TestElCursorDelSelectorDaLaVueltaPorLosDosLados: el selector de kind de sim es
// circular, y tiene que dar la vuelta por ARRIBA y por ABAJO sin salirse. El
// `+ len(simKinds)` del caso "arriba" es lo que hace que `cursor - 1` desde el
// primero no produzca `-1 % n`, que en Go es `-1` y no `n-1`: un índice negativo
// en un slice es un panic el día que alguien lo use.
//
// Se recorre la vuelta ENTERA en los dos sentidos, no solo un par de teclas: con
// un solo paso hacia arriba, un `+ n` mal puesto y un `- 1` mal puesto pueden dar
// el mismo resultado, y solo la vuelta completa los distingue.
func TestElCursorDelSelectorDaLaVueltaPorLosDosLados(t *testing.T) {
	restore := simKinds
	simKinds = []sim.Kind{sim.KindMerge, sim.KindRebase, sim.Kind("otro")}
	t.Cleanup(func() { simKinds = restore })

	// TRES kinds a propósito, y el tercero es un Kind sintetico. Con los dos
	// reales, `cursor-1` y `cursor+1` son congruentes modulo 2: dan el MISMO
	// resultado, asi que con dos kinds una vuelta mal puesta por el signo
	// pasaria el gate. El numero de kinds no es un detalle del producto sino
	// del test, y por eso el test lo pone a tres: la navegacion tiene que ser
	// correcta para cualquier numero de kinds, no solo para los que hay hoy.
	n := len(simKinds)
	if n < 3 {
		t.Fatalf("hacen falta 3 kinds para que +1 y -1 se distinguan, hay %d", n)
	}
	m := simModel(t, &fakeSimulator{available: true})
	m = press(t, m, "v")

	// Abajo desde el primero: recorre todos y vuelve al primero.
	m.sim.cursor = 0
	for i := range n {
		m = press(t, m, "j")
		if want := (i + 1) % n; m.sim.cursor != want {
			t.Fatalf("tras %d pasos abajo el cursor = %d, want %d", i+1, m.sim.cursor, want)
		}
	}

	// Arriba desde el primero: el caso del `+ n`, que es el que no puede
	// devolver un índice negativo.
	m.sim.cursor = 0
	m = press(t, m, "k")
	if m.sim.cursor != n-1 {
		t.Errorf("arriba desde el primero dio %d, want %d (el último): un índice negativo aquí es un panic", m.sim.cursor, n-1)
	}
	// Y toda la vuelta hacia arriba, que es donde cualquier `- 1` mal puesto se
	// nota: cada paso tiene que restar uno y involvedar.
	m.sim.cursor = 0
	for i := range n {
		m = press(t, m, "k")
		if want := (n - 1 - i) % n; m.sim.cursor != want {
			t.Fatalf("tras %d pasos arriba el cursor = %d, want %d", i+1, m.sim.cursor, want)
		}
	}

	// Los alias: k es arriba, y j/right/tab son abajo.
	for _, alias := range []string{"j", "right", "tab"} {
		m.sim.cursor = 0
		m = press(t, m, alias)
		if m.sim.cursor != 1 {
			t.Errorf("la tecla %q dio el cursor %d, want 1 (es alias de abajo)", alias, m.sim.cursor)
		}
	}
	m.sim.cursor = 0
	m = press(t, m, "k")
	if m.sim.cursor != n-1 {
		t.Errorf("la tecla k dio el cursor %d, want %d (es alias de arriba)", m.sim.cursor, n-1)
	}
}
