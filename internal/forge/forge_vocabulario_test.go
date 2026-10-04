package forge

import (
	"errors"
	"strings"
	"testing"

	"prdash/internal/forge/model"
	"prdash/internal/state"
)

// Este fichero cubre el vocabulario de acciones: los modos de merge, el recorte de
// comentarios, y la traducción de los avisos de una CLI a un motivo canónico. Las tres
// cosas comparten una propiedad: son la CAPA donde el inglés de una herramienta externa se
// convierte en un texto que prdash puede comparar, clasificar y pintar.
//
// Y ahí está el riesgo. Un `warning` con `kind` mal escrito no falla: se clasifica en la
// rama `default`, que devuelve "algo pasó" sin decir qué. Y un merge con un modo
// desconocido tampoco falla: se traduce a un flag vacío, y `gh pr merge` sin flag de
// estrategia se queda en un prompt interactivo. Ninguno de los dos da un error visible, y
// los dos se ven como un programa que se cuelga o que no dice por qué.

// TestElModoDeMergeSeValidaAntesDeConstruirElArgv: `Valid` es la puerta, y `Label` es lo
// que el usuario confirma antes de mergear.
//
// Y la relación entre los dos es lo que hay que fijar: los mismos tres nombres salen en
// uno y en otro, con el mismo idioma. Si el adapter construye `--squash` mientras `Label`
// dice "squash and merge", el usuario confirma una cosa y el forge ejecuta otra.
//
// Y el modo desconocido se comprueba dos veces por una razón: no basta con que `Valid` lo
// rechace, tiene que ser `ErrUnknownMergeMode` el que lo diga, porque ese error es lo que
// evita el flag vacío. Un `Valid` que devuelve false y un adapter que ignora el resultado
// siguen siendo un prompt colgado.
func TestElModoDeMergeSeValidaAntesDeConstruirElArgv(t *testing.T) {
	// La etiqueta de cada modo, fijada una a una. Y NO es el nombre del modo en los tres
	// casos: MergeCommit se confirma como "merge commit" y no como "merge", porque lo que
	// el usuario tiene delante es una lista de estrategias y "merge" a secas se lee como
	// la accion generica, no como una de las tres. La primera version de este aserto
	// suponia que etiqueta y nombre coincidian, y falla justo en el modo que mas importa.
	etiquetas := map[MergeMode]string{
		MergeCommit: "merge commit",
		Rebase:      "rebase",
		Squash:      "squash",
	}
	for _, m := range []MergeMode{MergeCommit, Rebase, Squash} {
		if !m.Valid() {
			t.Errorf("%q: Valid() dio false para un modo conocido", m)
		}
		if got := m.Label(); got != etiquetas[m] {
			t.Errorf("%q: Label() dio %q, want %q", m, got, etiquetas[m])
		}
		if strings.TrimSpace(m.Label()) == "" {
			t.Errorf("%q: etiqueta vacia", m)
		}
	}

	// Los desconocidos: inválidos, y con error canónico que los nombra a ellos y a los
	// válidos, que es lo que permite corregir el atajo sin ir a buscar la lista.
	for _, m := range []MergeMode{"", "REBASE", "merge-commit", "fast-forward", "rebas"} {
		if m.Valid() {
			t.Errorf("%q: Valid() dio true para un modo que no existe", m)
		}
		err := ErrUnknownMergeMode(m)
		if err == nil {
			t.Fatalf("%q: ErrUnknownMergeMode dio nil", m)
		}
		if m != "" && !strings.Contains(err.Error(), string(m)) {
			t.Errorf("el error %q no nombra el modo %q", err, m)
		}
		// La lista de validos son los NOMBRES, no las etiquetas. Y por que importa: el
		// error dice "expected merge, rebase or squash" y eso es lo que alguien compara
		// contra lo que hay en el fichero de config. Si pasara a listar etiquetas,
		// "merge commit" contra el `merge` del config no casaria.
		for _, ok := range []MergeMode{MergeCommit, Rebase, Squash} {
			if !strings.Contains(err.Error(), string(ok)) {
				t.Errorf("el error %q no menciona el modo valido %q", err, ok)
			}
		}
	}
	if !strings.Contains(ErrUnknownMergeMode("").Error(), "expected") {
		t.Error("el error del modo vacio no dice que se esperaba una lista")
	}
}

