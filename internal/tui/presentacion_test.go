package tui

import (
	"strings"
	"testing"
	"time"

	"prdash/internal/forge"
	"prdash/internal/testutil"
)

// El último grupo: funciones de la capa de presentación y de `plan`, donde el valor que
// devuelve es lo que el usuario LEE. Y eso las hace especiales: un valor neutro en una función
// de presentación no se manifiesta como un error sino como una etiqueta que no encaja.
//
// Y el caso peor de todos es el del `default` que devuelve el valor de otro: un Kind de pane
// desconocido que saliera con el binario del editor lanzaría el editor donde debía ir el
// revisor, y un modo de merge desconocido pintado como "merge commit" haría que el usuario
// creyera que va a pasar algo que nadie sabe qué es.

// TestUnMergeModoDesconocidoNoSePresentaComoUnModoConocido: `Label` y `modeKey`.
//
// Y son dos funciones con la misma forma y la misma consecuencia, y por eso van juntas: el
// `default` de las dos.
//
// En `Label` el `default` devuelve el propio valor, que es lo correcto y lo que se fija: un
// modo corrupto se muestra como lo que es en vez de disfrazarse de "merge commit". Y eso es
// justo lo que impide que el usuario confirme algo que nadie sabe qué va a pasar.
//
// Y `Valid` es la tercera del mismo grupo y la que de verdad protege: un modo desconocido
// tiene que ser un warning explícito, no un flag vacío, porque `gh pr merge` sin flag de
// estrategia cae en un prompt interactivo y se queda colgado.
func TestUnMergeModoDesconocidoNoSePresentaComoUnModoConocido(t *testing.T) {
	for _, c := range []struct {
		modo forge.MergeMode
		want string
	}{
		{forge.MergeCommit, "merge commit"},
		{forge.Rebase, "rebase"},
		{forge.Squash, "squash"},
		{forge.MergeMode("fast-forward"), "fast-forward"},
		{forge.MergeMode(""), ""},
	} {
		if got := c.modo.Label(); got != c.want {
			t.Errorf("%q.Label() = %q, want %q", string(c.modo), got, c.want)
		}
	}

	// Y `Valid` separa lo conocido de lo desconocido, que es lo que decide si se avisa antes
	// de construir el argv.
	for _, c := range []struct {
		modo forge.MergeMode
		want bool
	}{
		{forge.MergeCommit, true},
		{forge.Rebase, true},
		{forge.Squash, true},
		{forge.MergeMode("fast-forward"), false},
		{forge.MergeMode(""), false},
		{forge.MergeMode("MERGE"), false}, // las mayúsculas son otro modo, no el mismo
	} {
		if got := c.modo.Valid(); got != c.want {
			t.Errorf("%q.Valid() = %v, want %v", string(c.modo), got, c.want)
		}
	}

	// Y la tecla de la confirmación, que es el tercer sitio donde un modo desconocido tiene
	// que ser reconocible como tal: dos modos con la misma letra serían indistinguibles en la
	// línea "press m to merge".
	for _, c := range []struct {
		modo forge.MergeMode
		want string
	}{
		{forge.MergeCommit, "m"},
		{forge.Rebase, "r"},
		{forge.Squash, "s"},
		{forge.MergeMode("otro"), "?"},
	} {
		if got := modeKey(c.modo); got != c.want {
			t.Errorf("tecla de %q = %q, want %q", string(c.modo), got, c.want)
		}
	}
}

