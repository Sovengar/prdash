package tui

import (
	"context"
	"image"
	"strings"
	"testing"

	"prdash/internal/forge/model"
	"prdash/internal/herdr"
	"prdash/internal/testutil"
	"prdash/internal/worktree"
)

// Estas son las ramas sueltas que quedan en `tui`: funciones de una línea, guards de cursor y
// de vista. Individualmente no son nada; juntas son la diferencia entre una suite que sirve
// para Refactorizar y una que obliga a abrir el fichero a mano cada vez que se toca algo.
//
// Y dos de ellas merecen nombre porque su ausencia esconde bugs:
//
//   - `Wiring` es el accessor que permite comprobar el cableado del modelo SIN abrir sus
//     campos desde fuera del paquete. Existió sin un solo test que lo llamara, y un accessor sin
//     callers es un accessor que nadie ha comprobado que devuelva lo que dice.
//   - `pageRows` con la vista sin altura tiene un default, y ese default es lo que evita una
//     división por cero en el scroll.

// TestWiringDevuelveExactamenteLoQueElModeloTienePuesto: el accessor del cableado.
//
// Y es la pieza que hace testeable `cmd/prdash`, que arma el modelo desde otro paquete y no
// puede tocar sus campos privados. El contrato es que devuelva las CINCO dependencias tal cual,
// con nil para las que no están.
//
// Y lo que se fija es la identidad, no la igualdad de valor: el accessor tiene que devolver la
// interfaz CONCRETA que se puso, no una copia ni una envoltura. Un `Wiring` que construyera
// adaptadores nuevos sería inútil para el test que lo usa y silenciosamente distinto del modelo
// real.
func TestWiringDevuelveExactamenteLoQueElModeloTienePuesto(t *testing.T) {
	// Con todo puesto.
	completa := newTestModel(t)
	completa.mounter = &mounterFalso{}
	completa.simulator = &simuladorFalso{disponible: true}
	completa.graphics = &graphicsFalso{}
	completa.reviewLookup = &reviewLookupFalso{}
	completa.reviewRemover = &reviewRemoverFalso{}

	w := completa.Wiring()
	if w.Mounter == nil || w.Simulator == nil || w.Graphics == nil ||
		w.ReviewLookup == nil || w.ReviewRemover == nil {
		t.Errorf("Wiring perdió alguna dependencia: %+v", w)
	}
	// Y son las MISMAS instancias, que es lo que hace que el accessor sirva.
	if w.Simulator != completa.simulator {
		t.Error("Wiring devolvió otro Simulator: un accessor que copia no sirve para comprobar el cableado")
	}
	if w.Graphics != completa.graphics {
		t.Error("Wiring devolvió otro Graphics")
	}

	// Y con nada puesto: cinco nils, que es exactamente lo que el modelo trata como "pide
	// instalar X". Una dependencia ausente tiene que ser nil y no un valor cero de interfaz,
	// que no es lo mismo: una interfaz con valor cero no es nil y un `if x == nil` no la ve.
	vacia := newTestModel(t)
	vacia.mounter = nil
	vacia.simulator = nil
	vacia.graphics = nil
	wv := vacia.Wiring()
	if wv.Mounter != nil || wv.Simulator != nil || wv.Graphics != nil ||
		wv.ReviewLookup != nil || wv.ReviewRemover != nil {
		t.Errorf("Wiring con el modelo vacío devolvió algo: %+v", wv)
	}
}

