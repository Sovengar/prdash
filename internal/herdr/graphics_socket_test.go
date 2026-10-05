package herdr

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type rpcServer struct {
	t        *testing.T
	ln       net.Listener
	script   func(c net.Conn, request map[string]any)
	requests []map[string]any
	mu       sync.Mutex
	closed   bool
}

// The socket lives in t.TempDir() because Unix sockets are paths and a long one goes past
// sun_path's 108-byte limit.
func newServer(t *testing.T, script func(c net.Conn, request map[string]any)) (*Graphics, *rpcServer) {
	t.Helper()

	dir, err := os.MkdirTemp("", "herdr-sock")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "s")

	ln, err := net.Listen("unix", path)
	if err != nil {
		_ = os.RemoveAll(dir)
		t.Fatalf("listening on %s: %v", path, err)
	}

	s := &rpcServer{t: t, ln: ln, script: script}
	go s.serve()

	g := &Graphics{Socket: path, PaneID: "pane-1", Timeout: 2 * time.Second}
	t.Cleanup(func() {
		s.stop()
		_ = os.RemoveAll(dir)
	})
	return g, s
}

func (s *rpcServer) serve() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return // the listener was closed
		}
		go func() {
			defer func() { _ = conn.Close() }()
			line, err := bufio.NewReader(conn).ReadBytes('\n')
			if err != nil {
				return
			}
			var request map[string]any
			if err := json.Unmarshal(line, &request); err != nil {
				return
			}
			s.mu.Lock()
			s.requests = append(s.requests, request)
			s.mu.Unlock()
			if s.script != nil {
				s.script(conn, request)
			}
		}()
	}
}

func (s *rpcServer) stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	_ = s.ln.Close()
}

func (s *rpcServer) lastRequest(t *testing.T) map[string]any {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.requests) == 0 {
		t.Fatal("the server received no request")
	}
	return s.requests[len(s.requests)-1]
}

func respondWith(result any) func(net.Conn, map[string]any) {
	return func(c net.Conn, _ map[string]any) {
		_, _ = fmt.Fprintf(c, `{"jsonrpc":"2.0","id":"x","result":%s}`+"\n", mustJSON(result))
	}
}

func rpcErr(code, msg string) func(net.Conn, map[string]any) {
	return func(c net.Conn, _ map[string]any) {
		_, _ = fmt.Fprintf(c,
			`{"jsonrpc":"2.0","id":"x","error":{"code":%q,"message":%q}}`+"\n", code, msg)
	}
}

func dontAnswer(net.Conn, map[string]any) {}

func closeWithoutAnswering(c net.Conn, _ map[string]any) { _ = c.Close() }

