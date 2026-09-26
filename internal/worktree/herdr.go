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
	WorkspaceCreate(ctx context.Context, spec herdr.WorkspaceSpec) (herdr.WorkspaceInfo, error)
	PaneList(ctx context.Context, workspaceID string) ([]herdr.PaneInfo, error)
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
		return Worktree{}, fmt.Errorf("worktree: incomplete spec (repo, branch and destination are required)")
	}
	if Exists(spec.Path) {
		return h.reuse(ctx, spec)
	}

	info, err := h.client.WorktreeCreate(ctx, herdr.WorktreeSpec{
		Cwd:     spec.Repo,
		Branch:  spec.Branch,
		Path:    spec.Path,
		Label:   spec.Label,
		NoFocus: true,
	})
	if err != nil {
		return Worktree{}, fmt.Errorf("create the native worktree at %s: %w", spec.Path, err)
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
	// La etiqueta de ownership es la que pidió el llamador: `--label` etiqueta
	// el workspace, mientras que `worktree.label` del nativo es el nombre del
	// repo y no sirve como identificador de prdash.
	if wt.Label == "" {
		wt.Label = info.WorkspaceLabel
	}
	if wt.Label == "" {
		wt.Label = info.Label
	}
	if wt.Label == "" {
		wt.Label = filepath.Base(wt.Path)
	}
	return wt, nil
}

// reuse devuelve el worktree ya existente y, si sigue abierto como workspace,
// su id de contenedor. Verifica que la rama coincida con la pedida, igual que
// la provisión con git directo: reutilizar un checkout de otra rama sería un
// falso montaje.
//
// Si el checkout no tiene workspace abierto, abre uno con cwd en el propio
// worktree en vez de devolver el worktree sin contenedor. Sin esto el layout se
// fabricaba un workspace propio y el review aparecía como un workspace suelto,
// desligado del worktree que lo contiene. `herdr worktree create` no puede
// resolver el caso: hace internamente `git worktree add` y se niega a abrir un
// path que ya existe, pero `workspace create` con ese cwd sí queda registrado
// como el workspace abierto del worktree (verificado en Herdr 0.9.1).
func (h *HerdrNative) reuse(ctx context.Context, spec Spec) (Worktree, error) {
	existing, ok, err := h.scan.inspect(ctx, spec.Path)
	if err != nil {
		return Worktree{}, err
	}
	if !ok {
		return Worktree{}, fmt.Errorf("worktree: %s does not hold a linked worktree", spec.Path)
	}
	if existing.Branch != spec.Branch {
		return Worktree{}, fmt.Errorf("worktree: %s already holds branch %s, not %s", spec.Path, existing.Branch, spec.Branch)
	}

	wt := existing
	wt.Repo = spec.Repo
	if spec.Label != "" {
		wt.Label = spec.Label
	}
	if err := h.attach(ctx, &wt, spec); err != nil {
		return Worktree{}, err
	}
	return wt, nil
}

// attach deja el worktree con un workspace de Herdr. Si el checkout ya tiene uno
// abierto, se reutiliza; si no, se abre uno con cwd en el propio worktree, que es
// lo que Herdr registra como suyo.
//
// el `open_workspace_id` de `worktree list` NO se confía a ciegas: Herdr lo
// guarda en su sesión persistida, así que un workspace cerrado deja el id
// apuntando a nada y `pane list` responde `workspace_not_found` (comprobado en
// 0.9.1). Confiar en él dejaba el worktree sin pane base, y el layout se
// fabricaba entonces un workspace propio desligado del worktree: el review
// aparecía como un workspace suelto y nuevo en cada montaje. Por eso se
// comprueba con `pane list`, que además de validar el id da el pane base.
func (h *HerdrNative) attach(ctx context.Context, wt *Worktree, spec Spec) error {
	if infos, err := h.client.WorktreeList(ctx, spec.Repo); err == nil {
		for _, info := range infos {
			if info.Path != spec.Path || info.OpenWorkspaceID == "" {
				continue
			}
			if panes, err := h.client.PaneList(ctx, info.OpenWorkspaceID); err == nil && len(panes) > 0 {
				wt.WorkspaceID = info.OpenWorkspaceID
				wt.RootPaneID = panes[0].PaneID
				return nil
			}
			break // el id no es fiable: se cae a la adopción
		}
	}
	info, err := h.client.WorkspaceCreate(ctx, herdr.WorkspaceSpec{
		Cwd:     spec.Path,
		Label:   spec.Label,
		NoFocus: true,
	})
	if err != nil {
		return fmt.Errorf("open a Herdr workspace for the worktree at %s (remove it with `prdash worktrees remove %s` and mount again to recreate it): %w", spec.Path, spec.Path, err)
	}
	wt.WorkspaceID = info.WorkspaceID
	wt.RootPaneID = info.RootPaneID
	return nil
}

// Remove quita el worktree nativo (y su workspace) si puede resolver el
// workspace a partir de la ruta; si no, cae al borrado con git directo.
func (h *HerdrNative) Remove(ctx context.Context, id string) error {
	if repo := mainRepoOf(id); repo != "" {
		if infos, err := h.client.WorktreeList(ctx, repo); err == nil {
			for _, info := range infos {
				if info.Path == id && info.OpenWorkspaceID != "" {
					if err := h.client.WorktreeRemove(ctx, info.OpenWorkspaceID, true); err != nil {
						return fmt.Errorf("remove the native worktree %s: %w", id, err)
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

// Audit delega en el mismo escaneo con ownership y detección de huérfanos: los
// worktrees nativos también son worktrees de git, de modo que la limpieza
// funciona igual dentro y fuera de Herdr.
func (h *HerdrNative) Audit(ctx context.Context) []Entry { return h.scan.Audit(ctx) }
