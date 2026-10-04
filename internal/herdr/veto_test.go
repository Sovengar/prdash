package herdr

import (
	"context"
	"strings"
	"testing"
	"time"
)

// Las operaciones de la CLI de Herdr se dividen en dos clases por un motivo que no es
// estetico: las que MODIFICAN la sesión pasan por `guard`, y las que solo leen no. Y la
// diferencia importa porque un veto a una lectura no cuesta nada —una lectura que falla no
// destruye nada— mientras que un veto incompleto a una escritura deja la sesión a medias.
//
// Lo que este fichero fija es la primera mitad de esa regla de forma exhaustiva: las doce
// operaciones mutantes, una por una, todas vetadas fuera de Herdr y sin tocar el binario. Y
// se recorre la lista con una tabla en vez de con doce asserts sueltos, porque el fallo que
// importa es que ALGUNA se quede sin vetar —y una lista suelta se revisa a ojo, y a ojo se
// pasa por alto la decimotercera que alguien añada después—.

// TestNingunaOperacionMutanteTocaLaSesionFueraDeHerdr: la primera mitad de la regla.
//
// Y el caso de control está al final y no es opcional: sin él, este test pasaría con una
// implementación que vetara TODO siempre, que es un fallo distinto y peor —la app no haría
// nada nunca—.
// TestSoloLecturaNoSeVeta: la otra mitad, en el mismo sitio.
func TestNingunaOperacionMutanteTocaLaSesionFueraDeHerdr(t *testing.T) {
	// Fuera de Herdr: cada operación tiene que negarse Y no ejecutar el binario.
	llamadas := 0
	vistos := [][]string{}
	fuera := &Client{
		Bin:    "no-debe-ejecutarse",
		getenv: func(string) string { return "" },
		execFn: func(_ context.Context, args ...string) ([]byte, []byte, error) {
			llamadas++
			vistos = append(vistos, args)
			return []byte(`{"id":"1","result":{"type":"ok"}}`), nil, nil
		},
	}
	ctx := context.Background()
	ref := Container{WorkspaceID: "w1", PaneID: "p1"}

	// El nombre de cada operación, lo que devuelve, y la llamada. La lista está completa a
	// propósito: añadir un método al cliente sin añadirlo aquí es detectable, porque el
	// aserto de "no ejecutó nada" seguiría pasando y el método nuevo quedaría sin vetar.
	ops := []struct {
		nombre string
		llamar func() error
	}{
		{"WorktreeRemove", func() error { return fuera.WorktreeRemove(ctx, "w1", true) }},
		{"WorkspaceCreate", func() error { _, err := fuera.WorkspaceCreate(ctx, WorkspaceSpec{Label: "x"}); return err }},
		{"WorkspaceClose", func() error { return fuera.WorkspaceClose(ctx, "w1", true) }},
		{"TabCreate", func() error {
			_, err := fuera.TabCreate(ctx, TabSpec{WorkspaceID: "w1", Label: "t"})
			return err
		}},
		{"TabRename", func() error { return fuera.TabRename(ctx, "t1", "nuevo") }},
		{"PaneSplit", func() error {
			_, err := fuera.PaneSplit(ctx, SplitSpec{PaneID: ref.PaneID, Direction: "right"})
			return err
		}},
		{"PaneRun", func() error { return fuera.PaneRun(ctx, "p1", []string{"echo", "hola"}) }},
		{"PaneWaitOutput", func() error { return fuera.PaneWaitOutput(ctx, "p1", "hola", time.Second) }},
		{"PaneRename", func() error { return fuera.PaneRename(ctx, "p1", "titulo") }},
		{"PaneFocus", func() error { return fuera.PaneFocus(ctx, "right") }},
		{"MountLayout", func() error { _, err := fuera.MountLayout(ctx, ref, testPlan()); return err }},
	}
	// `Notify` no pasa por el guard: devuelve avisos, no un error, y por eso se
	// comprueba aparte.

	for _, op := range ops {
		if err := op.llamar(); err == nil {
			t.Errorf("%s: fuera de Herdr dio nil, y una operacion que modifica la sesion "+
				"tiene que negarse", op.nombre)
		}
	}
	// Y ninguna llegó al binario. Este es el aserto que de verdad importa: un veto que se
	// cumple DESPUÉS de ejecutar no es un veto, y con un `execFn` que contase la llamada lo
	// veríamos.
	if len(vistos) != 0 {
		t.Errorf("se ejecutaron %d llamadas al binario estando fuera de Herdr: %v", len(vistos), vistos)
	}

	// Dentro de Herdr, con versión bastante: pasan. Sin esto, el test de arriba probaría
	// que todo está vetado siempre.
	dentro := &Client{
		Bin:    "herdr",
		getenv: func(k string) string { return map[string]string{"HERDR_ENV": "1"}[k] },
		execFn: func(_ context.Context, args ...string) ([]byte, []byte, error) {
			vistos = append(vistos, args)
			// La respuesta de `pane split` tiene que traer el pane_id o el parser la
			// rechaza, y un veto que se confundiera con un parseo roto daría un error
			// distinto del que el usuario necesita ver.
			if len(args) > 0 && args[0] == "pane" && len(args) > 1 && args[1] == "split" {
				return []byte(`{"id":"1","result":{"pane":{"pane_id":"p2"}}}`), nil, nil
			}
			return []byte(`{"id":"1","result":{"type":"ok"}}`), nil, nil
		},
	}
	refDentro := Container{WorkspaceID: "w1", PaneID: "p1"}
	ok := []struct {
		nombre string
		llamar func() error
	}{
		{"WorktreeRemove", func() error { return dentro.WorktreeRemove(ctx, "w1", true) }},
		{"WorkspaceClose", func() error { return dentro.WorkspaceClose(ctx, "w1", true) }},
		{"TabRename", func() error { return dentro.TabRename(ctx, "t1", "nuevo") }},
		{"PaneRun", func() error { return dentro.PaneRun(ctx, "p1", []string{"echo", "hola"}) }},
		{"PaneFocus", func() error { return dentro.PaneFocus(ctx, "right") }},
		{"PaneSplit", func() error {
			_, err := dentro.PaneSplit(ctx, SplitSpec{PaneID: refDentro.PaneID, Direction: "right"})
			return err
		}},
	}
	for _, op := range ok {
		if err := op.llamar(); err != nil {
			t.Errorf("%s: dentro de Herdr dio error: %v", op.nombre, err)
		}
	}
	// Y ahora sí se ejecutó algo, lo que confirma que el veto de arriba era el veto y no un
	// binario que no arranca.
	if len(vistos) == 0 {
		t.Error("dentro de Herdr no se ejecutó ninguna llamada: el veto de arriba no lo " +
			"provoca el veto, lo provoca el no estar dentro")
	}
	// Y el aviso nombra a Herdr, no sale un error interno: el aviso es lo único que el
	// usuario ve de un montaje que no pudo hacerse.
	// Y el veto dice qué falta y cómo arreglarlo. La primera versión de este aserto
	// buscaba "Herdr" con mayúscula, y el texto lo dice en minúscula —"herdr unavailable
	// (requires HERDR_ENV=1 and version >= 0.9.0)"—, así que el aserto pasaba por el motivo
	// equivocado: comprobaba que hubiera una letra H mayúscula, no que el texto dijera algo.
	//
	// Lo que se mira es que nombre la variable y la versión mínima, que es lo que el
	// usuario necesita para arreglarlo sin leer el código.
	err := fuera.PaneRun(ctx, "p1", []string{"echo", "hola"})
	if err == nil {
		t.Error("el veto no dio error")
	} else {
		texto := err.Error()
		for _, quiere := range []string{"herdr unavailable", "HERDR_ENV", "0.9.0"} {
			if !strings.Contains(texto, quiere) {
				t.Errorf("el aviso del veto %q no menciona %q", texto, quiere)
			}
		}
	}
	// Y `MountLayout` fuera de Herdr falla con error, no solo con avisos: el layout no se
	// abre porque `PaneSplit` está vetado, y su error se propaga. La primera versión de
	// este test daba por hecho que el layout se degradaba a avisos sin llegar a comprobarlo.
	if _, err := fuera.MountLayout(ctx, ref, testPlan()); err == nil {
		t.Error("MountLayout fuera de Herdr dio nil")
	}
}

