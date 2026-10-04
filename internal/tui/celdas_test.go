package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"prdash/internal/forge/model"
	"prdash/internal/state"
)

// Two languages in one function, on purpose: four labels in English and three in Spanish. What is
// pinned is that neither list grows into the other.
func TestLaEtiquetaDelProblemaTraduceLoQueSeSabeYLoQueNo(t *testing.T) {
	casos := []struct {
		kind string
		want string
	}{
		{"auth", "not authenticated"},
		{"timeout", "timeout"},
		{"ratelimit", "rate limited"},
		{"parse", "respuesta ilegible"},
		{"unsupported", "no soportado"},
		{"validation", "rechazado"},
		{"network", "no connection"},
		{"algo-nuevo", "algo-nuevo"},
		{"", ""},
	}
	for _, c := range casos {
		got := problemLabel(c.kind)
		if got != c.want {
			t.Errorf("problemLabel(%q) dio %q, want %q", c.kind, got, c.want)
		}
		if got != strings.TrimSpace(got) {
			t.Errorf("problemLabel(%q) dio %q con espacios en los bordes", c.kind, got)
		}
	}
	for _, kind := range []string{"auth", "timeout", "ratelimit", "parse", "unsupported", "validation", "network"} {
		if l := len(problemLabel(kind)); l > 18 {
			t.Errorf("problemLabel(%q) son %d caracteres y el borde no tiene ese ancho", kind, l)
		}
	}
}

// Down, not up: a label saying 59s when a minute has passed reads as fresh data when it is not.
func TestLaEdadDeLaUltimaActualizacionSeRedondeaHaciaAbajo(t *testing.T) {
	ahora := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)

	casos := []struct {
		nombre string
		hace   time.Duration
		want   string
	}{
		{"ahora mismo", time.Millisecond, "now"},
		{"59 segundos", 59 * time.Second, "59s ago"},
		{"60 segundos", 60 * time.Second, "1m ago"},
		{"90 segundos", 90 * time.Second, "1m ago"},
		{"59 minutos", 59 * time.Minute, "59m ago"},
		{"60 minutos", 60 * time.Minute, "1h ago"},
		{"25 horas", 25 * time.Hour, "25h ago"},
	}
	for _, c := range casos {
		if got := lastRefreshLabel(ahora.Add(-c.hace), ahora); got != c.want {
			t.Errorf("%s (%s): dio %q, want %q", c.nombre, c.hace, got, c.want)
		}
	}

	if got := lastRefreshLabel(time.Time{}, ahora); got != "no data" {
		t.Errorf("un since a cero dio %q, want \"no data\"", got)
	}

	// The property that matters: read the label back as a duration and you get the real age within one
	// label unit.
	for _, d := range []time.Duration{
		time.Second, 30 * time.Second, 59 * time.Second,
		60 * time.Second, 90 * time.Second,
		30 * time.Minute, 59 * time.Minute, 60 * time.Minute, 90 * time.Minute,
		3 * time.Hour, 50 * time.Hour,
	} {
		got := lastRefreshLabel(ahora.Add(-d), ahora)
		representado, err := duracionDe(got)
		if err != nil {
			t.Errorf("el texto %q no se pudo volver a leer: %v", got, err)
			continue
		}
		// The error fits ONE label unit, not a fixed minute: at 1h30m that rounds to "1h ago" with 30
		// minutes of difference, which is rounding and not a failure.
		_, unid := numeroDe(got)
		if diff := d - representado; diff < 0 || diff >= duracionDeUnidad(unid) {
			t.Errorf("con %s la etiqueta %q representa %s: el error no cabe en una unidad "+
				"de esa etiqueta", d, got, representado)
		}
	}
	var previa int
	for _, d := range []time.Duration{time.Second, 59 * time.Second, 60 * time.Second,
		59 * time.Minute, 60 * time.Minute, 3 * time.Hour} {
		_, unid := numeroDe(lastRefreshLabel(ahora.Add(-d), ahora))
		if unid < previa {
			t.Errorf("con %s la unidad %d retrocede desde %d", d, unid, previa)
		}
		previa = unid
	}
}

// The CLI's text and not an invented one: an invented "could not read the branches" would hide both a
// 404 and an expired token.
func TestElAvisoDeRamasUsaElTextoDeLaCLI(t *testing.T) {
	got := warnMsg([]model.Warning{{Kind: "auth", Msg: "gh: Bad credentials"}})
	if got != "gh: Bad credentials" {
		t.Errorf("warnMsg dio %q, want el texto de la CLI sin tocar", got)
	}
	got = warnMsg([]model.Warning{
		{Kind: "auth", Msg: "el que explica"},
		{Kind: "network", Msg: "otro distinto"},
	})
	if got != "el que explica" {
		t.Errorf("warnMsg dio %q, want el primero con texto", got)
	}
	got = warnMsg([]model.Warning{
		{Kind: "a", Msg: "   "},
		{Kind: "b", Msg: "\t\n"},
		{Kind: "c", Msg: "el bueno"},
	})
	if got != "el bueno" {
		t.Errorf("warnMsg dio %q, want el que tenía texto de verdad", got)
	}
	for _, warns := range [][]model.Warning{
		nil,
		{},
		{{Kind: "auth", Msg: ""}},
		{{Kind: "a", Msg: "  "}},
	} {
		if got := warnMsg(warns); strings.TrimSpace(got) == "" {
			t.Errorf("warnMsg(%v) devolvió texto vacío", warns)
		}
	}
	if got := warnMsg(nil); got != "the branches could not be read" {
		t.Errorf("sin avisos dio %q, want el genérico", got)
	}
}

