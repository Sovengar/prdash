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

// glabQueRegistra es un `glab` falso que anota los args en un fichero y contesta con
// lo que se le pase. El registro es lo que convierte "se llamó con page=3" en un
// hecho en vez de una inferencia.
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

// TestElCursorDeLaAPIEsElNumeroDePaginaYSoloSiEsUnNumero: el cursor de la API de
// Todos es el número de página, y solo se usa si de verdad es un número.
//
// El cursor viene de la caché y de "cargar más", o sea de fuera: un cursor
// manipulado, un cursor de otra versión del formato, o un cursor vacío. Y la
// degradación tiene que ser a la PRIMERA página, no a la última ni a ninguna.
//
// Es un guard con dos condiciones, y las dos importan por separado:
//
//   - "no es un número" (Atoi falla) se va a la primera. Un cursor de texto no puede
//     convertirse en un número de página, y adivinarlo abriría una página al azar.
//   - "es cero o negativo" también se va a la primera, porque `page=0` en la API de
//     Todos es una página que no existe, y lo que sale de ahí es un error del
//     servidor en vez de la lista.
//
// Se afirma por los ARGS, que es donde se ve: el número de página que se pidió.
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
		// Cero y negativos: no son páginas que existan.
		{"0", "page=1"},
		{"-1", "page=1"},
		{"-99", "page=1"},
		// Texto: no se puede convertir.
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
			// La lista de "todos" es la de menciones, que es la que pagina.
			a.List(context.Background(), forge.Query{Section: model.SectionMentions, Cursor: c.cursor})

			log := argsRegistrados(t, argsFile)
			if !strings.Contains(log, c.quiere) {
				t.Errorf("con el cursor %q se pidió %q y se esperaba %q:\n%s", c.cursor, c.quiere, c.quiere, log)
			}
			// Y se pidió UNA página, no dos. Ojo al contar: los args llevan
			// "per_page=50" también, así que buscar "page=" a secas contaría dos y
			// el test pasaría siempre. El marcador lleva el espacio del flag.
			if n := strings.Count(log, " -f page="); n != 1 {
				t.Errorf("con el cursor %q se pidieron %d páginas:\n%s", c.cursor, n, log)
			}
		})
	}
}

// TestUn400DeGitLabDiceLoQueDiceElServidor: cuando el servidor manda un motivo, el
// aviso lo enseña.
//
// `glab` sale con código distinto de cero y escribe el error en stdout, no en stderr.
// El motivo sale del CUERPO de la respuesta, y eso es lo que lo hace accionable: un
// 400 de GitLab con "Reference 'x' does not exist" dice qué hacer, mientras que
// "`glab api -X PUT ... (exit 1)" no dice nada.
//
// Es el caso del retarget a una rama que no existe, que es el uso real de esta vía, y
// el motivo por el que el código mira el cuerpo y no stderr: en stderr solo llega el
// argv.
//
// Y si el cuerpo no trae un motivo reconocible, se queda el del error, que al menos
// es un hecho. Un aviso nunca queda con el texto de un código de salida y nada más.
func TestUn400DeGitLabDiceLoQueDiceElServidor(t *testing.T) {
	// Con cuerpo de API en stdout y salida con error.
	// El mensaje lleva comillas simples y el script va en un heredoc de Go, así que
	// se escriben con \' escapado para que la shell no las tome por sintaxis.
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
	// Y no dice solo el código de salida, que es justo lo que hay que evitar.
	if strings.TrimSpace(warns[0].Msg) == "exit status 1" {
		t.Errorf("el aviso es solo el código de salida: %q", warns[0].Msg)
	}

	// Sin cuerpo reconocible: se queda el del error, que no está vacío.
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

	// Y un 400 que NO lleva cuerpo: el mensaje del error, que al menos nombra la
	// operación. Un cuerpo vacío o ilegible no puede pisar un mensaje que sí dice
	// algo.
	script, _ = glabQueRegistra(t, `echo 'no soy json'
exit 1
`)
	a = New("h.example", script)
	warns = a.Retarget(context.Background(), model.RepoRef{Project: "grupo/proy"}, 3, "main")
	if len(warns) == 0 || strings.TrimSpace(warns[0].Msg) == "" {
		t.Errorf("con un cuerpo ilegible el aviso quedó vacío: %+v", warns)
	}
}

// TestRetargetSinRamaBaseNoSaleALaRed: sin rama destino no hay nada que poner, y se
// dice antes de tocar la red.
//
// El aviso es "unsupported" y no de red, porque no es un fallo de la API: es que la
// petición no tiene sentido. Confundir los dos hace que el usuario vaya a mirar la
// conexión cuando el problema es que no eligió rama.
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
	// Y con rama sí sale: el guard no se come la llamada buena.
	script, argsFile := glabQueRegistra(t, `echo '{}'
`)
	a := New("h.example", script)
	_ = a.Retarget(context.Background(), model.RepoRef{Project: "grupo/proy"}, 3, "main")
	if _, err := os.Stat(argsFile); err != nil {
		t.Error("con rama no se salio a la red: el guard se trago la llamada buena")
	}
}

// TestUnaReferenciaVaciaNoSaleALaRed: sin proyecto no hay MR que preguntar.
//
// Es un guard antes de la llamada, y por eso tiene que ser un "no se llama" y no un
// "se llama y falla". La diferencia se ve en la red: una llamada de más por cada ítem
// sin referencia es una llamada que el usuario paga en su límite de peticiones y no
// compra nada.
//
// El aviso que sale es de tipo "notfound", no de red: no es que no se haya encontrado
// el MR, es que no hay ni a cuál preguntar.
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
	// Y no se salió a la red. Ese es el punto del guard.
	if _, err := os.Stat(argsFile); err == nil {
		t.Errorf("una referencia vacida salió a la red:\n%s", argsRegistrados(t, argsFile))
	}

	// Y con referencia sí sale, que es el otro lado del borde.
	_, _ = a.ItemState(context.Background(), model.RepoRef{Project: "grupo/proy"}, 1)
	if _, err := os.Stat(argsFile); err != nil {
		t.Error("con referencia no salió a la red: el guard se tragó la llamada buena")
	}
}

// TestElStampDeGitLabPoneElReviewKindSoloEnReview: lo mismo que en el resto de
// adaptadores, y por el mismo motivo: las secciones de autor y de menciones se pintan
// con los mismos ítems, así que un ReviewKind fuera de la sección de review se acaba
// leyendo en una lista donde no significa nada.
//
// Y lo que se comprueba es la SECCIÓN de la consulta, no la del ítem: es la consulta
// la que sabe de qué lista se está pidiendo.
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
		// Y la sección se estampa siempre: sin ella el ítem no sabe a qué lista
		// pertenece, y el cursor cuenta sobre secciones.
		if items[0].Section != seccion {
			t.Errorf("sección %v: quedó estampada como %v", seccion, items[0].Section)
		}
	}
}
