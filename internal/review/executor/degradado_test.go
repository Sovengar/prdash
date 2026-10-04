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

// The three remaining ones share one property: they are DEGRADATION.

// The reason for the Planner field is not testability.
func TestElPlanSeConstruyeConElPlannerCuandoLoHay(t *testing.T) {
	it := model.NewItem(model.RepoRef{Project: "o/r"}, 7)
	wt := worktree.Worktree{Path: "/wt/pr-7", Branch: "feat/x", Label: "prdash-pr-7"}

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
	// The worktree arrives mapped to the three fields, because those are what the plan uses to build
	// the argv.
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

	sinPlanner := &Executor{
		Tools: plan.Tools{Tuicr: plan.Tool{Argv: []string{"tuicr"}}},
		Env:   plan.Env{Available: map[string]bool{}},
	}
	if got := sinPlanner.buildPlan(it, wt); len(got.Tabs) == 0 {
		t.Errorf("sin Planner, buildPlan devolvió un plan sin pestañas: %+v", got)
	}
}

// What makes this a degradation and not a failure.
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

// The two paths of mountLayout with Herdr available.
func TestElLayoutAportaSusAvisosYLosDeUnFallounoDeMas(t *testing.T) {
	wt := worktree.Worktree{Path: "/wt", Branch: "b", Label: "prdash-pr-1"}
	ctx := context.Background()

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
	// The container that reaches the layout is the worktree's, which is what connects the pane to the
	// right directory.
	if falso.container.WorkspaceID != wt.WorkspaceID || falso.container.PaneID != wt.RootPaneID {
		t.Errorf("el contenedor al layout no es el del worktree: %+v", falso.container)
	}

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

	// And on failure: the layout's warnings plus the failure's, and the two texts have to be
	// distinguishable.
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

// The condition is `created`, not whether the bare exists.
func TestLimpiarElBareSoloSiSeCreoEnEstaLlamada(t *testing.T) {
	it := model.NewItem(model.RepoRef{Project: "o/r"}, 1)

	r := &resolverQueRegistra{}
	e := &Executor{Resolver: r}
	e.cleanupBare(false, it)
	if r.quitados != 0 {
		t.Errorf("con created=false quitó %d clones: se llevaría por delante el clon de otra "+
			"pestaña del mismo PR", r.quitados)
	}

	e.cleanupBare(true, it)
	if r.quitados != 1 {
		t.Errorf("con created=true quitó %d clones, want 1", r.quitados)
	}
}

type resolverQueRegistra struct {
	reporesolverFalso
	quitados int
}

func (r *resolverQueRegistra) RemoveBare(model.RepoRef) error {
	r.quitados++
	return nil
}

// It implements the minimum of Resolver to be embeddable.
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

func TestLabelLlevaElNumeroYElPrefijoQueEsLoQueMarcaLaPropiedad(t *testing.T) {
	if got := Label(7); got != "prdash-pr-7" {
		t.Errorf("Label(7) dio %q", got)
	}
	// Two different numbers give two different labels, which is what stops two reviews from
	// colliding.
	if Label(7) == Label(8) {
		t.Error("dos números dieron la misma etiqueta")
	}
	// The label is what Owned recognises, and that is the link between the name and the
	// provisioning.
	if !strings.HasPrefix(Label(1), worktree.LabelPrefix) {
		t.Errorf("Label(1) = %q no empieza por %q: Owned no lo reconocería",
			Label(1), worktree.LabelPrefix)
	}
}
