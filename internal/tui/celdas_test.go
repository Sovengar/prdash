package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"prdash/internal/forge/model"
	"prdash/internal/state"
)

// Las funciones de este fichero son las celdas de la tabla del inbox y las etiquetas del
// borde. Y esa es exactamente la razón de fijarlas: no calculan nada que se pueda
// equivocar por un algoritmo —son un `switch` y una concatenación—, pero cada texto que
// sale de aquí es de los pocos que el usuario ve SIN su contexto alrededor, en una columna
// estrecha, y un texto equivocado ahí no se lee como un dato raro sino como que el PR está
// en un estado que no es.
//
// Y hay un caso que merece nombre en cada grupo, y en todos es el mismo: el `default`.
// Cuando el código no reconoce un estado —porque el forge añadió uno, o porque el dato
// todavía no ha llegado—, lo que sale no puede ser un texto inventado. Una celda con un
// estado inventado se lee como un dato. Un guion, o el valor crudo, se lee como "esto no lo
// sé".

// TestLaEtiquetaDelProblemaTraduceLoQueSeSabeYLoQueNo: `problemLabel`.
//
// Y el detalle que importa es que hay dos idiomas en la misma función, y es a propósito:
// cuatro etiquetas en inglés y tres en español. Lo que se fija es que ninguna de las dos
// listas se crece por la otra, porque la mezcla accidental se lee como una errata.
func TestLaEtiquetaDelProblemaTraduceLoQueSeSabeYLoQueNo(t *testing.T) {
	casos := []struct {
		kind string
		want string
	}{
		{"auth", "not authenticated"},
		{"timeout", "timeout"},
		{"ratelimit", "rate limited"},
		{"parse", "respuesta ilegible"},
		{"unsupported", "no soportado"},
		{"validation", "rechazado"},
		{"network", "no connection"},
		// Lo que no se reconoce: el valor crudo. No inventado, no vacío. Un kind vacío de
		// un forge que no avisó bien daría una etiqueta vacía en el borde.
		{"algo-nuevo", "algo-nuevo"},
		{"", ""},
	}
	for _, c := range casos {
		got := problemLabel(c.kind)
		if got != c.want {
			t.Errorf("problemLabel(%q) dio %q, want %q", c.kind, got, c.want)
		}
		// Y ninguna sale con espacios en los bordes: la etiqueta va en una caja con el ancho
		// calculado, y un espacio invisible desplaza todo lo de al lado.
		if got != strings.TrimSpace(got) {
			t.Errorf("problemLabel(%q) dio %q con espacios en los bordes", c.kind, got)
		}
	}
	// Y las siete etiquetas conocidas caben en el ancho del borde. El número sale del
	// presupuesto de la línea del borde, no de un gusto: "not authenticated" son 18
	// caracteres y es la más larga.
	for _, kind := range []string{"auth", "timeout", "ratelimit", "parse", "unsupported", "validation", "network"} {
		if l := len(problemLabel(kind)); l > 18 {
			t.Errorf("problemLabel(%q) son %d caracteres y el borde no tiene ese ancho", kind, l)
		}
	}
}

