package herdr

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestCallSinRespuestaEsUnErrorDeGraficos: cuando el socket se cierra sin contestar,
// la petición se declara fallida en vez de intentar descodificar la nada.
//
// El borde es que hay algo leído o no hay nada. Con una línea entera, da igual que
// venga con error de lectura: la línea es la respuesta y se usa. Con cero bytes, no
// hay respuesta que usar, y meter un error vacío en el descodificador daría un fallo
// de JSON que no dice nada de qué pasó realmente.
//
// Y el caso de "media línea sin salto" es el que de verdad importa: el servidor se
// murió a mitad. Ahí SÍ hay algo, y no es poco, así que el error de lectura tiene que
// llegar al usuario en vez de tragarse el error y decir "JSON inválido". Omitir el
// texto ya escrito sería tirar el único dato que hay.
func TestCallSinRespuestaEsUnErrorDeGraficos(t *testing.T) {
	// El servidor cierra sin decir nada.
	silencioso := &fakeSocket{silent: true}
	g := newTestGraphics(t, silencioso)
	err := g.call(context.Background(), "pane.graphics.info", nil, nil)
	if !errors.Is(err, ErrNoGraphics) {
		t.Errorf("con el socket cerrado en silencio dio %v, want ErrNoGraphics", err)
	}

	// El servidor manda media respuesta y se muere. NO es ErrNoGraphics, y esa es la
	// diferencia que importa: algo llegó, así que lo que se rompe es el protocolo, no
	// la disponibilidad de la capa. Decir "no hay capa de gráficos" cuando lo que pasó
	// es que el servidor se calló a mitad sería mandar al usuario a mirar un sitio
	// donde no está el problema.
	medio := &fakeSocket{halfLine: true, response: `{"id":"x","result":{"cell_width_px":9}}`}
	g = newTestGraphics(t, medio)
	err = g.call(context.Background(), "pane.graphics.info", nil, nil)
	if err == nil {
		t.Error("con media respuesta dio nil, want un error: una respuesta cortada no es una respuesta")
	}
	if errors.Is(err, ErrNoGraphics) {
		t.Errorf("con media respuesta dio ErrNoGraphics: algo llegó, así que el fallo es de protocolo (%v)", err)
	}

	// Y una respuesta entera funciona, que es el caso bueno y el que no hay que
	// romper. Es la otra mitad del borde: con línea, se usa la línea.
	entero := &fakeSocket{response: graphicsInfoJSON(9, 19, true)}
	g = newTestGraphics(t, entero)
	info, err := g.Info(context.Background())
	if err != nil {
		t.Fatalf("con una respuesta entera dio %v, want nil", err)
	}
	if info.CellWidthPx != 9 || info.CellHeightPx != 19 {
		t.Errorf("descodificó %dx%d, want 9x19", info.CellWidthPx, info.CellHeightPx)
	}
}

// TestCallUsaElPlazoPedidoYElDePorDefecto: el plazo viaja en el contexto que se pasa
// al dial, así que se puede mirar sin esperar.
//
// Y el punto es el otro: un plazo no positivo se sustituye por el de por defecto.
// Con un plazo de cero, la petición se cortaría ANTES de enviarse, y el socket vería
// una conexión que se abre y se cierra sin escribir. Eso se lee como "Herdr no
// responde" en vez de "el cliente no mandó nada", que son fallos opuestos con la misma
// causa.
func TestCallUsaElPlazoPedidoYElDePorDefecto(t *testing.T) {
	for _, c := range []struct {
		nombre  string
		timeout time.Duration
	}{
		{"el que viene", 3 * time.Second},
		{"cero", 0},
		{"negativo", -time.Second},
	} {
		f := &fakeSocket{response: graphicsInfoJSON(9, 19, true)}
		g := newTestGraphics(t, f)
		g.Timeout = c.timeout

		antes := time.Now()
		if err := g.call(context.Background(), "pane.graphics.info", nil, nil); err != nil {
			t.Fatalf("%s: dio %v", c.nombre, err)
		}
		if f.deadline.IsZero() {
			t.Errorf("%s: la conexión se abrió sin plazo: se perdería al primer bloqueo", c.nombre)
			continue
		}
		// El plazo tiene que estar en el futuro, o la petición se corta antes de
		// enviarse. Y tiene que estar cerca del pedido, o se pierde tiempo.
		if !f.deadline.After(antes) {
			t.Errorf("%s: el plazo ya estaba vencido (%v): la petición se corta antes de enviarse",
				c.nombre, f.deadline.Sub(antes))
		}
		if c.timeout > 0 {
			// Con plazo propio se respeta el suyo.
			if restante := f.deadline.Sub(antes); restante > c.timeout+time.Second {
				t.Errorf("%s: el plazo es de %v, want unos %v", c.nombre, restante, c.timeout)
			}
		}
	}
	// Y el de por defecto es el que entra cuando no hay plazo. Se comprueba con dos
	// timeouts que degradan al mismo: uno a cero y otro a menos, y el plazo que sale
	// es el de por defecto en los dos casos, no uno distinto cada vez.
	f1, f2 := &fakeSocket{response: graphicsInfoJSON(9, 19, true)}, &fakeSocket{response: graphicsInfoJSON(9, 19, true)}
	g1, g2 := newTestGraphics(t, f1), newTestGraphics(t, f2)
	g1.Timeout, g2.Timeout = 0, -time.Hour
	for i, g := range []*Graphics{g1, g2} {
		if err := g.call(context.Background(), "pane.graphics.info", nil, nil); err != nil {
			t.Fatalf("caso %d: %v", i, err)
		}
	}
	if f1.deadline.IsZero() || f2.deadline.IsZero() {
		t.Fatal("alguna petición salió sin plazo")
	}
	if dif := f1.deadline.Sub(f2.deadline); dif > time.Second || dif < -time.Second {
		t.Errorf("los plazos de por defecto difieren en %v: deberían ser el mismo", dif)
	}
	// Y ambos son el de por defecto de verdad, no un residuo del anterior.
	if graphicsTimeout <= 0 {
		t.Error("el plazo por defecto no es un plazo")
	}
}

// TestCallAbreUnaConexionPorPeticion: el servidor cierra después de responder, así
// que reutilizar la conexión solo produciría un error de tubería en la segunda
// llamada. Por eso es una por petición, y eso es observable contando conexiones.
func TestCallAbreUnaConexionPorPeticion(t *testing.T) {
	f := &fakeSocket{response: graphicsInfoJSON(9, 19, true)}
	g := newTestGraphics(t, f)

	for i := range 3 {
		if err := g.call(context.Background(), "pane.graphics.info", nil, nil); err != nil {
			t.Fatalf("petición %d: %v", i, err)
		}
	}
	if f.dials != 3 {
		t.Errorf("tres peticiones abrieron %d conexiones, want 3: el servidor cierra tras cada respuesta", f.dials)
	}
	if f.served != 3 {
		t.Errorf("el servidor atendió %d peticiones, want 3", f.served)
	}
}
