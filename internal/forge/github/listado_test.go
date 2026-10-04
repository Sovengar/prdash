package github

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

// El listado de GitHub tenía al 64,3%, y lo que faltaba no era el camino bueno —que ya está
// probado— sino el RESTO: el respaldo REST y las tres salidas del parseo de checks. Y las
// dos cosas comparten un rasgo que las hace delicadas: **el código de salida no es el
// veredicto**.

// `gh pr checks` sale con 8 (pendiente) o 1 (fallo) trayendo el JSON igualmente. Un runner
// que mirara el código de salida descartaría el JSON y clasificaría como error lo que
// realmente es un estado de checks. Por eso `checks` parsea PRIMERO y pregunta al runner
// después —y por eso el orden de esas dos líneas es lo que hay que fijar aquí—.

// ghQueRegistra es un `gh` falso que anota los args en un fichero y contesta lo que se le
// pase. El registro es lo que convierte "se llamó con --repo" en un hecho.
func ghQueRegistra(t *testing.T, cuerpo string) (script, argsFile string) {
	t.Helper()
	dir := t.TempDir()
	argsFile = filepath.Join(dir, "args.log")
	ruta := filepath.Join(dir, "gh")
	script = "#!/bin/sh\necho \"$@\" >> \"" + argsFile + "\"\n" + cuerpo
	if err := os.WriteFile(ruta, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return ruta, argsFile
}

func argsRegistrados(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("no se registró ninguna llamada: %v", err)
	}
	return string(raw)
}

// checksJSON son checks de `gh pr checks --json name,state,bucket`.
const checksJSON = `[{"name":"build","state":"FAILURE","bucket":"fail"},
{"name":"lint","state":"SUCCESS","bucket":"pass"},
{"name":"e2e","state":"PENDING","bucket":"pending"}]`

// searchJSON es la respuesta del respaldo REST de búsqueda.
const searchJSON = `{"total_count":1,"incomplete_results":false,"items":[
{"number":42,"title":"PR","html_url":"https://github.com/acme/proy/pull/42",
 "state":"open","created_at":"2026-09-20T07:00:00Z","updated_at":"2026-09-21T07:00:00Z",
 "user":{"login":"me"},
 "repository_url":"https://api.github.com/repos/acme/proy",
 "pull_request":{"url":"https://api.github.com/repos/acme/proy/pulls/42",
   "head":{"ref":"feat/x","sha":"deadbeef"},"base":{"ref":"main"}},
 "labels":[{"name":"uno"}]}
]}`

// TestLosChecksSeLeenAunQueElCodigoDeSalitaSeaDeError: el orden de las dos líneas.
//
// Y este es el hallazgo del caso, y es la razón de que `checks` parsee antes de preguntar
// al runner por el error. `gh pr checks` sale con 8 si algo está pendiente y con 1 si algo
// falló, en ambos casos con el JSON en stdout. Un runner que mirara el código de salida
// tiraría ese JSON y devolvería un error —que el clasificador de `Kind` convertiría en
// `validation`, es decir un fallo de la llamada—, cuando lo que hay en el stdout es
// exactamente el estado de los checks que la TUI quiere pintar.
func TestLosChecksSeLeenAunQueElCodigoDeSalitaSeaDeError(t *testing.T) {
	// Con los tres estados y un código de salida de error, que es lo que hace `gh` cuando
	// hay un fallo o un pendiente.
	script, argsFile := ghQueRegistra(t,
		"cat <<'JSON'\n"+checksJSON+"\nJSON\nexit 8\n")
	a := New("github.com", script)

	checks, warns := a.checks(context.Background(), "acme/proy", 42)

	if len(warns) != 0 {
		t.Fatalf("checks dio avisos %+v con el JSON parseable: el codigo de salida de "+
			"`gh pr checks` no es el veredicto", warns)
	}
	if checks.State != model.ChecksFailing {
		t.Errorf("estado = %q, want failing: hay un check en fail", checks.State)
	}
	if checks.Failing != 1 || checks.Pending != 1 || checks.Total != 3 {
		t.Errorf("recuentos = %+v, want 1/1/3", checks)
	}
	// Y el comando lleva el repo y el número, que es como se evita depender del cwd.
	args := argsRegistrados(t, argsFile)
	for _, quiere := range []string{"pr checks 42", "--repo acme/proy", "--json"} {
		if !strings.Contains(args, quiere) {
			t.Errorf("los args no traen %q: %s", quiere, args)
		}
	}
}

