// Package bitbucket implementa el adapter de Bitbucket: existe, compila y
// pasa la suite de conformidad junto a los forges reales, pero no realiza
// ninguna llamada de red. Todos sus métodos responden "no soportado".
package bitbucket

import (
	"context"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
)

// ForgeName es el identificador del forge.
const ForgeName = "bitbucket"

// Adapter implementa forge.Adapter sin tocar la red.
type Adapter struct {
	host string
}

// Aseguramos en compilación que el adapter cumple el contrato.
var _ forge.Adapter = (*Adapter)(nil)

// New construye el adapter inerte para un host.
func New(host string) *Adapter {
	if host == "" {
		host = "bitbucket.org"
	}
	return &Adapter{host: host}
}

// Forge devuelve el nombre del forge.
func (a *Adapter) Forge() string { return ForgeName }

// Host devuelve el host configurado.
func (a *Adapter) Host() string { return a.host }

// Auth informa que el forge no está operativo en esta versión.
func (a *Adapter) Auth(context.Context) model.AuthState {
	return model.AuthState{Forge: ForgeName, OK: false, Reason: "not supported in this version"}
}

// List responde "no soportado" sin consultar nada.
func (a *Adapter) List(_ context.Context, q forge.Query) (forge.Page, []model.Warning) {
	return forge.Page{}, a.unsupported(q.Section)
}

// ItemState responde "no soportado".
func (a *Adapter) ItemState(context.Context, model.RepoRef, int) (model.Item, []model.Warning) {
	return model.Item{}, a.unsupported("")
}

// Approve responde "no soportado".
func (a *Adapter) Approve(context.Context, model.RepoRef, int) []model.Warning {
	return a.unsupported("")
}

// Merge responde "no soportado".
func (a *Adapter) Merge(context.Context, model.RepoRef, int, forge.MergeMode) []model.Warning {
	return a.unsupported("")
}

func (a *Adapter) unsupported(section model.Section) []model.Warning {
	return []model.Warning{{
		Forge:   ForgeName,
		Section: section,
		Kind:    "unsupported",
		Msg:     "Bitbucket is not supported in this version",
	}}
}
