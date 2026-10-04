package executor

import (
	"context"
	"errors"
	"strings"
	"testing"

	"prdash/internal/cache"
	"prdash/internal/forge/model"
	"prdash/internal/herdr"
	"prdash/internal/review/plan"
	"prdash/internal/worktree"
)

// Las tres funciones de este fichero son las que quedan sin cubrir del ejecutor, y las tres
// comparten una propiedad: son el DEGRADADO del montaje. Ninguna es el camino bueno —el
// camino bueno ya está cubierto de punta a punta en `executor_test.go`—, y el degradado es
// donde se decide si un montaje fallido deja basura, avisa, o hace las dos cosas.

// TestElPlanSeConstruyeConElPlannerCuandoLoHay: el seam del plan.
//
// Y la razón de que exista el campo `Planner` no es que el plan sea difícil de construir, es
// que hay dos fuentes de verdad y sin el seam los tests tocarían las dos a la vez. Un test
// que verifica el layout necesita un plan, y un plan real depende de qué herramientas hay
// instaladas en la máquina que corre el test.
//
// Y lo que se fija es la PREFERENCIA: el `Planner` gana sobre `plan.Build`. Al revés, un
// test con un plan puesto seguiría viendo el plan real y el seam no serviría para nada, y
// el fallo aparecería como "el test del layout no ve mi plan".
func TestElPlanSeConstruyeConElPlannerCuandoLoHay(t *testing.T) {
	it := model.NewItem(model.RepoRef{Project: "o/r"}, 7)
	wt := worktree.Worktree{Path: "/wt/pr-7", Branch: "feat/x", Label: "prdash-pr-7"}

	// Con Planner: es el suyo, y lo recibe con el worktree ya mapeado a los tres campos
	// que el plan necesita.
	vistos := 0
	var recibidoItem model.Item
	var recibidoWT plan.Worktree
	e := &Executor{
		Planner: func(pr model.Item, pw plan.Worktree) plan.Plan {
			vistos++
			recibidoItem, recibidoWT = pr, pw
			return plan.Plan{Tabs: []plan.Tab{{Label: "MIO"}}}
		},
	}
	got := e.buildPlan(it, wt)
	if vistos != 1 {
		t.Errorf("el Planner se llamó %d veces, want 1", vistos)
	}
	if len(got.Tabs) != 1 || got.Tabs[0].Label != "MIO" {
		t.Errorf("buildPlan devolvió %+v, want el plan del Planner", got)
	}
	if recibidoItem.Number != 7 {
		t.Errorf("el Planner recibió el item %+v", recibidoItem)
	}
	// Y el worktree llega mapeado a los tres campos, porque son los que el plan usa para
	// nombrar los panes. Un `wt.Path` a cero aquí produce panes sin ruta, que es el plan
	// que no se puede montar.
	for nombre, valor := range map[string]string{
		"Path": recibidoWT.Path, "Branch": recibidoWT.Branch, "Label": recibidoWT.Label,
	} {
		if valor == "" {
			t.Errorf("el Planner recibió el worktree con %s vacío: %+v", nombre, recibidoWT)
		}
	}
	if recibidoWT.Path != wt.Path || recibidoWT.Branch != wt.Branch || recibidoWT.Label != wt.Label {
		t.Errorf("el worktree no llego integro al Planner: %+v", recibidoWT)
	}

	// Y sin Planner: el `plan.Build` de verdad, que es el camino de producción. Solo se
	// comprueba que devuelve algo usable, porque su contenido ya tiene sus propios tests.
	sinPlanner := &Executor{
		Tools: plan.Tools{Tuicr: plan.Tool{Argv: []string{"tuicr"}}},
		Env:   plan.Env{Available: map[string]bool{}},
	}
	if got := sinPlanner.buildPlan(it, wt); len(got.Tabs) == 0 {
		t.Errorf("sin Planner, buildPlan devolvió un plan sin pestañas: %+v", got)
	}
}

