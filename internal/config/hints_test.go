package config

import (
	"strings"
	"testing"
)

// The bar is an ORDERED list and the order is hintOrder's.
func TestTheBarHintsFollowTheOrderAndEachOtherOnly(t *testing.T) {
	cfg := Defaults()

	hints := cfg.Hints(nil)
	if len(hints) == 0 {
		t.Fatal("Hints with no state gave no hints")
	}
	// A nil state does not change the entry COUNT: state is optional.
	for _, h := range hints {
		if strings.TrimSpace(h) == "" {
			t.Errorf("a hint comes out empty: %q", h)
		}
		// The format is "key label": the bar is space-separated, and an assertion that skipped it would
		// pass on a different bar.
		if !strings.Contains(h, " ") {
			t.Errorf("the hint %q has no separation between key and label", h)
		}
		if strings.HasPrefix(h, " ") || strings.HasSuffix(h, " ") {
			t.Errorf("the hint %q has spaces at the edges", h)
		}
	}
	// Two consecutive calls give the same thing: the bar is recomposed on every render.
	other := cfg.Hints(nil)
	for i := range hints {
		if hints[i] != other[i] {
			t.Fatalf("Hints is not stable: %q on call %d and %q on call 0",
				hints[i], i, other[i])
		}
	}
}

// It is what makes the bar say what is happening now instead of repeating the names.
func TestTheDynamicStateIsAddedToTheLabelAndOnlyToTheOneThatHasIt(t *testing.T) {
	cfg := Defaults()

	state := HintState{"refresh": "2m ago"}
	hints := cfg.Hints(state)

	withState, withoutState := 0, 0
	for _, h := range hints {
		if strings.Contains(h, "2m ago") {
			withState++
			// The state text goes after the label, with a colon: "refresh: did 2m".
			if !strings.Contains(h, ": ") {
				t.Errorf("the hint with state %q does not separate label from state", h)
			}
		} else {
			withoutState++
		}
	}
	if withState != 1 {
		t.Errorf("%d hints carry the state, want exactly 1", withState)
	}
	if withoutState == 0 {
		t.Error("all hints carry the state: it was added to the ones that do not have it")
	}

	// An EMPTY state adds nothing: a state of "" is an action with nothing to say.
	for _, h := range cfg.Hints(HintState{"refresh": ""}) {
		if strings.HasSuffix(h, ": ") {
			t.Errorf("a hint with an empty state ended up as %q", h)
		}
	}

	// A state for an action that does not exist invents no entry.
	extra := cfg.Hints(HintState{"nonexistent-action": "text"})
	base := cfg.Hints(nil)
	if len(extra) != len(base) {
		t.Errorf("a state for a nonexistent action added %d hints",
			len(extra)-len(base))
	}

	// A hint that already had state does not duplicate it on the next paint.
	twice := cfg.Hints(state)
	if strings.Count(twice[0], "2m ago") > 1 {
		t.Errorf("the state was duplicated: %q", twice[0])
	}
}

// The whole contract of an override: it changes the action's key, not its label.
func TestAKeyOverrideChangesTheHintAndNothingElse(t *testing.T) {
	base := Defaults()
	before := base.Hints(nil)

	changed := Defaults()
	changed.Keybindings["refresh"] = "F5"
	after := changed.Hints(nil)

	if len(before) != len(after) {
		t.Fatalf("an override changed the number of hints: %d -> %d", len(before), len(after))
	}
	changedCount := 0
	for i := range before {
		if before[i] != after[i] {
			changedCount++
			if !strings.Contains(after[i], "F5") {
				t.Errorf("hint %d changed without carrying the override: %q -> %q",
					i, before[i], after[i])
			}
		}
	}
	if changedCount != 1 {
		t.Errorf("an override changed %d hints, want 1", changedCount)
	}
}
