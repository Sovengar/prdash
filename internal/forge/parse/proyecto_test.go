package parse

import (
	"strings"
	"testing"
	"time"
)

// Estas cuatro funciones son la traducción entre "como lo escribe la API" y "como lo usa
// prdash", y son el tipo de código que se rompe por una entrada rara sin que salte nada: un
// proyecto con barras, una fecha en otro formato, un owner vacío.
//
// Y el patrón común es el mismo en las cuatro: **cada rama tiene una razón de existir que no
// se ve en el `switch`**. Por eso los casos raros van en tabla y no en un par de asserts, y
// porque el caso raro es el que se ejecuta en producción.

// TestJoinProjectNoPoneSeparadoresHuerfanos: la composición de "owner/repo".
//
// Y las tres ramas cubren tres formas de dato incompleto que llegan de verdad:
//
//   - Sin owner: un proyecto que solo trae nombre. GitLab devuelve proyectos de grupo con
//     el path completo y GitHub devuelve owner y nombre separados, así que un solo campo
//     vacío es normal según el forge.
//   - Sin nombre: entonces el owner ES el proyecto.
//   - Los dos vacíos: nada, y no "/".
//
// Y lo que NO puede salir es un separador suelto: "/repo" o "owner/" metacen una barra que
// después no casa con ninguna ruta y convierten un ítem en "no encontrado" sin error.
func TestJoinProjectNoPoneSeparadoresHuerfanos(t *testing.T) {
	casos := []struct {
		caso         string
		owner, piece string
		want         string
	}{
		{"los dos", "acme", "proy", "acme/proy"},
		{"solo owner", "acme", "", "acme"},
		{"solo nombre", "", "proy", "proy"},
		{"ninguno", "", "", ""},
		{"owner con barras", "grupo/sub", "proy", "grupo/sub/proy"},
		{"nombre con barras", "acme", "sub/proy", "acme/sub/proy"},
		{"los dos con barras", "a/b", "c/d", "a/b/c/d"},
		{"espacios", " acme ", " proy ", " acme / proy "},
	}
	for _, c := range casos {
		got := joinProject(c.owner, c.piece)
		if got != c.want {
			t.Errorf("%s: joinProject(%q, %q) dio %q, want %q", c.caso, c.owner, c.piece, got, c.want)
		}
		// Y la propiedad que hace que la función exista: nunca hay una barra al principio
		// ni al final. Es lo que impide que un "/repo" se convierta en una ruta con un
		// segmento vacío que no casa con nada.
		if strings.HasPrefix(got, "/") || strings.HasSuffix(got, "/") {
			t.Errorf("%s: dio %q, con una barra en un borde", c.caso, got)
		}
	}
}

// TestSplitProjectPartePorLaUltimaBarraYNoPorLaPrimera: la operación inversa.
//
// Y partir por la PRIMERA barra es el error clásico aquí, y da un resultado que parece
// plausible: "grupo/sub/proy" partido por la primera da ("grupo", "sub/proy"), y ese
// "sub/proy" como nombre de repo no existe en ningún sitio. Se ve en los tests porque
// fallan al buscar el repo, no al parsear.
//
// Y la normalización de las barras de los bordes es lo que hace que "/grupo/proy/" y
// "grupo/proy" den lo mismo: la API manda la ruta con barra inicial en unas consultas y sin ella
// en otras.
func TestSplitProjectPartePorLaUltimaBarraYNoPorLaPrimera(t *testing.T) {
	casos := []struct {
		nombre string
		path   string
		wantA  string
		wantB  string
	}{
		{"una barra", "acme/proy", "acme", "proy"},
		// Y el caso que decide: dos barras. Por la primera daría ("grupo", "sub/proy").
		{"dos barras", "grupo/sub/proy", "grupo/sub", "proy"},
		{"tres barras", "a/b/c/proy", "a/b/c", "proy"},
		{"sin barra", "proy", "", "proy"},
		{"barra inicial", "/grupo/proy", "grupo", "proy"},
		{"barra final", "grupo/proy/", "grupo", "proy"},
		{"barras en los dos bordes", "/grupo/proy/", "grupo", "proy"},
		{"solo barras", "///", "", ""},
		{"vacio", "", "", ""},
		// Y con espacios: `Trim(path, "/")` quita barras, NO espacios, así que los
		// espacios se quedan dentro de las dos mitades. Es un dato sucio de la API y no
		// pasa por un `TrimSpace`, porque los espacios LEGÍTIMOS son parte del nombre de un
		// proyecto en GitLab —un grupo se puede llamar "mi equipo"—. Quitarlos sería
		// cambiar el nombre del proyecto, y dejarlo es cosa de la capa que resuelve.
		{"barra con espacios alrededor", " grupo/sub /proy ", " grupo/sub ", "proy "},
	}
	for _, c := range casos {
		a, b := splitProject(c.path)
		if a != c.wantA || b != c.wantB {
			t.Errorf("%s: splitProject(%q) dio (%q, %q), want (%q, %q)",
				c.nombre, c.path, a, b, c.wantA, c.wantB)
		}
		// Y la parte de la derecha nunca tiene barras: es un NOMBRE de proyecto, no un
		// path. Con barras ahí, la búsqueda del repo no encuentra nada.
		if strings.Contains(b, "/") {
			t.Errorf("%s: el nombre %q tiene barras", c.nombre, b)
		}
		// Y las dos mitades juntas reconstruyen el original sin la barra del medio, que
		// es lo que permite volver a componer con `joinProject`.
		if a != "" && b != "" {
			if unido := joinProject(a, b); unido != strings.Trim(c.path, "/") {
				t.Errorf("%s: joinProject del resultado dio %q, no reconstruye %q",
					c.nombre, unido, strings.Trim(c.path, "/"))
			}
		}
	}
}

