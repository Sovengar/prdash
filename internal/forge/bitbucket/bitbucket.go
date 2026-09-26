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
//
// El motivo dice "not implemented", no "not authenticated": la diferencia decide
// qué hace el operador. "No autenticado" se arregla retomando el token, y "no
// implementado" no se arregla de ninguna forma, así que el motivo equivocado
// manda a la persona a la autenticación a buscar un token que ya funciona.
func (a *Adapter) Auth(context.Context) model.AuthState {
	return model.AuthState{Forge: ForgeName, OK: false, Reason: "not implemented in this version"}
}

// List responde "no soportado" sin consultar nada.
func (a *Adapter) List(_ context.Context, q forge.Query) (forge.Page, []model.Warning) {
	return forge.Page{}, a.unsupported(q.Section)
}

// ItemState responde "no soportado".
func (a *Adapter) ItemState(context.Context, model.RepoRef, int) (model.Item, []model.Warning) {
	return model.Item{}, a.unsupported("")
}

// Comments responde "no soportado".
func (a *Adapter) Comments(context.Context, model.RepoRef, int) (forge.CommentPage, []model.Warning) {
	return forge.CommentPage{}, a.unsupported("")
}

// Approve responde "no soportado".
func (a *Adapter) Approve(context.Context, model.RepoRef, int) []model.Warning {
	return a.unsupported("")
}

// Merge responde "no soportado".
func (a *Adapter) Merge(context.Context, model.RepoRef, int, forge.MergeRequest) []model.Warning {
	return a.unsupported("")
}

// Retarget responde "no soportado".
func (a *Adapter) Retarget(context.Context, model.RepoRef, int, string) []model.Warning {
	return a.unsupported("")
}

// Branches responde "no soportado" con la lista vacía, que es lo que hace que el
// buscador se abra y explique que falta el forge en vez de abrirse sin nada.
func (a *Adapter) Branches(context.Context, model.RepoRef) ([]string, []model.Warning) {
	return nil, a.unsupported("")
}

func (a *Adapter) unsupported(section model.Section) []model.Warning {
	return []model.Warning{{
		Forge:   ForgeName,
		Section: section,
		Kind:    "unsupported",
		Msg:     "Bitbucket is not supported in this version",
	}}
}
