// Package executor aplica el plan de review usando puertos. Orquesta
// resolver→fetch→rama→worktree→layout sin conocer ningún detalle de git, de
// Herdr ni de la configuración: solo habla con las interfaces inyectadas.
//
// El puerto de Herdr puede ser nil (fuera de Herdr): el núcleo de F2 monta el
// worktree igualmente y el layout se reporta como no disponible.
package executor

import (
	"context"
	"fmt"

	"prdash/internal/cache"
	"prdash/internal/forge/model"
	"prdash/internal/herdr"
	"prdash/internal/review/plan"
	"prdash/internal/worktree"
)

// Resolver es el puerto del resolutor de repos: resuelve la ruta local, asegura
// el clon bare, trae el ref de review, decide rutas de worktree y recuerda
// rutas y reviews activos.
type Resolver interface {
	ResolveLocal(ref model.RepoRef) (string, bool)
	HasBare(ref model.RepoRef) bool
	EnsureBare(ctx context.Context, ref model.RepoRef) (string, error)
	RemoveBare(ref model.RepoRef) error
	FetchReviewRef(ctx context.Context, repo string, it model.Item) (string, error)
	WorktreePath(ref model.RepoRef, number int) string
	Remember(ref model.RepoRef, path string)
	RecordReview(it model.Item, rec cache.ReviewRecord) error
	ActiveReview(id model.ID) (cache.ReviewRecord, bool)
}

// HerdrPort es el puerto de montaje del layout dentro de Herdr. La
// implementación real es *herdr.Client.
type HerdrPort interface {
	// Available informa si Herdr está presente y operativo.
	Available() bool
	// MountLayout abre el layout de panes del plan sobre el contenedor y
	// devuelve los avisos no fatales.
	MountLayout(ctx context.Context, container herdr.Container, pl plan.Plan) ([]string, error)
	// Notify muestra una notificación en Herdr.
	Notify(ctx context.Context, title string, opts herdr.NotifyOptions) error
}

// Executor monta el review de un ítem.
type Executor struct {
	Resolver  Resolver
	Worktrees worktree.Provisioner
	Herdr     HerdrPort
	Tools     plan.Tools
	Env       plan.Env
	// Planner permite sustituir la construcción del plan en tests; nil usa
	// plan.Build.
	Planner func(pr model.Item, wt plan.Worktree) plan.Plan
}

// Result describe el review montado.
type Result struct {
	// RepoPath es el clon local (normal o bare) del que se sacó el worktree.
	RepoPath string
	// Branch es la rama local de trabajo del ítem.
	Branch string
	// Worktree es el worktree provisionado (o reutilizado).
	Worktree worktree.Worktree
	// Plan es el plan de panes construido.
	Plan plan.Plan
	// Reused indica que el worktree ya existía y no se creó otro.
	Reused bool
	// Herdr indica si el layout se montó (Herdr disponible).
	Herdr bool
	// Warnings son avisos no fatales (herramienta ausente, Herdr no disponible).
	Warnings []string
}

// Mount monta el review de un ítem: resuelve o clona el repo, trae el ref de
// review, crea (o reutiliza) el worktree y aplica el layout si Herdr está.
//
// Ante un fallo no deja basura: si hubo que crear el clon bare en esta llamada,
// se borra; un worktree a medias lo limpia el propio provisioner.
func (e *Executor) Mount(ctx context.Context, it model.Item) (Result, error) {
	var res Result

	repoPath, createdBare, err := e.resolveRepo(ctx, it)
	if err != nil {
		return res, err
	}
	res.RepoPath = repoPath

	wtPath := e.worktreePath(it)
	branch, err := e.Resolver.FetchReviewRef(ctx, repoPath, it)
	if err != nil {
		e.cleanupBare(createdBare, it)
		return res, err
	}
	res.Branch = branch

	res.Reused = worktree.Exists(wtPath)
	wt, err := e.Worktrees.Create(ctx, worktree.Spec{
		Repo:   repoPath,
		Branch: branch,
		Path:   wtPath,
		Label:  Label(it.Number),
	})
	if err != nil {
		e.cleanupBare(createdBare, it)
		return res, err
	}
	res.Worktree = wt

	res.Plan = e.buildPlan(it, wt)
	res.Warnings = append(res.Warnings, res.Plan.Warnings...)
	res.Herdr = e.mountLayout(ctx, wt, res.Plan, &res)

	if err := e.Resolver.RecordReview(it, cache.ReviewRecord{
		Repo:     repoPath,
		Worktree: wt.Path,
		Branch:   branch,
		Label:    wt.Label,
	}); err != nil {
		res.Warnings = append(res.Warnings, "no se pudo registrar el review activo: "+err.Error())
	}
	return res, nil
}

