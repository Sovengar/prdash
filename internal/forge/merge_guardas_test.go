package forge_test

import (
	"context"
	"strings"
	"testing"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/state"
	"prdash/internal/testutil"
)

// The two branches that are the boundary between forge and the TUI.

// The HeadSHA that travels is the one from the re-read, not the card's.
func TestRunActionDelMergePasaElHeadSHAReleidoYNoElDeLaFicha(t *testing.T) {
	visto := mkItem("github", "github.com", "acme/widget", 3)
	visto.State = "OPEN"
	visto.HeadSHA = "sha-que-el-usuario-vio"

	releido := visto
	releido.HeadSHA = "sha-nuevo-del-refresco"

	fake := adapterQuePideElEstado(releido)
	req := forge.MergeRequest{Mode: forge.Squash, DeleteBranch: true}
	out := forge.RunAction(context.Background(), fake, forge.ActionMerge, visto.Ref, 3, req)

	if !fake.seLlamo {
		t.Error("el merge no llegó a Merge: no hay caso bueno contra el que comparar")
	}
	if out.OK {
		t.Errorf("el fake no fusiona y el resultado salió bien: %+v", out)
	}
	if out.Mode != forge.Squash || !out.DeleteBranch {
		t.Errorf("la petición salió con modo %q y borrado %v: son lo que el usuario eligió",
			out.Mode, out.DeleteBranch)
	}
	if !out.HasItem {
		t.Error("el resultado no trae el estado releído: la TUI no puede refrescar la fila")
	}
	if out.Item.HeadSHA != "sha-nuevo-del-refresco" {
		t.Errorf("el estado releído trae SHA %q: se aplicaría sobre una ficha vieja",
			out.Item.HeadSHA)
	}
}

// The two states that really block, and it is convenient that they are only two.
func TestUnPRYaFusionadoOCerradoBloqueaElMergeSinLlamarAlForge(t *testing.T) {
	for _, c := range []struct {
		nombre string
		estado string
	}{
		{"ya fusionado", "MERGED"},
		{"ya cerrado", "CLOSED"},
	} {
		it := mkItem("github", "github.com", "acme/widget", 4)
		it.State = c.estado
		it.HeadSHA = "abc123"
		fake := adapterQuePideElEstado(it)

		out := forge.RunAction(context.Background(), fake, forge.ActionMerge, it.Ref, 4,
			forge.MergeRequest{Mode: forge.MergeCommit})

		if out.OK {
			t.Errorf("%s: el merge salió bien sobre un PR %q", c.nombre, c.estado)
		}
		if fake.seLlamo {
			t.Errorf("%s: se llamó a Merge: un merge lanzado sobre un PR %q puede "+
				"integrar medio sin que nadie lo sepa", c.nombre, c.estado)
		}
		if !out.Conflict {
			t.Errorf("%s: no se marcó como conflicto (%+v): la TUI no ofrecería refrescar",
				c.nombre, out)
		}
		// The reason IS the reason: "already merged", not a generic "something failed".
		if !strings.Contains(out.Msg, "already") {
			t.Errorf("%s: el motivo %q no dice que el PR ya está %s", c.nombre, out.Msg, c.estado)
		}
		if !out.HasItem || out.Item.Number != 4 {
			t.Errorf("%s: el resultado no trae el ítem re-leído", c.nombre)
		}
	}

	it := mkItem("github", "github.com", "acme/widget", 40)
	it.State = "DIRTY"
	it.HeadSHA = "abc123"
	fake := adapterQuePideElEstado(it)
	if out := forge.RunAction(context.Background(), fake, forge.ActionMerge, it.Ref, 40,
		forge.MergeRequest{Mode: forge.MergeCommit}); !fake.seLlamo || out.OK {
		t.Errorf("un PR con las ramas en conflicto no llegó a Merge, o salió bien: "+
			"llamado=%v out=%+v. El bloqueo blando lo aplica MergeBlock, no RunAction",
			fake.seLlamo, out)
	}
}