// TestSinAlturaConocidaElScrollAvanzaSeisFilas: el default de `pageRows`.
//
// Y es una degradación honesta y explícita: sin altura no se sabe cuántas filas caben, y seis es
// una aproximación que desplaza el cursor lo justo para que se vea. Lo que NO puede pasar es una
// división por cero, y eso es lo que el default evita.
//
// Y el caso que importa es el de una altura CERO —que llega cuando la terminal aún no ha
// informado de su tamaño—, porque es el primero de la sesión y por tanto el que se ve siempre.
func TestSinAlturaConocidaElScrollAvanzaSeisFilas(t *testing.T) {
	m := newTestModel(t)
	m.height = 0 // terminal sin tamaño todavía

	if filas := m.pageRows(); filas != 6 {
		t.Errorf("sin altura conocida pageRows = %d, want 6", filas)
	}
	// Y el scroll no revienta con altura cero.
	m.scroll = 0
	m.scroll += m.pageRows()
	if m.scroll != 6 {
		t.Errorf("el scroll quedó en %d", m.scroll)
	}

	// Y con altura conocida se usa la que se ve, que es el otro camino.
	m2 := newTestModel(t)
	m2.height = 40
	if filas := m2.pageRows(); filas == 6 {
		t.Error("con altura conocida sigue usando el default de seis filas")
	}
}

// TestUnForgeQueTerminaDeCargarDejaDeEstarEnLoading: `st.loading` de la página.
//
// Y es el final de una cadena: el ciclo arranca, cada página pone su forge en `loading`, y el
// spinner global deja de girar cuando el último termina. Si este `st.loading = false` faltara,
// el forge se quedaría marcado como cargando para siempre y la columna de ese forge no volvería
// a mostrar datos.
//
// Y el caso que hay que mirar es el de una página OBSOLETA: no toca nada —ni el loading ni los
// datos— porque pertenece a un ciclo anterior. Solo rearma la bomba. Un `loading = false` en ese
// camino apagaría el spinner del forge que SÍ está cargando.
func TestUnForgeQueTerminaDeCargarDejaDeEstarEnLoading(t *testing.T) {
	m := modeloConStatusesCargando(t)

	// Un `forgeDoneMsg` vigente de github: apaga su loading y NO el de gitlab.
	//
	// Y el mensaje es `forgeDoneMsg` y no `pageMsg`, que es un despiste que hice: el que
	// apaga el loading es el de "este forge terminó", que llega cuando el último stream de
	// ese forge contesta. Mi primera versión mandaba un `pageMsg` y el loading no se apagaba
	// —el código hacía bien y el test iba al mensaje equivocado—.
	salida := send(t, m, forgeDoneMsg{cycle: 3, forge: "github"})
	got := salida
	if got.statuses["github"].loading {
		t.Error("forgeDoneMsg de github no apagó su loading: el spinner de ese forge no pararía")
	}
	if !got.statuses["gitlab"].loading {
		t.Error("forgeDoneMsg de github apagó el loading de gitlab, que sigue cargando")
	}

	// Y un forge DESCONOCIDO no revienta: se da el caso cuando un forge se deshabilita
	// mientras su consulta está en vuelo.
	desconocido := send(t, m, forgeDoneMsg{cycle: 3, forge: "bitbucket"})
	if _, ok := desconocido.statuses["bitbucket"]; ok {
		t.Error("forgeDoneMsg de un forge desconocido lo creó en el mapa")
	}

	// Y uno OBSOLETO no toca ninguno de los dos.
	//
	// Y con un modelo NUEVO, porque el mapa de `statuses` se comparte entre las copias y sus
	// valores son PUNTEROS: el `forgeDoneMsg` vigente de arriba ya dejó `github.loading` a
	// false en el mapa que ambos modelos ven, y reusar `m` hacía que el caso obsoleto saliera
	// con el loading apagado de antemano. Mi primera versión lo reusaba y el aserto fallaba
	// por un efecto lateral del caso anterior, no por el obsoleto.
	//
	// Y el efecto no es un capricho del test: bubbletea pasa el modelo por valor, así que
	// cualquier modelo "anterior" que alguien guarde comparte el mapa con el vivo. Solo es
	// inofensivo porque bubbletea descarta el anterior, y por eso es inofensivo HOY.
	obsoleta := send(t, modeloConStatusesCargando(t), forgeDoneMsg{cycle: 1, forge: "github"})
	if !obsoleta.statuses["github"].loading || !obsoleta.statuses["gitlab"].loading {
		t.Error("un forgeDoneMsg obsoleto apagó un loading: apagaría el spinner de un forge " +
			"que sigue cargando de verdad")
	}
}

