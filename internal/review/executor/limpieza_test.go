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

// Los dos caminos que faltan en `Mount` son los dos finales, y los dos hacen la misma
// pregunta: **¿qué se limpia cuando algo falla a mitad?**
//
// Y la asimetría es el contrato. Un fallo al CREAR el worktree está antes de que haya nada
// que dejar: se quita el clon bare que este mismo montaje acaba de hacer y nada más. Un fallo
// al REGISTRAR el review viene DESPUÉS de que el worktree exista y el layout esté montado, y
// nada de eso se puede deshacer: lo único que cabe es un aviso.
//
// Y esa asimetría es lo que hay que fijar. El reflejo simétrico del código es "si falla,
// limpia" en los dos casos, y en el segundo significaría tirar un worktree y un layout ya
// montados porque no se pudo escribir una línea en la memoria. El aviso es lo correcto: el
// review está montado y se puede usar, y lo que falta es el registro para encontrarlo al
// volver.

// resolverQueFallaAlRegistrar devuelve error solo en `RecordReview`, que es el punto que se
// quiere ejercitar. El resto se comporta bien para que el montaje llegue entero hasta el
// final: un fallo antes haría que no se llegara a la línea que se está probando, y un test
// verde por no haber llegado es peor que uno rojo.
type resolverQueFallaAlRegistrar struct {
	resolverConBare
	err       error
	registros []cache.ReviewRecord
}

func (r *resolverQueFallaAlRegistrar) RecordReview(_ model.Item, rec cache.ReviewRecord) error {
	r.registros = append(r.registros, rec)
	return r.err
}

// provisionerSano monta el worktree sin tocar el disco, y sabe simular que no puede.
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

// herdrQueMonta dice que sí y monta sin hacer nada, que es lo que hace falta para que el
// montaje llegue al registro.
type herdrQueMonta struct{}

func (herdrQueMonta) Available() bool { return true }
func (herdrQueMonta) MountLayout(context.Context, herdr.Container, plan.Plan) ([]string, error) {
	return nil, nil
}
func (herdrQueMonta) Notify(context.Context, string, herdr.NotifyOptions) error { return nil }

// TestSiNoSePuedeRegistrarElReviewSeAviadoYElReviewSeQuedaMontado: el aviso, no la limpieza.
//
// Y lo que se comprueba es que el aviso existe y que el worktree sigue ahí. La segunda mitad
// es la que importa: si el código limpiara, el usuario vería el review aparecer y desaparecer
// con un aviso, y no sabría si tiene que volver a montarlo.
//
// Y el aviso tiene que traer el motivo, porque "no se pudo registrar" sin el motivo de
// verdad no ayuda a nadie: puede ser el disco lleno —que es lo más probable— y eso se
// arregla distinto que un error de permisos.
func TestSiNoSePuedeRegistrarElReviewSeAviadoYElReviewSeQuedaMontado(t *testing.T) {
	res := &resolverQueFallaAlRegistrar{err: errors.New("no space left on device")}
	e := &Executor{Resolver: res, Worktrees: &provisionerSanco{}, Herdr: herdrQueMonta{}}

	got, err := e.Mount(context.Background(), itemParaMontar())
	if err != nil {
		t.Fatalf("un fallo al registrar no debe abortar el montaje: %v", err)
	}
	// Y el montaje está entero: worktree, plan y layout.
	if got.Worktree.Path == "" {
		t.Error("el worktree no quedó montado tras el fallo de registro")
	}
	if !got.Herdr {
		t.Error("el layout se desmontó por un fallo de registro: se montó y se tirar")
	}
	// Y hay un aviso que menciona el motivo.
	conMotivo := false
	for _, w := range got.Warnings {
		if strings.Contains(w, "no space left on device") {
			conMotivo = true
		}
	}
	if !conMotivo {
		t.Errorf("ningún aviso trae el motivo del fallo: %v", got.Warnings)
	}
	// Y el aviso tiene que distinguishable de los del layout: los de Herdr ya venían en
	// `res.Warnings` antes de registrar, y si el aviso del registro no se distinguiera, un
	// layout con warnings y un registro fallido se leerían como el mismo problema.
	if len(got.Warnings) != 1 {
		t.Errorf("avisos = %v, want solo el del registro: los del layout no hay", got.Warnings)
	}
	// Y el registro se INTENTÓ. Sin este aserto, un código que no llamara a `RecordReview` en
	// absoluto pasaría el test anterior: no habría error que avisar y el aviso no estaría.
	if len(res.registros) != 1 {
		t.Fatalf("se intentó registrar %d veces, want 1", len(res.registros))
	}
	// Y con lo que hay que registrar: repo, worktree, rama y etiqueta. Es lo que permite
	// encontrar el review al volver a abrir prdash.
	rec := res.registros[0]
	if rec.Worktree != got.Worktree.Path || rec.Branch == "" || rec.Label == "" {
		t.Errorf("el registro está incompleto: %+v", rec)
	}
}

// TestSiRegistrarVaBienNoHayAvisoNiLimpieza: el camino bueno, para que el anterior sea un
// contraste y no el único caso.
//
// Y es un test de una línea que compra una cosa: sin él, un código que metiera SIEMPRE un
// aviso de "no se pudo registrar" pasaría el test anterior sin que nadie lo notara.
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

// TestSiNoSePuedeCrearElWorktreeSeQuitaElBareSoloSiLoCreóEsteMontaje: la limpieza.
//
// Y el "solo si lo creó este montaje" es la parte que hay que mirar. Si un clon ya existía —
// el bare de otro review del mismo repo, que es lo normal con dos PRs del mismo proyecto—,
// borrarlo se llevaría por delante el clon del otro review porque este fallara.
//
// Y la aserción va en las dos direcciones porque las dos importan y una sola no probaría la
// guarda: un `cleanupBare` que no borrara nada dejaría un clon a medias por cada montaje
// fallido, y uno que borrara siempre se llevaría los prestados.
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
		// Y el resultado no parece un montaje: sin worktree no hay nada que usar.
		if got.Worktree.Path != "" {
			t.Errorf("%s: devolvió un worktree pese a fallar: %+v", c.nombre, got.Worktree)
		}
		// Y el error nombra el trabajo que falló, que es lo que hace falta para saber si
		// reintentar tiene sentido.
		if !strings.Contains(err.Error(), "worktree") {
			t.Errorf("%s: el error %q no dice que falló el worktree", c.nombre, err)
		}
		// Y el registro del layout: no se montó Herdr porque no había worktree que montar.
		if got.Herdr {
			t.Errorf("%s: se montó Herdr sin worktree", c.nombre)
		}
		// Y la limpieza es la que corresponde.
		if res.quitados == 1 && !c.quita {
			t.Errorf("%s: quitó el clon ajeno: se llevaría el de otra pestaña del mismo PR",
				c.nombre)
		}
		if res.quitados == 0 && c.quita {
			t.Errorf("%s: no quitó el clon que este montaje creó: queda a medias", c.nombre)
		}
	}
}

// resolverConBare dice si el clon ya existía y cuenta los `RemoveBare`.
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

// itemParaMontar es un ítem con lo mínimo para que el montaje llegue hasta el final.
func itemParaMontar() model.Item {
	it := model.NewItem(model.RepoRef{
		Forge: "github", Host: "github.com", Project: "o/r", Owner: "o", Name: "r",
	}, 7)
	it.SourceBranch = "feat/x"
	it.TargetBranch = "main"
	it.HeadSHA = "abc123"
	return it
}