// Two forms of the same datum because of width: forgeLabel lives in the detail, where the whole path
// fits; forgeBadge in the column, where it does not.
func TestElForgeSeIdentificaDeDosFormasYLasDosCaben(t *testing.T) {
	casos := []struct {
		nombre string
		it     model.Item
		larga  string
		corta  string
	}{
		{
			nombre: "github publico",
			it:     model.Item{Forge: "github", Host: "github.com"},
			larga:  "github@github.com",
			corta:  "GH",
		},
		{
			nombre: "gitlab publico",
			it:     model.Item{Forge: "gitlab", Host: "gitlab.com"},
			larga:  "gitlab@gitlab.com",
			corta:  "GLab",
		},
		{
			nombre: "bitbucket publico",
			it:     model.Item{Forge: "bitbucket", Host: "bitbucket.org"},
			larga:  "bitbucket@bitbucket.org",
			corta:  "BB",
		},
		{
			nombre: "gitlab self-managed",
			it:     model.Item{Forge: "gitlab", Host: "git.umane.example"},
			larga:  "gitlab@git.umane.example",
			corta:  "GLab@git",
		},
		{
			nombre: "host sin puntos",
			it:     model.Item{Forge: "gitlab", Host: "localhost"},
			larga:  "gitlab@localhost",
			corta:  "GLab@localhost",
		},
		{
			// An unknown forge uses the FULL name: "PHab" would be an invention resembling the other
			// abbreviations and meaning nothing.
			nombre: "forge desconocido",
			it:     model.Item{Forge: "phabricator", Host: "phab.example"},
			larga:  "phabricator@phab.example",
			corta:  "phabricator@phab",
		},
		{
			nombre: "sin host",
			it:     model.Item{Forge: "github"},
			larga:  "github",
			corta:  "GH",
		},
		{
			nombre: "forge vacío",
			it:     model.Item{},
			larga:  "",
			corta:  "",
		},
		{
			nombre: "host publico en mayusculas",
			it:     model.Item{Forge: "github", Host: "GitHub.COM"},
			larga:  "github@GitHub.COM",
			corta:  "GH",
		},
	}
	for _, c := range casos {
		if got := forgeLabel(c.it); got != c.larga {
			t.Errorf("%s: forgeLabel dio %q, want %q", c.nombre, got, c.larga)
		}
		if got := forgeBadge(c.it); got != c.corta {
			t.Errorf("%s: forgeBadge dio %q, want %q", c.nombre, got, c.corta)
		}
		if len(c.corta) > len(c.larga) {
			t.Errorf("%s: la etiqueta corta %q es más larga que la larga %q", c.nombre, c.corta, c.larga)
		}
		if got := forgeBadge(c.it); got != strings.TrimSpace(got) {
			t.Errorf("%s: forgeBadge dio %q con espacios", c.nombre, got)
		}
	}
}

func TestElPapelDelUsuarioDistingueOwnDeReview(t *testing.T) {
	viewer := "yo"

	if got := roleText(model.Item{ReviewKind: model.ReviewRequested}, viewer); got != "review req" {
		t.Errorf("review requested dio %q", got)
	}
	if got := roleText(model.Item{ReviewKind: model.ReviewAssigned}, viewer); got != "assigned" {
		t.Errorf("review assigned dio %q", got)
	}
	// An owned item with a pending review stays "review req": the forge says a review is waiting, and
	// the viewer is who has to look at it, author or not.
	propioConReview := model.Item{ReviewKind: model.ReviewRequested, Author: viewer}
	if got := roleText(propioConReview, viewer); got != "review req" {
		t.Errorf("un review pedido sobre un item propio dio %q, want review req", got)
	}

	sinReview := model.Item{Author: "otro", Title: "algo"}
	if got := roleText(sinReview, viewer); got != "-" {
		t.Errorf("un ítem de otro sin review dio %q, want -", got)
	}
	if got := roleText(model.Item{Author: viewer, Title: "algo"}, viewer); got != "own" {
		t.Errorf("un ítem propio dio %q, want own", got)
	}
	if got := roleText(sinReview, ""); got == "" {
		t.Error("con viewer vacío la etiqueta salió vacía")
	}
	if got := roleText(model.Item{Section: model.SectionAuthored, Author: "alguien"}, ""); got != "own" {
		t.Errorf("un item creado con viewer vacío dio %q, want own", got)
	}
	for _, kind := range []model.ReviewKind{model.ReviewRequested, model.ReviewAssigned, ""} {
		if got := roleText(model.Item{ReviewKind: kind}, viewer); got != strings.TrimSpace(got) {
			t.Errorf("roleText dio %q con espacios para kind %q", got, kind)
		}
	}
}

