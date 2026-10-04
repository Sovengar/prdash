package testutil

import (
	"context"
	"strings"
	"testing"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
)

// This file tests the test double, which is the opposite of the usual —doubles are not normally
//tested— and the reason is that a double that lies does not fail, it weakens.

func refDePrueba() model.RepoRef {
	return model.RepoRef{Forge: "github", Host: "github.com", Project: "o/r", Owner: "o", Name: "r"}
}

func TestElDobleIdentificaForgeYHostYEstaAutenticadoPorDefecto(t *testing.T) {
	f := &FakeAdapter{ForgeName: "github", HostName: "github.com"}
	if f.Forge() != "github" || f.Host() != "github.com" {
		t.Errorf("Forge/Host dio %q/%q", f.Forge(), f.Host())
	}

	auth := f.Auth(context.Background())
	if !auth.OK {
		t.Error("sin configurar, Auth dio OK=false: los tests que no hablan de auth " +
			"aparecerían como degradados")
	}
	if auth.Forge != "github" {
		t.Errorf("el default de Auth no trae el nombre del forge: %q", auth.Forge)
	}

	f.AuthState = model.AuthState{Forge: "gitlab", OK: false, Reason: "token caducado"}
	auth = f.Auth(context.Background())
	if auth.OK || auth.Forge != "gitlab" || auth.Reason != "token caducado" {
		t.Errorf("el estado configurado salio alterado: %+v", auth)
	}
}

// The two halves matter for different reasons: advancing a page is what makes list pagination
// testable, and repeating the last is what makes a test that does not care stop mid-stream.
func TestListAvanzaDePaginaYLaUltimaSeRepite(t *testing.T) {
	key := FakeKey{Section: model.SectionReview, Kind: model.ReviewRequested}
	primera := forge.Page{Items: []model.Item{{Number: 1}}, More: true, Next: "cursor-1"}
	segunda := forge.Page{Items: []model.Item{{Number: 2}}, More: false}
	f := &FakeAdapter{ForgeName: "github", Pages: map[FakeKey][]forge.Page{key: {primera, segunda}}}
	q := forge.Query{Section: model.SectionReview, ReviewKind: model.ReviewRequested}

	page, _ := f.List(context.Background(), q)
	if len(page.Items) != 1 || page.Items[0].Number != 1 || !page.More || page.Next != "cursor-1" {
		t.Fatalf("la primera pagina dio %+v", page)
	}
	page, _ = f.List(context.Background(), q)
	if len(page.Items) != 1 || page.Items[0].Number != 2 || page.More {
		t.Fatalf("la segunda pagina dio %+v", page)
	}
	for i := 0; i < 3; i++ {
		page, _ = f.List(context.Background(), q)
		if len(page.Items) != 1 || page.Items[0].Number != 2 {
			t.Fatalf("la pagina %d tras agotar dio %+v, want la ultima repetida", i+3, page)
		}
	}
	if got := f.ListCallCount(); got != 5 {
		t.Errorf("ListCallCount dio %d, want 5", got)
	}

	otra := forge.Query{Section: model.SectionMentions}
	page, warns := f.List(context.Background(), otra)
	if len(page.Items) != 0 || len(warns) != 0 || page.More {
		t.Errorf("una lista sin configurar dio %+v / %v", page, warns)
	}
}

// The difference between "once" and "always": the TUI refreshes every six seconds.
func TestLosWarningsSalenEnCadaLlamada(t *testing.T) {
	key := FakeKey{Section: model.SectionReview, Kind: model.ReviewRequested}
	warns := []model.Warning{{Forge: "github", Kind: "ratelimit", Msg: "espera"}}
	f := &FakeAdapter{
		ForgeName:    "github",
		Pages:        map[FakeKey][]forge.Page{key: {{Items: []model.Item{{Number: 1}}}}},
		ListWarnings: map[FakeKey][]model.Warning{key: warns},
	}
	q := forge.Query{Section: model.SectionReview, ReviewKind: model.ReviewRequested}

	for i := 1; i <= 3; i++ {
		_, got := f.List(context.Background(), q)
		if len(got) != 1 || got[0].Kind != "ratelimit" {
			t.Fatalf("la llamada %d dio %v, want el ratelimit en todas", i, got)
		}
	}
}

