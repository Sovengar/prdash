// Package state deriva, a partir del estado crudo que devuelve cada forge, un
// estado único con precedencia explícita y un score de atención. Lo comparten
// la TUI y el modo de impresión para que el orden sea el mismo.
package state

import (
	"fmt"
	"strings"

	"prdash/internal/forge/model"
)

// State es el estado derivado de un ítem del inbox.
type State int

const (
	// StateDraft es un PR/MR en borrador.
	StateDraft State = iota
	// StatePending está abierto sin decisión de review.
	StatePending
	// StateApproved está aprobado.
	StateApproved
	// StateReviewRequired espera review.
	StateReviewRequired
	// StateChangesRequested tiene cambios pedidos.
	StateChangesRequested
	// StateMerged está mergeado.
	StateMerged
	// StateClosed está cerrado sin mergear.
	StateClosed
	// StateError tiene checks fallidos o un fallo del forge.
	StateError
)

// String devuelve la etiqueta del estado para la UI y el modo de impresión.
func (s State) String() string {
	switch s {
	case StateDraft:
		return "draft"
	case StatePending:
		return "pending"
	case StateApproved:
		return "approved"
	case StateReviewRequired:
		return "review required"
	case StateChangesRequested:
		return "changes requested"
	case StateMerged:
		return "merged"
	case StateClosed:
		return "closed"
	case StateError:
		return "error"
	default:
		return "unknown"
	}
}

// Score es la prioridad de atención para el orden del inbox:
// error > changes-requested > review-required > approved > pending >
// merged/closed > draft.
func (s State) Score() int {
	switch s {
	case StateError:
		return 8
	case StateChangesRequested:
		return 7
	case StateReviewRequired:
		return 6
	case StateApproved:
		return 5
	case StatePending:
		return 4
	case StateMerged, StateClosed:
		return 2
	case StateDraft:
		return 1
	default:
		return 0
	}
}

// Derive deduce el estado del ítem a partir de lo que reportó el forge.
// Precedencia: cerrado/mergeado > checks fallidos > cambios pedidos > review
// pedido > aprobado > borrador > abierto.
func Derive(it model.Item) State {
	switch normalize(it.State) {
	case "merged":
		return StateMerged
	case "closed":
		return StateClosed
	}

	if it.Checks.State == model.ChecksFailing {
		return StateError
	}

	switch normalize(it.ReviewDecision) {
	case "changes_requested":
		return StateChangesRequested
	case "review_required":
		return StateReviewRequired
	case "approved":
		return StateApproved
	}

	if it.IsDraft {
		return StateDraft
	}
	return StatePending
}

// Actionable indica si un ítem admite una acción de approve/merge y, si no,
// el motivo. Un ítem cerrado o mergeado ya no es accionable.
//
// Solo cubre lo que ninguna acción puede ignorar: el ítem ya no existe como
// cosa que integrar. Lo que depende de laacción (que el CI esté rojo, que haya
// cambios pedidos) lo decide MergeBlock, porque ahí la respuesta no es sí o no
// sino sí, pero solo si lo dices otra vez.
func Actionable(it model.Item) (bool, string) {
	switch Derive(it) {
	case StateMerged:
		return false, "item is already merged"
	case StateClosed:
		return false, "item is already closed"
	default:
		return true, ""
	}
}

// Block es el veredicto de un gate sobre una acción: por qué no debería
// ejecutarse sin más, y si levantarlo está en manos del usuario.
//
// La distinción entre duro y blando es la que hace que el gate sirva de algo.
// Un bloqueo duro es una propiedad del forge: un PR en borrador no se mergea
// porque GitHub no lo permite, y ninguna confirmación lo cambia. Un bloqueo blando
// es una política: el CI puede estar rojo por un check inestable, y puede haber
// cambios pedidos que quien mantiene el repositorio decide ignorar. Esos no se
// prohíben —eso convertiría la herramienta en un muro— pero tampoco pasan
// inadvertidos: se anuncian y se piden dos veces.
type Block struct {
	// Reason explica el bloqueo, ya traducido para el usuario.
	Reason string
	// Hard marca un bloqueo que ninguna confirmación puede levantar.
	Hard bool
}