func sendGarbage(c net.Conn, _ map[string]any) {
	_, _ = c.Write([]byte("esto no es JSON\n"))
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// What is checked is the message as it travels: the jsonrpc, the id, the method and the params with
// the pane_id.
func TestRealSocketOpensAndAnswers(t *testing.T) {
	g, s := newServer(t, respondWith(map[string]any{
		"cell_width_px": 11, "cell_height_px": 22,
		"pane_visible": true, "max_layers_per_pane": 4,
	}))

	info, err := g.Info(context.Background())
	if err != nil {
		t.Fatalf("Info against the real socket: %v", err)
	}
	if info.CellWidthPx != 11 || info.CellHeightPx != 22 {
		t.Errorf("the cell came out %dx%d, want 11x22", info.CellWidthPx, info.CellHeightPx)
	}
	if !info.PaneVisible || info.MaxLayers != 4 {
		t.Errorf("visibility and layers were not read: %+v", info)
	}

	p := s.lastRequest(t)
	if p["method"] != "pane.graphics.info" {
		t.Errorf("the method sent was %v", p["method"])
	}
	if p["jsonrpc"] != "2.0" {
		t.Errorf("jsonrpc = %v, want 2.0", p["jsonrpc"])
	}
	id, _ := p["id"].(string)
	if !strings.HasPrefix(id, "prdash:") || !strings.Contains(id, "info") {
		t.Errorf("the id is %q, and it does not identify the request", id)
	}
	params, _ := p["params"].(map[string]any)
	if params["pane_id"] != "pane-1" {
		t.Errorf("params.pane_id = %v, want pane-1", params["pane_id"])
	}
}

// The commonest case of all: Herdr is not running, or HERDR_SOCKET_PATH points at a session that no
// longer exists.
func TestMissingSocketDegradesToErrNoGraphicsAndSaysSo(t *testing.T) {
	// The path is inside a directory that does exist, so the failure is ENOENT of the socket and not of
	//the directory; the same error to errors.Is, but a different meaning.
	missing := filepath.Join(t.TempDir(), "no-existe.sock")
	g := &Graphics{Socket: missing, PaneID: "pane-1", Timeout: time.Second}

	_, err := g.Info(context.Background())
	if err == nil {
		t.Fatal("Info against a missing socket gave nil")
	}
	if !errors.Is(err, ErrNoGraphics) {
		t.Errorf("the error %v is not ErrNoGraphics: the popup cannot recognize it", err)
	}
	if !strings.Contains(err.Error(), "no such file") && !strings.Contains(err.Error(), "socket") {
		t.Errorf("the error %q does not say what failed", err)
	}

	if err := g.Clear(context.Background(), "layer"); !errors.Is(err, ErrNoGraphics) {
		t.Errorf("Clear gave %v, want ErrNoGraphics", err)
	}
	if err := g.SetImage(context.Background(), "layer", testImage(4, 4),
		Placement{Col: 0, Row: 0, Cols: 2, Rows: 2}); !errors.Is(err, ErrNoGraphics) {
		t.Errorf("SetImage gave %v, want ErrNoGraphics", err)
	}
}

// This is the case `len(line) == 0` exists for: a half close leaves the read with EOF.
func TestServerClosingWithoutAnsweringDoesNotHang(t *testing.T) {
	g, _ := newServer(t, closeWithoutAnswering)

	start := time.Now()
	_, err := g.Info(context.Background())
	took := time.Since(start)

	if err == nil {
		t.Fatal("Info against a closing server gave nil: an empty answer would be accepted")
	}
	if !errors.Is(err, ErrNoGraphics) {
		t.Errorf("the error %v is not ErrNoGraphics", err)
	}
	if took > time.Second {
		t.Errorf("it took %s to detect the close: it waited for the timeout", took)
	}
}

// The only test in the file that needs to wait, and it waits for real: a short timeout and a
// check that the call comes back before it.
func TestServerNotAnsweringIsCutByTheTimeout(t *testing.T) {
	g, _ := newServer(t, dontAnswer)
	g.Timeout = 80 * time.Millisecond

	start := time.Now()
	info, err := g.Info(context.Background())
	took := time.Since(start)

	if err == nil {
		t.Fatalf("Info against a mute server gave nil: %+v", info)
	}
	if !errors.Is(err, ErrNoGraphics) {
		t.Errorf("the error %v is not ErrNoGraphics", err)
	}
	// The margin is ten times the timeout plus a second on purpose, since the measurement is wall
	//clock.
	if took > time.Second {
		t.Errorf("it took %s with a timeout of 80ms: the timeout does not cut", took)
	}

	width, height := g.CellSize(context.Background())
	if width != defaultCellWidthPx || height != defaultCellHeightPx {
		t.Errorf("with the socket hung the cell came out %dx%d, want the approximation %dx%d",
			width, height, defaultCellWidthPx, defaultCellHeightPx)
	}
}

// This is the case where Herdr says something useful —"no such pane", "method not available in this
// version"— and that text has to reach the popup.
func TestServerErrorArrivesWithItsCodeAndMessage(t *testing.T) {
	g, _ := newServer(t, rpcErr("no_such_pane", "pane pane-1 not found"))

	_, err := g.Info(context.Background())
	if err == nil {
		t.Fatal("a server error gave nil")
	}
	// The server's error is NOT wrapped in ErrNoGraphics, and my first version assumed it was: only
	// probe (which asks whether there is one) and SetImage ever look at it.
	for _, want := range []string{"no_such_pane", "not found"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error %q does not bring %q from the server", err, want)
		}
	}
}

// HERDR_SOCKET_PATH can point at another program's socket —an SSH server, an LSP, a thing left
// running—and it answers.
func TestNonJSONResponseIsRejectedAndNotPaddedWithZeros(t *testing.T) {
	for _, tc := range []struct {
		name string
		f    func(net.Conn, map[string]any)
	}{
		{"garbage", sendGarbage},
		{"response without result field", func(c net.Conn, _ map[string]any) {
			_, _ = c.Write([]byte(`{"hola":"que tal"}` + "\n"))
		}},
		{"empty response", func(c net.Conn, _ map[string]any) {
			_, _ = c.Write([]byte("{}" + "\n"))
		}},
		{"result that is not the expected object", respondWith([]int{1, 2, 3})},
	} {
		g, _ := newServer(t, tc.f)
		info, err := g.Info(context.Background())
		if err == nil {
			t.Errorf("%s: it gave nil with info %+v", tc.name, info)
			continue
		}
		if err == nil || strings.HasPrefix(err.Error(), "herdr: pane graphics unavailable") {
			t.Errorf("%s: the error %v does not distinguish the data failure", tc.name, err)
		}
		if info != (GraphicsInfo{}) {
			t.Errorf("%s: with an error it returns info %+v, want the zero value", tc.name, info)
		}
	}
}

