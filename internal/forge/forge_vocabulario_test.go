package forge

import (
	"errors"
	"strings"
	"testing"

	"prdash/internal/forge/model"
	"prdash/internal/state"
)

func TestElModoDeMergeSeValidaAntesDeConstruirElArgv(t *testing.T) {
	// The labels are NOT the mode names in the three cases: MergeCommit is confirmed as "merge
	//commit", because that is what the user has to recognise.
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
		// The valid list holds NAMES, not labels, and it matters: the error says "expected merge, rebase
		//or squash" and that is what someone compares against.
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

// "By the tail" is what distinguishes this from a `[:limit]`.
func TestKeepLastRecortaPorLaCola(t *testing.T) {
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
	if p.Comments[0].Body == "c0" {
		t.Error("se perdio el comentario mas reciente, no el mas antiguo")
	}

	p = CommentPage{Comments: comments(CommentLimit), Total: CommentLimit}
	p = p.KeepLast()
	if len(p.Comments) != CommentLimit || p.Comments[0].Body != "c0" {
		t.Errorf("con el limite justo se recortó: quedan %d, primero %q",
			len(p.Comments), p.Comments[0])
	}

	p = CommentPage{Comments: comments(2), Total: 2}.KeepLast()
	if len(p.Comments) != 2 || p.Comments[1].Body != "c1" {
		t.Errorf("con menos del limite se recortó: %+v", p.Comments)
	}

	if got := (CommentPage{}).KeepLast(); len(got.Comments) != 0 {
		t.Errorf("una página vacia dio %d comentarios", len(got.Comments))
	}
}

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
			nombre:   "el permiso manda sobre el conflicto",
			warns:    []model.Warning{{Kind: "ratelimit", Msg: "403"}, {Kind: "permission", Msg: "no"}},
			wantPerm: true, wantMsg: "403",
		},
		{
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
		if len(c.warns) > 0 && strings.TrimSpace(msg) == "" {
			t.Errorf("%s: hay avisos y el motivo quedó vacío", c.nombre)
		}
	}
}

// The ORDER of the checks is what matters and it is subtle: `notfound` wins.
func TestCheckAntesDeLaAccionVetaLoQueNoSePuedeHacer(t *testing.T) {
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
	if out.HasItem {
		t.Error("trajo un item releido que no deberia existir")
	}

	out = checkBeforeAction(model.Item{Number: 1}, []model.Warning{{Kind: "permission", Msg: "no"}})
	if out == nil || !out.Perm || out.Conflict {
		t.Errorf("sin permiso dio %+v, want Perm", out)
	}

	out = checkBeforeAction(model.Item{Number: 7}, []model.Warning{{Kind: "raro", Msg: "que se yo"}})
	if out == nil || !out.Conflict {
		t.Fatalf("un aviso sin clasificar dio %+v, want conflicto", out)
	}
	if !out.HasItem || out.Item.Number != 7 {
		t.Errorf("con el item releido, no lo trajo: %+v", out.Item)
	}

	out = checkBeforeAction(model.Item{Number: 1}, []model.Warning{{Kind: "raro"}})
	if out == nil || out.Msg == "" {
		t.Fatalf("un aviso sin mensaje dio %+v, want un motivo", out)
	}
	if !strings.Contains(out.Msg, "re-read") {
		t.Errorf("el motivo de relleno %q no dice que no se pudo releer", out.Msg)
	}

	out = checkBeforeAction(model.Item{}, nil)
	if out == nil || !out.Conflict {
		t.Fatalf("un item que no se pudo leer dio %+v, want conflicto", out)
	}
	if out.HasItem {
		t.Error("trajo un item que no se pudo leer")
	}

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

	if out := checkBeforeAction(model.Item{Number: 3, State: "OPEN", ReviewDecision: "APPROVED"}, nil); out != nil {
		t.Errorf("un item accionable dio %+v, want nil", out)
	}
}

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

// The reason is that an errors.Is on this error is.
func TestUnMotivoDeMergeNoSeDeclaraDeDosManeras(t *testing.T) {
	uno := ErrUnknownMergeMode("nope")
	otro := ErrUnknownMergeMode("nope")
	if uno.Error() != otro.Error() {
		t.Errorf("dos llamadas al mismo error dieron textos distintos: %q y %q",
			uno, otro)
	}
	envuelto := errors.Join(otro)
	if !strings.Contains(envuelto.Error(), "nope") {
		t.Errorf("el error perdido al agruparlo: %v", envuelto)
	}
}

func comments(n int) []model.Comment {
	out := make([]model.Comment, n)
	for i := range out {
		out[i] = model.Comment{Author: "a" + itoa(i), Body: "c" + itoa(i)}
	}
	return out
}

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

// Complementary to Valid: Valid says it cannot be done, Label returns the name as given. Disguising
// an unknown mode as a known one would have the UI confirm something the forge will reject.
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
		for conocido := range conocidos {
			if got == conocido.Label() && string(m) != string(conocido) {
				t.Errorf("el modo %q dio la etiqueta %q, que es la de un modo conocido",
					m, got)
			}
		}
		if m.Valid() {
			t.Errorf("el modo %q dio Valid() = true: se aceptaría en `gh pr merge` sin flag "+
				"y abriría un prompt interactivo que deja la TUI colgada", m)
		}
	}

	for _, m := range []MergeMode{MergeCommit, Rebase, Squash} {
		if !m.Valid() || m.Label() != etiquetasDePrueba[m] {
			t.Errorf("el modo conocido %q cambió: Valid=%v Label=%q", m, m.Valid(), m.Label())
		}
	}
}

var etiquetasDePrueba = map[MergeMode]string{
	MergeCommit: "merge commit",
	Rebase:      "rebase",
	Squash:      "squash",
}
