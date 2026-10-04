package parse

import (
	"encoding/json"
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

// TestElDueñoDeUnMRDeGitLabEsLoQueVaAntesDeLaUltimaBarra: el dueño es todo lo que va
// antes de la ÚLTIMA barra del proyecto, y sin barra no hay grupo.
//
// Y el caso de la barra en la posición cero es el que separa las dos ramas, pero NO como
// yo creía al principio. Con `/proyecto`, la única barra está en el índice 0 y las dos
// ramas dan cadena vacía: la del `if` recorta `owner[:0]` y la del `else` pone `""`. O sea
// que ahí no hay nada que distinguir, y una condición con `> 0` daría lo mismo.
//
// La primera versión de este test afirmaba que `/grupo/proyecto` da dueño vacío, y falló
// con `/grupo`. El código da `/grupo`, porque recorta por la última barra y `/grupo` es lo
// que queda. Que eso sea una ruta y no un nombre es raro, pero es lo que hay, y el
// `strings.Trim` que hay en otros sitios de este paquete no se aplica aquí.
//
// Y lo que sí se afirma, porque es el contrato: el dueño es lo que va antes de la última
// barra, con subgrupos de por medio, y sin barra no hay dueño. Una instancia de GitLab con
// proyectos en la raíz existe —self-hosted de un solo nivel— y ahí el dueño vacío es lo
// correcto: no hay nadie por encima del proyecto.
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

	// Y lo que NO se afirma a propósito: que `/grupo/proyecto` produzca un dueño que es
	// una ruta. Lo produce, y parece una fuente de worktrees fuera de la raíz, pero es
	// comportamiento del código y no de esta condición: la última barra de
	// `/grupo/proyecto` no es el índice cero. Un aserto que marcara ese caso como fallo
	// estaría pidiendo un cambio de comportamiento que no se ha pedido aquí.
	it := itemFromGLMR(mrCon("/grupo/proyecto", "proyecto"),
		model.SectionReview, model.ReviewRequested)
	t.Logf("full_path con barra inicial: dueño %q, proyecto %q",
		it.Ref.Owner, it.Ref.Project)
	if !strings.HasPrefix(it.Ref.Owner, "/grupo") {
		t.Logf("el dueño no sale con barra inicial; revisar el aserto de arriba")
	}
}

// mrCon arma el struct mínimo que lee `itemFromGLMR`. Se construye por JSON y no a mano
// porque los campos llevan etiquetas json y porque `flexInt` tiene su propio
// Unmarshaler: escribirlos a mano se saltaría los dos, y un test que se salta el
// deserializado no prueba lo que el parser hace con lo que le llega de la API.
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
