package herdr

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"prdash/internal/review/plan"
)

// Los tres de layout.go que sobrevivían son tres negaciones, y las tres se ven en la
// misma cosa: qué se hace con un id que no es el que se esperaba.

// TestElWorkspaceObsoletoFallaYNoSeFingeUnMontaje: cuando el workspace del contenedor no
// existe, el montaje falla nombrando el id, y no abre un workspace de repuesto.
//
// Y lo que hace la condición invertida es peor de lo que parece a primera vista: en vez
// de "no comprobar el error", comprobaría el contrario y devolvería un error SIEMPRE,
// también en el camino bueno. O sea que el mutante no rompe el caso malo, rompe el caso
// normal: con el workspace sano, `listErr` es nil, el mutante entra y devuelve un error
// con un nil envuelto. Cualquier montaje bueno lo delata.
//
// Y el motivo de la comprobación está en el comentario del código y conviene repetirlo
// porque es lo que la hace necesaria: Herdr guarda el id del workspace en su sesión
// persistida, así que un workspace cerrado deja el contenedor apuntando a nada. Abrir
// otro workspace produciría un review desligado del worktree que lo contiene, sin
// avisar de nada —que es la clase de fallo que no se ve hasta que es tarde—.
func TestElWorkspaceObsoletoFallaYNoSeFingeUnMontaje(t *testing.T) {
	// El camino SANO: el workspace existe y el montaje tiene que pasar. Esta es la
	// mitad que el test anterior no afirmaba y la que mata la negación.
	f := newFakeLayout()
	if _, err := f.client().MountLayout(context.Background(),
		Container{WorkspaceID: "w18"}, testPlan()); err != nil {
		t.Fatalf("con el workspace vivo el montaje falló: %v", err)
	}

	// Y el camino MUERTO: el workspace no existe y hay que fallar nombrándolo.
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

// TestElTabSeBuscaPorElPaneQueSePasó: el tab de un pane se busca comparando el id, pane a
// pane, y sale el de ESE pane.
//
// Y la tentación aquí es dar por hecho que el pane buscado está el primero, porque en
// los layouts de prdash suele estarlo: es el root pane del worktree. Si fuera así, la
// comparación no miraría el id y devolvería cualquier cosa.
//
// El caso que lo distingue es el de un layout que ya tiene panes antes de montar: entonces
// el pane buscado NO es el primero, y una comparación que devuelve el primer pane sin
// mirar se lleva el tab equivocado. Renombrar el tab equivocado deja el review en una
// pestaña sin nombre y el trabajo en otra con el nombre de la primera, que es de donde
// el usuario no sabe qué está leyendo.
func TestElTabSeBuscaPorElPaneQueSePasó(t *testing.T) {
	// Un workspace con VARIOS panes, de los cuales el pedido no es el primero. El
	// fixture tiene al menos tres, y el pedido va al último a propósito.
	// El fixture de la suite tiene DOS panes y los dos en el MISMO tab, con lo que
	// comparar mal el id no se notaría: los dos panes devolverían el mismo tab. Y ese
	// caso no es raro, es el de un worktree recién montado.
	//
	// El caso que de verdad distingue es un workspace con varios TABS, que es lo que
	// pasa cuando el usuario ya tenía una sesión abierta en ese workspace antes de
	// montar el review encima. Entonces el pane base puede estar en el segundo o el
	// tercer tab, y devolver el tab del primer pane renombra la pestaña equivocada.
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

	// Los panes se piden con el `PaneList` de verdad en vez de leerse del fixture a
	// mano: el fixture es un documento JSON entero, no una línea por pane, y lo que se
	// quiere comprobar aquí es qué tab sale para un pane, no qué hay en el fichero.
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

	// Y un pane que no está en la lista no tiene tab, que es lo que hace el montaje
	// saltarse el renombrado en vez de inventarse uno.
	if got := c.tabOf(ctx, panes[0].WorkspaceID, "w18:no-existe"); got != "" {
		t.Errorf("un pane que no está en la lista dio el tab %q, want vacío", got)
	}
}

// TestLaDireccionDelPlanSeRespeta: la dirección de división sale del pane, y la que el
// plan no fija es la de la derecha.
//
// Y aquí está la trampa que hace que este test exista: el layout por defecto de prdash
// usa la derecha para los panes segundos, así que una condición mal puesta pasa todos
// los tests que usan el plan por defecto. La derecha es el valor por defecto Y el valor
// que pone el plan, con lo que de las dos ramas solo se nota la de abajo.
//
// Los tres casos que hay que mirar son los tres: la derecha explícita (que con la
// condición invertida también daría derecha, así que no la distingue), la izquierda, y
// la que no dice nada. Los dos últimos sí la distinguen, y el segundo de esos dos es el
// que más se ve: un pane de revisión pegado a la derecha de un diff que se lee de
// izquierda a derecha obliga a saltar la vista.
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
