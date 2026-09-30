package tui

import (
	"strings"
	"testing"
	"time"

	"prdash/internal/forge/model"
)

// TestOrderBranchesDejaLaBasePrimeraY deduplica: el buscador de ramas tiene un
// orden que no es un capricho. La base actual sale la PRIMERA porque es la única
// fila que describe el punto de partida, y en un repositorio largo es lo primero
// que se va de la ventana. El resto va alfabético, que es el único orden que se
// puede anticipar sin recorrerlo entero. Y un nombre repetido produce dos filas
// idénticas que parecen dos destinos, cuando elegir una u otra no cambia nada.
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
			// Y la base, si está, es la primera: es la invariante que la ventana
			// necesita para no perderla antes de que el usuario llegue al final.
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

// TestFilterBranchesFiltraSobreElNombreEnteroESinMayusculas: el filtro es una
// ayuda para recordar, no una escritura —el nombre que sale lo pone el forge, no
// el teclado—, así que no distingue mayúsculas. Y va sobre el NOMBRE ENTERO, no
// sobre el último segmento: `fix` tiene que encontrar `fix/hunk-pane-argv`, que
// es donde están las ramas de trabajo.
func TestFilterBranchesFiltraSobreElNombreEnteroESinMayusculas(t *testing.T) {
	all := []string{"main", "release/2.0", "fix/hunk-pane-argv", "feature/Fix-Login", "wip"}

	if got := filterBranches(all, ""); strings.Join(got, ",") != strings.Join(all, ",") {
		t.Errorf("filtro vacío = %q, want la lista entera", got)
	}
	// Sin filtro NO se devuelve el slice original: el llamante lo muta.
	if got := filterBranches(all, ""); &got[0] == &all[0] {
		t.Error("filterBranches devolvió el slice de entrada, no una copia")
	}
	if got := filterBranches(all, "   "); len(got) != len(all) {
		t.Errorf("filtro de espacios = %q, want la lista entera (se ignora el filtro vacío)", got)
	}

	// El nombre entero, no el último segmento: "hunk" solo aparece en
	// "fix/hunk-pane-argv", así que un filtro sobre el último segmento no lo
	// encontraría nunca.
	if got := filterBranches(all, "hunk"); len(got) != 1 || got[0] != "fix/hunk-pane-argv" {
		t.Errorf("filtro %q = %q, want solo la rama de trabajo (el filtro va sobre el nombre entero)", "hunk", got)
	}
	// Sin distinguir mayúsculas, en el filtro y en el nombre: "fix" encuentra
	// también "feature/Fix-Login", que es justo lo que hace útil el buscador.
	if got := filterBranches(all, "fix"); len(got) != 2 {
		t.Errorf("filtro %q = %q, want las dos que contienen fix sin distinguir caso", "fix", got)
	}
	if got := filterBranches(all, "FIX"); len(got) != 2 {
		t.Errorf("filtro %q = %q, want lo mismo que en minúsculas", "FIX", got)
	}
	if got := filterBranches(all, "2.0"); len(got) != 1 || got[0] != "release/2.0" {
		t.Errorf("filtro %q = %q", "2.0", got)
	}
	// Y un filtro que no casa deja la lista vacía, no la entera: un buscador que
	// no encuentra lo que se le pide y aun así muestra todo está mintiendo.
	if got := filterBranches(all, "noexiste"); len(got) != 0 {
		t.Errorf("filtro sin coincidencias = %q, want vacío", got)
	}
}

