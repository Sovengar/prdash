package herdr

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"net"
	"os"
	"time"
)

var ErrNoGraphics = errors.New("herdr: pane graphics unavailable")

// Better to fall to half-blocks than to leave the popup half painted.
const graphicsTimeout = 5 * time.Second

const (
	defaultCellWidthPx  = 1
	defaultCellHeightPx = 2
)

// Named and ours, so it can be removed without touching anything foreign: the layer belongs to the
// pane, and what is not prdash's does not get touched.
const GraphicsLayer = "prdash-sim"

type Placement struct {
	Col  int // columna del viewport donde empieza
	Row  int // fila del viewport donde empieza
	Cols int // anchura en celdas
	Rows int // altura en celdas
}

// Checked BEFORE sending, not left for the server to reject: a degenerate rectangle is worse than
// not drawing, because the server still has to decide what to do with it.
func (p Placement) Empty() bool { return p.Cols <= 0 || p.Rows <= 0 }

type GraphicsInfo struct {
	// The ratio between them is what decides how many columns per row an image needs to avoid
	// distortion, and it is not 2:1 but whatever the terminal measures.
	CellWidthPx  int
	CellHeightPx int
	// An invisible pane does not show its layer, so the image has to go to half-blocks.
	PaneVisible bool
	MaxLayers   int
}

// Over the socket because `herdr pane graphics` is not a subcommand: the socket is the only API.
// One connection per request because the server closes it after each response.
type Graphics struct {
	Socket  string
	PaneID  string
	Timeout time.Duration

	dial   func(ctx context.Context, network, addr string) (net.Conn, error)
	getenv func(string) string
}

func NewGraphics() *Graphics { return &Graphics{getenv: os.Getenv} }

func (g *Graphics) env(key string) string {
	if g.getenv != nil {
		return g.getenv(key)
	}
	return os.Getenv(key)
}

func (g *Graphics) socket() string {
	if g.Socket != "" {
		return g.Socket
	}
	return g.env("HERDR_SOCKET_PATH")
}

func (g *Graphics) pane() string {
	if g.PaneID != "" {
		return g.PaneID
	}
	return g.env("HERDR_PANE_ID")
}

// The method's existence is deduced from the pane id it is called with, so it cannot be asserted
// without asking, and asking costs a trip to the socket.
func (g *Graphics) Available() bool {
	return graphicsReady(g.env("HERDR_ENV"), g.socket(), g.pane()) && g.probe()
}

// Separate on purpose: the policy is a pure function of three strings and can be checked whole,
// while the question is the part that costs a socket trip.
func (g *Graphics) probe() bool {
	_, err := g.Info(context.Background())
	return err == nil
}

// The order matters: "outside Herdr" is checked FIRST. The other way round, a process running
// outside Herdr with the variables set would still write to the socket, and the socket may belong to
// another process: that would be writing into someone else's session.
func graphicsReady(herdrEnv, socket, pane string) bool {
	if herdrEnv != "1" {
		return false
	}
	return socket != "" && pane != ""
}

func graphicsTimeoutFor(t time.Duration) time.Duration {
	if t <= 0 {
		return graphicsTimeout
	}
	return t
}

func haveGraphicsTarget(socket, pane string) bool { return socket != "" && pane != "" }

func (g *Graphics) Info(ctx context.Context) (GraphicsInfo, error) {
	var info GraphicsInfo
	if g.pane() == "" {
		return info, ErrNoGraphics
	}
	// `call` already unwraps the envelope, so the payload is deserialised directly: one less level of
	// nesting is one less "{} result result" for someone to write by mistake.
	var res struct {
		CellWidthPx  int  `json:"cell_width_px"`
		CellHeightPx int  `json:"cell_height_px"`
		PaneVisible  bool `json:"pane_visible"`
		MaxLayers    int  `json:"max_layers_per_pane"`
	}
	if err := g.call(ctx, "pane.graphics.info", map[string]any{"pane_id": g.pane()}, &res); err != nil {
		return GraphicsInfo{}, err
	}
	return GraphicsInfo{
		CellWidthPx:  res.CellWidthPx,
		CellHeightPx: res.CellHeightPx,
		PaneVisible:  res.PaneVisible,
		MaxLayers:    res.MaxLayers,
	}, nil
}