// TestSinHerdrElWorktreeQuedaMontadoYAvisado: el degradado del layout.
//
// Y el detalle que hace que esto sea un degradado y no un fallo es que el worktree SE QUEDA.
// El worktree es lo caro de montar —un clon, un ref, un directorio— y Herdr es lo
// decorativo: sin él el review se puede abrir a mano en el directorio que el popup enseña, y
// eso es mejor que nada.
//
// Y el aviso tiene que decir las dos cosas: que falta Herdr Y que el worktree está montado.
// Un aviso que solo dijera "Herdr no disponible" dejaría al usuario pensando que el montaje
// falló, y probablemente podria borrar el worktree para no dejar basura.
func TestSinHerdrElWorktreeQuedaMontadoYAvisado(t *testing.T) {
	wt := worktree.Worktree{Path: "/wt", Branch: "b", Label: "prdash-pr-1"}
	res := Result{}

	// Sin puerto de Herdr en absoluto.
	e := &Executor{}
	if e.mountLayout(context.Background(), wt, plan.Plan{}, &res) {
		t.Error("sin Herdr se reportó el layout montado")
	}
	if len(res.Warnings) != 1 {
		t.Fatalf("avisos = %v, want 1", res.Warnings)
	}
	if !strings.Contains(res.Warnings[0], "Herdr") || !strings.Contains(res.Warnings[0], "worktree") {
		t.Errorf("el aviso %q tiene que decir que falta Herdr y que el worktree queda", res.Warnings[0])
	}

	// Con un puerto que dice que no está disponible: el mismo camino.
	noDisponible := &Executor{Herdr: &fakeHerdr{available: false}}
	res = Result{}
	if noDisponible.mountLayout(context.Background(), wt, plan.Plan{}, &res) {
		t.Error("con Herdr no disponible se reportó el layout montado")
	}
	if len(res.Warnings) != 1 {
		t.Errorf("avisos = %v, want 1", res.Warnings)
	}
}

// herdrQueFallaEsUnPuertoQueDisponiblePeroNoMonta.
type herdrQueFalla struct {
	err   error
	warns []string
}

func (h *herdrQueFalla) Available() bool { return true }

func (h *herdrQueFalla) MountLayout(context.Context, herdr.Container, plan.Plan) ([]string, error) {
	return h.warns, h.err
}

func (h *herdrQueFalla) Notify(context.Context, string, herdr.NotifyOptions) error { return nil }

// TestElLayoutAportaSusAvisosYLosDeUnFallounoDeMas: los dos caminos de `mountLayout` con
// Herdr disponible.
//
// Y son los dos que se confunden al leer el código, porque los dos devuelven `false`. La
// diferencia es qué pasa después: con Herdr disponible pero el layout fallando, el worktree
// sigue montado Y el aviso tiene que decir que no se pudo abrir el layout —no que falta
// Herdr, que sí está—. Confundir los dos mensajes manda al usuario a buscar un problema de
// Herdr que no existe.
func TestElLayoutAportaSusAvisosYLosDeUnFallounoDeMas(t *testing.T) {
	wt := worktree.Worktree{Path: "/wt", Branch: "b", Label: "prdash-pr-1"}
	ctx := context.Background()

	// Camino bueno: monta, avisa, y notifica.
	falso := &fakeHerdr{available: true}
	e := &Executor{Herdr: falso}
	res := Result{}
	if !e.mountLayout(ctx, wt, plan.Plan{}, &res) {
		t.Error("con Herdr disponible no montó")
	}
	if len(falso.notified) != 1 {
		t.Errorf("notificaciones = %v, want 1", falso.notified)
	}
	if len(res.Warnings) != 0 {
		t.Errorf("el camino bueno dio avisos %v", res.Warnings)
	}
	// Y el contenedor que llega al layout es el del worktree, que es lo que conecta el
	// pane con el workspace correcto. Con el contenedor vacío, el layout se abriría en un
	// sitio que no es.
	if falso.container.WorkspaceID != wt.WorkspaceID || falso.container.PaneID != wt.RootPaneID {
		t.Errorf("el contenedor al layout no es el del worktree: %+v", falso.container)
	}

	// Con avisos no fatales del layout: se suman a los que ya había, no se sustituyen.
	conAvisos := &Executor{Herdr: &herdrQueFalla{warns: []string{"hunk no está instalado"}}}
	res = Result{Warnings: []string{"aviso previo"}}
	if !conAvisos.mountLayout(ctx, wt, plan.Plan{}, &res) {
		t.Error("un layout con avisos no fatales no debe considerarse fallido")
	}
	if len(res.Warnings) != 2 {
		t.Fatalf("avisos = %v, want los dos", res.Warnings)
	}
	if res.Warnings[0] != "aviso previo" {
		t.Errorf("los avisos previos se perdieron: %v", res.Warnings)
	}
	if !strings.Contains(res.Warnings[1], "hunk") {
		t.Errorf("el aviso del layout no llegó: %v", res.Warnings)
	}

	// Y con fallo: los avisos del layout + el del fallo, y los dos textos tienen que
	// distinguirse. Este es el caso que importa: "falta Herdr" y "no se pudo abrir el
	// layout" son diagnósticos opuestos.
	conFallo := &Executor{Herdr: &herdrQueFalla{err: errors.New("el socket no responde")}}
	res = Result{}
	if conFallo.mountLayout(ctx, wt, plan.Plan{}, &res) {
		t.Error("un layout que falló se reportó como montado")
	}
	if len(res.Warnings) == 0 {
		t.Fatal("el fallo del layout no dejó aviso")
	}
	texto := strings.Join(res.Warnings, " | ")
	if !strings.Contains(texto, "layout") {
		t.Errorf("el aviso %q no dice que el layout falló", texto)
	}
	if strings.Contains(texto, "requires HERDR_ENV") {
		t.Errorf("el aviso %q dice que falta Herdr, que sí estaba: manda al usuario a "+
			"buscar un problema que no existe", texto)
	}
}

