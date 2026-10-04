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

type servidorRPC struct {
	t          *testing.T
	ln         net.Listener
	guion      func(c net.Conn, peticion map[string]any)
	peticiones []map[string]any
	mu         sync.Mutex
	cerrado    bool
}

// The socket lives in t.TempDir() because Unix sockets are paths and a long one goes past
// sun_path's 108-byte limit.
func nuevoServidor(t *testing.T, guion func(c net.Conn, peticion map[string]any)) (*Graphics, *servidorRPC) {
	t.Helper()

	dir, err := os.MkdirTemp("", "herdr-sock")
	if err != nil {
		t.Fatal(err)
	}
	ruta := filepath.Join(dir, "s")

	ln, err := net.Listen("unix", ruta)
	if err != nil {
		_ = os.RemoveAll(dir)
		t.Fatalf("escuchar en %s: %v", ruta, err)
	}

	s := &servidorRPC{t: t, ln: ln, guion: guion}
	go s.atender()

	g := &Graphics{Socket: ruta, PaneID: "pane-1", Timeout: 2 * time.Second}
	t.Cleanup(func() {
		s.parar()
		_ = os.RemoveAll(dir)
	})
	return g, s
}

func (s *servidorRPC) atender() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return // el listener se cerró
		}
		go func() {
			defer func() { _ = conn.Close() }()
			linea, err := bufio.NewReader(conn).ReadBytes('\n')
			if err != nil {
				return
			}
			var peticion map[string]any
			if err := json.Unmarshal(linea, &peticion); err != nil {
				return
			}
			s.mu.Lock()
			s.peticiones = append(s.peticiones, peticion)
			s.mu.Unlock()
			if s.guion != nil {
				s.guion(conn, peticion)
			}
		}()
	}
}

func (s *servidorRPC) parar() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cerrado {
		return
	}
	s.cerrado = true
	_ = s.ln.Close()
}

func (s *servidorRPC) ultimaPeticion(t *testing.T) map[string]any {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.peticiones) == 0 {
		t.Fatal("el servidor no recibió ninguna petición")
	}
	return s.peticiones[len(s.peticiones)-1]
}

