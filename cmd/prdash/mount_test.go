package main

import (
	"testing"

	"prdash/internal/config"
)

func TestClonePrefixesOf(t *testing.T) {
	cfg := config.Config{
		Forges: config.Forges{
			GitHub: config.GitHubConfig{Host: "github.com"},
			GitLab: config.GitLabConfig{Host: "gitlab.example.com", APIBase: "/git/api/v4/"},
		},
	}
	got := clonePrefixesOf(cfg)
	if len(got) != 1 || got["gitlab.example.com"] != "git" {
		t.Fatalf("prefixes = %v", got)
	}
	if _, ok := got["github.com"]; ok {
		t.Fatalf("github no debería tener prefijo: %v", got)
	}
}

func TestClonePrefixesOfHonorsExplicitOverride(t *testing.T) {
	cfg := config.Config{
		Forges: config.Forges{
			GitHub: config.GitHubConfig{Host: "github.enterprise.com", CloneBase: "/ent/"},
			GitLab: config.GitLabConfig{Host: "gitlab.example.com", APIBase: "/api/v4/"},
		},
	}
	got := clonePrefixesOf(cfg)
	if got["github.enterprise.com"] != "ent" {
		t.Fatalf("github enterprise = %v", got)
	}
	if _, ok := got["gitlab.example.com"]; ok {
		t.Fatalf("gitlab raíz no debería tener prefijo: %v", got)
	}
}
