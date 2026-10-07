package herdr

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCallWithoutResponseIsAGraphicsError(t *testing.T) {
	silent := &fakeSocket{silent: true}
	g := newTestGraphics(t, silent)
	err := g.call(context.Background(), "pane.graphics.info", nil, nil)
	if !errors.Is(err, ErrNoGraphics) {
		t.Errorf("with the socket silently closed it gave %v, want ErrNoGraphics", err)
	}

	// Half a reply is NOT ErrNoGraphics: something arrived, so the protocol is what broke.
	half := &fakeSocket{halfLine: true, response: `{"id":"x","result":{"cell_width_px":9}}`}
	g = newTestGraphics(t, half)
	err = g.call(context.Background(), "pane.graphics.info", nil, nil)
	if err == nil {
		t.Error("with half a response it gave nil, want an error: a cut-off response is not a response")
	}
	if errors.Is(err, ErrNoGraphics) {
		t.Errorf("with half a response it gave ErrNoGraphics: something arrived, so the failure is in the protocol (%v)", err)
	}

	full := &fakeSocket{response: graphicsInfoJSON(9, 19, true)}
	g = newTestGraphics(t, full)
	info, err := g.Info(context.Background())
	if err != nil {
		t.Fatalf("with a complete response it gave %v, want nil", err)
	}
	if info.CellWidthPx != 9 || info.CellHeightPx != 19 {
		t.Errorf("decoded %dx%d, want 9x19", info.CellWidthPx, info.CellHeightPx)
	}
}

// The deadline travels in the context passed to dial, so it can be checked without waiting.
func TestCallUsesTheRequestedDeadlineAndTheDefaultOne(t *testing.T) {
	for _, c := range []struct {
		name    string
		timeout time.Duration
	}{
		{"the one passed in", 3 * time.Second},
		{"zero", 0},
		{"negative", -time.Second},
	} {
		f := &fakeSocket{response: graphicsInfoJSON(9, 19, true)}
		g := newTestGraphics(t, f)
		g.Timeout = c.timeout

		before := time.Now()
		if err := g.call(context.Background(), "pane.graphics.info", nil, nil); err != nil {
			t.Fatalf("%s: it gave %v", c.name, err)
		}
		if f.deadline.IsZero() {
			t.Errorf("%s: the connection was opened without a deadline: it would lose the first block", c.name)
			continue
		}
		// The deadline has to be in the future, or the request is cut before being sent.
		if !f.deadline.After(before) {
			t.Errorf("%s: the deadline was already past (%v): the request is cut before being sent",
				c.name, f.deadline.Sub(before))
		}
		if c.timeout > 0 {
			if remaining := f.deadline.Sub(before); remaining > c.timeout+time.Second {
				t.Errorf("%s: the deadline is %v, want about %v", c.name, remaining, c.timeout)
			}
		}
	}
	f1, f2 := &fakeSocket{response: graphicsInfoJSON(9, 19, true)}, &fakeSocket{response: graphicsInfoJSON(9, 19, true)}
	g1, g2 := newTestGraphics(t, f1), newTestGraphics(t, f2)
	g1.Timeout, g2.Timeout = 0, -time.Hour
	for i, g := range []*Graphics{g1, g2} {
		if err := g.call(context.Background(), "pane.graphics.info", nil, nil); err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
	}
	if f1.deadline.IsZero() || f2.deadline.IsZero() {
		t.Fatal("some request went out without a deadline")
	}
	if diff := f1.deadline.Sub(f2.deadline); diff > time.Second || diff < -time.Second {
		t.Errorf("the default deadlines differ by %v: they should be the same", diff)
	}
	if graphicsTimeout <= 0 {
		t.Error("the default deadline is not a deadline")
	}
}

func TestCallOpensOneConnectionPerRequest(t *testing.T) {
	f := &fakeSocket{response: graphicsInfoJSON(9, 19, true)}
	g := newTestGraphics(t, f)

	for i := range 3 {
		if err := g.call(context.Background(), "pane.graphics.info", nil, nil); err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
	}
	if f.dials != 3 {
		t.Errorf("three requests opened %d connections, want 3: the server closes after each response", f.dials)
	}
	if f.served != 3 {
		t.Errorf("the server served %d requests, want 3", f.served)
	}
}
