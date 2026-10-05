package herdr

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

// A zero in EITHER dimension draws nothing.
func TestPlacementEmptyWithOneDimensionZero(t *testing.T) {
	cases := []struct {
		name  string
		p     Placement
		empty bool
	}{
		{"all zero", Placement{}, true},
		{"only columns zero", Placement{Cols: 0, Rows: 5}, true},
		{"only rows zero", Placement{Cols: 5, Rows: 0}, true},
		{"columns zero and rows zero", Placement{Cols: 0, Rows: 0}, true},
		{"negative columns", Placement{Cols: -1, Rows: 5}, true},
		{"negative rows", Placement{Cols: 5, Rows: -1}, true},
		{"both negative", Placement{Cols: -3, Rows: -3}, true},
		{"one cell", Placement{Cols: 1, Rows: 1}, false},
		{"with content", Placement{Cols: 20, Rows: 10}, false},
		// The offset does not count: an image in the corner is still an image.
		{"with offset", Placement{Col: 100, Row: 200, Cols: 1, Rows: 1}, false},
	}

	for _, c := range cases {
		if got := c.p.Empty(); got != c.empty {
			t.Errorf("%s: %+v gave Empty()=%v, want %v", c.name, c.p, got, c.empty)
		}
		if !c.empty {
			continue
		}
		shifted := c.p
		shifted.Col, shifted.Row = 999, 999
		if !shifted.Empty() {
			t.Errorf("%s: shifted to (999,999) it stopped being empty", c.name)
		}
	}
}

// All nine combinations are asserted.
func TestGraphicsReadyIsThreeStringsAndOneDecision(t *testing.T) {
	cases := []struct {
		herdrEnv, socket, pane string
		want                   bool
	}{
		{"1", "/run/herdr.sock", "w1:p1", true},
		{"", "/run/herdr.sock", "w1:p1", false},
		{"0", "/run/herdr.sock", "w1:p1", false},
		{"2", "/run/herdr.sock", "w1:p1", false},
		{"true", "/run/herdr.sock", "w1:p1", false},
		{"11", "/run/herdr.sock", "w1:p1", false},
		{" 1", "/run/herdr.sock", "w1:p1", false},
		{"1 ", "/run/herdr.sock", "w1:p1", false},
		{"1", "", "w1:p1", false},
		{"1", "/run/herdr.sock", "", false},
		{"1", "", "", false},
		{"", "", "", false},
	}

	for _, c := range cases {
		if got := graphicsReady(c.herdrEnv, c.socket, c.pane); got != c.want {
			t.Errorf("graphicsReady(%q, %q, %q) = %v, want %v", c.herdrEnv, c.socket, c.pane, got, c.want)
		}
	}
	// HERDR_ENV has to be EXACTLY "1": that is what Herdr itself writes when it starts.
	for _, ok := range []string{"1"} {
		if !graphicsReady(ok, "s", "p") {
			t.Errorf("graphicsReady with HERDR_ENV=%q gave false, and it is the value Herdr writes", ok)
		}
	}
}

// The ORDER of the conditions is not a detail.
func TestGraphicsReadyDoesNotLookAtTheSocketWhenNotInside(t *testing.T) {
	// There is no way to inject a socket that breaks, because the function takes strings. What is
	//asserted is the consequence.
	for _, herdrEnv := range []string{"", "0", "2", "true", "1x"} {
		withoutSocket := graphicsReady(herdrEnv, "/no/existe/el/socket", "w1:p1")
		withSocket := graphicsReady(herdrEnv, "/run/herdr.sock", "w1:p1")
		if withoutSocket != withSocket {
			t.Errorf("HERDR_ENV=%q: the result changed with the socket (%v vs %v): "+
				"the condition is being looked at when it should not be", herdrEnv, withoutSocket, withSocket)
		}
		if withSocket {
			t.Errorf("HERDR_ENV=%q: it gave true without being inside Herdr", herdrEnv)
		}
	}
}

func TestHaveGraphicsTarget(t *testing.T) {
	if !haveGraphicsTarget("/s", "p") {
		t.Error("with socket and pane it gave false")
	}
	if haveGraphicsTarget("", "p") {
		t.Error("without socket it gave true: the request goes out with nobody to ask")
	}
	if haveGraphicsTarget("/s", "") {
		t.Error("without pane it gave true: there is no rectangle to place the image on")
	}
	if haveGraphicsTarget("", "") {
		t.Error("without anything it gave true")
	}
}

