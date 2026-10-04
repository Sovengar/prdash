package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

// El último grupo: las tres ramas donde la TUI se ahorra trabajo, y las tres son del mismo tipo —
// "esto ya lo sé, no lo vuelvas a preguntar".
//
// Y las tres tienen en común que el ahorro es invisible cuando funciona y CARO cuando no: una
// página que se vuelve a pedir entera cada seis segundos no se nota; un `pane` que se lanza dos
// veces en la misma celda se nota mucho.

// TestUnStreamSinCambiosNoSeVuelveAPaginarEnteroYLoDice: `streamForge` con la cabecera igual.
//
// Y es la 곳에 que decide cuánta de forge se consume en cada refresco, y el ahorro es de
// verdad: con un inbox de veinte PRs en seis páginas, repedir la primera en cada ciclo que no
// cambia nada son seis llamadas cada seis segundos por forge.
//
// Y por eso el atajo tiene las cuatro condiciones: `prev.complete` —que el ciclo anterior
// terminó de paginar, no que se quedó a medias—, `page.More` —que queda algo por ver—, un cursor
// no vacío y que sea EL MISMO. Con cualquiera que falte no se puede afirmar que el resto no ha
// cambiado, y se pagina entero.
//
// Y el mensaje que sale lleva `unchanged`, que es lo que permite al inbox conservar lo cacheado
// en vez de sustituirlo por una lista vacía.
func TestUnStreamSinCambiosNoSeVuelveAPaginarEnteroYLoDice(t *testing.T) {
	for _, c := range []struct {
		nombre    string
		prev      streamHead
		page      forge.Page
		wantAtajo bool
	}{
		{
			nombre:    "ciclo completo y mismo cursor",
			prev:      streamHead{cursor: "CUR2", complete: true},
			page:      forge.Page{Next: "CUR2", More: true},
			wantAtajo: true,
		},
		{
			nombre:    "el ciclo anterior quedó a medias",
			prev:      streamHead{cursor: "CUR2", complete: false},
			page:      forge.Page{Next: "CUR2", More: true},
			wantAtajo: false,
		},
		{
			nombre:    "no queda nada por ver",
			prev:      streamHead{cursor: "CUR2", complete: true},
			page:      forge.Page{Next: "", More: false},
			wantAtajo: false,
		},
		{
			nombre:    "el cursor ha cambiado",
			prev:      streamHead{cursor: "CUR1", complete: true},
			page:      forge.Page{Next: "CUR2", More: true},
			wantAtajo: false,
		},
	} {
		if got := unchangedHead(c.prev, c.page); got != c.wantAtajo {
			t.Errorf("%s: unchangedHead = %v, want %v", c.nombre, got, c.wantAtajo)
		}
	}

	// Y el efecto observable: con el atajo, `streamForge` manda UNA página por stream con la
	// marca `unchanged` y la lista VACÍA, y no pagina más.
	//
	// Y `forge.Stream` recorre TODAS las listas, no una, así que el fake tiene que responder a
	// todas con la misma cabecera y el `prev` tiene que traer las cuatro claves. Con una sola
	// lista, tres de las cuatro irían por el camino largo y el recuento de páginas no
	// distinguiría el atajo de la paginación normal.
	pages := map[testutil.FakeKey][]forge.Page{}
	prev := map[streamKey]streamHead{}
	for _, q := range forge.Streams {
		pages[testutil.FakeKey{Section: q.Section, Kind: q.ReviewKind}] = []forge.Page{
			{Next: "CUR2", More: true},
			{Items: []model.Item{mkItem("github", "github.com", "acme/widget", "uno", 7, "")}},
		}
		prev[streamKey{forge: "github", section: q.Section, kind: q.ReviewKind}] =
			streamHead{cursor: "CUR2", complete: true}
	}
	adapter := &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com", Pages: pages}
	events := make(chan event, 32)
	streamForge(context.Background(), context.Background(), events, adapter, 3, prev)

	var vistos []pageMsg
	for len(events) > 0 {
		e := <-events
		if p, ok := e.(pageMsg); ok {
			vistos = append(vistos, p)
		}
	}

	if len(vistos) != len(forge.Streams) {
		t.Fatalf("salieron %d páginas con el atajo, want %d (una por stream): o se "+
			"paginó entero o algún stream no emitió", len(vistos), len(forge.Streams))
	}
	for _, p := range vistos {
		if !p.unchanged {
			t.Errorf("una página salió sin la marca de `unchanged`: %+v", p)
		}
		// Y la lista vacía, que es la mitad del contrato: si `unchanged` viniera con ítems, el
		// inbox los sustituiría por una lista parcial y perdería la paginación que ya tenía
		// en la caché. La marca sin la lista vacía es la mitad de un atajo inútil.
		if len(p.items) != 0 {
			t.Errorf("una página marcada como `unchanged` trae %d ítems: sustituirían la "+
				"caché por una lista parcial", len(p.items))
		}
		// Y el ciclo viaja en el mensaje, que es lo que permite descartar los de un ciclo
		// anterior sin haberlos esperado.
		if p.cycle != 3 {
			t.Errorf("la página lleva el ciclo %d, want 3", p.cycle)
		}
	}
}

