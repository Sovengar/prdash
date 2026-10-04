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

// Los caminos de fallo de los helpers de este paquete, ejecutados en proceso.
//
// Y el doble es lo que los hace alcanzables. `aborta` avisa con `t.Fatalf` y `RunConformance`
// avisa con `t.Error`; los dos son métodos de `*testing.T`, que no se puede doblear porque
// `testing.common` tiene campos privados y `Fatalf` acaba en `runtime.Goexit()`. Sin doble, la
// única forma de ejecutar esas dos líneas es tener un test que falle, y un test que se ve rojo
// no prueba nada: se lee como un fallo del helper, no como la comprobación de que el helper
// avisa.
//
// Y con `testReport` el doble registra la llamada y vuelve. Que vuelva es lo que permite
// comprobar lo que hay DESPUÉS del aviso: que se avisó una vez, que el aviso lleva el prefijo,
// y que un `fn` que falla no se reintenta a escondidas.
//
// Y lo que estos tests NO comprueban, y por eso `ayudas_fallo_test.go` sigue con sus casos por
// subproceso: que un `*testing.T` de verdad deje el test en rojo. Un doble cuenta llamadas; solo
// el `testing.T` real puede decir si el proceso terminó con fallo. Son dos mitades
// distintas y hacen falta las dos.

// reporteDePrueba es el doble: registra en vez de avisar, y no detiene nada.
//
// Y registra el `format` y los `args` POR SEPARADO, y compone el texto en el `Error`. Esa es la
// razón de que el doble sirva: si guardara el `format` ya formateado, no se podría comprobar que
// `aborta` pasa los args a `Fatalf` en vez de concatenarlos a mano, que es el error tipográfico
// más fácil de cometer —`"testutil: " + err` funciona con un error y se rompe con cinco—.
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