// TestLaEdadDeLaUltimaActualizacionSeRedondeaHaciaAbajo: `lastRefreshLabel`.
//
// Y los límites son lo que importa aquí, no los valores. El redondeo es hacia abajo, y por
// qué: un texto que va a 59s cuando en realidad ha pasado un minuto se lee como dato
// fresco cuando ya no lo es. Redondeando hacia arriba, un refresco de hace exactamente un
// segundo se pintaría como "0s ago", que no informa de nada.
//
// Y el agrupamiento son tres tramos, no seis: por debajo del minuto en segundos, por debajo
// de la hora en minutos, y horas para siempre. Un tramo de segundos hasta la hora daría
// "3000s ago", que nadie lee.
func TestLaEdadDeLaUltimaActualizacionSeRedondeaHaciaAbajo(t *testing.T) {
	ahora := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)

	casos := []struct {
		nombre string
		hace   time.Duration
		want   string
	}{
		{"ahora mismo", time.Millisecond, "now"},
		// Justo por debajo del minuto: segundos.
		{"59 segundos", 59 * time.Second, "59s ago"},
		// Justo por encima: minutos. El límite es el que decide, y un segundo de diferencia
		// cambia el texto entero.
		{"60 segundos", 60 * time.Second, "1m ago"},
		{"90 segundos", 90 * time.Second, "1m ago"},
		{"59 minutos", 59 * time.Minute, "59m ago"},
		{"60 minutos", 60 * time.Minute, "1h ago"},
		{"25 horas", 25 * time.Hour, "25h ago"},
	}
	for _, c := range casos {
		if got := lastRefreshLabel(ahora.Add(-c.hace), ahora); got != c.want {
			t.Errorf("%s (%s): dio %q, want %q", c.nombre, c.hace, got, c.want)
		}
	}

	// Y el caso de nunca: no dice "hace 2026 años", que es lo que daría un
	// `now.Sub(time.Time{})` sin comprobar. Un texto de 20 caracteres en el borde del inbox,
	// y ningún sentido.
	if got := lastRefreshLabel(time.Time{}, ahora); got != "no data" {
		t.Errorf("un since a cero dio %q, want \"no data\"", got)
	}

	// Y la propiedad que de verdad importa: la etiqueta, vuelta a leerse como duración,
	// vuelve a la edad real con el error de una unidad. Eso es lo que dice "redondeo hacia
	// abajo" en un sitio donde se pueda medir, y no la longitud del texto.
	//
	// Y la longitud del texto no sirve: "59s ago" son 7 caracteres y "1m ago" son 6. Y el
	// número pelado tampoco: 59 de segundos seguido de 1 de minutos baja, porque la unidad
	// cambió. Las dos formas se fixaron primero, fallaron, y las dos eran un artefacto del
	// formato y no un problema del código. Lo único que mide algo es si el número que se
	// pinta, con la unidad que lo acompaña, representa la edad.
	for _, d := range []time.Duration{
		time.Second, 30 * time.Second, 59 * time.Second,
		60 * time.Second, 90 * time.Second,
		30 * time.Minute, 59 * time.Minute, 60 * time.Minute, 90 * time.Minute,
		3 * time.Hour, 50 * time.Hour,
	} {
		got := lastRefreshLabel(ahora.Add(-d), ahora)
		representado, err := duracionDe(got)
		if err != nil {
			t.Errorf("el texto %q no se pudo volver a leer: %v", got, err)
			continue
		}
		// El error cabe en UNA UNIDAD DE LA ETIQUETA, no en un minuto fijo. Con un minuto
		// fijo el aserto fallaba a 1h30m, que redondea a "1h ago" con 30 minutos de
		// diferencia: es media hora de error y media hora es exactamente media unidad, así
		// que es redondeo y no un fallo. Comparar contra la unidad es lo que hace que la
		// comprobación sea la misma en los tres tramos.
		_, unid := numeroDe(got)
		if diff := d - representado; diff < 0 || diff >= duracionDeUnidad(unid) {
			t.Errorf("con %s la etiqueta %q representa %s: el error no cabe en una unidad "+
				"de esa etiqueta", d, got, representado)
		}
	}
	// Y la unidad nunca retrocede en la secuencia: segundos a minutos a horas.
	var previa int
	for _, d := range []time.Duration{time.Second, 59 * time.Second, 60 * time.Second,
		59 * time.Minute, 60 * time.Minute, 3 * time.Hour} {
		_, unid := numeroDe(lastRefreshLabel(ahora.Add(-d), ahora))
		if unid < previa {
			t.Errorf("con %s la unidad %d retrocede desde %d", d, unid, previa)
		}
		previa = unid
	}
}

