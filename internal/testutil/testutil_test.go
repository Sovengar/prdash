package testutil

import (
	"context"
	"strings"
	"testing"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
)

// Este fichero testea el doble de pruebas. Es lo contrario de lo habitual —normalmente los
// dobles no se testean— y aquí hay una razón: un doble que miente no falla, debilita.
//
// El `FakeAdapter` es lo que decide qué ven los tests de la TUI, del inbox, del review y
// del executor. Si `Merge` aceptara una petición sin pin, todos los tests de merge de la
// repo pasarían por un camino que los adapters reales rechazan, y el fallo aparecería en
// producción donde ningún test llega. Si el contador de llamadas mintiera, un test que
// afirma "no se lanzó la acción" pasaría sin que nada se lanzara. Ninguna de las dos
// cosas da un error: da un suite verde que no prueba nada.
//
// Y la segunda mitad del fichero es al revés de lo que parece: `RunConformance` es la
// puerta por la que pasan los tres adapters reales, y lo que se comprueba es que **falla**
// ante un adapter roto. Una suite de conformidad que no detecta un contrato roto es peor
// que no tenerla, porque hace que los tres adapters queden "verificados" sin que nadie
// haya comprobado nada.

func refDePrueba() model.RepoRef {
	return model.RepoRef{Forge: "github", Host: "github.com", Project: "o/r", Owner: "o", Name: "r"}
}

// TestElDobleIdentificaForgeYHostYEstaAutenticadoPorDefecto: los tres identificadores, y
// el default que más se usa.
//
// Y el default de `Auth` —autenticado— es lo que hace que un test que no habla de
// autenticación no tenga que configurarla. El detalle que importa es cómo se decide que
// "no está configurada": los tres campos vacíos. Si alguien configura `Reason` para
// explicar un fallo y olvida `Forge`, el estado se devuelve tal cual —con `Forge` vacío— y
// quien lo pinte enseña un estado sin nombre de forge.
func TestElDobleIdentificaForgeYHostYEstaAutenticadoPorDefecto(t *testing.T) {
	f := &FakeAdapter{ForgeName: "github", HostName: "github.com"}
	if f.Forge() != "github" || f.Host() != "github.com" {
		t.Errorf("Forge/Host dio %q/%q", f.Forge(), f.Host())
	}

	// Sin configurar nada: autenticado, y con el nombre del forge puesto.
	auth := f.Auth(context.Background())
	if !auth.OK {
		t.Error("sin configurar, Auth dio OK=false: los tests que no hablan de auth " +
			"aparecerían como degradados")
	}
	if auth.Forge != "github" {
		t.Errorf("el default de Auth no trae el nombre del forge: %q", auth.Forge)
	}

	// Configurado: se devuelve EXACTAMENTE lo configurado, sin completar nada. Que es lo
	// que permite probar un estado a medias, que es lo que hace un forge medio
	// autenticado.
	f.AuthState = model.AuthState{Forge: "gitlab", OK: false, Reason: "token caducado"}
	auth = f.Auth(context.Background())
	if auth.OK || auth.Forge != "gitlab" || auth.Reason != "token caducado" {
		t.Errorf("el estado configurado salio alterado: %+v", auth)
	}
}

