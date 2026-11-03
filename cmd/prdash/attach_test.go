package main

import (
	"slices"
	"testing"

	"prdash/internal/config"
)

func TestAttachArgvUsesTheSharedTable(t *testing.T) {
	argv, err := attachArgv(config.AIConfig{Harness: "pi", Model: "free", Effort: "high"}, "fix it")
	if err != nil {
		t.Fatalf("attachArgv: %v", err)
	}
	want := []string{"pi", "--model", "free", "--thinking", "high", "fix it"}
	if !slices.Equal(argv, want) {
		t.Errorf("argv = %v, want %v", argv, want)
	}
}

func TestAttachArgvFallsBackToHeadlessWithoutInteractivePrompt(t *testing.T) {
	argv, err := attachArgv(config.AIConfig{Harness: "jcode", Model: "m"}, "fix it")
	if err != nil {
		t.Fatalf("attachArgv: %v", err)
	}
	want := []string{"jcode", "run", "-m", "m", "fix it"}
	if !slices.Equal(argv, want) {
		t.Errorf("argv = %v, want %v", argv, want)
	}

	bare, err := attachArgv(config.AIConfig{Harness: "jcode"}, "")
	if err != nil {
		t.Fatalf("attachArgv without a prompt: %v", err)
	}
	if !slices.Equal(bare, []string{"jcode"}) {
		t.Errorf("argv = %v, want the bare interactive session", bare)
	}
}

func TestAttachArgvNeedsAHarness(t *testing.T) {
	if _, err := attachArgv(config.AIConfig{}, "p"); err == nil {
		t.Fatal("want an error without ai.harness")
	}
}
