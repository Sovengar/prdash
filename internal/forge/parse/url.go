// Parseo puro de URLs de review a referencias de repo. Lo usa el link handler
// del plugin para resolver un Ctrl+click sobre una URL de PR/MR al ítem que se
// va a montar. No toca red, disco ni el forge.
package parse

import (
	"net/url"
	"strconv"
	"strings"

	"prdash/internal/forge/model"
)

// ParseReviewURL traduce una URL de PR de GitHub
// (`https://<host>/<owner>/<repo>/pull/<n>`) o de MR de GitLab
// (`https://<host>/<grupo>/…/<proyecto>/-/merge_requests/<n>`) a la referencia
// del repo y su número.
//
// hosts mapea host → nombre de forge (p. ej. github.com → "github"); si el host
// no está en el mapa, el forge se deduce por la forma de la URL. Devuelve ok
// false si la URL no es de review o le falta el número.
func ParseReviewURL(raw string, hosts map[string]string) (model.RepoRef, int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return model.RepoRef{}, 0, false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return model.RepoRef{}, 0, false
	}
	host := u.Hostname()

	segments := splitPath(u.Path)
	if len(segments) < 4 {
		return model.RepoRef{}, 0, false
	}

	// GitLab: …/-/merge_requests/<n>; GitHub: …/pull/<n>.
	if i := indexOf(segments, "-"); i >= 0 && i+2 < len(segments) && segments[i+1] == "merge_requests" {
		number, ok := numberAt(segments, i+2)
		if !ok || i < 2 {
			return model.RepoRef{}, 0, false
		}
		return repoRef(host, segments[:i], hosts, "gitlab"), number, true
	}
	if i := indexOf(segments, "pull"); i >= 0 && i+1 < len(segments) && i >= 2 {
		number, ok := numberAt(segments, i+1)
		if !ok {
			return model.RepoRef{}, 0, false
		}
		return repoRef(host, segments[:i], hosts, "github"), number, true
	}
	return model.RepoRef{}, 0, false
}

// repoRef compone la referencia a partir de los segmentos de proyecto.
func repoRef(host string, project []string, hosts map[string]string, fallback string) model.RepoRef {
	forge := fallback
	if name, ok := hosts[host]; ok && name != "" {
		forge = name
	}
	path := strings.Join(project, "/")
	owner := ""
	name := ""
	if len(project) > 0 {
		owner = project[len(project)-1]
		name = project[len(project)-1]
	}
	if len(project) >= 2 {
		owner = project[len(project)-2]
	}
	return model.RepoRef{Forge: forge, Host: host, Project: path, Owner: owner, Name: name}
}

// splitPath parte el path de una URL en segmentos no vacíos.
func splitPath(path string) []string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	out := parts[:0]
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// indexOf devuelve el índice del primer segmento igual a want.
func indexOf(segments []string, want string) int {
	for i, s := range segments {
		if s == want {
			return i
		}
	}
	return -1
}

// numberAt devuelve el entero del segmento en i.
func numberAt(segments []string, i int) (int, bool) {
	if i >= len(segments) {
		return 0, false
	}
	n, err := strconv.Atoi(segments[i])
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}