func (g *Graphics) CellSize(ctx context.Context) (cellW, cellH int) {
	return cellSizeFrom(g.Info(ctx))
}

// Two distinct degradations: the question failed, or the pane answered with a dimension at or below
// zero. The second is easy to confuse with the first and for the same reason, since both mean there
// is no measurement, which is why the condition is an OR and why an exact 0 must fall to the
// approximation rather than being used as given.
func cellSizeFrom(info GraphicsInfo, err error) (cellW, cellH int) {
	if err != nil || info.CellWidthPx <= 0 || info.CellHeightPx <= 0 {
		return defaultCellWidthPx, defaultCellHeightPx
	}
	return info.CellWidthPx, info.CellHeightPx
}

// Sent unscaled: the caller already fitted it to the rectangle, and rescaling again would throw
// away detail. PNG in base64 because that is what the documented API accepts.
func (g *Graphics) SetImage(ctx context.Context, layer string, img image.Image, p Placement) error {
	if img == nil || p.Empty() {
		return ErrNoGraphics
	}
	encoded, err := encodePNG(img)
	if err != nil {
		return err
	}
	params := map[string]any{
		"pane_id":      g.pane(),
		"format":       "png",
		"image_width":  img.Bounds().Dx(),
		"image_height": img.Bounds().Dy(),
		"data_base64":  base64.StdEncoding.EncodeToString(encoded),
		"layer_id":     layer,
		"z_index":      0,
		"placement": map[string]any{
			"viewport_col": p.Col,
			"viewport_row": p.Row,
			"grid_cols":    p.Cols,
			"grid_rows":    p.Rows,
		},
	}
	return g.call(ctx, "pane.graphics.set", params, nil)
}

// The layer sits above the pane's content, so leaving it would leave the image on top of the UI.
func (g *Graphics) Clear(ctx context.Context, layer string) error {
	if g.pane() == "" {
		return ErrNoGraphics
	}
	return g.call(ctx, "pane.graphics.clear", map[string]any{"pane_id": g.pane(), "layer_id": layer}, nil)
}

// One connection per request: the server closes after answering, so reusing it would only produce
// a pipe error on the second call.
func (g *Graphics) call(ctx context.Context, method string, params map[string]any, out any) error {
	socket := g.socket()
	if !haveGraphicsTarget(socket, g.pane()) {
		return ErrNoGraphics
	}
	timeout := graphicsTimeoutFor(g.Timeout)
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	dial := g.dial
	if dial == nil {
		dial = func(ctx context.Context, network, addr string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, network, addr)
		}
	}
	conn, err := dial(cctx, "unix", socket)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNoGraphics, err)
	}
	defer func() { _ = conn.Close() }()

	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      "prdash:" + method,
		"method":  method,
		"params":  params,
	}
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	if _, err := conn.Write(append(body, '\n')); err != nil {
		return fmt.Errorf("%w: %v", ErrNoGraphics, err)
	}

	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil && len(line) == 0 {
		return fmt.Errorf("%w: %v", ErrNoGraphics, err)
	}
	return decodeResponse(line, out)
}

type rpcError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string {
	if e.Message == "" {
		return e.Code
	}
	return e.Code + ": " + e.Message
}

func decodeResponse(line []byte, out any) error {
	var env struct {
		Result json.RawMessage `json:"result"`
		Error  *rpcError       `json:"error"`
	}
	if err := json.Unmarshal(line, &env); err != nil {
		return fmt.Errorf("herdr: respuesta ilegible: %w", err)
	}
	if env.Error != nil {
		return env.Error
	}
	if out == nil {
		return nil
	}
	if len(env.Result) == 0 {
		// For a method that DOES expect a result, none being there is an invalid response, not an empty
		// result. Without this a socket pointing at another program that answers `{}` to everything
		// would make `probe()` report the graphics layer as working, and the UI would publish images
		// that never appear, with no warning at all.
		return errors.New("herdr: la respuesta no trae campo result")
	}
	return json.Unmarshal(env.Result, out)
}

// git-sim's JPEG is re-encoded because the API does not accept jpeg: it weighs more, and that is
// what the terminal can decode.
func encodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("herdr: codificar la imagen: %w", err)
	}
	return buf.Bytes(), nil
}