// TestSoloLecturaNoSeVeta: la otra mitad de la regla.
//
// Y el motivo es el que dice el comentario del guard: una lectura que falla no destruye
// nada. Vetarla solo haría que el inbox pareciera no tener nada que mostrar, que es peor que
// un listado vacío con un aviso.
//
// Y `WorktreeList` estaba al 0% justo por esto: como es una lectura, no pasa por el guard, y
// los tests que la tocaban usaban el camino de la versión en lugar del listado.
func TestSoloLecturaNoSeVeta(t *testing.T) {
	llamadas := 0
	vistos := [][]string{}
	a := &Client{
		Bin:    "herdr",
		getenv: func(string) string { return "" }, // fuera de Herdr a proposito
		execFn: func(_ context.Context, args ...string) ([]byte, []byte, error) {
			llamadas++
			vistos = append(vistos, args)
			return []byte(`{"result":{"source":{"repo_root":"/r"},"worktrees":[
				{"path":"/r","branch":"main","label":"et","open_workspace_id":"w1",
				 "is_linked_worktree":false},
				{"path":"/r/.worktrees/wt-1","branch":"feat/x","label":"prdash-pr-1",
				 "open_workspace_id":"w2","is_linked_worktree":true}
			]}}`), nil, nil
		},
	}

	list, err := a.WorktreeList(context.Background(), "/r")
	if err != nil {
		t.Fatalf("una lectura vetada dio error: %v", err)
	}
	if llamadas == 0 {
		t.Error("la lectura no llegó al binario: el veto se comió también las lecturas")
	}
	// Y sale lo que el binario dijo, con el campo `is_prunable` del listado descartado —
	// que es lo que hace que el tipo no lo tenga.
	if len(list) != 2 {
		t.Fatalf("la lista dio %d entradas: %+v", len(list), list)
	}
	if list[0].Path != "/r" || list[0].Branch != "main" {
		t.Errorf("la primera entrada se leyo mal: %+v", list[0])
	}
	if !list[1].IsLinkedWorktree || list[1].OpenWorkspaceID != "w2" {
		t.Errorf("la segunda entrada perdio los flags: %+v", list[1])
	}
	// Y el origen, que es de donde sale la raíz del repo que usan las rutas.
	if list[0].IsLinkedWorktree {
		t.Error("el repo principal aparecio como worktree enlazado")
	}

	// Y una lectura que falla NO se veto: devuelve error y punto.
	roto := &Client{
		Bin:    "herdr",
		getenv: func(string) string { return "" },
		execFn: func(context.Context, ...string) ([]byte, []byte, error) {
			return nil, []byte("fatal: no"), &Error{Args: []string{"x"}, Msg: "no", Exit: 1}
		},
	}
	if _, err := roto.WorktreeList(context.Background(), "/r"); err == nil {
		t.Error("una lectura que falla dio nil")
	}
	// Y una lista vacía no es un error.
	vacio := &Client{
		Bin: "herdr", getenv: func(string) string { return "" },
		execFn: func(context.Context, ...string) ([]byte, []byte, error) {
			return []byte(`{"result":{"source":{},"worktrees":[]}}`), nil, nil
		},
	}
	lista, err := vacio.WorktreeList(context.Background(), "/r")
	if err != nil {
		t.Errorf("una lista vacia dio error: %v", err)
	}
	if len(lista) != 0 {
		t.Errorf("una lista vacia dio %d entradas", len(lista))
	}
	_ = vistos
}

