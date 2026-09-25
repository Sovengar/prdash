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
// no está en el mapa, el forge se deduce por la forma de la URL. prefixes mapea
// host → relative URL root de la instancia: un host servido en subcarpeta se
// normaliza al MISMO Project que la vía API (sin el prefijo). Devuelve ok false
// si la URL no es de review o le falta el número.
func ParseReviewURL(raw string, hosts map[string]string, prefixes map[string]string) (model.RepoRef, int, bool) {
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
		if !ok {
			return model.RepoRef{}, 0, false
		}
		ref, ok := repoRef(host, segments[:i], hosts, prefixes, "gitlab")
		return ref, number, ok
	}
	if i := indexOf(segments, "pull"); i >= 0 && i+1 < len(segments) {
		number, ok := numberAt(segments, i+1)
		if !ok {
			return model.RepoRef{}, 0, false
		}
		ref, ok := repoRef(host, segments[:i], hosts, prefixes, "github")
		return ref, number, ok
	}
	return model.RepoRef{}, 0, false
}

// repoRef compone la referencia a partir de los segmentos de proyecto, quitando
// primero el relative URL root de la instancia para que la identidad coincida
// con la de la vía API.
func repoRef(host string, project []string, hosts map[string]string, prefixes map[string]string, fallback string) (model.RepoRef, bool) {
	project = stripPrefix(project, prefixes[host])
	if len(project) < 2 {
		return model.RepoRef{}, false
	}
	forge := fallback
	if name, ok := hosts[host]; ok && name != "" {
		forge = name
	}
	return model.RepoRef{
		Forge:   forge,
		Host:    host,
		Project: strings.Join(project, "/"),
		Owner:   project[len(project)-2],
		Name:    project[len(project)-1],
	}, true
}

// stripPrefix quita el relative URL root de la instancia de los segmentos del
// proyecto (p. ej. "git" de [git sub proj] → [sub proj]). prefixes puede ser
// multi-segmento ("git/repos") y solo se quita si coincide el prefijo completo.
func stripPrefix(project []string, prefix string) []string {
	parts := splitPath(prefix)
	if len(parts) == 0 || len(project) < len(parts) {
		return project
	}
	for i, p := range parts {
		if project[i] != p {
			return project
		}
	}
	return project[len(parts):]
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