// Two behaviours in one function and both matter: it refuses without a HeadSHA, and it records
// NOTHING, because a merge that did not go asked for no strategy.
func TestElDobleSeNiegaAMergearSinPinYNoLoRegistra(t *testing.T) {
	f := &FakeAdapter{ForgeName: "github"}
	ref := refDePrueba()

	warns := f.Merge(context.Background(), ref, 1, forge.MergeRequest{Mode: forge.Squash})
	if len(warns) != 1 || warns[0].Kind != "unsupported" {
		t.Fatalf("un merge sin pin dio %v, want un unsupported", warns)
	}
	if !strings.Contains(warns[0].Msg, "head") {
		t.Errorf("el aviso %q no dice que falta el head", warns[0].Msg)
	}
	if got := f.ActionCallCount("merge", ref, 1); got != 0 {
		t.Errorf("un merge rechazado conto %d llamadas de accion, want 0", got)
	}
	if got := f.MergeModeCount(forge.Squash); got != 0 {
		t.Errorf("un merge rechazado conto %d modos, want 0", got)
	}
	if len(f.MergeDeletes) != 0 {
		t.Errorf("un merge rechazado registro borrado: %v", f.MergeDeletes)
	}

	warns = f.Merge(context.Background(), ref, 1, forge.MergeRequest{
		Mode: forge.Squash, HeadSHA: "deadbeef", DeleteBranch: true,
	})
	if len(warns) != 0 {
		t.Errorf("un merge con pin dio avisos %v", warns)
	}
	if got := f.ActionCallCount("merge", ref, 1); got != 1 {
		t.Errorf("ActionCallCount dio %d, want 1", got)
	}
	if got := f.MergeModeCount(forge.Squash); got != 1 {
		t.Errorf("MergeModeCount dio %d, want 1", got)
	}
	if f.MergeDeleteCount(true) != 1 || f.MergeDeleteCount(false) != 0 {
		t.Errorf("MergeDeleteCount dio true=%d false=%d, want 1 y 0",
			f.MergeDeleteCount(true), f.MergeDeleteCount(false))
	}
}

// The order is what makes Retargets a list rather than a counter: the test's question is "which base
// did it move to?".
func TestElDobleSeNiegaARetargetSinRamaYLoRegistraEnOrden(t *testing.T) {
	f := &FakeAdapter{ForgeName: "github"}
	ref := refDePrueba()

	for _, vacia := range []string{"", "   ", "\t"} {
		warns := f.Retarget(context.Background(), ref, 1, vacia)
		if len(warns) != 1 || warns[0].Kind != "unsupported" {
			t.Errorf("un retarget a %q dio %v, want un unsupported", vacia, warns)
		}
	}
	if f.RetargetCount() != 0 {
		t.Errorf("un retarget rechazado conto %d, want 0", f.RetargetCount())
	}
	if got := f.ActionCallCount("retarget", ref, 1); got != 0 {
		t.Errorf("un retarget rechazado conto %d llamadas, want 0", got)
	}

	for _, rama := range []string{"main", "release/2.0", "develop"} {
		f.Retarget(context.Background(), ref, 1, rama)
	}
	want := []string{"main", "release/2.0", "develop"}
	if f.RetargetCount() != len(want) {
		t.Fatalf("RetargetCount dio %d, want %d", f.RetargetCount(), len(want))
	}
	for i := range want {
		if f.Retargets[i] != want[i] {
			t.Errorf("Retargets[%d] = %q, want %q (completo %v)", i, f.Retargets[i], want[i], f.Retargets)
		}
	}
}

