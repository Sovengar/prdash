package herdr

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"prdash/internal/review/plan"
)

func TestElWorkspaceObsoletoFallaYNoSeFingeUnMontaje(t *testing.T) {
	f := newFakeLayout()
	if _, err := f.client().MountLayout(context.Background(),
		Container{WorkspaceID: "w18"}, testPlan()); err != nil {
		t.Fatalf("con el workspace vivo el montaje falló: %v", err)
	}

	falle := newFakeLayout()
	base := falle.respond
	falle.respond = func(args []string) ([]byte, []byte, error) {
		if len(args) > 2 && args[0] == "pane" && args[1] == "list" {
			return nil, nil, fmt.Errorf("workspace w1B not found")
		}
		return base(args)
	}
	warns, err := falle.client().MountLayout(context.Background(),
		Container{WorkspaceID: "w1B"}, testPlan())
	if err == nil {
		t.Fatalf("un workspace muerto no debería montar en silencio (warnings=%v)", warns)
	}
	if !strings.Contains(err.Error(), "w1B") {
		t.Errorf("el error debería nombrar el workspace para que se sepa cuál está "+
			"obsoleto, y dice %q", err.Error())
	}
	if err.Error() == "" || strings.Contains(err.Error(), "<nil>") {
		t.Errorf("el error envuelve un nil: %q. Con la condición invertida el camino "+
			"sano devuelve un error con un nil dentro, que es como se ve el mutante",
			err.Error())
	}
	if falle.called("workspace", "create") {
		t.Error("no debería crear un workspace de repuesto: el montaje tiene que fallar, " +
			"no rehacer el review en otro sitio")
	}
}

// The temptation here is to take the first match.
func TestElTabSeBuscaPorElPaneQueSePasó(t *testing.T) {
	// A workspace with SEVERAL panes where the requested one is not the first; the fixture has at
	//least three and the request goes to the last on purpose.
	multi := `{"id":"cli:pane:list","result":{"type":"pane_list","panes":[
	  {"pane_id":"w18:p1","workspace_id":"w18","tab_id":"w18:t1","cwd":"/repo","label":"shell"},
	  {"pane_id":"w18:p2","workspace_id":"w18","tab_id":"w18:t2","cwd":"/repo","label":"vi"},
	  {"pane_id":"w18:p3","workspace_id":"w18","tab_id":"w18:t3","cwd":"/repo","label":"prdash"}
	]}}`
	f := newFakeLayout()
	base := f.respond
	f.respond = func(args []string) ([]byte, []byte, error) {
		if len(args) > 2 && args[0] == "pane" && args[1] == "list" {
			return []byte(multi), nil, nil
		}
		return base(args)
	}
	c := f.client()

	ctx := context.Background()
	panes, err := c.PaneList(ctx, "w18")
	if err != nil {
		t.Fatalf("PaneList: %v", err)
	}
	if len(panes) < 3 {
		t.Fatalf("el fixture tiene %d panes, y el test necesita al menos 3 para que "+
			"el pedido NO sea el primero", len(panes))
	}
	pedido := panes[len(panes)-1].PaneID

	got := c.tabOf(ctx, panes[0].WorkspaceID, pedido)
	if got == "" {
		t.Fatalf("el tab del pane %q salió vacío: existe en la lista", pedido)
	}
	want := panes[len(panes)-1].TabID
	if want == panes[0].TabID {
		t.Fatalf("los panes del fixturemulti comparten tab (%q), y entonces comparar "+
			"mal el id no se distinguiría", want)
	}
	if got != want {
		t.Errorf("el pane %q dio el tab %q, want %q. El tab se busca COMPARANDO el id: "+
			"devolver el del primer pane renombra la pestaña equivocada", pedido, got, want)
	}

	// A pane that is not in the list has no tab, which is what makes the mount skip it.
	if got := c.tabOf(ctx, panes[0].WorkspaceID, "w18:no-existe"); got != "" {
		t.Errorf("un pane que no está en la lista dio el tab %q, want vacío", got)
	}
}

// The trap is in what the plan does NOT set.
func TestLaDireccionDelPlanSeRespeta(t *testing.T) {
	casos := []struct {
		dir  string
		want string
		nota string
	}{
		{plan.DirRight, plan.DirRight, "la derecha explícita"},
		{plan.DirDown, plan.DirDown, "abajo explícito: es la del editor de diff, y se " +
			"lee mal si se pone a la derecha"},
		{"", plan.DirRight, "sin decir nada: la derecha, que es la lectura de un diff " +
			"junto a lo que lo comenta"},
	}
	for _, c := range casos {
		if got := direction(plan.Pane{Dir: c.dir}); got != c.want {
			t.Errorf("dirección %q dio %q, want %q. %s", c.dir, got, c.want, c.nota)
		}
	}
}
