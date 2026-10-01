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

	return composed(spec, info), nil
}

// composed junta lo que pidió el llamador con lo que contestó Herdr. Es una
// función pura a propósito: son reglas de precedencia entre dos fuentes, y una
// precedencia es exactamente el tipo de cosa que no se puede leer a ojo en un
// cuerpo de provisioning.
//
// La regla, y es una sola: PARA CADA CAMPO, lo que dijo Herdr gana si lo dijo, y
// si no, lo que pidió el llamador. Herdr manda porque es el que sabe: si movió el
// checkout a otro path, o si le renombró la rama, el Worktree tiene que reflejar
// donde está de verdad, no donde se pidió que estuviera. Mentir aquí no da un
// error visible: da un worktree que apunta a un sitio donde no hay nada, y el
// review se monta en el vacío.
//
// Y hay una excepción, que es la etiqueta. `spec.Label` es la etiqueta de
// OWNERSHIP que pidió el llamador, y es lo único por lo que prdash reconoce el
// worktree. En cambio `info.Label` es el nombre del repo que reporta el nativo, y
// `info.WorkspaceLabel` es el `--label` con el que se abrió el workspace: ninguno de
// los dos es lo que prdash pidió. Por eso la etiqueta NO se toma de Herdr si el
// llamador ya dio una; solo se recurre a las otras cuando no dio ninguna, y en ese
// orden. Si se invirtiera, el worktree se renombraría solo al nombre del repo y
// prdash dejaría de reconocerlo.
//
// El último recurso es el nombre del path, porque un worktree sin etiqueta no se
// puede distinguir de otro y el `id` acaba siendo la etiqueta de todos modos.
func composed(spec Spec, info herdr.WorktreeInfo) Worktree {
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
	if wt.Label == "" {
		wt.Label = info.WorkspaceLabel
	}
	if wt.Label == "" {
		wt.Label = info.Label
	}
	// El nombre del path es el último recurso porque es el único que siempre está.
	//
	// Y con un path VACÍO no hay nombre que tomar: `filepath.Base("")` sale ".", y
	// una etiqueta de "." es peor que no tener etiqueta. Vacía se ve que falta;
	// "." parece un nombre elegido, y dos worktrees sin path se llamarían igual.
	// Por eso la condición mira el PATH y no el nombre: lo que falta es el sitio
	// del que sacarlo.
	if wt.Label == "" && wt.Path != "" {
		wt.Label = filepath.Base(wt.Path)
	}
	return wt
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

	// La etiqueta se resuelve con la MISMA regla que en `Create`: la del llamador si la
	// dio, y si no, la que ya tenía el checkout. No es que se parezca, es que es la
	// misma decisión: tener la regla escrita en un sitio y repetida en otro es
	// garantizar que un día diverjan. Un `if` de tres líneas que dice lo mismo que
	// otro `if` de tres líneas es una duplicación con comentarios en los dos sitios.
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
//
// Antes de delegar en el cliente nativo aplica la MISMA guarda de ownership y de
// raíz gestionada que GitDirect.Remove: sin ella, una ruta propia fuera de la
// raíz se borraba bajo Herdr sin pasar por `removablePath`. El rechazo es un
// error, no un borrado, igual que en git directo.
func (h *HerdrNative) Remove(ctx context.Context, id string) error {
	if err := h.scan.removablePath(id); err != nil {
		return err
	}
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

// RemoveIfClean reproduce el candado "solo si limpio" resolviendo el estado con
// el escaneo de git real (los worktrees nativos también lo son) y delegando el
// borrado en el nativo, para no dejar el workspace huérfano. Comparte con
// GitDirect el mismo `shouldRemove`: los guardas no se relajan por correr dentro
// de Herdr.
func (h *HerdrNative) RemoveIfClean(ctx context.Context, id string) (bool, string, error) {
	ok, reason, err := h.scan.shouldRemove(ctx, id)
	if err != nil || !ok {
		return false, reason, err
	}
	if err := h.Remove(ctx, id); err != nil {
		return false, "", err
	}
	return true, "", nil
}

// List delega en el escaneo de worktrees enlazados bajo la raíz. Los worktrees
// nativos también son worktrees de git reales, así que el listado es el mismo.
func (h *HerdrNative) List(ctx context.Context) []Worktree { return h.scan.List(ctx) }

// Audit delega en el mismo escaneo con ownership y detección de huérfanos: los
// worktrees nativos también son worktrees de git, de modo que la limpieza
// funciona igual dentro y fuera de Herdr.
func (h *HerdrNative) Audit(ctx context.Context) []Entry { return h.scan.Audit(ctx) }
