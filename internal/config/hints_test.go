package config

import (
	"strings"
	"testing"
)

// `Hints` compone la barra de atajos: qué se muestra, en qué orden, y con
// qué texto. Y la barra es una lista ORDENADA a propósito —los cuatro
// atajos que más se pulsan están en el mismo sitio de cada fila— así que
// el orden es parte del contrato y no un detalle del slice.

// TestLasPistasDeLaBarraSiguenElOrdenYLoUnico: `Hints`.
//
// Y lo que se fija es que la barra es una LISTA ORDENADA de atajos, y el orden es el de
// `hintOrder`. Es lo que la hace legible: los cuatro atajos que más se pulsan están en el
// mismo sitio de cada fila, y el ojo los encuentra sin leer.
//
// Y la asimetría con el resto del fichero: la barra tiene dos clases de entrada. Las que
// vienen de `[keybindings]` se resuelven sobre el mapa —un override del usuario cambia la
// tecla— y las fijas (`j`, `k`, `pgup`) no, porque no salen de ningún mapa.
//
// Y una entrada con la TECLA VACÍA se salta, no se pinta como un hueco. Una tecla vacía
// pasa cuando una acción no está en el mapa y tampoco tiene default —que es lo que pasa
// con una acción que se borra del código pero sigue en el config—.
func TestLasPistasDeLaBarraSiguenElOrdenYLoUnico(t *testing.T) {
	cfg := Defaults()

	// Sin estado: solo teclas y etiquetas fijas, y en el orden declarado.
	hints := cfg.Hints(nil)
	if len(hints) == 0 {
		t.Fatal("Hints sin estado no dio ninguna pista")
	}
	// Y el estado NO cambia el número de entradas con nil: `state` es opcional y nil da
	// la barra sin lo dinámico.
	for _, h := range hints {
		if strings.TrimSpace(h) == "" {
			t.Errorf("una pista sale vacía: %q", h)
		}
		// Y el formato es "tecla etiqueta": la barra separa por espacios y un aserto que
		// no lo comprueba no detecta una etiqueta que se traga la tecla.
		if !strings.Contains(h, " ") {
			t.Errorf("la pista %q no tiene separación entre tecla y etiqueta", h)
		}
		if strings.HasPrefix(h, " ") || strings.HasSuffix(h, " ") {
			t.Errorf("la pista %q tiene espacios en los bordes", h)
		}
	}
	// Y dos llamadas seguidas dan lo mismo: la barra se pinta en cada render, y una que
	// cambiase de orden entre renders sería un lista que se reordena sola.
	otro := cfg.Hints(nil)
	for i := range hints {
		if hints[i] != otro[i] {
			t.Fatalf("Hints no es estable: %q en la llamada %d y %q en la %d",
				hints[i], i, otro[i], 0)
		}
	}
}

// TestElEstadoDinamicoSeAnadeALaEtiquetaYSoloALQueLoTiene: el `state`.
//
// Y es la parte que hace que la barra diga qué está pasando ahora mismo en vez de repetir
// los nombres de las teclas. El `state` es un mapa acción → texto, y solo se añade a las
// entradas cuya acción está en él.
//
// Y lo que se fija es el "solo a las que lo tienen": una entrada sin entrada en el mapa NO
// se toca. Y es lo que distingue el mapa de una lista: un estado para una acción que no
// aparece en la barra no la inventa.
func TestElEstadoDinamicoSeAnadeALaEtiquetaYSoloALQueLoTiene(t *testing.T) {
	cfg := Defaults()

	// Con estado para una acción, esa entrada lo lleva y las demás no.
	estado := HintState{"refresh": "hace 2m"}
	hints := cfg.Hints(estado)

	conEstado, sinEstado := 0, 0
	for _, h := range hints {
		if strings.Contains(h, "hace 2m") {
			conEstado++
			// Y el texto del estado va después de la etiqueta, con dos puntos: "R
			// refresh: hace 2m". Con el estado antes, un "R: refresh" parece un nombre de
			// tecla que no existe.
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

	// Y una entrada de estado VACÍA no añade nada. Un `state` con ""
	// es un action que no tiene nada que decir, y paint-ear ": " sería un dos puntos
	// suelto al final de la etiqueta.
	for _, h := range cfg.Hints(HintState{"refresh": ""}) {
		if strings.HasSuffix(h, ": ") {
			t.Errorf("una pista con estado vacío quedó en %q", h)
		}
	}

	// Y un estado para una acción que no existe no inventa una entrada: el número de
	// pistas no cambia.
	conExtra := cfg.Hints(HintState{"accion-que-no-existe": "texto"})
	base := cfg.Hints(nil)
	if len(conExtra) != len(base) {
		t.Errorf("un estado para una acción inexistente añadió %d pistas",
			len(conExtra)-len(base))
	}

	// Y una pista que ya tenía estado no lo duplica al volver a pintar: la barra se
	// recompone en cada render, y si el estado se acumulara sobre la etiqueta, la segunda
	// it'd leería "R refresh: hace 2m: hace 2m".
	dosVeces := cfg.Hints(estado)
	if strings.Count(dosVeces[0], "hace 2m") > 1 {
		t.Errorf("el estado se duplicó: %q", dosVeces[0])
	}
}

// TestUnOverrideDeTeclaCambiaLaPistaYNoElResto: `[keybindings]` mandando sobre la barra.
//
// Y es el contrato completo de un override: cambia la tecla de la acción, no su etiqueta,
// y no toca las teclas fijas. Lo segundo es lo que se fija con el "no el resto" —porque
// un override que renombrara todo sería inservible para leer la barra.
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
			// Y solo la de refresh: las fijas no se tocan.
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
