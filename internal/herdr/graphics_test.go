package herdr

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"net"
	"strings"
	"testing"
	"time"
)

// fakeSocket sirve el protocolo de Herdr en memoria: acepta una conexión, lee una
// línea, responde y cierra. Es exactamente lo que hace el servidor real, y por eso
// el cliente abre una conexión por petición.
type fakeSocket struct {
	// response es lo que se contesta tras leer la petición.
	response string
	// lastRequest es la petición ya leída, para poder afirmar sobre ella.
	lastRequest map[string]any
	served      int
	// fail hace que la escritura falle, para probar el camino de error.
	fail bool
}

func (f *fakeSocket) dial(_ context.Context, _, _ string) (net.Conn, error) {
	if f.fail {
		return nil, errors.New("socket no disponible")
	}
	client, server := net.Pipe()
	go func() {
		defer func() { _ = server.Close() }()
		line, err := bufio.NewReader(server).ReadBytes('\n')
		if err != nil {
			return
		}
		var req struct {
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		_ = json.Unmarshal(line, &req)
		f.lastRequest = req.Params
		f.served++
		_, _ = server.Write([]byte(f.response + "\n"))
	}()
	return client, nil
}

func newTestGraphics(t *testing.T, f *fakeSocket) *Graphics {
	t.Helper()
	g := &Graphics{
		Socket:  "/tmp/herdr-test.sock",
		PaneID:  "w1:p1",
		Timeout: time.Second,
		dial:    f.dial,
	}
	g.getenv = func(string) string { return "1" }
	return g
}

func graphicsInfoJSON(cellW, cellH int, visible bool) string {
	return `{"id":"x","result":{"type":"pane_graphics_info","cell_width_px":` +
		itoa(cellW) + `,"cell_height_px":` + itoa(cellH) +
		`,"pane_visible":` + boolJSON(visible) +
		`,"max_layers_per_pane":16}}`
}

func itoa(v int) string {
	if v < 0 {
		return "-" + itoa(-v)
	}
	if v < 10 {
		return string(rune('0' + v))
	}
	return itoa(v/10) + string(rune('0'+v%10))
}

func boolJSON(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

// TestInfoLeeElTamanoDeCelda: el tamaño de celda es lo que permite no deformar la
// imagen, y en kitty no es 2:1 sino 9:19. Suponerlo introducía un error del 5% en
// el tamaño final.
func TestInfoLeeElTamanoDeCelda(t *testing.T) {
	f := &fakeSocket{response: graphicsInfoJSON(9, 19, true)}
	g := newTestGraphics(t, f)

	info, err := g.Info(context.Background())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.CellWidthPx != 9 || info.CellHeightPx != 19 {
		t.Errorf("celda = %dx%d, want 9x19", info.CellWidthPx, info.CellHeightPx)
	}
	if !info.PaneVisible {
		t.Error("PaneVisible = false con la respuesta que lo dice true")
	}
	if f.lastRequest["pane_id"] != "w1:p1" {
		t.Errorf("pane_id = %v, want w1:p1", f.lastRequest["pane_id"])
	}
}

// TestCellSizeUsaLaMedidaYCaenEnElHabitual: 9x19 es lo que mide el pane de verdad;
// si no se puede preguntar, 1x2 es lo habitual en un terminal y es mejor que no
// pintar imagen.
func TestCellSizeUsaLaMedidaYCaenEnElHabitual(t *testing.T) {
	g := newTestGraphics(t, &fakeSocket{response: graphicsInfoJSON(9, 19, true)})
	w, h := g.CellSize(context.Background())
	if w != 9 || h != 19 {
		t.Errorf("CellSize = %dx%d, want 9x19", w, h)
	}

	g = newTestGraphics(t, &fakeSocket{response: `{"error":{"code":"x","message":"y"}}`})
	w, h = g.CellSize(context.Background())
	if w != 1 || h != 2 {
		t.Errorf("CellSize sin servidor = %dx%d, want 1x2", w, h)
	}
}

// TestSetImageMandaLaColocacionEnCeldas: la colocación va en celdas del viewport,
// que es como Herdr la entiende. Si se mandara en píxeles, la imagen caería en otro
// sitio y solo se notaría porque no se ve.
func TestSetImageMandaLaColocacionEnCeldas(t *testing.T) {
	f := &fakeSocket{response: `{"id":"x","result":{"type":"ok"}}`}
	g := newTestGraphics(t, f)

	img := solidImage(4, 3)
	place := Placement{Col: 10, Row: 5, Cols: 40, Rows: 20}
	if err := g.SetImage(context.Background(), GraphicsLayer, img, place); err != nil {
		t.Fatalf("SetImage: %v", err)
	}

	if f.lastRequest["format"] != "png" {
		t.Errorf("format = %v, want png", f.lastRequest["format"])
	}
	if int(f.lastRequest["image_width"].(float64)) != 4 {
		t.Errorf("image_width = %v, want 4", f.lastRequest["image_width"])
	}
	if f.lastRequest["layer_id"] != GraphicsLayer {
		t.Errorf("layer_id = %v, want %q", f.lastRequest["layer_id"], GraphicsLayer)
	}
	placement, ok := f.lastRequest["placement"].(map[string]any)
	if !ok {
		t.Fatalf("placement = %v, want un objeto", f.lastRequest["placement"])
	}
	for key, want := range map[string]float64{
		"viewport_col": 10, "viewport_row": 5, "grid_cols": 40, "grid_rows": 20,
	} {
		if got := placement[key].(float64); got != want {
			t.Errorf("placement.%s = %v, want %v", key, got, want)
		}
	}
	if data, _ := f.lastRequest["data_base64"].(string); data == "" {
		t.Error("data_base64 vacío: la imagen no viajaría")
	}
}

// TestSetImageRechazaLoQueNoDibuja: una imagen nil o un rectángulo vacío no se
// envían. Mandar una capa sin nada que dibujar gasta una capa de las 16 del pane.
func TestSetImageRechazaLoQueNoDibuja(t *testing.T) {
	f := &fakeSocket{response: `{"id":"x","result":{"type":"ok"}}`}
	g := newTestGraphics(t, f)

	if err := g.SetImage(context.Background(), GraphicsLayer, nil, Placement{Cols: 4, Rows: 4}); !errors.Is(err, ErrNoGraphics) {
		t.Errorf("SetImage(nil) = %v, want ErrNoGraphics", err)
	}
	if err := g.SetImage(context.Background(), GraphicsLayer, solidImage(2, 2), Placement{}); !errors.Is(err, ErrNoGraphics) {
		t.Errorf("SetImage(rect vacío) = %v, want ErrNoGraphics", err)
	}
	if f.served != 0 {
		t.Errorf("se enviaron %d peticiones, want 0", f.served)
	}
}

// TestClearQuitaLaCapa: la capa vive por encima del contenido del pane, así que si
// no se quita al cerrar el popup, la imagen tapa la TUI.
func TestClearQuitaLaCapa(t *testing.T) {
	f := &fakeSocket{response: `{"id":"x","result":{"type":"ok"}}`}
	g := newTestGraphics(t, f)

	if err := g.Clear(context.Background(), GraphicsLayer); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if f.lastRequest["layer_id"] != GraphicsLayer {
		t.Errorf("layer_id = %v, want %q", f.lastRequest["layer_id"], GraphicsLayer)
	}
}

// TestElErrorDelServidorSePropaga: un rechazo de Herdr tiene que llegar como error
// con su código, no como un "algo falló" que no dice nada. Es lo que decide si el
// popup cae a half-blocks o se queda sin imagen.
func TestElErrorDelServidorSePropaga(t *testing.T) {
	f := &fakeSocket{response: `{"error":{"code":"pane_graphics_disabled","message":"pane graphics are disabled by terminal.kitty_graphics"}}`}
	g := newTestGraphics(t, f)

	_, err := g.Info(context.Background())
	if err == nil {
		t.Fatal("Info no falló")
	}
	if !strings.Contains(err.Error(), "kitty_graphics") {
		t.Errorf("err = %v, want el motivo del servidor", err)
	}
	var rerr *rpcError
	if !errors.As(err, &rerr) {
		t.Errorf("err = %T, want *rpcError", err)
	}
	if g.Available() {
		t.Error("Available = true con un servidor que rechaza el método")
	}
}

// TestAvailableSinHerdrEsFalse: fuera de Herdr no hay socket ni pane, y el popup tiene
// que caer a half-blocks sin preguntar a nadie.
func TestAvailableSinHerdrEsFalse(t *testing.T) {
	g := &Graphics{getenv: func(string) string { return "" }}
	if g.Available() {
		t.Error("Available = true sin HERDR_ENV")
	}
	g = &Graphics{getenv: func(key string) string {
		if key == "HERDR_ENV" {
			return "1"
		}
		return ""
	}}
	if g.Available() {
		t.Error("Available = true sin socket ni pane")
	}
}

// TestSocketCaidoNoRompe: si el socket no está, es el camino de los half-blocks, no
// un fallo que pare la TUI.
func TestSocketCaidoNoRompe(t *testing.T) {
	f := &fakeSocket{fail: true}
	g := newTestGraphics(t, f)

	if _, err := g.Info(context.Background()); !errors.Is(err, ErrNoGraphics) {
		t.Errorf("Info = %v, want ErrNoGraphics", err)
	}
	if err := g.SetImage(context.Background(), GraphicsLayer, solidImage(2, 2), Placement{Cols: 2, Rows: 2}); !errors.Is(err, ErrNoGraphics) {
		t.Errorf("SetImage = %v, want ErrNoGraphics", err)
	}
}

func solidImage(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 40, A: 255})
		}
	}
	return img
}