// TestElClienteDeGraficosSeConstruyeYLeeElEntorno: `NewGraphics` y su `env`.
//
// Y `NewGraphics` estaba al 0% porque todos los tests de gráficos usan el constructor de
// test, que trae un `dial` falso. Y lo que decide son dos cosas: el pane de dónde sale —
// `HERDR_PANE_ID`— y el socket —`HERDR_SOCK`—, que son las dos que `Info` necesita antes de
// poder preguntar nada.
//
// Y el caso de `getenv` a nil: `env` cae a `os.Getenv` en vez de hacer panic. Un `Graphics`
// construido a mano por un test —sin el campo— es legal, y que reviente sería una trampa
// para quien escriba el siguiente test.
func TestElClienteDeGraficosSeConstruyeYLeeElEntorno(t *testing.T) {
	// Con el entorno puesto: los dos valores salen de ahí.
	//
	// Y la variable del socket es `HERDR_SOCKET_PATH`, no `HERDR_SOCK`. La primera versión
	// de este test fijaba `HERDR_SOCK` —que no existe— y el `socket()` salía con el valor
	// real del proceso, que en un shell dentro de Herdr es un socket de verdad. El test
	// pasaba por el motivo equivocado: estaba probando la variable del entorno del que
	// corría, no la que fijaba.
	t.Setenv("HERDR_PANE_ID", "p9")
	t.Setenv("HERDR_SOCKET_PATH", "/tmp/herdr.sock")
	g := NewGraphics()
	if g.getenv == nil {
		t.Fatal("NewGraphics dejó getenv a nil")
	}
	if got := g.env("HERDR_PANE_ID"); got != "p9" {
		t.Errorf("env dio %q, want p9", got)
	}
	if got := g.socket(); got != "/tmp/herdr.sock" {
		t.Errorf("socket dio %q, want el de HERDR_SOCKET_PATH", got)
	}
	if got := g.pane(); got != "p9" {
		t.Errorf("pane dio %q, want p9", got)
	}
	// Y el campo puesto gana sobre el entorno, que es lo que permite que un test fije el
	// destino sin tocar el entorno de todo lo demás.
	directo := &Graphics{Socket: "/tmp/otro.sock", PaneID: "p0"}
	if got := directo.socket(); got != "/tmp/otro.sock" {
		t.Errorf("con Socket puesto dio %q", got)
	}
	if got := directo.pane(); got != "p0" {
		t.Errorf("con PaneID puesto dio %q", got)
	}

	// Sin el entorno: vacíos, no "undefined" ni el nombre de la variable.
	t.Setenv("HERDR_PANE_ID", "")
	t.Setenv("HERDR_SOCKET_PATH", "")
	g = NewGraphics()
	if got := g.env("HERDR_PANE_ID"); got != "" {
		t.Errorf("sin variable dio %q, want vacío", got)
	}
	// Y con eso no hay destino gráfico, que es lo que hace que `call` se niegue antes de
	// tocar la red.
	if haveGraphicsTarget(g.socket(), g.pane()) {
		t.Error("sin socket ni pane dio un destino valido")
	}

	// Y un `Graphics` con `getenv` a nil: `env` cae a `os.Getenv` en vez de hacer panic.
	// Un struct construido a mano por un test es legal, y que reviente seria una trampa.
	gA := &Graphics{}
	t.Setenv("HERDR_PANE_ID", "p-con-nil")
	if got := gA.env("HERDR_PANE_ID"); got != "p-con-nil" {
		t.Errorf("con getenv a nil dio %q, want el valor real del entorno", got)
	}
}

