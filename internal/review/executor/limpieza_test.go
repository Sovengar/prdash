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

// The two missing ends of Mount both ask the same question: what is cleaned when something is
//missing.

// It fails only on RecordReview, the point under test.
type resolverQueFallaAlRegistrar struct {
	resolverConBare
	err       error
	registros []cache.ReviewRecord
}

func (r *resolverQueFallaAlRegistrar) RecordReview(_ model.Item, rec cache.ReviewRecord) error {
	r.registros = append(r.registros, rec)
	return r.err
}

type provisionerSanco struct {
	fallaCrear bool
	creados    []worktree.Spec
}

func (p *provisionerSanco) Create(_ context.Context, s worktree.Spec) (worktree.Worktree, error) {
	p.creados = append(p.creados, s)
	if p.fallaCrear {
		return worktree.Worktree{}, errors.New("git worktree add: exit 128")
	}
	return worktree.Worktree{
		ID: s.Path, Label: s.Label, Path: s.Path, Branch: s.Branch, Repo: s.Repo,
	}, nil
}

func (p *provisionerSanco) List(context.Context) []worktree.Worktree { return nil }
func (p *provisionerSanco) Audit(context.Context) []worktree.Entry   { return nil }
func (p *provisionerSanco) Remove(context.Context, string) error     { return nil }
func (p *provisionerSanco) RemoveIfClean(context.Context, string) (bool, string, error) {
	return false, "", nil
}

type herdrQueMonta struct{}

func (herdrQueMonta) Available() bool { return true }
func (herdrQueMonta) MountLayout(context.Context, herdr.Container, plan.Plan) ([]string, error) {
	return nil, nil
}
func (herdrQueMonta) Notify(context.Context, string, herdr.NotifyOptions) error { return nil }

// What is checked is that the mount survives.
func TestSiNoSePuedeRegistrarElReviewSeAviadoYElReviewSeQuedaMontado(t *testing.T) {
	res := &resolverQueFallaAlRegistrar{err: errors.New("no space left on device")}
	e := &Executor{Resolver: res, Worktrees: &provisionerSanco{}, Herdr: herdrQueMonta{}}

	got, err := e.Mount(context.Background(), itemParaMontar())
	if err != nil {
		t.Fatalf("un fallo al registrar no debe abortar el montaje: %v", err)
	}
	if got.Worktree.Path == "" {
		t.Error("el worktree no quedó montado tras el fallo de registro")
	}
	if !got.Herdr {
		t.Error("el layout se desmontó por un fallo de registro: se montó y se tirar")
	}
	conMotivo := false
	for _, w := range got.Warnings {
		if strings.Contains(w, "no space left on device") {
			conMotivo = true
		}
	}
	if !conMotivo {
		t.Errorf("ningún aviso trae el motivo del fallo: %v", got.Warnings)
	}
	// The warning has to be distinguishable from the layout's: Herdr's already came in the result.
	if len(got.Warnings) != 1 {
		t.Errorf("avisos = %v, want solo el del registro: los del layout no hay", got.Warnings)
	}
	// The record was ATTEMPTED. Without this an implementation that never called RecordReview would
	// pass.
	if len(res.registros) != 1 {
		t.Fatalf("se intentó registrar %d veces, want 1", len(res.registros))
	}
	rec := res.registros[0]
	if rec.Worktree != got.Worktree.Path || rec.Branch == "" || rec.Label == "" {
		t.Errorf("el registro está incompleto: %+v", rec)
	}
}

// The good path, so the previous is a contrast and not the only case.
func TestSiRegistrarVaBienNoHayAvisoNiLimpieza(t *testing.T) {
	res := &resolverQueFallaAlRegistrar{} // sin error
	e := &Executor{Resolver: res, Worktrees: &provisionerSanco{}, Herdr: herdrQueMonta{}}

	got, err := e.Mount(context.Background(), itemParaMontar())
	if err != nil {
		t.Fatalf("Mount: %v", err)
	}
	if len(got.Warnings) != 0 {
		t.Errorf("un registro correcto avisa: %v", got.Warnings)
	}
	if len(res.registros) != 1 {
		t.Errorf("se registró %d veces, want 1", len(res.registros))
	}
}

// The "only if this mount created it" is the whole condition.
func TestSiNoSePuedeCrearElWorktreeSeQuitaElBareSoloSiLoCreóEsteMontaje(t *testing.T) {
	for _, c := range []struct {
		nombre    string
		teniaBare bool
		quita     bool
	}{
		{"clon nuevo que se quita", false, true},
		{"clon preexistente que se respeta", true, false},
	} {
		res := &resolverConBare{teniaBare: c.teniaBare}
		prov := &provisionerSanco{fallaCrear: true}
		e := &Executor{Resolver: res, Worktrees: prov, Herdr: herdrQueMonta{}}

		got, err := e.Mount(context.Background(), itemParaMontar())
		if err == nil {
			t.Errorf("%s: un fallo al crear el worktree dio nil", c.nombre)
			continue
		}
		if got.Worktree.Path != "" {
			t.Errorf("%s: devolvió un worktree pese a fallar: %+v", c.nombre, got.Worktree)
		}
		// The error names the work that failed, which is what tells whether retrying makes sense.
		if !strings.Contains(err.Error(), "worktree") {
			t.Errorf("%s: el error %q no dice que falló el worktree", c.nombre, err)
		}
		if got.Herdr {
			t.Errorf("%s: se montó Herdr sin worktree", c.nombre)
		}
		if res.quitados == 1 && !c.quita {
			t.Errorf("%s: quitó el clon ajeno: se llevaría el de otra pestaña del mismo PR",
				c.nombre)
		}
		if res.quitados == 0 && c.quita {
			t.Errorf("%s: no quitó el clon que este montaje creó: queda a medias", c.nombre)
		}
	}
}

type resolverConBare struct {
	reporesolverFalso
	teniaBare bool
	quitados  int
}

func (r *resolverConBare) HasBare(model.RepoRef) bool { return r.teniaBare }

func (r *resolverConBare) EnsureBare(context.Context, model.RepoRef) (string, error) {
	return "/clones/o/r.git", nil
}

func (r *resolverConBare) RemoveBare(model.RepoRef) error {
	r.quitados++
	return nil
}

func (r *resolverConBare) FetchReviewRef(context.Context, string, model.Item) (string, error) {
	return "prdash/pr-7", nil
}

func (r *resolverConBare) WorktreePath(model.RepoRef, int) string { return "/wt/prdash-pr-7" }

func itemParaMontar() model.Item {
	it := model.NewItem(model.RepoRef{
		Forge: "github", Host: "github.com", Project: "o/r", Owner: "o", Name: "r",
	}, 7)
	it.SourceBranch = "feat/x"
	it.TargetBranch = "main"
	it.HeadSHA = "abc123"
	return it
}
