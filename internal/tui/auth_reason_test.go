package tui

import (
	"strings"
	"testing"

	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

// TestAuthReasonReachesTheScreen: el motivo que da el adapter tiene que llegar a
// algún sitio. El caso que lo justifica es un forge que no está implementado
// (bitbucket): con la etiqueta genérica, quien lo habilita en la config ve "not
// authenticated" y culpa a su token, que es perfectamente válido.
func TestAuthReasonReachesTheScreen(t *testing.T) {
	m := newTestModel(t, &testutil.FakeAdapter{
		ForgeName: "bitbucket", HostName: "bitbucket.org",
		AuthState: model.AuthState{Forge: "bitbucket", OK: false, Reason: "not implemented in this version"},
	})
	m = send(t, m, authMsg{cycle: 1, forge: "bitbucket",
		auth: model.AuthState{Forge: "bitbucket", OK: false, Reason: "not implemented in this version"}})
	m = send(t, m, page(1, "bitbucket", "bitbucket.org", model.SectionAuthored, "",
		[]model.Item{mkItem("bitbucket", "bitbucket.org", "team/repo", "MR", 4, "")}, false))

	m = press(t, m, "a")
	toast := lastToast(m)
	if !strings.Contains(toast, "not implemented") {
		t.Errorf("toast = %q, want el motivo real del adapter", toast)
	}
	if strings.Contains(toast, "not authenticated") {
		t.Errorf("toast = %q: la etiqueta genérica manda a la persona a la autenticación", toast)
	}
}

// TestAuthReasonFallsBackWhenEmpty: un adapter que no da motivo no puede dejar el
// aviso en blanco. El texto por defecto es el que había antes, así que ningún
// caso sale peor que antes del arreglo.
func TestAuthReasonFallsBackWhenEmpty(t *testing.T) {
	if got := authReason(model.AuthState{Forge: "github"}); got != "not authenticated" {
		t.Errorf("authReason = %q, want not authenticated", got)
	}
}
