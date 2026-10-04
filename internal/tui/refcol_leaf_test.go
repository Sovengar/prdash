package tui

import (
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

// The leaf of a project path is what comes AFTER the last separator.
func TestRefLeafCortaPorElUltimoSeparador(t *testing.T) {
	casos := []struct {
		proyecto string
		numero   int
		want     string
	}{
		{"acme/widget", 7, "widget#7"},
		{"grupo/sub/proyecto", 12, "proyecto#12"},
		{"a/b/c/d", 1, "d#1"},
		{"widget", 7, "widget#7"},
		{"", 7, "#7"},
		{"/proyecto", 7, "proyecto#7"},
		{"/a/b", 3, "b#3"},
		{"/", 7, "#7"},
		{"a/", 7, "#7"},
		{"acme/widget", 0, "widget#0"},
		{"acme/widget", -1, "widget#-1"},
		{"acme/widget", 12345, "widget#12345"},
	}

	for _, c := range casos {
		got := refLeaf(model.Item{Number: c.numero, Ref: model.RepoRef{Project: c.proyecto}})
		if got != c.want {
			t.Errorf("refLeaf(%q, %d) = %q, want %q", c.proyecto, c.numero, got, c.want)
		}
	}

	// And the leaf NEVER comes out with a slash: the leaf of a path is not a path.
	for _, proyecto := range []string{"a", "a/b", "a/b/c", "/a", "/a/b", "/"} {
		hoja := refLeaf(model.Item{Number: 1, Ref: model.RepoRef{Project: proyecto}})
		quedan := strings.TrimSuffix(hoja, "#1")
		if strings.Contains(quedan, "/") {
			t.Errorf("refLeaf(%q) dio %q: la hoja de una ruta no lleva barras", proyecto, hoja)
		}
	}

	// And the number is always there, because without it the cell does not identify the item.
	for _, proyecto := range []string{"", "a", "a/b", "/a"} {
		hoja := refLeaf(model.Item{Number: 42, Ref: model.RepoRef{Project: proyecto}})
		if !strings.HasSuffix(hoja, "#42") {
			t.Errorf("refLeaf(%q) dio %q, want el número detrás: sin él la celda no identifica el ítem",
				proyecto, hoja)
		}
	}
}

// The prefix has to be a strict directory for ALL of them.
func TestSectionPrefixConProyectosDeProfundidadDistinta(t *testing.T) {
	casos := []struct {
		nombre    string
		proyectos []string
		want      string
	}{
		{"misma profundidad", []string{"acme/a", "acme/b"}, "acme"},
		{"profundidad distinta", []string{"a/b/c", "a/b/d"}, "a/b"},
		{"uno sin barras", []string{"widget", "acme/a"}, ""},
		{"uno sin barras primero", []string{"widget", "acme/a"}, ""},
		{"los dos sin barras", []string{"a", "b"}, ""},
		// Tres profundidades: el mínimo manda.
		{"tres profundidades", []string{"x/y/z", "x/y", "x/y"}, "x"},
		// And the prefix has NO trailing slash: it is a prefix, not a path, and whoever joins it adds the
		// separator.
		{"el prefijo no lleva barra final", []string{"a/b/c", "a/b/d"}, "a/b"},
		{"un solo segmento comun", []string{"a/b", "a/c"}, "a"},
		// Y sin nada común, prefijo vacío: la celda lleva la referencia entera.
		{"nada comun", []string{"a/b", "c/d"}, ""},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			items := make([]model.Item, len(c.proyectos))
			for i, p := range c.proyectos {
				items[i] = model.Item{Number: i + 1, Ref: model.RepoRef{Project: p}}
			}
			if got := sectionPrefix(items); got != c.want {
				t.Errorf("sectionPrefix(%v) = %q, want %q", c.proyectos, got, c.want)
			}
		})
	}

	// And with a single item there is no prefix: nobody to share it with, and putting the name there
	// would duplicate the row.
	if got := sectionPrefix([]model.Item{{Ref: model.RepoRef{Project: "acme/widget"}}}); got != "" {
		t.Errorf("sectionPrefix con un item dio %q, want vacío", got)
	}
}
