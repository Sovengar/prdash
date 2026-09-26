// Parseo de la conversación de un ítem: los comentarios de GitHub y las notas de
// GitLab llegan con nombres de campo distintos pero con la misma forma underneath,
// así que se normalizan en un único tipo y una única función de conversión.
package parse

import (
	"encoding/json"
	"strings"

	"prdash/internal/forge/model"
)

// unknownAuthor es lo que se pone cuando el forge no devuelve autor: pasa cuando
// la cuenta que escribió se borró. Inventar un nombre sería peor que decir que no
// se sabe quién fue.
const unknownAuthor = "unknown"

// commentNode es la forma común de un comentario en los dos GraphQL. El autor
// llega como `login` en GitHub y como `username` en GitLab; `system` solo lo
// manda GitLab, y en GitHub se queda en su cero, que es justo lo que hace que el
// filtro compartido no tire nada.
//
// Los punteros de Comments/Notes y Nodes separan "la query no lo pidió" de
// "vino vacío": el primero significa que la respuesta no traía conversación, y
// pintarlo como "no hay comentarios" sería mentira.
type commentNode struct {
	Author struct {
		Login    string `json:"login"`
		Username string `json:"username"`
	} `json:"author"`
	Body      string `json:"body"`
	CreatedAt string `json:"createdAt"`
	System    bool   `json:"system"`
}

// author devuelve el login del autor sea cual sea el forge.
func (n commentNode) author() string {
	if n.Author.Login != "" {
		return n.Author.Login
	}
	if n.Author.Username != "" {
		return n.Author.Username
	}
	return unknownAuthor
}

// comment normaliza un nodo a un comentario del modelo. La marca de sistema se
// queda en el modelo y no se filtra aquí: el filtro depende de cuántos nodos se
// pidieron, que es cosa del adapter, no del formato de la respuesta.
func (n commentNode) comment() model.Comment {
	return model.Comment{
		Author:    n.author(),
		Body:      n.Body,
		CreatedAt: parseTime(n.CreatedAt),
	}
}

type ghCommentsResp struct {
	Data struct {
		Repository *struct {
			PullRequest *struct {
				Comments *struct {
					TotalCount int           `json:"totalCount"`
					Nodes      []commentNode `json:"nodes"`
				} `json:"comments"`
			} `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// ParseGHComments lee los comentarios de la conversación de un PR de GitHub
// (`repository.pullRequest.comments`). Devuelve los comentarios y cuántos hay en
// total: el total es lo que permite decir "5 de 23" y saber que la ficha se está
// quedando cosas, en vez de dar la impresión de que el PR solo tiene cinco
// comentarios.
func ParseGHComments(raw string) ([]model.Comment, int, error) {
	var resp ghCommentsResp
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return nil, 0, &Error{Tool: "gh-comments", Msg: "invalid JSON", Err: err}
	}
	if len(resp.Errors) > 0 {
		return nil, 0, &Error{Tool: "gh-comments", Msg: resp.Errors[0].Message}
	}
	pr := resp.Data.Repository
	if pr == nil || pr.PullRequest == nil || pr.PullRequest.Comments == nil {
		return nil, 0, &Error{Tool: "gh-comments", Msg: "response without comments"}
	}
	c := pr.PullRequest.Comments
	return toComments(c.Nodes), c.TotalCount, nil
}

type glCommentsResp struct {
	Data struct {
		Project *struct {
			MergeRequest *struct {
				Notes *struct {
					Nodes []commentNode `json:"nodes"`
				} `json:"notes"`
			} `json:"mergeRequest"`
		} `json:"project"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// ParseGLComments lee las notas de la conversación de un MR de GitLab
// (`project.mergeRequest.notes`). GitLab no expone un recuento de notas, así que
// el total que se devuelve es el de las leídas: el detalle solo lo usa para
// decir "5 de N" cuando hay más de las que caben, y con N igual a lo que se leyó
// esa línea no llega a aparecer nunca.
func ParseGLComments(raw string) ([]model.Comment, int, error) {
	var resp glCommentsResp
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return nil, 0, &Error{Tool: "gl-comments", Msg: "invalid JSON", Err: err}
	}
	if len(resp.Errors) > 0 {
		return nil, 0, &Error{Tool: "gl-comments", Msg: resp.Errors[0].Message}
	}
	mr := resp.Data.Project
	if mr == nil || mr.MergeRequest == nil || mr.MergeRequest.Notes == nil {
		return nil, 0, &Error{Tool: "gl-comments", Msg: "response without notes"}
	}
	notes := toComments(mr.MergeRequest.Notes.Nodes)
	return notes, len(notes), nil
}

// toComments convierte una lista de nodos en comentarios, descartando las notas
// de sistema de GitLab ("assigned to @x", "added 3 commits", "changed the
// description"). No son conversación: son el historial de acciones del MR, y
// mezclado con lo que escribió la gente se come las cinco filas de la ficha con
// ruido que ya está en el detalle de otra forma.
func toComments(nodes []commentNode) []model.Comment {
	out := make([]model.Comment, 0, len(nodes))
	for _, n := range nodes {
		if n.System {
			continue
		}
		out = append(out, n.comment())
	}
	return out
}

// isHTMLComment indica si una línea es un comentario HTML de markdown. Los bots
// los ponen delante de su texto (`<!-- ssf: origin=… -->`) y son la primera línea
// de una parte de las conversaciones de GitHub, así que quien elija la línea
// representativa tiene que saltárselos o acabaría leyendo metadata invisible.
func isHTMLComment(line string) bool {
	t := strings.TrimSpace(line)
	return strings.HasPrefix(t, "<!--") && strings.HasSuffix(t, "-->")
}

// CommentLines parte el cuerpo de un comentario en las líneas con contenido, en el
// orden en que las escribió el autor.
//
// Se saltan dos cosas que en una fila de ancho fijo son pura pérdida: los
// comentarios HTML de markdown, que en GitHub son el boilerplate invisible de los
// bots, y los blancos de separación, que en un panel son filas enteras por un hueco
// entre párrafos.
//
// NO se aplanan los saltos duros dentro del cuerpo. Un párrafo que el autor partió
// con dos espacios al final sigue siendo un párrafo con dos partes, y pegar el
// final de una con el principio de la siguiente produce "fix the timeout fix the
// backoff", que no dice nada. También se conservan los guiones de una lista: quitarlos
// convertiría tres elementos en una frase sin verbos.
func CommentLines(body string) []string {
	out := make([]string, 0, 8)
	for _, raw := range strings.Split(body, "\n") {
		if isHTMLComment(raw) {
			continue
		}
		if flat := strings.Join(strings.Fields(raw), " "); flat != "" {
			out = append(out, flat)
		}
	}
	return out
}