// TestElAvisoDeRamasUsaElTextoDeLaCLI: `warnMsg`, el motivo del popup de ramas.
//
// Y la regla es la que dice el comentario del código y no es negociable: el texto es el de
// la CLI, no uno inventado. Un "no se pudieron leer las ramas" de invención taparía tanto un
// 404 como un token caducado, que piden acciones opuestas —uno es "el repo no está", el
// otro es "vuelve a autenticarte"—.
//
// Y el caso que hace que la función exista: un warning con `Kind` puesto y `Msg` vacío. Es
// lo que deja un adapter que clasifica bien y no compone el mensaje, y sin el fallback el
// popup se abriría sin decir nada.
func TestElAvisoDeRamasUsaElTextoDeLaCLI(t *testing.T) {
	// Con texto: sale el de la CLI, tal cual.
	got := warnMsg([]model.Warning{{Kind: "auth", Msg: "gh: Bad credentials"}})
	if got != "gh: Bad credentials" {
		t.Errorf("warnMsg dio %q, want el texto de la CLI sin tocar", got)
	}
	// El primero con texto gana sobre los que vienen detrás, porque los adapters emiten los
	// avisos en orden de importancia y el que explica va el primero.
	got = warnMsg([]model.Warning{
		{Kind: "auth", Msg: "el que explica"},
		{Kind: "network", Msg: "otro distinto"},
	})
	if got != "el que explica" {
		t.Errorf("warnMsg dio %q, want el primero con texto", got)
	}
	// Con texto en blanco: se salta al siguiente que lo tenga.
	got = warnMsg([]model.Warning{
		{Kind: "a", Msg: "   "},
		{Kind: "b", Msg: "\t\n"},
		{Kind: "c", Msg: "el bueno"},
	})
	if got != "el bueno" {
		t.Errorf("warnMsg dio %q, want el que tenía texto de verdad", got)
	}
	// Y sin ningún texto: el genérico, que tiene que ser un texto y no la cadena vacía.
	for _, warns := range [][]model.Warning{
		nil,
		{},
		{{Kind: "auth", Msg: ""}},
		{{Kind: "a", Msg: "  "}},
	} {
		if got := warnMsg(warns); strings.TrimSpace(got) == "" {
			t.Errorf("warnMsg(%v) devolvió texto vacío", warns)
		}
	}
	if got := warnMsg(nil); got != "the branches could not be read" {
		t.Errorf("sin avisos dio %q, want el genérico", got)
	}
}

// TestElForgeSeIdentificaDeDosFormasYLasDosCaben: `forgeLabel` y `forgeBadge`.
//
// Y son las dos formas del mismo dato, y la razón de que existan dos está en el ancho:
// `forgeLabel` vive en el detalle, donde cabe la ruta entera; `forgeBadge` vive en la
// columna, donde no. Lo que se fija es que la versión larga NO pierde el host —un
// self-managed sin host es indistinguible de otro, y "GH" sin más no dice contra qué se está
// trabajando— y que la corta SÍ lo reduce.
//
// Y el caso que decide la corta: un host self-managed. "GLab@umane" frente a "GLab": con la
// misma abreviatura, el usuario no sabe si el ítem es del GitLab público o del de la oficina,
// y las ramas no son las mismas.
func TestElForgeSeIdentificaDeDosFormasYLasDosCaben(t *testing.T) {
	casos := []struct {
		nombre string
		it     model.Item
		larga  string
		corta  string
	}{
		{
			nombre: "github publico",
			it:     model.Item{Forge: "github", Host: "github.com"},
			larga:  "github@github.com",
			corta:  "GH",
		},
		{
			nombre: "gitlab publico",
			it:     model.Item{Forge: "gitlab", Host: "gitlab.com"},
			larga:  "gitlab@gitlab.com",
			corta:  "GLab",
		},
		{
			nombre: "bitbucket publico",
			it:     model.Item{Forge: "bitbucket", Host: "bitbucket.org"},
			larga:  "bitbucket@bitbucket.org",
			corta:  "BB",
		},
		{
			// Self-managed: solo la primera etiqueta del host, que es la que lo identifica
			// de verdad. "GLab@git" y no "GLab@git.intra.example" porque la columna no tiene
			// ese ancho, y "GLab@git" basta para no confundirlo con el público.
			nombre: "gitlab self-managed",
			it:     model.Item{Forge: "gitlab", Host: "git.umane.example"},
			larga:  "gitlab@git.umane.example",
			corta:  "GLab@git",
		},
		{
			// Un host con una sola etiqueta, sin punto: se usa tal cual.
			nombre: "host sin puntos",
			it:     model.Item{Forge: "gitlab", Host: "localhost"},
			larga:  "gitlab@localhost",
			corta:  "GLab@localhost",
		},
		{
			// Forge sin abreviatura conocida: se usa el NOMBRE COMPLETO, no la primera letra
			// ni la primera etiqueta. "PHab" sería un invento que se parece a las otras
			// abreviaturas y no significa nada. Y el host se reduce igual que en los demás,
			// que es lo que hace que la columna siga teniendo un ancho acotado.
			nombre: "forge desconocido",
			it:     model.Item{Forge: "phabricator", Host: "phab.example"},
			larga:  "phabricator@phab.example",
			corta:  "phabricator@phab",
		},
		{
			// Sin host: no se pone "@" colgando. Un "@" suelto se lee como un dato
			// truncado.
			nombre: "sin host",
			it:     model.Item{Forge: "github"},
			larga:  "github",
			corta:  "GH",
		},
		{
			nombre: "forge vacío",
			it:     model.Item{},
			larga:  "",
			corta:  "",
		},
		{
			// El host público se compara sin distinguir mayúsculas: los forges devuelven
			// "GitHub.com" en algunos sitios y "github.com" en otros.
			nombre: "host publico en mayusculas",
			it:     model.Item{Forge: "github", Host: "GitHub.COM"},
			larga:  "github@GitHub.COM",
			corta:  "GH",
		},
	}
	for _, c := range casos {
		if got := forgeLabel(c.it); got != c.larga {
			t.Errorf("%s: forgeLabel dio %q, want %q", c.nombre, got, c.larga)
		}
		if got := forgeBadge(c.it); got != c.corta {
			t.Errorf("%s: forgeBadge dio %q, want %q", c.nombre, got, c.corta)
		}
		// Y la corta nunca sale más larga que la larga: esa es su razón de existir.
		if len(c.corta) > len(c.larga) {
			t.Errorf("%s: la etiqueta corta %q es más larga que la larga %q", c.nombre, c.corta, c.larga)
		}
		// Y ninguna tiene espacios en los bordes: la etiqueta va en una caja con el ancho
		// calculado, y un espacio invisible desplaza lo de al lado.
		if got := forgeBadge(c.it); got != strings.TrimSpace(got) {
			t.Errorf("%s: forgeBadge dio %q con espacios", c.nombre, got)
		}
	}
}

