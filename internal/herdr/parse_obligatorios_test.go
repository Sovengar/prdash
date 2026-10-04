package herdr

import (
	"strings"
	"testing"
)

// El sobre de la CLI es `{"id":...,"result":{...}}`, y TODO comando devuelve un `.result`.
// Los tests de este fichero cubren la mitad que no se cubría: no el camino feliz —que ya
// está en parse_test.go— sino el que se llega cuando la respuesta llega rota.
//
// Y por qué esa mitad importa: un `.result` sin el campo obligatorio NO es un caso raro,
// es lo que pasa cuando Herdr cambia una clave entre versiones, cuando el comando falla
// a medias, o cuando lo que llega por stdout es un aviso en vez de un sobre. En las tres
// situaciones el programa tiene quedejar CON SU TEXTO, no devolver una struct a
// cero y dejar que el llamador monte un review sobre un pane que no existe.

// envelopeErroneo es el JSON que no se puede deserializar: hay texto donde va el objeto.
const envelopeErroneo = `esto no es json`

// TestLosParsersRechazanSobreIlegible: cada parser pasa por `decodeResult`, así que uno
// solo con el resto de sus casos probaría una rama que los otros siete heredan sin probar.
//
// Y la comprobación útil no es solo que den error: es que el error diga de dónde vino,
// porque en un flujo de montaje de review hay cuatro comandos seguidas y "respuesta
// ilegible" a secas no dice cuál. Aquí se comprueba que el mensaje trae el nombre del
// parser, que es lo que un humano necesita para saber qué comando fue.
func TestLosParsersRechazanSobreIlegible(t *testing.T) {
	casos := []struct {
		nombre  string
		parsear func([]byte) error
	}{
		{"worktreeCreated", func(b []byte) error { _, err := parseWorktreeCreated(b); return err }},
		{"worktreeList", func(b []byte) error { _, err := parseWorktreeList(b); return err }},
		{"workspaceCreated", func(b []byte) error { _, err := parseWorkspaceCreated(b); return err }},
		{"tabCreated", func(b []byte) error { _, err := parseTabCreated(b); return err }},
		{"paneSplit", func(b []byte) error { _, err := parsePaneSplit(b); return err }},
		{"paneList", func(b []byte) error { _, err := parsePaneList(b); return err }},
		{"notification", func(b []byte) error {
			_, _, err := parseNotification(b)
			return err
		}},
	}
	for _, c := range casos {
		err := c.parsear([]byte(envelopeErroneo))
		if err == nil {
			t.Errorf("%s no dio error con un sobre ilegible", c.nombre)
			continue
		}
		if !strings.Contains(err.Error(), "ilegible") {
			t.Errorf("%s dio %q, que no dice que el sobre era ilegible", c.nombre, err)
		}
	}
}

// TestLosParsersExigenSuClaveObligatoria: el sobre es JSON válido pero le falta la clave
// que hace que el resultado signifique algo.
//
// Y aquí está el caso que más caro sale si no se comprueba: un `worktree create` que
// responde bien pero sin `.result.worktree.path` deja que el montaje siga adelante. La
// ruta vacía acaba en un worktree del repo principal —donde vive el código del usuario—,
// y el review se monta encima. Por eso el parser lo para en vez de devolver la struct a
// cero y dejar que el llamador no mire.
func TestLosParsersExigenSuClaveObligatoria(t *testing.T) {
	// Sobre válido, resultado presente, clave que no.
	casos := []struct {
		nombre   string
		json     string
		quiereEn string
		parsear  func([]byte) error
	}{
		{
			nombre: "worktreeCreated sin path",
			json:   `{"result":{"workspace":{"workspace_id":"w1"},"worktree":{"branch":"b"}}}`,
			// El error tiene que NOMBRAR la clave que falta. Un "campo obligatorio"
			// genérico deja al que lee el log con tres parsers de por medio sin saber
			// cuál se cayó.
			quiereEn: "worktree.path",
			parsear:  func(b []byte) error { _, err := parseWorktreeCreated(b); return err },
		},
		{
			nombre:   "workspaceCreated sin workspace_id",
			json:     `{"result":{"tab":{"tab_id":"t1"}}}`,
			quiereEn: "workspace.workspace_id",
			parsear:  func(b []byte) error { _, err := parseWorkspaceCreated(b); return err },
		},
		{
			nombre:   "paneSplit sin pane_id",
			json:     `{"result":{"pane":{"workspace_id":"w1"}}}`,
			quiereEn: "pane.pane_id",
			parsear:  func(b []byte) error { _, err := parsePaneSplit(b); return err },
		},
	}
	for _, c := range casos {
		err := c.parsear([]byte(c.json))
		if err == nil {
			t.Errorf("%s: el parser acepto un resultado sin la clave obligatoria", c.nombre)
			continue
		}
		if !strings.Contains(err.Error(), c.quiereEn) {
			t.Errorf("%s: el error fue %q y no nombra %q, que es lo que hay que arreglar",
				c.nombre, err, c.quiereEn)
		}
	}
}

