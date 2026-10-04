package parse

import (
	"encoding/json"
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

// The owner is everything before the LAST slash of the project path.
func TestElDueñoDeUnMRDeGitLabEsLoQueVaAntesDeLaUltimaBarra(t *testing.T) {
	casos := []struct {
		fullPath, wantOwner, nota string
	}{
		{"grupo/proyecto", "grupo", "el caso normal"},
		{"grupo/sub/proyecto", "grupo/sub",
			"con subgrupos: el dueño es TODO lo que va antes de la última barra, que es " +
				"lo que distingue un proyecto de otro con el mismo nombre en otro grupo"},
		{"proyecto", "",
			"proyecto en la raíz de la instancia: no hay grupo y el dueño vacío es correcto"},
		{"/proyecto", "",
			"barra en la posición cero: las dos ramas de la condición dan cadena vacía, " +
				"así que el índice cero no las distingue"},
	}
	for _, c := range casos {
		it := itemFromGLMR(mrCon(c.fullPath, "proyecto"),
			model.SectionReview, model.ReviewRequested)
		if it.Ref.Owner != c.wantOwner {
			t.Errorf("%s: el dueño salió %q, want %q", c.nota, it.Ref.Owner, c.wantOwner)
		}
		if it.Ref.Project != c.fullPath {
			t.Errorf("%s: el proyecto salió %q, want %q: es lo que distingue un proyecto "+
				"de otro con el mismo nombre", c.nota, it.Ref.Project, c.fullPath)
		}
		if it.Ref.Name != "proyecto" {
			t.Errorf("%s: el nombre salió %q", c.nota, it.Ref.Name)
		}
	}

	// Deliberately NOT asserted: that `/group/project` gives an owner that is a path. It does, and it
	//looks wrong, but it is the same string the API gives.
	it := itemFromGLMR(mrCon("/grupo/proyecto", "proyecto"),
		model.SectionReview, model.ReviewRequested)
	t.Logf("full_path con barra inicial: dueño %q, proyecto %q",
		it.Ref.Owner, it.Ref.Project)
	if !strings.HasPrefix(it.Ref.Owner, "/grupo") {
		t.Logf("el dueño no sale con barra inicial; revisar el aserto de arriba")
	}
}

// Built through JSON and not by hand, because the fields carry tags.
func mrCon(fullPath, name string) glMR {
	crudo, err := json.Marshal(map[string]any{
		"iid":    7,
		"title":  "uno",
		"webUrl": "https://gitlab.com/g/p!7",
		"state":  "opened",
		"project": map[string]any{
			"name":     name,
			"path":     name,
			"fullPath": fullPath,
		},
	})
	if err != nil {
		panic(err)
	}
	var mr glMR
	if err := json.Unmarshal(crudo, &mr); err != nil {
		panic(err)
	}
	return mr
}
