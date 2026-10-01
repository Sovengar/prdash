package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"prdash/internal/forge"
	"prdash/internal/testutil"
)

// Los cuatro guards de update.go que sobrevivían son cuatro preguntas distintas y
// ninguna es difícil: una sobre un contador, una sobre si hay ítem y si trae URL, una
// sobre qué texto se pone en marcha, y una sobre cuántas filas salta una página.
//
// La común es que todas son `if algo { … }` cuyo cuerpo se nota en un estado
// observable, y todas se piden por la frontera: el valor exacto de la condición, uno
// más y uno menos.

// TestReleaseReaderNoBajaDeCero: el contador de lectores del canal tiene suelo en cero.
//
// El invariante declarado es que hay exactamente un lector en vuelo, y el suelo está
// para que un `releaseReader` de más no lo haga negativo: un contador negativo
// significaría que se entregó un evento que nadie esperaba, y a partir de ahí el rearme
// del siguiente `withPump` deja de cuadrar.
//
// Y la frontera es el cero, que es donde la condición decide: con `readers == 1` el
// decremento es lo de siempre y con `readers == 0` no hay nada que soltar.
func TestReleaseReaderNoBajaDeCero(t *testing.T) {
	// Con lectores, cada `releaseReader` suelta UNO y solo uno.
	for _, desde := range []int{1, 2, 5} {
		m := newTestModel(t)
		m.readers = desde
		m.releaseReader()
		if m.readers != desde-1 {
			t.Errorf("partiendo de %d lectores quedó en %d, want %d", desde, m.readers, desde-1)
		}
	}

	// Y en el suelo, seguir liberando no baja de cero. Con la condición `>=` en vez de
	// `>`, el cero se trata como "había uno" y baja a −1, y de ahí ya solo vuelve a
	// subir con cada rearme.
	m := newTestModel(t)
	m.readers = 0
	for i := range 4 {
		m.releaseReader()
		if m.readers < 0 {
			t.Fatalf("tras %d liberaciones desde cero el contador quedó en %d: el suelo "+
				"a cero es lo que impide que un evento de más lo hunda", i+1, m.readers)
		}
	}
	if m.readers != 0 {
		t.Errorf("el contador quedó en %d, want 0", m.readers)
	}
}

// TestAbrirElNavegadorSoloAvisaSinURL: el aviso de "no URL to open" es para los ítems SIN
// URL, no para todos.
//
// Y la condición es `!ok || it.URL == ""`, que son dos términos distintos: que no haya
// nada seleccionado, o que lo seleccionado no traiga URL. El segundo es el que se puede
// equivocar por el otro lado, y el error es el peor de los dos en apariencia: con un ítem
// que sí tiene URL, la guarda mal puesta avisa "no URL to open" sobre un ítem que la
// tiene, y el atajo de abrir el navegador no abre nada.
//
// La disyunción entera se prueba con los dos términos, porque una disyunción que solo se
// prueba con un término no está probada.
func TestAbrirElNavegadorSoloAvisaSinURL(t *testing.T) {
	tecla := newTestModel(t).cfg.KeyFor("open-browser")
	if tecla == "" {
		t.Fatal("la config por defecto no tiene tecla para open-browser y el test la necesita")
	}

	// El adapter hace falta: `rebuild` lee de los forges configurados, y sin uno el
	// ítem de la instantánea no llega a la lista y `selected` no devuelve nada.
	adapt := &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"}

	// Con un ítem QUE TIENE URL: se abre, y sin aviso.
	conURL := newTestModel(t, adapt)
	conURL.applySnapshot(snapshotCon(mkItem("github", "github.com", "acme/widget", "uno", 1, "")))
	conURL.rebuild()
	it, ok := conURL.selected()
	if !ok {
		t.Fatal("no hay nada seleccionado y el test necesita un ítem")
	}
	if it.URL == "" {
		t.Fatal("el ítem de `mkItem` no trae URL y el test necesita uno con URL")
	}

	updated, cmd := conURL.handleKey(tea.KeyPressMsg{Code: []rune(tecla)[0], Text: tecla})
	tras := updated.(Model)
	if cmd == nil {
		t.Error("con un ítem que tiene URL, open-browser no devolvió comando: el " +
			"navegador no se abre")
	}
	if texto := textoDeAvisos(tras); texto != "" {
		t.Errorf("con un ítem que tiene URL sebordó el aviso %q: el atajo tiene que "+
			"abrir, no avisar", texto)
	}

	// Y SIN selección: el aviso es lo correcto, porque no hay nada que abrir.
	vacia := newTestModel(t, adapt)
	vacia.applySnapshot(snapshotCon())
	vacia.rebuild()
	updated, cmd = vacia.handleKey(tea.KeyPressMsg{Code: []rune(tecla)[0], Text: tecla})
	if cmd != nil {
		t.Error("sin nada seleccionado open-browser devolvió comando, y no hay nada " +
			"que abrir")
	}
	if texto := textoDeAvisos(updated.(Model)); !strings.Contains(texto, "no URL to open") {
		t.Errorf("sin nada seleccionado el aviso fue %q, y tiene que mencionar que no hay URL", texto)
	}

	// Y con un ítem SIN URL, que es el otro término de la disyunción y el que la guarda
	// `it.URL == ""` existe para coger. Aquí el aviso es lo correcto y no hay comando:
	// abrir una URL vacía abriría la página de inicio del navegador.
	sinURL := newTestModel(t, adapt)
	sinItem := mkItem("github", "github.com", "acme/widget", "uno", 2, "")
	sinItem.URL = ""
	sinURL.applySnapshot(snapshotCon(sinItem))
	sinURL.rebuild()

	updated, cmd = sinURL.handleKey(tea.KeyPressMsg{Code: []rune(tecla)[0], Text: tecla})
	if cmd != nil {
		t.Error("con un ítem sin URL open-browser devolvió comando: abrir una URL " +
			"vacía abre la página de inicio del navegador")
	}
	if texto := textoDeAvisos(updated.(Model)); !strings.Contains(texto, "no URL to open") {
		t.Errorf("con un ítem sin URL el aviso fue %q, y tiene que mencionar que no hay URL", texto)
	}
}