// "Only after" is the half that matters: the merge does a re-read before deciding, and a hook that
// fired on it would corrupt the state being read.
func TestOnMergeSoloAplicaDespuesDelMerge(t *testing.T) {
	ref := refDePrueba()
	clave := ItemKey(ref.Project, 1)
	f := &FakeAdapter{
		ForgeName:  "github",
		ItemStates: map[string]model.Item{clave: {Number: 1, State: "OPEN", Ref: ref}},
		OnMerge:    func(it *model.Item) { it.State = "MERGED" },
	}

	it, _ := f.ItemState(context.Background(), ref, 1)
	if it.State != "OPEN" {
		t.Fatalf("sin merge dio %q, want OPEN: el hook se esta aplicando antes de tiempo", it.State)
	}

	f.Merge(context.Background(), ref, 1, forge.MergeRequest{Mode: forge.Squash, HeadSHA: "deadbeef"})

	it, _ = f.ItemState(context.Background(), ref, 1)
	if it.State != "MERGED" {
		t.Errorf("tras el merge dio %q, want MERGED: el hook no se esta aplicando", it.State)
	}
	if it.Number != 1 {
		t.Errorf("el hook se comió el resto del item: %+v", it)
	}

	limpio := &FakeAdapter{ForgeName: "github", ItemStates: map[string]model.Item{clave: {Number: 1, State: "OPEN"}}}
	limpio.Merge(context.Background(), ref, 1, forge.MergeRequest{Mode: forge.Squash, HeadSHA: "x"})
	it, _ = limpio.ItemState(context.Background(), ref, 1)
	if it.State != "OPEN" {
		t.Errorf("sin OnMerge dio %q, want OPEN", it.State)
	}
}

func TestLosWarningsDeEstadoYDerechosVienenConfigurados(t *testing.T) {
	ref := refDePrueba()
	clave := ItemKey(ref.Project, 7)
	if clave != "o/r#7" {
		t.Fatalf("ItemKey dio %q, want o/r#7", clave)
	}

	f := &FakeAdapter{
		ForgeName: "github",
		ItemStates: map[string]model.Item{
			clave: {Number: 7, Title: "uno", Ref: ref},
		},
		StateWarnings: map[string][]model.Warning{
			clave: {{Kind: "ratelimit", Msg: "espera"}},
		},
		Conversations: map[string]forge.CommentPage{
			clave: {Comments: []model.Comment{{Body: "hola"}}, Total: 1},
		},
		CommentWarnings: map[string][]model.Warning{
			clave: {{Kind: "network", Msg: "sin red"}},
		},
		ActionWarnings: map[string][]model.Warning{
			"approve:o/r#7": {{Kind: "permission", Msg: "no"}},
		},
	}

	it, warns := f.ItemState(context.Background(), ref, 7)
	if it.Title != "uno" || len(warns) != 1 || warns[0].Kind != "ratelimit" {
		t.Errorf("ItemState dio %+v / %v", it, warns)
	}
	page, warns := f.Comments(context.Background(), ref, 7)
	if len(page.Comments) != 1 || page.Comments[0].Body != "hola" || warns[0].Kind != "network" {
		t.Errorf("Comments dio %+v / %v", page, warns)
	}
	if warns := f.Approve(context.Background(), ref, 7); len(warns) != 1 || warns[0].Kind != "permission" {
		t.Errorf("Approve dio %v", warns)
	}

	it, warns = f.ItemState(context.Background(), ref, 999)
	if it.Number != 0 || len(warns) != 0 {
		t.Errorf("un numero sin configurar dio %+v / %v", it, warns)
	}
}