// TestUnItemDeUnForgeDesconocidoNoConsultaYNoRompeLaCadenaDeComentarios: el `a == nil`.
//
// Y es un caso que se da de verdad: un ítem llega de un forge que se deshabilitó mientras su
// consulta estaba en vuelo. El ítem sigue en pantalla y el forge ya no está en el mapa.
//
// Y lo que hay que comprobar es la mitad invisible: la cadena de ticks se rearma IGUAL. Un
// guard que devolviera un `nil` en vez del tick dejaría la cadena muerta, y a partir de ahí
// ningún cambio de selección volvería a consultar la conversación —que es exactamente lo que
// esa cadena existe para—.
func TestUnItemDeUnForgeDesconocidoNoConsultaYNoRompeLaCadenaDeComentarios(t *testing.T) {
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "gitlab", HostName: "gitlab.example.com"})
	// El ítem es de github, que no está en el mapa de forges.
	m = conSeleccion(t, m, mkItem("github", "github.com", "acme/widget", "uno", 7, ""))

	cmd := m.requestComments()
	if cmd == nil {
		t.Fatal("la cadena de comentarios se corta con un forge desconocido: a partir de " +
			"aquí ningún cambio de selección volvería a consultar la conversación")
	}
	// Y no se creó estado para ese ítem: una entrada `&commentState{}` sin consultar
	// significa "ya está cargada" para la siguiente pasada, y la conversación no volvería a
	// pedirse nunca.
	if _, hay := m.comments[mkItem("github", "github.com", "acme/widget", "uno", 7, "").ID()]; hay {
		t.Error("se marcó el ítem como consultado sin consultarlo: el próximo tick lo daría " +
			"por cargado y no volvería a pedir la conversación")
	}
}

// TestAbrirElNavegadorConURLDevuelveElComandoYNoUnAvisoDeFallo: el camino bueno de `open-browser`.
//
// Y es la mitad que faltaba: con una URL vacía el guard avisa, y con una URL de verdad lo que
// tiene que salir es un `tea.Cmd` —el que busca el abridor y lo lanza— y no un aviso.
//
// Y la asimetría importa porque las dos cosas se parecen en el papel: un aviso "abriendo la
// URL" y un aviso de fallo son los dos texto, y el primero se pinta en verde mientras el
// segundo en rojo. Que el camino bueno devuelva un comando y no un aviso es lo que evita que
// el popup se cierre como si hubiera fallado.
func TestAbrirElNavegadorConURLDevuelveElComandoYNoUnAvisoDeFallo(t *testing.T) {
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	it := mkItem("github", "github.com", "acme/widget", "uno", 7, "")
	it.URL = "https://github.com/acme/widget/pull/7"
	m = conSeleccion(t, m, it)

	_, cmd := pulsar(t, m, "o")
	if cmd == nil {
		t.Fatal("abrir una URL válida no devolvió comando: el popup se cerraría como si " +
			"el navegador hubiera fallado")
	}
	// Y sin aviso de por medio: el "abriendo" sale del propio comando, después, cuando ya se
	// sabe que el abridor existe. Ponerlo antes sería prometer algo que puede no pasar.
	if av := lastToast(m); strings.Contains(av, "abriendo") {
		t.Errorf("hay un aviso %q antes de intentar abrir: se promete algo que puede fallar", av)
	}
	// Y el comando lleva la URL del ítem, no otra: el `Cmd` se evalúa más tarde y para entonces
	// la selección puede haber cambiado.
	// El seam se anula para que no abra nada de verdad en la máquina de quien corre el test.
	m2 := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	m2 = conSeleccion(t, m2, it)
	var abierta string
	m2.openURL = func(u string) error { abierta = u; return nil }
	_, cmd2 := pulsar(t, m2, "o")
	if cmd2 == nil {
		t.Fatal("abrir con el seam no devolvió comando")
	}
	if msg, ok := cmd2().(notifyMsg); !ok || !strings.Contains(msg.text, it.URL) {
		t.Errorf("el comando de abrir no menciona la URL del ítem: %+v", msg)
	}
	// Y el abridor recibió EXACTAMENTE esa URL, que es lo que dice que el `Cmd` se evaluó con
	// la del ítem y no con otra: el `Cmd` se ejecuta después de que `Update` devuelva, y para
	// entonces la selección puede haber cambiado.
	if abierta != it.URL {
		t.Errorf("el abridor recibió %q, want %q", abierta, it.URL)
	}
}

