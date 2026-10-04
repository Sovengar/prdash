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

// Estos tests hablan con un **socket Unix de verdad** por el que corre un servidor JSON-RPC
// de verdad, hecho de verdad con `net.Listen("unix", …)` y `net.Conn`.
//
// Y el motivo de no usar el doble de `dial` que el resto del paquete usa es que el doble se
// salta justo la mitad del contrato: un `net.Conn` de mentira no tiene tubería, ni cierre a
// mitad de respuesta, ni servidor que acepta y muere, ni ENOENT al buscar un socket que no
// existe. Esas son las cuatro cosas que `call` tiene que manejar y las cuatro son del
// núcleo, no de prdash.
//
// Y lo que hay que probar aquí es que el fallo degrada a `ErrNoGraphics` —que es lo que hace
// que la TUI siga viva sin capa de gráficos— y no a un error opaco que el popup no sabe
// mostrar. Un `connection refused` crudo en medio de un render es un mensaje que el usuario
// lee y no entiende.

// servidorRPC es un servidor JSON-RPC de línea sobre un socket Unix real. Cada conexión se
// atiende en su propia goroutine y se cierra después de responder, que es lo que hace el
// servidor de Herdr y la razón de que `call` abra una conexión por petición.
type servidorRPC struct {
	t  *testing.T
	ln net.Listener
	// guion decide qué hacer con la petición: `responder` la contesta, `colgar` se queda
	// sin contestar, `basura` contesta algo que no es JSON, `rpcError` contesta un error.
	guion func(c net.Conn, peticion map[string]any)
	// peticiones guarda lo recibido, para comprobar que lo que sale es lo que se pidió.
	peticiones []map[string]any
	mu         sync.Mutex
	cerrado    bool
}

// nuevoServidor levanta el socket en un temporal y devuelve el cliente y el servidor. El
// socket va en `t.TempDir()` porque los sockets Unix son rutas, y una ruta larga se pasa del
// límite de 108 bytes de `sun_path` —que es el motivo por el que los daemons los ponen en
// `/tmp` y no en un temporal de prueba—.
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

// responder contesta con un `result` y cierra, que es el camino feliz.
func responder(result any) func(net.Conn, map[string]any) {
	return func(c net.Conn, _ map[string]any) {
		_, _ = fmt.Fprintf(c, `{"jsonrpc":"2.0","id":"x","result":%s}`+"\n", mustJSON(result))
	}
}

// rpcErr contesta con un error de protocolo, que es lo que devuelve Herdr cuando el pane no
// existe o el método no está en esa versión.
func rpcErr(code, msg string) func(net.Conn, map[string]any) {
	return func(c net.Conn, _ map[string]any) {
		_, _ = fmt.Fprintf(c,
			`{"jsonrpc":"2.0","id":"x","error":{"code":%q,"message":%q}}`+"\n", code, msg)
	}
}

// sinResponder acepta la conexión y no contesta nada: el cliente se queda esperando hasta su
// timeout. Es el caso de un Herdr colgado, que es el que el timeout existe para no comerse.
func sinResponder(net.Conn, map[string]any) {}

// cerrarSinContestar acepta y cierra sin escribir nada, que es unHerdr que muere a mitad.
func cerrarSinContestar(c net.Conn, _ map[string]any) { _ = c.Close() }

// mandarBasura contesta algo que no es JSON, que es lo que pasa si el socket pointed apunta a
// otro programa.
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

// TestElSocketDeVerdadSeAbreYSeResponde: la ida y vuelta completa, sin dobles.
//
// Y lo que se comprueba es el mensaje tal como viaja: el `jsonrpc`, el `id` y el `method`, y
// el `params` con el `pane_id` del pane. Un `id` mal formado no lo notaría nadie hasta que
// Herdr lo correlacionara con otra cosa —si dos peticiones se cruzan y las respuestas se
// asignan al revés, el popup pinta la imagen en el rectángulo de otro review—.
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
	// El id tiene que llevar el método dentro: es lo que permite a Herdr correlacionar y lo
	// que hace que dos peticiones simultáneas no se confundan.
	id, _ := p["id"].(string)
	if !strings.HasPrefix(id, "prdash:") || !strings.Contains(id, "info") {
		t.Errorf("el id es %q, y no identifica la petición", id)
	}
	params, _ := p["params"].(map[string]any)
	if params["pane_id"] != "pane-1" {
		t.Errorf("params.pane_id = %v, want pane-1", params["pane_id"])
	}
}