// TestListAvanzaDePaginaYLaUltimaSeRepite: la paginación del doble.
//
// Y las dos mitades importan por razones distintas. "Avanza una página" es lo que permite
// probar la paginación de la TUI sin un forge de verdad. Y "la última se repite" es lo que
// permite que un test recargue sinitemsrar un doble que se queda sin datos: sin eso, la
// segunda recarga devolvería una lista vacía y el test leería "el inbox se vació" en vez
// de "no había más".
//
// Y con `More` a true, `Next` es lo que trae la página siguiente, así que el doble tiene
// que honoring.
func TestListAvanzaDePaginaYLaUltimaSeRepite(t *testing.T) {
	key := FakeKey{Section: model.SectionReview, Kind: model.ReviewRequested}
	primera := forge.Page{Items: []model.Item{{Number: 1}}, More: true, Next: "cursor-1"}
	segunda := forge.Page{Items: []model.Item{{Number: 2}}, More: false}
	f := &FakeAdapter{ForgeName: "github", Pages: map[FakeKey][]forge.Page{key: {primera, segunda}}}
	q := forge.Query{Section: model.SectionReview, ReviewKind: model.ReviewRequested}

	page, _ := f.List(context.Background(), q)
	if len(page.Items) != 1 || page.Items[0].Number != 1 || !page.More || page.Next != "cursor-1" {
		t.Fatalf("la primera pagina dio %+v", page)
	}
	page, _ = f.List(context.Background(), q)
	if len(page.Items) != 1 || page.Items[0].Number != 2 || page.More {
		t.Fatalf("la segunda pagina dio %+v", page)
	}
	// Y la tercera vuelve a la última, en vez de quedarse sin items o dar error.
	for i := 0; i < 3; i++ {
		page, _ = f.List(context.Background(), q)
		if len(page.Items) != 1 || page.Items[0].Number != 2 {
			t.Fatalf("la pagina %d tras agotar dio %+v, want la ultima repetida", i+3, page)
		}
	}
	if got := f.ListCallCount(); got != 5 {
		t.Errorf("ListCallCount dio %d, want 5", got)
	}

	// Y una lista sin páginas configuradas: vacía y sin error. Es el caso de un forge que
	// no devuelve nada para esa sección, y tiene que ser indistinguible de "no hay
	// ítems", no un fallo.
	otra := forge.Query{Section: model.SectionMentions}
	page, warns := f.List(context.Background(), otra)
	if len(page.Items) != 0 || len(warns) != 0 || page.More {
		t.Errorf("una lista sin configurar dio %+v / %v", page, warns)
	}
}

// TestLosWarningsSalenEnCadaLlamada: los avisos de listado no se consumen.
//
// Y es la diferencia entre "una vez" y "siempre": la TUI refresca cada seis segundos, y un
// doble que entregara el aviso solo la primera vez dejaría al usuario viendo que el
// problema se arregló solo sin que se arreglara nada.
func TestLosWarningsSalenEnCadaLlamada(t *testing.T) {
	key := FakeKey{Section: model.SectionReview, Kind: model.ReviewRequested}
	warns := []model.Warning{{Forge: "github", Kind: "ratelimit", Msg: "espera"}}
	f := &FakeAdapter{
		ForgeName:    "github",
		Pages:        map[FakeKey][]forge.Page{key: {{Items: []model.Item{{Number: 1}}}}},
		ListWarnings: map[FakeKey][]model.Warning{key: warns},
	}
	q := forge.Query{Section: model.SectionReview, ReviewKind: model.ReviewRequested}

	for i := 1; i <= 3; i++ {
		_, got := f.List(context.Background(), q)
		if len(got) != 1 || got[0].Kind != "ratelimit" {
			t.Fatalf("la llamada %d dio %v, want el ratelimit en todas", i, got)
		}
	}
}