// The empty page has to carry Total at zero, not a total claiming there are 30 comments.
func TestCommentsSinConfigurarVieneResuelto(t *testing.T) {
	f := &FakeAdapter{ForgeName: "github"}
	page, warns := f.Comments(context.Background(), refDePrueba(), 1)
	if len(page.Comments) != 0 || len(warns) != 0 {
		t.Errorf("Comments sin configurar dio %+v / %v", page, warns)
	}
	if page.Total != 0 {
		t.Errorf("Comments sin configurar dio Total=%d, want 0", page.Total)
	}
	if got := f.CommentCallCount(); got != 1 {
		t.Errorf("CommentCallCount dio %d, want 1", got)
	}
	f.Comments(context.Background(), refDePrueba(), 1)
	if got := f.CommentCallCount(); got != 2 {
		t.Errorf("CommentCallCount dio %d, want 2", got)
	}
}

func TestBranchesPorProyectoYConContador(t *testing.T) {
	ref := refDePrueba()
	otro := model.RepoRef{Forge: "github", Host: "github.com", Project: "o/otro"}

	f := &FakeAdapter{
		ForgeName:      "github",
		BranchLists:    map[string][]string{"o/r": {"main", "feature"}},
		BranchWarnings: map[string][]model.Warning{"o/otro": {{Kind: "unsupported", Msg: "no"}}},
	}

	names, warns := f.Branches(context.Background(), ref)
	if len(names) != 2 || names[0] != "main" || len(warns) != 0 {
		t.Errorf("Branches dio %v / %v", names, warns)
	}
	if got := f.BranchCallCount("o/r"); got != 1 {
		t.Errorf("BranchCallCount dio %d, want 1", got)
	}
	if got := f.BranchCallCount("o/otro"); got != 0 {
		t.Errorf("BranchCallCount de un proyecto sin llamar dio %d, want 0", got)
	}

	names, warns = f.Branches(context.Background(), otro)
	if len(names) != 0 || len(warns) != 1 || warns[0].Kind != "unsupported" {
		t.Errorf("un proyecto sin configurar dio %v / %v", names, warns)
	}
	if got := f.BranchCallCount("o/otro"); got != 1 {
		t.Errorf("BranchCallCount dio %d tras la llamada, want 1", got)
	}
}

// Separate counters per action kind, not just per item: one counter would report "1" for three
// different actions.
func TestElContadorDeAccionesSeparaLasTres(t *testing.T) {
	ref := refDePrueba()
	f := &FakeAdapter{ForgeName: "github"}

	f.Approve(context.Background(), ref, 1)
	f.Approve(context.Background(), ref, 1)
	f.Approve(context.Background(), ref, 2) // otro ítem
	f.Merge(context.Background(), ref, 1, forge.MergeRequest{Mode: forge.Squash, HeadSHA: "a"})
	f.Retarget(context.Background(), ref, 1, "main")

	casos := []struct {
		kind   string
		number int
		want   int
	}{
		{"approve", 1, 2},
		{"approve", 2, 1},
		{"merge", 1, 1},
		{"retarget", 1, 1},
		{"approve", 3, 0},
		{"merge", 2, 0},
		{"inventada", 1, 0},
	}
	for _, c := range casos {
		if got := f.ActionCallCount(c.kind, ref, c.number); got != c.want {
			t.Errorf("ActionCallCount(%q, %d) dio %d, want %d", c.kind, c.number, got, c.want)
		}
	}
	f.ActionWarnings = map[string][]model.Warning{
		"approve:o/r#2": {{Kind: "unsupported", Msg: "no"}},
	}
	if warns := f.Approve(context.Background(), ref, 2); len(warns) != 1 {
		t.Errorf("Approve dio %v", warns)
	}
	if warns := f.Merge(context.Background(), ref, 2, forge.MergeRequest{HeadSHA: "x"}); len(warns) != 0 {
		t.Errorf("Merge con un aviso ajeno en la misma clave dio %v", warns)
	}
}