// TestLosChecksSinJSONSonUnAvisoConSuClase: las dos salidas que no son estado.
//
// Y son dos, no una, y la diferencia es qué clase lleva el aviso:
//
//   - El comando falla sin dejar JSON: el aviso lleva la clase de `Kind`, que es lo que
//     dice si es un problema de permisos, de red o de rate limit.
//   - El comando sale bien pero su stdout no se entiende: `Kind` de un `nil` sería "", que
//     no es una clase. Por eso hay un "parse" explícito — porque "gh dijo algo que no sé
//     leer" y "gh no dijo nada" son dos diagnósticos distintos, y sin distinguirlos el
//     primero se retryería para siempre sin motivo.
func TestLosChecksSinJSONSonUnAvisoConSuClase(t *testing.T) {
	// El comando falla sin JSON.
	script, _ := ghQueRegistra(t, "echo 'HTTP 403: Forbidden' >&2\nexit 1\n")
	checks, warns := New("github.com", script).checks(context.Background(), "acme/proy", 42)
	if len(warns) != 1 {
		t.Fatalf("sin JSON y con fallo dio %d avisos, want 1", len(warns))
	}
	if warns[0].Kind != "permission" {
		t.Errorf("un 403 dio la clase %q, want permission", warns[0].Kind)
	}
	if checks.State != model.ChecksUnknown {
		t.Errorf("un fallo dio estado %q, want desconocido: no se puede pintar un CI "+
			"que no se ha preguntado", checks.State)
	}

	// Y sale bien pero con stdout que no es JSON: "parse", que es explícito.
	script, _ = ghQueRegistra(t, "echo 'esto no es json'\nexit 0\n")
	_, warns = New("github.com", script).checks(context.Background(), "acme/proy", 42)
	if len(warns) != 1 || warns[0].Kind != "parse" {
		t.Fatalf("aviso = %+v, want kind parse", warns)
	}
	if !strings.Contains(warns[0].Msg, "unreadable") {
		t.Errorf("el mensaje %q no dice que la salida no se entendió", warns[0].Msg)
	}
}

// TestElRespaldoRESTTraeLosPropiosYNoTraeLosChecks: la parte que falta del respaldo.
//
// Y lo que se fija es que el respaldo NO pide checks, y por qué eso no es un olvido. El
// respaldo existe para no dejar el inbox en blanco cuando GraphQL falla; los checks son un
// detalle del ítem que se pide aparte y con calma, y meterlos aquí multiplicaría las
// llamadas por el número de PRs de la página. Un PR con el CI desconocido es un dato
// parcial aceptable; veinte PRs con veinte llamadas extra a la API no lo son.
func TestElRespaldoRESTTraeLosPropiosYNoTraeLosChecks(t *testing.T) {
	script, argsFile := ghQueRegistra(t, "cat <<'JSON'\n"+searchJSON+"\nJSON\n")
	a := New("github.com", script)

	page, ok := a.restAuthored(context.Background())
	if !ok {
		t.Fatal("el respaldo REST devolvió ok=false con una respuesta válida")
	}
	if len(page.Items) != 1 {
		t.Fatalf("trajo %d items, want 1", len(page.Items))
	}
	it := page.Items[0]
	if it.Number != 42 || it.Ref.Project == "" {
		t.Errorf("el item no se leyó bien: %+v", it)
	}
	// El stamp: la sección y la identidad del forge. Sin esto el respaldo devuelve
	// ítems que caen en la sección cero y no aparecen.
	if it.Section != model.SectionAuthored {
		t.Errorf("sección = %q, want la de propios", it.Section)
	}
	if it.Forge != ForgeName || it.Host != "github.com" {
		t.Errorf("identidad = %s@%s", it.Forge, it.Host)
	}
	// Y los checks vienen vacíos, que es lo correcto y no un fallo.
	if it.Checks.State != model.ChecksUnknown {
		t.Errorf("el respaldo trajo checks %q, want desconocido", it.Checks.State)
	}

	// Y la consulta es la de búsqueda de los propios, en una sola página.
	args := argsRegistrados(t, argsFile)
	for _, quiere := range []string{"search/issues", "is:pr is:open author:@me", "per_page"} {
		if !strings.Contains(args, quiere) {
			t.Errorf("los args no traen %q: %s", quiere, args)
		}
	}
}

