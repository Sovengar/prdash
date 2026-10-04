package model

import (
	"testing"
)

// The model's untested functions share one property: they are PRESENTATION and DEGRADATION.

// The three cases validated together.
func TestLaSeccionTieneDosNombresYSoloUnoEsDeAPI(t *testing.T) {
	casos := []struct {
		seccion   Section
		wantStr   string
		wantLendg string
	}{
		{SectionAuthored, "Created by me", "Mine"},
		{SectionReview, "Review / assigned", "Assigned"},
		{SectionMentions, "Mentions", "Mentioned"},
		{Section("otra"), "otra", "otra"},
		{Section(""), "", ""},
	}
	for _, c := range casos {
		if got := c.seccion.String(); got != c.wantStr {
			t.Errorf("Section(%q).String() dio %q, want %q", c.seccion, got, c.wantStr)
		}
		if got := c.seccion.Legend(); got != c.wantLendg {
			t.Errorf("Section(%q).Legend() dio %q, want %q", c.seccion, got, c.wantLendg)
		}
	}
}

// The test that saves the wrong refactorisation.
func TestLosDosNombresNoSeConfunden(t *testing.T) {
	// What is pinned is that they NEVER coincide: if they did, someone could delete Legend and
	//delegate to String.
	for _, s := range []Section{SectionAuthored, SectionReview, SectionMentions} {
		if s.Legend() == s.String() {
			t.Errorf("%q: la leyenda y el titulo son el mismo texto (%q). La leyenda va "+
				"en el borde del inbox y no cabe con los nombres largos", s, s.String())
		}
	}
	for _, s := range []Section{SectionAuthored, SectionReview, SectionMentions} {
		if len(s.Legend()) > 9 {
			t.Errorf("%q: la leyenda %q son %d caracteres y el borde no tiene ese ancho",
				s, s.Legend(), len(s.Legend()))
		}
	}
}

func TestElTotalDeLineasSumaLasDosCaras(t *testing.T) {
	casos := []struct {
		d    DiffStat
		want int
	}{
		{DiffStat{Additions: 3, Deletions: 4}, 7},
		{DiffStat{Additions: 100, Deletions: 0}, 100},
		{DiffStat{Additions: 0, Deletions: 100}, 100},
		{DiffStat{}, 0},
		// Diffstat desconocido: cero, no menos.
		{DiffStat{Known: false, Additions: 5, Deletions: 5}, 10},
	}
	for _, c := range casos {
		if got := c.d.Total(); got != c.want {
			t.Errorf("DiffStat%+v.Total() dio %d, want %d", c.d, got, c.want)
		}
	}
}

// MergeRulesAll returns the three strategies with Known at TRUE, which is wrong on purpose and is
// what tells a permissive default from real rules.
func TestLasReglasDeMergePermisivasLoDicenConSuBit(t *testing.T) {
	r := MergeRulesAll()
	if !r.Known {
		t.Error("MergeRulesAll dio Known=false: entonces no se distingue de unas reglas " +
			"que nadie ha leido del forge")
	}
	if !r.MergeCommit || !r.Rebase || !r.Squash {
		t.Errorf("MergeRulesAll dio %+v: el default tiene que admitir las tres", r)
	}
	if (MergeRules{}).Known {
		t.Error("las reglas vacias dio Known=true: Known debe decir si se leyeron, no " +
			"si son restrictivas")
	}
}
