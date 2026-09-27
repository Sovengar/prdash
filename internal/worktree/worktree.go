// Package worktree provisiona los worktrees de review. Define el puerto que el
// orquestador usa y una implementación de git directa. La implementación nativa
// de Herdr llega en una etapa posterior y cumple este mismo contrato, de modo
// que el llamador nunca sabe cuál corre.
package worktree

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"prdash/internal/gitcmd"
)

// Spec describe el worktree a provisionar.
type Spec struct {
	// Repo es el repo (o clon bare) que aloja el worktree.
	Repo string
	// Branch es la rama local ya existente que se va a sacar.
	Branch string
	// Path es el destino del worktree.
	Path string
	// Label es el nombre con ownership prdash que identifica el worktree.
	Label string
}

// Worktree es un worktree provisionado.
type Worktree struct {
	ID     string
	Label  string
	Path   string
	Branch string
	Repo   string
	// WorkspaceID y RootPaneID identifican el contenedor nativo de Herdr que
	// aloja el worktree. Quedan vacíos en la provisión con git directo.
	WorkspaceID string
	RootPaneID  string
}

// Motivos por los que RemoveIfClean conserva un worktree. Son la respuesta de
// "no lo borré, y por esto": un worktree sucio o un estado ilegible no son un
// error de la operación, son la razón por la que no se borra.
const (
	// KeptUncommitted marca un checkout con cambios sin commitear (incluidos
	// archivos sin trackear).
	KeptUncommitted = "the worktree has uncommitted changes"
	// KeptUnreadable marca un worktree cuyo estado de git no se pudo comprobar.
	KeptUnreadable = "could not read the worktree status"
)

// Provisioner es el puerto de provisión de worktrees.
type Provisioner interface {
	// Create provisiona el worktree del spec. Si ya existe uno en el destino
	// sobre la misma rama, lo reutiliza sin duplicar.
	Create(ctx context.Context, spec Spec) (Worktree, error)
	// Remove quita el worktree identificado por id (su ruta). Borra aunque haya
	// cambios sin commitear: es la vía de las rutas explícitas.
	Remove(ctx context.Context, id string) error
	// RemoveIfClean quita el worktree de id solo si está limpio. Con cambios sin
	// commitear (incluidos sin trackear) o con el estado de git ilegible lo
	// conserva y devuelve el motivo. Una ruta ausente es un no-op sin error.
	RemoveIfClean(ctx context.Context, id string) (removed bool, reason string, err error)
	// List devuelve los worktrees bajo la raíz con ownership prdash.
	List(ctx context.Context) []Worktree
	// Audit lista los worktrees con ownership prdash y marca los huérfanos.
	Audit(ctx context.Context) []Entry
}

// GitDirect provisiona con `git worktree add` directo. Es la implementación
// usada fuera de Herdr.
type GitDirect struct {
	// Base es la raíz de worktrees donde se escanea List y se acotan los
	// borrados de limpieza.
	Base string
	git  *gitcmd.Runner
}

// NewGitDirect construye un provisioner de git directo sobre la raíz dada.
func NewGitDirect(base string) *GitDirect {
	return &GitDirect{Base: base, git: gitcmd.New()}
}

// Create saca la rama local en un worktree nuevo. Nunca delega el fetch ni la
// creación de la rama: los recibe ya resueltos.
func (g *GitDirect) Create(ctx context.Context, spec Spec) (Worktree, error) {
	if spec.Repo == "" || spec.Branch == "" || spec.Path == "" {
		return Worktree{}, fmt.Errorf("worktree: incomplete spec (repo, branch and destination are required)")
	}

	if wt, ok, err := g.inspect(ctx, spec.Path); err != nil {
		return Worktree{}, err
	} else if ok {
		if wt.Branch != spec.Branch {
			return Worktree{}, fmt.Errorf("worktree: %s already holds branch %s, not %s", spec.Path, wt.Branch, spec.Branch)
		}
		return wt, nil
	}

	if err := os.MkdirAll(filepath.Dir(spec.Path), 0o755); err != nil {
		return Worktree{}, fmt.Errorf("prepare the worktree destination %s: %w", spec.Path, err)
	}
	if _, err := g.git.Run(ctx, spec.Repo, "worktree", "add", "--quiet", spec.Path, spec.Branch); err != nil {
		g.cleanPartial(spec.Path)
		return Worktree{}, fmt.Errorf("create the worktree at %s: %w", spec.Path, err)
	}

	label := spec.Label
	if label == "" {
		label = filepath.Base(spec.Path)
	}
	return Worktree{ID: spec.Path, Label: label, Path: spec.Path, Branch: spec.Branch, Repo: spec.Repo}, nil
}

