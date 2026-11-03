package main

import (
	"os"
	"path/filepath"
	"testing"

	"prdash/internal/config"
	"prdash/internal/forge/model"
)

func TestSimulationLocatorRefusesAForgeWithoutAKnownReviewRef(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	it := model.NewItem(model.RepoRef{Forge: "bitbucket", Host: "example.com", Project: "acme/widget"}, 7)

	repo := filepath.Join(t.TempDir(), "widget")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	ex := buildExecutor(config.Defaults())
	ex.Resolver.Remember(it.Ref, repo)

	if _, ok := (simLocator{ex: ex}).Locate(it); ok {
		t.Error("the locator offered a simulation for a forge with no review ref")
	}
}

func TestSimulationLocatorFallsBackToTheLocalCloneWithoutAMountedReview(t *testing.T) {
	cases := map[string]string{
		"github": "refs/pull/7/head",
		"gitlab": "refs/merge-requests/7/head",
	}
	for forgeName, wantSource := range cases {
		t.Run(forgeName, func(t *testing.T) {
			t.Setenv("XDG_CACHE_HOME", t.TempDir())
			it := model.NewItem(model.RepoRef{
				Forge: forgeName, Host: "example.com", Project: "acme/widget",
				Owner: "acme", Name: "widget",
			}, 7)

			repo := filepath.Join(t.TempDir(), "widget")
			if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
				t.Fatal(err)
			}

			ex := buildExecutor(config.Defaults())
			ex.Resolver.Remember(it.Ref, repo)

			place, ok := simLocator{ex: ex}.Locate(it)
			if !ok {
				t.Fatal("the locator did not find the local clone")
			}
			if place.Repo != repo {
				t.Errorf("Repo = %q, want the local clone", place.Repo)
			}
			if place.Branch != "" {
				t.Errorf("Branch = %q, want none: no review is mounted", place.Branch)
			}
			if place.Source != wantSource {
				t.Errorf("Source = %q, want %q", place.Source, wantSource)
			}
		})
	}
}
