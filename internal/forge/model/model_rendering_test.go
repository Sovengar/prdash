package model

import (
	"testing"
)

func TestTheSectionHasTwoNamesAndOnlyOneIsForTheAPI(t *testing.T) {
	cases := []struct {
		section   Section
		wantStr   string
		wantLendg string
	}{
		{SectionAuthored, "Created by me", "Mine"},
		{SectionReview, "Review / assigned", "Assigned"},
		{SectionMentions, "Mentions", "Mentioned"},
		{Section("other"), "other", "other"},
		{Section(""), "", ""},
	}
	for _, c := range cases {
		if got := c.section.String(); got != c.wantStr {
			t.Errorf("Section(%q).String() gave %q, want %q", c.section, got, c.wantStr)
		}
		if got := c.section.Legend(); got != c.wantLendg {
			t.Errorf("Section(%q).Legend() gave %q, want %q", c.section, got, c.wantLendg)
		}
	}
}

// The test that saves the wrong refactorisation.
func TestTheTwoNamesAreNotConfused(t *testing.T) {
	// They must NEVER coincide: if they did, someone could delete Legend and delegate to String.
	for _, s := range []Section{SectionAuthored, SectionReview, SectionMentions} {
		if s.Legend() == s.String() {
			t.Errorf("%q: the legend and the title are the same text (%q). The legend goes "+
				"on the inbox border and does not fit with long names", s, s.String())
		}
	}
	for _, s := range []Section{SectionAuthored, SectionReview, SectionMentions} {
		if len(s.Legend()) > 9 {
			t.Errorf("%q: the legend %q is %d characters and the border is not that wide",
				s, s.Legend(), len(s.Legend()))
		}
	}
}

func TestTheLineTotalAddsBothFaces(t *testing.T) {
	cases := []struct {
		d    DiffStat
		want int
	}{
		{DiffStat{Additions: 3, Deletions: 4}, 7},
		{DiffStat{Additions: 100, Deletions: 0}, 100},
		{DiffStat{Additions: 0, Deletions: 100}, 100},
		{DiffStat{}, 0},
		// Unknown diffstat: zero, not less.
		{DiffStat{Known: false, Additions: 5, Deletions: 5}, 10},
	}
	for _, c := range cases {
		if got := c.d.Total(); got != c.want {
			t.Errorf("DiffStat%+v.Total() gave %d, want %d", c.d, got, c.want)
		}
	}
}

// MergeRulesAll returns the three strategies with Known true on purpose, which tells a permissive default from real rules.
func TestThePermissiveMergeRulesSaySoWithTheirBit(t *testing.T) {
	r := MergeRulesAll()
	if !r.Known {
		t.Error("MergeRulesAll gave Known=false: then it cannot be told apart from rules " +
			"nobody read from the forge")
	}
	if !r.MergeCommit || !r.Rebase || !r.Squash {
		t.Errorf("MergeRulesAll gave %+v: the default has to allow all three", r)
	}
	if (MergeRules{}).Known {
		t.Error("the empty rules gave Known=true: Known has to say whether they were read, " +
			"not whether they are restrictive")
	}
}