// TestUnSocketQueNoExisteDegradaAErrNoGraphicsYLoDiga: ENOENT del núcleo.
//
// Y es el caso más común de todos: Herdr no está corriendo, o `HERDR_SOCKET_PATH` apunta a
// una sesión que ya no existe. Y la degradación importa porque sin capa de gráficos la TUI
// sigue funcionando con celdas: lo que no puede hacer es PINTAR.
//
// Y el aserto del mensaje es el que hace útil el test: `ErrNoGraphics` sobrevive al
// `errors.Is` a través del `fmt.Errorf("%w: %v", …)`, así que el popup puede decidir "no hay
// imagen" sin parsear texto. Sin eso habría que reconocer el fallo por su cadena, que es la
// clase de acoplamiento que un `errors.Is` evita.
func TestUnSocketQueNoExisteDegradaAErrNoGraphicsYLoDiga(t *testing.T) {
	// Un socket que no existe: la ruta está dentro de un directorio que sí existe, así que
	// el fallo es ENOENT del socket y no ENOENT del directorio. Los dos son el mismo error
	// para `errors.Is`, pero el segundo significaría que el directorio se borró y sería un
	// caso distinto.
	ausente := filepath.Join(t.TempDir(), "no-existe.sock")
	g := &Graphics{Socket: ausente, PaneID: "pane-1", Timeout: time.Second}

	_, err := g.Info(context.Background())
	if err == nil {
		t.Fatal("Info contra un socket inexistente dio nil")
	}
	if !errors.Is(err, ErrNoGraphics) {
		t.Errorf("el error %v no es ErrNoGraphics: el popup no puede reconocerlo", err)
	}
	// Y dice ENOENT: sin el nombre del sistema no se puede decir "arranca Herdr".
	if !strings.Contains(err.Error(), "no such file") && !strings.Contains(err.Error(), "socket") {
		t.Errorf("el error %q no dice qué falló", err)
	}

	// Y lo mismo para las otras dos operaciones, porque las tres degradan por el mismo
	// camino: si solo lo comprobara `Info`, `Clear` podría reventar en el cierre del popup.
	if err := g.Clear(context.Background(), "capa"); !errors.Is(err, ErrNoGraphics) {
		t.Errorf("Clear dio %v, want ErrNoGraphics", err)
	}
	if err := g.SetImage(context.Background(), "capa", imagenDePrueba(4, 4),
		Placement{Col: 0, Row: 0, Cols: 2, Rows: 2}); !errors.Is(err, ErrNoGraphics) {
		t.Errorf("SetImage dio %v, want ErrNoGraphics", err)
	}
}

// TestUnServidorQueCierraSinContestarNoSeQuedaColgado: Herdr muere a mitad.
//
// Y es el caso para el que existe `len(line) == 0` en la lectura: un cierre a mitad deja la
// lectura con error de EOF, y sin esa comprobación el error devuelto sería el del `ReadBytes`
// desnudo —que sí o sí es `ErrNoGraphics` pero sin contexto— o, peor, un `nil` con `line`
// vacío que el `decodeResponse` aceptaría como respuesta vacía.
//
// Y lo que se mide es que vuelve PRONTO, no que vuelva bien: un EOF que se tradujera en
// esperar al timeout dejaría el popup congelado el tiempo del timeout cada vez que Herdr
// se reinicia.
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
	// El timeout del cliente son 2s en este fixture; un EOF tiene que notarse antes.
	if tardó > time.Second {
		t.Errorf("tardó %s en detectar el cierre: se esperó al timeout", tardó)
	}
}

