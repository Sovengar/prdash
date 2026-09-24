package forge_test

import (
	"context"
	"testing"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

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

// TestCollectAggregatesItemsAndWarnings comprueba que la consulta de un forge
// devuelve ítems y que los warnings quedan etiquetados con su sección.
func TestCollectAggregatesItemsAndWarnings(t *testing.T) {
	it := model.NewItem(model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget"}, 1)
	fake := &testutil.FakeAdapter{
		ForgeName:     "github",
		HostName:      "github.com",
		AuthoredItems: []model.Item{it},
		ReviewWarnings: []model.Warning{
			{Forge: "github", Kind: "network", Msg: "boom"},
		},
	}

	res := forge.Collect(context.Background(), fake)

	if res.Forge != "github" || res.Host != "github.com" {
		t.Errorf("forge/host = %s/%s", res.Forge, res.Host)
	}
	if len(res.Authored) != 1 {
		t.Errorf("authored = %d", len(res.Authored))
	}
	if len(res.Warnings) != 1 {
		t.Fatalf("warnings = %+v", res.Warnings)
	}
	if res.Warnings[0].Section != model.SectionReview {
		t.Errorf("warning debería quedar etiquetado a review: %+v", res.Warnings[0])
	}
}

// TestCollectAuthFailureDoesNotDropData comprueba que un fallo de auth no
// vacía el inbox: se registra el warning pero los ítems se conservan.
func TestCollectAuthFailureDoesNotDropData(t *testing.T) {
	it := model.NewItem(model.RepoRef{Forge: "gitlab", Host: "gitlab.example.com", Project: "grp/proj"}, 4)
	fake := &testutil.FakeAdapter{
		ForgeName:     "gitlab",
		HostName:      "gitlab.example.com",
		AuthState:     model.AuthState{Forge: "gitlab", OK: false, Reason: "401"},
		AuthoredItems: []model.Item{it},
	}

	res := forge.Collect(context.Background(), fake)

	if len(res.Authored) != 1 {
		t.Errorf("authored = %d, want 1", len(res.Authored))
	}
	hasAuth := false
	for _, w := range res.Warnings {
		if w.Kind == "auth" {
			hasAuth = true
		}
	}
	if !hasAuth {
		t.Errorf("faltó el warning de auth: %+v", res.Warnings)
	}
}
