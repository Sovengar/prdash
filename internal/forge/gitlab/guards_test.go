package gitlab

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
)

func glabQueRegistra(t *testing.T, cuerpo string) (script, argsFile string) {
	t.Helper()
	dir := t.TempDir()
	argsFile = filepath.Join(dir, "args.log")
	script = writeScript(t, dir, "glab", "#!/bin/sh\necho \"$@\" >> \""+argsFile+"\"\n"+cuerpo)
	return script, argsFile
}

func argsRegistrados(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("no se registró ninguna llamada: %v", err)
	}
	return string(raw)
}

// The Todos API's cursor is the page NUMBER, and it is only used if it really is a number.
func TestElCursorDeLaAPIEsElNumeroDePaginaYSoloSiEsUnNumero(t *testing.T) {
	casos := []struct {
		cursor string
		quiere string
	}{
		{"", "page=1"},
		{"1", "page=1"},
		{"2", "page=2"},
		{"7", "page=7"},
		{"999", "page=999"},
		{"0", "page=1"},
		{"-1", "page=1"},
		{"-99", "page=1"},
		{"abc", "page=1"},
		{"1abc", "page=1"},
		{"1.5", "page=1"},
		{" 1", "page=1"},
		{"1 ", "page=1"},
		{"0x10", "page=1"},
		{"+1", "page=1"},
	}

	for _, c := range casos {
		t.Run("cursor="+c.cursor, func(t *testing.T) {
			script, argsFile := glabQueRegistra(t, `echo '[]'
`)
			a := New("h.example", script)
			a.List(context.Background(), forge.Query{Section: model.SectionMentions, Cursor: c.cursor})

			log := argsRegistrados(t, argsFile)
			if !strings.Contains(log, c.quiere) {
				t.Errorf("con el cursor %q se pidió %q y se esperaba %q:\n%s", c.cursor, c.quiere, c.quiere, log)
			}
			// ONE page was asked for, not two. Careful counting: the args also carry "per_page=50".
			if n := strings.Count(log, " -f page="); n != 1 {
				t.Errorf("con el cursor %q se pidieron %d páginas:\n%s", c.cursor, n, log)
			}
		})
	}
}

// When the server sends a reason, the warning teaches it.
func TestUn400DeGitLabDiceLoQueDiceElServidor(t *testing.T) {
	script, _ := glabQueRegistra(t, `echo "{\"message\":\"Reference does not exist on the remote\"}"
exit 1
`)
	a := New("h.example", script)
	warns := a.Retarget(context.Background(), model.RepoRef{Project: "grupo/proy"}, 3, "main")
	if len(warns) == 0 {
		t.Fatal("un runner que falla no dio ningún aviso")
	}
	if !strings.Contains(warns[0].Msg, "does not exist on the remote") {
		t.Errorf("el aviso no menciona lo que dijo el servidor: %q", warns[0].Msg)
	}
	// And it does not say only the exit code, which is exactly what has to be avoided.
	if strings.TrimSpace(warns[0].Msg) == "exit status 1" {
		t.Errorf("el aviso es solo el código de salida: %q", warns[0].Msg)
	}

	script, _ = glabQueRegistra(t, `exit 1
`)
	a = New("h.example", script)
	warns = a.Retarget(context.Background(), model.RepoRef{Project: "grupo/proy"}, 3, "main")
	if len(warns) == 0 {
		t.Fatal("un runner que falla sin cuerpo no dio ningún aviso")
	}
	if strings.TrimSpace(warns[0].Msg) == "" {
		t.Error("el aviso quedó vacío: un fallo sin texto no informa de nada")
	}

	script, _ = glabQueRegistra(t, `echo 'no soy json'
exit 1
`)
	a = New("h.example", script)
	warns = a.Retarget(context.Background(), model.RepoRef{Project: "grupo/proy"}, 3, "main")
	if len(warns) == 0 || strings.TrimSpace(warns[0].Msg) == "" {
		t.Errorf("con un cuerpo ilegible el aviso quedó vacío: %+v", warns)
	}
}

// The warning is "unsupported", not a network one: nothing was sent.
func TestRetargetSinRamaBaseNoSaleALaRed(t *testing.T) {
	for _, rama := range []string{"", "   ", "\t\n"} {
		script, argsFile := glabQueRegistra(t, `echo '{}'
`)
		a := New("h.example", script)
		warns := a.Retarget(context.Background(), model.RepoRef{Project: "grupo/proy"}, 3, rama)
		if len(warns) == 0 {
			t.Errorf("rama %q: no salió ningún aviso", rama)
			continue
		}
		if warns[0].Kind != "unsupported" {
			t.Errorf("rama %q: el aviso es de tipo %q, want unsupported: la peticion no tiene sentido, no es un fallo de la red",
				rama, warns[0].Kind)
		}
		if _, err := os.Stat(argsFile); err == nil {
			t.Errorf("rama %q: se salio a la red:\n%s", rama, argsRegistrados(t, argsFile))
		}
	}
	script, argsFile := glabQueRegistra(t, `echo '{}'
`)
	a := New("h.example", script)
	_ = a.Retarget(context.Background(), model.RepoRef{Project: "grupo/proy"}, 3, "main")
	if _, err := os.Stat(argsFile); err != nil {
		t.Error("con rama no se salio a la red: el guard se trago la llamada buena")
	}
}

// A guard before the call, so it has to be a "not found", not a failure.
func TestUnaReferenciaVaciaNoSaleALaRed(t *testing.T) {
	script, argsFile := glabQueRegistra(t, `echo '{}'
`)
	a := New("h.example", script)

	_, warns := a.ItemState(context.Background(), model.RepoRef{Project: ""}, 1)
	if len(warns) == 0 {
		t.Fatal("una referencia vacía no dio ningún aviso: debería decir que no hay a qué preguntar")
	}
	if warns[0].Kind != "notfound" {
		t.Errorf("el aviso es de tipo %q, want notfound: no es que falte el MR, es que no hay a cuál preguntar",
			warns[0].Kind)
	}
	// And it did not reach the network, which is the point of the guard.
	if _, err := os.Stat(argsFile); err == nil {
		t.Errorf("una referencia vacida salió a la red:\n%s", argsRegistrados(t, argsFile))
	}

	_, _ = a.ItemState(context.Background(), model.RepoRef{Project: "grupo/proy"}, 1)
	if _, err := os.Stat(argsFile); err != nil {
		t.Error("con referencia no salió a la red: el guard se tragó la llamada buena")
	}
}

func TestElStampDeGitLabPoneElReviewKindSoloEnReview(t *testing.T) {
	for _, seccion := range []model.Section{model.SectionReview, model.SectionAuthored, model.SectionMentions} {
		a := New("h.example", "glab")
		items := []model.Item{{Number: 1, Ref: model.RepoRef{
			Forge: ForgeName, Host: "h.example", Project: "grupo/proy", Owner: "grupo", Name: "proy"}}}
		a.stamp(items, forge.Query{Section: seccion, ReviewKind: model.ReviewRequested})

		tiene := items[0].ReviewKind != ""
		quiere := seccion == model.SectionReview
		if tiene != quiere {
			t.Errorf("sección %v: ReviewKind %q presente=%v, quiere %v",
				seccion, items[0].ReviewKind, tiene, quiere)
		}
		// The section is always stamped: without it the item does not know which list it belongs to.
		if items[0].Section != seccion {
			t.Errorf("sección %v: quedó estampada como %v", seccion, items[0].Section)
		}
	}
}
