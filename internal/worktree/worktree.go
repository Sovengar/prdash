// Package worktree provisiona los worktrees de review. Define el puerto que el
// orquestador usa y una implementación de git directa. La implementación nativa
// de Herdr llega en una etapa posterior y cumple este mismo contrato, de modo
// que el llamador nunca sabe cuál corre.
package worktree

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
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

// Provisioner es el puerto de provisión de worktrees.
type Provisioner interface {
	// Create provisiona el worktree del spec. Si ya existe uno en el destino
	// sobre la misma rama, lo reutiliza sin duplicar.
	Create(ctx context.Context, spec Spec) (Worktree, error)
	// Remove quita el worktree identificado por id (su ruta).
	Remove(ctx context.Context, id string) error
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
		return Worktree{}, fmt.Errorf("worktree: spec incompleto (repo, rama y destino son obligatorios)")
	}

	if wt, ok, err := g.inspect(ctx, spec.Path); err != nil {
		return Worktree{}, err
	} else if ok {
		if wt.Branch != spec.Branch {
			return Worktree{}, fmt.Errorf("worktree: %s ya aloja la rama %s, no %s", spec.Path, wt.Branch, spec.Branch)
		}
		return wt, nil
	}

	if err := os.MkdirAll(filepath.Dir(spec.Path), 0o755); err != nil {
		return Worktree{}, fmt.Errorf("preparar el destino del worktree %s: %w", spec.Path, err)
	}
	if _, err := g.git.Run(ctx, spec.Repo, "worktree", "add", "--quiet", spec.Path, spec.Branch); err != nil {
		g.cleanPartial(spec.Path)
		return Worktree{}, fmt.Errorf("crear el worktree en %s: %w", spec.Path, err)
	}

	label := spec.Label
	if label == "" {
		label = filepath.Base(spec.Path)
	}
	return Worktree{ID: spec.Path, Label: label, Path: spec.Path, Branch: spec.Branch, Repo: spec.Repo}, nil
}

// Remove quita el worktree en id. Resuelve el repo principal desde el propio
// worktree (fichero .git con gitdir:) para no depender de estado en memoria.
func (g *GitDirect) Remove(ctx context.Context, id string) error {
	repo := mainRepoOf(id)
	if repo == "" {
		return fmt.Errorf("worktree: no se pudo localizar el repo de %s", id)
	}
	if _, err := g.git.Run(ctx, repo, "worktree", "remove", "--force", id); err != nil {
		return fmt.Errorf("quitar el worktree %s: %w", id, err)
	}
	return nil
}

// List escanea la raíz y devuelve los worktrees enlazados, ordenados por ruta.
func (g *GitDirect) List(ctx context.Context) []Worktree {
	if g.Base == "" {
		return nil
	}
	var out []Worktree
	_ = filepath.WalkDir(g.Base, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if d.Name() == ".git" {
			return fs.SkipDir
		}
		if !isLinkedWorktree(path) {
			return nil
		}
		branch, _ := g.git.Run(ctx, path, "rev-parse", "--abbrev-ref", "HEAD")
		out = append(out, Worktree{
			ID:     path,
			Label:  filepath.Base(path),
			Path:   path,
			Branch: strings.TrimSpace(branch),
			Repo:   mainRepoOf(path),
		})
		return fs.SkipDir
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
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
		return Worktree{}, false, fmt.Errorf("worktree: %s ya existe y no es un worktree enlazado", path)
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