// TestParseTimeSoloAceptaRFC3339YElRestoDaCero: el tiempo de la API.
//
// Y la decisión que se fija es qué pasa con una fecha que no se entiende: se devuelve el
// tiempo CERO, no la fecha actual y no un error. Y eso no es pereza: el tiempo cero es lo
// que la tabla ordena al final —el ítem más viejo al final del inbox— y un ítem con la
// fecha de hoy mal parseada subiría a lo más alto del inbox.
//
// Y la consecuencia de que `time.Parse` devuelva el error y no una fecha cero es lo que hace
// que este `if` sea necesario: `time.Parse` con un layout fijo devuelve la fecha en cero y un
// error, y sin el chequeo del error un item con un timestamp raro seria 0001-01-01, que es
// indistinguible de "nunca lo han tocado".
func TestParseTimeSoloAceptaRFC3339YElRestoDaCero(t *testing.T) {
	// El camino bueno, con y sin zona.
	buenos := []string{
		"2026-04-01T12:00:00Z",
		"2026-04-01T12:00:00+02:00",
		"2026-04-01T12:00:00.123456789Z",
	}
	for _, s := range buenos {
		got := parseTime(s)
		if got.IsZero() {
			t.Errorf("parseTime(%q) dio el tiempo cero", s)
		}
		if got.Year() != 2026 {
			t.Errorf("parseTime(%q) dio el año %d", s, got.Year())
		}
	}

	// Y los que se caen al cero. Cada uno es un formato que alguna vez se ha visto.
	malos := []struct {
		nombre string
		valor  string
	}{
		{"vacio", ""},
		{"solo espacios", "   "},
		{"solo fecha", "2026-04-01"},
		{"formato americano", "04/01/2026"},
		{"epoch en segundos", "1775044800"},
		{"epoch en milis", "1775044800000"},
		{"basura", "cuando sea"},
		{"mes invalido", "2026-13-01T12:00:00Z"},
		{"texto", "<time datetime=\"2026-04-01\"></time>"},
	}
	for _, c := range malos {
		if got := parseTime(c.valor); !got.IsZero() {
			t.Errorf("%s: parseTime(%q) dio %v, want el tiempo cero", c.nombre, c.valor, got)
		}
	}

	// Y la propiedad con la que se distingue de `time.Parse` a secas: el cero de un parseo
	// fallido y el cero de un valor ausente son el MISMO valor. Es lo que permite que la
	// tabla los ordene igual y que "nunca lo han tocado" y "no lo sé" no se distingan en la
	// columna, que es justo lo que se quiere.
	if parseTime("") != parseTime("basura") {
		t.Error("el valor ausente y el ilegible dan tiempos distintos, y no deberían")
	}
	// Y el cero es antes de cualquier fecha real, que es lo que lo hace ordenable al final.
	if parseTime("").After(time.Now()) {
		t.Error("el tiempo cero no es el mínimo")
	}
}

