package herdr

import (
	"strconv"
	"time"
)

// args son los argumentos de una invocación de la CLI de Herdr, con los flags
// opcionales.
//
// Existe para que el ARGV sea comprobable. Antes cada comando escribía sus guards
// en línea, y un guard en línea no se puede comprobar sin ejecutar la CLI: o no se
// comprueba, o se comprueba con un doble de `execFunc` que ve la llamada pero no
// dice qué garantiza. Con el argv en una función pura, el test afirma la lista
// entera, que es exactamente el contrato con la CLI: el orden importa (Herdr
// distingue `tab create --label` de `tab rename`) y un flag de más es un error de
// la CLI, no una línea más o menos en un dibujo.
//
// La regla de los flags opcionales vive aquí y en ningún otro sitio: un flag con
// valor vacío NO se manda. No por descuido, sino porque mandar `--cwd ""` le dice a
// Herdr "usa el directorio vacío", que no es lo mismo que no decir nada, y esa
// diferencia sale en el sitio más malo: en el repo equivocado.
type args []string

// with añade `flag value` si el valor no está vacío, y no añade nada si lo está.
func (a args) with(flag, value string) args {
	if value == "" {
		return a
	}
	return append(a, flag, value)
}

// withIf añade `flag` si la condición se cumple, y nada si no.
func (a args) withIf(cond bool, flag string) args {
	if !cond {
		return a
	}
	return append(a, flag)
}

// Los argv de cada comando. Funciones puras a propósito: son el contrato con la
// CLI, y un contrato se prueba, no seINESPECTA.

// worktreeCreateArgs: --cwd, --branch, --path, --label y --no-focus, en ese orden.
func worktreeCreateArgs(spec WorktreeSpec) args {
	return args{"worktree", "create"}.
		with("--cwd", spec.Cwd).
		with("--branch", spec.Branch).
		with("--path", spec.Path).
		with("--label", spec.Label).
		withIf(spec.NoFocus, "--no-focus")
}

// worktreeListArgs: --cwd es opcional porque sin él Herdr lista los del directorio
// actual, que es justo lo que quiere quien no pasa ninguno.
func worktreeListArgs(cwd string) args {
	return args{"worktree", "list"}.with("--cwd", cwd)
}

// worktreeRemoveArgs: --force es opt-in. Nunca se manda por defecto: quitar el
// checkout de un workspace con cambios sin preguntar es peor que no poder hacerlo.
func worktreeRemoveArgs(workspaceID string, force bool) args {
	return args{"worktree", "remove", "--workspace", workspaceID}.withIf(force, "--force")
}

// workspaceCreateArgs: --cwd, --label y --no-focus.
func workspaceCreateArgs(spec WorkspaceSpec) args {
	return args{"workspace", "create"}.
		with("--cwd", spec.Cwd).
		with("--label", spec.Label).
		withIf(spec.NoFocus, "--no-focus")
}

// workspaceCloseArgs: --group es opt-in por una razón de versión: algunas versiones
// lo exigen para cerrar el grupo de worktrees, y mandarlo en las que no lo entienden
// es un error. Así que solo lo manda quien lo pide.
func workspaceCloseArgs(workspaceID string, group bool) args {
	return args{"workspace", "close", workspaceID}.withIf(group, "--group")
}

// tabCreateArgs: --workspace, --cwd, --label y --no-focus.
func tabCreateArgs(spec TabSpec) args {
	return args{"tab", "create"}.
		with("--workspace", spec.WorkspaceID).
		with("--cwd", spec.Cwd).
		with("--label", spec.Label).
		withIf(spec.NoFocus, "--no-focus")
}

// splitArgs: --ratio y --cwd, más un --env por variable.
//
// El ratio se omite si es cero porque cero es el default de Herdr, y mandarlo
// sobrescribe ese default con un valor que nadie pidió. Un ratio de 0 además es
// imposible (es una fracción), así que mandarlo no da error: da un pane de tamaño
// cero. Y el env vacío no se manda, porque `--env ""` le dice a Herdr que defina una
// variable sin nombre.
//
// El orden de los --env es el del slice, y eso importa: son pares clave=valor y
// reordenar un slice de env es reordenar el entorno del proceso que se va a
// lanzar, que se lee como un cambio de comportamiento.
func splitArgs(spec SplitSpec) args {
	a := args{"pane", "split", "--pane", spec.PaneID, "--direction", spec.Direction}
	if spec.Ratio > 0 {
		a = append(a, "--ratio", strconv.FormatFloat(spec.Ratio, 'f', -1, 64))
	}
	a = a.with("--cwd", spec.Cwd)
	for _, kv := range spec.Env {
		if kv == "" {
			continue
		}
		a = append(a, "--env", kv)
	}
	return a.withIf(spec.NoFocus, "--no-focus")
}

// waitOutputArgs: --timeout se omite si no hay plazo, y entonces Herdr espera su
// default. Un timeout de cero quiere decir "sin límite", no "cero segundos": por eso
// se omite en vez de mandarse, porque mandarlo haría fallar la espera al instante.
//
// Y se manda en MILISEGUNDOS enteros, que es la unidad que espera la CLI. Un valor
// con decimales sería un rechazo de la CLI en el sitio más tarde posible.
func waitOutputArgs(paneID, match string, timeout time.Duration) args {
	a := args{"pane", "wait-output", "--match", match, paneID}
	if timeout > 0 {
		a = append(a, "--timeout", strconv.FormatInt(timeout.Milliseconds(), 10))
	}
	return a
}

// paneListArgs: --workspace es opcional; sin él, los panes del workspace actual.
func paneListArgs(workspaceID string) args {
	return args{"pane", "list"}.with("--workspace", workspaceID)
}

// notifyArgs: --body y --sound. El sonido vacío es el default de Herdr, y mandarlo
// fijaría uno.
func notifyArgs(title string, opts NotifyOptions) args {
	return args{"notification", "show", title}.
		with("--body", opts.Body).
		with("--sound", opts.Sound)
}