// TestAbrirEnElNavegadorSinURLSeNiegaYLoDice: el guard de `open-browser`.
//
// Y es un guard que ya no debería dispararse —`ItemState` con `it.URL` vacío devuelve el valor
// cero y el ítem aplicado encima conserva la URL anterior—, pero si llegara un ítem sin URL, la
// alternativa sería `xdg-open ""`, que arranca sin hacer nada útil y deja un toast sin nada
// detrás.
func TestAbrirEnElNavegadorSinURLSeNiegaYLoDice(t *testing.T) {
	// Sin selección: el mismo guard salta, con el mismo mensaje.
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	salida, cmd := pulsar(t, m, "o")
	got := salida
	if cmd != nil {
		t.Error("abrir sin selección devolvió comando: ejecutaría un visor sin nada")
	}
	// Y el aviso NO distingue "no hay selección" de "el ítem no tiene URL": el guard es uno
	// solo —`!ok || it.URL == ""`— y el mensaje es el de la URL. Mi primera versión pedía el
	// de "selecciona un ítem" y fallaba; los dos casos se resuelven igual y el usuario solo
	// necesita saber que no se va a abrir nada.
	if av := lastToast(got); !strings.Contains(av, "no URL") {
		t.Errorf("el aviso %q no dice que no hay nada que abrir", av)
	}

	// Con un ítem sin URL: el aviso tiene que decir eso y no "abriendo".
	m2 := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	sinURL := mkItem("github", "github.com", "acme/widget", "uno", 7, "")
	sinURL.URL = ""
	m2 = conSeleccion(t, m2, sinURL)

	salida2, cmd2 := pulsar(t, m2, "o")
	got2 := salida2
	if cmd2 != nil {
		t.Error("abrir un ítem sin URL devolvió comando")
	}
	if av := lastToast(got2); !strings.Contains(av, "no URL") {
		t.Errorf("el aviso %q no dice que el ítem no tiene URL", av)
	}
	if lastToastLevel(got2) != toastWarning {
		t.Errorf("nivel %v, want aviso", lastToastLevel(got2))
	}
}

// graphicsFalso es un Graphics que dice que no y no hace nada, para el test de `Wiring`.
type graphicsFalso struct{}

func (graphicsFalso) Available() bool                     { return false }
func (graphicsFalso) CellSize(context.Context) (int, int) { return 1, 2 }
func (graphicsFalso) SetImage(context.Context, string, image.Image, herdr.Placement) error {
	return nil
}
func (graphicsFalso) Clear(context.Context, string) error { return nil }

// reviewLookupFalso y reviewRemoverFalso son los dos puertos de review, que son interfaces y
// no funciones: eso es lo que permite que `Wiring` devuelva "ninguno" con un nil y no con un
// valor cero que un `if == nil` no distinguiría de una dependencia presente.
type reviewLookupFalso struct{}

func (reviewLookupFalso) ActiveReview(model.Item) (worktree.Worktree, bool) {
	return worktree.Worktree{}, false
}

type reviewRemoverFalso struct{}

func (reviewRemoverFalso) RemoveReview(context.Context, model.Item) (bool, string, error) {
	return false, "", nil
}

// modeloConStatusesCargando es un modelo con los dos forges en `loading`, para los casos que
// necesitan un estado limpio.
//
// Y está aparte porque el mapa de `statuses` es COMPARTIDO entre las copias del modelo y sus
// valores son punteros: dos `send` sobre el mismo modelo se ven el uno al otro. No es un
// problema del código —bubbletea descarta el modelo anterior—, pero sí lo es de un test que
// encadena mensajes y espera que cada uno parta del estado original.
func modeloConStatusesCargando(t *testing.T) Model {
	t.Helper()
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	m.cycle = 3
	m.statuses = map[string]*forgeStatus{
		"github": {forge: "github", host: "github.com", loading: true},
		"gitlab": {forge: "gitlab", host: "gitlab.example.com", loading: true},
	}
	return m
}