// TestClampRetargetCursorNoDejaElCursorFuera: el filtro encoge la vista y el
// cursor se queda donde estaba, en una fila que ya no existe. Si no se corrigiera,
// el `enter` aplicaría una base que el usuario no está viendo. Con la vista
// vacía el cursor es cero (no -1), porque es el único índice que existe.
func TestClampRetargetCursorNoDejaElCursorFuera(t *testing.T) {
	m := newTestModel(t, ghAdapter())

	// Vista vacía: el cursor es cero, no un índice negativo.
	m.retarget.view = nil
	m.retarget.cursor = 7
	m.clampRetargetCursor()
	if m.retarget.cursor != 0 {
		t.Errorf("con la vista vacía el cursor = %d, want 0", m.retarget.cursor)
	}
	// Y selectedBranch no inventa una rama con el cursor a cero sobre nada.
	if b, ok := m.selectedBranch(); ok || b != "" {
		t.Errorf("con la vista vacía selectedBranch = (%q, %v), want (\"\", false)", b, ok)
	}

	view := []string{"a", "b", "c"}
	// Cursor dentro de rango: no se toca, incluso en las dos fronteras.
	for _, c := range []int{0, 1, 2} {
		m.retarget.view, m.retarget.cursor = view, c
		m.clampRetargetCursor()
		if m.retarget.cursor != c {
			t.Errorf("cursor %d se movió a %d sin motivo", c, m.retarget.cursor)
		}
	}
	// Cursor pasado: cae a la ÚLTIMA fila, que es lo que el usuario está viendo
	// de verdad. Un cursor menor del principio es imposible aquí (lo pone el
	// filtro a cero), pero el clamp lo cubre igual.
	for _, c := range []int{3, 4, 100} {
		m.retarget.view, m.retarget.cursor = view, c
		m.clampRetargetCursor()
		if m.retarget.cursor != len(view)-1 {
			t.Errorf("cursor %d quedó en %d, want %d (la última fila)", c, m.retarget.cursor, len(view)-1)
		}
	}
	// Y el borde de selectedBranch: el último índice es válido, uno más no.
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

// TestElCacheDeRamasVenceJustoEnLaFrontera: un caché de TTL que se quedara un
// instante de más, o se fuera un instante antes, hace que el buscador no coincida
// con lo que el repositorio tiene. El borde es estricto: al cumplir el TTL
// EXACTO el listado ya no vale y hay que volver a pedirlo. Un instante antes sí
// vale, que es la diferencia entre una caducidad exacta y una que marea.
func TestElCacheDeRamasVenceJustoEnLaFrontera(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	key := keyOf(m.retargetItemForTest())

	// Cacheado ahora mismo: vale.
	m.storeBranches(key, []string{"main", "otra"})
	if got, ok := m.cachedBranches(key); !ok || len(got) != 2 {
		t.Errorf("recién cacheado = (%q, %v), want los dos nombres", got, ok)
	}

	// Justo en el TTL: vence. Este es el caso que separa `>` de `>=`.
	m.branchCache[key] = branchCache{names: []string{"main"}, fetchedAt: time.Now().Add(-branchCacheTTL)}
	// El reloj no para entre dos llamadas, así que se mide con un margen: se
	// busca el instante en el que el valor de `time.Since` cruza el TTL. Un
	// instante ANTES del TTL tiene que valer, y uno DESPUÉS, no.
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

	// Un repositorio sin entrada NO se confunde con uno con lista vacía: el
	// primero hay que pedirlo, el segundo no se puede pedir de nuevo.
	if got, ok := m.cachedBranches(repoKey{forge: "otro", host: "otro.example.com", project: "x/y"}); ok || got != nil {
		t.Errorf("un repo sin cachear dio (%q, %v), want (nil, false)", got, ok)
	}
	m.storeBranches(key, nil)
	if got, ok := m.cachedBranches(key); !ok || got != nil {
		t.Errorf("un repo con lista vacía cacheada dio (%q, %v), want (nil, true): ya se preguntó y no había nada", got, ok)
	}

	// Y la clave lleva forge y host: el mismo owner/repo en dos forges son dos
	// listas distintas, y confundirlas hace que un popup ofrezca ramas que el
	// forge de al lado no tiene.
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

// TestCerrarElPopupInvalidaElListadoEnVuelo: cerrar el popup tiene que caducar
// el pedido que estaba en vuelo. Sin esa invalidación, un listado que tarda
// reabre el popup solo, con datos de un repo al que el usuario ya renunció, y lo
// aplica si el cursor cae donde toca.
//
// La prueba envía el mensaje con el seq ANTERIOR al cierre, que es el caso real:
// el mensaje se generó antes de que el usuario cerrara. Si el cierre no bumpara
// la secuencia, ese mensaje se acepta.
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

	// El mensaje llega con la secuencia que tenía antes del cierre: es obsleto.
	m = send(t, m, branchesMsg{seq: seqAntes, names: []string{"otra/cosa"}})
	if m.retarget.state != retargetClosed {
		t.Errorf("un listado con la secuencia anterior al cierre reabrió el popup: estado = %v", m.retarget.state)
	}
	if len(m.retarget.view) != 0 {
		t.Errorf("un listado obsoleto llenó la vista: %q", m.retarget.view)
	}
	// Y con el error, tampoco: se pintaría un aviso de un repo al que se renunció.
	m = send(t, m, branchesMsg{seq: seqAntes, errMsg: "gh: Not Found"})
	if stripANSI(m.retargetOverlay2()) != "" {
		t.Errorf("un error obsoleto pintó un aviso: %q", stripANSI(m.retargetOverlay2()))
	}

	// Y el filtro no es un muro: con el popup ABIERTO y la secuencia vigente, el
	// listado se acepta. Sin este caso, una secuencia que no avanzara al cerrar
	// pasaría el test de arriba sin que nada se entere.
	//
	// Cerrado, en cambio, el popup se queda cerrado pase lo que pase: un listado
	// que llega tarde no puede reabrir por su cuenta algo que el usuario cerró.
	// Eso lo decide el ESTADO, no la secuencia, y son dos guardas distintas.
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

// TestBackspaceBorraUnRuneYNoMas: el filtro se borra rune a rune, no por
// graphemes, y eso tiene una consecuencia observable: una tecla de composición
// deshecha a medias deja un carácter de más, que el siguiente `backspace` quita.
// Lo que no puede pasar es que `backspace` en un filtro vacío haga algo, porque
// eso borraría una letra de la lista de por debajo.
func TestBackspaceBorraUnRuneYNoMas(t *testing.T) {
	m, _ := retargetFixture(t, "main", "release/2.0")
	m = pressFilter(t, m, "fi")

	m = press(t, m, "backspace")
	if m.retarget.query != "f" {
		t.Errorf("query = %q tras un backspace, want %q", m.retarget.query, "f")
	}
	// Con un carácter multibyte: un rune, no un byte. Sin esto, "é" se
	// deshace a la mitad y deja un texto que ni es el que había ni se puede
	// escribir.
	m = pressFilter(t, m, "é")
	m = press(t, m, "backspace")
	if m.retarget.query != "f" {
		t.Errorf("query = %q tras borrar un rune multibyte, want %q", m.retarget.query, "f")
	}
	// Vaciar del todo.
	m = press(t, m, "backspace")
	if m.retarget.query != "" {
		t.Errorf("query = %q, want vacía", m.retarget.query)
	}
	// Y con el filtro ya vacío, `backspace` no toca NADA: la lista de por debajo
	// sigue intacta y el overlay sigue abierto.
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
	// Y con el filtro vacío la vista es el listado entero, en orden.
	if got := m.retarget.view; len(got) != len(m.retarget.all) {
		t.Errorf("con el filtro vacío la vista son %d de %d ramas", len(got), len(m.retarget.all))
	}
	_ = cursorAntes
}

// TestApplyQueryVuelveAlPrincipioYACortaLaVista: escribir un carácter más con
// el cursor abajo dejaría seleccionada una fila que el filtro acaba de mover, y el
// `enter` de después aplicaría una base que el usuario no está viendo. Volver
// al principio es lo único que no sorprende, porque nada se aplica sin haberse
// leído, y cuesta una flecha.
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
	// Y el listado completo NO se toca: es lo que se restaura al vaciar el
	// filtro, y recortarlo en su lugar dejaría el buscador sin salida.
	if len(m.retarget.all) != 4 {
		t.Errorf("el listado se recortó al filtrar: %q", m.retarget.all)
	}
	// Un filtro que no casa deja la vista vacía y el cursor a cero, que es lo
	// que permite decir "no hay coincidencias" sin SelectedBranch mintiendo.
	m.retarget.query = "noexiste"
	m.applyQuery()
	if len(m.retarget.view) != 0 {
		t.Errorf("un filtro sin coincidencias dio vista %q", m.retarget.view)
	}
	if m.retarget.cursor != 0 {
		t.Errorf("cursor = %d con la vista vacía, want 0", m.retarget.cursor)
	}
}

// TestKeyOfSeparaForgeYHost: la clave del caché de ramas son forge, host y
// proyecto. Sin forge y host, el mismo owner/repo en GitHub y en un GitLab
// self-managed son la misma clave, y el buscador ofrece al usuario ramas que el
// forge de al lado no tiene: un destino que no existe.
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
	// Y el proyecto también, y el mismo repo en el mismo forge es la misma clave
	// (que es lo que hace que la segunda apertura salga del caché).
	if gh != keyOf(mk("github", "github.com", "acme/widget")) {
		t.Error("el mismo repo en el mismo forge dio dos claves distintas")
	}
	if keyOf(mk("github", "github.com", "acme/widget")) == keyOf(mk("github", "github.com", "otro/widget")) {
		t.Error("dos proyectos distintos dieron la misma clave")
	}
}