// TestUnaListaVaciaNoEsUnError: `pane list` y `worktree list` con cero elementos es una
// respuesta legítima, no una respuesta rota.
//
// Y esto es lo que separa este fichero del anterior: los parsers con clave obligatoria
// (worktree create, workspace create, pane split) la exigen porque un resultado sin ella
// no identifica nada. Los de lista NO pueden, porque lista vacía es un estado real: un
// repo sin worktrees es un repo recién clonado, y tratarlo como error haría que el inbox
// aparecería como roto en el primer arranque.
func TestUnaListaVaciaNoEsUnError(t *testing.T) {
	panes, err := parsePaneList([]byte(`{"result":{"panes":[]}}`))
	if err != nil {
		t.Fatalf("una lista de panes vacia dio error: %v", err)
	}
	if len(panes) != 0 {
		t.Errorf("una lista vacia devolvio %d panes", len(panes))
	}

	lista, err := parseWorktreeList([]byte(`{"result":{"source":{"repo_root":"/r"},"worktrees":[]}}`))
	if err != nil {
		t.Fatalf("una lista de worktrees vacia dio error: %v", err)
	}
	if len(lista.Worktrees) != 0 {
		t.Errorf("una lista vacia devolvio %d worktrees", len(lista.Worktrees))
	}
	// Y el origen se lee igual con lista vacía, que es de donde sale la raíz del repo
	// que luego usan las rutas. Si esto se perdiera, el listado vacío sería inútil
	// justo en el caso en que se da.
	if lista.RepoRoot != "/r" {
		t.Errorf("con lista vacia, RepoRoot = %q, want /r", lista.RepoRoot)
	}
}

// TestParseServerErrorAceptaLasDosFormas: el error del servidor puede venir anidado o
// plano, y el parser acepta ambos.
//
// Y el motivo de aceptar dos formas está en el propio comentario del código: la forma
// exacta no está verificada. Es un parser *best-effort* sobre una API de la que no se
// controla la versión. Lo que no puede ser es tirar: si el parser rechazara una forma no
// verificada, un cambio de versión en el servidor convertiría todos los errores en
// errores opacos "no se pudo leer", que es el peor resultado para alguien depurando.
//
// Y por eso el caso interesante es el último: algo que es JSON pero no tiene ni `error`
// ni `code`. Ahí la respuesta correcta es no inventar código ni mensaje.
func TestParseServerErrorAceptaLasDosFormas(t *testing.T) {
	casos := []struct {
		nombre   string
		stderr   string
		wantCode string
		wantMsg  string
	}{
		{
			nombre:   "anidado",
			stderr:   `{"error":{"code":"pane_gone","message":"el pane ya no existe"}}`,
			wantCode: "pane_gone",
			wantMsg:  "el pane ya no existe",
		},
		{
			nombre:   "plano",
			stderr:   `{"code":"pane_gone","message":"el pane ya no existe"}`,
			wantCode: "pane_gone",
			wantMsg:  "el pane ya no existe",
		},
		{
			nombre: "anidado con codigo y sin mensaje",
			// El mensaje vacío es un caso real: un error de servidor puede traer solo el
			// código. Perder el código por no haber mensaje sería tirar la única
			// información que hay.
			stderr:   `{"error":{"code":"timeout"}}`,
			wantCode: "timeout",
			wantMsg:  "",
		},
		{
			nombre:   "json que no es un error de servidor",
			stderr:   `{"result":{"ok":true}}`,
			wantCode: "",
			wantMsg:  "",
		},
		{
			nombre:   "array json",
			stderr:   `[1,2,3]`,
			wantCode: "",
			wantMsg:  "",
		},
		{
			nombre:   "texto libre",
			stderr:   `error: no such pane`,
			wantCode: "",
			wantMsg:  "",
		},
		{
			nombre:   "vacio",
			stderr:   `   `,
			wantCode: "",
			wantMsg:  "",
		},
	}
	for _, c := range casos {
		code, msg := parseServerError([]byte(c.stderr))
		if code != c.wantCode || msg != c.wantMsg {
			t.Errorf("%s: parseServerError dio (%q, %q), want (%q, %q)",
				c.nombre, code, msg, c.wantCode, c.wantMsg)
		}
	}
}

// TestElErrorDelRPCSinMensajeEsElCodigo: un error del servidor puede traer solo el
// código, y el texto tiene que ser algo legible igualmente.
//
// Y esto no es un detalle de presentación: si el mensaje acaba siendo un `:` suelto
// o un código vacío, el toast que ve el usuario dice "error: " o nada, y no hay forma de
// buscarlo. El código ES la información, así que cuando falta el mensaje el código
// ocupa su lugar.
func TestElErrorDelRPCSinMensajeEsElCodigo(t *testing.T) {
	if got := (&rpcError{Code: "timeout"}).Error(); got != "timeout" {
		t.Errorf("un rpcError sin mensaje dio %q, want timeout", got)
	}
	if got := (&rpcError{Code: "timeout", Message: "tardo"}).Error(); got != "timeout: tardo" {
		t.Errorf("un rpcError con mensaje dio %q, want \"timeout: tardo\"", got)
	}
}

