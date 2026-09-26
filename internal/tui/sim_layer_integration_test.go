package tui

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"prdash/internal/forge/model"
	"prdash/internal/herdr"
	"prdash/internal/sim"
)

// TestSimulateEndToEndThroughThePaneLayer recorre la cadena entera con las dos
// piezas de verdad: el socket de Herdr y el git-sim instalado. Las piezas sueltas ya
// tienen tests con dobles; lo que esto comprueba es que encajan y que Herdr acepta
// de verdad el mensaje, con su límite de tamaño y su formato.
//
// Deja pasar un momento una imagen real en el pane donde corre el test, y la quita
// al terminar. Se salta fuera de Herdr, donde la capa no existe.
func TestSimulateEndToEndThroughThePaneLayer(t *testing.T) {
	if os.Getenv("HERDR_ENV") != "1" {
		t.Skip("fuera de Herdr no hay capa de gráficos")
	}
	if !sim.NewRunner().Available() {
		t.Skip("git-sim no está instalado")
	}

	place, it := simFixture(t)
	svc := sim.New(realLocator{place: place})
	svc.CacheDir = t.TempDir()

	g := herdr.NewGraphics()
	if !g.Available() {
		t.Skip("Herdr responde pero sin capa de gráficos (terminal.kitty_graphics?)")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = g.Clear(ctx, simLayer)
	})

	m := newTestModel(t, ghAdapter())
	m.SetSimulator(svc)
	m.SetGraphics(g)
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{it}, false))
	t.Cleanup(m.closeSim)

	m = press(t, m, "v")
	if m.sim.state != simChoosing {
		t.Fatalf("state = %v, want simChoosing", m.sim.state)
	}
	m = press(t, m, "enter")

	m, msg := waitSim(t, m)
	if msg.err != nil {
		t.Fatalf("la simulación real falló: %v", msg.err)
	}
	if !m.sim.viaGraphics {
		t.Fatalf("la imagen no llegó a la capa del pane: el popup usaría half-blocks.\n"+
			"celdas pintadas: %d", len(m.sim.cells))
	}

	// El ratio de celda tiene que venir medido, no supuesto: es lo que hace que la
	// imagen no se deforme y no se mande más resolución de la que se ve.
	if m.sim.cellW_px <= 1 || m.sim.cellH_px <= 2 {
		t.Errorf("celda = %dx%d, want la medida del pane (9x19 en kitty)", m.sim.cellW_px, m.sim.cellH_px)
	}

	// El marco se sigue pintando: es lo que dice qué se está viendo y da la ayuda.
	view := viewText(m)
	if !strings.Contains(view, "simulate: merge") {
		t.Errorf("el popup perdió el título:\n%s", view)
	}
	if !strings.Contains(view, "o open image") {
		t.Errorf("el popup perdió la ayuda:\n%s", view)
	}

	// Cerrar tiene que quitar la capa, o la imagen se queda encima de la TUI.
	m = press(t, m, "esc")
	if m.sim.state != simClosed {
		t.Errorf("state = %v, want cerrado", m.sim.state)
	}
}
