package tui

import (
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

// The merge confirmation is the warning that has to list the modes.
func TestMergeConfirmNombraLosModosQueElRepoAdmite(t *testing.T) {
	base := func(t *testing.T, rules model.MergeRules) Model {
		m2 := newTestModel(t, ghAdapter())
		it := mkItem("github", "github.com", "acme/widget", "Uno", 1, "")
		it.Merge = rules
		m2 = send(t, m2, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{it}, false))
		return m2
	}

	m := base(t, model.MergeRulesAll())
	txt := stripANSI(m.mergeConfirmText())
	for _, want := range []string{"press the mode", "esc cancel"} {
		if !strings.Contains(txt, want) {
			t.Errorf("la caja debería decir %q: %q", want, txt)
		}
	}
	for _, want := range []string{"r rebase", "m merge commit", "s squash"} {
		if !strings.Contains(txt, want) {
			t.Errorf("con los tres modos debería salir %q: %q", want, txt)
		}
	}
	if n := strings.Count(txt, "·"); n != 3 {
		t.Errorf("con tres modos hay tres separadores, hay %d: %q", n, txt)
	}

	// One known mode: ONLY that one.
	m = base(t, model.MergeRules{Known: true, MergeCommit: true})
	txt = stripANSI(m.mergeConfirmText())
	if !strings.Contains(txt, "m merge commit") {
		t.Errorf("con un solo modo conocido debería salir ese: %q", txt)
	}
	for _, no := range []string{"rebase", "squash"} {
		if strings.Contains(txt, no) {
			t.Errorf("un repositorio que solo admite merge commit no debería ofrecer %q: %q", no, txt)
		}
	}

	// Unknown rules offer all three: not knowing is not the same as forbidding.
	m = base(t, model.MergeRules{})
	txt = stripANSI(m.mergeConfirmText())
	for _, want := range []string{"rebase", "merge commit", "squash"} {
		if !strings.Contains(txt, want) {
			t.Errorf("sin reglas conocidas deberían ofrecerse las tres: %q", txt)
		}
	}

	m = base(t, model.MergeRules{Known: true})
	txt = stripANSI(m.mergeConfirmText())
	if !strings.Contains(txt, "the repository allows no merge strategy") {
		t.Errorf("un repo sin estrategias debería decirlo: %q", txt)
	}
	m = base(t, model.MergeRulesAll())
	txt = stripANSI(m.mergeConfirmText())
	if !strings.Contains(txt, "merge acme") {
		t.Errorf("la caja debería nombrar el ítem: %q", txt)
	}
	if !strings.Contains(txt, "delete") {
		t.Errorf("la caja debería decir si se borra la rama: %q", txt)
	}
}

func TestMergeConfirmConBloqueoBlandoLoDiceAntes(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	it := mkItem("github", "github.com", "acme/widget", "Uno", 1, "")
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{it}, false))

	sinBloqueo := stripANSI(m.mergeConfirmText())
	if strings.Contains(sinBloqueo, "anyway") {
		t.Errorf("sin bloqueo no debería decir \"anyway\": %q", sinBloqueo)
	}

	m.mergeBlockReason = "CI is failing (2 of 5 checks)"
	conBloqueo := stripANSI(m.mergeConfirmText())
	if !strings.Contains(conBloqueo, "CI is failing (2 of 5 checks)") {
		t.Errorf("el aviso del bloqueo debería salir: %q", conBloqueo)
	}
	if !strings.Contains(conBloqueo, "anyway") {
		t.Errorf("con bloqueo blando debería decir que se puede pulsar igual: %q", conBloqueo)
	}
	if !strings.Contains(conBloqueo, "esc cancel") {
		t.Errorf("con bloqueo sigue habiendo salida: %q", conBloqueo)
	}
	if strings.Index(conBloqueo, "CI is failing") > strings.Index(conBloqueo, "press the mode") {
		t.Errorf("el aviso debería ir antes de las teclas: %q", conBloqueo)
	}
}

// The ITEM column's prefix is the last directory component's parent.
func TestSectionPrefixNoSeComeLaUltimaParteDeLaRuta(t *testing.T) {
	cases := []struct {
		name  string
		projs []string
		want  string
	}{
		{"mismo repo", []string{"acme/widget", "acme/widget"}, "acme"},
		{"grupo comun", []string{"grp/proj", "grp/otro"}, "grp"},
		{"grupo comun de tres", []string{"a/b", "a/c", "a/d"}, "a"},
		{"subgrupos distintos", []string{"grp/sub/proj", "grp/otro/proj"}, "grp"},
		{"grupos distintos", []string{"acme/widget", "otro/widget"}, ""},
		{"uno sin barra", []string{"widget", "acme/widget"}, ""},
		{"solo uno", []string{"acme/widget"}, ""},
		{"nada en comun con tres", []string{"a/x", "b/y", "c/z"}, ""},
		{"solo la hoja igual", []string{"x/widget", "y/widget"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			items := make([]model.Item, 0, len(c.projs))
			for i, proj := range c.projs {
				items = append(items, model.NewItem(model.RepoRef{Forge: "github", Host: "github.com", Project: proj}, i+1))
			}
			if got := sectionPrefix(items); got != c.want {
				t.Errorf("sectionPrefix(%v) = %q, want %q", c.projs, got, c.want)
			}
		})
	}
	if got := sectionPrefix(nil); got != "" {
		t.Errorf("sectionPrefix(nil) = %q, want \"\"", got)
	}
	if got := sectionPrefix([]model.Item{}); got != "" {
		t.Errorf("sectionPrefix([]) = %q, want \"\"", got)
	}
}

// The hints are the help and cannot take over the screen.
func TestWrapHintAcotaElNumeroDeLineas(t *testing.T) {
	plano := func(s string) string { return stripANSI(s) }

	got := wrapHint("uno", 80, plano)
	if len(got) != 1 || got[0] != "uno" {
		t.Errorf("un hint dio %q, want [\"uno\"]", got)
	}

	for _, w := range []int{3, 4, 10} {
		got = wrapHint("uno dos", w, plano)
		for _, l := range got {
			if len(l) > w {
				t.Errorf("ancho %d: la línea %q se pasa", w, l)
			}
		}
	}

	largo := strings.TrimSpace(strings.Repeat("palabra ", 200))
	got = wrapHint(largo, 400, plano)
	if len(got) > maxHintLines {
		t.Errorf("un hint enorme dio %d líneas, want <= %d", len(got), maxHintLines)
	}
	if len(got) > 0 && !strings.HasPrefix(largo, got[0]) {
		t.Errorf("el recorte debería tirar por el final, la primera línea es %q", got[0])
	}

	got = wrapHint("uno dos tres", 4, func(s string) string { return "[" + s + "]" })
	for _, l := range got {
		if !strings.HasPrefix(l, "[") || !strings.HasSuffix(l, "]") {
			t.Errorf("la línea no está vestida: %q", l)
		}
	}

	if got = wrapHint("", 20, plano); len(got) != 1 {
		t.Errorf("texto vacío dio %q, want una línea", got)
	}
}