// TestReplaceSustituyeElAvisoVigenteYNoDuplicaElTexto: la composición de avisos.
//
// Y lo que permite es superponer texto sobre un aviso —"retarget (main → release/2.0) en
// curso…" encima de "consultando ramas"— sin que se vean dos avisos del mismo origen. Y el caso
// que hay que probar es el de un aviso que YA NO está: el `prev` que se busca caducó o lo
// sustituyó otro, y entonces el nuevo se apila en vez de perderse.
//
// Y es un caso real, no un adorno: el aviso que se sustituye es el del spinner, que cambia de
// texto varias veces por segundo, y su texto anterior puede haber caducado entre medias.
func TestReplaceSustituyeElAvisoVigenteYNoDuplicaElTexto(t *testing.T) {
	tm := &toastManager{now: func() time.Time { return time.Unix(0, 0) }}

	// Sustituye: un solo aviso, con el texto nuevo.
	tm.show("consultando ramas", toastInfo)
	tm.replace("consultando ramas", "consultando ramas…", toastInfo)
	if len(tm.toasts) != 1 {
		t.Fatalf("quedan %d avisos tras sustituir, want 1: el texto nuevo se apiló en vez "+
			"de sustituir al viejo y se ven dos avisos del mismo origen", len(tm.toasts))
	}
	if tm.toasts[0].message != "consultando ramas…" {
		t.Errorf("el texto quedó en %q", tm.toasts[0].message)
	}

	// Y con un `prev` que no está: apila, no se pierde. Un aviso que se pierde es el peor
	// desenlace, porque el usuario no ve ni el texto ni el error.
	tm.replace("un texto que ya no existe", "otro aviso", toastWarning)
	if len(tm.toasts) != 2 {
		t.Errorf("un `prev` inexistente dejó %d avisos, want 2: el nuevo se apila", len(tm.toasts))
	}

	// Y un mensaje VACÍO no se apila: un aviso vacío es indistinguible de no haber avisado, y
	// ocuparía una de las cuatro filas del panel.
	antes := len(tm.toasts)
	tm.replace("consultando ramas…", "", toastInfo)
	if len(tm.toasts) != antes {
		t.Errorf("un mensaje vacío cambió el número de avisos: %d -> %d", antes, len(tm.toasts))
	}
}

// TestLaCabeceraMuestraElSpinnerPrimeroYElEstadoDeLosForgesDetras: `headerSection`.
//
// Y el orden es deliberado y es lo que hay que fijar: el indicador de refresco va PRIMERO para
// que sobreviva al recorte en anchos estrechos, y el estado de los forges detrás. Al revés, un
// terminal de cuarenta columnas cortaría el final de la línea y el spinner desaparecería justo
// cuando hay algo en marcha, que es el peor momento para perder la señal.
//
// Y sin carga el spinner se va pero el estado de los forges se queda, porque el estado de
// autenticación es lo que explica por qué un inbox puede estar vacío.
func TestLaCabeceraMuestraElSpinnerPrimeroYElEstadoDeLosForgesDetras(t *testing.T) {
	nuevo := func(t *testing.T) Model {
		t.Helper()
		m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
		m.width, m.height = 120, 40
		return m
	}

	// Cargando: el spinner está.
	cargando := nuevo(t)
	cargando.loading = true
	texto := cargando.headerSection().text
	if !strings.Contains(texto, "refreshing") {
		t.Errorf("cargando, la cabecera no dice que está refrescando:\n%s", texto)
	}
	if !strings.Contains(texto, "PRDash") {
		t.Errorf("la cabecera perdió el título:\n%s", texto)
	}
	// Y el spinner va antes que el estado de los forges.
	dondeSpinner := strings.Index(texto, "refreshing")
	dondeEstado := len(texto)
	for _, marca := range []string{"github", "not authenticated", "no auth"} {
		if i := strings.Index(texto, marca); i >= 0 && i < dondeEstado {
			dondeEstado = i
		}
	}
	if dondeEstado < dondeSpinner {
		t.Errorf("el estado de los forges va antes del spinner: %q", texto)
	}

	// Y sin carga: el spinner se va y el resto se queda.
	quieto := nuevo(t)
	quieto.loading = false
	if strings.Contains(quieto.headerSection().text, "refreshing") {
		t.Errorf("sin carga la cabecera sigue diciendo que refresca:\n%s",
			quieto.headerSection().text)
	}

	// Y el caso de la terminal estrecha, que es el motivo del orden: el spinner sobrevive
	// al recorte y el resto se pierde, que es lo que se buscaba.
	estrecho := nuevo(t)
	estrecho.loading = true
	estrecho.width = 40
	if !strings.Contains(estrecho.headerSection().text, "refreshing") {
		t.Errorf("con 40 columnas el spinner desaparece: %q", estrecho.headerSection().text)
	}
}

