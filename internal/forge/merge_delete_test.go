package forge_test

import (
	"context"
	"testing"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

// TestRunActionMergeAsksTheAdapterToDeleteTheBranch: lo que la Confirmación
// nombra tiene que llegar al adapter. Un toggle que se ve en pantalla y no
// cambia el argv es un toggle decorativo, y el borrado de una rama es de las
// pocas cosas de las que nadie se da cuenta hasta que es tarde.
func TestRunActionMergeAsksTheAdapterToDeleteTheBranch(t *testing.T) {
	for _, want := range []bool{true, false} {
		item := mkItem("github", "github.com", "acme/widget", 10)
		item.State = "OPEN"
		item.HeadSHA = "9f1c0de"
		fake := &testutil.FakeAdapter{
			ForgeName:  "github",
			HostName:   "github.com",
			ItemStates: map[string]model.Item{testutil.ItemKey("acme/widget", 10): item},
		}

		out := forge.RunAction(context.Background(), fake, forge.ActionMerge, item.Ref, 10,
			forge.MergeRequest{Mode: forge.Squash, DeleteBranch: want})
		if !out.OK {
			t.Fatalf("borrar=%v: outcome = %+v", want, out)
		}
		if out.DeleteBranch != want {
			t.Errorf("outcome.DeleteBranch = %v, want %v", out.DeleteBranch, want)
		}
		if got := fake.MergeDeleteCount(want); got != 1 {
			t.Errorf("el adapter recibió %d merges con DeleteBranch=%v, want 1", got, want)
		}
	}
}

// TestRunActionKeepsTheMergeWhenOnlyTheDeleteFails es el caso que hace
// necesario el pin de la relectura. El borrado va en el MISMO comando que el
// merge, así que cuando el forge lo rechaza la CLI sale con error aunque la
// integración ya esté hecha: sin push, con la rama protegida, o contra un repo
// con merge queue. Si eso se reportara como "merge falló", el usuario buscaría
// un cambio de estado del forge que no ocurrió, y la rama seguiría ahí sin que
// nadie lo dijera.
func TestRunActionKeepsTheMergeWhenOnlyTheDeleteFails(t *testing.T) {
	item := mkItem("github", "github.com", "acme/widget", 11)
	item.State = "OPEN"
	item.HeadSHA = "9f1c0de"
	fake := &testutil.FakeAdapter{
		ForgeName:  "github",
		HostName:   "github.com",
		ItemStates: map[string]model.Item{testutil.ItemKey("acme/widget", 11): item},
		ActionWarnings: map[string][]model.Warning{
			"merge:acme/widget#11": {{Forge: "github", Kind: "permission", Msg: "Resource not accessible by integration"}},
		},
	}
	// El merge sí salió: el ítem releído después viene mergeado, y eso es
	// justamente lo que separa "no se pudo mergear" de "se mergeó y la rama no se
	// borró".
	fake.OnMerge = func(it *model.Item) { it.State = "MERGED" }

	out := forge.RunAction(context.Background(), fake, forge.ActionMerge, item.Ref, 11,
		forge.MergeRequest{Mode: forge.Squash, DeleteBranch: true})
	if !out.OK {
		t.Fatalf("un merge que salió no puede reportarse como fallido: %+v", out)
	}
	if out.Perm {
		t.Error("no debe quedar registrado como denegado: el merge sí se puede hacer")
	}
	if out.DeleteMsg == "" {
		t.Error("falta el motivo por el que la rama no se borró")
	}
}

// TestRunActionSaysNothingAboutTheBranchWithoutThePin: sin headSHA el merge no
// sale, así que no hay rama que borrar y no hay nada que decir sobre ella. Un
// aviso de borrado ahí sería ruido sobre una acción que no ocurrió.
func TestRunActionSaysNothingAboutTheBranchWithoutThePin(t *testing.T) {
	// Sin HeadSHA el adapter real se niega (los dos lo hacen), y el fake copia
	// esa negativa: sin pin no hay merge, y por tanto no hay rama que borrar.
	item := mkItem("github", "github.com", "acme/widget", 12)
	item.State = "OPEN"
	fake := &testutil.FakeAdapter{
		ForgeName:  "github",
		HostName:   "github.com",
		ItemStates: map[string]model.Item{testutil.ItemKey("acme/widget", 12): item},
	}

	out := forge.RunAction(context.Background(), fake, forge.ActionMerge, item.Ref, 12,
		forge.MergeRequest{Mode: forge.Squash, DeleteBranch: true})
	if out.OK {
		t.Fatalf("sin pin no puede haber merge: %+v", out)
	}
	if out.DeleteMsg != "" {
		t.Errorf("DeleteMsg = %q, want vacío: no hubo merge", out.DeleteMsg)
	}
}

// TestRunActionDoesNotBlamTheBranchOnAForkPR: un PR de fork no tiene rama que
// borrar en el repo destino y el forge no protesta —lo da por hecho y sale con
// éxito—, así que el aviso tiene que decirlo. Sin esto, "branch deleted" sería
// mentira en exactamente los PRs de los que más conviene fiarse.
func TestRunActionDoesNotBlamTheBranchOnAForkPR(t *testing.T) {
	item := mkItem("github", "github.com", "acme/widget", 13)
	item.State = "OPEN"
	item.HeadSHA = "9f1c0de"
	item.IsFork = true
	fake := &testutil.FakeAdapter{
		ForgeName:  "github",
		HostName:   "github.com",
		ItemStates: map[string]model.Item{testutil.ItemKey("acme/widget", 13): item},
		OnMerge:    func(it *model.Item) { it.State = "MERGED" },
	}

	out := forge.RunAction(context.Background(), fake, forge.ActionMerge, item.Ref, 13,
		forge.MergeRequest{Mode: forge.Squash, DeleteBranch: true})
	if !out.OK {
		t.Fatalf("outcome = %+v", out)
	}
	if out.DeleteMsg == "" {
		t.Error("un PR de fork no puede reportarse como rama borrada")
	}
}
