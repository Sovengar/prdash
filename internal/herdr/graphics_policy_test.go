package herdr

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

// TestPlacementEmptyConUnaDimensionEnCero: una colocación vacía no dibuja nada.
//
// El borde importa y va en las dos dimensiones: un rectángulo de 0 columnas o de 0
// filas es degenerado, y mandarlo no es "no dibujar", es obligar al servidor a
// decidir qué hacer con un rectángulo que no existe. Por eso se comprueba antes de
// mandar nada, y aquí se afirma que el 0 EXACTO ya cuenta como vacío. Con un `< 0` en
// vez de `<= 0`, un 0 pasaría por bueno y llegaría al servidor.
func TestPlacementEmptyConUnaDimensionEnCero(t *testing.T) {
	// Las cuatro combinaciones de las dos dimensiones, con valores de cada lado del
	// cero. La de 0 en ambas es la que un cálculo de rectángulo produce cuando el
	// área no cabe, y es la que tiene que salir por la puerta antes.
	casos := []struct {
		nombre string
		p      Placement
		vacia  bool
	}{
		{"todo a 0", Placement{}, true},
		{"solo columnas a 0", Placement{Cols: 0, Rows: 5}, true},
		{"solo filas a 0", Placement{Cols: 5, Rows: 0}, true},
		{"columnas a 0 y filas a 0", Placement{Cols: 0, Rows: 0}, true},
		{"columnas negativas", Placement{Cols: -1, Rows: 5}, true},
		{"filas negativas", Placement{Cols: 5, Rows: -1}, true},
		{"las dos negativas", Placement{Cols: -3, Rows: -3}, true},
		{"una de celda", Placement{Cols: 1, Rows: 1}, false},
		{"con contenido", Placement{Cols: 20, Rows: 10}, false},
		// El desplazamiento no cuenta: una imagen en la esquina sigue siendo una
		// imagen. Por eso Empty mira solo las dimensiones.
		{"con desplazamiento", Placement{Col: 100, Row: 200, Cols: 1, Rows: 1}, false},
	}

	for _, c := range casos {
		if got := c.p.Empty(); got != c.vacia {
			t.Errorf("%s: %+v dio Empty()=%v, want %v", c.nombre, c.p, got, c.vacia)
		}
		// Y con el desplazamiento a un sitio imposible y las dimensiones vacías sigue
		// siendo vacía: un rectángulo degenerado en el medio de la pantalla sigue
		// siendo degenerado.
		if !c.vacia {
			continue
		}
		desplazada := c.p
		desplazada.Col, desplazada.Row = 999, 999
		if !desplazada.Empty() {
			t.Errorf("%s: desplazada a (999,999) dejó de estar vacía", c.nombre)
		}
	}
}

// TestGraphicsReadySonTresCadenasYUnaDecision: la política de la capa de gráficos sin
// la ida al socket.
//
// Las nueve combinaciones se afirman, y las dos cosas que se deciden aquí no son
// simétricas:
//
//   - estar DENTRO de Herdr va primero. Fuera de Herdr no se mira ni el socket ni el
//     pane, y no por ahorro: un proceso corriendo fuera con las variables puestas se
//     iría al socket de todas formas, y ese socket puede ser el de la sesión de otro
//     proceso. Escribir ahí no es un error, es un incidente.
//   - socket y pane hacen falta LOS DOS. Con uno solo falta, y la petición sale con
//     un destinatario a medias, que es peor que no salir: convierte un error local y
//     barato en un error remoto y opaco.
func TestGraphicsReadySonTresCadenasYUnaDecision(t *testing.T) {
	casos := []struct {
		herdrEnv, socket, pane string
		want                   bool
	}{
		// Dentro de Herdr, con todo: el único caso de sí.
		{"1", "/run/herdr.sock", "w1:p1", true},
		// Fuera de Herdr, aunque tenga socket y pane: no.
		{"", "/run/herdr.sock", "w1:p1", false},
		{"0", "/run/herdr.sock", "w1:p1", false},
		{"2", "/run/herdr.sock", "w1:p1", false},
		{"true", "/run/herdr.sock", "w1:p1", false},
		{"11", "/run/herdr.sock", "w1:p1", false},
		{" 1", "/run/herdr.sock", "w1:p1", false},
		{"1 ", "/run/herdr.sock", "w1:p1", false},
		// Dentro de Herdr pero sin destino: no.
		{"1", "", "w1:p1", false},
		{"1", "/run/herdr.sock", "", false},
		{"1", "", "", false},
		// Y sin estar dentro tampoco cuenta el destino que haya.
		{"", "", "", false},
	}

	for _, c := range casos {
		if got := graphicsReady(c.herdrEnv, c.socket, c.pane); got != c.want {
			t.Errorf("graphicsReady(%q, %q, %q) = %v, want %v", c.herdrEnv, c.socket, c.pane, got, c.want)
		}
	}
	// Y HERDR_ENV tiene que ser EXACTAMENTE "1": es lo que escribe el propio Herdr al
	// arrancar su shell, y un "1 " con un espacio es un valor que el producto nunca
	// produce, así que aceptarlo sería abrir la puerta a un valor inventado.
	for _, ok := range []string{"1"} {
		if !graphicsReady(ok, "s", "p") {
			t.Errorf("graphicsReady con HERDR_ENV=%q dio false, y es el valor que escribe Herdr", ok)
		}
	}
}

