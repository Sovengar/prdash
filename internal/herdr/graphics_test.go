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

type fakeSocket struct {
	response    string
	lastRequest map[string]any
	served      int
	fail        bool
	// silent closes the connection WITHOUT answering, like a server that dies midway; distinct from fail
	//(which does not even accept).
	silent   bool
	halfLine bool
	deadline time.Time
	dials    int
}

func (f *fakeSocket) dial(ctx context.Context, _, _ string) (net.Conn, error) {
	if f.fail {
		return nil, errors.New("socket not available")
	}
	f.dials++
	if dl, ok := ctx.Deadline(); ok {
		f.deadline = dl
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
		if f.silent {
			// Closes without answering: the client is left with an empty read and an error.
			return
		}
		if f.halfLine {
			// Writes half a response and closes: ReadBytes reads something but not a whole line.
			_, _ = server.Write([]byte(f.response[:len(f.response)/2]))
			return
		}
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

func TestInfoReadsTheCellSize(t *testing.T) {
	f := &fakeSocket{response: graphicsInfoJSON(9, 19, true)}
	g := newTestGraphics(t, f)

	info, err := g.Info(context.Background())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.CellWidthPx != 9 || info.CellHeightPx != 19 {
		t.Errorf("cell = %dx%d, want 9x19", info.CellWidthPx, info.CellHeightPx)
	}
	if !info.PaneVisible {
		t.Error("PaneVisible = false with a response that says true")
	}
	if f.lastRequest["pane_id"] != "w1:p1" {
		t.Errorf("pane_id = %v, want w1:p1", f.lastRequest["pane_id"])
	}
}

func TestCellSizeUsesTheMeasureAndFallsBackToTheUsualOne(t *testing.T) {
	g := newTestGraphics(t, &fakeSocket{response: graphicsInfoJSON(9, 19, true)})
	w, h := g.CellSize(context.Background())
	if w != 9 || h != 19 {
		t.Errorf("CellSize = %dx%d, want 9x19", w, h)
	}

	g = newTestGraphics(t, &fakeSocket{response: `{"error":{"code":"x","message":"y"}}`})
	w, h = g.CellSize(context.Background())
	if w != 1 || h != 2 {
		t.Errorf("CellSize without a server = %dx%d, want 1x2", w, h)
	}
}

func TestSetImageSendsThePlacementInCells(t *testing.T) {
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
		t.Fatalf("placement = %v, want an object", f.lastRequest["placement"])
	}
	for key, want := range map[string]float64{
		"viewport_col": 10, "viewport_row": 5, "grid_cols": 40, "grid_rows": 20,
	} {
		if got := placement[key].(float64); got != want {
			t.Errorf("placement.%s = %v, want %v", key, got, want)
		}
	}
	if data, _ := f.lastRequest["data_base64"].(string); data == "" {
		t.Error("data_base64 empty: the image would not travel")
	}
}

func TestSetImageRejectsWhatItCannotDraw(t *testing.T) {
	f := &fakeSocket{response: `{"id":"x","result":{"type":"ok"}}`}
	g := newTestGraphics(t, f)

	if err := g.SetImage(context.Background(), GraphicsLayer, nil, Placement{Cols: 4, Rows: 4}); !errors.Is(err, ErrNoGraphics) {
		t.Errorf("SetImage(nil) = %v, want ErrNoGraphics", err)
	}
	if err := g.SetImage(context.Background(), GraphicsLayer, solidImage(2, 2), Placement{}); !errors.Is(err, ErrNoGraphics) {
		t.Errorf("SetImage(empty rect) = %v, want ErrNoGraphics", err)
	}
	if f.served != 0 {
		t.Errorf("%d requests were sent, want 0", f.served)
	}
}

func TestClearRemovesTheLayer(t *testing.T) {
	f := &fakeSocket{response: `{"id":"x","result":{"type":"ok"}}`}
	g := newTestGraphics(t, f)

	if err := g.Clear(context.Background(), GraphicsLayer); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if f.lastRequest["layer_id"] != GraphicsLayer {
		t.Errorf("layer_id = %v, want %q", f.lastRequest["layer_id"], GraphicsLayer)
	}
}

func TestServerErrorPropagates(t *testing.T) {
	f := &fakeSocket{response: `{"error":{"code":"pane_graphics_disabled","message":"pane graphics are disabled by terminal.kitty_graphics"}}`}
	g := newTestGraphics(t, f)

	_, err := g.Info(context.Background())
	if err == nil {
		t.Fatal("Info did not fail")
	}
	if !strings.Contains(err.Error(), "kitty_graphics") {
		t.Errorf("err = %v, want the server's reason", err)
	}
	var rerr *rpcError
	if !errors.As(err, &rerr) {
		t.Errorf("err = %T, want *rpcError", err)
	}
	if g.Available() {
		t.Error("Available = true with a server that rejects the method")
	}
}

func TestAvailableWithoutHerdrIsFalse(t *testing.T) {
	g := &Graphics{getenv: func(string) string { return "" }}
	if g.Available() {
		t.Error("Available = true without HERDR_ENV")
	}
	g = &Graphics{getenv: func(key string) string {
		if key == "HERDR_ENV" {
			return "1"
		}
		return ""
	}}
	if g.Available() {
		t.Error("Available = true without socket or pane")
	}
}

func TestDownSocketDoesNotBreak(t *testing.T) {
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