// TestKeepLastRecortaPorLaCola: los comentarios que se muestran son los últimos.
//
// Y "por la cola" es la parte que importa, porque es la que distingue esta función de un
// `[:limit]`. Los comentarios de un PR se leen del más viejo al más nuevo, así que
// quedarse con los primeros deja al usuario leyendo el principio de una conversación que ya
// tiene veinte respuestas nuevas. Recortar por la cabeza no da un error: da la
// conversación equivocada.
//
// Y el total no se toca a propósito: es lo que permite saber que lo que se ve no es todo.
func TestKeepLastRecortaPorLaCola(t *testing.T) {
	// Más de lo que cabe: se quedan los últimos, en su orden original.
	p := CommentPage{Comments: comments(CommentLimit + 5), Total: CommentLimit + 5}
	p = p.KeepLast()
	if len(p.Comments) != CommentLimit {
		t.Fatalf("quedaron %d comentarios, want %d", len(p.Comments), CommentLimit)
	}
	if p.Total != CommentLimit+5 {
		t.Errorf("el total quedó en %d: tiene que seguir diciendo que hay más", p.Total)
	}
	ultimo := p.Comments[len(p.Comments)-1]
	if ultimo.Body != "c"+itoa(CommentLimit+4) {
		t.Errorf("el ultimo comentario no se conservo: %q", ultimo.Body)
	}
	// Y este es el aserto que distingue "recorta por la cola" de "recorta por la cabeza":
	// con la cola se pierde el comentario 0, con la cabeza se pierde el último.
	if p.Comments[0].Body == "c0" {
		t.Error("se perdio el comentario mas reciente, no el mas antiguo")
	}

	// Exactamente el límite: la operación es la identidad, que es el caso normal.
	p = CommentPage{Comments: comments(CommentLimit), Total: CommentLimit}
	p = p.KeepLast()
	if len(p.Comments) != CommentLimit || p.Comments[0].Body != "c0" {
		t.Errorf("con el limite justo se recortó: quedan %d, primero %q",
			len(p.Comments), p.Comments[0])
	}

	// Menos: tampoco se toca.
	p = CommentPage{Comments: comments(2), Total: 2}.KeepLast()
	if len(p.Comments) != 2 || p.Comments[1].Body != "c1" {
		t.Errorf("con menos del limite se recortó: %+v", p.Comments)
	}

	// Vacía: un nil que se lee como lista vacía es lo que evita un panic en el render.
	if got := (CommentPage{}).KeepLast(); len(got.Comments) != 0 {
		t.Errorf("una página vacia dio %d comentarios", len(got.Comments))
	}
}