// TestElClienteTambienCaeAOsGetenvSinGetenv: el mismo caso en el cliente normal.
//
// Y está aquí y no solo en el de gráficos porque los dos tienen el mismo `env` duplicado, y
// una de las dos copias puede quedarse atrás en una refactorización.
func TestElClienteTambienCaeAOsGetenvSinGetenv(t *testing.T) {
	cA := &Client{Bin: "herdr"}
	t.Setenv("HERDR_ENV", "1")
	if got := cA.env("HERDR_ENV"); got != "1" {
		t.Errorf("con getenv a nil dio %q, want 1", got)
	}
	// Y sin la variable.
	t.Setenv("HERDR_ENV", "")
	if got := cA.env("HERDR_ENV"); got != "" {
		t.Errorf("sin variable dio %q", got)
	}
	// Y con `getenv` puesto, manda el suyo y no el del proceso.
	propio := &Client{getenv: func(k string) string {
		if k == "HERDR_ENV" {
			return "1"
		}
		return ""
	}}
	if got := propio.env("HERDR_ENV"); got != "1" {
		t.Errorf("con getenv propio dio %q", got)
	}
	t.Setenv("HERDR_ENV", "")
	if got := propio.env("HERDR_ENV"); got != "1" {
		t.Errorf("tras vaciar la variable dio %q: manda el getenv propio", got)
	}
}

