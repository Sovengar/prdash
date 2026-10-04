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

// defaultCellWidthPx y defaultCellHeightPx son las dimensiones de celda con las que
// se dibuja cuando el pane no las dice. 1×2 es lo habitual en un terminal, y es una
// aproximación declarada: una imagen deformada se nota pero se mira, y sin imagen no
// se mira nada.
const (
	defaultCellWidthPx  = 1
	defaultCellHeightPx = 2
)

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
//
// Una colocación con una dimensión en cero o menos no dibuja nada, y ademas es peor
// que no dibujar: el rectángulo de celdas se manda igual y el servidor tiene que
// decidir qué hacer con un rectángulo degenerado. Por eso se comprueba ANTES de
// mandar nada, y no se deja que lo rechace el otro lado.
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
	return graphicsReady(g.env("HERDR_ENV"), g.socket(), g.pane()) && g.probe()
}

// probe pregunta por el pane para confirmar que el método existe en la versión
// instalada. Va aparte de la política a propósito: la política es una función pura
// de tres cadenas y se puede comprobar entera, y la pregunta es la parte que cuesta
// una ida al socket.
func (g *Graphics) probe() bool {
	_, err := g.Info(context.Background())
	return err == nil
}

// graphicsReady son las condiciones para que la capa de gráficos sea un camino
// posible, SIN la ida al socket: estar dentro de Herdr y tener socket y pane.
//
// Se separa de Available por el mismo motivo que el resto de la geometría de este
// repo: dentro de Available, con su ida al socket detrás, estas tres comparaciones
// no se pueden comprobar sin montar un socket falso. Como lo que decide es una
// regla de tres entradas y una salida, se afirma como lo que es, con las nueve
// combinaciones.
//
// Y el orden importa: estar fuera de Herdr se comprueba PRIMERO, antes de mirar el
// socket. Al revés, un proceso corriendo fuera de Herdr con las variables puestas se
// irait al socket de todas formas, y no es solo una ida inútil: el socket puede estar
// ahí, de otro proceso, y escribirle sería escribir en la sesión de otro.
func graphicsReady(herdrEnv, socket, pane string) bool {
	if herdrEnv != "1" {
		return false
	}
	return socket != "" && pane != ""
}

// graphicsTimeoutFor es el plazo de una petición: el que venga, o el de por defecto.
//
// Un plazo de cero o NEGATIVO no es "sin plazo", es un plazo que ya pasó, y eso hace
// que la petición se corte antes de enviarse. Por eso se sustituye, no se usa tal
// cual.
func graphicsTimeoutFor(t time.Duration) time.Duration {
	if t <= 0 {
		return graphicsTimeout
	}
	return t
}

// haveGraphicsTarget dice si hay a quién preguntar: socket y pane.
//
// Los dos hacen falta, y no por simetría: sin pane no hay rectángulo donde colocar la
// imagen, y sin socket no hay a quién preguntarlo. Mandar la petición con solo uno
// de los dos convierte un error local y barato (antes de tocar nada) en un error
// remoto y opaco.
func haveGraphicsTarget(socket, pane string) bool { return socket != "" && pane != "" }

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
	return cellSizeFrom(g.Info(ctx))
}

// cellSizeFrom decide el tamaño de celda a partir de lo que respondió el pane, sin
// la ida al socket.
//
// Las tres degradaciones, y las tres son distintas:
//
//   - No se pudo preguntar (error): aproximación 1×2.
//   - El pane respondió con una dimensión en cero o menos: también aproximación,
//     porque una celda de cero píxeles no es una celda, es una división por cero que
//     el que la use no ve venir.
//
// La segunda es la que se confunde con la primera: una celda de 0 de ancho y otra
// ausente dan el mismo resultado, y no es casualidad: las dos significan que no
// hay medida. Por eso la condición es un O, y por eso se comprueba que un 0 exacto
// cae ya en la aproximación y no en "usa el 0 que te han dado".
func cellSizeFrom(info GraphicsInfo, err error) (cellW, cellH int) {
	if err != nil || info.CellWidthPx <= 0 || info.CellHeightPx <= 0 {
		return defaultCellWidthPx, defaultCellHeightPx
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
	if out == nil {
		// Un método sin resultado no espera uno: `pane.graphics.clear` contesta `{}` y ya
		// está. Pedirlo aquí rechazaría la respuesta buena de los métodos sin retorno.
		return nil
	}
	if len(env.Result) == 0 {
		// Y para un método que SÍ espera resultado, que no haya ninguno es una respuesta
		// inválida, no un resultado vacío. Sin esta comprobación, un socket apuntando a otro
		// programa que conteste `{}` a todo haría que `probe()` dijera que la capa de
		// gráficos funciona —porque solo mira si hay error— y la TUI publicaría imágenes
		// que no aparecerían nunca, sin un solo aviso.
		//
		// Y `Info` devuelve su valor junto al error, así que el que se lleva los ceros de las
		// medidas es el que se lleva la respuesta vacía, no este.
		return errors.New("herdr: la respuesta no trae campo result")
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
