package parse

import (
	"strings"
	"testing"
)

// TestParseGHComments: la conversación de GitHub llega en la conexión que se
// pidió, con el total aparte para poder decir "5 de 23".
func TestParseGHComments(t *testing.T) {
	raw := `{"data":{"repository":{"pullRequest":{"comments":{
		"totalCount":23,
		"nodes":[
			{"author":{"login":"alice"},"body":"please add a test","createdAt":"2026-09-20T10:00:00Z"},
			{"author":{"login":"bob"},"body":"nit: typo","createdAt":"2026-09-21T11:30:00Z"}
		]}}}}}`

	comments, total, err := ParseGHComments(raw)
	if err != nil {
		t.Fatalf("ParseGHComments: %v", err)
	}
	if total != 23 {
		t.Errorf("total = %d, want 23", total)
	}
	if len(comments) != 2 {
		t.Fatalf("len = %d, want 2", len(comments))
	}
	if comments[0].Author != "alice" || comments[1].Author != "bob" {
		t.Errorf("autores = %q/%q, want alice/bob", comments[0].Author, comments[1].Author)
	}
	if comments[0].Body != "please add a test" {
		t.Errorf("cuerpo = %q", comments[0].Body)
	}
	if got := comments[1].CreatedAt.Format("2006-01-02"); got != "2026-09-21" {
		t.Errorf("fecha = %q, want 2026-09-21", got)
	}
}

// TestParseGHCommentsDeletedAuthor: una cuenta borrada deja el autor a null. Se
// dice "unknown" en vez de dejar el hueco en blanco, que en la ficha se leería
// como un comentario sin autor.
func TestParseGHCommentsDeletedAuthor(t *testing.T) {
	raw := `{"data":{"repository":{"pullRequest":{"comments":{
		"totalCount":1,
		"nodes":[{"author":null,"body":"gone","createdAt":"2026-09-20T10:00:00Z"}]
	}}}}}`

	comments, _, err := ParseGHComments(raw)
	if err != nil {
		t.Fatalf("ParseGHComments: %v", err)
	}
	if comments[0].Author != "unknown" {
		t.Errorf("autor = %q, want %q", comments[0].Author, unknownAuthor)
	}
}

// TestParseGHCommentsErrors: una respuesta sin la conexión de comentarios no es
// "este PR no tiene comentarios", es una respuesta que no se entiende. Sin
// distinguirlas, un cambio en la query se paintaría como una conversación vacía.
func TestParseGHCommentsErrors(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"json inválido", `{"data":`, "invalid JSON"},
		{"error del forge", `{"errors":[{"message":"Field 'comments' doesn't exist"}]}`, "doesn't exist"},
		{"sin pull request", `{"data":{"repository":{}}}`, "without comments"},
		{"sin comments", `{"data":{"repository":{"pullRequest":{}}}}`, "without comments"},
		{"respuesta vacía", `{}`, "without comments"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			comments, total, err := ParseGHComments(tc.raw)
			if err == nil {
				t.Fatalf("debería fallar, devolvió %d comentarios", len(comments))
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want que contenga %q", err, tc.want)
			}
			if total != 0 {
				t.Errorf("un fallo no debe inventar un total: %d", total)
			}
		})
	}
}

// TestParseGLComments: las notas de GitLab traen el autor como `username` y una
// marca `system` que no existe en GitHub.
func TestParseGLComments(t *testing.T) {
	raw := `{"data":{"project":{"mergeRequest":{"notes":{"nodes":[
		{"author":{"username":"alice"},"body":"ok for me","createdAt":"2026-09-20T10:00:00Z","system":false},
		{"author":{"username":null},"body":"assigned to @bob","createdAt":"2026-09-20T10:01:00Z","system":true}
	]}}}}}`

	comments, total, err := ParseGLComments(raw)
	if err != nil {
		t.Fatalf("ParseGLComments: %v", err)
	}
	if len(comments) != 1 || comments[0].Author != "alice" {
		t.Fatalf("notas = %+v, want solo la de alice", comments)
	}
	if total != 1 {
		t.Errorf("total = %d, want 1 (el de las leídas, GitLab no expone recuento)", total)
	}
}

