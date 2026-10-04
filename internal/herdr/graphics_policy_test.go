package herdr

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

// A zero in EITHER dimension draws nothing.
func TestPlacementEmptyConUnaDimensionEnCero(t *testing.T) {
	casos := []struct {
		nombre string
		p      Placement
		vacia  bool
	}{
		{"todo a 0", Placement{}, true},
		{"solo columnas a 0", Placement{Cols: 0, Rows: 5}, true},
		{"solo filas a 0", Placement{Cols: 5, Rows: 0}, true},
		{"columnas a 0 y filas a 0", Placement{Cols: 0, Rows: 0}, true},
		{"columnas negativas", Placement{Cols: -1, Rows: 5}, true},
		{"filas negativas", Placement{Cols: 5, Rows: -1}, true},
		{"las dos negativas", Placement{Cols: -3, Rows: -3}, true},
		{"una de celda", Placement{Cols: 1, Rows: 1}, false},
		{"con contenido", Placement{Cols: 20, Rows: 10}, false},
		// The offset does not count: an image in the corner is still an image.
		{"con desplazamiento", Placement{Col: 100, Row: 200, Cols: 1, Rows: 1}, false},
	}

	for _, c := range casos {
		if got := c.p.Empty(); got != c.vacia {
			t.Errorf("%s: %+v dio Empty()=%v, want %v", c.nombre, c.p, got, c.vacia)
		}
		if !c.vacia {
			continue
		}
		desplazada := c.p
		desplazada.Col, desplazada.Row = 999, 999
		if !desplazada.Empty() {
			t.Errorf("%s: desplazada a (999,999) dejó de estar vacía", c.nombre)
		}
	}
}

