package tui

import (
	"strings"
	"testing"
	"time"

	"prdash/internal/forge/model"
)

func TestOrderBranchesDejaLaBasePrimeraYDeduplica(t *testing.T) {
	cases := []struct {
		name  string
		names []string
		base  string
		want  []string
	}{
		{
			"la base primera aunque sea la ultima alfabeticamente",
			[]string{"zebra", "main", "alfa"},
			"main",
			[]string{"main", "alfa", "zebra"},
		},
		{
			"sin base, todo alfabetico",
			[]string{"zebra", "alfa", "medio"},
			"",
			[]string{"alfa", "medio", "zebra"},
		},
		{
			"sin base y base ausente del listado",
			[]string{"zebra", "alfa"},
			"main",
			[]string{"alfa", "zebra"},
		},
		{
			"deduplica conservando la primera aparicion",
			[]string{"main", "alfa", "main", "alfa"},
			"main",
			[]string{"main", "alfa"},
		},
		{
			"descarta vacios y espacios",
			[]string{"main", "", "   ", "alfa", "\talfa\t"},
			"main",
			[]string{"main", "alfa"},
		},
		{
			"solo la base",
			[]string{"main"},
			"main",
			[]string{"main"},
		},
		{
			"la base repetida no repite la fila",
			[]string{"main", "main", "otro"},
			"main",
			[]string{"main", "otro"},
		},
		{
			"nada",
			nil,
			"main",
			nil,
		},
		{
			"solo espacios",
			[]string{"", "  "},
			"main",
			nil,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := orderBranches(c.names, c.base)
			if strings.Join(got, ",") != strings.Join(c.want, ",") {
				t.Errorf("orderBranches(%q, %q) = %q, want %q", c.names, c.base, got, c.want)
			}
			// The base, if any, goes first: the invariant the window needs to keep it in view.
			if c.base != "" && contains(got, c.base) && len(got) > 0 && got[0] != c.base {
				t.Errorf("la base %q no salió la primera: %q", c.base, got)
			}
		})
	}
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func TestFilterBranchesFiltraSobreElNombreEnteroESinMayusculas(t *testing.T) {
	all := []string{"main", "release/2.0", "fix/hunk-pane-argv", "feature/Fix-Login", "wip"}

	if got := filterBranches(all, ""); strings.Join(got, ",") != strings.Join(all, ",") {
		t.Errorf("filtro vacío = %q, want la lista entera", got)
	}
	if got := filterBranches(all, ""); &got[0] == &all[0] {
		t.Error("filterBranches devolvió el slice de entrada, no una copia")
	}
	if got := filterBranches(all, "   "); len(got) != len(all) {
		t.Errorf("filtro de espacios = %q, want la lista entera (se ignora el filtro vacío)", got)
	}

	// The WHOLE name, not the last segment: "hunk" only appears in "fix/hunk-pane-argv".
	if got := filterBranches(all, "hunk"); len(got) != 1 || got[0] != "fix/hunk-pane-argv" {
		t.Errorf("filtro %q = %q, want solo la rama de trabajo (el filtro va sobre el nombre entero)", "hunk", got)
	}
	if got := filterBranches(all, "fix"); len(got) != 2 {
		t.Errorf("filtro %q = %q, want las dos que contienen fix sin distinguir caso", "fix", got)
	}
	if got := filterBranches(all, "FIX"); len(got) != 2 {
		t.Errorf("filtro %q = %q, want lo mismo que en minúsculas", "FIX", got)
	}
	if got := filterBranches(all, "2.0"); len(got) != 1 || got[0] != "release/2.0" {
		t.Errorf("filtro %q = %q", "2.0", got)
	}
	if got := filterBranches(all, "noexiste"); len(got) != 0 {
		t.Errorf("filtro sin coincidencias = %q, want vacío", got)
	}
}

// The filter shrinks the view and the cursor is clamped to it.
func TestClampRetargetCursorNoDejaElCursorFuera(t *testing.T) {
	m := newTestModel(t, ghAdapter())

	m.retarget.view = nil
	m.retarget.cursor = 7
	m.clampRetargetCursor()
	if m.retarget.cursor != 0 {
		t.Errorf("con la vista vacía el cursor = %d, want 0", m.retarget.cursor)
	}
	if b, ok := m.selectedBranch(); ok || b != "" {
		t.Errorf("con la vista vacía selectedBranch = (%q, %v), want (\"\", false)", b, ok)
	}

	view := []string{"a", "b", "c"}
	for _, c := range []int{0, 1, 2} {
		m.retarget.view, m.retarget.cursor = view, c
		m.clampRetargetCursor()
		if m.retarget.cursor != c {
			t.Errorf("cursor %d se movió a %d sin motivo", c, m.retarget.cursor)
		}
	}
	for _, c := range []int{3, 4, 100} {
		m.retarget.view, m.retarget.cursor = view, c
		m.clampRetargetCursor()
		if m.retarget.cursor != len(view)-1 {
			t.Errorf("cursor %d quedó en %d, want %d (la última fila)", c, m.retarget.cursor, len(view)-1)
		}
	}
	m.retarget.view, m.retarget.cursor = view, 2
	if b, ok := m.selectedBranch(); !ok || b != "c" {
		t.Errorf("en la última fila selectedBranch = (%q, %v), want (\"c\", true)", b, ok)
	}
	m.retarget.cursor = 3
	if b, ok := m.selectedBranch(); ok || b != "" {
		t.Errorf("una fila por encima del final dio (%q, %v), want (\"\", false)", b, ok)
	}
	m.retarget.cursor = -1
	if b, ok := m.selectedBranch(); ok || b != "" {
		t.Errorf("un cursor negativo dio (%q, %v), want (\"\", false)", b, ok)
	}
}

