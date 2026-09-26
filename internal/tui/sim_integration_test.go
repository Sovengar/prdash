package tui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"prdash/internal/forge/model"
	"prdash/internal/sim"
	"prdash/internal/testutil"
)

// realLocator apunta al repo de un fixture de git de verdad, que es lo que hace
// el cableado de producción con el review activo.
type realLocator struct{ place sim.Place }

func (l realLocator) Locate(model.Item) (sim.Place, bool) { return l.place, true }

// simFixture monta un repo con base y rama de PR, como el que deja el executor
// después de montar la review.
func simFixture(t *testing.T) (sim.Place, model.Item) {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	testutil.InitRepo(t, src)
	testutil.CommitFile(t, src, "f.txt", "base", "base")
	testutil.RunGit(t, src, "branch", "-M", "main")
	testutil.RunGit(t, src, "checkout", "-b", "prdash/pr-7")
	testutil.CommitFile(t, src, "f.txt", "pr", "pr")
	testutil.RunGit(t, src, "checkout", "--quiet", "main")

	bare := filepath.Join(dir, "remote.git")
	testutil.InitBare(t, bare)
	testutil.RunGit(t, src, "remote", "add", "origin", bare)
	testutil.Push(t, src, "origin", "main", "prdash/pr-7")

	it := model.NewItem(model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget"}, 7)
	it.SourceBranch = "feat/x"
	it.TargetBranch = "main"
	return sim.Place{Repo: bare, Branch: "prdash/pr-7"}, it
}

// waitSim entrega el resultado de una simulación como lo haría la bomba de
// eventos: se lee del canal hasta que llegue un simMsg.
func waitSim(t *testing.T, m Model) (Model, simMsg) {
	t.Helper()
	deadline := time.After(60 * time.Second)
	for {
		select {
		case ev := <-m.events:
			out, _ := m.Update(ev)
			m = out.(Model)
			if msg, ok := ev.(simMsg); ok {
				return m, msg
			}
		case <-deadline:
			t.Fatal("la simulación no entregó ningún resultado")
		}
	}
}

// TestSimulateEndToEndWithRealGitSim recorre la cadena entera con la herramienta
// de verdad: la tecla, el popup, el clon temporal, el render, el JPEG y las
// celdas en la vista. Las piezas sueltas ya tienen tests; lo que esto comprueba es
// que encajan, que es justo lo que ningún doble puede demostrar.
func TestSimulateEndToEndWithRealGitSim(t *testing.T) {
	if !sim.NewRunner().Available() {
		t.Skip("git-sim no está instalado")
	}
	place, it := simFixture(t)

	svc := sim.New(realLocator{place: place})
	svc.CacheDir = t.TempDir()

	m := newTestModel(t, ghAdapter())
	m.SetSimulator(svc)
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{it}, false))

	m = press(t, m, "v")
	if m.sim.state != simChoosing {
		t.Fatalf("state = %v, want simChoosing", m.sim.state)
	}
	m = press(t, m, "enter")
	if m.sim.state != simRendering {
		t.Fatalf("state = %v, want simRendering", m.sim.state)
	}

	m, msg := waitSim(t, m)
	if msg.err != nil {
		t.Fatalf("la simulación real falló: %v", msg.err)
	}
	if m.sim.state != simShowing {
		t.Fatalf("state = %v, want simShowing", m.sim.state)
	}

	// La imagen del render tiene que estar en pantalla, y el fondo detrás.
	view := viewText(m)
	if !strings.Contains(view, "simulate: merge") {
		t.Errorf("la vista no muestra el título del popup:\n%s", view)
	}
	if !strings.Contains(view, "Inbox") {
		t.Error("el popup tapó la vista de fondo")
	}
	if len(m.sim.cells) != m.sim.cellH {
		t.Errorf("celdas = %d líneas, want %d", len(m.sim.cells), m.sim.cellH)
	}
	// Las celdas van al centro de la caja, no en la primera línea: si el overlay
	// no calculó bien la posición, el grafo saldría pegado al borde superior.
	block := strings.Index(view, "▀")
	if block < 0 {
		t.Fatalf("la vista no contiene ni un half-block:\n%s", view)
	}
	if row := strings.Count(view[:block], "\n"); row < 2 {
		t.Errorf("el popup se pegó al borde superior (fila %d)", row)
	}

	// Cerrar devuelve la vista a su estado normal.
	m = press(t, m, "esc")
	if m.sim.state != simClosed {
		t.Errorf("state = %v, want cerrado", m.sim.state)
	}
	if strings.Contains(viewText(m), "simulate: merge") {
		t.Error("el popup siguió en la vista tras cerrarlo")
	}
}

// TestSimulateOverAnArmedMergeDoesNotOpen: la `v` con el merge armado desarma y se
// consume, como cualquier otra tecla. Es lo que evita que una pulsación a
// destiempo abra un modal encima de una Confirmación.
func TestSimulateOverAnArmedMergeDoesNotOpen(t *testing.T) {
	place, it := simFixture(t)
	m := newTestModel(t, ghAdapter())
	m.SetSimulator(sim.New(realLocator{place: place}))
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{it}, false))

	m = press(t, m, "m")
	m = press(t, m, "v")

	if m.mergeArmed {
		t.Error("el merge sigue armado")
	}
	if m.sim.state != simClosed {
		t.Errorf("state = %v, want cerrado", m.sim.state)
	}
}

// simAvailableInPath documenta la dependencia sin la que la acción solo avisa: si
// esto dejara de ser cierto, Available() mentiría y el popup se abriría para nada.
func TestSimulatorAvailabilityFollowsTheBinary(t *testing.T) {
	place, _ := simFixture(t)
	svc := sim.New(realLocator{place: place})
	if got := svc.Available(); got != sim.NewRunner().Available() {
		t.Errorf("Available() = %v, no coincide con el runner", got)
	}
}