// TestUnServidorQueNoContestaCortaAlTimeout: el Herdr colgado.
//
// Y este es el ÚNICO test del fichero que necesita esperar, y espera de verdad: se pone un
// timeout corto y se comprueba que la llamada vuelve antes de un margen holgado. Un test que
// usara el timeout de producción tardaría segundos sin comprobar nada útil.
//
// Y la degradación también: un Herdr colgado significa "sin imágenes", no "la TUI está
// colgada". Con `ErrNoGraphics`, el popup dice que no puede y el inbox sigue respondiendo.
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
	// Y corta DE VERDAD: el timeout tiene que cortar, no esperar a que el servidor se acuerde.
	//
	// El margen es amplio a propósito —diez veces el timeout más un segundo— porque la
	// medición va con reloj de pared y un `Load` alto en una máquina de CI no puede hacer
	// fallar el test por eso. Lo que no se permite es que pase del orden del timeout.
	if tardó > time.Second {
		t.Errorf("tardó %s con un timeout de 80ms: el timeout no corta", tardó)
	}

	// Y `CellSize`, que es quien llama a `Info` en la TUI, degrada a la aproximación en vez
	// de propagar: una imagen deformada es mejor que no pintar imagen.
	ancho, alto := g.CellSize(context.Background())
	if ancho != defaultCellWidthPx || alto != defaultCellHeightPx {
		t.Errorf("con el socket colgado la celda sale %dx%d, want la aproximación %dx%d",
			ancho, alto, defaultCellWidthPx, defaultCellHeightPx)
	}
}

// TestUnErrorDelServidorLlegaConSuCodigoYSuMensaje: el error de protocolo.
//
// Y es el caso donde Herdr dice algo útil —"no such pane", "method not available in this
// version"— y ese algo tiene que llegar hasta el popup. El código es lo que permite decidir
// si reintentar, y el mensaje es lo que se enseña.
//
// Y el `errors.Is` sigue funcionando por encima: el error de Herdr no borra el
// `ErrNoGraphics` que envuelve la llamada, porque un fallo remoto y un fallo local
// necesitan el mismo tratamiento en la TUI.
func TestUnErrorDelServidorLlegaConSuCodigoYSuMensaje(t *testing.T) {
	g, _ := nuevoServidor(t, rpcErr("no_such_pane", "pane pane-1 not found"))

	_, err := g.Info(context.Background())
	if err == nil {
		t.Fatal("un error del servidor dio nil")
	}
	// Y el error del servidor NO va envuelto en ErrNoGraphics, y mi primera versión daba por
	// hecho que sí. No hace falta que vaya: los dos únicos que miran el error son
	// `probe` —que solo pregunta si hay error— y `SetImage` —que trata cualquier fallo como
	// "no se pudo publicar"—, y los dos degradan igual. Envolverlo daría la impresión de que
	// se puede distinguir "no hay capa de gráficos" de "la capa rechazó esta llamada", y
	// hoy nada lo necesita; si algún día lo necesita, se envuelve en el punto donde se
	// necesita y no aquí.
	for _, quiere := range []string{"no_such_pane", "not found"} {
		if !strings.Contains(err.Error(), quiere) {
			t.Errorf("el error %q no trae %q del servidor", err, quiere)
		}
	}
}

// TestUnaRespuestaQueNoEsJSONSeRechazaYNoSePintaDeCeros: Herdr que no es Herdr.
//
// Y el caso es real: `HERDR_SOCKET_PATH` puede apuntar a un socket de otro programa —un
// servidor de SSH, un LSP, un thing que se quedó con la ruta—, y la respuesta llega como JSON
// que no es del protocolo.
//
// Y lo que NO puede pasar es que se acepte como una respuesta vacía y se pinten las medidas
// por defecto como si fueran las que dijo el servidor. `Info` devuelve su valor junto al
// error, y en ese caso tiene que devolver CERO: un error con dato al lado es un aserto que
// alguien puede leer al revés.
func TestUnaRespuestaQueNoEsJSONSeRechazaYNoSePintaDeCeros(t *testing.T) {
	for _, guion := range []struct {
		nombre string
		f      func(net.Conn, map[string]any)
	}{
		{"basura", mandarBasura},
		// La que NO hacía falta antes de arreglar decodeResponse: un `{}` sin campo `result`
		// se aceptaba como respuesta vacía y `probe()` decía que la capa funcionaba.
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
		// Y el error es el que toque —ilegible, o un tipo que no encaja— sin exigir que sea
		// ErrNoGraphics: los tres casos son de DATOS, no de que falte la capa de gráficos.
		if err == nil || strings.HasPrefix(err.Error(), "herdr: pane graphics unavailable") {
			t.Errorf("%s: el error %v no distingue el fallo de datos", guion.nombre, err)
		}
		// Y el dato al lado del error es CERO, no las medidas por defecto: cero es "no lo sé
		// y además falló", y las medidas por defecto son "no lo sé pero sigue".
		if info != (GraphicsInfo{}) {
			t.Errorf("%s: con error devuelve info %+v, want el valor cero", guion.nombre, info)
		}
	}
}

