package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"prdash/internal/cache"
	"prdash/internal/config"
	"prdash/internal/forge/model"
	"prdash/internal/worktree"
)

// Los bordes de `cmd/prdash` que quedan: la degradación de `run` cuando no hay forges, el
// `continue` del lote de borrados cuando uno falla, y el `simLocator` que decide de dónde saca
// los refs.
//
// Y los tres degradan, que es la palabra que los unifica. Ninguno aborta, y ninguno lo hace
// sin decir por qué.

// provisionerQueFallaQuitar cuenta los intentos y falla solo en las rutas que se le digan, que
// es lo que permite comprobar que un fallo NO tira el resto del lote.
type provisionerQueFallaQuitar struct {
	quitados []string
	fallaCon error
	fallaEn  map[string]bool
}

func (p *provisionerQueFallaQuitar) Create(context.Context, worktree.Spec) (worktree.Worktree, error) {
	return worktree.Worktree{}, nil
}

func (p *provisionerQueFallaQuitar) RemoveIfClean(context.Context, string) (bool, string, error) {
	return false, "", nil
}

func (p *provisionerQueFallaQuitar) List(context.Context) []worktree.Worktree { return nil }

func (p *provisionerQueFallaQuitar) Audit(context.Context) []worktree.Entry { return nil }

func (p *provisionerQueFallaQuitar) Remove(_ context.Context, id string) error {
	p.quitados = append(p.quitados, id)
	if p.fallaEn[id] {
		return p.fallaCon
	}
	return nil
}