func TestClearTravelsTheSocketWithItsLayerAndPane(t *testing.T) {
	g, s := newServer(t, respondWith(map[string]any{}))

	if err := g.Clear(context.Background(), "prdash-sim"); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	p := s.lastRequest(t)
	if p["method"] != "pane.graphics.clear" {
		t.Errorf("the method was %v", p["method"])
	}
	params, _ := p["params"].(map[string]any)
	if params["layer_id"] != "prdash-sim" {
		t.Errorf("layer_id = %v, want prdash-sim", params["layer_id"])
	}
	if params["pane_id"] != "pane-1" {
		t.Errorf("pane_id = %v, want pane-1", params["pane_id"])
	}

	g2, _ := newServer(t, rpcErr("layer_not_found", "no such layer"))
	if err := g2.Clear(context.Background(), "prdash-sim"); err == nil {
		t.Error("Clear with a server error gave nil: the popup would close believing it removed the layer")
	}
}

// "Without resizing" is the contract, not a detail: the caller already fitted the image and rescaling
// again would throw detail away.
func TestSetImageSendsThePNGAndTheSizeWithoutResizing(t *testing.T) {
	g, s := newServer(t, respondWith(map[string]any{}))

	img := testImage(7, 5)
	rect := Placement{Col: 2, Row: 3, Cols: 4, Rows: 3}

	if err := g.SetImage(context.Background(), "prdash-sim", img, rect); err != nil {
		t.Fatalf("SetImage: %v", err)
	}
	p := s.lastRequest(t)
	if p["method"] != "pane.graphics.set" {
		t.Errorf("the method was %v", p["method"])
	}
	params, _ := p["params"].(map[string]any)
	if params["format"] != "png" {
		t.Errorf("format = %v, want png", params["format"])
	}
	if params["image_width"] != float64(7) || params["image_height"] != float64(5) {
		t.Errorf("the image dimensions are %v x %v, want 7 x 5",
			params["image_width"], params["image_height"])
	}
	pl, _ := params["placement"].(map[string]any)
	for key, want := range map[string]any{
		"viewport_col": float64(2), "viewport_row": float64(3),
		"grid_cols": float64(4), "grid_rows": float64(3),
	} {
		if pl[key] != want {
			t.Errorf("placement.%s = %v, want %v", key, pl[key], want)
		}
	}
	expected, err := encodePNG(img)
	if err != nil {
		t.Fatal(err)
	}
	if got := params["data_base64"]; got != base64.StdEncoding.EncodeToString(expected) {
		t.Error("the PNG that travelled is not the image's one: it was rescaled or re-encoded")
	}
	if params["z_index"] != float64(0) {
		t.Errorf("z_index = %v, want 0", params["z_index"])
	}

	g3, s3 := newServer(t, respondWith(map[string]any{}))
	if err := g3.SetImage(context.Background(), "layer", nil, rect); !errors.Is(err, ErrNoGraphics) {
		t.Errorf("SetImage with a nil image gave %v", err)
	}
	if err := g3.SetImage(context.Background(), "layer", img, Placement{}); !errors.Is(err, ErrNoGraphics) {
		t.Errorf("SetImage with an empty rectangle gave %v", err)
	}
	s3.mu.Lock()
	n := len(s3.requests)
	s3.mu.Unlock()
	if n != 0 {
		t.Errorf("%d requests went out to the socket with an invalid image or rectangle", n)
	}
}

// A zero-sized image is what makes png.Encode reject it with "invalid image size".
func TestUnencodableImageDoesNotLeaveTheProcess(t *testing.T) {
	g, s := newServer(t, respondWith(map[string]any{}))
	empty := image.NewRGBA(image.Rect(0, 0, 0, 0))

	err := g.SetImage(context.Background(), "layer", empty,
		Placement{Col: 0, Row: 0, Cols: 1, Rows: 1})
	if err == nil {
		t.Fatal("encoding a zero-size image gave nil")
	}
	if errors.Is(err, ErrNoGraphics) {
		t.Error("an encoding failure would be wrapped in ErrNoGraphics: the popup would say " +
			"there is no Herdr when the problem is the image")
	}
	if !strings.Contains(err.Error(), "herdr") {
		t.Errorf("the error %q does not say where it comes from", err)
	}

	s.mu.Lock()
	n := len(s.requests)
	s.mu.Unlock()
	if n != 0 {
		t.Errorf("%d requests went out over the socket with an unencodable image", n)
	}
}

func testImage(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{R: uint8(x * 7), G: uint8(y * 11), B: 128, A: 255})
		}
	}
	return img
}
