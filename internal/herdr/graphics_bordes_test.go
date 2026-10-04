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
func TestSinPaneConocidoNiInfoNiClearTocanElSocket(t *testing.T) {
	g, srv := nuevoServidor(t, func(c net.Conn, _ map[string]any) {
		t.Error("se llamó al socket sin pane conocido")
		_ = c.Close()
	})
	// The empty pane is what fires the guard: an empty PaneID makes pane() return early.
	g.PaneID = ""
	t.Setenv("HERDR_PANE_ID", "")

	info, err := g.Info(context.Background())
	if !errors.Is(err, ErrNoGraphics) {
		t.Errorf("Info sin pane dio %v, want ErrNoGraphics: la TUI no sabría que tiene que "+
			"pintar half-blocks", err)
	}
	if info.CellWidthPx != 0 || info.CellHeightPx != 0 {
		t.Errorf("Info sin pane devolvió %+v: un tamaño de celda inventado escala la "+
			"imagen a una resolución que no cabe, y es la relación de esas dos dimensiones "+
			"la que decide el aspecto", info)
	}
	// MaxLayers at zero would make the TUI think there are no layers.
	if info.PaneVisible || info.MaxLayers != 0 {
		t.Errorf("Info sin pane devolvió %+v: los campos de capacidad tienen que venir a cero", info)
	}

	if err := g.Clear(context.Background(), "prdash-sim"); !errors.Is(err, ErrNoGraphics) {
		t.Errorf("Clear sin pane dio %v, want ErrNoGraphics", err)
	}
	err = g.SetImage(context.Background(), "prdash-sim", imagenRGBA(2, 2), Placement{Col: 1, Row: 1})
	if !errors.Is(err, ErrNoGraphics) {
		t.Errorf("SetImage sin pane dio %v, want ErrNoGraphics", err)
	}

	srv.parar()
}

// Real: Herdr restarts while the image is being written, and the socket answers RST.
func TestUnSocketQueMuerreConRSTAlEscribirSeReportaComoFalloDeLaCapaYNoComoUnPanePerdido(t *testing.T) {
	// La escritura falla siempre.
	g := graphicsConElWriteRoto(t)

	err := g.SetImage(context.Background(), "prdash-sim", imagenRGBA(4, 4), Placement{Col: 1, Row: 1, Cols: 4, Rows: 2})
	if err == nil {
		t.Fatal("una conexión muerta dio nil: el popup se abriría con marco y hueco")
	}
	if !errors.Is(err, ErrNoGraphics) {
		t.Errorf("el error %v no es ErrNoGraphics: la TUI no degradaría a half-blocks", err)
	}
	if !strings.Contains(err.Error(), "graphics") {
		t.Errorf("el motivo %q no dice que falla la capa de gráficos", err)
	}
	// The reason for the WRITE failure is in the message, which is what tells this apart from the
	// others.
	if !strings.Contains(err.Error(), "broken pipe") {
		t.Errorf("el motivo %q no trae la causa de la escritura fallida", err)
	}
}

// The params of the three operations is a map[string]any, so an unserialisable value can reach it.
func TestUnParametroQueNoSePuedeSerializarFallaAntesDeTocarElSocket(t *testing.T) {
	g, srv := nuevoServidor(t, func(c net.Conn, _ map[string]any) {
		t.Error("se conectó al socket con una petición que no se puede serializar")
		_ = c.Close()
	})

	var salida any
	err := g.call(context.Background(), "pane.graphics.set", map[string]any{
		// A channel cannot be serialised, and no real operation passes one.
		"imagen": make(chan int),
	}, &salida)

	if err == nil {
		t.Fatal("un parámetro que no se puede serializar dio nil")
	}
	if errors.Is(err, ErrNoGraphics) {
		t.Errorf("el error %v viene envuelto en ErrNoGraphics: no es un fallo de la capa, es "+
			"un fallo de la petición, y envolverlo haría que la TUI lo trata como «no hay "+
			"pane» y pintara half-blocks", err)
	}
	if !strings.Contains(err.Error(), "chan") && !strings.Contains(err.Error(), "json") &&
		!strings.Contains(err.Error(), "unsupported") {
		t.Errorf("el error %q no dice que el parámetro no se puede serializar", err)
	}
	// The server saw nothing: the call has to fail BEFORE connecting, because a server that
	// answered would prove nothing.
	srv.mu.Lock()
	vistas := len(srv.peticiones)
	srv.mu.Unlock()
	if vistas != 0 {
		t.Errorf("el servidor recibió %d peticiones de una llamada que no llegó a escribir",
			vistas)
	}
	srv.parar()
}

// The difference between "no Herdr" and "Herdr is somewhere else".
func TestElSocketVacioVieneDelEntornoYNoSeAdivina(t *testing.T) {
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
		t.Fatal("sin socket dio nil")
	}
	if !errors.Is(err, ErrNoGraphics) {
		t.Errorf("el error %v no es ErrNoGraphics", err)
	}
	// The message does NOT name the environment variable, and that gap is written down on purpose:
	//socket() returns "" and the Dial fails with an unhelpful message.
	if !strings.Contains(err.Error(), "graphics") {
		t.Errorf("el error %q no dice que falla la capa de gráficos", err)
	}
	if !strings.Contains(err.Error(), "HERDR_SOCKET_PATH") {
		t.Logf("LAGUNA: el motivo %q no nombra HERDR_SOCKET_PATH, que es lo que "+
			"permitiría arreglarlo sin leer el código", err)
	}
}

// The embedded net.Conn is NOT nil: call does a deferred Close and a nil embedded interface
// panics there.
type connConElWriteRoto struct{ net.Conn }

func (connConElWriteRoto) Write([]byte) (int, error) {
	return 0, errors.New("broken pipe")
}

// graphicsConElWriteRoto devuelve un `Graphics` cuya escritura falla siempre.
func graphicsConElWriteRoto(t *testing.T) *Graphics {
	t.Helper()
	cliente, servidor := net.Pipe()
	t.Cleanup(func() {
		_ = cliente.Close()
		_ = servidor.Close()
	})
	roto := connConElWriteRoto{Conn: cliente}
	return &Graphics{
		PaneID:  "pane-1",
		Timeout: 2 * time.Second,
		dial: func(context.Context, string, string) (net.Conn, error) {
			return roto, nil
		},
	}
}

func imagenRGBA(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 64, A: 255})
		}
	}
	return img
}
