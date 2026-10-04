// prdash — inbox cross-forge de PRs/MRs en TUI.
//
// Consulta GitHub (vía `gh`) y un GitLab self-managed (vía `glab`) y muestra
// las tres secciones que le importan al usuario: creados por mí, review
// pedido/asignados y menciones. Es de solo lectura.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	tea "charm.land/bubbletea/v2"

	"prdash/internal/config"
	"prdash/internal/forge"
	"prdash/internal/forge/bitbucket"
	"prdash/internal/forge/github"
	"prdash/internal/forge/gitlab"
	"prdash/internal/herdr"
	"prdash/internal/review/executor"
	"prdash/internal/tui"
	"prdash/internal/worktree"
)

// main es la concha: decide el modo, avisa del config y delega. Todo lo que hay debajo es
// una función con su propio tipo de retorno, y eso es lo que la hace testeable — antes,
// `main` era el único sitio donde se decidía el modo, se cargaba la config y se armaba la
// TUI, y `main` no se puede llamar desde un test.
//
// Lo que queda aquí son tres cosas que no se pueden abstraer sin inventar una abstracción
// peor que el problema: leer `os.Args`, cargar la config de disco, y salir del proceso.
func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// mode es el modo de ejecución que decided la línea de órdenes.
type mode int

const (
	modeTUI mode = iota
	modePrint
	modeWorktrees
)

// opts es lo que se puede pedir por línea de órdenes.
//
// Y el modo es la ÚNICA forma de decir qué hacer. La primera versión tenía además un
// `print bool`, y tener las dos es peor que no tener ninguna: `run` acababa mirando las
// dos y cualquier divergencia entre ellas salía como un modo que no ocurre. Un solo
// campo, y los tests leen un solo campo.
type opts struct {
	// mode es qué hacer.
	mode mode
	// sub es el subcomando de `worktrees`: "list" o "remove".
	sub string
	// args son los argumentos que quedan tras el subcomando.
	args []string
}

// parseOpts interpreta la línea de órdenes sin tocar `os.Args` ni el `flag` global, de
// modo que se pueda probar con cualquier argv.
//
// Y hay una asimetría deliberada en el orden: el subcomando `worktrees` se mira ANTES de
// los flags. La razón es que las rutas que gestiona `worktrees remove` son absolutas, y
// una ruta absoluta no empieza por `-`, así que no hay colisión posible —pero si los flags
// se miraran primero, `prdash worktrees remove --orphans` leería `--orphans` como flag de
// prdash en vez de como flag del subcomando.
func parseOpts(args []string) (opts, error) {
	o := opts{mode: modeTUI, sub: "list"}

	if len(args) > 0 && args[0] == "worktrees" {
		o.mode = modeWorktrees
		o.args = args[1:]
		if len(o.args) > 0 && o.args[0] != "" {
			o.sub = o.args[0]
		}
		return o, nil
	}

	// El parseo de flags con un `FlagSet` propio, no el global. El global es un
	// singletón que `flag.Parse()` modifica para siempre, así que un test que lo usara
	// contaminaría el resto de la suite —y el orden de los tests dentro de un binario no
	// está garantizado—.
	fs := flag.NewFlagSet("prdash", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // los errores los formatea `run`, con el stderr del llamador
	print := fs.Bool("print", false, "print the inbox and exit")
	if err := fs.Parse(args); err != nil {
		return opts{}, err
	}
	if *print {
		o.mode = modePrint
	}
	// Y lo que sobra tras los flags se guarda, para que un `--print` con argumentos
	// raros no los ignore en silencio.
	o.args = fs.Args()
	return o, nil
}

// run es el cuerpo del programa con sus tres entradas inyectadas: la línea de órdenes, la
// salida y el error. Devuelve el código de salida en vez de llamar a `os.Exit`, que es lo
// que permite probarlo entero.
func run(args []string, stdout, stderr io.Writer) int {
	o, err := parseOpts(args)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "prdash:", err)
		return 2
	}

	cfg, warn := config.Load()
	if warn != "" {
		// Avisar sin abortar: un config malo degrada a defaults, y la TUI tiene que
		// arrancar igual. Es la regla del proyecto.
		_, _ = fmt.Fprintln(stderr, "prdash:", warn)
	}

	// El subcomando no necesita la TUI ni los adapters: solo el provisioner.
	if o.mode == modeWorktrees {
		return runWorktrees(worktree.Select(herdr.New(), cfg.WorktreeDir), stdout, stderr, o.args)
	}

	adapters := buildAdapters(cfg)
	if len(adapters) == 0 {
		// No es fatal: se sigue y el inbox aparece vacío, que es lo que hace útil
		// abrir la TUI para ver POR QUÉ no hay nada.
		_, _ = fmt.Fprintln(stderr, "prdash: no forges enabled in the config")
	}

	// El executor se construye una vez y se comparte: es el mismo en los tres modos de
	// uso, y el mismo en el registro de reviews y en el simulador.
	ex := buildExecutor(cfg)

	if o.mode == modePrint {
		// El executor resuelve el review activo de cada ítem desde la memoria
		// de rutas; no toca Herdr ni la red.
		runPrintTo(stdout, stderr, adapters, ex.ActiveReview)
		return 0
	}

	model := wire(cfg, adapters, ex)
	if err := arrancaTUI(model); err != nil {
		_, _ = fmt.Fprintln(stderr, "prdash:", err)
		return 1
	}
	return 0
}