// TestElDobleSeNiegaAMergearSinPinYNoLoRegistra: la negativa que protege el resto de la
// repo.
//
// Y son dos comportamientos en una función, y los dos importan. Se niega: sin
// `HeadSHA` devuelve el mismo aviso que los adapters reales. Y **no registra**: un merge
// que no sale no pidió ninguna estrategia, así que `MergeModes` no lo cuenta y
// `ActionCallCount` da cero.
//
// El "no registra" es lo que hace útil el contador. Sin él, un test que afirma "no se
// pidió ningún modo de merge" pasaría con un merge rechazado que sí dejó rastro, y el
// contador se usaría como prueba de un camino que no existe.
func TestElDobleSeNiegaAMergearSinPinYNoLoRegistra(t *testing.T) {
	f := &FakeAdapter{ForgeName: "github"}
	ref := refDePrueba()

	warns := f.Merge(context.Background(), ref, 1, forge.MergeRequest{Mode: forge.Squash})
	if len(warns) != 1 || warns[0].Kind != "unsupported" {
		t.Fatalf("un merge sin pin dio %v, want un unsupported", warns)
	}
	if !strings.Contains(warns[0].Msg, "head") {
		t.Errorf("el aviso %q no dice que falta el head", warns[0].Msg)
	}
	// Y nada quedó registrado.
	if got := f.ActionCallCount("merge", ref, 1); got != 0 {
		t.Errorf("un merge rechazado conto %d llamadas de accion, want 0", got)
	}
	if got := f.MergeModeCount(forge.Squash); got != 0 {
		t.Errorf("un merge rechazado conto %d modos, want 0", got)
	}
	if len(f.MergeDeletes) != 0 {
		t.Errorf("un merge rechazado registro borrado: %v", f.MergeDeletes)
	}

	// Con pin: sale, y se registra TODO —el modo, el borrado y la llamada.
	warns = f.Merge(context.Background(), ref, 1, forge.MergeRequest{
		Mode: forge.Squash, HeadSHA: "deadbeef", DeleteBranch: true,
	})
	if len(warns) != 0 {
		t.Errorf("un merge con pin dio avisos %v", warns)
	}
	if got := f.ActionCallCount("merge", ref, 1); got != 1 {
		t.Errorf("ActionCallCount dio %d, want 1", got)
	}
	if got := f.MergeModeCount(forge.Squash); got != 1 {
		t.Errorf("MergeModeCount dio %d, want 1", got)
	}
	if f.MergeDeleteCount(true) != 1 || f.MergeDeleteCount(false) != 0 {
		t.Errorf("MergeDeleteCount dio true=%d false=%d, want 1 y 0",
			f.MergeDeleteCount(true), f.MergeDeleteCount(false))
	}
}

// TestElDobleSeNiegaARetargetSinRamaYLoRegistraEnOrden: lo mismo, y el orden.
//
// Y el orden es lo que hace que `Retargets` sea una lista y no un contador: la pregunta de
// un test es "¿a qué base lo movió?", y eso solo lo responde la secuencia. Con un contador
// por rama habría que además suponer el orden de las llamadas.
func TestElDobleSeNiegaARetargetSinRamaYLoRegistraEnOrden(t *testing.T) {
	f := &FakeAdapter{ForgeName: "github"}
	ref := refDePrueba()

	// Sin rama, y con rama hecha de espacios: ninguna de las dos sale.
	for _, vacia := range []string{"", "   ", "\t"} {
		warns := f.Retarget(context.Background(), ref, 1, vacia)
		if len(warns) != 1 || warns[0].Kind != "unsupported" {
			t.Errorf("un retarget a %q dio %v, want un unsupported", vacia, warns)
		}
	}
	if f.RetargetCount() != 0 {
		t.Errorf("un retarget rechazado conto %d, want 0", f.RetargetCount())
	}
	if got := f.ActionCallCount("retarget", ref, 1); got != 0 {
		t.Errorf("un retarget rechazado conto %d llamadas, want 0", got)
	}

	// Y los que salen, en orden.
	for _, rama := range []string{"main", "release/2.0", "develop"} {
		f.Retarget(context.Background(), ref, 1, rama)
	}
	want := []string{"main", "release/2.0", "develop"}
	if f.RetargetCount() != len(want) {
		t.Fatalf("RetargetCount dio %d, want %d", f.RetargetCount(), len(want))
	}
	for i := range want {
		if f.Retargets[i] != want[i] {
			t.Errorf("Retargets[%d] = %q, want %q (completo %v)", i, f.Retargets[i], want[i], f.Retargets)
		}
	}
}

