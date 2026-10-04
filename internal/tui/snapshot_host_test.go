package tui

import (
	"testing"

	"prdash/internal/cache"
	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

// TestElSnapshotGuardaElHostDeCadaForge: cada stream se guarda con el host del forge al
// que pertenece, y no con un host vacío.
//
// Y esto no es un detalle de formato. El host es lo que separa dos streams que son el
// mismo forge en dos instancias: un GitHub.com y un GitHub self-hosted se numeran igual,
// y sin el host guardado no se puede saber cuál es cuál al releer el snapshot. Con la
// condición al revés, el host no se guarda NUNCA —porque con `st != nil` es justo cuando
// hay un estado— y la apertura siguiente lee streams sin instancia.
//
// Y el caso que lo distingue tiene que llevar las dos mitades: un forge CON host, que es
// lo que la condición debe dejar pasar, y uno SIN estado, que es lo que la guarda cubre. El
// segundo es alcanzable: un stream puede venir de un forge que ya no está en la
// configuración, y su estado no existe.
func TestElSnapshotGuardaElHostDeCadaForge(t *testing.T) {
	adapt := &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"}
	m := newTestModel(t, adapt)

	// Un forge con host conocido: es el que la condición tiene que cubrir.
	if st := m.statuses["github"]; st == nil {
		t.Fatal("el adapter registrado no tiene estado, y el test necesita uno con host")
	}

	m.applySnapshot(snapshotCon(mkItem("github", "github.com", "acme/widget", "uno", 1, "")))
	m.rebuild()

	f := m.snapshot()
	if len(f.Streams) == 0 {
		t.Fatal("el snapshot salió sin streams, y el test necesita al menos uno")
	}

	vistos := map[string]bool{}
	for _, s := range f.Streams {
		vistos[s.Forge] = true
		if s.Forge == "github" && s.Host == "" {
			t.Errorf("el stream de github se guardó sin host. El host es lo que separa "+
				"dos streams del mismo forge en dos instancias, y sin él la apertura "+
				"siguiente no puede saber cuál es cuál: %+v", s)
		}
		if s.Forge == "github" && s.Host != "github.com" {
			t.Errorf("el stream de github se guardó con host %q, want github.com", s.Host)
		}
	}
	if !vistos["github"] {
		t.Error("el snapshot no tiene stream de github")
	}

	// Y el caso del otro lado: un stream de un forge SIN estado tiene que guardarse con el
	// host vacío, sin reventar. Un stream puede llegar de un forge que el usuario quitó
	// de la configuración, y perderlo entero sería peor que guardarlo sin host.
	m.applySnapshot(cache.File{Streams: []cache.Stream{{
		Forge:   "gitlab",
		Host:    "gitlab.com",
		Section: "review",
		Kind:    "requested",
		Items:   nil,
	}}})
	m.rebuild()

	f = m.snapshot()
	conHost, sinEstado := 0, 0
	for _, s := range f.Streams {
		if s.Forge == "github" && s.Host == "github.com" {
			conHost++
		}
		if s.Forge == "gitlab" && s.Host == "" {
			sinEstado++
		}
	}
	if conHost == 0 {
		t.Error("después de meter un forge sin estado, el stream de github perdió su host")
	}
	if sinEstado == 0 {
		t.Error("el stream de un forge sin estado no llegó al snapshot: perderlo entero " +
			"sería peor que guardarlo sin host")
	}
}

// TestElSnapshotVaOrdenadoPorForgeSeccionYTipo: el snapshot sale ordenado por forge, y
// dentro de cada forge por sección y tipo, y el orden es el de las letras.
//
// Y la primera versión de este test miraba lo que NO se puede mirar: que el orden fuera
// COHERENTE consigo mismo, comparando contra la primera pasada. Con esa forma el test pasa
// con el comparador invertido, porque un orden invertido también es coherente consigo
// mismo —solo que al revés—. Un assert de estabilidad solo mira que no haya itertools, y
// eso lo cumple cualquier orden.
//
// Lo que hay que afirmar es el ORDEN, y aquí se calcula aparte: la lista de claves que se
// mete a propósito desordenada, y la lista de la misma clave ordenada por las tres columnas.
// Comparar las dos es comparar contra un dato escrito en el test, no contra el resultado de
// una vuelta anterior.
//
// Y los tres niveles de comparación hacen falta, no solo el primero: forge, sección y tipo
// son tres desempates en cascada, y `sort.Slice` con un solo nivel dejaría el resto sin
// ordenar. Y el `!=` del desempate importa: puesto al revés declara ordenados los dos
// sentidos para todo lo que no sea idéntico, y el resultado deja de ser un orden.
func TestElSnapshotVaOrdenadoPorForgeSeccionYTipo(t *testing.T) {
	adapt := &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"}
	m := newTestModel(t, adapt)

	// Las claves, escritas en un orden que NO es el ordenado.
	//
	// Y hay VARIOS tipos por grupo de forge y sección a propósito. El tercer nivel del
	// desempate es `kind`, y hace falta más de un par por grupo para que se vea: con dos
	// por grupo el `sort` acierta igual con un `!=` en el comparador, porque con tan pocos
	// elementos usa orden por inserción y el resultado sale bien por casualidad. Con cinco
	// por grupo el `!=` declara que todo par del grupo está ordenado en los dos sentidos a
	// la vez, y la salida deja de ser un orden.
	claves := []struct{ forge, section, kind string }{
		{"github", "authored", "commented"},
		{"gitlab", "review", "assigned"},
		{"github", "mentions", "requested"},
		{"github", "authored", "requested"},
		{"gitlab", "authored", "approved"},
		{"github", "authored", "approved"},
		{"bitbucket", "review", "assigned"},
		{"github", "review", "commented"},
		{"github", "review", "requested"},
		{"gitlab", "review", "requested"},
		{"gitlab", "review", "approved"},
		{"github", "review", "approved"},
		{"gitlab", "authored", "requested"},
		{"gitlab", "review", "commented"},
		{"github", "authored", "assigned"},
	}
	for _, c := range claves {
		m.applySnapshot(cache.File{Streams: []cache.Stream{{
			Forge: c.forge, Section: model.Section(c.section), Kind: model.ReviewKind(c.kind),
		}}})
	}
	m.rebuild()

	// El orden esperado, escrito en el test y NO derivado de una pasada anterior: forge
	// por letras, y dentro de cada forge sección por letras y tipo por letras.
	want := []string{
		"bitbucket/review/assigned",
		"github/authored/approved",
		"github/authored/assigned",
		"github/authored/commented",
		"github/authored/requested",
		"github/mentions/requested",
		"github/review/approved",
		"github/review/commented",
		"github/review/requested",
		"gitlab/authored/approved",
		"gitlab/authored/requested",
		"gitlab/review/approved",
		"gitlab/review/assigned",
		"gitlab/review/commented",
		"gitlab/review/requested",
	}

	f := m.snapshot()
	if len(f.Streams) != len(want) {
		t.Fatalf("el snapshot tiene %d streams, want %d", len(f.Streams), len(want))
	}
	for i, s := range f.Streams {
		got := s.Forge + "/" + string(s.Section) + "/" + string(s.Kind)
		if got != want[i] {
			t.Errorf("stream %d es %q, want %q. El snapshot se ordena por forge, luego "+
				"sección y luego tipo, y el orden es el de las letras: se compara entre "+
				"ejecuciones para saber si el inbox ha cambiado, y un orden invertido "+
				"produce un «ha cambiado» cada vez que se abre",
				i, got, want[i])
		}
	}
}
