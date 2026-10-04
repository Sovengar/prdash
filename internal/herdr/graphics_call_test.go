package herdr

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCallSinRespuestaEsUnErrorDeGraficos(t *testing.T) {
	// El servidor cierra sin decir nada.
	silencioso := &fakeSocket{silent: true}
	g := newTestGraphics(t, silencioso)
	err := g.call(context.Background(), "pane.graphics.info", nil, nil)
	if !errors.Is(err, ErrNoGraphics) {
		t.Errorf("con el socket cerrado en silencio dio %v, want ErrNoGraphics", err)
	}

	// The server sends half a reply and dies. NOT ErrNoGraphics, and that is the difference that matters:
	//something arrived, so the PROTOCOL is what broke.
	medio := &fakeSocket{halfLine: true, response: `{"id":"x","result":{"cell_width_px":9}}`}
	g = newTestGraphics(t, medio)
	err = g.call(context.Background(), "pane.graphics.info", nil, nil)
	if err == nil {
		t.Error("con media respuesta dio nil, want un error: una respuesta cortada no es una respuesta")
	}
	if errors.Is(err, ErrNoGraphics) {
		t.Errorf("con media respuesta dio ErrNoGraphics: algo llegó, así que el fallo es de protocolo (%v)", err)
	}

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

// The deadline travels in the context passed to dial, so it can be checked without waiting.
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
		// The deadline has to be in the future, or the request is cut before being sent.
		if !f.deadline.After(antes) {
			t.Errorf("%s: el plazo ya estaba vencido (%v): la petición se corta antes de enviarse",
				c.nombre, f.deadline.Sub(antes))
		}
		if c.timeout > 0 {
			if restante := f.deadline.Sub(antes); restante > c.timeout+time.Second {
				t.Errorf("%s: el plazo es de %v, want unos %v", c.nombre, restante, c.timeout)
			}
		}
	}
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
	if graphicsTimeout <= 0 {
		t.Error("el plazo por defecto no es un plazo")
	}
}

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