// arrancaTUI es lo único que `run` hace con bubbletea, en una función propia.
//
// Y el motivo es el mismo que el seam del navegador: `tea.NewProgram(m).Run()` necesita una
// terminal, así que la línea que devuelve `1` —la de una TUI que no arranca— se podía probar
// con un subproceso sin terminal, pero la del `return 0` de una que sí arranca, no. Sin terminal
// la mitad fácil del camino y la difícil caían en el mismo sitio.
//
// Y la función no abstrae nada: lee bubbletea, ejecuta bubbletea y devuelve lo que bubbletea
// devuelve. Lo que cambia es que `run` ya no depende del terminal para poder ser probada, que es
// lo mismo que hizo `Model.openURL` con el navegador.
var arrancaTUI = func(m tea.Model) error {
	_, err := tea.NewProgram(m).Run()
	return err
}

// wire arma el modelo de la TUI con todo lo que necesita inyectado.
//
// Y el motivo de que sea una función y no un bloque dentro de `main` es que el
// cableado es donde se olvidan cosas: son siete `SetX` seguidos, y uno que falte no
// rompe la compilación —el modelo arranca igual y solo falla cuando el usuario pulsa la
// tecla—, que es la clase de fallo más cara de esta TUI. Con la función, un test
// comprueba que las siete están.
//
// Y que el executor se pase por parámetro en vez de construirse dentro es lo que hace
// comprobable que el simulador, el mounter, el registro de reviews y el borrador usan
// LA MISMA instancia. Cuatro instancias distintas funcionan en los tests y fallan en
// producción, cuando un merge monta un review que el simulador no reconoce.
func wire(cfg config.Config, adapters []forge.Adapter, ex *executor.Executor) tui.Model {
	model := tui.New(cfg, adapters)
	model.SetMounter(ex)
	model.SetSimulator(buildSimulator(cfg, ex))
	model.SetGraphics(herdr.NewGraphics())
	// El registro de reviews montados es lo que permite avisar de que un cambio
	// de base deja desfasado un worktree ya montado. Se inyecta el mismo executor:
	// el sitio que sabe qué review está vivo es el que los montó.
	model.SetReviewLookup(ex)
	// El auto-borrado del worktree tras un merge lo expone el mismo executor, que
	// ya tiene el provisioner. Sin él, el merge funciona igual y no borra nada.
	model.SetReviewRemover(ex)
	return model
}

// buildAdapters construye los adapters de los forges habilitados. Bitbucket se
// registra aunque no esté operativo, para que el inbox lo reporte.
func buildAdapters(cfg config.Config) []forge.Adapter {
	var adapters []forge.Adapter
	if cfg.Forges.GitHub.Enabled {
		adapters = append(adapters, github.New(cfg.Forges.GitHub.Host, cfg.Tools.GH))
	}
	if cfg.Forges.GitLab.Enabled {
		adapters = append(adapters, gitlab.New(cfg.Forges.GitLab.Host, cfg.Tools.Glab))
	}
	if cfg.Forges.Bitbucket.Enabled {
		adapters = append(adapters, bitbucket.New("bitbucket.org"))
	}
	return adapters
}