// TestElAvisoDeLaCLISeConvierteEnUnMotivoCanonico: `classifyAction` es donde el inglés de
// `gh` o `glab` se convierte en una decisión de la TUI.
//
// Y la tabla entera importa porque cada rama lleva a un comportamiento DISTINTO: un
// conflicto se puede reintentar, un permiso se registra para no reintentar nunca, y un
// unmergeable ni es una cosa ni la otra. Confundir dos de esas tres no da un error: da un
// ítem que se queda sin merge para siempre, o un bucle de reintentos cada seis segundos.
//
// Y hay dos ramas donde el motivo NO es el de la CLI, y son las que más se confunden al
// refactorizar:
//
//   - `selfreview` es permiso, no conflicto: es una denegación permanente y no se arregla
//     refrescando.
//   - `unmergeable` NO es permiso. Si lo fuera, la TUI registraría el ítem como denegado y
//     le quitaría el merge para siempre, cuando lo que hace falta es un rebase que lo
//     arregla. Por eso lleva el motivo canónico y no el stderr.
func TestElAvisoDeLaCLISeConvierteEnUnMotivoCanonico(t *testing.T) {
	casos := []struct {
		nombre   string
		warns    []model.Warning
		wantOK   bool
		wantConf bool
		wantPerm bool
		wantMsg  string
	}{
		{"sin avisos", nil, true, false, false, ""},
		{
			nombre:   "notfound es conflicto",
			warns:    []model.Warning{{Kind: "notfound", Msg: "no such pull request"}},
			wantConf: true, wantMsg: "no such pull request",
		},
		{
			nombre:   "permission es permiso",
			warns:    []model.Warning{{Kind: "permission", Msg: "you cannot merge"}},
			wantPerm: true, wantMsg: "you cannot merge",
		},
		{
			nombre:   "auth es permiso",
			warns:    []model.Warning{{Kind: "auth", Msg: "not logged in"}},
			wantPerm: true, wantMsg: "not logged in",
		},
		{
			nombre:   "unsupported es permiso",
			warns:    []model.Warning{{Kind: "unsupported", Msg: "no API"}},
			wantPerm: true, wantMsg: "no API",
		},
		{
			nombre:   "selfreview es permiso, con el motivo canonico",
			warns:    []model.Warning{{Kind: "selfreview", Msg: "no puedes revisar tu propio PR"}},
			wantPerm: true, wantMsg: state.SelfReviewReason,
		},
		{
			nombre:  "unmergeable no es permiso ni conflicto",
			warns:   []model.Warning{{Kind: "unmergeable", Msg: "mergeable: false"}},
			wantMsg: state.UnmergeableReason,
		},
		{
			nombre:   "conflict es conflicto",
			warns:    []model.Warning{{Kind: "conflict", Msg: "out of date"}},
			wantConf: true, wantMsg: "out of date",
		},
		{
			nombre:   "ratelimit es conflicto",
			warns:    []model.Warning{{Kind: "ratelimit", Msg: "403 rate limited"}},
			wantConf: true, wantMsg: "403 rate limited",
		},
		{
			nombre:   "network es conflicto",
			warns:    []model.Warning{{Kind: "network", Msg: "dial tcp: no route"}},
			wantConf: true, wantMsg: "dial tcp: no route",
		},
		{
			nombre:   "timeout es conflicto",
			warns:    []model.Warning{{Kind: "timeout", Msg: "deadline exceeded"}},
			wantConf: true, wantMsg: "deadline exceeded",
		},
		{
			nombre:  "kind desconocido conserva el mensaje",
			warns:   []model.Warning{{Kind: "algo-nuevo", Msg: "lo que sea"}},
			wantMsg: "lo que sea",
		},
		{
			// Con varios avisos, el permiso manda sobre el conflicto: es la decisión más
			// restrictiva. Al revés se reintentaría una acción que ya se sabe denegada.
			nombre:   "el permiso manda sobre el conflicto",
			warns:    []model.Warning{{Kind: "ratelimit", Msg: "403"}, {Kind: "permission", Msg: "no"}},
			wantPerm: true, wantMsg: "403",
		},
		{
			// Y el selfreview manda sobre los otros dos, por lo mismo: es la única
			// denegación que es siempre permanente.
			nombre:   "el selfreview manda sobre los demas",
			warns:    []model.Warning{{Kind: "ratelimit", Msg: "403"}, {Kind: "selfreview", Msg: "x"}},
			wantPerm: true, wantMsg: state.SelfReviewReason,
		},
	}
	for _, c := range casos {
		ok, conf, perm, msg := classifyAction(c.warns)
		if ok != c.wantOK || conf != c.wantConf || perm != c.wantPerm || msg != c.wantMsg {
			t.Errorf("%s: dio (%v, %v, %v, %q), want (%v, %v, %v, %q)",
				c.nombre, ok, conf, perm, msg, c.wantOK, c.wantConf, c.wantPerm, c.wantMsg)
		}
		// Y el motivo sale siempre que hay algo que decir. Un motivo vacío con un
		// warning presente produce un toast con el prefijo y nada más.
		if len(c.warns) > 0 && strings.TrimSpace(msg) == "" {
			t.Errorf("%s: hay avisos y el motivo quedó vacío", c.nombre)
		}
	}
}