func TestElCacheDeRamasVenceJustoEnLaFrontera(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	key := keyOf(m.retargetItemForTest())

	m.storeBranches(key, []string{"main", "otra"})
	if got, ok := m.cachedBranches(key); !ok || len(got) != 2 {
		t.Errorf("recién cacheado = (%q, %v), want los dos nombres", got, ok)
	}

	m.branchCache[key] = branchCache{names: []string{"main"}, fetchedAt: time.Now().Add(-branchCacheTTL)}
	fresco := true
	for _, delta := range []time.Duration{-time.Second, -time.Millisecond} {
		m.branchCache[key] = branchCache{names: []string{"main"}, fetchedAt: time.Now().Add(-branchCacheTTL - delta)}
		if _, ok := m.cachedBranches(key); !ok {
			fresco = false
			t.Errorf("un caché de %v antes de cumplir el TTL caducó: el TTL se está aplicando antes de tiempo", -delta)
		}
	}
	_ = fresco
	for _, delta := range []time.Duration{time.Millisecond, time.Second} {
		m.branchCache[key] = branchCache{names: []string{"main"}, fetchedAt: time.Now().Add(-branchCacheTTL - delta)}
		if _, ok := m.cachedBranches(key); ok {
			t.Errorf("un caché de %v después del TTL seguía valiendo", -delta)
		}
	}

	if got, ok := m.cachedBranches(repoKey{forge: "otro", host: "otro.example.com", project: "x/y"}); ok || got != nil {
		t.Errorf("un repo sin cachear dio (%q, %v), want (nil, false)", got, ok)
	}
	m.storeBranches(key, nil)
	if got, ok := m.cachedBranches(key); !ok || got != nil {
		t.Errorf("un repo con lista vacía cacheada dio (%q, %v), want (nil, true): ya se preguntó y no había nada", got, ok)
	}

	// The key carries forge and host: the same owner/repo in two forges are two lists.
	otro := repoKey{forge: "gitlab", host: key.host, project: key.project}
	m.storeBranches(otro, []string{"main", "solo-gitlab"})
	m.storeBranches(key, []string{"main", "solo-github"})
	if got, _ := m.cachedBranches(otro); !contains(got, "solo-gitlab") {
		t.Errorf("la clave de gitlab devolvió %q", got)
	}
	if got, _ := m.cachedBranches(key); contains(got, "solo-gitlab") {
		t.Errorf("la clave de github devolvió ramas de gitlab: %q", got)
	}
}

func TestCerrarElPopupInvalidaElListadoEnVuelo(t *testing.T) {
	m, _ := retargetFixture(t, "main", "release/2.0")
	seqAntes := m.branchSeq

	m = press(t, m, "esc")
	if m.branchSeq == seqAntes {
		t.Fatal("cerrar el popup debería invalidar el listado en vuelo (subir la secuencia)")
	}
	if m.retarget.state != retargetClosed {
		t.Fatalf("estado = %v, want cerrado", m.retarget.state)
	}

	m = send(t, m, branchesMsg{seq: seqAntes, names: []string{"otra/cosa"}})
	if m.retarget.state != retargetClosed {
		t.Errorf("un listado con la secuencia anterior al cierre reabrió el popup: estado = %v", m.retarget.state)
	}
	if len(m.retarget.view) != 0 {
		t.Errorf("un listado obsoleto llenó la vista: %q", m.retarget.view)
	}
	m = send(t, m, branchesMsg{seq: seqAntes, errMsg: "gh: Not Found"})
	if stripANSI(m.retargetOverlay2()) != "" {
		t.Errorf("un error obsoleto pintó un aviso: %q", stripANSI(m.retargetOverlay2()))
	}

	abierto, _ := retargetFixture(t, "main", "release/2.0")
	abierto = send(t, abierto, branchesMsg{seq: abierto.branchSeq, names: []string{"main", "otra"}})
	if abierto.retarget.state != retargetChoosing {
		t.Errorf("con la secuencia vigente y el popup abierto debería buscar: %v", abierto.retarget.state)
	}
	if len(abierto.retarget.view) != 2 {
		t.Errorf("vista = %q, want las dos ramas", abierto.retarget.view)
	}
	abierto = press(t, abierto, "esc")
	abierto = send(t, abierto, branchesMsg{seq: abierto.branchSeq, names: []string{"nueva"}})
	if abierto.retarget.state != retargetClosed || len(abierto.retarget.view) != 0 {
		t.Errorf("un listado con la secuencia vigente reabrió un popup cerrado: %v, %q",
			abierto.retarget.state, abierto.retarget.view)
	}
}

