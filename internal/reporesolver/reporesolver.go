// Package reporesolver es el único dueño del namespace de rutas de prdash.
//
// Indexa los clones locales sobre los roots configurados, recuerda rutas ya
// resueltas, crea el clon bare cuando falta, trae el ref de review y prepara la
// rama local de trabajo. No llama a la API del forge: solo usa git, de modo que
// resolver un ítem no depende de credenciales ni de red más allá del propio
// fetch del ref.
package reporesolver

import (
	"context"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"prdash/internal/cache"
	"prdash/internal/forge/model"
	"prdash/internal/gitcmd"
)

// Options configura un Resolver.
type Options struct {
	// Roots son los directorios donde buscar clones locales.
	Roots []string
	// CloneDir es la raíz de los clones bare.
	CloneDir string
	// WorktreeDir es la raíz de los worktrees.
	WorktreeDir string
	// MemoPath es el fichero de memoria de rutas. Vacío usa el XDG de cache.
	MemoPath string
	// Hosts mapea host → nombre de forge para normalizar remotos.
	Hosts map[string]string
	// Prefixes mapea host → relative URL root de la instancia (p. ej. "git"
	// para un GitLab self-managed en subcarpeta). Se aplica simétricamente al
	// construir la URL de clonado (CloneURL) y al normalizar remotos
	// (ParseRemoteURL). Vacío = instancia en la raíz del host.
	Prefixes map[string]string
	// CloneURL construye la URL de clonado de un repo. Inyectable para usar
	// remotos locales en tests; por defecto arma la URL canónica del forge
	// incluyendo el prefijo de subcarpeta del host.
	CloneURL func(model.RepoRef) string
	// ParseRemote normaliza una URL remota a RepoRef. Inyectable para tests;
	// por defecto parsea las URLs de git de los forges conocidos quitando el
	// prefijo de subcarpeta del host.
	ParseRemote func(string) (model.RepoRef, bool)
	// Git permite sustituir el ejecutor de git; vacío usa el binario del PATH.
	Git *gitcmd.Runner
}

// Resolver resuelve rutas locales, provisiona clones bare y prepara la rama de
// review. Es seguro para uso concurrente.
type Resolver struct {
	roots       []string
	cloneDir    string
	worktreeDir string
	cloneURL    func(model.RepoRef) string
	parseRemote func(string) (model.RepoRef, bool)
	git         *gitcmd.Runner
	store       *cache.Store

	mu      sync.Mutex
	index   map[string]string
	indexed bool
}

// New construye un Resolver con las opciones dadas.
func New(opts Options) *Resolver {
	r := &Resolver{
		roots:       append([]string(nil), opts.Roots...),
		cloneDir:    opts.CloneDir,
		worktreeDir: opts.WorktreeDir,
		git:         opts.Git,
	}
	if r.git == nil {
		r.git = gitcmd.New()
	}
	r.cloneURL = opts.CloneURL
	if r.cloneURL == nil {
		prefixes := opts.Prefixes
		r.cloneURL = func(ref model.RepoRef) string {
			return CloneURL(ref, prefixes[ref.Host])
		}
	}
	r.parseRemote = opts.ParseRemote
	if r.parseRemote == nil {
		hosts := opts.Hosts
		prefixes := opts.Prefixes
		r.parseRemote = func(raw string) (model.RepoRef, bool) {
			return ParseRemoteURL(raw, hosts, prefixes)
		}
	}
	memoPath := opts.MemoPath
	if memoPath == "" {
		if p, err := cache.MemoPath(); err == nil {
			memoPath = p
		}
	}
	r.store = cache.OpenStore(memoPath)
	return r
}

// ResolveLocal devuelve la ruta del clon local de un repo, si existe. Consulta
// primero la memoria de rutas y, si falla, el índice de los roots (que se
// construye una sola vez).
func (r *Resolver) ResolveLocal(ref model.RepoRef) (string, bool) {
	key := repoKey(ref)
	if p, ok := r.store.Route(key); ok && isRepo(p) {
		return p, true
	}

	r.mu.Lock()
	if !r.indexed {
		r.index = r.buildIndex()
		r.indexed = true
	}
	p, ok := r.index[key]
	r.mu.Unlock()
	if !ok {
		return "", false
	}
	r.Remember(ref, p)
	return p, true
}

// HasBare informa si ya existe el clon bare de un repo.
func (r *Resolver) HasBare(ref model.RepoRef) bool { return isRepo(r.barePath(ref)) }