// TestOnMergeSoloAplicaDespuesDelMerge: el hook que existe para un caso que no se puede
// expresar con un estado fijo.
//
// Y "solo después" es la mitad que importa. El merge hace una relectura del ítem ANTES de
// decidir si sale —es la que comprueba que no está bloqueado—, y si el hook se aplicara
// también ahí, el ítem aparecería como borrado antes de tiempo y el merge nunca saldría.
// Con el hook mal colocado, todos los tests de merge de la TUI fallarían, que es visible.
//
// Y lo invisible es lo otro: si `merged` no se activara nunca, el hook no se aplicaría
// después tampoco, y los tests del borrado de rama —donde el merge sale pero el comando
// falla— comprobarían un ítem que sigue abierto sin que nada lo indique.
func TestOnMergeSoloAplicaDespuesDelMerge(t *testing.T) {
	ref := refDePrueba()
	clave := ItemKey(ref.Project, 1)
	f := &FakeAdapter{
		ForgeName:  "github",
		ItemStates: map[string]model.Item{clave: {Number: 1, State: "OPEN", Ref: ref}},
		OnMerge:    func(it *model.Item) { it.State = "MERGED" },
	}

	// Antes del merge: el estado configurado, sin el hook.
	it, _ := f.ItemState(context.Background(), ref, 1)
	if it.State != "OPEN" {
		t.Fatalf("sin merge dio %q, want OPEN: el hook se esta aplicando antes de tiempo", it.State)
	}

	// Mergeando. Con pin, porque sin él no sale.
	f.Merge(context.Background(), ref, 1, forge.MergeRequest{Mode: forge.Squash, HeadSHA: "deadbeef"})

	// Y ahora sí: el estado del forge y el del hook juntos.
	it, _ = f.ItemState(context.Background(), ref, 1)
	if it.State != "MERGED" {
		t.Errorf("tras el merge dio %q, want MERGED: el hook no se esta aplicando", it.State)
	}
	// Y el resto del ítem sigue ahí: el hook parte de una COPIA, no borra el ítem.
	if it.Number != 1 {
		t.Errorf("el hook se comió el resto del item: %+v", it)
	}

	// Y sin hook configurado, `merged` no rompe nada.
	limpio := &FakeAdapter{ForgeName: "github", ItemStates: map[string]model.Item{clave: {Number: 1, State: "OPEN"}}}
	limpio.Merge(context.Background(), ref, 1, forge.MergeRequest{Mode: forge.Squash, HeadSHA: "x"})
	it, _ = limpio.ItemState(context.Background(), ref, 1)
	if it.State != "OPEN" {
		t.Errorf("sin OnMerge dio %q, want OPEN", it.State)
	}
}

// TestLosWarningsDeEstadoYDerechosVienenConfigurados: los tres mapas por clave.
//
// Y la clave es "proyecto#número", con `ItemKey` componiéndola. Que haya un helper para
// eso no es solo comodidad: es lo que evita que un test escriba la clave a mano con un
// formato distinto y receive un ítem vacío sin enterarse.
func TestLosWarningsDeEstadoYDerechosVienenConfigurados(t *testing.T) {
	ref := refDePrueba()
	clave := ItemKey(ref.Project, 7)
	if clave != "o/r#7" {
		t.Fatalf("ItemKey dio %q, want o/r#7", clave)
	}

	f := &FakeAdapter{
		ForgeName: "github",
		ItemStates: map[string]model.Item{
			clave: {Number: 7, Title: "uno", Ref: ref},
		},
		StateWarnings: map[string][]model.Warning{
			clave: {{Kind: "ratelimit", Msg: "espera"}},
		},
		Conversations: map[string]forge.CommentPage{
			clave: {Comments: []model.Comment{{Body: "hola"}}, Total: 1},
		},
		CommentWarnings: map[string][]model.Warning{
			clave: {{Kind: "network", Msg: "sin red"}},
		},
		ActionWarnings: map[string][]model.Warning{
			"approve:o/r#7": {{Kind: "permission", Msg: "no"}},
		},
	}

	it, warns := f.ItemState(context.Background(), ref, 7)
	if it.Title != "uno" || len(warns) != 1 || warns[0].Kind != "ratelimit" {
		t.Errorf("ItemState dio %+v / %v", it, warns)
	}
	page, warns := f.Comments(context.Background(), ref, 7)
	if len(page.Comments) != 1 || page.Comments[0].Body != "hola" || warns[0].Kind != "network" {
		t.Errorf("Comments dio %+v / %v", page, warns)
	}
	if warns := f.Approve(context.Background(), ref, 7); len(warns) != 1 || warns[0].Kind != "permission" {
		t.Errorf("Approve dio %v", warns)
	}

	// Y un número que no está configurado: ítem vacío, sin error. Que es lo que permite
	// que un test hable de un ítem que el doble no conoce sin preparar nada.
	it, warns = f.ItemState(context.Background(), ref, 999)
	if it.Number != 0 || len(warns) != 0 {
		t.Errorf("un numero sin configurar dio %+v / %v", it, warns)
	}
}