// The filter clears rune by rune, not by grapheme.
func TestBackspaceBorraUnRuneYNoMas(t *testing.T) {
	m, _ := retargetFixture(t, "main", "release/2.0")
	m = pressFilter(t, m, "fi")

	m = press(t, m, "backspace")
	if m.retarget.query != "f" {
		t.Errorf("query = %q tras un backspace, want %q", m.retarget.query, "f")
	}
	m = pressFilter(t, m, "é")
	m = press(t, m, "backspace")
	if m.retarget.query != "f" {
		t.Errorf("query = %q tras borrar un rune multibyte, want %q", m.retarget.query, "f")
	}
	m = press(t, m, "backspace")
	if m.retarget.query != "" {
		t.Errorf("query = %q, want vacía", m.retarget.query)
	}
	viewAntes := append([]string(nil), m.retarget.view...)
	cursorAntes := m.retarget.cursor
	allAntes := append([]string(nil), m.retarget.all...)
	m = press(t, m, "backspace")
	if m.retarget.query != "" {
		t.Errorf("backspace con el filtro vacío escribió %q", m.retarget.query)
	}
	if m.retarget.state != retargetChoosing {
		t.Errorf("backspace con el filtro vacío cerró el popup: %v", m.retarget.state)
	}
	if len(m.retarget.view) != len(viewAntes) {
		t.Errorf("backspace con el filtro vacío cambió la vista: %q -> %q", viewAntes, m.retarget.view)
	}
	if len(m.retarget.all) != len(allAntes) {
		t.Errorf("backspace con el filtro vacío cambió el listado: %q -> %q", allAntes, m.retarget.all)
	}
	if got := m.retarget.view; len(got) != len(m.retarget.all) {
		t.Errorf("con el filtro vacío la vista son %d de %d ramas", len(got), len(m.retarget.all))
	}
	_ = cursorAntes
}

func TestApplyQueryVuelveAlPrincipioYACortaLaVista(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m.retarget.all = []string{"main", "fix/uno", "fix/dos", "wip"}
	m.retarget.view = append([]string(nil), m.retarget.all...)
	m.retarget.cursor = 3
	m.retarget.win = 2

	m.retarget.query = "fix"
	m.applyQuery()

	if m.retarget.cursor != 0 {
		t.Errorf("cursor = %d tras filtrar, want 0: el enter aplicaría una fila que se movió", m.retarget.cursor)
	}
	if m.retarget.win != 0 {
		t.Errorf("win = %d tras filtrar, want 0: la ventana se reencuadra con la vista nueva", m.retarget.win)
	}
	if got := m.retarget.view; len(got) != 2 || got[0] != "fix/uno" {
		t.Errorf("vista = %q, want las dos ramas de fix", got)
	}
	// The FULL listing is what clearing the filter restores, so it must not be trimmed in place.
	if len(m.retarget.all) != 4 {
		t.Errorf("el listado se recortó al filtrar: %q", m.retarget.all)
	}
	m.retarget.query = "noexiste"
	m.applyQuery()
	if len(m.retarget.view) != 0 {
		t.Errorf("un filtro sin coincidencias dio vista %q", m.retarget.view)
	}
	if m.retarget.cursor != 0 {
		t.Errorf("cursor = %d con la vista vacía, want 0", m.retarget.cursor)
	}
}

// Forge, host and project: without forge and host the same owner/repo in two forges collides.
func TestKeyOfSeparaForgeYHost(t *testing.T) {
	mk := func(forge, host, project string) model.Item {
		return model.NewItem(model.RepoRef{Forge: forge, Host: host, Project: project}, 1)
	}
	gh := keyOf(mk("github", "github.com", "acme/widget"))
	gl := keyOf(mk("gitlab", "gitlab.example.com", "acme/widget"))
	self := keyOf(mk("github", "github.enterprise.corp", "acme/widget"))

	if gh == gl || gh == self || gl == self {
		t.Errorf("forge y host tienen que entrar en la clave: %+v %+v %+v", gh, gl, self)
	}
	if gh != keyOf(mk("github", "github.com", "acme/widget")) {
		t.Error("el mismo repo en el mismo forge dio dos claves distintas")
	}
	if keyOf(mk("github", "github.com", "acme/widget")) == keyOf(mk("github", "github.com", "otro/widget")) {
		t.Error("dos proyectos distintos dieron la misma clave")
	}
}