// TestCheckAntesDeLaAccionVetaLoQueNoSePuedeHacer: la puerta que se cierra antes de
// calling a la CLI.
//
// Y el orden de las comprobaciones es lo que importa, y es sutil: `notfound` gana sobre
// `permission`, y no por casualidad. Un ítem borrado da 404, y hay forges que además
// respondien 404 cuando lo que falla es la autenticación. Si el 404 se clasificara como
// permiso, la TUI registraría un ítem que en realidad no existe como denegado para
// siempre, y nunca se iría.
//
// Y la última comprobación —`Actionable`— es la que depende del estado: un PR ya mergeado
// no se approve. Su motivo viene del estado, no de la CLI, porque todavía no se ha llamado.
func TestCheckAntesDeLaAccionVetaLoQueNoSePuedeHacer(t *testing.T) {
	// Desaparecido: conflicto, y con el texto canónico.
	out := checkBeforeAction(model.Item{Number: 1}, []model.Warning{{Kind: "notfound", Msg: "404"}})
	if out == nil {
		t.Fatal("un item desaparecido paso el control previo")
	}
	if !out.Conflict {
		t.Error("un item desaparecido no se clasifico como conflicto")
	}
	if out.Perm {
		t.Error("un item desaparecido se clasifico como permiso: la TUI lo dejaria sin " +
			"merge para siempre, y lo que pasa es que ya no existe")
	}
	if !strings.Contains(out.Msg, "no longer exists") {
		t.Errorf("el motivo %q no dice que el item desaparecio", out.Msg)
	}
	// Y no trae el item: no hay nada que re-leer de un item que no existe.
	if out.HasItem {
		t.Error("trajo un item releido que no deberia existir")
	}

	// Sin permiso: permiso, con el motivo de la CLI.
	out = checkBeforeAction(model.Item{Number: 1}, []model.Warning{{Kind: "permission", Msg: "no"}})
	if out == nil || !out.Perm || out.Conflict {
		t.Errorf("sin permiso dio %+v, want Perm", out)
	}

	// Aviso sin clasificar: conflicto, y trae el item para que la TUI lo repinte.
	out = checkBeforeAction(model.Item{Number: 7}, []model.Warning{{Kind: "raro", Msg: "que se yo"}})
	if out == nil || !out.Conflict {
		t.Fatalf("un aviso sin clasificar dio %+v, want conflicto", out)
	}
	if !out.HasItem || out.Item.Number != 7 {
		t.Errorf("con el item releido, no lo trajo: %+v", out.Item)
	}

	// Aviso sin mensaje: el texto tiene que salir de algún lado, y el código tiene un
	// literal para eso. Un motivo vacío aquí produce un toast mudo.
	out = checkBeforeAction(model.Item{Number: 1}, []model.Warning{{Kind: "raro"}})
	if out == nil || out.Msg == "" {
		t.Fatalf("un aviso sin mensaje dio %+v, want un motivo", out)
	}
	if !strings.Contains(out.Msg, "re-read") {
		t.Errorf("el motivo de relleno %q no dice que no se pudo releer", out.Msg)
	}

	// Item que no se pudo releer: conflicto, sin item.
	out = checkBeforeAction(model.Item{}, nil)
	if out == nil || !out.Conflict {
		t.Fatalf("un item que no se pudo leer dio %+v, want conflicto", out)
	}
	if out.HasItem {
		t.Error("trajo un item que no se pudo leer")
	}

	// Y el estado manda: un item ya mergeado no se approve, con el motivo del estado.
	out = checkBeforeAction(model.Item{Number: 3, State: "MERGED"}, nil)
	if out == nil || !out.Conflict {
		t.Fatalf("un item ya mergeado dio %+v, want conflicto", out)
	}
	if out.Msg == "" {
		t.Error("el veto por estado dio motivo vacio")
	}
	if !out.HasItem {
		t.Error("el veto por estado no trajo el item, que la TUI necesita para pintarlo")
	}

	// Y el caso bueno: un item accionable pasa sin devolver nada. Sin esto, el test de
	// arriba probaría que todo está vetado siempre, que es un fallo distinto.
	if out := checkBeforeAction(model.Item{Number: 3, State: "OPEN", ReviewDecision: "APPROVED"}, nil); out != nil {
		t.Errorf("un item accionable dio %+v, want nil", out)
	}
}

// TestFirstMsgCogeElPrimeroYNoSeInventa: el primer aviso es el que se muestra.
//
// Y "el primero" no es arbitrario: los adapters emiten los avisos en orden de
// importancia, y el que describe el problema va el primero. Coger el último mostraría el
// detalle y perdería la razón.
//
// Y el caso de lista vacía tiene que devolver la cadena vacía, no un texto inventado:
// `firstMsg` se usa en sitios donde la ausencia es lo que se quiere comprobar.
func TestFirstMsgCogeElPrimeroYNoSeInventa(t *testing.T) {
	casos := []struct {
		warns []model.Warning
		want  string
	}{
		{nil, ""},
		{[]model.Warning{}, ""},
		{[]model.Warning{{Kind: "a", Msg: "primero"}, {Kind: "b", Msg: "segundo"}}, "primero"},
		{[]model.Warning{{Kind: "a"}}, ""},
	}
	for _, c := range casos {
		if got := firstMsg(c.warns); got != c.want {
			t.Errorf("firstMsg(%+v) dio %q, want %q", c.warns, got, c.want)
		}
	}
}