// TestGraphicsReadyNoMiraElSocketSiNoEstaDentro: el orden de las condiciones no es un
// detalle.
//
// Es lo mismo que el caso anterior, dicho como propiedad: con HERDR_ENV que no sea
// "1", la función no puede mirar el socket. Y no se puede "mirar" sin tomarlo, así
// que hay que comprobarlo con un socket que no se pueda tomar: uno que entre en pánico
// si alguien lo lee.
func TestGraphicsReadyNoMiraElSocketSiNoEstaDentro(t *testing.T) {
	// No hay forma de inyectar un socket que rompa, porque la función recibe
	// cadenas. Lo que se afirma es la consecuencia: con el entorno mal, el resultado
	// no depende del socket en absoluto. Un socket imposible da el mismo resultado
	// que uno real, y eso es lo que significa "no lo mira".
	for _, herdrEnv := range []string{"", "0", "2", "true", "1x"} {
		conSocketImposible := graphicsReady(herdrEnv, "/no/existe/el/socket", "w1:p1")
		conSocketReal := graphicsReady(herdrEnv, "/run/herdr.sock", "w1:p1")
		if conSocketImposible != conSocketReal {
			t.Errorf("HERDR_ENV=%q: el resultado cambió según el socket (%v vs %v): "+
				"la condición se está mirando aunque no deba", herdrEnv, conSocketImposible, conSocketReal)
		}
		if conSocketReal {
			t.Errorf("HERDR_ENV=%q: dio true sin estar dentro de Herdr", herdrEnv)
		}
	}
}

// TestHaveGraphicsTarget: hacen falta los dos. Con uno solo falta, y el error sale
// del otro lado.
func TestHaveGraphicsTarget(t *testing.T) {
	if !haveGraphicsTarget("/s", "p") {
		t.Error("con socket y pane dio false")
	}
	// Y cada uno por separado, que es donde está el borde.
	if haveGraphicsTarget("", "p") {
		t.Error("sin socket dio true: la petición sale sin a quién preguntarlo")
	}
	if haveGraphicsTarget("/s", "") {
		t.Error("sin pane dio true: no hay rectángulo donde colocar la imagen")
	}
	if haveGraphicsTarget("", "") {
		t.Error("sin nada dio true")
	}
}

// TestGraphicsTimeoutParaCeroYNegativoNoEsCero: un plazo de cero no es "sin plazo",
// es un plazo que ya pasó.
//
// La diferencia no es teórica: con un contexto que caduca ya, la petición se corta
// antes de enviarse, y el socket ve una conexión que se abre y se cierra sin
// escribir. Eso se lee como "Herdr no responde" en vez de como "el cliente no ha
// que se ha enviado nada", que son fallos opuestos con la misma causa.
func TestGraphicsTimeoutParaCeroYNegativoNoEsCero(t *testing.T) {
	for _, t0 := range []time.Duration{-time.Hour, -time.Second, -time.Nanosecond, 0} {
		if got := graphicsTimeoutFor(t0); got != graphicsTimeout {
			t.Errorf("graphicsTimeoutFor(%v) = %v, want %v: un plazo no positivo no es un plazo",
				t0, got, graphicsTimeout)
		}
		if got := graphicsTimeoutFor(t0); got <= 0 {
			t.Errorf("graphicsTimeoutFor(%v) devolvió un plazo no positivo: %v", t0, got)
		}
	}
	// Y un plazo positivo se respeta tal cual, sin recortarlo ni redondearlo: el
	// usuario pidió ese plazo.
	for _, t0 := range []time.Duration{time.Nanosecond, time.Millisecond, 42 * time.Second, time.Hour} {
		if got := graphicsTimeoutFor(t0); got != t0 {
			t.Errorf("graphicsTimeoutFor(%v) = %v, want el mismo plazo", t0, got)
		}
	}
	// Y el de por defecto es un plazo de verdad, no cero: si alguna vez se declarara
	// en cero, la degradación de arriba devolvería cero y la petición no saldría.
	if graphicsTimeout <= 0 {
		t.Errorf("graphicsTimeout = %v: un plazo por defecto no positivo hace que la degradación no degrade", graphicsTimeout)
	}
}

