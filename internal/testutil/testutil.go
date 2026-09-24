// Package testutil ofrece dobles en memoria de los contratos de prdash para
// tests: sin red, sin subproceso y sin disco.
package testutil

import (
	"context"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
)

// FakeAdapter es una implementación en memoria de forge.Adapter. Cada método
// devuelve lo que el test configure, de modo que las pruebas de inbox y TUI no
// tocan ninguna CLI.
type FakeAdapter struct {
	ForgeName string
	HostName  string

	AuthState model.AuthState

	AuthoredItems []model.Item
	ReviewItems   []model.Item
	MentionItems  []model.Item

	AuthoredWarnings []model.Warning
	ReviewWarnings   []model.Warning
	MentionWarnings  []model.Warning
}

// Cumple el contrato en compilación.
var _ forge.Adapter = (*FakeAdapter)(nil)

// Forge devuelve el nombre configurado.
func (f *FakeAdapter) Forge() string { return f.ForgeName }

// Host devuelve el host configurado.
func (f *FakeAdapter) Host() string { return f.HostName }

// Auth devuelve el estado configurado; por defecto, autenticado.
func (f *FakeAdapter) Auth(_ context.Context) model.AuthState {
	if f.AuthState.Forge == "" && !f.AuthState.OK && f.AuthState.Reason == "" {
		return model.AuthState{Forge: f.ForgeName, OK: true}
	}
	return f.AuthState
}

// Authored devuelve los ítems y warnings configurados.
func (f *FakeAdapter) Authored(_ context.Context) ([]model.Item, []model.Warning) {
	return f.AuthoredItems, f.AuthoredWarnings
}

// ReviewRequested devuelve los ítems y warnings configurados.
func (f *FakeAdapter) ReviewRequested(_ context.Context) ([]model.Item, []model.Warning) {
	return f.ReviewItems, f.ReviewWarnings
}

// Mentions devuelve los ítems y warnings configurados.
func (f *FakeAdapter) Mentions(_ context.Context) ([]model.Item, []model.Warning) {
	return f.MentionItems, f.MentionWarnings
}

// ItemState no está implementado en esta etapa.
func (f *FakeAdapter) ItemState(_ context.Context, _ model.RepoRef, _ int) (model.Item, []model.Warning) {
	return model.Item{}, nil
}

// Approve no está implementado en esta etapa.
func (f *FakeAdapter) Approve(_ context.Context, _ model.RepoRef, _ int) []model.Warning {
	return nil
}

// Merge no está implementado en esta etapa.
func (f *FakeAdapter) Merge(_ context.Context, _ model.RepoRef, _ int) []model.Warning {
	return nil
}
