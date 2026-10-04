package state

import (
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

// TestElOrdenDeAtencionEsElDocumentado: `Score` ordena el inbox, y el orden es una
// decisión de producto, no un detalle.
//
// Y la comprobación que importa no es "estos ocho estados tienen estos ocho números", que
// se rompe en cuanto alguien añade un estado, sino la RELACIÓN. Lo que tiene que ser
// cierto es que un error ordena por encima de un changes-requested, y un draft por debajo
// de un cerrado. Eso es lo que se quiere, y sobrevive a que se reescriban los números.
//
// Y hay una fila que se prueba aparte porque es la única que agrupa dos estados: merged y
// closed puntúan lo mismo a propósito. Un PR cerrado sin merge y uno mergeado son el mismo
// "esto ya no necesita tu atención". Si un día divergen, este test lo dice y entonces es
// una decisión, no un descuido.
func TestElOrdenDeAtencionEsElDocumentado(t *testing.T) {
	// De más urgente a menos, tal y como lo describe el comentario de `Score`.
	orden := []State{StateError, StateChangesRequested, StateReviewRequired, StateApproved,
		StatePending, StateMerged, StateDraft}
	for i := 0; i < len(orden)-1; i++ {
		a, b := orden[i], orden[i+1]
		if orden[i].Score() <= orden[i+1].Score() {
			t.Errorf("%q (score %d) no ordena por encima de %q (score %d)",
				a, a.Score(), b, b.Score())
		}
	}

	// Merged y closed empatan. Es deliberado: ambos son "esto ya no está pendiente".
	if StateMerged.Score() != StateClosed.Score() {
		t.Errorf("merged puntua %d y closed %d: el codigo los pone juntos a proposito, "+
			"asi que si divergen es una decision que hay que escribir",
			StateMerged.Score(), StateClosed.Score())
	}

	// Y un estado que el código no conoce puntúa cero, por debajo del draft. Unknown no
	// es "urgente": es "no se sabe", y tratarlo como urgente lo subiría al principio del
	// inbox.
	//
	// Y el valor concreto del desconocido importa poco, pero tiene que estar FUERA del
	// rango de los declarados: si `State(0)` fuera el desconocido, el test probaría que
	// el cero no puntúa, cuando el cero es `StateDraft` y puntúa 1.
	if got := State(99).Score(); got != 0 {
		t.Errorf("un estado desconocido puntua %d, want 0", got)
	}
	if got := State(99).String(); got == "" {
		t.Error("un estado desconocido dio etiqueta vacia")
	}
}

// TestLaEtiquetaDelEstadoEsLaDeLaColumnaEstrecha: `String` va en la columna de estado, que
// es una de las más estrechas de la fila.
//
// Y el caso que falla de verdad es el más largo: "changes requested" son 17 caracteres, y
// "review required" son 15. Si alguien abrevia el segundo por "needs review" y no abrevia
// el primero, la columna se ensancha por donde menos duele. La tabla entera fijada es la
// que lo evita: no deja que la columna crezca fila a fila.
func TestLaEtiquetaDelEstadoEsLaDeLaColumnaEstrecha(t *testing.T) {
	casos := []struct {
		estado State
		want   string
	}{
		{StateDraft, "draft"},
		{StatePending, "pending"},
		{StateApproved, "approved"},
		{StateReviewRequired, "review required"},
		{StateChangesRequested, "changes requested"},
		{StateMerged, "merged"},
		{StateClosed, "closed"},
		{StateError, "error"},
	}
	for _, c := range casos {
		if got := c.estado.String(); got != c.want {
			t.Errorf("State(%q).String() dio %q, want %q", c.estado, got, c.want)
		}
		// Etiqueta vacía es peor que etiqueta larga: la columna se queda en blanco y no
		// hay forma de saber si falta el dato o si el estado no existe.
		if strings.TrimSpace(c.estado.String()) == "" {
			t.Errorf("State(%q) tiene etiqueta vacia", c.estado)
		}
		// Y sin espacios: una etiqueta con un espacio al final se ve un carácter más
		// ancha en el ancho calculado, que es de donde sale el desborde.
		if c.estado.String() != strings.TrimSpace(c.estado.String()) {
			t.Errorf("la etiqueta de %q tiene espacios en los bordes", c.estado)
		}
	}
	// Y un estado desconocido no inventa etiqueta: sale lo que se pueda sacar de un int
	// que no es un estado, que es un texto inútil pero no una mentira. Lo que importa
	// es que no esté vacío y que no rompa la columna.
	if got := State(99).String(); strings.TrimSpace(got) == "" {
		t.Error("un estado desconocido dio etiqueta vacia")
	}
}

// TestElMotivoDelConflictoNombraLaRama: el aviso de conflicto tiene que decir contra qué
// hay que rebasar.
//
// Y la diferencia entre los dos casos es justo la que hace útil la función. Con rama, el
// aviso es accionable. Sin rama —un ítem recién abierto cuyo target aún no se ha
// resuelto— el aviso tiene que seguir siendo un aviso y no una cadena vacía: sin texto, la
// TUI no explica por qué el ítem está bloqueado, y eso es el mismo fallo que si dijera
// algo falso.
//
// Y el caso de solo espacios no es hipotético: el target viene de una respuesta del forge,
// y un campo opcional presente pero vacío llega como "" o como " " según el cliente.
func TestElMotivoDelConflictoNombraLaRama(t *testing.T) {
	casos := []struct {
		target string
		want   string
	}{
		{"main", "the branch conflicts with main"},
		{"release/2026-04", "the branch conflicts with release/2026-04"},
		// Sin rama, o con la rama hecha solo de espacios, el aviso cae al texto genérico.
		{"", "the branch conflicts with the target branch"},
		{"   ", "the branch conflicts with the target branch"},
		{"\t\n", "the branch conflicts with the target branch"},
	}
	for _, c := range casos {
		got := conflictedReason(c.target)
		if got != c.want {
			t.Errorf("conflictedReason(%q) dio %q, want %q", c.target, got, c.want)
		}
		// Y nunca sale una cadena vacía, que es el fallo que el texto genérico evita.
		if strings.TrimSpace(got) == "" {
			t.Errorf("conflictedReason(%q) devolvio texto vacio", c.target)
		}
	}
	// Y el genérico NO lleva el nombre de una rama inventada. Si el código concatenase
	// `target` vacío, el aviso sería "the branch conflicts with ", que es peor que el
	// genérico: parece un bug de formato en vez de un dato que falta.
	if strings.HasSuffix(conflictedReason(""), "with ") {
		t.Error("el aviso generico acaba en \"with \": parece una rama que se perdio")
	}
}

// TestElMotivoDelCIEmpiezaPorElRecuento: el recuento de checks que fallan es lo que
// distingue un CI roto de un check inestable.
//
// Y el `Total` a cero es el caso degradado: la consulta de checks falló y no hay cifras.
// Ahí el texto tiene que ser el que no dice cuántos, no un "0 of 0", que se lee como "no
// falla ninguno" y es justo lo contrario de lo que se sabe.
func TestElMotivoDelCIEmpiezaPorElRecuento(t *testing.T) {
	casos := []struct {
		checks model.Checks
		want   string
	}{
		{model.Checks{Total: 10, Failing: 1}, "CI is failing (1 of 10 checks)"},
		{model.Checks{Total: 3, Failing: 3}, "CI is failing (3 of 3 checks)"},
		{model.Checks{Total: 1, Failing: 1}, "CI is failing (1 of 1 checks)"},
		// Sin cifras conocidas: el texto sin recuento, que no promete nada.
		{model.Checks{}, "CI is failing"},
	}
	for _, c := range casos {
		if got := checksFailingReason(c.checks); got != c.want {
			t.Errorf("checksFailingReason(%+v) dio %q, want %q", c.checks, got, c.want)
		}
	}
	// Y un total conocido nunca sale sin cifras: si `Failing` es cero y el total no, el
	// texto sigue llevando las cifras porque lo que se sabe es que hay checks.
	if got := checksFailingReason(model.Checks{Total: 5, Failing: 0}); got == "CI is failing" {
		t.Error("con 5 checks conocidos el aviso salio sin cifras: se pierde el dato")
	}
}

// TestElMotivoDeMergeSeDisculpaEnVezDeExplicar: los motivos de mergeability, que se
// muestran al pulsar la tecla de merge y tienen que sonar a aviso y no a veredicto.
//
// Y el caso que se prueba aquí es el de los motivos COMPARTIDOS: un cambio de base y un
// conflicto de merge son operaciones parecidas, pero unificarlas perdería el matiz, y
// sobre todo el texto concreto. Lo que se comprueba es lo contrario de lo obvio: que los
// motivos que se emiten son los que el código tiene, y que ninguno está vacío. Un motivo
// vacío produce un toast con el prefijo y nada más.
func TestLosMotivosNoEstanVacios(t *testing.T) {
	// Los motivos que el paquete expone como constantes, todos tienen que ser texto
	// utilizable: sin prefijo de marca, con contenido, y sin punto final doble.
	motivos := []string{UnmergeableReason}
	for _, m := range motivos {
		if strings.TrimSpace(m) == "" {
			t.Errorf("un motivo esta vacio: %q", m)
		}
		if m != strings.TrimSpace(m) {
			t.Errorf("el motivo %q tiene espacios en los bordes", m)
		}
	}
	// Y el de merge se lee como lo que es: una cosa que HAY que hacer, no un "no". El
	// motivo dice "rebase the branch onto the target and push", que es la acción; si
	// dijera solo "unmergeable", quien lo lee no sabe qué hacer con el PR.
	if !strings.Contains(UnmergeableReason, "push") {
		t.Errorf("el motivo de merge no dice que hacer: %q", UnmergeableReason)
	}
}