// TestCommentsSinConfigurarVieneResuelto: el default que evita un "cargando" eterno.
//
// Y la página vacía tiene que venir con `Total` a cero, no con un total que diga "hay 30
// comentarios" sin traerlos: el total es lo que la ficha pinta como "y 25 más", y un total
// sin comentarios detrás hace que un test de un ítem sin conversación enseñe un número.
func TestCommentsSinConfigurarVieneResuelto(t *testing.T) {
	f := &FakeAdapter{ForgeName: "github"}
	page, warns := f.Comments(context.Background(), refDePrueba(), 1)
	if len(page.Comments) != 0 || len(warns) != 0 {
		t.Errorf("Comments sin configurar dio %+v / %v", page, warns)
	}
	if page.Total != 0 {
		t.Errorf("Comments sin configurar dio Total=%d, want 0", page.Total)
	}
	if got := f.CommentCallCount(); got != 1 {
		t.Errorf("CommentCallCount dio %d, want 1", got)
	}
	f.Comments(context.Background(), refDePrueba(), 1)
	if got := f.CommentCallCount(); got != 2 {
		t.Errorf("CommentCallCount dio %d, want 2", got)
	}
}

// TestBranchesPorProyectoYConContador: el buscador de ramas.
//
// Y el default es el mismo caso que los comentarios: sin configurar devuelve una lista
// vacía, para que un test que no habla del buscador no tenga que inventar ramas. La
// diferencia con `Comments` es que aquí la lista vacía es `nil`, y eso no se comprueba
// aquí sino en el test de la TUI que la pinta.
func TestBranchesPorProyectoYConContador(t *testing.T) {
	ref := refDePrueba()
	otro := model.RepoRef{Forge: "github", Host: "github.com", Project: "o/otro"}

	f := &FakeAdapter{
		ForgeName:      "github",
		BranchLists:    map[string][]string{"o/r": {"main", "feature"}},
		BranchWarnings: map[string][]model.Warning{"o/otro": {{Kind: "unsupported", Msg: "no"}}},
	}

	names, warns := f.Branches(context.Background(), ref)
	if len(names) != 2 || names[0] != "main" || len(warns) != 0 {
		t.Errorf("Branches dio %v / %v", names, warns)
	}
	// Y el contador es POR PROYECTO, no global: es lo que permite afirmar que el buscador
	// de un repo cachea sin que el otro lo enmascare.
	if got := f.BranchCallCount("o/r"); got != 1 {
		t.Errorf("BranchCallCount dio %d, want 1", got)
	}
	if got := f.BranchCallCount("o/otro"); got != 0 {
		t.Errorf("BranchCallCount de un proyecto sin llamar dio %d, want 0", got)
	}

	// Un proyecto sin configurar: lista vacía y su aviso.
	names, warns = f.Branches(context.Background(), otro)
	if len(names) != 0 || len(warns) != 1 || warns[0].Kind != "unsupported" {
		t.Errorf("un proyecto sin configurar dio %v / %v", names, warns)
	}
	if got := f.BranchCallCount("o/otro"); got != 1 {
		t.Errorf("BranchCallCount dio %d tras la llamada, want 1", got)
	}
}