// Remove quita el worktree en id, previa comprobación de ownership y de que la
// ruta vive bajo la raíz gestionada: nunca toca worktrees ajenos. Resuelve el
// repo principal desde el propio worktree (fichero .git con gitdir:) para no
// depender de estado en memoria.
//
// Si el repo de origen existe, delega en `git worktree remove` y, si la entrada
// está corrupta, poda el registro. Si el origen desapareció (huérfano), borra
// el checkout directamente: no hay metadatos que podar.
func (g *GitDirect) Remove(ctx context.Context, id string) error {
	if err := g.removablePath(id); err != nil {
		return err
	}
	repo := mainRepoOf(id)
	if repo == "" {
		return fmt.Errorf("worktree: could not locate the repo for %s", id)
	}
	if _, err := g.git.Run(ctx, repo, "worktree", "remove", "--force", id); err == nil {
		return nil
	}
	if sourceReachable(id) {
		// El repo vive pero la entrada no se pudo quitar (registro corrupto):
		// se poda y se borra el residuo, siempre dentro del ownership.
		_, _ = g.git.Run(ctx, repo, "worktree", "prune")
	}
	if err := os.RemoveAll(id); err != nil {
		return fmt.Errorf("remove the worktree checkout %s: %w", id, err)
	}
	return nil
}

// removablePath exige ownership prdash y que la ruta viva bajo la raíz
// gestionada. Es la barrera que garantiza que la limpieza nunca toca ajenos.
func (g *GitDirect) removablePath(id string) error {
	if !Owned(filepath.Base(id), id) {
		return fmt.Errorf("worktree: %s is not a prdash worktree; leaving it alone", id)
	}
	if g.Base != "" {
		rel, err := filepath.Rel(g.Base, id)
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("worktree: %s is outside the managed root", id)
		}
	}
	return nil
}

// shouldRemove comprueba, sin borrar, si el worktree puede quitarse con el
// candado "solo si limpio". Aplica los mismos guardas que Remove; una ruta
// ausente no es un error (no hay nada que quitar), un checkout sucio se
// conserva con su motivo y un estado ilegible también, por fail-safe.
func (g *GitDirect) shouldRemove(ctx context.Context, id string) (ok bool, reason string, err error) {
	if err := g.removablePath(id); err != nil {
		return false, "", err
	}
	if !Exists(id) {
		return false, "", nil
	}
	dirty, err := g.dirty(ctx, id)
	if err != nil {
		// No se pudo leer el estado: ante la duda, no se borra y no es un fallo
		// de la operación, es la razón por la que se conserva.
		return false, KeptUnreadable, nil
	}
	if dirty {
		return false, KeptUncommitted, nil
	}
	return true, "", nil
}

// RemoveIfClean quita el worktree de id solo si está limpio. Es la vía del
// auto-borrado tras un merge: a diferencia de Remove, no destruye trabajo sin
// commitear y ante un estado ilegible conserva.
func (g *GitDirect) RemoveIfClean(ctx context.Context, id string) (bool, string, error) {
	ok, reason, err := g.shouldRemove(ctx, id)
	if err != nil || !ok {
		return false, reason, err
	}
	if err := g.Remove(ctx, id); err != nil {
		return false, "", err
	}
	return true, "", nil
}

// dirty informa si el árbol de trabajo tiene cambios sin commitear. Usa
// `status --porcelain` y no `diff --quiet`: un archivo nuevo sin trackear es
// trabajo sin commitear y `diff` lo ignora.
func (g *GitDirect) dirty(ctx context.Context, id string) (bool, error) {
	out, err := g.git.Run(ctx, id, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

// List escanea la raíz y devuelve los worktrees con ownership prdash,
// ordenados por ruta.
func (g *GitDirect) List(ctx context.Context) []Worktree {
	entries := g.Audit(ctx)
	out := make([]Worktree, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Worktree)
	}
	return out
}

// Exists informa si la ruta ya aloja un worktree enlazado.
func Exists(path string) bool { return isLinkedWorktree(path) }

// inspect clasifica el destino: worktree reutilizable, ocupado por algo que no
// es un worktree, o libre.
func (g *GitDirect) inspect(ctx context.Context, path string) (Worktree, bool, error) {
	info, err := os.Lstat(filepath.Join(path, ".git"))
	if err != nil {
		return Worktree{}, false, nil
	}
	if info.IsDir() {
		return Worktree{}, false, fmt.Errorf("worktree: %s already exists and is not a linked worktree", path)
	}
	branch, _ := g.git.Run(ctx, path, "rev-parse", "--abbrev-ref", "HEAD")
	return Worktree{
		ID:     path,
		Label:  filepath.Base(path),
		Path:   path,
		Branch: strings.TrimSpace(branch),
		Repo:   mainRepoOf(path),
	}, true, nil
}

// cleanPartial borra restos de un `git worktree add` fallido, solo dentro de la
// raíz propia y solo si no llegó a registrarse como worktree válido.
func (g *GitDirect) cleanPartial(path string) {
	if g.Base != "" {
		rel, err := filepath.Rel(g.Base, path)
		if err != nil || strings.HasPrefix(rel, "..") {
			return
		}
	}
	if isLinkedWorktree(path) {
		return
	}
	_ = os.RemoveAll(path)
}

// isLinkedWorktree reconoce un worktree enlazado (`.git` es un fichero).
func isLinkedWorktree(path string) bool {
	info, err := os.Lstat(filepath.Join(path, ".git"))
	return err == nil && !info.IsDir()
}

// mainRepoOf deduce el repo principal desde el fichero .git de un worktree.
func mainRepoOf(path string) string {
	gitdir := linkedGitDir(path)
	if gitdir == "" {
		return ""
	}
	// <main>/.git/worktrees/<nombre>
	return filepath.Dir(filepath.Dir(gitdir))
}