// TestUnRecuentoNegativoNoSePresentaComoUnoRedondeado: `compactCount`.
//
// Y el caso negativo es el que importa: un recuento de diferencias no puede ser negativo en una
// ficha normal, así que un negativo es un dato corrupto —o un adapter que resta en vez de
// sumar— y lo que hay que evitar es que se presente como "0" o dentro de la ventana de los
// "1.2k", donde sería indistinguible de un recuento real.
//
// Y el motivo del `default` en enteros está en el propio código: un decimal redondeado al alza
// que se sale de la ventana no cabe en cuatro runes, así que a partir de cierto punto se
// delegan en el entero.
func TestUnRecuentoNegativoNoSePresentaComoUnoRedondeado(t *testing.T) {
	for _, c := range []struct {
		n    int
		want string
	}{
		{0, "0"},
		{1, "1"},
		{999, "999"},
		{1000, "1.0k"},
		{1050, "1.1k"},
		{1500, "1.5k"},
		// 9999 redondea al alza a "10k" y no a "9.9k", y con eso salen cinco runes —
		// por eso a partir de cinco dígitos se delegan en los enteros.
		{9999, "10k"},
		{10000, "10k"},
		{999999, "999k"},
		{1000000, "1000k"},
		{-1, "-1"},
		{-1500, "-1500"},
	} {
		if got := compactCount(c.n); got != c.want {
			t.Errorf("compactCount(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}

// TestUnDiffstatQueNoEsDosCifrasConSignoNoSePinta: el filtro de `diffSpans`.
//
// Y `additions` y `deletions` llegan del forge como CADENAS, y una cadena vacía, un número sin
// signo o una frase con unidades se cuelan en celdas pintadas de otros sitios. El filtro
// devuelve nil —no se pinta nada— en vez de un recorte a medias.
//
// Y los rechazos son tres y el tercero es el interesante: "+12 archivos" tiene los dos signos y
// dos números, así que un `Cut` por espacios sin comprobar el resto la aceptaría y pintaría un
// diffstat que el forge no dijo. Y hay un cuarto motivo de peso: un color puesto sobre algo que
// no es una cifra mentiría sobre el dato.
func TestUnDiffstatQueNoEsDosCifrasConSignoNoSePinta(t *testing.T) {
	for _, c := range []struct {
		nombre string
		plain  string
		pinta  bool
	}{
		{"los dos con signo", "+12 -3", true},
		// La cola va DESPUÉS del recuento de borrados, y por eso "+12 archivos -3" NO se
		// pinta: el segundo corte lee "archivos" como el número de borrados y falla ahí.
		// Mi primera versión lo daba por bueno y el filtro hacía bien.
		{"con unidades al final", "+12 -3 archivos", true},
		{"unidades en medio", "+12 archivos -3", false},
		{"sin el separado", "+12", false},
		{"sin signos", "12 3", false},
		{"vacío", "", false},
		{"texto que no es un recuento", "no changes", false},
		{"un solo signo", "+12 -", false},
		{"signo suelto", "- 3", false},
		{"cola vacía", "+12 -3   ", true},
	} {
		spans := diffSpans(c.plain)
		pinta := len(spans) > 0
		if pinta != c.pinta {
			t.Errorf("%s: pinta=%v, want %v (spans=%v)", c.nombre, pinta, c.pinta, spans)
		}
		// Y lo que se pinta es el dato tal cual, con los signs dentro del span: si el signo
		// fuera a parte, el color dejaría de cubrirlo y el "+" se pintaría del color del
		// título.
		for _, s := range spans {
			if s.text == "" {
				t.Errorf("%s: hay un span vacío en %v", c.nombre, spans)
			}
		}
	}
}
