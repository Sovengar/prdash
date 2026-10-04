package tui

import (
	"strings"
	"testing"

	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

// Los siete de comments.go se parten en dos clases y solo una se puede matar.
//
// La primera son IDENTIDADES EN LA FRONTERA, y son tres: el total igual a la lista, el
// recorte al tope de cinco y el reparto cuando el presupuesto da exactamente lo que se
// pide. En los tres, `>` y `>=` dan el mismo resultado porque la operación del cuerpo es
// una identidad en ese punto. No es que falte un test: es que no hay entrada que los
// separe.
//
// La segunda son las que sí tienen un caso que las distingue, y son tres. La más
// interesante es la guarda de `avail <= 0`, porque parece absorbida por la de abajo y
// no lo está en todos los caminos.

// TestSinFilasNoSePintaNiUnAvisoDeCarga: con cero filas no hay caja, y eso vale
// también cuando los comentarios todavía se están cargando.
//
// Y aquí está el caso que no estaba probado. La guarda de `avail <= 0` parece
// innecesaria porque más abajo hay `if avail < commentChrome+len(st.list)`, y con
// `avail == 0` esa segunda devuelve nil siempre. Pero ENTRE MEDIAS hay un `switch` con
// cuatro salidas, y tres de ellas devuelven algo: cargando, error y ninguno.
//
// Con el estado ya listo, la segunda guardase come a la primera. Con el estado
// AÚN CARGANDO, el switch devuelve la línea de "cargando…" y a la segunda no se llega
// nunca. O sea que la primera guarda solo está absorbida a medias, y el test que
// existía solo miraba el camino listo.
//
// Y lo que se ve con la guarda mutada es una línea fantasma: "Comments loading…" en un
// hueco de cero filas, que es un texto que no cabe en ninguna parte. No es que se vea
// mal, es que no hay sitio donde se vea.
func TestSinFilasNoSePintaNiUnAvisoDeCarga(t *testing.T) {
	adapt := &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"}

	// Los cuatro estados del switch, uno por uno, con CERO filas. Todos tienen que
	// devolver nil: sin filas no hay caja, y un texto suelto no es una caja.
	estados := []struct {
		nombre string
		st     *commentState
	}{
		{"sin preguntar", &commentState{}},
		{"cargando", &commentState{list: []model.Comment{{Author: "a", Body: "b"}}}},
		{"con error", &commentState{ready: true, err: "boom"}},
		{"sin comentarios", &commentState{ready: true}},
		{"listo con comentarios", &commentState{ready: true,
			list: []model.Comment{{Author: "a", Body: "b"}}}},
	}

	// SIN filas no hay nada, en los cinco estados del switch. Y aquí está el caso que
	// no estaba probado: la segunda comprobación, la de `commentChrome`, solo se
	// alcanza con el estado LISTO. Con el estado cargando, con error o sin comentarios,
	// el switch devuelve su línea antes y a la segunda no se llega.
	//
	// O sea que la guarda de `avail <= 0` no la absorbe la de abajo en todos los
	// caminos, y la primera versión de este test —que solo miraba el estado listo— no
	// se enteró.
	for _, e := range estados {
		for _, avail := range []int{-5, 0} {
			m := newTestModel(t, adapt)
			it := mkItem("github", "github.com", "acme/widget", "uno", 1, "")
			// El estado que se quiere, tal cual: el resto de campos a cero.
			st := *e.st
			m.comments[it.ID()] = &st

			if got := m.commentLines(it, avail, m.contentWidth()); len(got) != 0 {
				t.Errorf("estado %q con %d filas dio %d líneas (%q): sin filas no hay "+
					"caja, y un texto suelto no es una caja. Con la guarda puesta como "+
					"`avail < 0` el switch de estados devuelve cargando/error antes de "+
					"llegar a la comprobación que la absorbe",
					e.nombre, avail, len(got),
					primeraLineaCon(stripANSI(strings.Join(got, " ")), 80))
			}
		}
	}

	// Y el otro lado del mismo borde: con UNA o DOS filas, los estados que tienen texto
	// que decir lo dicen. Que es lo que separa `avail <= 0` de `avail <= 1`: con el
	// suelo en uno, una fila se pierde, y una fila de un estado suelto es justo lo que
	// hace que se vea que el PR tiene conversación sin poder leerla.
	//
	// Los estados que NO están aquí, y por qué, que es la parte que hace falta decir:
	//
	//   - "sin preguntar" devuelve nil siempre, porque no hay nada en vuelo que
	//     anunciar y el propio código lo dice;
	//   - "listo con comentarios" no es un estado suelto: entra por la caja, y la caja
	//     necesita sus dos bordes más una fila por comentario. Con una o dos filas no
	//     cabe, y no cabe por la comparación de abajo, no por la guarda.
	conTexto := estados[1:4]
	for _, e := range conTexto {
		for _, avail := range []int{1, 2} {
			m := newTestModel(t, adapt)
			it := mkItem("github", "github.com", "acme/widget", "uno", 1, "")
			st := *e.st
			m.comments[it.ID()] = &st

			if got := m.commentLines(it, avail, m.contentWidth()); len(got) == 0 {
				t.Errorf("estado %q con %d filas no dio ninguna línea: los estados "+
					"sueltos son de una línea y con una fila hay sitio para ellos. Con "+
					"el suelo en uno se pierde, y una fila perdida de un estado suelto "+
					"hace que no se vea que el PR tiene conversación",
					e.nombre, avail)
			}
		}
	}
}

// TestLaAnchuraDeLaCajaDescuentaElSangradoYTieneSuelo: el ancho exterior de la caja de
// comentarios es el del hueco menos el sangrado de los dos lados, con un suelo.
//
// Y el suelo no es un detalle: por debajo de ocho la caja se estrecha tanto que el
// nombre del autor no cabe ni truncado y el cuerpo tiene menos de un ancho utilizable.
// Un suelo de ocho es lo que hace que el `truncate` del autor y el del error tengan un
// ancho que no hay que volver a comprobar.
//
// Y el sangrado son DOS por lado, no uno: `commentInset` es uno y el ancho se descuenta
// dos veces, una por cada lado. Con una sola vez la caja se sale un lado, y como el
// sangrado está para que los bordes no se pisen, un borde pisado es exactamente el
// defecto que el sangrado evita.
func TestLaAnchuraDeLaCajaDescuentaElSangradoYTieneSuelo(t *testing.T) {
	// Donde el suelo NO manda: el ancho pedido menos el sangrado de los dos lados.
	for _, outer := range []int{10, 11, 20, 60, 100, 200} {
		want := outer - 2*commentInset
		if got := commentBoxWidth(outer); got != want {
			t.Errorf("con un hueco de %d columnas la caja mide %d, want %d: el sangrado "+
				"es de %d por lado y son dos lados", outer, got, want, commentInset)
		}
	}

	// Y donde el suelo MANDA: por debajo de ocho sale ocho, no un número negativo ni un
	// ancho de tres columnas.
	for _, outer := range []int{-10, 0, 5, 9} {
		got := commentBoxWidth(outer)
		if got != 8 {
			t.Errorf("con un hueco de %d columnas la caja mide %d, want 8: por debajo del "+
				"suelo sale el suelo, porque un nombre de autor que no cabe ni truncado "+
				"hace la caja ilegible", outer, got)
		}
	}
}

// TestLaFilaDelCorteSeVuelveAComponerEnSuSitio: cuando un comentario no cabe entero, la
// ÚLTIMA fila que se escribió se vuelve a componer marcando el corte.
//
// Y "en su sitio" es la parte que importa: se recompone la fila del índice `len(out)-1`,
// o sea la última que se pintó, y no otra. Con el índice corrido una fila hacia delante
// se marca como cortada una que no lo está, y el corte queda en una fila que se lee
// entera —la frase terminada— mientras la de verdad, que está a medias, se queda sin
// marcar. Y lo que se lee es al revés de lo que pasó: el usuario ve un comentario que
// acaba limpio en una frase que no acababa.
//
// Y con el índice AL revés, `len(out)+1`, es un índice fuera de rango, y eso es un panic
// —que también es una muerte, aunque no salga un `--- FAIL`—.
//
// El caso que dispara todo esto es un comentario largo con un presupuesto de una o dos
// filas: sobra texto y hay que marcar por dónde se corta.
func TestLaFilaDelCorteSeVuelveAComponerEnSuSitio(t *testing.T) {
	adapt := &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"}
	m := newTestModel(t, adapt)
	it := mkItem("github", "github.com", "acme/widget", "uno", 1, "")

	// Un comentario de diez párrafos, que necesita muchas más filas de las que se le
	// van a dar en todos los presupuestos siguientes.
	//
	// Y el ancho importa: con el del modelo, que son 158 columnas, tres párrafos de
	// treinta palabras cabían en tres filas JUSTAS y no había nada que cortar. Con un
	// hueco de 40 columnas los mismos tres párrafos piden nueve, y entonces sí sobra
	// texto. Un caso que no dispara la condición no prueba nada de la condición.
	parrafos := make([]string, 10)
	for i := range parrafos {
		parrafos[i] = strings.Repeat("palabra de relleno para la frase ", 4)
	}
	cuerpo := strings.Join(parrafos, "\n\n")
	const inner = 40
	m.comments[it.ID()] = &commentState{ready: true, list: []model.Comment{
		{Author: "alice", Body: cuerpo},
	}, total: 1}

	// Con presupuesto de una y de dos filas, el bloque tiene que salir con el número de
	// filas pedido y con el corte marcado en la última.
	for _, filas := range []int{1, 2, 3, 4, 5, 6} {
		got := m.commentLines(it, commentChrome+filas, inner)
		if len(got) == 0 {
			t.Fatalf("con %d filas no salió bloque ninguno", filas)
		}
		// Y el número de filas del bloque tiene que ser el pedido: ni una más (que
		// invadiría la lista) ni una menos.
		contenido := len(got) - commentChrome
		if contenido > filas {
			t.Errorf("con %d filas el bloque pintó %d de contenido: se pasó de largo",
				filas, contenido)
		}
		plano := stripANSI(strings.Join(got, "\n"))
		// Y el corte tiene que estar marcado, y en la fila que se leyó la última.
		if !strings.Contains(plano, "…") {
			t.Errorf("con %d filas el bloque sale sin marca de corte: %q", filas,
				primeraLineaCon(plano, 160))
		}
		// Y en la ÚLTIMA fila de contenido, que es la única que se recompone marcando.
		// La última fila DE LA CAJA es el borde de abajo con la leyenda, así que "la
		// última" hay que entenderla como la última fila que NO es borde. Y eso se
		// afirma por posición: si la marca estuviera más arriba, el usuario leería un
		// comentario que acaba limpio en medio del texto, que es justo lo que el
		// marcado existe para que no pase.
		//
		// La primera versión de este aserto miraba la última fila a secas y siempre
		// miraba el borde de abajo, con lo que no afirmaba nada. Es el cuarto assert
		// que hay que mirar donde está el DATO y no donde está el OBJETO.
		conLasMarcas := []string{}
		for _, l := range strings.Split(plano, "\n") {
			if strings.Contains(l, "…") {
				conLasMarcas = append(conLasMarcas, l)
			}
		}
		if len(conLasMarcas) != 1 {
			t.Errorf("con %d filas hay %d filas con la marca de corte, want 1: el corte "+
				"se marca una vez, en la última fila que se pintó", filas, len(conLasMarcas))
			continue
		}
		// Y esa fila es la última de contenido: no queda ninguna fila de texto detrás.
		ultimaDeTexto := ""
		for _, l := range strings.Split(plano, "\n") {
			if strings.Contains(l, "─") {
				continue
			}
			ultimaDeTexto = l
		}
		if !strings.Contains(ultimaDeTexto, "…") {
			t.Errorf("con %d filas la marca de corte quedó en %q y la última fila de "+
				"texto es %q: la marca tiene que caer en la última fila que se pintó",
				filas, conLasMarcas[0], primeraLineaCon(ultimaDeTexto, 90))
		}

		// Y el índice que se pasa al recomponer decide si esa fila lleva el nombre del
		// autor. `commentRow` solo se-branch-ea con `idx == 0`: la primera fila de un
		// comentario lleva el autor y las siguientes no. O sea que el índice importa
		// SOLO cuando la fila recompuesta es la cero, que es el caso de un comentario
		// al que solo le ha cabido una fila.
		//
		// Ahí es donde un índice corrido se ve: si la fila cero se recompone con el
		// índice de la fila siguiente, deja de llevar el nombre del autor y el
		// comentario se lee como texto suelto, sin saber de quién es. Y si el índice
		// es el de más, pasa lo mismo al revés.
		//
		// Y el caso de DOS filas es el del otro lado: si la fila uno se recompone con el
		// índice de la cero, el nombre del autor aparece DOS veces, una al principio del
		// comentario y otra en su última fila, que es lo que hace parecer que hay dos
		// comentarios donde hay uno.
		primeraDeTexto := ""
		for _, l := range strings.Split(plano, "\n") {
			if strings.Contains(l, "─") {
				continue
			}
			primeraDeTexto = l
			break
		}
		if !strings.Contains(primeraDeTexto, "alice") {
			t.Errorf("con %d filas la primera fila de texto es %q y no lleva el autor: "+
				"es la fila cero y solo la fila cero lo lleva", filas,
				primeraLineaCon(primeraDeTexto, 90))
		}
		if filas > 1 && strings.Contains(ultimaDeTexto, "alice") {
			t.Errorf("con %d filas la última fila lleva también el autor: %q. Con más "+
				"de una fila solo la primera lo lleva, y verlo dos veces hace pensar "+
				"que hay dos comentarios", filas, primeraLineaCon(ultimaDeTexto, 90))
		}
	}
}

// TestElRepartoEsElMismoCuandoElPresupuestoDaExacto: cuando lo que se pide y lo que hay
// son lo mismo, `allocate` devuelve exactamente lo que se le pasó.
//
// Esta es la cuenta que sostiene la identidad de `total > budget` en el reparto. Si
// `allocate(need, budget)` con `sum(need) == budget` devolviera otra cosa, entonces con
// el presupuesto JUSTO el bloque repartiría filas de otra manera, y el `>` necesitaría un
// `>=` para no repartir en el caso que ya da exacto.
//
// Y la cuenta es esta: `allocate` pone una fila a cada uno y reparte las sobrantes una a
// una a quien menos tiene, parando cuando nadie puede absorber más. Si la suma de lo que
// pide cada uno ES el presupuesto, las sobrantes son exactamente las que faltan, y el
// reparto termina con cada comentario en su número. El `break` de "nadie puede absorber
// más" no se llega a tocar, o se toca con las filas ya en su sitio.
//
// Y el supuesto de que todas las peticiones son de una fila o más lo pone `commentBody`,
// que aplica `max(1, lines)`. Sin ese suelo, un comentario de cuerpo vacío pediría cero
// filas y `allocate` le daría una, que es más de lo que pidió: por eso el suelo está y
// por eso se afirma abajo.
func TestElRepartoEsElMismoCuandoElPresupuestoDaExacto(t *testing.T) {
	casos := []struct {
		need   []int
		budget int
		nota   string
	}{
		{[]int{1, 1, 1}, 3, "tres de una fila con presupuesto exacto"},
		{[]int{1, 5}, 6, "uno corto y uno largo, presupuesto exacto"},
		{[]int{5, 1}, 6, "el largo primero: el reparto no depende del orden"},
		{[]int{1, 1, 1, 1, 1}, 5, "los cinco de una fila"},
		{[]int{3, 3, 3}, 9, "tres iguales con presupuesto exacto"},
		{[]int{10, 1, 1, 1}, 13, "uno enorme y tres de una"},
		{[]int{2, 2, 2, 2, 2}, 10, "cinco de dos filas"},
	}
	for _, c := range casos {
		got := allocate(c.need, c.budget)
		if len(got) != len(c.need) {
			t.Errorf("%s: allocate devolvió %d filas para %d comentarios",
				c.nota, len(got), len(c.need))
			continue
		}
		for i := range got {
			if got[i] != c.need[i] {
				t.Errorf("%s: allocate devolvió %v, want %v. Con el presupuesto justo "+
					"el reparto tiene que ser una identidad, o el `>` del reparto necesita "+
					"un `>=` que no tiene", c.nota, got, c.need)
				break
			}
		}
	}
}

// TestNadiePideCeroFilas: el suelo que hace posible lo anterior.
//
// `allocate` reparte desde una fila por comentario, así que un comentario que pidiera
// CERO filas se quedaría con una de más: es la única razón por la que `commentBody`
// aplica `max(1, lines)`. Y esa es la condición que hace que `allocate` con el
// presupuesto justo sea una identidad, porque con un cero de por medio el reparto
// terminaría con ese comentario en una fila que no pidió.
//
// Y el suelo no se quita por prudencia: un comentario con el cuerpo vacío o solo con
// boilerplate tiene que ocupar algo, porque una fila vacía se lee como un comentario
// borrado.
func TestNadiePideCeroFilas(t *testing.T) {
	cuerpos := []model.Comment{
		{Author: "alice"},
		{Author: "alice", Body: ""},
		{Author: "alice", Body: "\n\n\n"},
		{Author: "alice", Body: "   \n  \n"},
		{Author: "", Body: ""},
		{Author: "alice", Body: "una linea"},
	}
	for _, c := range cuerpos {
		for _, lineas := range []int{1, 2, 5, 20} {
			for _, ancho := range []int{12, 38, 60} {
				got := commentBody(c, lineas, commentBodyWidth(ancho))
				if len(got) == 0 {
					t.Errorf("comentario %+v con %d filas y ancho %d devolvió NADA: una "+
						"fila vacía se lee como un comentario borrado, y el suelo a una "+
						"fila es lo que además hace que el reparto con presupuesto justo "+
						"sea una identidad",
						c, lineas, ancho)
				}
				if len(got) > lineas && lineas >= 1 {
					t.Errorf("comentario %+q con tope de %d filas devolvió %d",
						c.Body, lineas, len(got))
				}
			}
		}
	}
}

// primeraLineaCon recorta un texto para un mensaje de fallo, sin cortar por la mitad de
// un rune —que es lo que pasa con un `[:n]` sobre texto UTF-8—.
func primeraLineaCon(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