// TestElRespaldoRESTNoEntraSiFallaONoSeEntiende: las dos negativas del respaldo.
//
// Y las dos devuelven `false` en vez de un error, que es lo que hace que el llamador pueda
// decidir entre "te doy un resultado a medias con aviso" y "no tengo nada, con el error de
// verdad". Un respaldo que devolviera un error se confundiría con un fallo de GraphQL, y el
// usuario vería el aviso equivocado —"GitHub no responde" cuando lo que pasó es que el
// respaldo tampoco funcionó—.
func TestElRespaldoRESTNoEntraSiFallaONoSeEntiende(t *testing.T) {
	// Falla.
	script, _ := ghQueRegistra(t, "echo 'boom' >&2\nexit 1\n")
	if _, ok := New("github.com", script).restAuthored(context.Background()); ok {
		t.Error("un respaldo que falla dio ok=true")
	}

	// Sale bien pero no se entiende.
	script, _ = ghQueRegistra(t, "echo 'esto no es json'\nexit 0\n")
	if _, ok := New("github.com", script).restAuthored(context.Background()); ok {
		t.Error("un respaldo ilegible dio ok=true: no es un respaldo, es un fallo")
	}

	// Y una respuesta válida pero vacía: SÍ entra, con cero items. Una lista vacía no es un
	// fallo del respaldo —el forge respondió— y distinguirlo de "falló" convierte un
	// "no tienes PRs" en un "GitHub no responde".
	script, _ = ghQueRegistra(t,
		`echo '{"total_count":0,"incomplete_results":false,"items":[]}'`)
	page, ok := New("github.com", script).restAuthored(context.Background())
	if !ok {
		t.Error("una respuesta vacía dio ok=false: no tienes PRs y no es un fallo")
	}
	if len(page.Items) != 0 {
		t.Errorf("una respuesta vacia trajo %d items", len(page.Items))
	}
}

// TestRelecturarUnItemSinDueñoAvisaSinLlamarAlForge: la guarda de `ItemState`.
//
// Y lo que importa es que avisa SIN llamar al forge. Una referencia sin dueño no se puede
// consultar —la query necesita los dos nombres—, y una consulta con nombre vacío le pide
// al forge el PR #0 de un repositorio vacío, que devuelve algo o un 404 que no dice nada.
//
// Y el aviso es `notfound`, no un error genérico: quien recibe un `notfound` en un ítem que
// estaba en el inbox sabe que ese ítem ya no existe, que es lo que pasó.
func TestRelecturarUnItemSinDueñoAvisaSinLlamarAlForge(t *testing.T) {
	// Un `gh` que registra: si se llamara, el fichero existiría.
	script, argsFile := ghQueRegistra(t, "echo '{\"data\":{}}'\n")
	a := New("github.com", script)

	// "proy" a secas no tiene dueño, porque no hay barra.
	_, warns := a.ItemState(context.Background(),
		model.RepoRef{Project: "proy"}, 1)
	if len(warns) != 1 || warns[0].Kind != "notfound" {
		t.Fatalf("aviso = %+v, want un notfound", warns)
	}
	if !strings.Contains(warns[0].Msg, "invalid repo reference") {
		t.Errorf("el mensaje %q no dice que la referencia es inválida", warns[0].Msg)
	}
	if _, err := os.Stat(argsFile); err == nil {
		t.Error("se llamó al forge con una referencia sin dueño: la query necesita los " +
			"dos nombres y habría pedido el PR #0 de un repositorio vacío")
	}

	// Y con barra sí llega al forge, que es el otro lado de la guarda.
	script, argsFile = ghQueRegistra(t, "echo '{\"data\":{\"search\":{\"nodes\":[]}}}'\n")
	a = New("github.com", script)
	_, warns = a.ItemState(context.Background(), model.RepoRef{Project: "acme/proy"}, 1)
	if _, err := os.Stat(argsFile); err != nil {
		t.Error("con dueño no se llamó al forge")
	}
	// Y un PR que no existe en un repo que sí: también `notfound`, con un mensaje que lo
	// dice — es el caso de un PR borrado mientras estaba en el inbox.
	if len(warns) != 1 || warns[0].Kind != "notfound" {
		t.Errorf("un PR inexistente dio %+v, want notfound", warns)
	}
	if !strings.Contains(warns[0].Msg, "no encontrado") {
		t.Errorf("el mensaje %q no dice que el PR no se encontró", warns[0].Msg)
	}

	// Y una referencia completamente vacía, que es lo mismo que sin dueño.
	_, warns = New("github.com", "/no-debe-ejecutarse").ItemState(context.Background(),
		model.RepoRef{}, 1)
	if len(warns) != 1 || warns[0].Kind != "notfound" {
		t.Errorf("una referencia vacía dio %+v, want notfound", warns)
	}
}

