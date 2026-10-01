package tui

import (
	"testing"
	"time"
)

// TestBranchCacheFreshElTTLJustoSigueSirviendo: un listado cacheado sirve hasta que
// pasa el TTL, y en el instante del TTL todavía.
//
// El reloj es un parámetro (`now`), no una llamada dentro. No por gusto: la frontera
// de esta función ES el TTL, y con el reloj dentro habría que esperar el TTL entero
// para comprobarla. Un test que espera un TTL para verificar una comparación está
// midiendo el reloj, no la regla, y además es un test que se pone flakiness con el
// resto de la suite.
//
// Y la frontera importa por el coste, no por la fe: tirar el caché en el TTL
// significa una llamada a la red, y `git ls-remote` es de los más lentos. Un ">="
// descartaría el caché en el último momento en que todavía servía.
func TestBranchCacheFreshElTTLJustoSigueSirviendo(t *testing.T) {
	ahora := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

	for _, c := range []struct {
		nombre        string
		edad          time.Duration
		quiereVigente bool
	}{
		{"recién hecho", 0, true},
		{"un segundo", time.Second, true},
		{"la mitad del TTL", branchCacheTTL / 2, true},
		{"un TTL menos un nanosegundo", branchCacheTTL - time.Nanosecond, true},
		{"EXACTAMENTE el TTL", branchCacheTTL, true},
		{"el TTL más un nanosegundo", branchCacheTTL + time.Nanosecond, false},
		{"el doble del TTL", 2 * branchCacheTTL, false},
		{"mucho más viejo", time.Hour, false},
		// Y un caso raro que puede pasar con un reloj que atrasa: un fetchedAt en el
		// futuro. La diferencia es negativa, o sea "más joven que ahora", y tiene que
		// decir que sirve. Tratarlo como caducado sería tirar el caché por un reloj
		// desajustado, que es justo lo que no hay que hacer.
		{"en el futuro", -time.Hour, true},
	} {
		fetched := ahora.Add(-c.edad)
		if got := branchCacheFresh(fetched, ahora); got != c.quiereVigente {
			t.Errorf("%s: un listado de hace %v dio vigente=%v, want %v",
				c.nombre, c.edad, got, c.quiereVigente)
		}
	}
}

