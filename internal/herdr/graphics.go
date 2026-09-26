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

// ErrNoGraphics es lo que devuelven las operaciones de la capa cuando no hay
// Herdr detrás. No es un fallo: es el camino de los half-blocks.
var ErrNoGraphics = errors.New("herdr: pane graphics unavailable")

// graphicsTimeout acota una operación de la capa. Son lecturas y escrituras
// locales por socket: si tardan, algo va mal y es mejor caer a half-blocks que
// dejar el popup a medias.
const graphicsTimeout = 5 * time.Second

// GraphicsLayer es la capa donde prdash publica la imagen de la simulación. Es
// propia y con nombre, para poder quitarla sin tocar nada ajeno: la capa es del
// pane, no de prdash, y lo que no es de prdash no se toca.
const GraphicsLayer = "prdash-sim"

// Placement sitúa la imagen en el pane, en celdas del viewport. Es lo que Herdr
// entiende: el pane es una rejilla de celdas y las coordenadas de la imagen son
// celdas, no píxeles.
type Placement struct {
	Col  int // columna del viewport donde empieza
	Row  int // fila del viewport donde empieza
	Cols int // anchura en celdas
	Rows int // altura en celdas
}

// Empty informa si la colocación no dibuja nada.
func (p Placement) Empty() bool { return p.Cols <= 0 || p.Rows <= 0 }

// GraphicsInfo es lo que Herdr sabe del pane que importa para colocar una imagen.
type GraphicsInfo struct {
	// CellWidthPx y CellHeightPx son las dimensiones reales de una celda. La
	// relación entre ellas es la que decide cuántas columnas por línea necesita
	// una imagen para no deformarse, y no es 2:1 sino lo que mida el terminal.
	CellWidthPx  int
	CellHeightPx int
	// PaneVisible indica si el pane se está viendo ahora. Un pane invisible no
	// muestra su capa, así que en ese caso la imagen tiene que ir en half-blocks:
	// es lo único que se ve.
	PaneVisible bool
	MaxLayers   int
}

// Graphics dibuja imágenes en la capa gráfica de un pane.
//
// Va por socket y no por CLI a propósito: `herdr pane graphics` no existe como
// subcomando, la API es solo socket. Y usa una conexión por petición porque el
// servidor la cierra después de cada respuesta.
type Graphics struct {
	// Socket es la ruta del socket; vacía usa HERDR_SOCKET_PATH.
	Socket string
	// PaneID es el pane donde se dibuja; vacío usa HERDR_PANE_ID.
	PaneID string
	// Timeout por petición; <=0 usa graphicsTimeout.
	Timeout time.Duration

	dial   func(ctx context.Context, network, addr string) (net.Conn, error)
	getenv func(string) string
}

// NewGraphics construye el cliente con el socket y el pane del entorno.
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

// Available informa si la capa de gráficos es un camino posible: hay que estar
// dentro de Herdr, con socket y pane conocido, y el método tiene que existir en la
// versión instalada. La última parte se deduce del id de pane con el que se llama,
// así que no se puede afirmar sin preguntar, y preguntar cuesta una ida al socket.
func (g *Graphics) Available() bool {
	if g.env("HERDR_ENV") != "1" {
		return false
	}
	if g.socket() == "" || g.pane() == "" {
		return false
	}
	_, err := g.Info(context.Background())
	return err == nil
}

// Info pregunta por el pane: tamaño de celda y visibilidad.
func (g *Graphics) Info(ctx context.Context) (GraphicsInfo, error) {
	var info GraphicsInfo
	if g.pane() == "" {
		return info, ErrNoGraphics
	}
	// call ya desenvuelve el sobre y entrega solo el campo result, así que aquí se
	// deserializa directamente el payload: un nivel menos de anidamiento es un
	// nivel menos de "{} result result" que alguien va a escribir por error.
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

// CellSize devuelve los píxeles de una celda del pane. Devuelve 1×2, lo habitual en
// un terminal, cuando no se puede preguntar: es una aproximación, y una imagen
// deformada es preferible a no pintar imagen.
func (g *Graphics) CellSize(ctx context.Context) (cellW, cellH int) {
	info, err := g.Info(ctx)
	if err != nil || info.CellWidthPx <= 0 || info.CellHeightPx <= 0 {
		return 1, 2
	}
	return info.CellWidthPx, info.CellHeightPx
}

// SetImage publica la imagen en la capa indicada, colocada en un rectángulo de
// celdas.
//
// La imagen se manda tal cual, sin reescalar: la caller ya la ajustó al tamaño del
// rectángulo, y aquí reescalar otra vez sería tirar detalle. Va como PNG en base64
// porque es lo que acepta la API documentada; ajustarla al tamaño del rectángulo es
// también lo que mantiene el mensaje en un tamaño razonable.
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

// Clear quita la capa indicada. Es lo que se llama al cerrar el popup: la capa vive
// por encima del contenido del pane, así que si no se quita, la imagen se queda
// encima de la TUI.
func (g *Graphics) Clear(ctx context.Context, layer string) error {
	if g.pane() == "" {
		return ErrNoGraphics
	}
	return g.call(ctx, "pane.graphics.clear", map[string]any{"pane_id": g.pane(), "layer_id": layer}, nil)
}

// call hace una petición JSON-RPC por el socket de Herdr y decodifica la respuesta
// en out (nil para no mirar).
//
// Una conexión por petición: el servidor la cierra después de responder, así que
// reutilizarla solo produciría un error de pipe en la segunda llamada.
func (g *Graphics) call(ctx context.Context, method string, params map[string]any, out any) error {
	socket := g.socket()
	if socket == "" || g.pane() == "" {
		return ErrNoGraphics
	}
	timeout := g.Timeout
	if timeout <= 0 {
		timeout = graphicsTimeout
	}
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
	defer conn.Close()

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
	// El servidor lee líneas enteras, así que el mensaje necesita su salto final.
	if _, err := conn.Write(append(body, '\n')); err != nil {
		return fmt.Errorf("%w: %v", ErrNoGraphics, err)
	}

	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil && len(line) == 0 {
		return fmt.Errorf("%w: %v", ErrNoGraphics, err)
	}
	return decodeResponse(line, out)
}

// rpcError es el error que devuelve el servidor en un campo error.
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

// decodeResponse separa el error del resultado, que vienen en el mismo sobre.
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
	if out == nil || len(env.Result) == 0 {
		return nil
	}
	return json.Unmarshal(env.Result, out)
}

// encodePNG serializa la imagen. El JPEG de git-sim se re-codifica porque la API no
// acepta jpeg: pesa más que el original, pero es lo que el terminal sabe decodificar,
// y la caller ya la ajustó al tamaño del rectángulo para que ese sobrecoste sea
// pequeño.
func encodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("herdr: codificar la imagen: %w", err)
	}
	return buf.Bytes(), nil
}
