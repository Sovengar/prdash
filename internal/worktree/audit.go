package worktree

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LabelPrefix es el prefijo de ownership de los worktrees de review de prdash.
// Un worktree ajeno (cualquier otro nombre) nunca se lista ni se borra.
const LabelPrefix = "prdash-"

// Owned informa si un worktree pertenece a prdash por su etiqueta o por el
// nombre de su ruta. Es la única puerta de entrada a la limpieza: lo que no es
// propio no se toca.
func Owned(label, path string) bool {
	return strings.HasPrefix(label, LabelPrefix) || strings.HasPrefix(filepath.Base(path), LabelPrefix)
}

// Entry es un worktree propio auditado.
type Entry struct {
	Worktree
	// Orphan marca un worktree cuyo repo de origen ya no es accesible, de modo
	// que la limpieza pueda reportarlo antes de borrarlo.
	Orphan bool
	// Reason explica por qué se considera huérfano.
	Reason string
}

// Audit recorre la raíz y devuelve solo los worktrees con ownership prdash,
// ordenados por ruta. Marca huérfano el worktree cuyo enlace al repo de origen
// está roto (gitdir inexistente).
func (g *GitDirect) Audit(ctx context.Context) []Entry {
	if g.Base == "" {
		return nil
	}
	var out []Entry
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
		if !Owned(filepath.Base(path), path) {
			return fs.SkipDir
		}
		branch, _ := g.git.Run(ctx, path, "rev-parse", "--abbrev-ref", "HEAD")
		e := Entry{Worktree: Worktree{
			ID:     path,
			Label:  filepath.Base(path),
			Path:   path,
			Branch: strings.TrimSpace(branch),
			Repo:   mainRepoOf(path),
		}}
		if !sourceReachable(path) {
			e.Orphan = true
			e.Reason = "el repo de origen del worktree ya no es accesible"
		}
		out = append(out, e)
		return fs.SkipDir
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// linkedGitDir devuelve el gitdir apuntado por el fichero .git de un worktree
// enlazado, resuelto a absoluto, o "" si la ruta no lo declara.
func linkedGitDir(path string) string {
	raw, err := os.ReadFile(filepath.Join(path, ".git"))
	if err != nil {
		return ""
	}
	rest, ok := strings.CutPrefix(strings.TrimSpace(string(raw)), "gitdir:")
	if !ok {
		return ""
	}
	gitdir := strings.TrimSpace(rest)
	if gitdir == "" {
		return ""
	}
	if !filepath.IsAbs(gitdir) {
		gitdir = filepath.Join(path, gitdir)
	}
	return gitdir
}

// sourceReachable informa si el gitdir al que apunta el worktree sigue
// existiendo: un enlace roto es un worktree huérfano.
func sourceReachable(path string) bool {
	gitdir := linkedGitDir(path)
	if gitdir == "" {
		return false
	}
	_, err := os.Stat(gitdir)
	return err == nil
}