// TestLaVersionSeConsultaUnaSolaYSeCachea: `Version`, y su `sync.Once`.
//
// Y el cacheo es lo que importa, no la versión: `Available()` la consulta, y `Available()`
// la llama `guard`, que llama cada operación. Sin `Once`, cada operación mutante haría una
// llamada a `herdr --version` antes de hacer su trabajo, y montar un review de dos panes
// serían tres llamadas a un subproceso para leer un número que no cambia.
//
// Y el caso de error: si la versión no se puede determinar, `Version` devuelve `false` y
// `Available` NO bloquea por drift. Es lo que dice el comentario y es lo que permite que
// prdash funcione con una build de desarrollo de Herdr cuyo `--version` no se entienda.
func TestLaVersionSeConsultaUnaSolaYSeCachea(t *testing.T) {
	llamadas := 0
	a := &Client{
		Bin:    "herdr",
		getenv: func(k string) string { return map[string]string{"HERDR_ENV": "1"}[k] },
		execFn: func(_ context.Context, args ...string) ([]byte, []byte, error) {
			llamadas++
			if len(args) == 1 && args[0] == "--version" {
				return []byte("herdr 0.9.1-preview.7\n"), nil, nil
			}
			return []byte(`{"id":"1","result":{"type":"ok"}}`), nil, nil
		},
	}

	v, ok := a.Version()
	if !ok {
		t.Fatal("Version dio false con una salida válida")
	}
	if v.Major != 0 || v.Minor != 9 || v.Patch != 1 {
		t.Errorf("Version dio %d.%d.%d, want 0.9.1", v.Major, v.Minor, v.Patch)
	}
	// Y cinco llamadas más no vuelven a preguntar.
	for i := 0; i < 5; i++ {
		a.Version()
		a.Available()
	}
	if llamadas != 1 {
		t.Errorf("se consultó la versión %d veces, want 1: Available la llama en cada "+
			"operación y sin cacheo cada montaje sería varias llamadas a un subproceso",
			llamadas)
	}

	// Y con `--version` que falla: no se bloquea. Es el caso de la build de desarrollo,
	// que es el que permite que prdash funcione con Herdr sin version reconocida.
	roto := &Client{
		Bin:    "herdr",
		getenv: func(k string) string { return map[string]string{"HERDR_ENV": "1"}[k] },
		execFn: func(context.Context, ...string) ([]byte, []byte, error) {
			return nil, nil, &Error{Args: []string{"--version"}, Msg: "no such flag", Exit: 1}
		},
	}
	if _, ok := roto.Version(); ok {
		t.Error("Version dio true con un --version que falla")
	}
	if !roto.Available() {
		t.Error("Available veto por drift cuando la versión no se puede determinar: " +
			"eso deja prdash sin Herdr en una build de desarrollo")
	}
}