// EnsureBare devuelve el clon bare del repo, clonándolo si falta. El clon se
// arma en un directorio temporal y se publica con rename, de modo que un fallo
// no deja un clon a medias.
func (r *Resolver) EnsureBare(ctx context.Context, ref model.RepoRef) (string, error) {
	dest := r.barePath(ref)
	if isRepo(dest) {
		return dest, nil
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", fmt.Errorf("preparar el clon bare %s: %w", dest, err)
	}
	if _, err := os.Stat(dest); err == nil {
		// Restos de un intento previo: se limpian antes de reintentar.
		if err := os.RemoveAll(dest); err != nil {
			return "", fmt.Errorf("limpiar el clon bare incompleto %s: %w", dest, err)
		}
	}

	url := r.cloneURL(ref)
	tmp := dest + ".tmp-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if _, err := r.git.Run(ctx, "", "clone", "--bare", "--quiet", "--", url, tmp); err != nil {
		os.RemoveAll(tmp)
		return "", fmt.Errorf("clonar %s: %w", url, err)
	}
	if err := os.Rename(tmp, dest); err != nil {
		os.RemoveAll(tmp)
		return "", fmt.Errorf("publicar el clon bare %s: %w", dest, err)
	}
	return dest, nil
}

// RemoveBare borra el clon bare de un repo si existe. Se usa para no dejar
// basura cuando el montaje falla después de haberlo creado.
func (r *Resolver) RemoveBare(ref model.RepoRef) error {
	dest := r.barePath(ref)
	if _, err := os.Stat(dest); err != nil {
		return nil
	}
	return os.RemoveAll(dest)
}

// FetchReviewRef trae el ref de review del ítem y asegura una rama local de
// trabajo. Devuelve el nombre de la rama local. Nunca toca una rama que ya
// exista (reutiliza el worktree en curso).
func (r *Resolver) FetchReviewRef(ctx context.Context, repo string, it model.Item) (string, error) {
	src, ok := ReviewRef(it)
	if !ok {
		return "", fmt.Errorf("forge %q sin ref de review conocido", it.Forge)
	}
	branch := ReviewBranch(it.Number)
	track := fmt.Sprintf("refs/prdash/%s/%d", it.Forge, it.Number)

	if _, err := r.git.Run(ctx, repo, "fetch", "--no-tags", "origin", "+"+src+":"+track); err != nil {
		return "", fmt.Errorf("traer %s de %s: %w", src, it.Ref.Project, err)
	}
	if r.branchExists(ctx, repo, branch) {
		return branch, nil
	}
	if _, err := r.git.Run(ctx, repo, "branch", branch, track); err != nil {
		return "", fmt.Errorf("crear la rama local %s: %w", branch, err)
	}
	return branch, nil
}

// Remember recuerda la ruta local de un repo en la memoria persistida.
func (r *Resolver) Remember(ref model.RepoRef, path string) {
	if path == "" {
		return
	}
	r.store.SetRoute(repoKey(ref), path)
}

// RecordReview registra el worktree de un ítem como su review activo.
func (r *Resolver) RecordReview(it model.Item, rec cache.ReviewRecord) error {
	r.store.SetReview(itemKey(it.ID()), rec)
	return nil
}

// ActiveReview devuelve el review activo registrado para un ítem.
func (r *Resolver) ActiveReview(id model.ID) (cache.ReviewRecord, bool) {
	return r.store.Review(itemKey(id))
}

// WorktreePath devuelve la ruta destino del worktree de un ítem. Es la única
// fuente de rutas de worktree: el módulo worktree nunca las construye.
func (r *Resolver) WorktreePath(ref model.RepoRef, number int) string {
	return filepath.Join(
		r.worktreeDir,
		ref.Forge,
		ref.Host,
		filepath.FromSlash(ref.Project),
		fmt.Sprintf("prdash-pr-%d", number),
	)
}

// barePath devuelve la ruta del clon bare de un repo.
func (r *Resolver) barePath(ref model.RepoRef) string {
	return filepath.Join(r.cloneDir, ref.Forge, ref.Host, filepath.FromSlash(ref.Project))
}