func TestLaSuiteDeConformidadCubreTodaLaSuperficieDelContrato(t *testing.T) {
	a := &contadorDeLlamadas{FakeAdapter: &FakeAdapter{
		ForgeName: "github",
		HostName:  "github.com",
	}}
	a.Pages = map[FakeKey][]forge.Page{
		{Section: model.SectionAuthored}:                            {{Items: []model.Item{{Number: 1}}}},
		{Section: model.SectionReview, Kind: model.ReviewRequested}: {{Items: []model.Item{{Number: 2}}}},
		{Section: model.SectionReview, Kind: model.ReviewAssigned}:  {{Items: []model.Item{{Number: 3}}}},
		{Section: model.SectionMentions}:                            {{Items: []model.Item{{Number: 4}}}},
	}
	RunConformance(t, a, ConformanceOptions{})

	vistos := a.vistos()
	esperados := []string{
		"List", "ItemState", "Comments", "Approve", "Merge", "Retarget", "Branches",
	}
	for _, m := range esperados {
		if vistos[m] == 0 {
			t.Errorf("RunConformance no invocó %s: un método que la suite no toca es un "+
				"método que nadie prueba", m)
		}
	}
	for _, q := range forge.Streams {
		k := FakeKey{Section: q.Section, Kind: q.ReviewKind}
		if a.Pages[k] == nil && a.listas[k] == 0 {
			t.Errorf("la suite no pregunto el stream %+v", q)
		}
	}
	if vistos["Auth"] != 0 {
		t.Error("la suite invoca Auth y la lista de este test no lo esperaba: revisar")
	}
}

type contadorDeLlamadas struct {
	*FakeAdapter
	listas    map[FakeKey]int
	vistosMap map[string]int
}

func (c *contadorDeLlamadas) Forge() string { c.ver("Forge"); return c.FakeAdapter.Forge() }
func (c *contadorDeLlamadas) Host() string  { c.ver("Host"); return c.FakeAdapter.Host() }
func (c *contadorDeLlamadas) Auth(ctx context.Context) model.AuthState {
	c.ver("Auth")
	return c.FakeAdapter.Auth(ctx)
}
func (c *contadorDeLlamadas) List(ctx context.Context, q forge.Query) (forge.Page, []model.Warning) {
	c.ver("List")
	if c.listas == nil {
		c.listas = map[FakeKey]int{}
	}
	c.listas[FakeKey{Section: q.Section, Kind: q.ReviewKind}]++
	return c.FakeAdapter.List(ctx, q)
}
func (c *contadorDeLlamadas) ItemState(ctx context.Context, ref model.RepoRef, n int) (model.Item, []model.Warning) {
	c.ver("ItemState")
	return c.FakeAdapter.ItemState(ctx, ref, n)
}
func (c *contadorDeLlamadas) Comments(ctx context.Context, ref model.RepoRef, n int) (forge.CommentPage, []model.Warning) {
	c.ver("Comments")
	return c.FakeAdapter.Comments(ctx, ref, n)
}
func (c *contadorDeLlamadas) Approve(ctx context.Context, ref model.RepoRef, n int) []model.Warning {
	c.ver("Approve")
	return c.FakeAdapter.Approve(ctx, ref, n)
}
func (c *contadorDeLlamadas) Merge(ctx context.Context, ref model.RepoRef, n int, req forge.MergeRequest) []model.Warning {
	c.ver("Merge")
	return c.FakeAdapter.Merge(ctx, ref, n, req)
}
func (c *contadorDeLlamadas) Retarget(ctx context.Context, ref model.RepoRef, n int, b string) []model.Warning {
	c.ver("Retarget")
	return c.FakeAdapter.Retarget(ctx, ref, n, b)
}
func (c *contadorDeLlamadas) Branches(ctx context.Context, ref model.RepoRef) ([]string, []model.Warning) {
	c.ver("Branches")
	return c.FakeAdapter.Branches(ctx, ref)
}

func (c *contadorDeLlamadas) ver(m string) {
	if c.vistosMap == nil {
		c.vistosMap = map[string]int{}
	}
	c.vistosMap[m]++
}

func (c *contadorDeLlamadas) vistos() map[string]int { return c.vistosMap }