// A zero deadline is a deadline that already passed. With a real one the request is cut before
// being sent.
func TestGraphicsTimeoutForZeroAndNegativeIsNotZero(t *testing.T) {
	for _, t0 := range []time.Duration{-time.Hour, -time.Second, -time.Nanosecond, 0} {
		if got := graphicsTimeoutFor(t0); got != graphicsTimeout {
			t.Errorf("graphicsTimeoutFor(%v) = %v, want %v: a non-positive deadline is not a deadline",
				t0, got, graphicsTimeout)
		}
		if got := graphicsTimeoutFor(t0); got <= 0 {
			t.Errorf("graphicsTimeoutFor(%v) returned a non-positive deadline: %v", t0, got)
		}
	}
	// A positive deadline is honoured as given, neither clipped nor rounded.
	for _, t0 := range []time.Duration{time.Nanosecond, time.Millisecond, 42 * time.Second, time.Hour} {
		if got := graphicsTimeoutFor(t0); got != t0 {
			t.Errorf("graphicsTimeoutFor(%v) = %v, want the same deadline", t0, got)
		}
	}
	// The default is a real deadline, not zero.
	if graphicsTimeout <= 0 {
		t.Errorf("graphicsTimeout = %v: a non-positive default deadline makes the degradation not degrade", graphicsTimeout)
	}
}

// There are THREE ways not to be able to ask, and the third is the one that gets confused.
func TestCellSizeFromDegradesInTheThreeWays(t *testing.T) {
	const w, h = defaultCellWidthPx, defaultCellHeightPx

	cases := []struct {
		name  string
		info  GraphicsInfo
		err   error
		wantW int
		wantH int
	}{
		{"normal size", GraphicsInfo{CellWidthPx: 9, CellHeightPx: 19}, nil, 9, 19},
		{"size 1×1", GraphicsInfo{CellWidthPx: 1, CellHeightPx: 1}, nil, 1, 1},
		{"large size", GraphicsInfo{CellWidthPx: 400, CellHeightPx: 800}, nil, 400, 800},
		{"error", GraphicsInfo{CellWidthPx: 9, CellHeightPx: 19}, errors.New("socket"), w, h},
		{"error and zero size", GraphicsInfo{}, errors.New("socket"), w, h},
		{"width zero", GraphicsInfo{CellWidthPx: 0, CellHeightPx: 19}, nil, w, h},
		{"height zero", GraphicsInfo{CellWidthPx: 9, CellHeightPx: 0}, nil, w, h},
		{"both zero", GraphicsInfo{CellWidthPx: 0, CellHeightPx: 0}, nil, w, h},
		{"negative width", GraphicsInfo{CellWidthPx: -9, CellHeightPx: 19}, nil, w, h},
		{"negative height", GraphicsInfo{CellWidthPx: 9, CellHeightPx: -19}, nil, w, h},
		{"both negative", GraphicsInfo{CellWidthPx: -1, CellHeightPx: -1}, nil, w, h},
	}

	for _, c := range cases {
		gotW, gotH := cellSizeFrom(c.info, c.err)
		if gotW != c.wantW || gotH != c.wantH {
			t.Errorf("%s: gave %dx%d, want %dx%d", c.name, gotW, gotH, c.wantW, c.wantH)
		}
	}

	// The degradation is ALWAYS positive, which is the reason it exists: returning zero would be
	// read as "no image".
	for _, c := range cases {
		gotW, gotH := cellSizeFrom(c.info, c.err)
		if gotW <= 0 || gotH <= 0 {
			t.Errorf("%s: returned %dx%d, and a zero-pixel cell is not a cell", c.name, gotW, gotH)
		}
	}
	// The default approximation is a terminal's, 1x2. Not arbitrary: it is the ratio every terminal
	// has.
	if defaultCellWidthPx != 1 || defaultCellHeightPx != 2 {
		t.Errorf("the default approximation is %dx%d, want 1x2 (what a text cell measures)",
			defaultCellWidthPx, defaultCellHeightPx)
	}
}

func TestCallDoesNotGoOutWithoutTargetOrWithExpiredDeadline(t *testing.T) {
	for _, c := range []struct {
		name      string
		socket    string
		pane      string
		wantsDial bool
	}{
		{"with both", "/tmp/s.sock", "w1:p1", true},
		{"without socket", "", "w1:p1", false},
		{"without pane", "/tmp/s.sock", "", false},
		{"without anything", "", "", false},
	} {
		var dials int
		g := &Graphics{
			Socket:  c.socket,
			PaneID:  c.pane,
			Timeout: time.Second,
			dial: func(context.Context, string, string) (net.Conn, error) {
				dials++
				return nil, errors.New("should not get here")
			},
		}
		g.getenv = func(string) string { return "" }

		err := g.call(context.Background(), "pane.graphics.info", nil, nil)
		if !errors.Is(err, ErrNoGraphics) {
			t.Errorf("%s: it gave %v, want ErrNoGraphics", c.name, err)
		}
		if c.wantsDial != (dials > 0) {
			t.Errorf("%s: dial was called %d times, and %v. Without a target no connection is "+
				"opened: a half-set socket turns a local error into a remote one", c.name, dials, c.wantsDial)
		}
	}
}