// TestElRollupDelGraphQLSobreviveSiLosChecksNoSePuedenLeer: el orden, y lo que NO se
// pierde.
//
// Y la primera versión de este test afirmaba que los checks quedaban en "desconocido"
// cuando `gh pr checks` fallaba. Es al revés, y al revés es lo correcto:
//
// El item de GraphQL YA trae el `statusCheckRollup` en la propia consulta. `gh pr checks`
// es una segunda fuente que se pide aparte y solo se usa cuando funciona. Así que si esa
// segunda fuente falla, el CI del item NO se pierde — el del rollup sigue ahí—, y si
// funciona, SOBREESCRIBE al rollup.
//
// Y por qué se pide una segunda fuente cuando la primera ya trae los checks: porque no
// siempre viene. El rollup viene en la query del listado, pero hay caminos —el respaldo
// REST, un forge sin soporte de Actions— donde no viene nada. `gh pr checks` es el que
// cubre esos casos, y por eso es el que manda cuando está.
//
// El fallo que esto evita: perder el estado del CI de todos los items por un comando
// secundario que falló. El item se devolvería con el CI en blanco, que se lee como "nadie lo
// ha comprobado" en vez de como "no lo sé".
func TestElRollupDelGraphQLSobreviveSiLosChecksNoSePudenLeer(t *testing.T) {
	script, _ := ghQueRegistra(t, cuerpoChecksQueFallan())
	it, warns := New("github.com", script).ItemState(context.Background(),
		model.RepoRef{Project: "acme/proy"}, 42)

	if len(warns) != 0 {
		t.Fatalf("ItemState dio avisos %+v: los checks que fallan no deben tumbar la "+
			"lectura del item", warns)
	}
	if it.Number != 42 || it.Title != "PR" {
		t.Fatalf("el item no se leyo: %+v", it)
	}
	// Y el CI NO se perdió: sigue el del rollup, que en este fixture dice SUCCESS.
	if it.Checks.State != model.ChecksPassing {
		t.Errorf("los checks = %q, want el del rollup: una segunda fuente que falla no "+
			"puede borrar lo que ya se sabia", it.Checks.State)
	}
}

// TestLosChecksDeLaSegundaFuenteSobrescribenAlRollup: la otra dirección.
//
// Y aquí está la razón de que `gh pr checks` exista: el rollup y el comando pueden
// discrepar, y cuando discrepan manda el comando. El caso real es un check que ha
// terminado entre la query del listado y el clic —el rollup se quedó en PENDING y el
// comando ya ve SUCCESS—. Al revés, el CI se quedaría diciendo "pendiente" para siempre
// hasta el siguiente refresco del inbox.
func TestLosChecksDeLaSegundaFuenteSobrescribenAlRollup(t *testing.T) {
	// El rollup dice SUCCESS; `gh pr checks` dice que hay uno fallando.
	cuerpo := `case "$*" in
  *"pr checks"*) cat <<'JSON'
` + checksJSON + `
JSON
    exit 8;;
  *) cat <<'JSON'
` + graphqlJSONDeUnItem() + `
JSON
    exit 0;;
esac
exit 1
`
	script, _ := ghQueRegistra(t, cuerpo)
	it, warns := New("github.com", script).ItemState(context.Background(),
		model.RepoRef{Project: "acme/proy"}, 42)

	if len(warns) != 0 {
		t.Fatalf("avisos = %+v", warns)
	}
	if it.Checks.State != model.ChecksFailing {
		t.Errorf("los checks = %q, want failing: la segunda fuente manda cuando discrepa "+
			"del rollup", it.Checks.State)
	}
	if it.Checks.Failing != 1 {
		t.Errorf("recuentos = %+v, want un fallando", it.Checks)
	}
}

// cuerpoChecksQueFallan responde bien a la query del PR y mal a los checks.
func cuerpoChecksQueFallan() string {
	return `case "$*" in
  *"pr checks"*) echo 'nope' >&2; exit 1;;
  *) cat <<'JSON'
` + graphqlJSONDeUnItem() + `
JSON
    exit 0;;
esac
exit 1
`
}

// graphqlJSONDeUnItem es la respuesta de la query de un PR, con el rollup en SUCCESS.
func graphqlJSONDeUnItem() string {
	return `{"data":{"search":{"nodes":[
{"number":42,"title":"PR","state":"OPEN","url":"https://github.com/acme/proy/pull/42",
 "headRefName":"feat/x","baseRefName":"main","isDraft":false,
 "updatedAt":"2026-09-21T07:00:00Z","author":{"login":"me"},
 "headRefOid":"deadbeef","mergeable":"MERGEABLE","reviewDecision":"REVIEW_REQUIRED",
 "additions":5,"deletions":1,"changedFiles":2,
 "repository":{"nameWithOwner":"acme/proy","name":"proy","owner":{"login":"acme"},
   "viewerPermission":"WRITE","mergeCommitAllowed":true,"rebaseMergeAllowed":true,
   "squashMergeAllowed":true},
 "commits":{"nodes":[{"commit":{"statusCheckRollup":{"state":"SUCCESS",
   "contexts":{"nodes":[]}}}}]}}
],"pageInfo":{"hasNextPage":false,"endCursor":null}}}}`
}