// TestParseVersionSeparaElNumeroDelTexto: de "herdr 0.9.1-preview.7" sale la versión
// numérica Y el texto entero.
//
// Y las dos mitades se usan para cosas distintas, que es por lo que el parser devuelve
// las dos. El número decide si la app puede montar un review —`Available()` compara
// contra `MinVersion`—. El texto se muestra en el diagnóstico de degradación, donde el
// usuario necesita el `-preview` para saber que está en una build de desarrollo.
//
// Y el caso del que más se cuelga: una versión con prefijo `v`, como la de un binario
// instalado por un gestor de paquetes. Si el regex exigiera el dígito al principio del
// texto, `v0.9.1` no casaría y la app creería que no sabe su versión —que es el mismo
// resultado que si la CLI no estuviera, o sea, se degradaría sin avisar.
func TestParseVersionSeparaElNumeroDelTexto(t *testing.T) {
	casos := []struct {
		entrada string
		want    Version
		wantOK  bool
	}{
		{"herdr 0.9.1-preview.7\n", Version{Major: 0, Minor: 9, Patch: 1}, true},
		{"herdr v1.2.3\n", Version{Major: 1, Minor: 2, Patch: 3}, true},
		{"0.10.0", Version{Major: 0, Minor: 10, Patch: 0}, true},
		{"herdr 2.0.0", Version{Major: 2, Minor: 0, Patch: 0}, true},
		{"herdr unknown", Version{}, false},
		{"", Version{}, false},
		{"herdr", Version{}, false},
		// Solo dos componentes: no es una versión. Devolver 0.9 sería inventar un patch
		// que no está, y comparar contra MinVersion con un patch inventado decide si la
		// app puede montar un review con un dato falso.
		{"herdr 0.9", Version{}, false},
	}
	for _, c := range casos {
		got, ok := parseVersion([]byte(c.entrada))
		if ok != c.wantOK {
			t.Errorf("parseVersion(%q) dio ok=%v, want %v", c.entrada, ok, c.wantOK)
			continue
		}
		if !c.wantOK {
			continue
		}
		if got.Major != c.want.Major || got.Minor != c.want.Minor || got.Patch != c.want.Patch {
			t.Errorf("parseVersion(%q) dio %d.%d.%d, want %d.%d.%d", c.entrada,
				got.Major, got.Minor, got.Patch, c.want.Major, c.want.Minor, c.want.Patch)
		}
		// Y el texto va limpio, sin el salto de línea final: es lo que se muestra.
		if strings.ContainsAny(got.Raw, "\n") {
			t.Errorf("parseVersion(%q) dejó el salto en Raw=%q", c.entrada, got.Raw)
		}
		if got.Raw == "" {
			t.Errorf("parseVersion(%q) devolvio Raw vacío: el diagnóstico lo necesita", c.entrada)
		}
	}
}

// TestFirstLineSeQuedaConLaPrimera: el primer aviso de un binario es el que explica, y a
// veces llega con más líneas debajo que no.
//
// Y el recorte tiene que respetar una cosa: si la primera línea está EN BLANCO y la
// segunda es la que explica, quedarse con la vacía produce un toast sin texto. Por eso
// el orden es trim general, luego corte. Al revés, cortar y luego trim solo, deja la
// primera línea en blanco si el binario empieza con un salto.
func TestFirstLineSeQuedaConLaPrimera(t *testing.T) {
	casos := []struct {
		entrada string
		want    string
	}{
		{"aviso importante\notra cosa\n", "aviso importante"},
		{"  aviso  \n  otra  ", "aviso"},
		{"solo una linea", "solo una linea"},
		{"", ""},
		{"\n\nla tercera", "la tercera"},
		{"con\n", "con"},
	}
	for _, c := range casos {
		if got := firstLine(c.entrada); got != c.want {
			t.Errorf("firstLine(%q) dio %q, want %q", c.entrada, got, c.want)
		}
	}
}

// TestHaveGraphicsTargetExigeLosDos: sin socket no hay a quién preguntar, y sin pane no
// hay dónde poner la imagen. Faltando cualquiera de los dos, la petición no se manda.
func TestHaveGraphicsTargetExigeLosDos(t *testing.T) {
	casos := []struct {
		socket, pane string
		want         bool
	}{
		{"/tmp/sock", "p1", true},
		{"", "p1", false},
		{"/tmp/sock", "", false},
		{"", "", false},
	}
	for _, c := range casos {
		if got := haveGraphicsTarget(c.socket, c.pane); got != c.want {
			t.Errorf("haveGraphicsTarget(%q, %q) dio %v, want %v", c.socket, c.pane, got, c.want)
		}
	}
}