// TestElPapelDelUsuarioDistingueOwnDeReview: `roleText`, la columna que avisa de que approve
// no va a funcionar.
//
// Y "own" es la que importa: es la señal de que un approve no va a funcionar, ANTES de
// pulsarlo. Sin ella el usuario pulsa approve sobre su propio PR, espera, y ve el rechazo
// del forge —que con el texto de GitHub en inglés, en un error que desaparece en un toast
// de dos segundos—.
//
// Y la jerarquía es la que dice el switch: primero el `ReviewKind`, que es lo que el forge
// dice, y solo si no hay kind se mira quién es el viewer.
func TestElPapelDelUsuarioDistingueOwnDeReview(t *testing.T) {
	viewer := "yo"

	// Con review pendiente: la etiqueta del kind, sin mirar quién es.
	if got := roleText(model.Item{ReviewKind: model.ReviewRequested}, viewer); got != "review req" {
		t.Errorf("review requested dio %q", got)
	}
	if got := roleText(model.Item{ReviewKind: model.ReviewAssigned}, viewer); got != "assigned" {
		t.Errorf("review assigned dio %q", got)
	}
	// Y un ítem propio con review pendiente sigue siendo "review req", no "own": el forge
	// dice que hay review pendiente, y quien tiene que mirarlo es el viewer aunque sea el
	// autor. Poner "own" ahí sería marcar como inaccionable algo que el forge está pidiendo
	// mirar, y en un repo con un solo participante eso es el caso normal.
	propioConReview := model.Item{ReviewKind: model.ReviewRequested, Author: viewer}
	if got := roleText(propioConReview, viewer); got != "review req" {
		t.Errorf("un review pedido sobre un item propio dio %q, want review req", got)
	}

	// Sin review pendiente: aquí sí mira quién es.
	sinReview := model.Item{Author: "otro", Title: "algo"}
	if got := roleText(sinReview, viewer); got != "-" {
		t.Errorf("un ítem de otro sin review dio %q, want -", got)
	}
	// Y el propio.
	if got := roleText(model.Item{Author: viewer, Title: "algo"}, viewer); got != "own" {
		t.Errorf("un ítem propio dio %q, want own", got)
	}
	// Y con un viewer vacío. `CanApprove` cae al otro camino —el `Section` del ítem—, así que
	// la etiqueta ya no depende de quién mira sino de dónde vino el ítem. Que salga una
	// etiqueta y no una vacía es lo que importa: la columna existe siempre.
	if got := roleText(sinReview, ""); got == "" {
		t.Error("con viewer vacío la etiqueta salió vacía")
	}
	// Y con un viewer vacío sobre un ítem de la sección de creados: `CanApprove` dice que no,
	// así que la etiqueta es "own" aunque no se sepa quién mira.
	if got := roleText(model.Item{Section: model.SectionAuthored, Author: "alguien"}, ""); got != "own" {
		t.Errorf("un item creado con viewer vacío dio %q, want own", got)
	}
	// Y ninguna etiqueta tiene espacios en los bordes: van en una columna fija.
	for _, kind := range []model.ReviewKind{model.ReviewRequested, model.ReviewAssigned, ""} {
		if got := roleText(model.Item{ReviewKind: kind}, viewer); got != strings.TrimSpace(got) {
			t.Errorf("roleText dio %q con espacios para kind %q", got, kind)
		}
	}
}