// TestElContadorDeAccionesSeparaLasTres: tres acciones, tres contadores.
//
// Y se separan por tipo de acción, no solo por ítem. Un contador único daría "1" con tres
// acciones distintas sobre el mismo ítem, y el aserto "approve no llegó a lanzarse"
// pasaría sin comprobar nada.
func TestElContadorDeAccionesSeparaLasTres(t *testing.T) {
	ref := refDePrueba()
	f := &FakeAdapter{ForgeName: "github"}

	f.Approve(context.Background(), ref, 1)
	f.Approve(context.Background(), ref, 1)
	f.Approve(context.Background(), ref, 2) // otro ítem
	f.Merge(context.Background(), ref, 1, forge.MergeRequest{Mode: forge.Squash, HeadSHA: "a"})
	f.Retarget(context.Background(), ref, 1, "main")

	casos := []struct {
		kind   string
		number int
		want   int
	}{
		{"approve", 1, 2},
		{"approve", 2, 1},
		{"merge", 1, 1},
		{"retarget", 1, 1},
		{"approve", 3, 0},
		{"merge", 2, 0},
		{"inventada", 1, 0},
	}
	for _, c := range casos {
		if got := f.ActionCallCount(c.kind, ref, c.number); got != c.want {
			t.Errorf("ActionCallCount(%q, %d) dio %d, want %d", c.kind, c.number, got, c.want)
		}
	}
	// Y los avisos salen por acción, así que un approve puede estar avisado y un merge
	// del mismo ítem no.
	f.ActionWarnings = map[string][]model.Warning{
		"approve:o/r#2": {{Kind: "unsupported", Msg: "no"}},
	}
	if warns := f.Approve(context.Background(), ref, 2); len(warns) != 1 {
		t.Errorf("Approve dio %v", warns)
	}
	if warns := f.Merge(context.Background(), ref, 2, forge.MergeRequest{HeadSHA: "x"}); len(warns) != 0 {
		t.Errorf("Merge con un aviso ajeno en la misma clave dio %v", warns)
	}
}

// TestLaSuiteDeConformidadCubreTodaLaSuperficieDelContrato: la puerta, probada por lo que
// toca y no por lo que rechaza.
//
// Y hay una razón por la que se prueba así y no la obvia. Lo obvio sería comprobar que la
// suite RECHAZA un adapter roto, y no se puede: `RunConformance` recibe un `*testing.T`, y
// cuando llama a `t.Error` dentro de un `t.Run` el subtest falla Y el padre queda marcado
// como fallido. `t.Run` devuelve false, pero no hay forma de mirar ese false sin arrastrar
// el fallo del padre. Una suite de conformidad necesita un `testing.TB` inyectable —y
// `testing.TB` tiene un método privado, así que tampoco se puede sustituir por un doble—
// para poder probarse por el lado del rechazo. Lo que sí se puede, y es lo que importa,
// es que la suite no deje de cubrir nada en silencio.
//
// Y este es el fallo real que sería invisible: alguien añade un método a `forge.Adapter` y
// el adapter nuevo no lo implementa bien. `RunConformance` no lo comprueba, porque la suite
// no lo menciona, y los tres adapters siguen dando "conforme". Un método que la suite no
// toca es un método que nadie prueba. Por eso se cuenta lo que la suite invoca y se exige
// que invoque TODO.
//
// Con `Unsupported: true` la suite recorre además las cinco acciones, que son las que más
// se olvidan: una suite que solo mirara los listados dejaría sin comprobar approve, merge,
// retarget y branches, que son justamente las que modifican algo.
func TestLaSuiteDeConformidadCubreTodaLaSuperficieDelContrato(t *testing.T) {
	a := &contadorDeLlamadas{FakeAdapter: &FakeAdapter{
		ForgeName: "github",
		HostName:  "github.com",
	}}
	a.Pages = map[FakeKey][]forge.Page{
		{Section: model.SectionAuthored}:                            {{Items: []model.Item{{Number: 1}}}},
		{Section: model.SectionReview, Kind: model.ReviewRequested}: {{Items: []model.Item{{Number: 2}}}},
		{Section: model.SectionReview, Kind: model.ReviewAssigned}:  {{Items: []model.Item{{Number: 3}}}},
		{Section: model.SectionMentions}:                            {{Items: []model.Item{{Number: 4}}}},
	}
	RunConformance(t, a, ConformanceOptions{})

	vistos := a.vistos()
	esperados := []string{
		"List", "ItemState", "Comments", "Approve", "Merge", "Retarget", "Branches",
	}
	for _, m := range esperados {
		if vistos[m] == 0 {
			t.Errorf("RunConformance no invocó %s: un método que la suite no toca es un "+
				"método que nadie prueba", m)
		}
	}
	// Y los cuatro streams, que es lo que hace que un adapter que responde solo a "los
	// asignados" pase sin que nadie lo note.
	for _, q := range forge.Streams {
		k := FakeKey{Section: q.Section, Kind: q.ReviewKind}
		if a.Pages[k] == nil && a.listas[k] == 0 {
			t.Errorf("la suite no pregunto el stream %+v", q)
		}
	}
	// Y `Auth` no está en la lista de la suite: la comprobación de disponibilidad la hace
	// quien compone el inbox, no la conformidad del adapter. Se deja constancia aquí para
	// que añadirlo a la lista sea una decisión y no un olvido.
	if vistos["Auth"] != 0 {
		t.Error("la suite invoca Auth y la lista de este test no lo esperaba: revisar")
	}
}