// El reloj solo aparece en la aserción de estabilidad del spinner.
var _ = time.Second

// TestLaCadenaDeTicksDevuelveSuMensajeYNoSoloUnNoNil: los dos `tea.Tick`.
//
// Y son las dos cadenas que mantienen viva la sesión, y la diferencia entre las dos es lo que
// las hace no triviales: un `tea.Cmd` que devuelve `nil` ya es un fallo de forma —nada se
// rearma— y un `tea.Cmd` que devuelve algo que no es un `tea.Msg` tampoco sirve, porque
// bubbletea lo que espera es el mensaje.
//
// Y hay que EVALUAR el `Cmd` para comprobarlo, no solo comprobar que no es nil. `tea.Tick`
// devuelve una función que duerme el intervalo y luego produce el mensaje, así que un test que
// solo mira "no es nil" no comprueba nada del contenido. Y los dos intervalos son distintos: el
// de comentarios son 200ms fijos y el del refresco sale de la config, que el test pone a 1ms
// para no pagar una espera.
//
// Y el intervalo del refresco va a 1ms y no a 0 a propósito: con 0 el tick se rearmaría en bucle
// sin parar, que es justo el bug que `tickCmd` previene con `d <= 0`.
func TestLaCadenaDeTicksDevuelveSuMensajeYNoSoloUnNoNil(t *testing.T) {
	// La de comentarios, con su intervalo fijo.
	m := newTestModel(t)
	cmd := m.commentsCmd()
	if cmd == nil {
		t.Fatal("commentsCmd devolvió nil: la cadena de la conversación se corta")
	}
	if msg, ok := cmd().(commentsTickMsg); !ok {
		t.Errorf("commentsCmd devolvió %T, want commentsTickMsg", msg)
	}

	// Y la del refresco, con la config puesta a 1ms.
	m2 := newTestModel(t)
	m2.cfg.RefreshInterval = time.Millisecond
	tick := m2.tickCmd()
	if tick == nil {
		t.Fatal("tickCmd devolvió nil con un intervalo válido")
	}
	if msg, ok := tick().(tickMsg); !ok {
		t.Errorf("tickCmd devolvió %T, want tickMsg", msg)
	}
}

// TestLaFlechaArribaTambienMueveElCursorDelSelectorDeRamas: `case "up"`.
//
// Y es la tecla que falta entre `j`/`k` y las flechas, y la que se cuela en una tabla de teclas
// porque funciona en la lista principal y nadie la prueba en el popup.
//
// Y la asimetría con `j`/`k` es lo que la hace útil: `k` NO mueve el cursor cuando hay un filtro
// escrito —pasa a ser letra del filtro—, pero `up` siempre mueve. Con lo que se puede escribir
// en el filtro —cualquier letra— y donde no se puede perder la navegación porque no sea texto.
func TestLaFlechaArribaTambienMueveElCursorDelSelectorDeRamas(t *testing.T) {
	// Con el filtro vacío: las dos mueven.
	m := modelEnRetarget(t, retargetChoosing)
	if antes := pulsarM(t, m, "down").retarget.cursor; pulsarM(t, pulsarM(t, m, "down"), "up").retarget.cursor == antes {
		t.Error("con el filtro vacío, up no deshizo el movimiento de down")
	}

	// Con el filtro escrito: `k` es letra y `up` sigue moviendo. Es el caso que justifica
	// tener las dos teclas.
	m2 := modelEnRetarget(t, retargetChoosing)
	m2.retarget.query = "re"
	despuesDeK := pulsarM(t, m2, "k")
	if despuesDeK.retarget.cursor != 0 {
		t.Errorf("con filtro escrito, k movió el cursor: se escribiría y se navegaría a la vez")
	}
	m2.retarget.cursor = 2
	sube := pulsarM(t, m2, "up")
	if sube.retarget.cursor != 1 {
		t.Errorf("con filtro escrito, up dejó el cursor en %d, want 1: la navegación no puede "+
			"depender de lo que haya escrito", sube.retarget.cursor)
	}
	// Y el filtro no se tocó: `up` es una tecla de control, no de escribir.
	if sube.retarget.query != "re" {
		t.Errorf("up alteró el filtro: quedó %q", sube.retarget.query)
	}
}