// TestRetargetWindowForSigueAlCursor: la ventana de la lista de ramas se mueve lo
// justo para que la fila del cursor siga dentro, y no más.
//
// Tres reglas, y se rompen por lados distintos, que es por lo que van en un test
// con las tres:
//
//   - cursor por ENCIMA de la ventana: la ventana sube a él. Es el caso de filtrar:
//     la lista se acorta por arriba y la selección se queda donde estaba, fuera.
//   - cursor por DEBAJO: la ventana baja lo justo, con la fila del cursor en la
//     última posición de la caja. Un "- rows" sin el "+1" dejaría la fila del cursor
//     justo debajo de la caja, que es no mover la ventana.
//   - y la ventana nunca se sale: ni por encima del 0, ni por debajo de la última
//     ventana que cabe en la lista.
//
// Es aritmética pura de cuatro números, así que se afirma entera. Y lo que se
// afirma en cada caso es la relación entre las tres salidas, que es la que no tiene
// sentido leer de un dibujo: la ventana contiene al cursor, la ventana cabe en la
// lista, y la ventana no ha salido de donde puede.
func TestRetargetWindowForSigueAlCursor(t *testing.T) {
	const rows = 5

	// Todas las ventanas para una lista de 20 con 20 cursores: matriz completa, para
	// que ningún par (ventana, cursor) se quede sin mirar.
	for total := 0; total <= 22; total++ {
		for win := 0; win <= total+3; win++ {
			for cursor := 0; cursor <= total+3; cursor++ {
				got := retargetWindowFor(win, cursor, rows, total)

				// 1. La ventana no se sale por arriba ni por abajo.
				if got < 0 {
					t.Fatalf("total=%d win=%d cursor=%d dio ventana %d: negativa", total, win, cursor, got)
				}
				if maxWin := max(0, total-rows); got > maxWin {
					t.Fatalf("total=%d win=%d cursor=%d dio ventana %d, más que la última posible %d",
						total, win, cursor, got, maxWin)
				}

				// 2. Si la ventana NO cabe entera (pocas ramas), vale 0: no se
				// desplaza una lista que ya cabe entera.
				if total <= rows {
					if got != 0 {
						t.Fatalf("total=%d win=%d cursor=%d dio ventana %d, want 0: "+
							"la lista cabe entera, no hay nada que desplazar", total, win, cursor, got)
					}
					continue
				}

				// 3. Y con la lista larga, la ventana CONTIENE al cursor. Eso no
				// siempre se puede: con el cursor fuera de rango no. Dentro, sí, y
				// es la propiedad que importa, porque una selección fuera de la caja
				// es la casilla en la que el cursor está y no se ve.
				if cursor < total {
					if cursor < got || cursor >= got+rows {
						t.Errorf("total=%d win=%d cursor=%d dio ventana %d, y el cursor "+
							"queda fuera: no se ve la fila seleccionada", total, win, cursor, got)
					}
				}
			}
		}
	}

	// Y los casos que dicen más, uno a uno.
	// El cursor justo en la última fila visible: la ventana no se mueve. Es el
	// borde de "por debajo", y por eso el "+1" del otro lado cuenta.
	if got := retargetWindowFor(4, 4, rows, 20); got != 4 {
		t.Errorf("con el cursor en la última fila visible dio ventana %d, want 4: no hay nada que mover", got)
	}
	// Una fila dentro de la ventana: no se mueve. La ventana tiene `rows` filas, así
	// que el cursor sigue siendo visible y no hay nada que hacer. Este es el caso que
	// hace evidente que el "por debajo" no es "uno más": es "fuera de la ventana".
	if got := retargetWindowFor(4, 5, rows, 20); got != 4 {
		t.Errorf("con el cursor una fila dentro de la ventana dio %d, want 4: sigue viendose", got)
	}
	// Y el borde de verdad, el cursor en la primera fila FUERA de la ventana: la
	// ventana baja lo justo para incluirlo, con su ultima fila en el borde. Un
	// "- rows" sin el "+1" lo dejaria justo debajo, que es no mover nada.
	if got := retargetWindowFor(4, 9, rows, 20); got != 5 {
		t.Errorf("con el cursor en la primera fila fuera dio ventana %d, want 5: baja lo justo", got)
	}
	// Y dos filas fuera: baja dos, no hasta el cursor. Con 20 ramas y 5 filas solo se
	// ve una ventana, asi que "pegado al cursor" daria una ventana distinta cada vez.
	if got := retargetWindowFor(4, 11, rows, 20); got != 7 {
		t.Errorf("con el cursor dos filas fuera dio ventana %d, want 7", got)
	}
	// Y si el cursor está en la última rama, la ventana es la última posible.
	if got := retargetWindowFor(0, 19, rows, 20); got != 15 {
		t.Errorf("con el cursor en la última rama dio ventana %d, want 15", got)
	}
	// Filtrar: la lista se acorta por arriba y el cursor se queda donde estaba.
	if got := retargetWindowFor(4, 2, rows, 20); got != 2 {
		t.Errorf("con el cursor por encima de la ventana dio %d, want 2: la ventana sube al cursor", got)
	}
	// Una ventana imposible que alguien dejó puesta: se corrige sola. Sin esto, un
	// `win` grande de un estado viejo dejaría la lista vacía sin avisar.
	if got := retargetWindowFor(500, 3, rows, 20); got != 3 {
		t.Errorf("con una ventana imposible dio %d, want 3 (la del cursor)", got)
	}
}