// MergeBlock decide si un merge debería salir sin más sobre un ítem.
//
// Se apoya en datos que el inbox YA trae y que hasta ahora no se usaban para
// nada: el estado derivado distingue un ítem en borrador de uno con cambios
// pedidos de uno con checks en rojo, y la ficha los enseña. La consecuencia de no
// mirar es que `merge` salía igual en los tres casos, y la de mirar es que el
// merge se frena en el único sitio donde el daño es irreversible.
//
// La precedencia reproduce la de Derive, que ya decide el orden de atención: un
// ítem con CI rojo y cambios pedidos se bloquea por el CI, que es lo que el
// operador quiere saber primero. Los checks pendientes cuentan como bloqueo
// blando y no como IGNorado: mergear mientras el CI corre es exactamente la
// carrera que el pin del head SHA no cierra, porque el CI puede pasar después del
// merge.
func MergeBlock(it model.Item) Block {
	// Borrador, mergeado y cerrado se miran en la propiedad del ítem y no en
	// Derive, y no es un detalle: Derive ordena por atención al operador, así que
	// un ítem en borrador que además está aprobado devuelve StateApproved y el
	// borrador se pierde. Aquí la pregunta es otra —¿puede este forge integrar
	// esto?—, y la respuesta no depende de la decisión de review.
	switch normalize(it.State) {
	case "merged":
		return Block{Reason: "item is already merged", Hard: true}
	case "closed":
		return Block{Reason: "item is already closed", Hard: true}
	}
	if it.IsDraft {
		// GitHub rechaza el merge de un PR en borrador antes de mirar nada más,
		// así que ofrecerlo sería gastar una llamada para recibir un error que no
		// depende de la estrategia.
		return Block{Reason: "item is a draft", Hard: true}
	}
	// El conflicto de ramas es BLANDO y no duro, y esa es la decisión de diseño
	// que importa: GitHub no lo va a integrar mientras siga así, pero un rebase
	// lo arregla en un comando, y el gate no puede saber si el usuario ya lo ha
	// hecho. Prohibirlo sería dejar al PR sin salida desde aquí —la que de verdad
	// no tiene arreglo es la del CI inestable, y esa se avisa sin vetar—, así que
	// se anuncia y la segunda pulsación decide.
	//
	// Va antes que el CI a propósito: un PR que choca no va a pasar el CI, y decir
	// "CI is failing" cuando lo que hay que rehacer es un rebase manda al
	// operador a mirar el sitio equivocado.
	if it.Mergeable.Known && it.Mergeable.Conflicted {
		return Block{Reason: conflictedReason(it.TargetBranch)}
	}
	switch {
	case it.Checks.State == model.ChecksFailing:
		return Block{Reason: checksFailingReason(it.Checks)}
	case it.Checks.State == model.ChecksPending:
		return Block{Reason: fmt.Sprintf("CI is still running (%d pending)", it.Checks.Pending)}
	// La decisión se normaliza porque el mismo dato llega en convenciones
	// distintas según el forge: GitHub manda el enum `CHANGES_REQUESTED` en
	// GraphQL y `CHANGES_REQUESTED` en REST, y compararlo en crudo hacía que el
	// gate no viera los cambios pedidos en ninguno de los dos casos.
	case normalize(it.ReviewDecision) == "changes_requested":
		return Block{Reason: "changes were requested on this item"}
	}
	return Block{}
}

// UnmergeableReason explica un rechazo del forge que no se arregla refrescando.
//
// Es el motivo de la casa para el fallo que llega por la vía rápida: cuando el
// gate ya sabía que las ramas se pisan y aun así se intentó, o cuando el dato no
// venía, el forge responde con su propio inglés y prdash lo tradujo. La
// diferencia con un "conflicto" —el ítem cambió mientras lo mirabas, que se
// resuelve solo con refrescar— es la que importa: aquí la acción correcta es
// rebasar y pushear, y un aviso que diga "refresca" manda al sitio equivocado.
const UnmergeableReason = "the forge will not merge it as it is: rebase the branch onto the target and push"

// conflictedReason nombra la rama con la que choca el ítem. Con el nombre de la
// rama delante el aviso dice qué hay que rebasar; sin él, "choca" obliga a abrir
// el PR para saber contra qué.
func conflictedReason(target string) string {
	if strings.TrimSpace(target) == "" {
		return "the branch conflicts with the target branch"
	}
	return "the branch conflicts with " + target
}

// checksFailingReason nombra los checks que fallan. Con el recuento delante, el
// aviso deja de ser un "no puedes" abstracto y dice cuántos hay: es lo que
// distingue un CI roto de un check inestable.
func checksFailingReason(c model.Checks) string {
	if c.Total > 0 {
		return fmt.Sprintf("CI is failing (%d of %d checks)", c.Failing, c.Total)
	}
	return "CI is failing"
}

// SelfReviewReason explica por qué no se puede aprobar un ítem propio. Es el
// motivo que enseñan la TUI y la clasificación del rechazo del forge, para que
// el usuario lea lo mismo en los dos caminos.
const SelfReviewReason = "you cannot approve your own PR/MR"

// CanApprove indica si un ítem admite la acción de approve y, si no, el
// motivo. Ningún forge admite aprobar lo propio: GitHub lo rechaza en la API
// sin opción de activarlo, y quien lo permite por configuración (GitLab) sigue
// siendo el repositorio, no el cliente, quien decide.
//
// La identidad se decide con el login del viewer cuando ambos se conocen: es
// un hecho. Si falta alguno, se cae a la sección, que para un forge significa
// "el usuario lo escribió" (`author:@me` en GitHub, `currentUser` en GitLab).
func CanApprove(it model.Item, viewer string) (bool, string) {
	if viewer != "" && it.Author != "" {
		if strings.EqualFold(viewer, it.Author) {
			return false, SelfReviewReason
		}
		return true, ""
	}
	if it.Section == model.SectionAuthored {
		return false, SelfReviewReason
	}
	return true, ""
}

// normalize compara estados sin depender de mayúsculas ni separadores.
func normalize(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch r {
		case '-', ' ', '\t':
			out = append(out, '_')
		default:
			if r >= 'A' && r <= 'Z' {
				r += 'a' - 'A'
			}
			out = append(out, r)
		}
	}
	return string(out)
}