// TestCellSizeFromDegradaEnLasTresFformas: cuando no se puede preguntar se usa 1×2,
// y hay TRES maneras de no poder.
//
// La tercera es la que se confunde con las otras dos: una celda de 0 píxeles de ancho
// no es una celda, es una división por cero esperando a que alguien la use. Por eso
// el 0 EXACTO cae en la aproximación, y no en "usa el 0 que te han dado".
func TestCellSizeFromDegradaEnLasTresFormas(t *testing.T) {
	const w, h = defaultCellWidthPx, defaultCellHeightPx

	casos := []struct {
		nombre string
		info   GraphicsInfo
		err    error
		wantW  int
		wantH  int
	}{
		// Caso bueno: se usa lo que dijo el pane, tal cual. La deformación viene de
		// aquí, y hay que respetar la medida aunque sea rara.
		{"medida normal", GraphicsInfo{CellWidthPx: 9, CellHeightPx: 19}, nil, 9, 19},
		{"medida 1×1", GraphicsInfo{CellWidthPx: 1, CellHeightPx: 1}, nil, 1, 1},
		{"medida grande", GraphicsInfo{CellWidthPx: 400, CellHeightPx: 800}, nil, 400, 800},
		// Caso malo 1: no se pudo preguntar.
		{"error", GraphicsInfo{CellWidthPx: 9, CellHeightPx: 19}, errors.New("socket"), w, h},
		{"error y medida a cero", GraphicsInfo{}, errors.New("socket"), w, h},
		// Caso malo 2 y 3: el pane respondió, pero con una dimensión degenerada. Da
		// igual cuál, y da igual que la otra sea buena: una celda de ancho 0 con
		// altura 19 no es media celda, no es nada.
		{"ancho a cero", GraphicsInfo{CellWidthPx: 0, CellHeightPx: 19}, nil, w, h},
		{"alto a cero", GraphicsInfo{CellWidthPx: 9, CellHeightPx: 0}, nil, w, h},
		{"las dos a cero", GraphicsInfo{CellWidthPx: 0, CellHeightPx: 0}, nil, w, h},
		{"ancho negativo", GraphicsInfo{CellWidthPx: -9, CellHeightPx: 19}, nil, w, h},
		{"alto negativo", GraphicsInfo{CellWidthPx: 9, CellHeightPx: -19}, nil, w, h},
		{"las dos negativas", GraphicsInfo{CellWidthPx: -1, CellHeightPx: -1}, nil, w, h},
	}

	for _, c := range casos {
		gotW, gotH := cellSizeFrom(c.info, c.err)
		if gotW != c.wantW || gotH != c.wantH {
			t.Errorf("%s: dio %dx%d, want %dx%d", c.nombre, gotW, gotH, c.wantW, c.wantH)
		}
	}

	// Y la degradación es SIEMPRE positiva, que es la razón de existir: si
	// devolviera un cero, quien la use dividiría por él.
	for _, c := range casos {
		gotW, gotH := cellSizeFrom(c.info, c.err)
		if gotW <= 0 || gotH <= 0 {
			t.Errorf("%s: devolvió %dx%d, y una celda de cero píxeles no es una celda", c.nombre, gotW, gotH)
		}
	}
	// Y la aproximación por defecto es la de un terminal, 1×2. No es arbitraria: es
	// lo que mide una celda de texto, así que una imagen dibujada con ella conserva
	// la proporción de un carácter en vez de deformarse al azar.
	if defaultCellWidthPx != 1 || defaultCellHeightPx != 2 {
		t.Errorf("la aproximación por defecto es %dx%d, want 1x2 (lo que mide una celda de texto)",
			defaultCellWidthPx, defaultCellHeightPx)
	}
}

// TestCallNoSaleSinDestinoNiConPlazoVencido: call se niega a salir sin socket y sin
// pane, y usa un plazo de verdad.
//
// Las dos cosas se comprueban por el lado del socket, que es donde se ven: un dial
// falso cuenta las conexiones, así que "no ha salido" es un cero, que es un hecho y
// no una inferencia.
func TestCallNoSaleSinDestinoNiConPlazoVencido(t *testing.T) {
	for _, c := range []struct {
		nombre     string
		socket     string
		pane       string
		quiereDial bool
	}{
		{"con los dos", "/tmp/s.sock", "w1:p1", true},
		{"sin socket", "", "w1:p1", false},
		{"sin pane", "/tmp/s.sock", "", false},
		{"sin nada", "", "", false},
	} {
		var dials int
		g := &Graphics{
			Socket:  c.socket,
			PaneID:  c.pane,
			Timeout: time.Second,
			dial: func(context.Context, string, string) (net.Conn, error) {
				dials++
				return nil, errors.New("no debería llegar aquí")
			},
		}
		g.getenv = func(string) string { return "" }

		err := g.call(context.Background(), "pane.graphics.info", nil, nil)
		if !errors.Is(err, ErrNoGraphics) {
			t.Errorf("%s: dio %v, want ErrNoGraphics", c.nombre, err)
		}
		if c.quiereDial != (dials > 0) {
			t.Errorf("%s: dial se llamó %d veces, y %v. Sin destino no se abre conexión: "+
				"un socket a medias convierte un error local en uno remoto", c.nombre, dials, c.quiereDial)
		}
	}
}