// TestParseGLCommentsDropsSystemNotes: "assigned to @x" y "added 3 commits" son
// el historial de acciones del MR, no conversación. Si se colaran, se comerían las
// cinco filas de la ficha con ruido que ya está en otra parte del detalle.
func TestParseGLCommentsDropsSystemNotes(t *testing.T) {
	raw := `{"data":{"project":{"mergeRequest":{"notes":{"nodes":[
		{"author":{"username":null},"body":"mentioned in commit abc","createdAt":"2026-09-20T10:00:00Z","system":true},
		{"author":{"username":"alice"},"body":"first","createdAt":"2026-09-20T10:01:00Z","system":false},
		{"author":{"username":null},"body":"added 56 commits","createdAt":"2026-09-20T10:02:00Z","system":true},
		{"author":{"username":"bob"},"body":"second","createdAt":"2026-09-20T10:03:00Z","system":false},
		{"author":{"username":null},"body":"changed the description","createdAt":"2026-09-20T10:04:00Z","system":true}
	]}}}}}`

	comments, _, err := ParseGLComments(raw)
	if err != nil {
		t.Fatalf("ParseGLComments: %v", err)
	}
	if len(comments) != 2 {
		t.Fatalf("len = %d, want 2 (las dos escritas por personas)", len(comments))
	}
	if comments[0].Body != "first" || comments[1].Body != "second" {
		t.Errorf("se colaron notas de sistema: %+v", comments)
	}
}

// TestParseGLCommentsKeepsEmptySystemFlag: GitHub no manda `system` y el campo
// llega ausente, que es su cero. El filtro compartido no debe confundir ausente
// con system=true y tirar los comentarios de GitHub.
func TestParseGLCommentsKeepsEmptySystemFlag(t *testing.T) {
	raw := `{"data":{"project":{"mergeRequest":{"notes":{"nodes":[
		{"author":{"username":"alice"},"body":"sin el campo system","createdAt":"2026-09-20T10:00:00Z"}
	]}}}}}`

	comments, _, err := ParseGLComments(raw)
	if err != nil {
		t.Fatalf("ParseGLComments: %v", err)
	}
	if len(comments) != 1 {
		t.Fatalf("len = %d, want 1: `system` ausente es false, no system", len(comments))
	}
}

func TestParseGLCommentsErrors(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"json inválido", `nope`, "invalid JSON"},
		{"error del forge", `{"errors":[{"message":"Field 'notes' doesn't exist on type"}]}`, "doesn't exist"},
		{"sin merge request", `{"data":{"project":{}}}`, "without notes"},
		{"sin notes", `{"data":{"project":{"mergeRequest":{}}}}`, "without notes"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := ParseGLComments(tc.raw); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want que contenga %q", err, tc.want)
			}
		})
	}
}

// TestCommentLines: el cuerpo se parte en las líneas con contenido, en el orden
// del autor. Los bots de GitHub abren con un comentario HTML invisible y los blancos
// de separación son filas enteras en un panel, así que los dos se van. Los párrafos
// NO se pegan entre sí: "fix the timeout fix the backoff" no dice nada.
func TestCommentLines(t *testing.T) {
	cases := []struct {
		name string
		body string
		want []string
	}{
		{"una línea", "please add a test", []string{"please add a test"}},
		{
			"con comentario HTML delante",
			"<!-- ssf: origin=o/r#553 -->\n\nssf attaching agent",
			[]string{"ssf attaching agent"},
		},
		{"con blancos", "\n\n   \nfirst\n\nsecond\n", []string{"first", "second"}},
		{
			"párrafos separados",
			"line one\n\nline two",
			[]string{"line one", "line two"},
		},
		{
			"lista conserva los guiones",
			"- fix the timeout\n- fix the backoff",
			[]string{"- fix the timeout", "- fix the backoff"},
		},
		{
			"el heading conserva su marcado",
			"### Fixes aplicados\n\n**HIGH** doble prefijo",
			[]string{"### Fixes aplicados", "**HIGH** doble prefijo"},
		},
		{"espacios y tabs dentro de la línea", "a\t\tb   c", []string{"a b c"}},
		{"solo boilerplate", "<!-- x -->\n\n<!-- y -->", nil},
		{"vacío", "", nil},
		{"solo espacios", "   \n\t", nil},
		{"unicode", "el timeout son 30× más alto 🚀", []string{"el timeout son 30× más alto 🚀"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CommentLines(tc.body)
			if len(got) != len(tc.want) {
				t.Fatalf("CommentLines(%q) = %q, want %q", tc.body, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("línea %d = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}