// ActiveReview devuelve el review ya montado de un ítem, si lo hay.
func (e *Executor) ActiveReview(it model.Item) (worktree.Worktree, bool) {
	rec, ok := e.Resolver.ActiveReview(it.ID())
	if !ok || rec.Worktree == "" {
		return worktree.Worktree{}, false
	}
	return worktree.Worktree{
		ID:     rec.Worktree,
		Label:  rec.Label,
		Path:   rec.Worktree,
		Branch: rec.Branch,
		Repo:   rec.Repo,
	}, true
}

// resolveRepo devuelve la ruta del repo local, clonándolo en bare si no existe.
// created informa si el clon bare se creó en esta llamada (para poder limpiarlo
// si algo falla después).
func (e *Executor) resolveRepo(ctx context.Context, it model.Item) (path string, created bool, err error) {
	if p, ok := e.Resolver.ResolveLocal(it.Ref); ok {
		return p, false, nil
	}
	hadBare := e.Resolver.HasBare(it.Ref)
	p, err := e.Resolver.EnsureBare(ctx, it.Ref)
	if err != nil {
		return "", false, err
	}
	e.Resolver.Remember(it.Ref, p)
	return p, !hadBare, nil
}

// worktreePath usa el review activo si ya existe, y si no la ruta canónica del
// resolutor (único dueño del namespace de rutas).
func (e *Executor) worktreePath(it model.Item) string {
	if rec, ok := e.Resolver.ActiveReview(it.ID()); ok && rec.Worktree != "" {
		return rec.Worktree
	}
	return e.Resolver.WorktreePath(it.Ref, it.Number)
}

// buildPlan construye el plan de panes del review.
func (e *Executor) buildPlan(it model.Item, wt worktree.Worktree) plan.Plan {
	pw := plan.Worktree{Path: wt.Path, Branch: wt.Branch, Label: wt.Label}
	if e.Planner != nil {
		return e.Planner(it, pw)
	}
	return plan.Build(it, pw, e.Tools, e.Env)
}

// mountLayout aplica el plan por el puerto de Herdr. Sin Herdr no es un error:
// se avisa y el worktree queda montado igualmente.
func (e *Executor) mountLayout(ctx context.Context, wt worktree.Worktree, pl plan.Plan, res *Result) bool {
	if e.Herdr == nil || !e.Herdr.Available() {
		res.Warnings = append(res.Warnings, "el layout de review requiere Herdr; el worktree quedó montado")
		return false
	}
	container := herdr.Container{WorkspaceID: wt.WorkspaceID, PaneID: wt.RootPaneID}
	warns, err := e.Herdr.MountLayout(ctx, container, pl)
	res.Warnings = append(res.Warnings, warns...)
	if err != nil {
		res.Warnings = append(res.Warnings, fmt.Sprintf("no se pudo abrir el layout: %v", err))
		return false
	}
	_ = e.Herdr.Notify(ctx, "prdash: review listo", herdr.NotifyOptions{Sound: "done"})
	return true
}

// cleanupBare borra el clon bare creado en esta llamada cuando el montaje falla.
func (e *Executor) cleanupBare(created bool, it model.Item) {
	if !created {
		return
	}
	_ = e.Resolver.RemoveBare(it.Ref)
}

// Label es el nombre con ownership prdash de un review.
func Label(number int) string { return fmt.Sprintf("prdash-pr-%d", number) }
