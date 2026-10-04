package testutil

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
)

// The double records the format and the args SEPARATELY, which is what lets it check that aborta
// passes the args instead of concatenating them.
type reporteDePrueba struct {
	helper  int
	fatales []string
	errores []string
}

func (r *reporteDePrueba) Helper() { r.helper++ }

func (r *reporteDePrueba) Fatalf(format string, args ...any) {
	r.fatales = append(r.fatales, fmt.Sprintf(format, args...))
}

func (r *reporteDePrueba) Error(args ...any) {
	r.errores = append(r.errores, fmt.Sprint(args...))
}

func (r *reporteDePrueba) cumplio() bool { return len(r.fatales) == 0 && len(r.errores) == 0 }

// Three things in one line of code.
func TestAbortaAverteConElPrefijoYConElErrorEntero(t *testing.T) {
	tranquilo := &reporteDePrueba{}
	aborta(tranquilo, nil)
	if !tranquilo.cumplio() {
		t.Errorf("aborta sin error avisó: %v / %v", tranquilo.fatales, tranquilo.errores)
	}
	if tranquilo.helper == 0 {
		t.Error("aborta no llamó a Helper: el fallo se atribuiría a la línea del helper y no " +
			"a la del test")
	}

	conFallo := &reporteDePrueba{}
	aborta(conFallo, errors.New("mkdir: permission denied"))
	if len(conFallo.fatales) != 1 {
		t.Fatalf("aborta,%d avisos, want 1: %v", len(conFallo.fatales), conFallo.fatales)
	}
	quiere := "testutil: mkdir: permission denied"
	if conFallo.fatales[0] != quiere {
		t.Errorf("el aviso es %q, want %q", conFallo.fatales[0], quiere)
	}
	if len(conFallo.errores) != 0 {
		t.Errorf("aborta con error fatal también escribió un Error: %v", conFallo.errores)
	}
}

// What is checked is that fn runs ONCE, not that it warns: an idempotent mkdir is fine but a
// git init is not.
func TestAbortaConPasaElErrorDeLaFuncionYNoLaVuelveALlamar(t *testing.T) {
	llamadas := 0
	tranquilo := &reporteDePrueba{}
	abortaCon(tranquilo, func() error {
		llamadas++
		return nil
	})
	if !tranquilo.cumplio() {
		t.Errorf("abortaCon con fn sin error avisó: %v / %v", tranquilo.fatales, tranquilo.errores)
	}
	if llamadas != 1 {
		t.Errorf("la función se ejecutó %d veces, want 1", llamadas)
	}

	llamadas = 0
	conFallo := &reporteDePrueba{}
	abortaCon(conFallo, func() error {
		llamadas++
		return errors.New("git init: el directorio existe")
	})
	if len(conFallo.fatales) != 1 {
		t.Fatalf("abortaCon,%d avisos, want 1: %v", len(conFallo.fatales), conFallo.fatales)
	}
	if !strings.Contains(conFallo.fatales[0], "testutil: git init") {
		t.Errorf("el aviso %q no trae el prefijo ni el error", conFallo.fatales[0])
	}
	if llamadas != 1 {
		t.Errorf("la función se ejecutó %d veces, want 1: un `git init` repetido se encuentra "+
			"el directorio lleno y el aviso sería de otro fallo", llamadas)
	}
}

// "Exactly" is the word that matters: a gate that reported everything in one Error would also
// turn the test red, and a gate that reported nothing would let a broken adapter pass.
func TestLaPuertaDeConformidadRegistraUnErrorPorIncumplimientoYNoLosTomaPorBuenos(t *testing.T) {
	roto := &FakeAdapter{
		ForgeName: "roto",
		HostName:  "roto.example.com",
		Pages: map[FakeKey][]forge.Page{
			{Section: model.SectionReview}: {
				{Items: []model.Item{model.NewItem(model.RepoRef{Project: "acme/widget"}, 1)}},
			},
		},
	}

	for _, opts := range []ConformanceOptions{
		{Unsupported: true},
		{MissingBinary: true},
	} {
		esperados := ConformanceViolations(roto, opts)
		if len(esperados) == 0 {
			t.Fatalf("ConformanceViolations con %+v no encontró nada: el fixture no incumple y "+
				"este test no probaría nada", opts)
		}

		d := &reporteDePrueba{}
		RunConformance(d, roto, opts)

		if len(d.errores) != len(esperados) {
			t.Errorf("%+v: la puerta registró %d errores y hay %d incumplimientos:\n%s",
				opts, len(d.errores), len(esperados), strings.Join(d.errores, "\n"))
		}
		if len(d.fatales) != 0 {
			t.Errorf("%+v: la puerta abortó el test en vez de registrar: %v", opts, d.fatales)
		}
		// Every violation arrives, and each ONE time.
		for _, e := range esperados {
			n := 0
			for _, registrada := range d.errores {
				if registrada == e {
					n++
				}
			}
			if n != 1 {
				t.Errorf("%+v: el incumplimiento %q se registró %d veces, want 1", opts, e, n)
			}
		}
	}
}

// The half that is not the failure.
func TestLaPuertaSeCallaConUnAdapterQueCumpleYMarcaQueEsUnHelper(t *testing.T) {
	bueno := adapterInerte()
	opts := ConformanceOptions{Unsupported: true}
	if v := ConformanceViolations(bueno, opts); len(v) != 0 {
		t.Fatalf("el fixture no cumple: %v", v)
	}

	d := &reporteDePrueba{}
	RunConformance(d, bueno, opts)
	if !d.cumplio() {
		t.Errorf("la puerta dijo algo de un adapter que cumple: %v / %v", d.fatales, d.errores)
	}
	if d.helper == 0 {
		t.Error("la puerta no llamó a Helper: un fallo se atribuiría a la puerta y no al adapter")
	}
}

// The violations that are not the opts'.
func TestLaPuertaConUnQueNoCumpleNoSeSaltaElForgeNiElHost(t *testing.T) {
	sinNombre := &FakeAdapter{}
	v := ConformanceViolations(sinNombre, ConformanceOptions{Unsupported: true})
	if len(v) == 0 {
		t.Fatal("ConformanceViolations no vio que el adapter no tiene ni forge ni host")
	}

	if len(v) < 2 || v[0] != "Forge() vacío" || v[1] != "Host() vacío" {
		t.Fatalf("las dos primeras entradas no son las del forge y el host: %v", v[:min(4, len(v))])
	}

	// The rest start with ": ", because the list formats with a.Forge() and an empty forge prints
	//empty. My first version assumed the code avoided that and the test failed: it does not.
	for _, e := range v[2:] {
		if !strings.HasPrefix(e, ": ") {
			t.Errorf("el incumplimiento %q no lleva el hueco del forge vacío", e)
		}
	}

	d := &reporteDePrueba{}
	RunConformance(d, sinNombre, ConformanceOptions{Unsupported: true})
	if len(d.errores) != len(v) {
		t.Errorf("la puerta registró %d y ConformanceViolations devolvió %d", len(d.errores), len(v))
	}

	if len(forge.Streams) == 0 {
		t.Fatal("forge.Streams está vacío: la puerta no tiene consultas que comprobar")
	}
	if ctx := context.Background(); ctx == nil {
		t.Fatal("nil")
	}
}
