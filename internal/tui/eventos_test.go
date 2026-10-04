package tui

import (
	"context"
	"testing"
	"time"
)

// La bomba de eventos es el invariante que el resto del modelo da por bueno, y no hay un
// solo test de los existentes que la toque. Es lo que permite decir "un único lector" en
// tres sitios distintos del código, y es la clase de infraestructura donde un error es
// silencioso: un lector de más no da error, reparte un evento entre dos sitios y la mitad de
// las actualizaciones se pierden.
//
// Y `waitForEvent` es un `tea.Cmd` que devuelve UN evento del canal. Probarlo no necesita
// bubbletea entero: un Cmd es una función, y la función se puede llamar directamente con un
// canal detrás.

type eventoMarcado struct{ n int }

// TestLaBombaLeeUnEventoYSoloUno: lo que promete el nombre.
//
// Y la parte que importa es el "y solo uno": el patrón de Bubbletea es un Cmd por evento, y
// un Cmd que leyera dos se llevaría el segundo a un sitio que nadie rearmó. Es la razón de
// que la función se llame `waitForEvent` y no `drainEvents`.
func TestLaBombaLeeUnEventoYSoloUno(t *testing.T) {
	ch := make(chan event, 3)
	ch <- eventoMarcado{n: 1}
	ch <- eventoMarcado{n: 2}

	cmd := waitForEvent(ch)

	// El primer Cmd saca el primero.
	ev, ok := cmd().(eventoMarcado)
	if !ok {
		t.Fatal("el Cmd no devolvió un evento")
	}
	if ev.n != 1 {
		t.Errorf("el evento leído es %+v, want el primero", ev)
	}
	// Y NO saca el segundo: sigue en el canal, para el siguiente Cmd.
	//
	// Y se comprueba con `len(ch)`, NO con un receive en un `select` con `default`. Un
	// receive consume, así que la comprobación se llevaba por delante el evento que
	// después leía el segundo `cmd()` — y ese `cmd()` bloqueaba para siempre en un canal
	// vacío, que es el cuelgue más caro que se puede tener en un test. Un assert que
	// observa el canal tiene que observarlo sin tocarlo.
	if n := len(ch); n != 1 {
		t.Errorf("quedan %d eventos en el canal tras un Cmd, want 1: se llevó más de uno", n)
	}

	// El segundo Cmd saca el segundo. Y el orden se respeta, que es lo que evita que dos
	// páginas del mismo stream se apliquen al revés.
	ev2 := cmd().(eventoMarcado)
	if ev2.n != 2 {
		t.Errorf("el segundo evento es %+v, want el segundo", ev2)
	}
}

// TestLaBombaSeDetieneConElCanalCerrado: lo que evita el cuelgue.
//
// Y esto es lo que más caro sale si se rompe. Bubbletea cancela el contexto al salir, lo que
// cierra el canal de eventos, y si un Cmd construyera un evento vacío en vez de `nil` el
// runtime entra en un bucle de reintentos esperando de un canal que ya no tiene nada que
// dar.
//
// `nil` es la señal de bubbletea de "no hay más mensajes". Devolver un `Msg` inventado, o un
// evento vacío, hace que la TUI se quede esperando mensajes que nadie va a mandar.
func TestLaBombaSeDetieneConElCanalCerrado(t *testing.T) {
	ch := make(chan event)
	close(ch)

	got := waitForEvent(ch)()
	if got != nil {
		t.Errorf("con el canal cerrado el Cmd devolvió %v (%T), want nil: nil es lo que le "+
			"dice a bubbletea que no hay más mensajes", got, got)
	}
	// Y pedir más veces no revive el canal ni devuelve otra cosa, que es lo que pasaría si
	// el Cmd reintentara con un select en vez de recibir.
	for i := range 3 {
		if got := waitForEvent(ch)(); got != nil {
			t.Errorf("la llamada %d con el canal cerrado devolvió %v", i, got)
			break
		}
	}
}

// TestPublicarRespetaLaCancelacion: `sendEvent`, que es el otro lado.
//
// Y el caso que hay que probar es el de la cancelación, porque es el que ocurre al salir de
// la TUI: un reader sigue esperando un evento que ya no se va a leer, y publicar sin mirar el
// contexto se queda bloqueado para siempre en una goroutine que nadie va a terminar.
//
// Y el caso bueno también, con el contexto vivo: el evento se publica y se lee.
func TestPublicarRespetaLaCancelacion(t *testing.T) {
	// Con contexto vivo: se publica y se lee.
	ch := make(chan event, 1)
	sendEvent(context.Background(), ch, eventoMarcado{n: 1})

	select {
	case got := <-ch:
		if m, ok := got.(eventoMarcado); !ok || m.n != 1 {
			t.Errorf("se publicó el 1 y llegó %v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("el evento no llegó")
	}

	// Con el contexto YA CANCELADO y sin nadie leyendo: no se bloquea. Eso es lo que prueba
	// el `case <-ctx.Done()` del select.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	sinLeer := make(chan event) // sin buffer y sin lector: publicar aquí bloquearía
	hecho := make(chan struct{})
	go func() {
		defer close(hecho)
		sendEvent(ctx, sinLeer, eventoMarcado{n: 2})
	}()
	select {
	case <-hecho:
	case <-time.After(2 * time.Second):
		t.Fatal("sendEvent se bloqueó con el contexto cancelado: se queda en una goroutine " +
			"que nadie va a terminar")
	}

	// Y con el contexto vivo pero sin lector, publicar BLOQUEA a propósito. Esa es la
	// propiedad que hace que el caso anterior sea necesario en vez de opcional. No se
	// comprueba porque bloquearía el test, pero dejarlo escrito evita que alguien lo lea
	// como un caso forgot y lo "arregle" con un select.
}

// TestElContadorDeLectoresEsLoQuePermiteAfirmarElInvariante: el contador.
//
// Y el contador existe para poder AFIRMAR que hay un solo lector, que es lo que permite
// decir en el código que un Cmd por evento no pierde mensajes. Sin él, esa afirmación es una
// promesa sin prueba, y las promesas sin prueba en la infraestructura de eventos son las que
// se rompen en silencio.
func TestElContadorDeLectoresEsLoQuePermiteAfirmarElInvariante(t *testing.T) {
	m := newTestModel(t)
	m.events = make(chan event, 1)
	m.readers = 0

	cmd := m.armReader()
	if m.readers != 1 {
		t.Errorf("armReader dejó el contador en %d, want 1", m.readers)
	}
	if cmd == nil {
		t.Fatal("armReader devolvió un Cmd nil")
	}

	// Y armar dos veces son dos lectores, que es exactamente lo que el invariante prohíbe en
	// el arranque. Que se pueda comprobar es lo que lo convierte en invariante y no en
	// comentario.
	m.armReader()
	if m.readers != 2 {
		t.Errorf("tras armar dos veces el contador quedó en %d", m.readers)
	}

	// Y el Cmd armado lee del canal del MODELO, no de otro: si `armReader` usara un canal
	// distinto del que el modelo publica, los eventos se irían a un sitio donde nadie mira.
	m.events <- eventoMarcado{n: 9}
	got, ok := cmd().(eventoMarcado)
	if !ok || got.n != 9 {
		t.Errorf("el Cmd de armReader no leyó del canal del modelo: %v", cmd())
	}
}