// branchExists comprueba si una rama local existe en el repo.
func (r *Resolver) branchExists(ctx context.Context, repo, branch string) bool {
	_, err := r.git.Run(ctx, repo, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil
}

// buildIndex recorre los roots y construye el índice remoto→local. Poda
// directorios ocultos y los worktrees enlazados (su repo principal ya se
// indexa por sí mismo).
func (r *Resolver) buildIndex() map[string]string {
	idx := map[string]string{}
	ctx := context.Background()
	for _, root := range r.roots {
		abs, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		_ = filepath.WalkDir(abs, func(path string, d fs.DirEntry, err error) error {
			if err != nil || !d.IsDir() {
				return nil
			}
			if path != abs && strings.HasPrefix(d.Name(), ".") {
				return fs.SkipDir
			}
			if d.Name() == ".git" || isWorktreeMarker(path) {
				return fs.SkipDir
			}
			if !isGitRepo(path) {
				return nil
			}
			raw, err := r.git.Run(ctx, path, "remote", "get-url", "origin")
			if err != nil || raw == "" {
				return nil
			}
			ref, ok := r.parseRemote(raw)
			if !ok {
				return nil
			}
			key := repoKey(ref)
			if _, exists := idx[key]; !exists {
				idx[key] = path
			}
			return nil
		})
	}
	return idx
}

// ReviewBranch es el nombre de la rama local de review de un ítem.
func ReviewBranch(number int) string { return fmt.Sprintf("prdash/pr-%d", number) }

// ReviewRef devuelve el ref remoto de review de un ítem según su forge.
func ReviewRef(it model.Item) (string, bool) {
	switch it.Forge {
	case "github":
		return fmt.Sprintf("refs/pull/%d/head", it.Number), true
	case "gitlab":
		return fmt.Sprintf("refs/merge-requests/%d/head", it.Number), true
	default:
		return "", false
	}
}

// CloneURL arma la URL de clonado canónica de un repo (HTTPS), incluyendo el
// relative URL root de la instancia. prefix es el prefijo de subcarpeta del
// host (p. ej. "/git/" o "git" para un GitLab self-managed); vacío = raíz.
func CloneURL(ref model.RepoRef, prefix string) string {
	project := strings.TrimSuffix(ref.Project, ".git")
	host := strings.Trim(ref.Host, "/")
	base := strings.Trim(prefix, "/")
	if base == "" {
		return fmt.Sprintf("https://%s/%s.git", host, project)
	}
	return fmt.Sprintf("https://%s/%s/%s.git", host, base, project)
}

// ParseRemoteURL normaliza una URL remota de git a una referencia de repo.
// Reconoce la forma SCP (git@host:owner/repo) y las URLs con esquema. El forge
// se deduce del host con el mapa hosts; un host desconocido no se normaliza.
// prefixes mapea host → relative URL root de la instancia; el prefijo se quita
// del path para que un remoto en subcarpeta (https://host/git/grupo/proy.git)
// normalice al mismo Project que uno en la raíz.
func ParseRemoteURL(raw string, hosts map[string]string, prefixes map[string]string) (model.RepoRef, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return model.RepoRef{}, false
	}

	var host, path string
	if i := strings.Index(raw, "://"); i >= 0 {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			return model.RepoRef{}, false
		}
		host = u.Hostname()
		path = u.Path
	} else if at := strings.Index(raw, "@"); at >= 0 {
		rest := raw[at+1:]
		colon := strings.Index(rest, ":")
		if colon < 0 {
			return model.RepoRef{}, false
		}
		host = rest[:colon]
		path = rest[colon+1:]
	} else {
		return model.RepoRef{}, false
	}

	path = strings.Trim(strings.TrimSuffix(strings.TrimSpace(path), ".git"), "/")
	if prefix := strings.Trim(prefixes[host], "/"); prefix != "" {
		path = strings.TrimPrefix(path, prefix+"/")
	}
	parts := strings.Split(path, "/")
	if host == "" || len(parts) < 2 {
		return model.RepoRef{}, false
	}
	for _, p := range parts {
		if p == "" {
			return model.RepoRef{}, false
		}
	}
	forge, ok := hosts[host]
	if !ok || forge == "" {
		return model.RepoRef{}, false
	}
	return model.RepoRef{
		Forge:   forge,
		Host:    host,
		Project: strings.Join(parts, "/"),
		Owner:   parts[len(parts)-2],
		Name:    parts[len(parts)-1],
	}, true
}

func repoKey(ref model.RepoRef) string {
	return ref.Forge + "/" + ref.Host + "/" + ref.Project
}

func itemKey(id model.ID) string {
	return id.Forge + "/" + id.Host + "/" + id.Project + "#" + strconv.Itoa(id.Number)
}

// isRepo reconoce un repo git normal o un clon bare por su estructura.
func isRepo(path string) bool {
	if _, err := os.Lstat(filepath.Join(path, ".git")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(path, "HEAD")); err != nil {
		return false
	}
	info, err := os.Stat(filepath.Join(path, "objects"))
	return err == nil && info.IsDir()
}

// isGitRepo reconoce un repo con `.git` como directorio (no un worktree).
func isGitRepo(path string) bool {
	info, err := os.Lstat(filepath.Join(path, ".git"))
	return err == nil && info.IsDir()
}

// isWorktreeMarker reconoce un worktree enlazado (`.git` es un fichero).
func isWorktreeMarker(path string) bool {
	info, err := os.Lstat(filepath.Join(path, ".git"))
	return err == nil && !info.IsDir()
}