// TestUnListadoDeRamasConAvisosLosPegaEnElMensajeEn vez de Tirarlos: el `fetchBranches` con
// avisos.
//
// Y es el camino que convierte una respuesta parcial en un aviso, y la regla de por qué el aviso
// va DENTRO del mismo mensaje que las ramas es concreta: el popup solo pinta un error o un
// listado, y si los dos vienen por el canal de eventos en mensajes separados habría que
// emparejarlos por el número de secuencia para saber cuál va con cuál.
//
// Y el caso que de verdad pasa: el forge contesta las ramas PERO con un aviso —una columna que
// no se pudo consultar, un permiso parcial— y el popup tiene que decirlo. Sin el aviso, el
// usuario cambiaría la base creyendo que la lista es completa, y un cambio de base sobre una
// lista incompleta es peor que no cambiar la base: lo aplica en el forge.
func TestUnListadoDeRamasConAvisosLosPegaEnElMensajeEnVezDeTirarlos(t *testing.T) {
	ref := model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget"}
	a := &testutil.FakeAdapter{
		ForgeName:   "github",
		HostName:    "github.com",
		BranchLists: map[string][]string{ref.Project: {"main", "feat/x"}},
		BranchWarnings: map[string][]model.Warning{
			ref.Project: {{Kind: "permission", Msg: "sin permiso para leer refs"}},
		},
	}
	m := newTestModel(t, a)
	m.retarget.state = retargetListing
	m.retarget.item = model.NewItem(ref, 7)
	// Y un canal propio, con margen para no bloquear a la goroutine: `fetchBranches` publica
	// desde un `go` y un canal sin margen lo dejaría esperando para siempre. El canal que
	// `New` crea va de 256 porque es el de toda la sesión; aquí solo se prueba una llamada.
	m.events = make(chan event, 4)

	// `fetchBranches` devuelve un `tea.Cmd` cuyo trabajo real es arrancar la goroutine. Sin
	// llamarlo no se consulta nada, y la línea que se quiere probar —pegar los avisos al
	// mensaje— está dentro de esa goroutine.
	m.fetchBranches(m.retarget.item)

	// Y el mensaje llega por el canal, que es lo que la TUI consume. Con `withTimeout` porque
	// la goroutine es real: un fallo dentro de ella no hace fallar el test, se queda callado y
	// el `for` de abajo no vería nada —por eso hay un `t.Fatal` si no llega nada.
	varSaw := false
	for esperando := time.Now().Add(2 * time.Second); ; {
		select {
		case e := <-m.events:
			msg, ok := e.(branchesMsg)
			if !ok {
				continue
			}
			varSaw = true
			if len(msg.names) != 2 {
				t.Errorf("llegaron %d ramas, want 2: los avisos no deben tirar el listado",
					len(msg.names))
			}
			if !strings.Contains(msg.errMsg, "sin permiso") {
				t.Errorf("el aviso del forge no llegó al mensaje: errMsg=%q", msg.errMsg)
			}
			// Y el mensaje lleva el `seq` del ciclo, que es lo que permite descartar los
			// de una petición vieja cuando el popup se cerró y se reabrió.
			if msg.seq != m.branchSeq {
				t.Errorf("el mensaje lleva seq=%d y el modelo va por %d: sin eso un listado "+
					"obsoleto se aceptaría", msg.seq, m.branchSeq)
			}
		default:
			if varSaw || !time.Now().Before(esperando) {
				goto comprobado
			}
			time.Sleep(2 * time.Millisecond)
		}
	}
comprobado:
	if !varSaw {
		t.Fatal("no llegó ningún branchesMsg por el canal de eventos")
	}
}