// TestRetargetVisibleFromRecortaPorLosDosLados: el recorte de la lista a la ventana
// tiene dos suelos, y cada uno tapa un lado.
//
//   - `start` no puede pasar de la longitud: sin ese suelo, cortar por la ventana
//     se sale del slice.
//   - y el alto no puede pasar de lo que queda desde `start`: sin ese suelo, una
//     última página a medias pinta ramas que no hay.
//
// El caso de "caben más filas que ramas" da un tramo vacío, y eso NO es un error: es
// lo que se ve cuando un filtro no deja nada, y una caja de ramas vacía con su marco
// se lee como "no hay coincidencias", que es la verdad.
func TestRetargetVisibleFromRecortaPorLosDosLados(t *testing.T) {
	view := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}

	// El rango que llega de verdad: `win` sale de retargetWindow, que tiene suelo 0,
	// y `rows` de retargetRows, que tiene suelo retargetMinRows. Un `win` negativo o
	// unas filas negativas no se prueban porque no se dan, y no se blindan con un suelo
	// porque un suelo que no puede llegar es la clase de codigo que esta campana lleva
	// quitando: lo que protege es una precondicion, y lo que hace falta es DECIRLA, que
	// esta en el comentario de la funcion.
	for _, win := range []int{0, 3, 9, 10, 11, 100} {
		for _, rows := range []int{0, 1, 3, 10, 20} {
			visibles, start := retargetVisibleFrom(view, win, rows)

			// Lo pintado viene de la lista, en orden y sin repetir.
			for i, v := range visibles {
				if i >= len(view) {
					t.Fatalf("win=%d rows=%d pintó %d ramas de las %d que hay", win, rows, len(visibles), len(view))
				}
				if view[start+i] != v {
					t.Fatalf("win=%d rows=%d la fila %d es %q, want %q: la ventana no está donde dice",
						win, rows, i, v, view[start+i])
				}
			}
			// Y no se pinta más de lo que hay, ni de lo que cabe.
			if len(visibles) > len(view) {
				t.Errorf("win=%d rows=%d pintó %d ramas de las %d que hay", win, rows, len(visibles), len(view))
			}
			if len(visibles) > rows {
				t.Errorf("win=%d rows=%d pintó %d ramas, más de las %d que caben", win, rows, len(visibles), rows)
			}
			// Y `start` es un índice válido de la lista, que es lo que hace que la
			// comprobación de arriba no haya reventado.
			if start < 0 || start > len(view) {
				t.Errorf("win=%d rows=%d dio start=%d, fuera de la lista de %d", win, rows, start, len(view))
			}
		}
	}

	// Y los casos que dicen:
	// Todo: ventana 0, 3 filas, salen las 3 primeras.
	vis, start := retargetVisibleFrom(view, 0, 3)
	if start != 0 || len(vis) != 3 || vis[0] != "a" || vis[2] != "c" {
		t.Errorf("con ventana 0 y 3 filas dio %q desde %d, want [a b c] desde 0", vis, start)
	}
	// La ventana en medio: sale el tramo y el índice de la primera, que es lo que
	// hace falta para pintar el cursor.
	vis, start = retargetVisibleFrom(view, 3, 3)
	if start != 3 || len(vis) != 3 || vis[0] != "d" {
		t.Errorf("con ventana 3 dio %q desde %d, want [d e f] desde 3", vis, start)
	}
	// Una ventana que no existe: se recorta a la lista, y se pinta lo que queda. Con
	// ventana 20 y 3 filas, `start` cae al final y no hay nada que pintar: es el
	// "se sale por la derecha" del suelo.
	vis, start = retargetVisibleFrom(view, 20, 3)
	if start != len(view) || len(vis) != 0 {
		t.Errorf("con una ventana imposible dio %q desde %d, want vacío desde %d", vis, start, len(view))
	}
	// Más filas que ramas: sale la lista entera, que es lo que cabe.
	vis, start = retargetVisibleFrom(view, 0, 100)
	if start != 0 || len(vis) != len(view) {
		t.Errorf("con 100 filas dio %d ramas desde %d, want las %d de la lista", len(vis), start, len(view))
	}
	// Y con cero filas que pintar: tramo vacío, y no un corte negativo. El suelo de
	// retargetRows hace que no se den, pero la funcion se comporta bien si se dan.
	vis, start = retargetVisibleFrom(view, 0, 0)
	if start != 0 || len(vis) != 0 {
		t.Errorf("con cero filas dio %q desde %d, want vacío desde 0", vis, start)
	}

	// Y con filtro sin resultados: tramo vacío, y el índice al principio. La caja se
	// pinta vacía y eso dice "no hay coincidencias", que es la verdad.
	vis, start = retargetVisibleFrom(nil, 0, 5)
	if start != 0 || len(vis) != 0 {
		t.Errorf("con una lista vacía dio %q desde %d, want vacío desde 0", vis, start)
	}
}
