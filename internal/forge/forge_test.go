package forge_test

import (
	"context"
	"strings"
	"testing"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

func mkItem(forgeName, host, project string, number int) model.Item {
	return model.NewItem(model.RepoRef{Forge: forgeName, Host: host, Project: project, Owner: "acme", Name: "widget"}, number)
}

func TestRegistryRegisterAndGet(t *testing.T) {
	reg := forge.NewRegistry()
	gh := &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"}
	gl := &testutil.FakeAdapter{ForgeName: "gitlab", HostName: "gitlab.example.com"}
	reg.Register(gh)
	reg.Register(gl)

	if got, ok := reg.Get("github"); !ok || got != gh {
		t.Fatalf("Get(github) = %v, %v", got, ok)
	}
	if _, ok := reg.Get("bitbucket"); ok {
		t.Fatal("bitbucket no debería estar registrado")
	}

	all := reg.All()
	if len(all) != 2 || all[0].Forge() != "github" || all[1].Forge() != "gitlab" {
		t.Fatalf("All() = %v", all)
	}
}

func TestRegistryRegisterNilIgnored(t *testing.T) {
	reg := forge.NewRegistry()
	reg.Register(nil)
	if len(reg.All()) != 0 {
		t.Fatal("registrar nil no debería añadir nada")
	}
}

// TestCollectPagesThroughAllStreams cubre la paginación sin tope: agota las
// páginas de una lista y consulta las cuatro listas.
func TestCollectPagesThroughAllStreams(t *testing.T) {
	key := testutil.FakeKey{Section: model.SectionAuthored}
	fake := &testutil.FakeAdapter{
		ForgeName: "github",
		HostName:  "github.com",
		Pages: map[testutil.FakeKey][]forge.Page{
			key: {
				{Items: []model.Item{mkItem("github", "github.com", "acme/widget", 1)}, Next: "c1", More: true},
				{Items: []model.Item{mkItem("github", "github.com", "acme/widget", 2)}, More: false},
			},
		},
	}

	res := forge.Collect(context.Background(), fake)

	if len(res.Authored) != 2 {
		t.Fatalf("authored = %d, want 2 (dos páginas)", len(res.Authored))
	}
	if fake.ListCallCount() < len(forge.Streams) {
		t.Errorf("List se llamó %d veces, want >= %d", fake.ListCallCount(), len(forge.Streams))
	}
}

func TestCollectAuthFailureDoesNotDropData(t *testing.T) {
	fake := &testutil.FakeAdapter{
		ForgeName: "gitlab",
		HostName:  "gitlab.example.com",
		AuthState: model.AuthState{Forge: "gitlab", OK: false, Reason: "401"},
		Pages: map[testutil.FakeKey][]forge.Page{
			{Section: model.SectionAuthored}: {{Items: []model.Item{mkItem("gitlab", "gitlab.example.com", "grp/proj", 4)}}},
		},
	}

	res := forge.Collect(context.Background(), fake)

	if len(res.Authored) != 1 {
		t.Errorf("authored = %d, want 1", len(res.Authored))
	}
	assertKind(t, res.Warnings, "auth")
}

// TestStreamEmitsPages cubre la carga progresiva: la primera página llega
// marcada como First y las siguientes como continuación.
func TestStreamEmitsPages(t *testing.T) {
	key := testutil.FakeKey{Section: model.SectionReview, Kind: model.ReviewRequested}
	fake := &testutil.FakeAdapter{
		ForgeName: "github",
		HostName:  "github.com",
		Pages: map[testutil.FakeKey][]forge.Page{
			key: {
				{Items: []model.Item{mkItem("github", "github.com", "acme/widget", 1)}, Next: "c1", More: true},
				{Items: []model.Item{mkItem("github", "github.com", "acme/widget", 2)}, More: false},
			},
		},
	}

	var firsts, nexts int
	forge.Stream(context.Background(), fake, func(p forge.PageResult) bool {
		if p.Query.Section != model.SectionReview || p.Query.ReviewKind != model.ReviewRequested {
			return true // ignora las listas vacías
		}
		if len(p.Items) == 0 {
			return true
		}
		if p.First {
			firsts++
		} else {
			nexts++
		}
		return true
	})

	if firsts != 1 || nexts != 1 {
		t.Fatalf("firsts=%d nexts=%d, want 1 y 1", firsts, nexts)
	}
}

// TestStreamStopsWhenEmitReturnsFalse cubre el corte del refresco incremental:
// si emit pide parar tras la primera página, no se piden más.
func TestStreamStopsWhenEmitReturnsFalse(t *testing.T) {
	key := testutil.FakeKey{Section: model.SectionAuthored}
	fake := &testutil.FakeAdapter{
		ForgeName: "github",
		HostName:  "github.com",
		Pages: map[testutil.FakeKey][]forge.Page{
			key: {
				{Items: []model.Item{mkItem("github", "github.com", "acme/widget", 1)}, Next: "c1", More: true},
				{Items: []model.Item{mkItem("github", "github.com", "acme/widget", 2)}, More: false},
			},
		},
	}

	forge.Stream(context.Background(), fake, func(p forge.PageResult) bool {
		return p.Query.Section != model.SectionAuthored // corta en authored
	})

	if got := fake.ListCallCount(); got > len(forge.Streams) {
		t.Fatalf("List se llamó %d veces; el corte debería evitar la 2ª página de authored", got)
	}
}

func TestRunActionApproveOK(t *testing.T) {
	item := mkItem("github", "github.com", "acme/widget", 1)
	item.State = "OPEN"
	fake := &testutil.FakeAdapter{
		ForgeName:  "github",
		HostName:   "github.com",
		ItemStates: map[string]model.Item{testutil.ItemKey("acme/widget", 1): item},
	}

	out := forge.RunAction(context.Background(), fake, forge.ActionApprove, item.Ref, 1)
	if !out.OK || out.Conflict || out.Perm {
		t.Fatalf("outcome = %+v", out)
	}
	if !out.HasItem {
		t.Error("debería traer el estado releído")
	}
}

// TestRunActionConflictWhenMerged cubre "el ítem cambió en el forge entre
// refresco y acción": si ya está mergeado, no se ejecuta acción y se reporta
// conflicto con el estado releído.
func TestRunActionConflictWhenMerged(t *testing.T) {
	item := mkItem("github", "github.com", "acme/widget", 2)
	item.State = "MERGED"
	fake := &testutil.FakeAdapter{
		ForgeName:  "github",
		HostName:   "github.com",
		ItemStates: map[string]model.Item{testutil.ItemKey("acme/widget", 2): item},
	}

	out := forge.RunAction(context.Background(), fake, forge.ActionMerge, item.Ref, 2)
	if !out.Conflict || out.OK {
		t.Fatalf("outcome = %+v", out)
	}
	if !out.HasItem || out.Item.State != "MERGED" {
		t.Errorf("debería traer el estado releído: %+v", out.Item)
	}
}

func TestRunActionConflictWhenNotFound(t *testing.T) {
	item := mkItem("gitlab", "gitlab.example.com", "grp/proj", 3)
	fake := &testutil.FakeAdapter{
		ForgeName:     "gitlab",
		HostName:      "gitlab.example.com",
		StateWarnings: map[string][]model.Warning{testutil.ItemKey("grp/proj", 3): {{Forge: "gitlab", Kind: "notfound", Msg: "404"}}},
	}

	out := forge.RunAction(context.Background(), fake, forge.ActionApprove, item.Ref, 3)
	if !out.Conflict {
		t.Fatalf("outcome = %+v", out)
	}
}

func TestRunActionPermissionDisabled(t *testing.T) {
	item := mkItem("gitlab", "gitlab.example.com", "grp/proj", 4)
	fake := &testutil.FakeAdapter{
		ForgeName:      "gitlab",
		HostName:       "gitlab.example.com",
		ItemStates:     map[string]model.Item{testutil.ItemKey("grp/proj", 4): item},
		ActionWarnings: map[string][]model.Warning{"approve:grp/proj#4": {{Forge: "gitlab", Kind: "permission", Msg: "no tienes permiso"}}},
	}

	out := forge.RunAction(context.Background(), fake, forge.ActionApprove, item.Ref, 4)
	if !out.Perm || out.OK {
		t.Fatalf("outcome = %+v", out)
	}
}

func TestRunActionUnsupportedDisabled(t *testing.T) {
	item := mkItem("bitbucket", "bitbucket.org", "acme/widget", 5)
	fake := &testutil.FakeAdapter{
		ForgeName:      "bitbucket",
		HostName:       "bitbucket.org",
		ItemStates:     map[string]model.Item{testutil.ItemKey("acme/widget", 5): item},
		ActionWarnings: map[string][]model.Warning{"merge:acme/widget#5": {{Forge: "bitbucket", Kind: "unsupported", Msg: "no soportado"}}},
	}

	out := forge.RunAction(context.Background(), fake, forge.ActionMerge, item.Ref, 5)
	if !out.Perm {
		t.Fatalf("outcome = %+v", out)
	}
}

func assertKind(t *testing.T, warns []model.Warning, kind string) {
	t.Helper()
	for _, w := range warns {
		if w.Kind == kind {
			return
		}
	}
	t.Fatalf("no hay warning de tipo %q en %+v", kind, warns)
}

// TestCollectStopsOnRateLimit: ante un warning de rate limit, no se insiste con
// la página siguiente.
func TestCollectStopsOnRateLimit(t *testing.T) {
	key := testutil.FakeKey{Section: model.SectionAuthored}
	fake := &testutil.FakeAdapter{
		ForgeName: "github",
		HostName:  "github.com",
		Pages: map[testutil.FakeKey][]forge.Page{
			key: {
				{Items: []model.Item{mkItem("github", "github.com", "acme/widget", 1)}, Next: "c1", More: true},
				{Items: []model.Item{mkItem("github", "github.com", "acme/widget", 2)}, More: false},
			},
		},
		ListWarnings: map[testutil.FakeKey][]model.Warning{
			key: {{Forge: "github", Kind: "ratelimit", Msg: "429"}},
		},
	}

	res := forge.Collect(context.Background(), fake)

	if fake.ListCallCount() != len(forge.Streams) {
		t.Fatalf("List se llamó %d veces; con rate limit debería parar en la primera página", fake.ListCallCount())
	}
	assertKind(t, res.Warnings, "ratelimit")
}

// TestEscapeGraphQL comprueba que el escapado también cubre saltos de línea.
func TestEscapeGraphQL(t *testing.T) {
	in := "x\"y\\z\nw\tv\ru"
	out := forge.EscapeGraphQL(in)
	if strings.ContainsAny(out, "\n\t\r") {
		t.Fatalf("no debe quedar control crudo: %q", out)
	}
	for _, want := range []string{`\"`, `\\`, `\n`, `\t`, `\r`} {
		if !strings.Contains(out, want) {
			t.Errorf("falta el escape %q en %q", want, out)
		}
	}
}