// TestProjectFromRefQuitaElSufijoDelNumero: el `!N` de GitLab.
//
// Y la tentación es partir por el primer `!`, que con un path que lo contenga daría un
// nombre de proyecto truncado. `LastIndex` es lo correcto y lo que se fija.
//
// Y el caso sin `!` devuelve el path entero, que es lo que pasa con un ítem de GitHub: no
// lleva sufijo y el proyecto se queda como estaba.
func TestProjectFromRefQuitaElSufijoDelNumero(t *testing.T) {
	casos := []struct {
		nombre string
		ref    string
		want   string
	}{
		{"con numero", "grupo/proy!12", "grupo/proy"},
		{"sin numero", "grupo/proy", "grupo/proy"},
		{"con barras", "a/b/proy!3", "a/b/proy"},
		{"numero grande", "grupo/proy!1234", "grupo/proy"},
		{"vacio", "", ""},
		{"solo el signo", "!", ""},
		// Y el `!` con espacios alrededor: `LastIndex` corta por el signo, y lo que queda
		// detrás es el proyecto con su espacio final. No hay `TrimSpace`, por lo mismo que
		// arriba.
		{"con espacios", "grupo/proy ! 12", "grupo/proy "},
	}
	for _, c := range casos {
		if got := projectFromRef(c.ref); got != c.want {
			t.Errorf("%s: projectFromRef(%q) dio %q, want %q", c.nombre, c.ref, got, c.want)
		}
	}
	// Y la propiedad: lo que sale NO tiene `!`, porque se usa como ruta de proyecto y un
	// `!` en ella no casa con ningún repo.
	for _, c := range casos {
		if got := projectFromRef(c.ref); strings.Contains(got, "!") {
			t.Errorf("%s: quedo un signo de exclamacion en %q", c.nombre, got)
		}
	}
}

// TestSplitRepoURLSacaElProyectoSoloDelCaminoDeLaAPI: el otro extractor.
//
// Y lo que se fija es la diferencia con `joinProject`: aquí solo hay un caso bueno —una URL
// con el marcador `/repos/`— y todo lo demás devuelve vacío. Y vacío es correcto: una URL
// que no es de la API REST no tiene un proyecto escondido, y adivinar sería inventar.
//
// Y la parte con `/repos/` es lo que la distingue de una URL de clon, que no lo lleva. Una
// `https://github.com/acme/proy.git` NO tiene `/repos/`, así que sale vacío — que es lo
// correcto, porque de una URL de clon no se puede deducir owner y nombre de forma fiable
// con grupos.
func TestSplitRepoURLSacaElProyectoSoloDelCaminoDeLaAPI(t *testing.T) {
	casos := []struct {
		nombre string
		url    string
		wantA  string
		wantB  string
	}{
		{"API de un repo", "https://api.github.com/repos/acme/proy", "acme", "proy"},
		// Y con subgrupos, que es lo que devuelve una organización con equipos.
		{"API con subgrupos", "https://api.github.com/repos/grupo/sub/proy", "grupo/sub", "proy"},
		{"API self-hosted", "https://git.umane.example/api/v3/repos/acme/proy", "acme", "proy"},
		// Y con query string: se queda pegada al nombre. `splitRepoURL` no la quita, y
		// eso es aceptable porque la URL de un ítem no lleva query —la que lleva es la de
		// la API, y esa no se pasa por aquí—. Lo que se fija es que la función NO promete
		// limpiar la query, para que nadie dé por hecho que sí.
		{"con query", "https://api.github.com/repos/acme/proy?x=1", "acme", "proy?x=1"},
		// Y los que NO son de la API REST.
		{"url de clon", "https://github.com/acme/proy.git", "", ""},
		{"url html", "https://github.com/acme/proy", "", ""},
		// Y sin esquema: también sale el proyecto. `splitRepoURL` solo busca el marcador
		// `/repos/`, no el esquema, así que no distingue una URL de una ruta. Es lo que
		// quiere: lo que se pasa aquí viene de una respuesta de la API, no de texto del
		// usuario, así que comprobar el esquema sería una comprobación que nunca falla.
		{"sin esquema", "api.github.com/repos/acme/proy", "acme", "proy"},
		{"vacia", "", "", ""},
	}
	for _, c := range casos {
		a, b := splitRepoURL(c.url)
		if a != c.wantA || b != c.wantB {
			t.Errorf("%s: splitRepoURL(%q) dio (%q, %q), want (%q, %q)",
				c.nombre, c.url, a, b, c.wantA, c.wantB)
		}
	}

	// Y la propiedad: cuando hay proyecto, las dos mitades se pueden recomponer en una
	// ruta real. Es la conexión con `joinProject` que hace que un ítem de la API REST y
	// otro de la GraphQL acaben en la misma clave de memo.
	a, b := splitRepoURL("https://api.github.com/repos/acme/proy")
	if got := joinProject(a, b); got != "acme/proy" {
		t.Errorf("el proyecto de la URL dio %q, que no se puede recomponer", got)
	}
}