// TestLosChecksSeResumenConElRecuentoYElSemaforo: `checksText` y `styleChecks`.
//
// Y la asimetría entre los tres estados es deliberada y hay que fijarla: el que falla lleva
// el RECUENTO, el pendiente lleva el recuento, y el que pasa no lleva nada. Lo que pasa no
// lleva nada porque un "✓12" es ruido —no hay nada que hacer con "pasan doce"— y porque el
// número engaña: GitHub solo devuelve los checks del primer workflow.
//
// Y el `-` del estado desconocido es lo importante. Un "✓" ahí sería mentir: diría que el CI
// está bien cuando lo que se sabe es que no se ha preguntado.
func TestLosChecksSeResumenConElRecuentoYElSemaforo(t *testing.T) {
	casos := []struct {
		nombre  string
		checks  model.Checks
		wantTxt string
	}{
		{"failing con recuento", model.Checks{State: model.ChecksFailing, Failing: 3, Total: 12}, "✗3"},
		{"failing de uno", model.Checks{State: model.ChecksFailing, Failing: 1}, "✗1"},
		{"pending con recuento", model.Checks{State: model.ChecksPending, Pending: 2}, "…2"},
		{"passing sin recuento", model.Checks{State: model.ChecksPassing, Total: 12}, "✓"},
		{"desconocido", model.Checks{State: model.ChecksUnknown}, "-"},
		{"estado vacio", model.Checks{}, "-"},
		{
			// Y el caso que decide: estado desconocido con cifras. El estado es lo que dice la
			// verdad; las cifras sin estado son de un sitio que no clasificó lo que le dieron.
			// Un "✓" ahí sería lo peor que puede pintar esta celda.
			nombre:  "cifras sin estado",
			checks:  model.Checks{Failing: 3, Total: 9},
			wantTxt: "-",
		},
	}
	render := func(cs model.Checks) string { return styleChecks(cs).Render("x") }
	for _, c := range casos {
		if got := checksText(c.checks); got != c.wantTxt {
			t.Errorf("%s: checksText dio %q, want %q", c.nombre, got, c.wantTxt)
		}
		// Y la celda nunca sale vacía: una celda vacía desplaza la columna y hace que el
		// resto de la fila parezca desalineada.
		if got := checksText(c.checks); strings.TrimSpace(got) == "" {
			t.Errorf("%s: checksText devolvió vacío", c.nombre)
		}
		// Y el color acompaña al texto: mismo estado, mismo estilo, con independencia de los
		// números. Un "✗" en verde es peor que ningún color, porque el ojo lee el color antes
		// que el glifo.
		//
		// Se compara el resultado de renderizar y no el estilo, porque `lipgloss.Style` lleva
		// slices dentro y no se puede comparar con `==`.
		if render(c.checks) != render(model.Checks{State: c.checks.State}) {
			t.Errorf("%s: el estilo no depende solo del estado", c.nombre)
		}
	}

	// Y los cuatro estados pintan de cuatro maneras distintas. Es lo que distingue la columna
	// por color: si dos comparten estilo, uno de los cuatro se pierde, y el que peor sale es
	// el que más se mira.
	vistos := map[string]model.CheckState{}
	for _, estado := range []model.CheckState{
		model.ChecksFailing, model.ChecksPending, model.ChecksPassing, model.ChecksUnknown,
	} {
		pintado := render(model.Checks{State: estado})
		if otro, ya := vistos[pintado]; ya {
			t.Errorf("los estados %q y %q pintan igual (%q)", estado, otro, pintado)
		}
		vistos[pintado] = estado
	}
}

