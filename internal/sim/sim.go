// Package sim renders a simulation of an integration with git-sim and turns the
// result into something a terminal can show.
//
// git-sim is a renderer, not a simulator: it draws what a git command would do
// as an image and the only trace of that work is the picture, so nothing here
// reads the output to decide anything. What it does give us is a real conflict
// check (it performs the merge in a throwaway clone under the temp dir) and a
// drawing of the history, which is why the image is the whole deliverable.
package sim

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Kind es el comando git-sim que se ejecuta.
type Kind string

const (
	// KindMerge integra la rama del ítem en su rama base, con la base activa.
	KindMerge Kind = "merge"
	// KindRebase rebasa la rama del ítem sobre su base, con la rama del ítem
	// activa.
	KindRebase Kind = "rebase"
)

// String devuelve el nombre del comando, que es lo que se muestra y se pasa a
// git-sim.
func (k Kind) String() string { return string(k) }

// Spec describe la simulación a ejecutar: el comando y el ref contra el que se
// corre. El ref no es la misma rama en los dos casos, y por eso lo lleva quien
// la pide y no lo deduce el runner: merge integra el ref en la rama activa,
// rebase lo rebasa sobre la rama activa.
type Spec struct {
	Kind Kind
	Ref  string
}

// Limits por defecto del render.
const (
	// DefaultBin es el binario de git-sim.
	DefaultBin = "git-sim"
	// DefaultTimeout acota un render. Uno sano tarda un par de segundos; el
	// límite existe para el modo de fallo en el que git-sim se queda pegado al
	// entregar la imagen al visor del escritorio, que sin display no vuelve.
	DefaultTimeout = 60 * time.Second
)

// Runner ejecuta git-sim.
type Runner struct {
	// Bin es el binario; vacío usa DefaultBin.
	Bin string
	// Timeout por render; <=0 usa DefaultTimeout.
	Timeout time.Duration
	// lookPath permite probar la disponibilidad sin tocar el PATH real.
	lookPath func(string) (string, error)
}

// NewRunner construye un runner con los valores por defecto.
func NewRunner() *Runner { return &Runner{Bin: DefaultBin, Timeout: DefaultTimeout} }

// Available informa si el binario se puede ejecutar. Es lo que permite
// configurar la acción y avisar en vez de fallar en caliente.
func (r *Runner) Available() bool {
	look := r.lookPath
	if look == nil {
		look = exec.LookPath
	}
	_, err := look(r.bin())
	return err == nil
}

// bin es el binario efectivo.
func (r *Runner) bin() string {
	if r.Bin == "" {
		return DefaultBin
	}
	return r.Bin
}

// Error es el fallo de una invocación de git-sim, con el código de salida para
// poder clasificarlo sin depender del texto.
type Error struct {
	Args     []string
	Dir      string
	ExitCode int
	Msg      string
	Err      error
}

// Error compone el mensaje incluyendo el directorio y el código de salida.
func (e *Error) Error() string {
	base := fmt.Sprintf("git-sim %s: %s", strings.Join(e.Args, " "), e.Msg)
	if e.Dir != "" {
		base = fmt.Sprintf("git-sim -C %s %s: %s", e.Dir, strings.Join(e.Args, " "), e.Msg)
	}
	if e.ExitCode != 0 {
		return fmt.Sprintf("%s (exit %d)", base, e.ExitCode)
	}
	return base
}

// Unwrap expone la causa subyacente.
func (e *Error) Unwrap() error { return e.Err }

// args compone el argv de una simulación.
//
// Las opciones globales van antes del subcomando: git-sim las declara en el
// grupo raíz y las del subcomodo no las acepta.
//
// --output-only-path reduce la salida a la ruta de la imagen, que es lo único que
// hace falta aquí. Y --quiet no se puede pedir junto a eso: git-sim imprime esa
// ruta solo cuando no está en silencio, así que los dos juntos dejan la salida
// vacía. Es la trampa de este binario, y por eso el test del argv los fija.
func (r *Runner) args(spec Spec, mediaDir string) []string {
	return []string{
		"--output-only-path",
		"--no-animate",
		"--media-dir", mediaDir,
		string(spec.Kind), spec.Ref,
	}
}

// Render ejecuta la simulación en workdir y devuelve la ruta de la imagen
// generada, que existe solo mientras workdir/media siga en pie.
//
// El comando se acota por timeout: su modo de fallo característico no es
// terminar mal, sino no terminar, así que el límite es lo único que lo hace
// recuperable.
func (r *Runner) Render(ctx context.Context, workdir, mediaDir string, spec Spec) (string, error) {
	if spec.Kind != KindMerge && spec.Kind != KindRebase {
		return "", fmt.Errorf("sim: unknown kind %q", spec.Kind)
	}
	if spec.Ref == "" {
		return "", errors.New("sim: no ref to simulate against")
	}

	timeout := r.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := r.args(spec, mediaDir)
	cmd := exec.CommandContext(cctx, r.bin(), args...)
	cmd.Dir = workdir
	cmd.Env = env()

	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		// El código de salida es lo que separa "git-sim se quejó" de "se colgó":
		// sin él no hay forma de distinguir un rechazo de un render que no
		// terminó.
		simErr := &Error{Args: args, Dir: workdir, Msg: message(errb.String(), err), Err: err}
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			simErr.ExitCode = exit.ExitCode()
		}
		return "", simErr
	}
	image := imagePath(out.String())
	if image == "" {
		return "", &Error{Args: args, Dir: workdir, Msg: "git-sim produced no image"}
	}
	return image, nil
}

// env compone el entorno del subproceso.
//
// git_sim_auto_open=false no es opcional: git-sim termina entregando la imagen al
// visor del escritorio, y sin display esa llamada no vuelve nunca. El flag no
// tiene forma negativa en la línea de comandos, pero Settings de git-sim lee las
// variables git_sim_* del entorno, así que es ahí donde se apaga. Se filtran las
// preexistentes para que la última copia sea la nuestra y no la del usuario.
func env() []string {
	out := make([]string, 0, len(os.Environ())+1)
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "git_sim_") {
			continue
		}
		out = append(out, kv)
	}
	return append(out, "git_sim_auto_open=false")
}

// message elige el primer error legible: el que dice git-sim si lo hubo, y el
// del runtime si no.
func message(stderr string, err error) string {
	for _, line := range strings.Split(stderr, "\n") {
		if s := strings.TrimSpace(line); s != "" {
			return s
		}
	}
	if err != nil {
		return err.Error()
	}
	return "unknown error"
}

// imagePath extrae la ruta de la última línea no vacía de la salida: con
// --output-only-path es la única que se escribe, pero un manim que se queja por
// stdout no debe desplazar el resultado.
func imagePath(out string) string {
	lines := strings.Split(out, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if s := strings.TrimSpace(lines[i]); s != "" {
			return s
		}
	}
	return ""
}
