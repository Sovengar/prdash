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

func TestSimulateEndToEndThroughThePaneLayer(t *testing.T) {
	if os.Getenv("HERDR_ENV") != "1" {
		t.Skip("outside Herdr there is no graphics layer")
	}
	if !sim.NewRunner().Available() {
		t.Skip("git-sim is not installed")
	}

	place, it := simFixture(t)
	svc := sim.New(realLocator{place: place})
	svc.CacheDir = t.TempDir()

	g := herdr.NewGraphics()
	if !g.Available() {
		t.Skip("Herdr answers but with no graphics layer (terminal.kitty_graphics?)")
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
		t.Fatalf("the real simulation failed: %v", msg.err)
	}
	if !m.sim.viaGraphics {
		t.Fatalf("the image never reached the pane layer: the popup would use half-blocks.\n"+
			"cells pintadas: %d", len(m.sim.cells))
	}

	if m.sim.cellW_px <= 1 || m.sim.cellH_px <= 2 {
		t.Errorf("cell = %dx%d, want the panes measure (9x19 in kitty)", m.sim.cellW_px, m.sim.cellH_px)
	}

	view := viewText(m)
	if !strings.Contains(view, "simulate: merge") {
		t.Errorf("the popup lost its title:\n%s", view)
	}
	if !strings.Contains(view, "o open image") {
		t.Errorf("the popup lost its help:\n%s", view)
	}

	m = press(t, m, "esc")
	if m.sim.state != simClosed {
		t.Errorf("state = %v, want cerrado", m.sim.state)
	}
}