// Failing carries the count and passing carries none, on purpose: a big number is what makes a red
// cell worth reading.
func TestLosChecksSeResumenConElRecuentoYElSemaforo(t *testing.T) {
	casos := []struct {
		nombre  string
		checks  model.Checks
		wantTxt string
	}{
		{"failing con recuento", model.Checks{State: model.ChecksFailing, Failing: 3, Total: 12}, "✗3"},
		{"failing de uno", model.Checks{State: model.ChecksFailing, Failing: 1}, "✗1"},
		{"pending con recuento", model.Checks{State: model.ChecksPending, Pending: 2}, "…2"},
		{"passing sin recuento", model.Checks{State: model.ChecksPassing, Total: 12}, "✓"},
		{"desconocido", model.Checks{State: model.ChecksUnknown}, "-"},
		{"estado vacio", model.Checks{}, "-"},
		{
			nombre:  "cifras sin estado",
			checks:  model.Checks{Failing: 3, Total: 9},
			wantTxt: "-",
		},
	}
	render := func(cs model.Checks) string { return styleChecks(cs).Render("x") }
	for _, c := range casos {
		if got := checksText(c.checks); got != c.wantTxt {
			t.Errorf("%s: checksText dio %q, want %q", c.nombre, got, c.wantTxt)
		}
		if got := checksText(c.checks); strings.TrimSpace(got) == "" {
			t.Errorf("%s: checksText devolvió vacío", c.nombre)
		}
		// Compared as rendered rather than as a style, because lipgloss.Style carries the colour.
		if render(c.checks) != render(model.Checks{State: c.checks.State}) {
			t.Errorf("%s: el estilo no depende solo del estado", c.nombre)
		}
	}

	vistos := map[string]model.CheckState{}
	for _, estado := range []model.CheckState{
		model.ChecksFailing, model.ChecksPending, model.ChecksPassing, model.ChecksUnknown,
	} {
		pintado := render(model.Checks{State: estado})
		if otro, ya := vistos[pintado]; ya {
			t.Errorf("los estados %q y %q pintan igual (%q)", estado, otro, pintado)
		}
		vistos[pintado] = estado
	}
}

// The reason matters as much as the veto: an empty one produces a toast with the prefix and nothing.
func TestElEstadoAccionableDaMotivosQueDicenQueHacer(t *testing.T) {
	casos := []struct {
		nombre string
		estado string
		veto   bool
	}{
		{"mergeado", "MERGED", true},
		{"cerrado", "CLOSED", true},
		{"abierto", "OPEN", false},
		{"en blanco", "", false},
		{"mergeado en minusculas", "merged", true},
		{"cerrado en minusculas", "closed", true},
	}
	for _, c := range casos {
		ok, motivo := state.Actionable(model.Item{Number: 1, State: c.estado})
		if ok == c.veto {
			t.Errorf("%s: Actionable dio ok=%v, want %v", c.nombre, ok, !c.veto)
		}
		if !ok && strings.TrimSpace(motivo) == "" {
			t.Errorf("%s: vetó sin dar motivo", c.nombre)
		}
		if ok && motivo != "" {
			t.Errorf("%s: no vetó pero dio motivo %q", c.nombre, motivo)
		}
	}
}

// Number and unit are NOT separated by a space: "30s ago" is one field with the count glued to the
// letter.
func numeroDe(texto string) (int, int) {
	campos := strings.Fields(texto)
	if len(campos) == 0 {
		return 0, 0
	}
	n, unid, _ := partesDe(campos[0])
	return n, unid
}

func duracionDe(texto string) (time.Duration, error) {
	campos := strings.Fields(texto)
	if len(campos) == 0 {
		return 0, errors.New("texto vacío")
	}
	n, unid, habia := partesDe(campos[0])
	if !habia {
		if campos[0] == "now" || campos[0] == "no" {
			return 0, nil
		}
		return 0, errors.New("sin número")
	}
	switch unid {
	case 1:
		return time.Duration(n) * time.Second, nil
	case 2:
		return time.Duration(n) * time.Minute, nil
	case 3:
		return time.Duration(n) * time.Hour, nil
	}
	return 0, errors.New("unidad desconocida")
}

func duracionDeUnidad(unidad int) time.Duration {
	switch unidad {
	case 1:
		return time.Second
	case 2:
		return time.Minute
	case 3:
		return time.Hour
	}
	return 0
}

// The third value tells "0" from "no number": "0s ago" is data and "now" has no count. Without the
// distinction a "now" would read as zero seconds.
func partesDe(campo string) (n, unidad int, habiaNumero bool) {
	i := 0
	for i < len(campo) && campo[i] >= '0' && campo[i] <= '9' {
		n = n*10 + int(campo[i]-'0')
		i++
	}
	if i == 0 {
		return 0, 0, false
	}
	switch {
	case i < len(campo) && campo[i] == 's':
		unidad = 1
	case i < len(campo) && campo[i] == 'm':
		unidad = 2
	case i < len(campo) && campo[i] == 'h':
		unidad = 3
	}
	return n, unidad, true
}
