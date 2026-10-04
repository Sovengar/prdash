package tui

import (
	"testing"

	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

// Estas son las funciones auxilares de la TUI que no tienen un camino de error pero sí tienen
// un borde: un ítem que no está, una sección que no está en el ciclo, un mapa de posiciones sin
// inicializar, un forge que no existe.
//
// Y el patrón es el mismo en todas: degradan a un valor NEUTRO. Y la razón por la que eso hay
// que fijar es que un valor neutro mal elegido no falla —devuelve 0, devuelve false— sino que
// desplaza el cursor a la primera posición, cuenta la sección equivocada o decide que no hay
// selección. Nada peta y la vista se queda medio sutil.

// TestFindItemEncuentraPorIdentidadYNoConfundeItemsDeForgesDistintos: `findItem`.
//
// Y la identidad incluye forge, host y proyecto, y ese "y" es lo que hay que probar. Un ítem de
// GitHub con el número 7 y otro de GitLab con el número 7 son ÍTEMS DISTINTOS, y un `findItem`
// que comparara solo por número devolvería el primero de los dos.
//
// Y el motivo de que importe: el resultado se usa para re-leer el ítem después de una acción, y
// equivocarse de repo al aplicar el resultado de un approve es escribir en el sitio
// equivocado.
func TestFindItemEncuentraPorIdentidadYNoConfundeItemsDeForgesDistintos(t *testing.T) {
	gh := mkItem("github", "github.com", "acme/widget", "uno", 7, "")
	gl := mkItem("gitlab", "gitlab.example.com", "otro/widget", "otro", 7, "")
	items := []model.Item{gh, gl}

	got, ok := findItem(items, gh.ID())
	if !ok || got.ID() != gh.ID() {
		t.Errorf("findItem del primero devolvió (%v, %v)", got.Number, ok)
	}
	got, ok = findItem(items, gl.ID())
	if !ok || got.ID() != gl.ID() {
		t.Errorf("findItem del segundo devolvió (%v, %v): %+v", got.Number, ok, got)
	}

	// Y un id que no está: el valor CERO y false. El cero importa porque un `model.Item{}`
	// tiene el Forge vacío, y un caller que ignorara el `ok` buscaría un ítem del host
	// vacío.
	id := gh.ID()
	id.Project = "no/existe"
	got, ok = findItem(items, id)
	if ok {
		t.Error("findItem de un id inexistente dio ok")
	}
	if got.ID() != (model.ID{}) {
		t.Errorf("findItem devolvió %+v en vez del valor cero", got.ID())
	}
	// Y sobre una lista vacía.
	if _, ok := findItem(nil, gh.ID()); ok {
		t.Error("findItem sobre nil dio ok")
	}
}

// TestElCicloDeSeccionesCiclaYLaSeccionActivaSeRecuerda: `sectionCycleIndex` y `setActiveSection`.
//
// Y el ciclo tiene dos propiedades que se complementan: **cicla siempre**, aunque la sección
// destino esté vacía —que es justo cuando el usuario quiere verla, para leer su estado y su
// conteo— y **la posición se recuerda por sección**, para que volver a una devuelva el cursor y
// el scroll donde estaban.
//
// Y lo que se prueba es la segunda, porque la primera es un módulo y no necesita test. Y hay un
// caso que distingue las dos: ir y volver a la MISMA sección debe restaurar su posición, y
// hacerlo no es lo mismo que reiniciarla.
//
// Y el `m.pos == nil` del principio: un modelo recién construido no lo tiene, y `setActiveSection`
// lo crea. Sin ese `if`, escribir en un mapa nil es un panic —no un error visible— que saldría en
// la primera pulsación de `section-next`.
func TestElCicloDeSeccionesCiclaYLaSeccionActivaSeRecuerda(t *testing.T) {
	// El mapa de posiciones se crea en `New`, así que el `if m.pos == nil` de
	// `setActiveSection` solo se dispararía para un `Model{}` a valor cero. Mi primera versión
	// daba por hecho que arrancaba a nil —como el de `branchCache` antes— y falló: aquí lo
	// inicializa `New`. Se comprueba con un modelo de valor cero aparte.
	m := newTestModel(t)

	// Una vuelta completa al ciclo vuelve a donde se estaba.
	inicio := m.activeSection
	for range sectionCycle {
		m.cycleSection()
	}
	if m.activeSection != inicio {
		t.Errorf("una vuelta completa al ciclo terminó en %v, want %v", m.activeSection, inicio)
	}

	// Y la posición se recuerda: se sale de review con el cursor en 1, se va a otra y al
	// volver el cursor está en 1.
	// Tres ítems en REVIEW, que es la sección de la que se sale. Con uno solo el cursor
	// válido sería 0 y el aserto de "vuelve donde estaba" no distinguiría una restauración de
	// un reinicio —mi primera versión repartió un ítem por sección y por eso falló.
	m2 := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	k := streamKey{forge: "github", section: model.SectionReview, kind: model.ReviewRequested}
	m2.streams[k] = &stream{}
	for _, n := range []int{1, 2, 3} {
		it := mkItem("github", "github.com", "acme/widget", "t", n, "")
		it.Section = model.SectionReview
		m2.streams[k].items = append(m2.streams[k].items, it)
	}
	m2.rebuild()
	m2.cursor = 2
	m2.scroll = 1

	m2.cycleSection() // a otra sección, sin ítems: el cursor se recorta a cero
	m2.setActiveSection(model.SectionReview)
	if m2.cursor != 2 {
		t.Errorf("al volver a review el cursor quedó en %d, want 2: la posición se recuerda "+
			"por sección y no se reinicia", m2.cursor)
	}
	// Y el scroll se recuerda en la MEMORIA, pero con tres ítems en una terminal de cuarenta
	// filas no hay nada que desplazar y el render lo recorta a cero. Mi primera versión
	// afirmaba que volvía a 1 y fallaba: no es que se pierda, es que no hay contenido que lo
	// sostenga. Comprobarlo con contenido que desborde requiere una terminal diminuta, y eso
	// es otro test con otro motivo.

	// Y una sección donde no se ha estado nunca empieza en cero, que es el otro default.
	m2.setActiveSection(model.SectionMentions)
	if m2.cursor != 0 || m2.scroll != 0 {
		t.Errorf("una sección nueva quedó en cursor=%d scroll=%d, want 0 y 0",
			m2.cursor, m2.scroll)
	}

	// Y el `m.pos == nil` del principio se comprueba con un modelo de valor cero, que es el
	// único caso donde se dispara.
	var cero Model
	cero.setActiveSection(model.SectionReview)
	if len(cero.pos) != 1 {
		t.Errorf("setActiveSection sobre un modelo de valor cero dejó %d posiciones, want 1: "+
			"escribir en un mapa nil es un PANIC, no un error visible", len(cero.pos))
	}

	// Y una sección ACTIVA que no está en el ciclo da índice cero, que es lo que hace que
	// `cycleSection` empiece por el principio en vez de indexar fuera de rango.
	m2.activeSection = "una-seccion-inventada"
	if got := m2.sectionCycleIndex(); got != 0 {
		t.Errorf("una sección fuera del ciclo dio índice %d, want 0", got)
	}
	// Y ciclar desde ahí no revienta.
	m2.cycleSection()
}

// TestRequestCommentsRechazaLasTresNegativasYSiempreRearmaElTick: `requestComments`.
//
// Y son tres negativas y una propiedad que es la que de verdad importa: **el tick se rearma
// siempre**, consultara o no. Un tick que solo se rearmara "si consultó" es una cadena que se
// muere en el primer cambio a un ítem que ya tiene la conversación cargada, y el siguiente ya
// no se entera.
//
// Y las tres negativas —sin selección, ya cargado, forge desconocido— se devuelven con el tick
// y sin consultar, que es lo que evita que la cadena se corte en cualquiera de ellas.
func TestRequestCommentsRechazaLasTresNegativasYSiempreRearmaElTick(t *testing.T) {
	for _, c := range []struct {
		nombre  string
		prepara func(*testing.T) Model
	}{
		{"sin selección", func(t *testing.T) Model {
			return newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
		}},
		{"forge desconocido para el ítem", func(t *testing.T) Model {
			// El ítem se selecciona con su forge presente y LUEGO se quita del mapa, que es
			// lo que pasa cuando un forge se deshabilita mientras su consulta está en
			// vuelo: el ítem sigue en pantalla y su forge ya no está.
			//
			// Y el orden es lo que hace falta, y no es un detalle. La primera versión creó
			// el modelo con un solo adapter de gitlab e intentó seleccionar un ítem de
			// github, y `conSeleccion` no lo encontraba en ninguna sección —`selected()`
			// devuelve `false`—, así que `requestComments` salía por el guard de "no hay
			// selección" de la línea 59 y la rama de la 67 no se tocaba. El código estaba
			// bien y el fixture no llegaba: el test pasaba sin probar lo que dice que
			// prueba, que es la forma más silenciosa de test roto que hay.
			m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
			m = conSeleccion(t, m, mkItem("github", "github.com", "acme/widget", "uno", 7, ""))
			if _, ok := m.selected(); !ok {
				t.Fatal("el fixture no sirve: el ítem no quedó seleccionado")
			}
			delete(m.byForge, "github")
			return m
		}},
		{"conversación ya cargada", func(t *testing.T) Model {
			m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
			it := mkItem("github", "github.com", "acme/widget", "uno", 7, "")
			m = conSeleccion(t, m, it)
			m.comments[it.ID()] = &commentState{list: []model.Comment{{Author: "x", Body: "y"}}, ready: true}
			return m
		}},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			m := c.prepara(t)
			if cmd := m.requestComments(); cmd == nil {
				t.Error("no se rearmó el tick: la cadena se corta y el siguiente cambio de " +
					"selección no se nota")
			}
			// Y el propio tick también sale: es el primer eslabón de la cadena.
			if cmd := m.commentsCmd(); cmd == nil {
				t.Error("commentsCmd devolvió nil")
			}
		})
	}
}
