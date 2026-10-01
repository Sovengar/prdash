package reporesolver

import (
	"testing"

	"prdash/internal/forge/model"
)

// TestParseRemoteURLClasificaLasFormasAntesDeNormalizar: un remoto se clasifica en
// una de TRES formas, y solo en una de tres.
//
// La clasificación es lo primero que hace la función, y de ella sale la lista de
// formas que se aceptan:
//
//   - con esquema: algo://host/ruta
//   - SCP: usuario@host:ruta
//   - y nada más
//
// Lo que se afirma aquí es la frontera entre las tres, no el resultado. Y la razón de
// que merezca un test propio es que las tres condiciones se detectan con
// strings.Index sobre un separador, y un separador puede aparecer en un sitio que no
// es donde debe.
//
// El caso que de verdad importa es el del "@" en la posición 0. Un "@" al principio
// parece un SCP, y con un ">= 0" se aceptaría como usuario vacío. Con un "> 0" no
// entra: un remoto sin usuario no es un remoto de git, y normalizarlo produce un
// repo que no existe. El test lo dice porque esa es la decisión, y una decisión que
// no está escrita en un test la cambia el que la lea sin querer.
//
// Y el del "://" en la posición 0 es el espejo: un esquema vacío NO puede colarse por
// la rama de SCP, porque si se cuela, una basura se normaliza a un repo real y
// prdash luego intenta clonarlo. Con "://" al principio, url.Parse lo rechaza por
// esquema ausente y la entrada cae sola; lo que no puede pasar es que la entrada
// esquive esa rama y acabe aceptada por la otra.
func TestParseRemoteURLClasificaLasFormasAntesDeNormalizar(t *testing.T) {
	hosts := map[string]string{"github.com": "github", "gitlab.example.com": "gitlab"}

	casos := []struct {
		nombre string
		raw    string
		ok     bool
	}{
		// Las dos formas buenas, que tienen que seguir funcionando.
		{"con esquema https", "https://github.com/acme/widget.git", true},
		{"con esquema ssh", "ssh://git@github.com/acme/widget.git", true},
		{"scp", "git@github.com:acme/widget.git", true},
		// Y con un usuario de UNA columna, que es donde el "@" está justo en el
		// borde de "tiene que haber algo antes". Un "> 1" en vez de un "> 0" lo
		// rechazaría, y un usuario de una letra es legal en git.
		{"scp con usuario de una columna", "a@github.com:acme/widget.git", true},

		// "@" en la posición 0: usuario vacío. No es un remoto de git.
		{"scp con usuario vacío", "@github.com:acme/widget.git", false},
		{"scp con solo arroba", "@", false},
		{"arroba al principio y dos puntos", "@github.com:acme/widget", false},

		// "://" en la posición 0: esquema vacío. No puede entrar por SCP.
		{"esquema vacío con dos puntos", "://github.com:acme/widget", false},
		{"esquema vacío y arroba", "://git@github.com/acme/widget", false},
		{"solo esquema vacío", "://", false},

		// "@" sin dos puntos detrás: no es SCP, es otra cosa.
		{"arroba sin dos puntos", "git@github.com", false},
		{"arroba y barra", "git@github.com/acme/widget", false},
		{"arroba y nada", "git@", false},

		// Ni "@" ni "://": un path local o cualquier otra cosa.
		{"path local", "/home/u/dev/widget", false},
		{"path relativo", "../widget", false},
		{"solo dos puntos", "acme:widget", false},
		{"solo una palabra", "widget", false},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got, ok := ParseRemoteURL(c.raw, hosts, nil)
			if ok != c.ok {
				t.Fatalf("ParseRemoteURL(%q) dio ok=%v, quiero %v (ref %+v)", c.raw, ok, c.ok, got)
			}
			// Y cuando dice que no, no devuelve medio repo con datos: unRepoRef a
			// medias se parece a uno bueno en los logs y se parece MUCHO en el
			// código que decide si algo ya está resuelto.
			if !ok && got != (model.RepoRef{}) {
				t.Errorf("ParseRemoteURL(%q) dijo que no pero devolvió %+v", c.raw, got)
			}
		})
	}
}

// TestParseRemoteURLNoSeConfundeConUnSeparadorEnElSitioEquivocado: los separadores
// que se buscan pueden aparecer antes de donde corresponde.
//
// Es el otro lado de la clasificación, y son los casos que un remoto real produce
// más de lo que uno pensaría:
//
//   - un "@" en el RUTA de una URL con esquema, que no convierte nada porque la URL
//     con esquema se resuelve entera antes de mirar el "@";
//   - un ":" en el host de una URL con esquema (el puerto), que no la convierte en
//     SCP;
//   - un "@" en la ruta de una URL con esquema, que es legal en una ruta y no es un
//     usuario.
//
// La razón de que esto sea una propriedade y no un caso suelto es que el orden de
// las comprobaciones es lo que decide: si se mirara el "@" antes que el "://", un
// remoto de Bitbucket con un "@" en la ruta se leería como usuario.
func TestParseRemoteURLNoSeConfundeConUnSeparadorEnElSitioEquivocado(t *testing.T) {
	hosts := map[string]string{"github.com": "github"}

	// Cada entrada con su resultado exacto, porque lo que se afirma es que el
	// separador de más se queda DENTRO de la ruta y no se come un trozo de ella.
	casos := []struct {
		raw  string
		want model.RepoRef
	}{
		// Con esquema: gana el esquema, y el "@" que lleva es el usuario, no un
		// separador de formato.
		{"https://git@github.com/acme/widget.git",
			model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget", Owner: "acme", Name: "widget"}},
		// Un "@" dentro de la ruta es legal en una ruta de git y NO es un usuario.
		// Por eso el repo se llama así, con el "@" dentro, y no se parte en dos.
		{"https://github.com/acme/wi@get.git",
			model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/wi@get", Owner: "acme", Name: "wi@get"}},
		// Con puerto en el host: el puerto se queda con el host y no se lee como
		// separador SCP. Y el host sin puerto es el que se busca en el mapa.
		{"ssh://git@github.com:22/acme/widget.git",
			model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget", Owner: "acme", Name: "widget"}},
		// Un ":" dentro de la ruta de un SCP: el separador es el PRIMERO, y los
		// siguientes son parte del nombre del repo.
		{"git@github.com:acme/wi:get.git",
			model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/wi:get", Owner: "acme", Name: "wi:get"}},
	}

	for _, c := range casos {
		got, ok := ParseRemoteURL(c.raw, hosts, nil)
		if !ok {
			t.Errorf("ParseRemoteURL(%q) dijo que no, y es una de las tres formas", c.raw)
			continue
		}
		if got != c.want {
			t.Errorf("ParseRemoteURL(%q) dio %+v, want %+v: un separador de más no es un separador de menos",
				c.raw, got, c.want)
		}
	}
}
