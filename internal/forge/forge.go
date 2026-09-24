// Package forge define el contrato que implementa cada forge y el registro de
// adapters habilitados.
//
// Los métodos de listado devuelven ítems más warnings tipados: nunca un error
// duro. Un forge caído o sin autenticar no puede vaciar el inbox.
package forge

import (
	"context"
	"sort"

	"prdash/internal/forge/model"
	"prdash/internal/inbox"
)

// Adapter es el contrato de un forge. Las implementaciones hablan con su CLI
// (`gh`, `glab`, …) por subproceso; los métodos de acción están definidos aquí
// para que el contrato sea completo aunque una etapa concreta no los use.
type Adapter interface {
	// Forge devuelve el nombre del forge ("github", "gitlab", …).
	Forge() string
	// Host devuelve el host configurado ("github.com", …).
	Host() string
	// Auth informa si el forge está autenticado y operativo.
	Auth(ctx context.Context) model.AuthState
	// Authored lista los PR/MR creados por el usuario.
	Authored(ctx context.Context) ([]model.Item, []model.Warning)
	// ReviewRequested lista los review pedidos y las asignaciones al usuario.
	ReviewRequested(ctx context.Context) ([]model.Item, []model.Warning)
	// Mentions lista los ítems donde el usuario aparece mencionado.
	Mentions(ctx context.Context) ([]model.Item, []model.Warning)
	// ItemState relee el estado de un ítem concreto.
	ItemState(ctx context.Context, ref model.RepoRef, number int) (model.Item, []model.Warning)
	// Approve aprueba un ítem.
	Approve(ctx context.Context, ref model.RepoRef, number int) []model.Warning
	// Merge mergea un ítem.
	Merge(ctx context.Context, ref model.RepoRef, number int) []model.Warning
}

// Registry mantiene los adapters habilitados por nombre de forge.
type Registry struct {
	adapters map[string]Adapter
}

// NewRegistry construye un registro vacío.
func NewRegistry() *Registry {
	return &Registry{adapters: map[string]Adapter{}}
}

// Register añade un adapter al registro, reemplazando el anterior del mismo
// forge si lo hubiera.
func (r *Registry) Register(a Adapter) {
	if a == nil {
		return
	}
	r.adapters[a.Forge()] = a
}

// Get devuelve el adapter de un forge.
func (r *Registry) Get(forge string) (Adapter, bool) {
	a, ok := r.adapters[forge]
	return a, ok
}

// All devuelve los adapters ordenados por nombre de forge, para un arranque
// determinista.
func (r *Registry) All() []Adapter {
	out := make([]Adapter, 0, len(r.adapters))
	for _, a := range r.adapters {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Forge() < out[j].Forge() })
	return out
}

// Collect consulta un forge y compone su resultado del inbox. Si la
// autenticación falla se registra un warning, pero las consultas se intentan
// igual: un fallo de `auth status` no debe ocultar datos que sí se pueden leer.
func Collect(ctx context.Context, a Adapter) inbox.ForgeResult {
	res := inbox.ForgeResult{Forge: a.Forge(), Host: a.Host()}

	if auth := a.Auth(ctx); !auth.OK {
		res.Warnings = append(res.Warnings, model.Warning{
			Forge: a.Forge(),
			Kind:  "auth",
			Msg:   auth.Reason,
		})
	}

	res.Authored, res.Warnings = collectSection(ctx, a, model.SectionAuthored, a.Authored, res.Warnings)
	res.Review, res.Warnings = collectSection(ctx, a, model.SectionReview, a.ReviewRequested, res.Warnings)
	res.Mentions, res.Warnings = collectSection(ctx, a, model.SectionMentions, a.Mentions, res.Warnings)
	return res
}

// collectSection ejecuta un listado y etiqueta con su sección los warnings que
// produzca, para que la UI pueda distinguir qué sección no se pudo leer.
func collectSection(
	ctx context.Context,
	a Adapter,
	section model.Section,
	list func(context.Context) ([]model.Item, []model.Warning),
	warnings []model.Warning,
) ([]model.Item, []model.Warning) {
	items, warns := list(ctx)
	for i := range warns {
		if warns[i].Section == "" {
			warns[i].Section = section
		}
	}
	return items, append(warnings, warns...)
}
