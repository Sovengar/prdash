package tui

import (
	"context"
	"testing"
	"time"
)

// The event bomb is an invariant the model assumes: a leak here is a goroutine per keypress.

type markedEvent struct{ n int }

func TestTheTimerReadsOneEventAndOnlyOne(t *testing.T) {
	ch := make(chan event, 3)
	ch <- markedEvent{n: 1}
	ch <- markedEvent{n: 2}

	cmd := waitForEvent(ch)

	ev, ok := cmd().(markedEvent)
	if !ok {
		t.Fatal("the Cmd did not return an event")
	}
	if ev.n != 1 {
		t.Errorf("the event read is %+v, want the first one", ev)
	}
	if n := len(ch); n != 1 {
		t.Errorf("%d events stay in the channel after one Cmd, want 1: it took more than one", n)
	}

	ev2 := cmd().(markedEvent)
	if ev2.n != 2 {
		t.Errorf("the second event is %+v, want the second one", ev2)
	}
}

// This is what prevents the hang.
func TestTheTimerStopsWhenTheChannelCloses(t *testing.T) {
	ch := make(chan event)
	close(ch)

	got := waitForEvent(ch)()
	if got != nil {
		t.Errorf("with the channel closed the Cmd returned %v (%T), want nil: nil is what "+
			"tells bubbletea there are no more messages", got, got)
	}
	for i := range 3 {
		if got := waitForEvent(ch)(); got != nil {
			t.Errorf("call %d with the channel closed returned %v", i, got)
			break
		}
	}
}

func TestPublishRespectsTheCancellation(t *testing.T) {
	ch := make(chan event, 1)
	sendEvent(context.Background(), ch, markedEvent{n: 1})

	select {
	case got := <-ch:
		if m, ok := got.(markedEvent); !ok || m.n != 1 {
			t.Errorf("1 was published and %v arrived", got)
		}
	case <-time.After(time.Second):
		t.Fatal("the event never arrived")
	}

	// With the context already cancelled and nobody reading, it does not block.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	unread := make(chan event) // no buffer and no reader: publishing here would block
	doneF := make(chan struct{})
	go func() {
		defer close(doneF)
		sendEvent(ctx, unread, markedEvent{n: 2})
	}()
	select {
	case <-doneF:
	case <-time.After(2 * time.Second):
		t.Fatal("sendEvent blocked with the context cancelled: it stays in a goroutine " +
			"that nobody will ever finish")
	}

	// Publishing BLOCKS on purpose when the context is alive and there is no reader.
}

// The guard is the only thing between a double release and a counter that lies; this forces it.
func TestReleasingMoreReadersThanAreArmedDoesNotGoNegative(t *testing.T) {
	m := newTestModel(t)
	m.readers = 0

	m.releaseReader()
	if m.readers != 0 {
		t.Errorf("releasing with none armed left the counter at %d, want 0: it must not go negative", m.readers)
	}
	m.releaseReader()
	if m.readers != 0 {
		t.Errorf("after releasing twice with none armed the counter is %d, want 0", m.readers)
	}

	m.armReader()
	m.releaseReader()
	if m.readers != 0 {
		t.Errorf("one arm and one release left the counter at %d, want 0", m.readers)
	}
}

// The counter exists so the invariant can be asserted at all.
func TestTheReaderCounterIsWhatAllowsAssertingTheInvariant(t *testing.T) {
	m := newTestModel(t)
	m.events = make(chan event, 1)
	m.readers = 0

	cmd := m.armReader()
	if m.readers != 1 {
		t.Errorf("armReader left the counter at %d, want 1", m.readers)
	}
	if cmd == nil {
		t.Fatal("armReader returned a nil Cmd")
	}

	// Arming twice is two readers, which is exactly what the invariant forbids.
	m.armReader()
	if m.readers != 2 {
		t.Errorf("after arming twice the counter stayed at %d", m.readers)
	}

	m.events <- markedEvent{n: 9}
	got, ok := cmd().(markedEvent)
	if !ok || got.n != 9 {
		t.Errorf("the armReader Cmd did not read from the models channel: %v", cmd())
	}
}