// The test documenting a bug: an unknown action returned OK:true without doing anything.
func TestUnaAccionQueNoEsNiApproveNiMergeNoSeDespachaYNoSeLeeElItem(t *testing.T) {
	for _, kind := range []forge.ActionKind{
		forge.ActionRetarget, forge.ActionKind("inventado"), forge.ActionKind(""),
	} {
		it := mkItem("github", "github.com", "acme/widget", 8)
		it.State = "OPEN"
		it.HeadSHA = "abc123"
		fake := adapterQuePideElEstado(it)
		fake.leeStates = 0

		out := forge.RunAction(context.Background(), fake, kind, it.Ref, 8,
			forge.MergeRequest{Mode: forge.MergeCommit})

		if out.OK {
			t.Errorf("la acción %q salió bien sin hacerse: el aviso de la cabecera diría "+
				"que la operación funcionó", kind)
		}
		if fake.seLlamo {
			t.Errorf("la acción %q llegó a Merge", kind)
		}
		if fake.leeStates != 0 {
			t.Errorf("la acción %q releyó el ítem %d veces: una acción imposible no debe "+
				"gastar un viaje al forge", kind, fake.leeStates)
		}
		// The reason is canonical and NAMES the action, so whoever debugs does not have to guess.
		if !strings.Contains(out.Msg, "does not implement") {
			t.Errorf("la acción %q dio el motivo %q, que no dice que no está implementada", kind, out.Msg)
		}
		if !strings.Contains(out.Msg, string(kind)) {
			t.Errorf("la acción %q dio el motivo %q, que no la nombra", kind, out.Msg)
		}
		// Neither a conflict ("the item changed, refresh") nor a permission.
		if out.Conflict || out.Perm || out.Unmergeable {
			t.Errorf("la acción %q se clasificó como %+v: no es un conflicto ni un permiso, "+
				"es un fallo de quién llamó", kind, out)
		}
		if out.Kind != kind {
			t.Errorf("el Kind del resultado es %q, want %q", out.Kind, kind)
		}
		if out.ID.Project != "acme/widget" || out.ID.Number != 8 {
			t.Errorf("el ID del resultado es %+v: el aviso no se podría atribuir al ítem", out.ID)
		}
	}

	it := mkItem("github", "github.com", "acme/widget", 9)
	it.State = "OPEN"
	bueno := adapterQuePideElEstado(it)
	bueno.leeStates = 0
	forge.RunAction(context.Background(), bueno, forge.ActionApprove, it.Ref, 9,
		forge.MergeRequest{})
	if bueno.leeStates == 0 {
		t.Error("con approve no se releyó el ítem: el guard no es lo que cortó, y el estado " +
			"del inbox se quedaría sin refrescar")
	}
}

// A veto that does not depend on the forge: nobody can approve their own.
func TestAprobarLoPropioSeRechazaConMotivo(t *testing.T) {
	it := mkItem("github", "github.com", "acme/widget", 7)
	it.State = "OPEN"
	it.HeadSHA = "abc123"
	it.Author = "yo-mismo"
	fake := adapterQuePideElEstado(it)
	fake.approveVeto = true

	out := forge.RunAction(context.Background(), fake, forge.ActionApprove, it.Ref, 7,
		forge.MergeRequest{})
	if out.OK {
		t.Fatal("se aprobó el propio PR")
	}
	if strings.TrimSpace(out.Msg) == "" {
		t.Fatal("sin motivo: el usuario ve que la tecla no hace nada")
	}
	// The reason is the CANONICAL one, not the CLI's text: the adapter may say whatever it wants
	// —the text changes between versions—.
	if out.Msg != state.SelfReviewReason {
		t.Errorf("el motivo es %q, want el canónico %q", out.Msg, state.SelfReviewReason)
	}
	// Classified as PERMISSION and not as a conflict, which is what stops the TUI from asking for
	// a refresh that fixes nothing.
	if !out.Perm {
		t.Errorf("el veto salió como %+v: sin la marca de permiso la TUI lo reintentaría "+
			"en cada refresco", out)
	}
	if out.Conflict {
		t.Error("el veto salió además como conflicto: dos clases a la vez y la TUI no sabe " +
			"cuál ofrecer")
	}
}

// It embeds the project's FakeAdapter and only records whether a merge was asked for.
// Embedding over a struct means the real methods still work.
type adapterQueCuestaSiSeMergeo struct {
	testutil.FakeAdapter
	seLlamo     bool
	leeStates   int
	approveVeto bool
}

func (a *adapterQueCuestaSiSeMergeo) Approve(
	_ context.Context, _ model.RepoRef, _ int,
) []model.Warning {
	if a.approveVeto {
		return []model.Warning{{Forge: a.ForgeName, Kind: "selfreview", Msg: "you cannot approve"}}
	}
	return a.FakeAdapter.Approve(context.Background(), model.RepoRef{}, 0)
}

func (a *adapterQueCuestaSiSeMergeo) ItemState(
	ctx context.Context, ref model.RepoRef, number int,
) (model.Item, []model.Warning) {
	a.leeStates++
	return a.FakeAdapter.ItemState(ctx, ref, number)
}

func (a *adapterQueCuestaSiSeMergeo) Merge(
	_ context.Context, _ model.RepoRef, _ int, _ forge.MergeRequest,
) []model.Warning {
	a.seLlamo = true
	return []model.Warning{{Forge: a.ForgeName, Kind: "denegado", Msg: "el fake no fusiona"}}
}

func adapterQuePideElEstado(it model.Item) *adapterQueCuestaSiSeMergeo {
	return &adapterQueCuestaSiSeMergeo{
		FakeAdapter: testutil.FakeAdapter{
			ForgeName:  it.Forge,
			HostName:   it.Host,
			ItemStates: map[string]model.Item{testutil.ItemKey(it.Ref.Project, it.Number): it},
		},
	}
}
