package herdr

import (
	"context"
	"errors"
	"image"
	"image/color"
	"net"
	"strings"
	"testing"
	"time"
)

// Honest degradation of a capability that depends on the ENVIRONMENT.
func TestNoKnownPaneInfoOrClearTouchTheSocket(t *testing.T) {
	g, srv := newServer(t, func(c net.Conn, _ map[string]any) {
		t.Error("the socket was called with no known pane")
		_ = c.Close()
	})
	// The empty pane is what fires the guard: an empty PaneID makes pane() return early.
	g.PaneID = ""
	t.Setenv("HERDR_PANE_ID", "")

	info, err := g.Info(context.Background())
	if !errors.Is(err, ErrNoGraphics) {
		t.Errorf("Info without a pane gave %v, want ErrNoGraphics: the TUI would not know it has to "+
			"paint half-blocks", err)
	}
	if info.CellWidthPx != 0 || info.CellHeightPx != 0 {
		t.Errorf("Info without a pane returned %+v: an invented cell size scales the "+
			"image to a resolution that does not fit, and it is the ratio of those two "+
			"dimensions that decides the aspect", info)
	}
	// MaxLayers at zero would make the TUI think there are no layers.
	if info.PaneVisible || info.MaxLayers != 0 {
		t.Errorf("Info without a pane returned %+v: the capability fields have to come back zero", info)
	}

	if err := g.Clear(context.Background(), "prdash-sim"); !errors.Is(err, ErrNoGraphics) {
		t.Errorf("Clear without a pane gave %v, want ErrNoGraphics", err)
	}
	err = g.SetImage(context.Background(), "prdash-sim", imageRGBA(2, 2), Placement{Col: 1, Row: 1})
	if !errors.Is(err, ErrNoGraphics) {
		t.Errorf("SetImage without a pane gave %v, want ErrNoGraphics", err)
	}

	srv.stop()
}

func TestSocketDyingWithRSTOnWriteIsReportedAsLayerFailureNotMissingPane(t *testing.T) {
	g := graphicsWithBrokenWrite(t)

	err := g.SetImage(context.Background(), "prdash-sim", imageRGBA(4, 4), Placement{Col: 1, Row: 1, Cols: 4, Rows: 2})
	if err == nil {
		t.Fatal("a dead connection gave nil: the popup would open with a frame and a hole")
	}
	if !errors.Is(err, ErrNoGraphics) {
		t.Errorf("the error %v is not ErrNoGraphics: the TUI would not degrade to half-blocks", err)
	}
	if !strings.Contains(err.Error(), "graphics") {
		t.Errorf("the reason %q does not say the graphics layer failed", err)
	}
	// The write failure's reason is in the message, which tells this apart from the others.
	if !strings.Contains(err.Error(), "broken pipe") {
		t.Errorf("the reason %q does not carry the cause of the failed write", err)
	}
}

// The params of the three operations is a map[string]any, so an unserialisable value can reach it.
func TestUnserializableParamFailsBeforeTouchingTheSocket(t *testing.T) {
	g, srv := newServer(t, func(c net.Conn, _ map[string]any) {
		t.Error("the socket was connected with a request that cannot be serialized")
		_ = c.Close()
	})

	var out any
	err := g.call(context.Background(), "pane.graphics.set", map[string]any{
		// A channel cannot be serialised, and no real operation passes one.
		"image": make(chan int),
	}, &out)

	if err == nil {
		t.Fatal("a parameter that cannot be serialized gave nil")
	}
	if errors.Is(err, ErrNoGraphics) {
		t.Errorf("the error %v comes wrapped in ErrNoGraphics: it is not a layer failure, it is "+
			"a request failure, and wrapping it would make the TUI treat it as «no pane» "+
			"and paint half-blocks", err)
	}
	if !strings.Contains(err.Error(), "chan") && !strings.Contains(err.Error(), "json") &&
		!strings.Contains(err.Error(), "unsupported") {
		t.Errorf("the error %q does not say the parameter cannot be serialized", err)
	}
	// The call has to fail BEFORE connecting: a server that answered would prove nothing.
	srv.mu.Lock()
	seen := len(srv.requests)
	srv.mu.Unlock()
	if seen != 0 {
		t.Errorf("the server received %d requests from a call that never got to write",
			seen)
	}
	srv.stop()
}

func TestEmptySocketComesFromTheEnvironmentAndIsNotGuessed(t *testing.T) {
	t.Setenv("HERDR_SOCKET_PATH", "")
	t.Setenv("HERDR_PANE_ID", "pane-1")

	g := &Graphics{getenv: func(k string) string {
		if k == "HERDR_PANE_ID" {
			return "pane-1"
		}
		return ""
	}}
	_, err := g.Info(context.Background())
	if err == nil {
		t.Fatal("without a socket it gave nil")
	}
	if !errors.Is(err, ErrNoGraphics) {
		t.Errorf("the error %v is not ErrNoGraphics", err)
	}
	// The message does NOT name the variable on purpose: socket() returns "" and Dial fails unhelpfully.
	if !strings.Contains(err.Error(), "graphics") {
		t.Errorf("the error %q does not say the graphics layer failed", err)
	}
	if !strings.Contains(err.Error(), "HERDR_SOCKET_PATH") {
		t.Logf("GAP: the reason %q does not name HERDR_SOCKET_PATH, which is what "+
			"would allow fixing it without reading the code", err)
	}
}

// The embedded net.Conn is NOT nil: call's deferred Close panics on a nil interface.
type connWithBrokenWrite struct{ net.Conn }

func (connWithBrokenWrite) Write([]byte) (int, error) {
	return 0, errors.New("broken pipe")
}

func graphicsWithBrokenWrite(t *testing.T) *Graphics {
	t.Helper()
	client, server := net.Pipe()
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
	})
	broken := connWithBrokenWrite{Conn: client}
	return &Graphics{
		// Socket is pinned: without it call() returns a bare ErrNoGraphics before dialing, so the broken write is never reached.
		Socket:  "/tmp/herdr.sock",
		PaneID:  "pane-1",
		Timeout: 2 * time.Second,
		dial: func(context.Context, string, string) (net.Conn, error) {
			return broken, nil
		},
	}
}

func imageRGBA(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 64, A: 255})
		}
	}
	return img
}