// textoDeAvisos junta los mensajes de los avisos vivos del modelo, que es lo que el
// usuario lee del último aviso. Vacío cuando no hay ninguno.
func textoDeAvisos(m Model) string {
	var out []string
	for _, x := range m.toast.toasts {
		out = append(out, x.message)
	}
	return strings.Join(out, " | ")
}

// TestElAvisoDeMergeDigaEnQueModoVa: el progreso de un merge lleva el modo, y el de
// cualquier otra acción no.
//
// Es la pregunta que decide si el usuario espera un squash o un rebase: son operaciones
// con tiempos muy distintos y el texto es lo único que se lo dice mientras corre. Por eso
// el modo entra SOLO en merge —en approve no hay modo— y no al revés.
//
// Y el otro lado de la frontera importa igual: una acción que NO es merge tiene
// que decir exactamente lo de antes, sin paréntesis. Si la condición se invirtiera,
// approve diría "approve (squash) en curso" y merge no diría nada, que es justo lo
// contrario de lo que ayuda.
func TestElAvisoDeMergeDigaEnQueModoVa(t *testing.T) {
	modos := []forge.MergeMode{forge.Squash, forge.Rebase, forge.MergeCommit}
	for _, modo := range modos {
		got := actionProgressNotice(forge.ActionMerge, modo)
		if !strings.Contains(got, modo.Label()) {
			t.Errorf("el aviso de merge salió %q y no menciona el modo %q: quien "+
				"espera un rebase tiene que leerlo antes de que termine", got, modo.Label())
		}
		if !strings.Contains(got, string(forge.ActionMerge)) {
			t.Errorf("el aviso de merge salió %q y no menciona la acción", got)
		}
		if !strings.Contains(got, "en curso") {
			t.Errorf("el aviso de merge salió %q y no dice que está en curso", got)
		}
	}

	for _, kind := range []forge.ActionKind{forge.ActionApprove, forge.ActionRetarget} {
		got := actionProgressNotice(kind, forge.Squash)
		if !strings.Contains(got, string(kind)) {
			t.Errorf("el aviso de %s salió %q y no menciona la acción", kind, got)
		}
		if !strings.Contains(got, "en curso") {
			t.Errorf("el aviso de %s salió %q y no dice que está en curso", kind, got)
		}
		// Y sin paréntesis de modo: approve no tiene modo, así que anunciarlo sería
		// mentir sobre algo que no existe.
		if strings.Contains(got, "(") {
			t.Errorf("el aviso de %s sale %q con un paréntesis de modo: %s no tiene modo "+
				"y anunciarlo es decir algo que no ocurre", kind, got, kind)
		}
	}
}

// TestPageRowsSaltaLoQueSeVe: el salto de página mide la ventana de lista, y sin altura
// conocida cae en seis filas.
//
// La razón de ser es de lectura: `pgdn` tiene que avanzar justo lo que se ve, porque si
// avanzara más saltaría ítems por encima sin pasar por ellos. Y el caso sin altura
// conocida es el que ocurre antes del primer `WindowSizeMsg` —y también cuando el layout
// no deja filas—, donde no hay ventana que medir.
//
// Y la frontera de la condición es el CERO de filas, que es justo lo que pasa antes del
// primer tamaño: `computeLayout` devuelve un layout vacío, `bodyLines` es cero y el salto
// tiene que ser la media docena. Con la condición invertida saltaría cero filas y `pgdn`
// no movería nada, que es el peor síntoma posible porque parece que la tecla no responde.
func TestPageRowsSaltaLoQueSeVe(t *testing.T) {
	// Con altura conocida, el salto es la ventana de lista.
	for _, alto := range []int{20, 40, 60} {
		m := newTestModel(t)
		m.width, m.height = 160, alto
		want := m.layout().bodyLines
		if want < 1 {
			t.Fatalf("con altura %d el layout dio bodyLines=%d, y el test necesita un "+
				"número positivo para comparar", alto, want)
		}
		if got := m.pageRows(); got != want {
			t.Errorf("con altura %d pageRows=%d, want %d: la tecla tiene que avanzar "+
				"lo que se ve", alto, got, want)
		}
	}

	// Y sin altura conocida, la media docena. Las alturas cero y negativa son las dos que
	// dejan el layout vacío: `computeLayout` devuelve `layout{}` si la altura no es
	// positiva, así que `bodyLines` es cero en ambas.
	for _, alto := range []int{0, -10} {
		m := newTestModel(t)
		m.width, m.height = 160, alto
		if got := m.pageRows(); got != 6 {
			t.Errorf("con altura %d pageRows=%d, want 6: sin ventana que medir el salto "+
				"es la media docena, y con la condición invertida saltaría cero filas y "+
				"pgdn no movería nada", alto, got)
		}
	}
}