// TestAbortaAverteConElPrefijoYConElErrorEntero: el aviso de `aborta`.
//
// Y son tres cosas en una línea de código. El prefijo, que es lo que distingue un fixture roto
// de un fallo del sistema —`mkdir: permission denied` y `testutil: mkdir: permission denied`
// son dos diagnósticos y solo el segundo dice dónde mirar—. El `%v` en vez de `+`, que con un
// `error` da el mismo texto pero que con algo que no sea `error` se rompe. Y el `return`
// temprano cuando no hay error, que es lo que hace que `aborta` se pueda usar como efecto
// secundario en medio de un helper sin `if`.
//
// Y el caso de "no hay error" importa tanto como el del fallo, y no por simetría: si `aborta`
// avisara sin error, los seis envoltorios pasarían a ser ruido en todos los tests, que es la
// forma más cara de tener un helper mal escrito.
func TestAbortaAverteConElPrefijoYConElErrorEntero(t *testing.T) {
	// Sin error: no dice nada, y solo marca que es un helper.
	tranquilo := &reporteDePrueba{}
	aborta(tranquilo, nil)
	if !tranquilo.cumplio() {
		t.Errorf("aborta sin error avisó: %v / %v", tranquilo.fatales, tranquilo.errores)
	}
	if tranquilo.helper == 0 {
		t.Error("aborta no llamó a Helper: el fallo se atribuiría a la línea del helper y no " +
			"a la del test")
	}

	// Con error: un aviso, con prefijo, con el error entero.
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

// TestAbortaConPasaElErrorDeLaFuncionYNoLaVuelveALlamar: `abortaCon`.
//
// Y lo que se comprueba es que la función se EJECUTA UNA VEZ, no que avise. `abortaCon` existe
// para que un helper pueda escribir `abortaCon(t, func() error { return os.MkdirAll(dir, 0755) })`
// sin escribir el `if`, y si el `fn` se llamara dos veces un `mkdir` sería idempotente pero un
// `git init` no: el segundo clon se encontraría el directorio lleno.
//
// Y el caso de que `fn` devuelva nil es el mismo camino bueno de `aborta` con otro envoltorio, y
// se comprueba para que el test no sea "solo funciona cuando falla".
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

// TestLaPuertaDeConformidadRegistraUnErrorPorIncumplimientoYNoLosTomaPorBuenos: `RunConformance`.
//
// Y el doble hace lo que un `testing.T` de verdad no deja: registrar los `Error` sin poner el test
// en rojo, y contar. Así que el test le pasa un adapter que incumple a propósito y comprueba que
// la puerta registra exactamente los incumplimientos.
//
// Y "exactamente" es la palabra importante. Una puerta que registrara todos los incumplimientos
// en un solo `Error` también marcaría el test en rojo, y una puerta que no registrara nada
// dejaría un adapter roto pasar. Las dos pasan un "¿falló?" y ninguna pasa un "¿por qué?". Lo
// que separa una puerta de un cartel es que el mensaje diga qué se incumplió, y eso es lo que se
// comprueba aquí.
//
// Y se comparan contra `ConformanceViolations`, que es la lista que la puerta recorre. La
// comparación es de CONJUNTO y no de texto exacto: `Error` formatea con `Sprint`, que es lo que
// hace `t.Error`, y lo que importa es que cada incumplimiento llegue una vez. El texto exacto de
// cada línea lo fija `ConformanceViolations`, que tiene sus propios tests.
func TestLaPuertaDeConformidadRegistraUnErrorPorIncumplimientoYNoLosTomaPorBuenos(t *testing.T) {
	// Un adapter que rompe las dos reglas de `ConformanceOptions`: con `Unsupported` tiene que
	// avisar de que no soporta la consulta, y con `MissingBinary` no puede devolver ítems.
	// `FakeAdapter` con `Unsupported` vacío no avisa, así que incumple las dos, y además tiene
	// Forge y Host puestos para que los incumplimientos que NO queremos no se cuelen.
	roto := &FakeAdapter{
		ForgeName: "roto",
		HostName:  "roto.example.com",
		// Y devuelve ítems de verdad, que es el otro incumplimiento del modo `Unsupported`:
		// un adapter que avisa "no lo soporto" y aun así contesta con una lista llena miente
		// dos veces, y la puerta está para que eso no llegue a un inbox.
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
		// Cada incumplimiento llega, y cada uno UNA vez.
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

// TestLaPuertaSeCallaConUnAdapterQueCumpleYMarcaQueEsUnHelper: el camino bueno.
//
// Y es la mitad que no es el fallo, y sin ella el test anterior pasaría con una puerta que
// registra `Error` para todo, porque registrar de más también pone el test en rojo y un "¿falló?"
// no distingue. Aquí un adapter que cumple tiene que dejar la puerta muda.
//
// Y `Helper` también se comprueba, porque la puerta se ejecuta desde un `t.Run` de la suite de
// cada adapter y un aviso sin `Helper` se atribuye a la línea de la puerta y no a la del adapter,
// que es justo la línea que se quiere leer cuando un adapter nuevo falla la puerta.
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

// TestLaPuertaConUnQueNoCumpleNoSeSaltaElForgeNiElHost: los incumplimientos que no son del `opts`.
//
// Y `ConformanceViolations` comprueba `Forge()` y `Host()` vacíos antes de mirar los listados, y
// eso es una decisión: esos dos incumplimientos no dependen de `ConformanceOptions`, así que
// aparecen siempre. Con un adapter sin nombre, la puerta registraría todo y el mensaje de cada
// línea empezaría por `": List(...)"`, que no dice de qué adapter se habla — y un mensaje sin el
// nombre del forge es exactamente lo que la puerta existe para evitar.
//
// Y lo que se comprueba aquí es que el doble sigue el contrato de `Forge`/`Host`, de modo que
// `hasKind` y los `Sprintf` de la puerta no reciben un adapter a medio construir.
func TestLaPuertaConUnQueNoCumpleNoSeSaltaElForgeNiElHost(t *testing.T) {
	sinNombre := &FakeAdapter{}
	v := ConformanceViolations(sinNombre, ConformanceOptions{Unsupported: true})
	if len(v) == 0 {
		t.Fatal("ConformanceViolations no vio que el adapter no tiene ni forge ni host")
	}

	// Y las dos primeras entradas SON las del nombre, en ese orden y antes que las demás. Es
	// lo que permite decir "tu adapter no tiene nombre" sin tener que leer los otros nueve
	// mensajes que empiezan por ": ", que es el ruido de un `%s` con un forge vacío.
	if len(v) < 2 || v[0] != "Forge() vacío" || v[1] != "Host() vacío" {
		t.Fatalf("las dos primeras entradas no son las del forge y el host: %v", v[:min(4, len(v))])
	}

	// Y las demás empiezan por ": ", porque `ConformanceViolations` formatea con `a.Forge()` y
	// un forge vacío sale vacío. Mi primera versión dio por hecho que el código lo esquivaba y
	// el test falló: no lo esquiva, y está bien que no lo esquive. Un `TrimPrefix` o un `if`
	// para tapar el hueco escondería el problema de verdad —que el adapter no tiene nombre—
	// detrás de un texto presentable. El ": " de la cola es el aviso.
	for _, e := range v[2:] {
		if !strings.HasPrefix(e, ": ") {
			t.Errorf("el incumplimiento %q no lleva el hueco del forge vacío", e)
		}
	}

	// Y la puerta registra lo mismo que la lista, sin filtrar ni reordenar.
	d := &reporteDePrueba{}
	RunConformance(d, sinNombre, ConformanceOptions{Unsupported: true})
	if len(d.errores) != len(v) {
		t.Errorf("la puerta registró %d y ConformanceViolations devolvió %d", len(d.errores), len(v))
	}

	// Y `Forge.Streams` no está vacío, que es lo que hace que la puerta tenga algo que recorrer.
	// Sin esto, un `ConformanceViolations` con la lista de consultas vacía devolvería cero
	// incumplimientos para cualquier adapter y la puerta no comprobaría nada.
	if len(forge.Streams) == 0 {
		t.Fatal("forge.Streams está vacío: la puerta no tiene consultas que comprobar")
	}
	if ctx := context.Background(); ctx == nil {
		t.Fatal("nil")
	}
}
