package worktree

import (
	"context"
	"fmt"
	"path/filepath"

	"prdash/internal/herdr"
)

// HerdrRunner es el subconjunto del puerto de Herdr que necesita la provisión
// nativa. *herdr.Client lo implementa.
type HerdrRunner interface {
	Available() bool
	WorktreeCreate(ctx context.Context, spec herdr.WorktreeSpec) (herdr.WorktreeInfo, error)
	WorktreeList(ctx context.Context, cwd string) ([]herdr.WorktreeInfo, error)
	WorktreeRemove(ctx context.Context, workspaceID string, force bool) error
}

// HerdrNative provisiona worktrees delegando en el nativo de Herdr, que liga el
// checkout a un workspace de Herdr (el contenedor que el layout usa). Solo se
// usa dentro de Herdr; el fetch y la rama local los sigue resolviendo prdash.
type HerdrNative struct {
	client HerdrRunner
	scan   *GitDirect
}

// NewHerdrNative construye el provisioner nativo sobre la raíz de worktrees
// dada (para List y como respaldo de Remove).
func NewHerdrNative(client HerdrRunner, base string) *HerdrNative {
	return &HerdrNative{client: client, scan: NewGitDirect(base)}
}

// Select elige la implementación de provisión según el entorno: nativa dentro
// de Herdr (con el binario disponible) y git directo fuera. El llamador nunca
// sabe cuál corre.
func Select(client HerdrRunner, base string) Provisioner {
	if client != nil && client.Available() {
		return NewHerdrNative(client, base)
	}
	return NewGitDirect(base)
}

// Create provisiona el worktree con `herdr worktree create --no-focus`. Si el
// destino ya aloja un worktree enlazado, lo reutiliza sin duplicar y resuelve
// su workspace si sigue abierto.
func (h *HerdrNative) Create(ctx context.Context, spec Spec) (Worktree, error) {
	if spec.Repo == "" || spec.Branch == "" || spec.Path == "" {
		return Worktree{}, fmt.Errorf("worktree: spec incompleto (repo, rama y destino son obligatorios)")
	}
	if Exists(spec.Path) {
		return h.reuse(ctx, spec), nil
	}

	info, err := h.client.WorktreeCreate(ctx, herdr.WorktreeSpec{
		Cwd:     spec.Repo,
		Branch:  spec.Branch,
		Path:    spec.Path,
		Label:   spec.Label,
		NoFocus: true,
	})
	if err != nil {
		return Worktree{}, fmt.Errorf("crear el worktree nativo en %s: %w", spec.Path, err)
	}

	wt := Worktree{
		ID:          spec.Path,
		Label:       spec.Label,
		Path:        spec.Path,
		Branch:      spec.Branch,
		Repo:        spec.Repo,
		WorkspaceID: info.WorkspaceID,
		RootPaneID:  info.RootPaneID,
	}
	if info.Path != "" {
		wt.Path = info.Path
		wt.ID = info.Path
	}
	if info.Branch != "" {
		wt.Branch = info.Branch
	}
	if info.Label != "" {
		wt.Label = info.Label
	}
	if wt.Label == "" {
		wt.Label = spec.Label
	}
	return wt, nil
}

// reuse devuelve el worktree ya existente y, si sigue abierto como workspace,
// su id de contenedor. Un worktree cerrado se devuelve sin contenedor: el
// layout pedirá entonces un workspace propio.
func (h *HerdrNative) reuse(ctx context.Context, spec Spec) Worktree {
	wt := Worktree{ID: spec.Path, Label: spec.Label, Path: spec.Path, Branch: spec.Branch, Repo: spec.Repo}
	if wt.Label == "" {
		wt.Label = filepath.Base(spec.Path)
	}
	infos, err := h.client.WorktreeList(ctx, spec.Repo)
	if err != nil {
		return wt
	}
	for _, info := range infos {
		if info.Path == spec.Path {
			wt.WorkspaceID = info.OpenWorkspaceID
			if info.Branch != "" {
				wt.Branch = info.Branch
			}
			if info.Label != "" {
				wt.Label = info.Label
			}
			break
		}
	}
	return wt
}

// Remove quita el worktree nativo (y su workspace) si puede resolver el
// workspace a partir de la ruta; si no, cae al borrado con git directo.
func (h *HerdrNative) Remove(ctx context.Context, id string) error {
	if repo := mainRepoOf(id); repo != "" {
		if infos, err := h.client.WorktreeList(ctx, repo); err == nil {
			for _, info := range infos {
				if info.Path == id && info.OpenWorkspaceID != "" {
					if err := h.client.WorktreeRemove(ctx, info.OpenWorkspaceID, true); err != nil {
						return fmt.Errorf("quitar el worktree nativo %s: %w", id, err)
					}
					return nil
				}
			}
		}
	}
	return h.scan.Remove(ctx, id)
}

// List delega en el escaneo de worktrees enlazados bajo la raíz. Los worktrees
// nativos también son worktrees de git reales, así que el listado es el mismo.
func (h *HerdrNative) List(ctx context.Context) []Worktree { return h.scan.List(ctx) }