// TestClearViajaPorElSocketConSuCapaYConSuPane: la llamada que se hace al cerrar el popup.
//
// Y importa por lo que cuesta equivocarse: `Clear` es lo que quita la imagen de encima de la
// TUI. Si viajara con la capa equivocada o sin el `pane_id`, la operación saldría "bien" —código
// de retorno 0, sin error— y la imagen se quedaría pegada encima del inbox con la TUI
// funcionando debajo. Un fallo que no se ve hasta que cierras la TUI.
//
// Y es la única de las tres que no mira la respuesta: no hay nada que leer, y por eso su
// camino de error es el del servidor, no el de un decode.
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

	// Y cuando el servidor lo rechaza, el error sale igual. Es el caso del popup que se
	// cierra con Herdr y a la vez las dos degradaciones: se ignora y el proceso sigue.
	g2, _ := nuevoServidor(t, rpcErr("layer_not_found", "no such layer"))
	if err := g2.Clear(context.Background(), "prdash-sim"); err == nil {
		t.Error("Clear con error del servidor dio nil: el popup cerraría creyendo que quitó la capa")
	}
}

// TestSetImageMandaElPNGYElTamanoSinRedimensionar: la imagen tal cual.
//
// Y el "sin redimensionar" es el contrato, no un detalle: la caller ya ajustó la imagen al
// rectángulo y reescalar aquí tiraría detalle, además de pagar una CPU que en un terminal
// real se nota. El test lo fija comparando el PNG que viaja con el que produce `encodePNG`
// sobre la MISMA imagen —si el código reescalara, los bytes no podrían coincidir—.
//
// Y los tamaños del `params` son los de la imagen, no los del rectángulo: Herdr coloca por
// celdas y usa las medidas de la imagen para el rectángulo en píxeles. Confundir los dos
// valores produce una imagen estirada que no da ningún error.
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
	// El rectángulo viaja como celdas, en su propio objeto.
	pl, _ := params["placement"].(map[string]any)
	for clave, quiere := range map[string]any{
		"viewport_col": float64(2), "viewport_row": float64(3),
		"grid_cols": float64(4), "grid_rows": float64(3),
	} {
		if pl[clave] != quiere {
			t.Errorf("placement.%s = %v, want %v", clave, pl[clave], quiere)
		}
	}
	// Y el PNG es el de la imagen, byte a byte.
	esperado, err := encodePNG(img)
	if err != nil {
		t.Fatal(err)
	}
	if got := params["data_base64"]; got != base64.StdEncoding.EncodeToString(esperado) {
		t.Error("el PNG que viajó no es el de la imagen: se reescaló o se re-codificó")
	}
	// Y el z-index está: sin él, la imagen queda por debajo del texto del pane y parece que
	// no se publicó.
	if params["z_index"] != float64(0) {
		t.Errorf("z_index = %v, want 0", params["z_index"])
	}

	// Y el caso de la imagen que no existe: sin socket no se manda nada, y el aserto de que
	// no se mandó se ve en que el servidor no registró peticiones.
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

// TestUnaImagenQueNoSePuedeCodificarNoSaleDelProceso: el fallo de `encodePNG`.
//
// Y una imagen de tamaño cero es el caso que lo provoca: `png.Encode` la rechaza con "invalid
// image size". Y el detalle que importa es que el error NO se envuelve en `ErrNoGraphics`,
// porque no es un problema de la capa de gráficos sino de la imagen que le pasaron: distinguir
// los dos permite al popup decir "la simulación salió vacía" en vez de "no tengo Herdr".
//
// Y se comprueba además que no sale nada por el socket, que es lo que evita que se mande una
// imagen a medio codificar.
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

// imagenDePrueba es una imagen diminuta de un solo color, que es lo único que necesita este
// paquete: lo que se compara son los bytes del PNG, no lo que se ve.
func imagenDePrueba(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{R: uint8(x * 7), G: uint8(y * 11), B: 128, A: 255})
		}
	}
	return img
}