// All nine combinations are asserted.
func TestGraphicsReadySonTresCadenasYUnaDecision(t *testing.T) {
	casos := []struct {
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

	for _, c := range casos {
		if got := graphicsReady(c.herdrEnv, c.socket, c.pane); got != c.want {
			t.Errorf("graphicsReady(%q, %q, %q) = %v, want %v", c.herdrEnv, c.socket, c.pane, got, c.want)
		}
	}
	// HERDR_ENV has to be EXACTLY "1": that is what Herdr itself writes when it starts.
	for _, ok := range []string{"1"} {
		if !graphicsReady(ok, "s", "p") {
			t.Errorf("graphicsReady con HERDR_ENV=%q dio false, y es el valor que escribe Herdr", ok)
		}
	}
}

// The ORDER of the conditions is not a detail.
func TestGraphicsReadyNoMiraElSocketSiNoEstaDentro(t *testing.T) {
	// There is no way to inject a socket that breaks, because the function takes strings. What is
	//asserted is the consequence.
	for _, herdrEnv := range []string{"", "0", "2", "true", "1x"} {
		conSocketImposible := graphicsReady(herdrEnv, "/no/existe/el/socket", "w1:p1")
		conSocketReal := graphicsReady(herdrEnv, "/run/herdr.sock", "w1:p1")
		if conSocketImposible != conSocketReal {
			t.Errorf("HERDR_ENV=%q: el resultado cambió según el socket (%v vs %v): "+
				"la condición se está mirando aunque no deba", herdrEnv, conSocketImposible, conSocketReal)
		}
		if conSocketReal {
			t.Errorf("HERDR_ENV=%q: dio true sin estar dentro de Herdr", herdrEnv)
		}
	}
}

func TestHaveGraphicsTarget(t *testing.T) {
	if !haveGraphicsTarget("/s", "p") {
		t.Error("con socket y pane dio false")
	}
	if haveGraphicsTarget("", "p") {
		t.Error("sin socket dio true: la petición sale sin a quién preguntarlo")
	}
	if haveGraphicsTarget("/s", "") {
		t.Error("sin pane dio true: no hay rectángulo donde colocar la imagen")
	}
	if haveGraphicsTarget("", "") {
		t.Error("sin nada dio true")
	}
}

// A zero deadline is a deadline that already passed. With a real one the request is cut before
// being sent.
func TestGraphicsTimeoutParaCeroYNegativoNoEsCero(t *testing.T) {
	for _, t0 := range []time.Duration{-time.Hour, -time.Second, -time.Nanosecond, 0} {
		if got := graphicsTimeoutFor(t0); got != graphicsTimeout {
			t.Errorf("graphicsTimeoutFor(%v) = %v, want %v: un plazo no positivo no es un plazo",
				t0, got, graphicsTimeout)
		}
		if got := graphicsTimeoutFor(t0); got <= 0 {
			t.Errorf("graphicsTimeoutFor(%v) devolvió un plazo no positivo: %v", t0, got)
		}
	}
	// A positive deadline is honoured as given, neither clipped nor rounded.
	for _, t0 := range []time.Duration{time.Nanosecond, time.Millisecond, 42 * time.Second, time.Hour} {
		if got := graphicsTimeoutFor(t0); got != t0 {
			t.Errorf("graphicsTimeoutFor(%v) = %v, want el mismo plazo", t0, got)
		}
	}
	// The default is a real deadline, not zero.
	if graphicsTimeout <= 0 {
		t.Errorf("graphicsTimeout = %v: un plazo por defecto no positivo hace que la degradación no degrade", graphicsTimeout)
	}
}

// There are THREE ways not to be able to ask, and the third is the one that gets confused.
func TestCellSizeFromDegradaEnLasTresFormas(t *testing.T) {
	const w, h = defaultCellWidthPx, defaultCellHeightPx

	casos := []struct {
		nombre string
		info   GraphicsInfo
		err    error
		wantW  int
		wantH  int
	}{
		{"medida normal", GraphicsInfo{CellWidthPx: 9, CellHeightPx: 19}, nil, 9, 19},
		{"medida 1×1", GraphicsInfo{CellWidthPx: 1, CellHeightPx: 1}, nil, 1, 1},
		{"medida grande", GraphicsInfo{CellWidthPx: 400, CellHeightPx: 800}, nil, 400, 800},
		{"error", GraphicsInfo{CellWidthPx: 9, CellHeightPx: 19}, errors.New("socket"), w, h},
		{"error y medida a cero", GraphicsInfo{}, errors.New("socket"), w, h},
		{"ancho a cero", GraphicsInfo{CellWidthPx: 0, CellHeightPx: 19}, nil, w, h},
		{"alto a cero", GraphicsInfo{CellWidthPx: 9, CellHeightPx: 0}, nil, w, h},
		{"las dos a cero", GraphicsInfo{CellWidthPx: 0, CellHeightPx: 0}, nil, w, h},
		{"ancho negativo", GraphicsInfo{CellWidthPx: -9, CellHeightPx: 19}, nil, w, h},
		{"alto negativo", GraphicsInfo{CellWidthPx: 9, CellHeightPx: -19}, nil, w, h},
		{"las dos negativas", GraphicsInfo{CellWidthPx: -1, CellHeightPx: -1}, nil, w, h},
	}

	for _, c := range casos {
		gotW, gotH := cellSizeFrom(c.info, c.err)
		if gotW != c.wantW || gotH != c.wantH {
			t.Errorf("%s: dio %dx%d, want %dx%d", c.nombre, gotW, gotH, c.wantW, c.wantH)
		}
	}

	// The degradation is ALWAYS positive, which is the reason it exists: returning zero would be
	// read as "no image".
	for _, c := range casos {
		gotW, gotH := cellSizeFrom(c.info, c.err)
		if gotW <= 0 || gotH <= 0 {
			t.Errorf("%s: devolvió %dx%d, y una celda de cero píxeles no es una celda", c.nombre, gotW, gotH)
		}
	}
	// The default approximation is a terminal's, 1x2. Not arbitrary: it is the ratio every terminal
	// has.
	if defaultCellWidthPx != 1 || defaultCellHeightPx != 2 {
		t.Errorf("la aproximación por defecto es %dx%d, want 1x2 (lo que mide una celda de texto)",
			defaultCellWidthPx, defaultCellHeightPx)
	}
}

func TestCallNoSaleSinDestinoNiConPlazoVencido(t *testing.T) {
	for _, c := range []struct {
		nombre     string
		socket     string
		pane       string
		quiereDial bool
	}{
		{"con los dos", "/tmp/s.sock", "w1:p1", true},
		{"sin socket", "", "w1:p1", false},
		{"sin pane", "/tmp/s.sock", "", false},
		{"sin nada", "", "", false},
	} {
		var dials int
		g := &Graphics{
			Socket:  c.socket,
			PaneID:  c.pane,
			Timeout: time.Second,
			dial: func(context.Context, string, string) (net.Conn, error) {
				dials++
				return nil, errors.New("no debería llegar aquí")
			},
		}
		g.getenv = func(string) string { return "" }

		err := g.call(context.Background(), "pane.graphics.info", nil, nil)
		if !errors.Is(err, ErrNoGraphics) {
			t.Errorf("%s: dio %v, want ErrNoGraphics", c.nombre, err)
		}
		if c.quiereDial != (dials > 0) {
			t.Errorf("%s: dial se llamó %d veces, y %v. Sin destino no se abre conexión: "+
				"un socket a medias convierte un error local en uno remoto", c.nombre, dials, c.quiereDial)
		}
	}
}
