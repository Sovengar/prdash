package herdr

import (
	"strings"
	"testing"
)

// The CLI's envelope is {"id":...,"result":{...}} and EVERY command returns a .result.

const envelopeErroneo = `esto no es json`

// Each parser goes through decodeResult, so one covering only its own cases exercises a branch the
// other seven share.
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

// Valid JSON missing the key that makes the result mean something.
func TestLosParsersExigenSuClaveObligatoria(t *testing.T) {
	casos := []struct {
		nombre   string
		json     string
		quiereEn string
		parsear  func([]byte) error
	}{
		{
			nombre: "worktreeCreated sin path",
			json:   `{"result":{"workspace":{"workspace_id":"w1"},"worktree":{"branch":"b"}}}`,
			// The error has to NAME the missing key.
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

// An empty list is a legitimate reply, not a broken one.
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
	if lista.RepoRoot != "/r" {
		t.Errorf("con lista vacia, RepoRoot = %q, want /r", lista.RepoRoot)
	}
}

// Accepting both forms is because the exact server shape is unverified.
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
			// The empty message is a real case: a server error may carry only the code.
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

func TestElErrorDelRPCSinMensajeEsElCodigo(t *testing.T) {
	if got := (&rpcError{Code: "timeout"}).Error(); got != "timeout" {
		t.Errorf("un rpcError sin mensaje dio %q, want timeout", got)
	}
	if got := (&rpcError{Code: "timeout", Message: "tardo"}).Error(); got != "timeout: tardo" {
		t.Errorf("un rpcError con mensaje dio %q, want \"timeout: tardo\"", got)
	}
}

// Both halves are used, which is why both are parsed.
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
		// Two components is not a version. Returning 0.9 would invent a patch that is not there.
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
		if strings.ContainsAny(got.Raw, "\n") {
			t.Errorf("parseVersion(%q) dejó el salto en Raw=%q", c.entrada, got.Raw)
		}
		if got.Raw == "" {
			t.Errorf("parseVersion(%q) devolvio Raw vacío: el diagnóstico lo necesita", c.entrada)
		}
	}
}

// The clipping to the first line.
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