// contadorDeLlamadas lleva la cuenta de a qué métodos se llama, que es lo que hace falta
// para comprobar que la suite los recorre todos. Embebe `FakeAdapter` y la usa con Pages
// configuradas para que `Unsupported` no vacíe los listados.
type contadorDeLlamadas struct {
	*FakeAdapter
	listas    map[FakeKey]int
	vistosMap map[string]int
}

func (c *contadorDeLlamadas) Forge() string { c.ver("Forge"); return c.FakeAdapter.Forge() }
func (c *contadorDeLlamadas) Host() string  { c.ver("Host"); return c.FakeAdapter.Host() }
func (c *contadorDeLlamadas) Auth(ctx context.Context) model.AuthState {
	c.ver("Auth")
	return c.FakeAdapter.Auth(ctx)
}
func (c *contadorDeLlamadas) List(ctx context.Context, q forge.Query) (forge.Page, []model.Warning) {
	c.ver("List")
	if c.listas == nil {
		c.listas = map[FakeKey]int{}
	}
	c.listas[FakeKey{Section: q.Section, Kind: q.ReviewKind}]++
	return c.FakeAdapter.List(ctx, q)
}
func (c *contadorDeLlamadas) ItemState(ctx context.Context, ref model.RepoRef, n int) (model.Item, []model.Warning) {
	c.ver("ItemState")
	return c.FakeAdapter.ItemState(ctx, ref, n)
}
func (c *contadorDeLlamadas) Comments(ctx context.Context, ref model.RepoRef, n int) (forge.CommentPage, []model.Warning) {
	c.ver("Comments")
	return c.FakeAdapter.Comments(ctx, ref, n)
}
func (c *contadorDeLlamadas) Approve(ctx context.Context, ref model.RepoRef, n int) []model.Warning {
	c.ver("Approve")
	return c.FakeAdapter.Approve(ctx, ref, n)
}
func (c *contadorDeLlamadas) Merge(ctx context.Context, ref model.RepoRef, n int, req forge.MergeRequest) []model.Warning {
	c.ver("Merge")
	return c.FakeAdapter.Merge(ctx, ref, n, req)
}
func (c *contadorDeLlamadas) Retarget(ctx context.Context, ref model.RepoRef, n int, b string) []model.Warning {
	c.ver("Retarget")
	return c.FakeAdapter.Retarget(ctx, ref, n, b)
}
func (c *contadorDeLlamadas) Branches(ctx context.Context, ref model.RepoRef) ([]string, []model.Warning) {
	c.ver("Branches")
	return c.FakeAdapter.Branches(ctx, ref)
}

func (c *contadorDeLlamadas) ver(m string) {
	if c.vistosMap == nil {
		c.vistosMap = map[string]int{}
	}
	c.vistosMap[m]++
}

func (c *contadorDeLlamadas) vistos() map[string]int { return c.vistosMap }

// Un adapter con el Forge y el Host puestos y los cuatro streams configurados, que es lo
// que `RunConformance` espera de uno conforme.
