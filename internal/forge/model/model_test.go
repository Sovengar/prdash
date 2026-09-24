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
