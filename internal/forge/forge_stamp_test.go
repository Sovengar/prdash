package forge

import (
	"testing"

	"prdash/internal/forge/model"
)

func TestStampingDoesNotOverwriteTheSectionThatAlreadyHasOne(t *testing.T) {
	const (
		review    = model.SectionReview
		mentions  = model.SectionMentions
		authored  = model.SectionAuthored
		noSection = model.Section("")
	)

	cases := []struct {
		name  string
		in    model.Section
		stamp model.Section
		want  model.Section
	}{
		{"no section, it gets stamped", noSection, review, review},
		{"with its own section, untouched", review, mentions, review},
		{"with a mentions section, untouched", mentions, review, mentions},
		{"with an authorship section, untouched", authored, review, authored},
	}
	for _, c := range cases {
		warns := StampSection([]model.Warning{{Msg: "something", Section: c.in}}, c.stamp)
		if len(warns) != 1 {
			t.Fatalf("case %q: %d warnings, want 1", c.name, len(warns))
		}
		if warns[0].Section != c.want {
			t.Errorf("case %q: the section ended up %q, want %q", c.name, warns[0].Section, c.want)
		}
	}

	warns := StampSection([]model.Warning{
		{Msg: "the first one already knows where it came from", Section: mentions},
		{Msg: "the second one does not"},
	}, review)
	if len(warns) != 2 {
		t.Fatalf("%d warnings left, want 2", len(warns))
	}
	if warns[0].Section != mentions {
		t.Errorf("the warning that already carried a section ended up %q: stamping everything "+
			"the same makes a comments warning end up with the listing section stuck on it",
			warns[0].Section)
	}
	if warns[1].Section != review {
		t.Errorf("the warning without a section ended up %q, want %q: this one does get stamped",
			warns[1].Section, review)
	}

	if got := StampSection(nil, review); len(got) != 0 {
		t.Errorf("an empty list returned %d warnings", len(got))
	}
}