// TestElEstadoAccionableDaMotivosQueDicenQueHacer: `Actionable`, la puerta antes de la
// acción.
//
// Y el motivo importa tanto como el veto: "item is already merged" dice qué ha pasado, y un
// motivo vacío produce un toast con el prefijo y nada.
//
// Y los estados vetados son los que no tienen arreglo. Un PR cerrado sin merge también, y
// ese es el que más se olvida: cerrarlo no es lo mismo que mergearlo, y quien solo
// contempla el caso de "ya mergeado" deja pasar el otro.
func TestElEstadoAccionableDaMotivosQueDicenQueHacer(t *testing.T) {
	casos := []struct {
		nombre string
		estado string
		veto   bool
	}{
		{"mergeado", "MERGED", true},
		{"cerrado", "CLOSED", true},
		{"abierto", "OPEN", false},
		{"en blanco", "", false},
		// Y minúsculas: GitHub las manda en mayúsculas y GitLab en minúsculas, y `Derive`
		// normaliza antes de decidir. Un `merged` que no se normalizara daría "está bien" como
		// si fuera un estado cualquiera, y el approve se iría a un PR ya integrado.
		{"mergeado en minusculas", "merged", true},
		{"cerrado en minusculas", "closed", true},
	}
	for _, c := range casos {
		ok, motivo := state.Actionable(model.Item{Number: 1, State: c.estado})
		if ok == c.veto {
			t.Errorf("%s: Actionable dio ok=%v, want %v", c.nombre, ok, !c.veto)
		}
		// Y el motivo solo se exige cuando se veta: cuando está bien, una cadena vacía es lo
		// correcto.
		if !ok && strings.TrimSpace(motivo) == "" {
			t.Errorf("%s: vetó sin dar motivo", c.nombre)
		}
		if ok && motivo != "" {
			t.Errorf("%s: no vetó pero dio motivo %q", c.nombre, motivo)
		}
	}
}

// numeroDe extrae la cantidad y la unidad de un texto de edad.
//
// Y el formato no separa número y unidad con un espacio: "30s ago" es un solo campo con la
// cantidad pegada a la letra. La primera versión de esto leía el segundo campo buscando la
// unidad, que es "ago", así que la unidad le salía siempre desconocida y la comprobación de
// que no retrocede pasaba siempre —un aserto que no puede fallar es un aserto que no
// comprueba—. La cantidad sí salía bien, porque los dígitos van al principio del primer
// campo.
//
// Se trabaja con bytes y no con runes porque la entrada es ASCII —dígitos y una letra de
// unidad—, y mezclar los dos es de donde se cuelan los caracteres de más en un parser
// escrito a mano.
func numeroDe(texto string) (int, int) {
	campos := strings.Fields(texto)
	if len(campos) == 0 {
		return 0, 0
	}
	n, unid, _ := partesDe(campos[0])
	return n, unid
}

// duracionDe vuelve a convertir una etiqueta de edad en la duración que representa, que es
// lo que permite comprobar el redondeo sin depender del formato.
func duracionDe(texto string) (time.Duration, error) {
	campos := strings.Fields(texto)
	if len(campos) == 0 {
		return 0, errors.New("texto vacío")
	}
	n, unid, habia := partesDe(campos[0])
	if !habia {
		// "now" y "no data": no llevan cantidad, y las dos son la edad cero.
		if campos[0] == "now" || campos[0] == "no" {
			return 0, nil
		}
		return 0, errors.New("sin número")
	}
	switch unid {
	case 1:
		return time.Duration(n) * time.Second, nil
	case 2:
		return time.Duration(n) * time.Minute, nil
	case 3:
		return time.Duration(n) * time.Hour, nil
	}
	return 0, errors.New("unidad desconocida")
}

// duracionDeUnidad convierte el número de unidad que devuelve `partesDe` en la duración que
// representa, para poder comparar el error del redondeo contra ella.
func duracionDeUnidad(unidad int) time.Duration {
	switch unidad {
	case 1:
		return time.Second
	case 2:
		return time.Minute
	case 3:
		return time.Hour
	}
	return 0
}

// partesDe saca la cantidad del principio de un campo y la unidad que la sigue.
//
// El tercer valor distingue "0" de "no había número": "0s ago" es un dato y "now" no tiene
// cantidad. Sin esa distinción, un "now" se leería como cero segundos y el aserto de
// redondeo no notaría la diferencia entre las dos cosas.
func partesDe(campo string) (n, unidad int, habiaNumero bool) {
	i := 0
	for i < len(campo) && campo[i] >= '0' && campo[i] <= '9' {
		n = n*10 + int(campo[i]-'0')
		i++
	}
	if i == 0 {
		return 0, 0, false
	}
	switch {
	case i < len(campo) && campo[i] == 's':
		unidad = 1
	case i < len(campo) && campo[i] == 'm':
		unidad = 2
	case i < len(campo) && campo[i] == 'h':
		unidad = 3
	}
	return n, unidad, true
}