// TestLimpiarElBareSoloSiSeCreoEnEstaLlamada: `cleanupBare`, la condición que la protege.
//
// Y la condición es `created`, no "existe", y la diferencia es de seguridad. `Mount` la llama
// en dos caminos de error: fallo al traer el ref y fallo al crear el worktree. En el primero
// puede que el clon bare VINIERA de una llamada anterior —otra pestaña del mismo PR ya lo
// clonó— y borrarlo se llevaría por delante el clon que la otra pestaña está usando.
//
// O sea: `created` significa "esta llamada lo trajo", y borrarlo cuando no lo trajo es
// romper el review de otra sesión. Un `if existiera` en vez de `if created` no fallaría
// nunca: borraría más de lo que debe, que es el modo de fallo que no se ve.
func TestLimpiarElBareSoloSiSeCreoEnEstaLlamada(t *testing.T) {
	it := model.NewItem(model.RepoRef{Project: "o/r"}, 1)

	// Con `created` a false: no hace nada, y se comprueba que no se puede observar.
	// El `Resolver` de este caso registra las llamadas a `RemoveBare` en vez de borrar.
	r := &resolverQueRegistra{}
	e := &Executor{Resolver: r}
	e.cleanupBare(false, it)
	if r.quitados != 0 {
		t.Errorf("con created=false quitó %d clones: se llevaría por delante el clon de otra "+
			"pestaña del mismo PR", r.quitados)
	}

	// Con `created` a true: quita uno.
	e.cleanupBare(true, it)
	if r.quitados != 1 {
		t.Errorf("con created=true quitó %d clones, want 1", r.quitados)
	}
}

// resolverQueRegistra es un Resolver que cuenta las llamadas a RemoveBare en vez de borrar
// nada, para poder observar la decisión sin tocar el disco.
type resolverQueRegistra struct {
	reporesolverFalso
	quitados int
}

func (r *resolverQueRegistra) RemoveBare(model.RepoRef) error {
	r.quitados++
	return nil
}

// reporesolverFalso implementa lo mínimo de Resolver para poder embeberlo sin levantar un
// resolutor de verdad. Lo único que mira este test es `RemoveBare`, así que el resto
// devuelve ceros: lo que importa es que el embedded existe para que el override de
// `RemoveBare` del struct que lo envuelve tenga a qué superponerse.
type reporesolverFalso struct{}

func (reporesolverFalso) ResolveLocal(model.RepoRef) (string, bool) { return "", false }
func (reporesolverFalso) HasBare(model.RepoRef) bool                { return false }
func (reporesolverFalso) EnsureBare(context.Context, model.RepoRef) (string, error) {
	return "", nil
}
func (reporesolverFalso) RemoveBare(model.RepoRef) error { return nil }
func (reporesolverFalso) FetchReviewRef(context.Context, string, model.Item) (string, error) {
	return "", nil
}
func (reporesolverFalso) WorktreePath(model.RepoRef, int) string            { return "" }
func (reporesolverFalso) Remember(model.RepoRef, string)                    {}
func (reporesolverFalso) RecordReview(model.Item, cache.ReviewRecord) error { return nil }
func (reporesolverFalso) ActiveReview(model.ID) (cache.ReviewRecord, bool) {
	return cache.ReviewRecord{}, false
}
func (reporesolverFalso) ForgetReview(model.Item) error { return nil }

// Etiqueta con ownership de prdash, que es lo que la lista usa para decidir.
func TestLabelLlevaElNumeroYElPrefijoQueEsLoQueMarcaLaPropiedad(t *testing.T) {
	if got := Label(7); got != "prdash-pr-7" {
		t.Errorf("Label(7) dio %q", got)
	}
	// Y dos números distintos dan dos etiquetas distintas, que es lo que evita que dos
	// reviews del mismo repo se pisen el directorio.
	if Label(7) == Label(8) {
		t.Error("dos números dieron la misma etiqueta")
	}
	// Y la etiqueta es la que `Owned` reconoce, que es la conexión entre el nombre y la
	// propiedad. Si `Label` dejara de empezar por el prefijo, todos los worktrees dejarían
	// de ser de prdash y `Audit` no los listaría.
	if !strings.HasPrefix(Label(1), worktree.LabelPrefix) {
		t.Errorf("Label(1) = %q no empieza por %q: Owned no lo reconocería",
			Label(1), worktree.LabelPrefix)
	}
}