// TestSinForgesEnElConfigSeAviadoYLaSesionSigue: la degradación de `run`.
//
// Y lo que se comprueba es el código de salida, que es la afirmación entera: es 0, porque la
// sesión arranca. Un código de error ahí sería un proceso que muere al abrirse, y el usuario
// vería un parpadeo sin explicación en vez de una TUI con el motivo en la cabecera.
//
// Y el aviso va a stderr y no a stdout a propósito: stdout es lo que un script lee para obtener
// datos, y un "no hay forges" mezclado con el listado del modo `--print` haría que un script
// lo contara como una fila.
func TestSinForgesEnElConfigSeAviadoYLaSesionSigue(t *testing.T) {
	// Un config sin ningún forge habilitado, en el sitio que `config.Load` lee.
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := os.MkdirAll(filepath.Join(xdg, "prdash"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(xdg, "prdash", "config.toml"), []byte(`
[forge.github]
enabled = false

[forge.gitlab]
enabled = false

[forge.bitbucket]
enabled = false
`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Y en modo `--print`, que es el único camino de `run` que se puede comprobar sin abrir una
	// TUI: el otro termina en `tea.NewProgram(...).Run()`, que necesita una terminal.
	var stdout, stderr bytes.Buffer
	code := run([]string{"--print"}, &stdout, &stderr)

	if code != 0 {
		t.Errorf("sin forges el código de salida es %d, want 0: la sesión tiene que arrancar "+
			"igual para que se vea el motivo", code)
	}
	if !strings.Contains(stderr.String(), "no forges") {
		t.Errorf("stderr = %q, want el aviso de que no hay forges", stderr.String())
	}
	// Y stdout NO lleva el aviso: es lo que un script lee.
	if strings.Contains(stdout.String(), "no forges") {
		t.Errorf("el aviso salió por stdout, que es lo que lee un script: %q", stdout.String())
	}
	// Y stdout sí lleva la tabla aunque esté vacía. El modo `--print` promete una tabla, y sin
	// adapters la imprime con las cabeceras: eso es lo que distingue "no hay nada" de "no se
	// pudo consultar".
	if strings.TrimSpace(stdout.String()) == "" {
		t.Error("stdout está vacío: el modo --print debe imprimir la tabla aunque no haya " +
			"nada que enseñar")
	}
}

// TestUnBorradoQueFallaNoTiraElRestoDelLote: el `continue` después de `removeOne`.
//
// Y es el mismo patrón del layout de Herdr y por el mismo motivo: un lote con un elemento malo
// tiene que procesarse entero.
//
// Y las dos mitades del aserto importan igual, porque una sola no basta:
//
//   - La ruta que falló no se-annuncia como borrada, y su causa va a stderr.
//   - Las demás sí. Un abort en el primer fallo obligaría al usuario a recordar cuáles eran
//     las rutas buenas, y el siguiente comando tendría que adivinarlo.
func TestUnBorradoQueFallaNoTiraElRestoDelLote(t *testing.T) {
	base, owned, _ := worktreeFixture(t)
	otra := filepath.Join(base, "prdash-pr-2")
	if err := escribirComoWorktree(t, owned, otra); err != nil {
		t.Fatal(err)
	}

	prov := &provisionerQueFallaQuitar{
		fallaCon: errors.New("workspace is not empty"),
		fallaEn:  map[string]bool{otra: true},
	}
	var stdout, stderr bytes.Buffer

	code := removeWorktreesWithin(prov, &stdout, &stderr, false, false,
		[]string{owned, otra, filepath.Join(base, "prdash-pr-3")}, 5*time.Second)

	if code != 1 {
		t.Errorf("código = %d con un borrado fallido, want 1", code)
	}
	// Y stdout solo nombra lo que SE BORRÓ. Un fallo ahí haría que un script contara un
	// worktree que sigue en disco.
	if !strings.Contains(stdout.String(), "worktree removed: "+owned) {
		t.Errorf("stdout no dice que se borró %s:\n%s", owned, stdout.String())
	}
	if strings.Contains(stdout.String(), otra) {
		t.Errorf("stdout dice que se borró %s, que sigue en disco:\n%s", otra, stdout.String())
	}
	// Y stderr lleva la causa, que es lo que permite saber si reintentar tiene sentido: un
	// workspace ocupado no se arregla reintentando.
	errOut := stderr.String()
	if !strings.Contains(errOut, "workspace is not empty") {
		t.Errorf("stderr no trae la causa del fallo:\n%s", errOut)
	}
	if !strings.Contains(errOut, otra) {
		t.Errorf("stderr no nombra la ruta que falló:\n%s", errOut)
	}
	// Y el lote siguió: tras el fallo de la segunda se intentó la tercera. Y son DOS
	// `removeOne`, no tres, porque la tercera no existe y la guarda de la entrada la rechaza
	// ANTES de llamar al provisioner —que es lo que impide pedirle que borre algo que no
	// existe—. Mi primera versión pedía tres y falló; el número correcto lo dice el código,
	// no mi suposición.
	if len(prov.quitados) != 2 {
		t.Errorf("se intentó borrar %d rutas, want 2: %v", len(prov.quitados), prov.quitados)
	}
	// Y stdout NO lista la tercera, que ni se intentó, mientras que stderr explica por qué.
	if strings.Contains(stdout.String(), "prdash-pr-3") {
		t.Errorf("stdout menciona una ruta que ni se intentó borrar:\n%s", stdout.String())
	}
	if !strings.Contains(errOut, "prdash-pr-3") {
		t.Errorf("stderr no explica el rechazo de la tercera ruta:\n%s", errOut)
	}
}

// escribirComoWorktree copia el `.git` de un worktree real a otra ruta del mismo repo, para
// tener un segundo worktree sin pagar un `git worktree add`.
//
// Y es la vía barata porque lo que se prueba es la MECÁNICA del lote, no la creación de
// worktrees: lo que importa es que `worktree.Exists` los reconozca para que el lote llegue al
// `removeOne`.
func escribirComoWorktree(t *testing.T, origen, destino string) error {
	t.Helper()
	gitdir, err := os.ReadFile(filepath.Join(origen, ".git"))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(destino, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(destino, ".git"), gitdir, 0o644)
}

// TestElSubcomandoWorktreesNoConstruyeNiAdaptersNiExecutor: la rama de `run` que no estaba.
//
// Y es una rama que vale por lo que NO hace. `runWorktrees` necesita solo el provisioner, así
// que entrar por ella evita la construcción de adapters —que lee la config y puede abrir
// conexiones— y del executor. Y eso no es una optimisation: si el subcomando construyera
// adapters, `prdash worktrees list` fallaría o tardaría en una máquina sin `gh` instalado, que
// es justo la máquina donde más hace falta poder limpiar worktrees.
//
// Y se comprueba con un config de un solo forge deshabilitado y un `PATH` sin `gh`/`glab`: si
// `run` tocara un adapter, el aviso sería de otro tipo o el código de salida sería distinto.
//
// Y es la única forma de cubrir esa línea sin abrir la TUI, porque el otro camino de `run` es
// `tea.NewProgram`, que necesita una terminal.
func TestElSubcomandoWorktreesNoConstruyeNiAdaptersNiExecutor(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := os.MkdirAll(filepath.Join(xdg, "prdash"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Un config que no habilita ningún forge Y un worktree dir vacío: la lista tiene que salir
	// vacía sin que nada más intervene.
	config := "[worktree]\ndir = " + filepath.Join(t.TempDir(), "worktrees") + "\n\n" +
		"[forge.github]\nenabled = false\n\n[forge.gitlab]\nenabled = false\n"
	if err := os.WriteFile(filepath.Join(xdg, "prdash", "config.toml"),
		[]byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	// Y un PATH sin forges, para que si `run` los construyera el fallo fuera visible.
	t.Setenv("PATH", t.TempDir())

	var stdout, stderr bytes.Buffer
	code := run([]string{"worktrees", "list"}, &stdout, &stderr)

	if code != 0 {
		t.Errorf("el código de salida es %d, want 0: no hay worktrees y eso no es un fallo.\n%s",
			code, stderr.String())
	}
	// Y lo dice, en vez de salir con un mapa vacío: el subcomando es un script y su salida
	// se lee.
	if !strings.Contains(stdout.String()+stderr.String(), "worktree") {
		t.Errorf("la salida no menciona worktrees:\nstdout: %s\nstderr: %s",
			stdout.String(), stderr.String())
	}
	// Y NO se quejó de forges: esa es la mitad del valor de la rama. Si `run` construyera los
	// adapters antes de mirar el subcomando, aquí habría un aviso de "no forges enabled".
	for _, sale := range []string{"no forges", "gh:", "glab:"} {
		if strings.Contains(stderr.String(), sale) {
			t.Errorf("el subcomando worktrees menciona %q: ha construido algo que no "+
				"necesita.\nstderr: %s", sale, stderr.String())
		}
	}
}

// TestElLocalizadorDeSimulacionEncuentraElRepoDeUnReviewMontado: el camino bueno de `Locate`.
//
// Y este es el otro camino del par, y el que más importa: sin él la simulación no puede
// clonar nada. `simLocator` lee el review activo del ítem de la memoria y de ahí saca el clon y
// la rama; sin las dos, `sim` avisaría de que no hay review montado aunque lo haya.
//
// Y la forma de darle un review montado es escribirlo en la memoria de verdad —el fichero
// `memo.json` del caché de prdash— y dejar que `buildExecutor` lo encuentre. Un doble del
// `Resolver` mediría que `simLocator` copia dos campos de un struct; esto mide que la CLAVE de
// la memoria es la que se espera y que el ejecutor la construye con el mismo criterio que el
// que la escribió —que es donde se cuelan los errores de encoding—.
//
// Y la asimetría con el caso negativo es lo que importa: la misma llamada sin registro devuelve
// `false`, y sin repo devuelve `false` también. Un locator que devolviera `true` con un `Place`
// vacío haría que `git clone` recibiera una ruta vacía.
func TestElLocalizadorDeSimulacionEncuentraElRepoDeUnReviewMontado(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheDir)

	it := model.NewItem(model.RepoRef{
		Forge: "github", Host: "github.com", Project: "acme/widget",
		Owner: "acme", Name: "widget",
	}, 7)

	// Sin registro: no hay review, y no hay dónde mirar.
	loc := simLocator{ex: buildExecutor(config.Defaults())}
	if _, ok := loc.Locate(it); ok {
		t.Fatal("sin review registrado el locator dijo que sabe localizar")
	}

	// Con el review registrado en la memoria de verdad.
	memo := cache.Memo{
		Version: 1,
		Reviews: map[string]cache.ReviewRecord{
			"github/github.com/acme/widget#7": {
				Repo:     "/clones/acme/widget.git",
				Worktree: "/wt/prdash-pr-7",
				Branch:   "prdash/pr-7",
				Label:    "prdash-pr-7",
			},
		},
	}
	ruta := filepath.Join(cacheDir, cache.DirName, cache.MemoFileName)
	if err := os.MkdirAll(filepath.Dir(ruta), 0o755); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(memo)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ruta, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	loc = simLocator{ex: buildExecutor(config.Defaults())}
	place, ok := loc.Locate(it)
	if !ok {
		t.Fatal("con el review registrado en la memoria el locator dijo que no: la clave " +
			"de la memoria no coincide con la que construye el ejecutor")
	}
	// Y trae LAS DOS cosas que `sim` necesita: el clon de donde clonar y la rama que
	// materializar. Con una sola, la simulación clona bien y falla al activar, o al revés.
	if place.Repo != "/clones/acme/widget.git" {
		t.Errorf("Repo = %q, want el clon registrado", place.Repo)
	}
	if place.Branch != "prdash/pr-7" {
		t.Errorf("Branch = %q, want la rama del review", place.Branch)
	}

	// Y un ítem DISTINTO no se confunde con este: el registro es por ítem, y dos PRs del
	// mismo repo —el caso normal cuando se revisan varios— tienen registros separados.
	otro := model.NewItem(it.Ref, 8)
	if _, ok := loc.Locate(otro); ok {
		t.Error("el locator encontró el review de otro ítem: el registro es por número")
	}
}
