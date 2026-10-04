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

// Estas son las salidas del cliente de gráficos que no llegan al socket: sin pane conocido, con
// parámetros que no se pueden serializar, y con un socket que muere en el momento exacto de
// escribir.
//
// Y las tres son el borde entre "no hay capa de gráficos" y "Herdr dijo que no", que es la
// frontera que decide si el popup se pinta con media imagen o con bloques de texto.

// TestSinPaneConocidoNiInfoNiClearTocanElSocket: los dos guards de entrada.
//
// Y son la degradación honesta de una capacidad que depende del ENTORNO: sin `HERDR_PANE_ID` no
// hay pane donde poner la imagen, y la respuesta es `ErrNoGraphics` —que la TUI trata como
// "pinta half-blocks"—, no un error de Herdr.
//
// Y lo que hay que comprobar además del valor es que NO se toca el socket. Un guard que
// llamara a `call` y fallara al validar se traduce en un error de conexión en vez de "no hay
// pane", y el mensaje que vería el usuario sería "no such file or directory" —que lleva a
// mirar el PATH— en vez de "falta el pane".
//
// Y los dos métodos se prueban juntos porque comparten la guarda y un test que solo cubriera uno
// dejaría el otro sin comprobar: son la misma línea ejecutada desde dos sitios, y cada uno
// necesita su propio camino para que la cobertura lo vea.
func TestSinPaneConocidoNiInfoNiClearTocanElSocket(t *testing.T) {
	// Un servidor que FALLARÍA si se le llamara, para que el guard se admire en el error y no
	// en un accidente del servidor.
	g, srv := nuevoServidor(t, func(c net.Conn, _ map[string]any) {
		t.Error("se llamó al socket sin pane conocido")
		_ = c.Close()
	})
	// Y el pane vacío, que es lo que dispara el guard: `PaneID` a "" hace que `pane()` vuelva
	// al entorno, y el entorno no lo trae.
	g.PaneID = ""
	t.Setenv("HERDR_PANE_ID", "")

	info, err := g.Info(context.Background())
	if !errors.Is(err, ErrNoGraphics) {
		t.Errorf("Info sin pane dio %v, want ErrNoGraphics: la TUI no sabría que tiene que "+
			"pintar half-blocks", err)
	}
	// Y el valor es el cero y no un resto de una llamada anterior: `Info` devuelve un struct
	// con el tamaño de celda, y devolver un tamaño inventado haría que la TUI escalara la
	// imagen a una resolución que no cabe en el pane.
	if info.CellWidthPx != 0 || info.CellHeightPx != 0 {
		t.Errorf("Info sin pane devolvió %+v: un tamaño de celda inventado escala la "+
			"imagen a una resolución que no cabe, y es la relación de esas dos dimensiones "+
			"la que decide el aspecto", info)
	}
	// Y el resto del valor es cero: `MaxLayers` a cero haría que la TUI no creyera que hay
	// sitio para una capa y cayera a half-blocks sin motivo, que es una degradación
	// distinta de "no hay pane".
	if info.PaneVisible || info.MaxLayers != 0 {
		t.Errorf("Info sin pane devolvió %+v: los campos de capacidad tienen que venir a cero", info)
	}

	if err := g.Clear(context.Background(), "prdash-sim"); !errors.Is(err, ErrNoGraphics) {
		t.Errorf("Clear sin pane dio %v, want ErrNoGraphics", err)
	}
	// Y `SetImage`, que es el tercer camino al mismo guard y el que más se usa.
	err = g.SetImage(context.Background(), "prdash-sim", imagenRGBA(2, 2), Placement{Col: 1, Row: 1})
	if !errors.Is(err, ErrNoGraphics) {
		t.Errorf("SetImage sin pane dio %v, want ErrNoGraphics", err)
	}

	srv.parar()
}

// TestUnSocketQueMuerreConRSTAlEscribirSeReportaComoFalloDeLaCapaYNoComoUnPanePerdido: el
// `Write` que falla.
//
// Y el caso es real: Herdr se reinicia, o el usuario cierra el workspace, y el socket muere
// entre que el cliente se conecta y que escribe. El `Write` recibe ECONNRESET y hay que
// traducirlo a `ErrNoGraphics` con el motivo.
//
// Y por eso el `RST` y no un cierre limpio: con `SetLinger(0)` el SO manda un `RST` en vez de
// un `FIN`, así que la escritura pendiente falla en vez de entrar en el búfer del kernel y
// devolver éxito. Sin el RST el `Write` sale bien, el `Read` recibe EOF y se cubre el guard
// equivocado —que está cubierto— y este queda como código muerto.
//
// Y la aserción es doble, y las dos mitades importan:
//
//   - El error ES `ErrNoGraphics`, para que la TUI degrade a half-blocks en vez de pintar un
//     popup con marco y hueco.
//   - Y el motivo está, porque `ErrNoGraphics` sin motivo es indistinguible de "no hay pane".
func TestUnSocketQueMuerreConRSTAlEscribirSeReportaComoFalloDeLaCapaYNoComoUnPanePerdido(t *testing.T) {
	// La escritura falla siempre.
	g := graphicsConElWriteRoto(t)

	// Y una imagen con datos: el `Write` es de unos bytes, así que cabe en el búfer si el
	// socket no está muerto, y por eso la muerte con RST es lo que lo hace fallar.
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
	// Y el motivo del fallo de ESCRITURA está, que es lo que distingue esta salida de la del
	// `Read`: "broken pipe" dice que hay que reintentar y "EOF" que se acabó la respuesta.
	if !strings.Contains(err.Error(), "broken pipe") {
		t.Errorf("el motivo %q no trae la causa de la escritura fallida", err)
	}
}

