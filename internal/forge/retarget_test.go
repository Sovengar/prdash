package forge_test

import (
	"context"
	"strings"
	"testing"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

func retargetFixture(t *testing.T, it model.Item) *testutil.FakeAdapter {
	t.Helper()
	a := &testutil.FakeAdapter{
		ForgeName:  it.Forge,
		HostName:   it.Host,
		ItemStates: map[string]model.Item{},
	}
	a.ItemStates[testutil.ItemKey(it.Ref.Project, it.Number)] = it
	return a
}

func retargetItem(state string) model.Item {
	it := model.NewItem(model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget", Owner: "acme", Name: "widget"}, 7)
	it.Title = "Add widget"
	it.SourceBranch = "feat/x"
	it.TargetBranch = "main"
	it.State = state
	return it
}

// The path is the same as the other actions —re-read, check, act, re-read—.
func TestRunRetargetCambiaLaBaseYRelee(t *testing.T) {
	it := retargetItem("OPEN")
	a := retargetFixture(t, it)

	out := forge.RunRetarget(context.Background(), a, it.Ref, it.Number, "release/2.0")
	if !out.OK {
		t.Fatalf("RunRetarget = %+v, want OK", out)
	}
	if a.RetargetCount() != 1 || a.Retargets[0] != "release/2.0" {
		t.Errorf("el adapter recibió %v, want una llamada con release/2.0", a.Retargets)
	}
	if out.Base != "release/2.0" {
		t.Errorf("Outcome.Base = %q, want la rama pedida para el aviso", out.Base)
	}
	if !out.HasItem {
		t.Error("Outcome no trae el ítem releído: la vista se quedaría con la base vieja")
	}
}

func TestRunRetargetNiegaSinRama(t *testing.T) {
	it := retargetItem("OPEN")
	a := retargetFixture(t, it)

	out := forge.RunRetarget(context.Background(), a, it.Ref, it.Number, "   ")
	if !out.Perm {
		t.Errorf("Outcome = %+v, want Perm: un flag de base vacío deja el PR sin base", out)
	}
	if out.Msg != forge.ErrMissingBaseBranch.Error() {
		t.Errorf("Msg = %q, want el motivo canónico", out.Msg)
	}
	if out.HasItem {
		t.Error(" releó el ítem para nada: sin rama no hay acción que guardar")
	}
	if a.RetargetCount() != 0 {
		t.Errorf("llamó al adapter (%v) sin rama que enviar", a.Retargets)
	}
}

func TestRunRetargetRespetaElGuardDeAbierto(t *testing.T) {
	for _, state := range []string{"MERGED", "CLOSED"} {
		it := retargetItem(state)
		a := retargetFixture(t, it)

		out := forge.RunRetarget(context.Background(), a, it.Ref, it.Number, "release/2.0")
		if !out.Conflict {
			t.Errorf("RunRetarget sobre %s = %+v, want conflicto", state, out)
		}
		if a.RetargetCount() != 0 {
			t.Errorf("llamó al adapter sobre un ítem %s", state)
		}
	}
}

func TestRunRetargetClasificaElPermisoDelForge(t *testing.T) {
	it := retargetItem("OPEN")
	a := retargetFixture(t, it)
	a.ActionWarnings = map[string][]model.Warning{
		"retarget:" + testutil.ItemKey(it.Ref.Project, it.Number): {
			{Forge: "github", Kind: "permission", Msg: "must have push access"},
		},
	}

	out := forge.RunRetarget(context.Background(), a, it.Ref, it.Number, "release/2.0")
	if !out.Perm {
		t.Errorf("Outcome = %+v, want Perm por falta de push", out)
	}
	if !strings.Contains(out.Msg, "push access") {
		t.Errorf("Msg = %q, want el motivo del forge", out.Msg)
	}
}
