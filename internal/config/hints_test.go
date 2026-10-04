package config

import (
	"strings"
	"testing"
)

// The bar is an ORDERED list and the order is hintOrder's.
func TestLasPistasDeLaBarraSiguenElOrdenYLoUnico(t *testing.T) {
	cfg := Defaults()

	hints := cfg.Hints(nil)
	if len(hints) == 0 {
		t.Fatal("Hints sin estado no dio ninguna pista")
	}
	// A nil state does not change the entry COUNT: state is optional.
	for _, h := range hints {
		if strings.TrimSpace(h) == "" {
			t.Errorf("una pista sale vacía: %q", h)
		}
		// The format is "key label": the bar is space-separated, and an assertion that skipped it would
		// pass on a different bar.
		if !strings.Contains(h, " ") {
			t.Errorf("la pista %q no tiene separación entre tecla y etiqueta", h)
		}
		if strings.HasPrefix(h, " ") || strings.HasSuffix(h, " ") {
			t.Errorf("la pista %q tiene espacios en los bordes", h)
		}
	}
	// Two consecutive calls give the same thing: the bar is recomposed on every render.
	otro := cfg.Hints(nil)
	for i := range hints {
		if hints[i] != otro[i] {
			t.Fatalf("Hints no es estable: %q en la llamada %d y %q en la %d",
				hints[i], i, otro[i], 0)
		}
	}
}

// It is what makes the bar say what is happening now instead of repeating the names.
func TestElEstadoDinamicoSeAnadeALaEtiquetaYSoloALQueLoTiene(t *testing.T) {
	cfg := Defaults()

	estado := HintState{"refresh": "hace 2m"}
	hints := cfg.Hints(estado)

	conEstado, sinEstado := 0, 0
	for _, h := range hints {
		if strings.Contains(h, "hace 2m") {
			conEstado++
			// The state text goes after the label, with a colon: "refresh: did 2m".
			if !strings.Contains(h, ": ") {
				t.Errorf("la pista con estado %q no separa la etiqueta del estado", h)
			}
		} else {
			sinEstado++
		}
	}
	if conEstado != 1 {
		t.Errorf("%d pistas llevan el estado, want exactamente 1", conEstado)
	}
	if sinEstado == 0 {
		t.Error("todas las pistas llevan el estado: se añadió a las que no lo tienen")
	}

	// An EMPTY state adds nothing: a state of "" is an action with nothing to say.
	for _, h := range cfg.Hints(HintState{"refresh": ""}) {
		if strings.HasSuffix(h, ": ") {
			t.Errorf("una pista con estado vacío quedó en %q", h)
		}
	}

	// A state for an action that does not exist invents no entry.
	conExtra := cfg.Hints(HintState{"accion-que-no-existe": "texto"})
	base := cfg.Hints(nil)
	if len(conExtra) != len(base) {
		t.Errorf("un estado para una acción inexistente añadió %d pistas",
			len(conExtra)-len(base))
	}

	// A hint that already had state does not duplicate it on the next paint.
	dosVeces := cfg.Hints(estado)
	if strings.Count(dosVeces[0], "hace 2m") > 1 {
		t.Errorf("el estado se duplicó: %q", dosVeces[0])
	}
}

// The whole contract of an override: it changes the action's key, not its label.
func TestUnOverrideDeTeclaCambiaLaPistaYNoElResto(t *testing.T) {
	base := Defaults()
	antes := base.Hints(nil)

	cambiada := Defaults()
	cambiada.Keybindings["refresh"] = "F5"
	despues := cambiada.Hints(nil)

	if len(antes) != len(despues) {
		t.Fatalf("un override cambió el número de pistas: %d -> %d", len(antes), len(despues))
	}
	cambiadas := 0
	for i := range antes {
		if antes[i] != despues[i] {
			cambiadas++
			if !strings.Contains(despues[i], "F5") {
				t.Errorf("la pista %d cambió sin llevar el override: %q -> %q",
					i, antes[i], despues[i])
			}
		}
	}
	if cambiadas != 1 {
		t.Errorf("un override cambió %d pistas, want 1", cambiadas)
	}
}
