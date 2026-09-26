// Tests del ancho de la Confirmación de merge con el toggle de borrado.
//
// La Confirmación es la caja que sustituye a la barra de atajos y va envuelta al
// ancho, así que el texto nuevo se parte donde caiga. Lo que hay que fijar aquí
// es que la parte que NO se puede perder —el valor del borrado y su tecla— siga
// siendo legible en un terminal estrecho, porque en un terminal estrecho es
// donde se cometen los errores.
package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestMergeConfirmKeepsTheDeleteValueWhenItWraps: el valor del borrado y la
// tecla que lo cambia tienen que aparecer enteros en la caja sea cual sea el
// ancho. Que la palabra "delete branch" se parta por la mitad da igual; que no
// aparezca el `yes (tab)` sí.
func TestMergeConfirmKeepsTheDeleteValueWhenItWraps(t *testing.T) {
	for _, width := range []int{40, 50, 60, 76, 100, 160} {
		f := newMergeFixture(t, mergeItems()...)
		m := send(t, f.m, tea.WindowSizeMsg{Width: width, Height: 40})
		m = press(t, m, "m")

		view := stripANSI(m.View().Content)
		if !strings.Contains(view, "yes (tab)") {
			t.Errorf("ancho %d: el valor del borrado no se lee entero\n%s", width, view)
		}
	}
}

// TestMergeConfirmKeepsBothDeleteValuesVisible: el conmutado tiene que enseñar
// el valor nuevo en la MISMA caja, sin tener que deducirlo de cuál era antes.
// Un toggle que solo se insinúa al cambiar obliga a recordar el valor
// anterior, que es justo lo que un toggle debe evitar.
func TestMergeConfirmKeepsBothDeleteValuesVisible(t *testing.T) {
	for _, width := range []int{50, 76, 120} {
		f := newMergeFixture(t, mergeItems()...)
		m := send(t, f.m, tea.WindowSizeMsg{Width: width, Height: 40})
		m = press(t, m, "m")
		m = press(t, m, "tab")

		view := stripANSI(m.View().Content)
		if !strings.Contains(view, "no (tab)") {
			t.Errorf("ancho %d: el valor conmutado no aparece\n%s", width, view)
		}
		if strings.Contains(view, "yes (tab)") {
			t.Errorf("ancho %d: sigue enseñando el valor anterior\n%s", width, view)
		}
	}
}
