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

	// With the shortcut, streamForge sends ONE page per stream with the unchanged mark and an EMPTY
	//list, and does not page further.
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
		if len(p.items) != 0 {
			t.Errorf("una página marcada como `unchanged` trae %d ítems: sustituirían la "+
				"caché por una lista parcial", len(p.items))
		}
		if p.cycle != 3 {
			t.Errorf("la página lleva el ciclo %d, want 3", p.cycle)
		}
	}
}

// This happens for real: an item arrives from a forge that was disabled.
func TestUnItemDeUnForgeDesconocidoNoConsultaYNoRompeLaCadenaDeComentarios(t *testing.T) {
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "gitlab", HostName: "gitlab.example.com"})
	m = conSeleccion(t, m, mkItem("github", "github.com", "acme/widget", "uno", 7, ""))

	cmd := m.requestComments()
	if cmd == nil {
		t.Fatal("la cadena de comentarios se corta con un forge desconocido: a partir de " +
			"aquí ningún cambio de selección volvería a consultar la conversación")
	}
	if _, hay := m.comments[mkItem("github", "github.com", "acme/widget", "uno", 7, "").ID()]; hay {
		t.Error("se marcó el ítem como consultado sin consultarlo: el próximo tick lo daría " +
			"por cargado y no volvería a pedir la conversación")
	}
}

// The half that was missing: with an empty URL the guard warns, and this is the other half.
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
	if av := lastToast(m); strings.Contains(av, "abriendo") {
		t.Errorf("hay un aviso %q antes de intentar abrir: se promete algo que puede fallar", av)
	}
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
	if abierta != it.URL {
		t.Errorf("el abridor recibió %q, want %q", abierta, it.URL)
	}
}

var _ = time.Second

// A tea.Cmd that is only checked with "not nil" checks nothing: the content of the message is the
// whole point.
func TestLaCadenaDeTicksDevuelveSuMensajeYNoSoloUnNoNil(t *testing.T) {
	m := newTestModel(t)
	cmd := m.commentsCmd()
	if cmd == nil {
		t.Fatal("commentsCmd devolvió nil: la cadena de la conversación se corta")
	}
	if msg, ok := cmd().(commentsTickMsg); !ok {
		t.Errorf("commentsCmd devolvió %T, want commentsTickMsg", msg)
	}

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

func TestLaFlechaArribaTambienMueveElCursorDelSelectorDeRamas(t *testing.T) {
	m := modelEnRetarget(t, retargetChoosing)
	if antes := pulsarM(t, m, "down").retarget.cursor; pulsarM(t, pulsarM(t, m, "down"), "up").retarget.cursor == antes {
		t.Error("con el filtro vacío, up no deshizo el movimiento de down")
	}

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
	if sube.retarget.query != "re" {
		t.Errorf("up alteró el filtro: quedó %q", sube.retarget.query)
	}
}

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
	m.events = make(chan event, 4)

	m.fetchBranches(m.retarget.item)

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