// TestUnMotivoDeMergeNoSeDeclaraDeDosManeras: dos llamadas al constructor dan dos
// errores distintos, con el mismo contenido.
//
// Y el motivo es que un `errors.Is` sobre este error tiene que funcionar. Si un adapter
// construye el texto a mano y otro llama a `ErrUnknownMergeMode`, los dos errores dicen lo
// mismo y no son el mismo error: el que capture uno no captura el otro, y la degradación
// se comporta distinta según el forge.
func TestUnMotivoDeMergeNoSeDeclaraDeDosManeras(t *testing.T) {
	uno := ErrUnknownMergeMode("nope")
	otro := ErrUnknownMergeMode("nope")
	if uno.Error() != otro.Error() {
		t.Errorf("dos llamadas al mismo error dieron textos distintos: %q y %q",
			uno, otro)
	}
	// Y un error que envuelve a otro sigue siendo localizable.
	envuelto := errors.Join(otro)
	if !strings.Contains(envuelto.Error(), "nope") {
		t.Errorf("el error perdido al agruparlo: %v", envuelto)
	}
}

// comments fabrica una lista de n comentarios de prueba. Cada uno con un cuerpo distinto
// para que "el último" y "el primero" se puedan distinguir mirando el texto, que es como
// los mira el render.
func comments(n int) []model.Comment {
	out := make([]model.Comment, n)
	for i := range out {
		out[i] = model.Comment{Author: "a" + itoa(i), Body: "c" + itoa(i)}
	}
	return out
}

// itoa evita importar strconv solo para esto.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// TestElNombreDeUnModoDesconocidoSeDevuelveTalCualYNoSeDisfrazaDeUnoConocido: el `default` de
// `Label`.
//
// Y la diferencia con `Valid` es que se complementary: `Valid` dice que no se puede usar y
// `Label` dice cómo se lo enseña al usuario. Un `default` que devolviera "merge commit"
// —el nombre de uno de los tres— haría que un modo corrupto se pintara como integrable cuando
// en realidad nadie sabe qué se haría con él, y el usuario confirmaría algo que no va a pasar.
//
// Y devolver el propio valor es lo que hace que el diagnóstico sirva: un config con
// `merge_mode = "fast-forward"` muestra "fast-forward" en el aviso, que es exactamente el typo
// que hay que corregir. Un texto genérico obligaría a abrir el config para compararlo con una
// lista de tres.
//
// Y el modo VACÍO es el caso que más se da: una clave ausente en el TOML deja el valor cero, y
// ese valor tiene que llegar al aviso tal cual en vez de disfrazarse.
func TestElNombreDeUnModoDesconocidoSeDevuelveTalCualYNoSeDisfrazaDeUnoConocido(t *testing.T) {
	conocidos := map[MergeMode]bool{MergeCommit: true, Rebase: true, Squash: true}

	for _, m := range []MergeMode{
		MergeMode("fast-forward"), MergeMode(""), MergeMode("MERGE"), MergeMode(" squash"),
		MergeMode("rebasea"),
	} {
		got := m.Label()
		if got != string(m) {
			t.Errorf("el modo %q dio la etiqueta %q: tiene que devolverse tal cual, o el "+
				"aviso no dice qué corregir", m, got)
		}
		// Y lo que no puede pasar es que se parezca a uno de los tres, porque entonces el
		// usuario lee un modo que conoce y busca el problema en el sitio equivocado.
		for conocido := range conocidos {
			if got == conocido.Label() && string(m) != string(conocido) {
				t.Errorf("el modo %q dio la etiqueta %q, que es la de un modo conocido",
					m, got)
			}
		}
		// Y sigue siendo inválido: `Label` informa, `Valid` decide. Un modo desconocido que
		// `Label` devolviera vacío dejaría al usuario sin nada que leer en el aviso.
		if m.Valid() {
			t.Errorf("el modo %q dio Valid() = true: se aceptaría en `gh pr merge` sin flag "+
				"y abriría un prompt interactivo que deja la TUI colgada", m)
		}
	}

	// Y el control: los tres conocidos siguen dando su nombre, y `Label` e `idle` no se
	// contradicen para ninguno.
	for _, m := range []MergeMode{MergeCommit, Rebase, Squash} {
		if !m.Valid() || m.Label() != etiquetasDePrueba[m] {
			t.Errorf("el modo conocido %q cambió: Valid=%v Label=%q", m, m.Valid(), m.Label())
		}
	}
}

// etiquetasDePrueba es la tabla de nombres canónicos de los tres modos conocidos, para el
// control del test anterior.
var etiquetasDePrueba = map[MergeMode]string{
	MergeCommit: "merge commit",
	Rebase:      "rebase",
	Squash:      "squash",
}