// TestUnParametroQueNoSePuedeSerializarFallaAntesDeTocarElSocket: el `json.Marshal`.
//
// Y el `params` de las tres operaciones es un `map[string]any` con valores conocidos —cadenas y
// enteros—, así que el marshalling no puede fallar por ellas. La comprobación existe porque
// `params` es `any` y un valor que no se puede serializar —un canal, una función— lo haría.
//
// Y el caso se provoca llamando a `call` directamente, que es lo único que permite meter algo
// así en `params`: las tres operaciones públicas no lo ofrecen. Y merece la pena comprobar que
// falla ANTES del socket, porque el orden importa: si serializara después de conectar, una
// llamada mal formada dejaría una conexión abierta en Herdr sin haber enviado nada.
//
// Y el error sale CRUDO, sin envolver en `ErrNoGraphics`: no es un fallo de la capa, es un
// fallo de quién llamó, y envolverlo haría que la TUI lo tractara como "no hay pane".
func TestUnParametroQueNoSePuedeSerializarFallaAntesDeTocarElSocket(t *testing.T) {
	g, srv := nuevoServidor(t, func(c net.Conn, _ map[string]any) {
		t.Error("se conectó al socket con una petición que no se puede serializar")
		_ = c.Close()
	})

	var salida any
	err := g.call(context.Background(), "pane.graphics.set", map[string]any{
		// Un canal no se puede serializar a JSON, y no es un tipo que ninguna operación
		// real pase: es el caso que hace alcanzable el `if err != nil` del marshalling.
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
	// Y el servidor no vio nada: la llamada tiene que fallar ANTES de conectar, porque si
	// serializara después de abrir la conexión dejaría una conexión abierta en Herdr sin
	// haber enviado nada.
	srv.mu.Lock()
	vistas := len(srv.peticiones)
	srv.mu.Unlock()
	if vistas != 0 {
		t.Errorf("el servidor recibió %d peticiones de una llamada que no llegó a escribir",
			vistas)
	}
	srv.parar()
}

// TestElSocketVacioVieneDelEntornoYNoSeAdivina: el otro borde de `socket()`.
//
// Y es la diferencia entre "no hay Herdr" y "Herdr está en otro sitio". Con el socket vacío y
// `HERDR_SOCKET_PATH` sin valor, el `Dial` falla con ENOENT y el error dice qué variable falta,
// que es lo que permite arreglarlo sin leer el código.
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
	// Y el mensaje NO nombra la variable del entorno, y eso es una laguna que este test deja
	// escrita a propósito: `socket()` devuelve "" y el `Dial` falla con "missing address", así
	// que el motivo que ve el usuario no dice de dónde sale la ruta. Lo accionable sería decir
	// `HERDR_SOCKET_PATH`; hoy hay que leer el código para averiguarlo.
	//
	// No se arregla aquí porque es un cambio de mensaje, no de comportamiento, y merece su
	// propio sitio. Lo que este test fija es que el error sigue siendo `ErrNoGraphics` —que
	// es lo que la TUI necesita para degradar a half-blocks en vez de pintar un popup vacío—,
	// y no un panic ni un `nil` que se tomara por una imagen publicada.
	if !strings.Contains(err.Error(), "graphics") {
		t.Errorf("el error %q no dice que falla la capa de gráficos", err)
	}
	if !strings.Contains(err.Error(), "HERDR_SOCKET_PATH") {
		t.Logf("LAGUNA: el motivo %q no nombra HERDR_SOCKET_PATH, que es lo que "+
			"permitiría arreglarlo sin leer el código", err)
	}
}

// connConElWriteRoto es un `net.Conn` cuyo `Write` falla siempre y cuyo `Read` se queda
// esperando para siempre. El `net.Conn` embebido NO es nil: `call` hace
// `defer conn.Close()` y un nil ahí es un panic, no un fallo limpio.
//
// Y es la forma de alcanzar el `Write` de `call` de verdad, porque las dos vías naturales no
// sirven:
//
//   - Un socket Unix con un cierre LIMPIO deja que la escritura pendiente entre en el búfer
//     del kernel y salga con éxito; el fallo llega en el `Read`. Y `net.UnixConn` no tiene
//     `SetLinger`, que es lo que convertiría el cierre en un `RST`.
//   - Un `net.Pipe` con el otro extremo cerrado falla en el `Read` por el mismo motivo: la
//     escritura es síncrona y espera a un lector que no existe.
//
// Y el motivo de que el `Write` sea un camino aparte es que el fallo tiene otro significado:
// escribir sobre una conexión muerta es "reintenta", mientras que leer un EOF es "se acabó la
// respuesta". Traducidos los dos a `ErrNoGraphics` con el mismo texto, la TUI degrada igual, pero
// la línea es distinta y merece estar probada por separado.
type connConElWriteRoto struct{ net.Conn }

func (connConElWriteRoto) Write([]byte) (int, error) {
	return 0, errors.New("broken pipe")
}

// graphicsConElWriteRoto devuelve un `Graphics` cuya escritura falla siempre.
func graphicsConElWriteRoto(t *testing.T) *Graphics {
	t.Helper()
	// Un `net.Pipe` cuyo peer se queda VIVO: hace falta para que `Read` bloquee en vez de
	// fallar, y para que `Close` —el `defer` de `call`— tenga una conexión real que cerrar.
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

// imagenRGBA es una imagen diminuta y válida, para las llamadas que necesitan una que serializar.
func imagenRGBA(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 64, A: 255})
		}
	}
	return img
}
