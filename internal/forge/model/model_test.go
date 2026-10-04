package model

import "testing"

func TestWithIdentity(t *testing.T) {
	id := With("github", "github.com", "acme/widget", 42)
	if id.Forge != "github" || id.Host != "github.com" || id.Project != "acme/widget" || id.Number != 42 {
		t.Fatalf("id = %+v", id)
	}
}

func TestItemIDUsesNormalizedFields(t *testing.T) {
	it := NewItem(RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget"}, 42)
	it.Forge = "github"
	it.Host = "github.com"

	if got := it.ID(); got != (ID{Forge: "github", Host: "github.com", Project: "acme/widget", Number: 42}) {
		t.Fatalf("ID = %+v", got)
	}
}

func TestNewItemSyncsForgeAndHost(t *testing.T) {
	ref := RepoRef{Forge: "gitlab", Host: "gitlab.example.com", Project: "grp/proj"}
	it := NewItem(ref, 7)
	if it.Forge != "gitlab" || it.Host != "gitlab.example.com" {
		t.Fatalf("forge/host no sincronizados: %+v", it)
	}
	if it.Ref != ref || it.Number != 7 {
		t.Fatalf("item = %+v", it)
	}
}

// Legend() is the short one; String() must not change.
func TestSectionLegendKeepsStringIntact(t *testing.T) {
	for _, tc := range []struct {
		s      Section
		legend string
		str    string
	}{
		{SectionAuthored, "Mine", "Created by me"},
		{SectionReview, "Assigned", "Review / assigned"},
		{SectionMentions, "Mentioned", "Mentions"},
	} {
		if got := tc.s.Legend(); got != tc.legend {
			t.Errorf("%q: Legend() = %q, want %q", tc.s, got, tc.legend)
		}
		if got := tc.s.String(); got != tc.str {
			t.Errorf("%q: String() = %q, want %q (no debe cambiar)", tc.s, got, tc.str)
		}
	}
}