func responder(result any) func(net.Conn, map[string]any) {
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

func sinResponder(net.Conn, map[string]any) {}

func cerrarSinContestar(c net.Conn, _ map[string]any) { _ = c.Close() }

func mandarBasura(c net.Conn, _ map[string]any) {
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
func TestElSocketDeVerdadSeAbreYSeResponde(t *testing.T) {
	g, s := nuevoServidor(t, responder(map[string]any{
		"cell_width_px": 11, "cell_height_px": 22,
		"pane_visible": true, "max_layers_per_pane": 4,
	}))

	info, err := g.Info(context.Background())
	if err != nil {
		t.Fatalf("Info contra el socket de verdad: %v", err)
	}
	if info.CellWidthPx != 11 || info.CellHeightPx != 22 {
		t.Errorf("la celda sale %dx%d, want 11x22", info.CellWidthPx, info.CellHeightPx)
	}
	if !info.PaneVisible || info.MaxLayers != 4 {
		t.Errorf("visibilidad y capas no se leyeron: %+v", info)
	}

	p := s.ultimaPeticion(t)
	if p["method"] != "pane.graphics.info" {
		t.Errorf("el método enviado fue %v", p["method"])
	}
	if p["jsonrpc"] != "2.0" {
		t.Errorf("jsonrpc = %v, want 2.0", p["jsonrpc"])
	}
	id, _ := p["id"].(string)
	if !strings.HasPrefix(id, "prdash:") || !strings.Contains(id, "info") {
		t.Errorf("el id es %q, y no identifica la petición", id)
	}
	params, _ := p["params"].(map[string]any)
	if params["pane_id"] != "pane-1" {
		t.Errorf("params.pane_id = %v, want pane-1", params["pane_id"])
	}
}

// The commonest case of all: Herdr is not running, or HERDR_SOCKET_PATH points at a session that no
// longer exists.
func TestUnSocketQueNoExisteDegradaAErrNoGraphicsYLoDiga(t *testing.T) {
	// The path is inside a directory that does exist, so the failure is ENOENT of the socket and not of
	//the directory; the same error to errors.Is, but a different meaning.
	ausente := filepath.Join(t.TempDir(), "no-existe.sock")
	g := &Graphics{Socket: ausente, PaneID: "pane-1", Timeout: time.Second}

	_, err := g.Info(context.Background())
	if err == nil {
		t.Fatal("Info contra un socket inexistente dio nil")
	}
	if !errors.Is(err, ErrNoGraphics) {
		t.Errorf("el error %v no es ErrNoGraphics: el popup no puede reconocerlo", err)
	}
	if !strings.Contains(err.Error(), "no such file") && !strings.Contains(err.Error(), "socket") {
		t.Errorf("el error %q no dice qué falló", err)
	}

	if err := g.Clear(context.Background(), "capa"); !errors.Is(err, ErrNoGraphics) {
		t.Errorf("Clear dio %v, want ErrNoGraphics", err)
	}
	if err := g.SetImage(context.Background(), "capa", imagenDePrueba(4, 4),
		Placement{Col: 0, Row: 0, Cols: 2, Rows: 2}); !errors.Is(err, ErrNoGraphics) {
		t.Errorf("SetImage dio %v, want ErrNoGraphics", err)
	}
}

// This is the case `len(line) == 0` exists for: a half close leaves the read with EOF.
func TestUnServidorQueCierraSinContestarNoSeQuedaColgado(t *testing.T) {
	g, _ := nuevoServidor(t, cerrarSinContestar)

	antes := time.Now()
	_, err := g.Info(context.Background())
	tardó := time.Since(antes)

	if err == nil {
		t.Fatal("Info contra un servidor que cierra dio nil: se aceptaría una respuesta vacía")
	}
	if !errors.Is(err, ErrNoGraphics) {
		t.Errorf("el error %v no es ErrNoGraphics", err)
	}
	if tardó > time.Second {
		t.Errorf("tardó %s en detectar el cierre: se esperó al timeout", tardó)
	}
}

// The only test in the file that needs to wait, and it waits for real: a short timeout and a
// check that the call comes back before it.
func TestUnServidorQueNoContestaCortaAlTimeout(t *testing.T) {
	g, _ := nuevoServidor(t, sinResponder)
	g.Timeout = 80 * time.Millisecond

	antes := time.Now()
	info, err := g.Info(context.Background())
	tardó := time.Since(antes)

	if err == nil {
		t.Fatalf("Info contra un servidor mudo dio nil: %+v", info)
	}
	if !errors.Is(err, ErrNoGraphics) {
		t.Errorf("el error %v no es ErrNoGraphics", err)
	}
	// The margin is ten times the timeout plus a second on purpose, since the measurement is wall
	//clock.
	if tardó > time.Second {
		t.Errorf("tardó %s con un timeout de 80ms: el timeout no corta", tardó)
	}

	ancho, alto := g.CellSize(context.Background())
	if ancho != defaultCellWidthPx || alto != defaultCellHeightPx {
		t.Errorf("con el socket colgado la celda sale %dx%d, want la aproximación %dx%d",
			ancho, alto, defaultCellWidthPx, defaultCellHeightPx)
	}
}

// This is the case where Herdr says something useful —"no such pane", "method not available in this
// version"— and that text has to reach the popup.
func TestUnErrorDelServidorLlegaConSuCodigoYSuMensaje(t *testing.T) {
	g, _ := nuevoServidor(t, rpcErr("no_such_pane", "pane pane-1 not found"))

	_, err := g.Info(context.Background())
	if err == nil {
		t.Fatal("un error del servidor dio nil")
	}
	// The server's error is NOT wrapped in ErrNoGraphics, and my first version assumed it was: only
	// probe (which asks whether there is one) and SetImage ever look at it.
	for _, quiere := range []string{"no_such_pane", "not found"} {
		if !strings.Contains(err.Error(), quiere) {
			t.Errorf("el error %q no trae %q del servidor", err, quiere)
		}
	}
}

// HERDR_SOCKET_PATH can point at another program's socket —an SSH server, an LSP, a thing left
// running—and it answers.
func TestUnaRespuestaQueNoEsJSONSeRechazaYNoSePintaDeCeros(t *testing.T) {
	for _, guion := range []struct {
		nombre string
		f      func(net.Conn, map[string]any)
	}{
		{"basura", mandarBasura},
		{"respuesta sin campo result", func(c net.Conn, _ map[string]any) {
			_, _ = c.Write([]byte(`{"hola":"que tal"}` + "\n"))
		}},
		{"respuesta vacia", func(c net.Conn, _ map[string]any) {
			_, _ = c.Write([]byte("{}" + "\n"))
		}},
		{"result que no es el objeto esperado", responder([]int{1, 2, 3})},
	} {
		g, _ := nuevoServidor(t, guion.f)
		info, err := g.Info(context.Background())
		if err == nil {
			t.Errorf("%s: dio nil con info %+v", guion.nombre, info)
			continue
		}
		if err == nil || strings.HasPrefix(err.Error(), "herdr: pane graphics unavailable") {
			t.Errorf("%s: el error %v no distingue el fallo de datos", guion.nombre, err)
		}
		if info != (GraphicsInfo{}) {
			t.Errorf("%s: con error devuelve info %+v, want el valor cero", guion.nombre, info)
		}
	}
}

func TestClearViajaPorElSocketConSuCapaYConSuPane(t *testing.T) {
	g, s := nuevoServidor(t, responder(map[string]any{}))

	if err := g.Clear(context.Background(), "prdash-sim"); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	p := s.ultimaPeticion(t)
	if p["method"] != "pane.graphics.clear" {
		t.Errorf("el método fue %v", p["method"])
	}
	params, _ := p["params"].(map[string]any)
	if params["layer_id"] != "prdash-sim" {
		t.Errorf("layer_id = %v, want prdash-sim", params["layer_id"])
	}
	if params["pane_id"] != "pane-1" {
		t.Errorf("pane_id = %v, want pane-1", params["pane_id"])
	}

	g2, _ := nuevoServidor(t, rpcErr("layer_not_found", "no such layer"))
	if err := g2.Clear(context.Background(), "prdash-sim"); err == nil {
		t.Error("Clear con error del servidor dio nil: el popup cerraría creyendo que quitó la capa")
	}
}

// "Without resizing" is the contract, not a detail: the caller already fitted the image and rescaling
// again would throw detail away.
func TestSetImageMandaElPNGYElTamanoSinRedimensionar(t *testing.T) {
	g, s := nuevoServidor(t, responder(map[string]any{}))

	img := imagenDePrueba(7, 5)
	rect := Placement{Col: 2, Row: 3, Cols: 4, Rows: 3}

	if err := g.SetImage(context.Background(), "prdash-sim", img, rect); err != nil {
		t.Fatalf("SetImage: %v", err)
	}
	p := s.ultimaPeticion(t)
	if p["method"] != "pane.graphics.set" {
		t.Errorf("el método fue %v", p["method"])
	}
	params, _ := p["params"].(map[string]any)
	if params["format"] != "png" {
		t.Errorf("format = %v, want png", params["format"])
	}
	if params["image_width"] != float64(7) || params["image_height"] != float64(5) {
		t.Errorf("las medidas de la imagen son %v x %v, want 7 x 5",
			params["image_width"], params["image_height"])
	}
	pl, _ := params["placement"].(map[string]any)
	for clave, quiere := range map[string]any{
		"viewport_col": float64(2), "viewport_row": float64(3),
		"grid_cols": float64(4), "grid_rows": float64(3),
	} {
		if pl[clave] != quiere {
			t.Errorf("placement.%s = %v, want %v", clave, pl[clave], quiere)
		}
	}
	esperado, err := encodePNG(img)
	if err != nil {
		t.Fatal(err)
	}
	if got := params["data_base64"]; got != base64.StdEncoding.EncodeToString(esperado) {
		t.Error("el PNG que viajó no es el de la imagen: se reescaló o se re-codificó")
	}
	if params["z_index"] != float64(0) {
		t.Errorf("z_index = %v, want 0", params["z_index"])
	}

	g3, s3 := nuevoServidor(t, responder(map[string]any{}))
	if err := g3.SetImage(context.Background(), "capa", nil, rect); !errors.Is(err, ErrNoGraphics) {
		t.Errorf("SetImage con imagen nil dio %v", err)
	}
	if err := g3.SetImage(context.Background(), "capa", img, Placement{}); !errors.Is(err, ErrNoGraphics) {
		t.Errorf("SetImage con rectángulo vacío dio %v", err)
	}
	s3.mu.Lock()
	n := len(s3.peticiones)
	s3.mu.Unlock()
	if n != 0 {
		t.Errorf("salieron %d peticiones al socket con una imagen o un rectángulo inválido", n)
	}
}

// A zero-sized image is what makes png.Encode reject it with "invalid image size".
func TestUnaImagenQueNoSePuedeCodificarNoSaleDelProceso(t *testing.T) {
	g, s := nuevoServidor(t, responder(map[string]any{}))
	vacia := image.NewRGBA(image.Rect(0, 0, 0, 0))

	err := g.SetImage(context.Background(), "capa", vacia,
		Placement{Col: 0, Row: 0, Cols: 1, Rows: 1})
	if err == nil {
		t.Fatal("codificar una imagen de tamaño cero dio nil")
	}
	if errors.Is(err, ErrNoGraphics) {
		t.Error("un fallo de codificación se envolvería en ErrNoGraphics: el popup diría " +
			"que no hay Herdr cuando el problema es la imagen")
	}
	if !strings.Contains(err.Error(), "herdr") {
		t.Errorf("el error %q no dice de dónde viene", err)
	}

	s.mu.Lock()
	n := len(s.peticiones)
	s.mu.Unlock()
	if n != 0 {
		t.Errorf("salieron %d peticiones por el socket con una imagen incodificable", n)
	}
}

func imagenDePrueba(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{R: uint8(x * 7), G: uint8(y * 11), B: 128, A: 255})
		}
	}
	return img
}
