package tui

import (
	"context"
	"testing"
	"time"
)

// The event bomb is an invariant the rest of the model assumes, and not one test covers it: a
//leak here is a goroutine per keypress.

type eventoMarcado struct{ n int }

// What matters is the "and only one".
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
	// It does NOT take the second: it stays in the channel for the next Cmd.
	if n := len(ch); n != 1 {
		t.Errorf("quedan %d eventos en el canal tras un Cmd, want 1: se llevó más de uno", n)
	}

	ev2 := cmd().(eventoMarcado)
	if ev2.n != 2 {
		t.Errorf("el segundo evento es %+v, want el segundo", ev2)
	}
}

// This is what prevents the hang.
func TestLaBombaSeDetieneConElCanalCerrado(t *testing.T) {
	ch := make(chan event)
	close(ch)

	got := waitForEvent(ch)()
	if got != nil {
		t.Errorf("con el canal cerrado el Cmd devolvió %v (%T), want nil: nil es lo que le "+
			"dice a bubbletea que no hay más mensajes", got, got)
	}
	// Asking more times does not revive the channel or return something else.
	for i := range 3 {
		if got := waitForEvent(ch)(); got != nil {
			t.Errorf("la llamada %d con el canal cerrado devolvió %v", i, got)
			break
		}
	}
}

func TestPublicarRespetaLaCancelacion(t *testing.T) {
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

	// With the context ALREADY cancelled and nobody reading, it does not block. That is what proves
	// it.
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

	// Publishing BLOCKS on purpose when the context is alive and there is no reader.
}

// The counter exists so the invariant can be asserted at all.
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

	// Arming twice is two readers, which is exactly what the invariant forbids.
	m.armReader()
	if m.readers != 2 {
		t.Errorf("tras armar dos veces el contador quedó en %d", m.readers)
	}

	// The armed Cmd reads from the MODEL's channel, not another one.
	m.events <- eventoMarcado{n: 9}
	got, ok := cmd().(eventoMarcado)
	if !ok || got.n != 9 {
		t.Errorf("el Cmd de armReader no leyó del canal del modelo: %v", cmd())
	}
}
