package tui

import (
	"testing"
	"time"

	"prdash/internal/forge/model"
)

// `applyItemUpdate` es la función que hace que una acción se vea. Sin ella, mergear un PR
// lo dejaría con su estado viejo en la tabla hasta el siguiente refresco —hasta un minuto—,
// y el usuario pulsaría la tecla otra vez.
//
// Y su caso interesante no es el de actualizar un ítem que ya está: es el de un ítem que NO
// está. Una acción puede sacar un ítem del inbox —un retarget lo cambia de sección—, y el
// resultado de la consulta viene con un ítem nuevo que la tabla no conoce. Añadirlo es lo
// que evita que el resultado de un merge desaparezca de la vista un segundo antes de
// aparecer como mergeado.
//
// Y `mergeItem` al lado resuelve el problema que crea ese append: la relectura del forge no
// sabe de qué sección del inbox venía el ítem, así que sinertia lo metería en una sección
// vacía y desaparecería de donde estaba.

// TestUnItemQueNoEstaSeAnadeYNoSePierdeLaSeccion: la relectura sin sección.
//
// Y es el fallo que `mergeItem` existe para evitar, y es silencioso: el ítem se guarda con
// la sección a cero, la tabla lo busca en un stream que no existe, y el resultado de la
// acción que el usuario acaba de ejecutar desaparece de la pantalla. No hay error, no hay
// toast, y el ítem vuelve al siguiente refresco.
func TestUnItemQueNoEstaSeAnadeYNoSePierdeLaSeccion(t *testing.T) {
	m := newTestModel(t)
	original := model.Item{
		Section: model.SectionReview, ReviewKind: model.ReviewRequested,
		Forge: "github", Host: "github.com", Number: 7,
		Ref: model.RepoRef{Project: "o/r"}, Title: "antes", UpdatedAt: time.Now(),
	}
	conItems(t, &m, original)
	m.rebuild()

	// La relectura: el forge contesta el estado pero no sabe de qué sección del inbox venía.
	releida := original
	releida.Section = ""
	releida.ReviewKind = ""
	releida.Title = "después"
	m.applyItemUpdate(releida)

	// El ítem se conserva, con su título nuevo Y su sección original.
	it, ok := findItem(todosLosItems(m), original.ID())
	if !ok {
		t.Fatalf("el item desaparecio tras actualizarlo: %v", todosLosItems(m))
	}
	if it.Title != "después" {
		t.Errorf("el titulo no se actualizó: %q", it.Title)
	}
	if it.Section != model.SectionReview {
		t.Errorf("la seccion se perdio: %q", it.Section)
	}
	if it.ReviewKind != model.ReviewRequested {
		t.Errorf("el tipo de review se perdio: %q", it.ReviewKind)
	}
	// Y sigue en el stream de antes, que es donde el usuario lo tenía delante. Si se
	// hubiera creado uno nuevo con la seccion a cero, la tabla no lo pintaría.
	conItems := 0
	for _, s := range m.streams {
		conItems += len(s.items)
	}
	if conItems != 1 {
		t.Errorf("hay %d items en los streams tras actualizar, want 1", conItems)
	}
}

// TestUnItemQueNoEstaSeAnadeAlStreamQueToca: el append, y a qué stream.
//
// Y este es el otro caso de `applyItemUpdate`: un ítem que la tabla no conoce se crea en el
// stream de su propia sección. El motivo es el retarget: al cambiar la base, la sección del
// ítem cambia, y si la relectura trae la sección nueva, tiene que ir a un stream distinto
// del que estaba.
//
// Y lo que se comprueba es la clave completa del stream —forge, sección y tipo— y no solo
// la sección. Dos forges con un ítem del mismo número son dos ítems distintos, y meterlos
// en el mismo stream los mezcla en la tabla.
func TestUnItemQueNoEstaSeAnadeAlStreamQueToca(t *testing.T) {
	m := newTestModel(t)
	m.rebuild()

	nuevo := model.Item{
		Section: model.SectionMentions, ReviewKind: "",
		Forge: "gitlab", Host: "gitlab.com", Number: 3,
		Ref: model.RepoRef{Project: "o/r"}, Title: "nuevo",
	}
	m.applyItemUpdate(nuevo)

	it, ok := findItem(todosLosItems(m), nuevo.ID())
	if !ok {
		t.Fatalf("el item anadido no aparece: %v", todosLosItems(m))
	}
	if it.Title != "nuevo" || it.Forge != "gitlab" {
		t.Errorf("el item anadido no llego entero: %+v", it)
	}
	// Y está en el stream de menciones, no en el de review.
	enReview := false
	for k, s := range m.streams {
		for _, si := range s.items {
			if si.ID() == nuevo.ID() && k.section != model.SectionMentions {
				enReview = true
			}
		}
	}
	if enReview {
		t.Error("el item se metio en un stream que no es el suyo")
	}

	// Y dos ítems del mismo número en forges distintos son dos ítems, y no se pisan.
	otro := nuevo
	otro.Forge = "github"
	otro.Host = "github.com"
	m.applyItemUpdate(otro)
	total := 0
	for _, s := range m.streams {
		total += len(s.items)
	}
	if total != 2 {
		t.Errorf("dos forges con el mismo numero acabaron con %d items, want 2", total)
	}
}

// TestActualizarNoDuplicaNiBorraLoDemas: la actualización in situ.
//
// Y son las tres cosas que un update tiene que NO hacer, y cada una tiene un síntoma
// distinto. Duplicar pone dos filas del mismo PR en la tabla. Perder el foco después de una
// acción manda al usuario a otra parte del inbox. Y borrar los demás ítems deja la tabla
// con uno solo, que es el síntoma más difícil de leer porque parece que el filtro funcionó.
func TestActualizarNoDuplicaNiBorraLoDemas(t *testing.T) {
	m := newTestModel(t)
	base := model.Item{
		Section: model.SectionReview, ReviewKind: model.ReviewRequested,
		Forge: "github", Host: "github.com", Number: 1,
		Ref: model.RepoRef{Project: "o/r"}, Title: "uno",
	}
	dos := base
	dos.Number = 2
	dos.Title = "dos"
	tres := base
	tres.Number = 3
	tres.Title = "tres"
	conItems(t, &m, base, dos, tres)
	m.rebuild()
	antes := len(todosLosItems(m))

	actualizada := base
	actualizada.Title = "uno cambiado"
	actualizada.State = "MERGED"
	m.applyItemUpdate(actualizada)

	if len(todosLosItems(m)) != antes {
		t.Errorf("tras actualizar hay %d items y habia %d", len(todosLosItems(m)), antes)
	}
	// Y el actualizado aparece UNA vez, no dos.
	vistos := 0
	for _, it := range todosLosItems(m) {
		if it.ID() == base.ID() {
			vistos++
			if it.Title != "uno cambiado" || it.State != "MERGED" {
				t.Errorf("el item no quedo actualizado: %+v", it)
			}
		}
	}
	if vistos != 1 {
		t.Errorf("el item aparece %d veces tras actualizarlo", vistos)
	}
	// Y los otros siguen con su título.
	titulos := map[string]bool{}
	for _, it := range todosLosItems(m) {
		titulos[it.Title] = true
	}
	for _, quiere := range []string{"uno cambiado", "dos", "tres"} {
		if !titulos[quiere] {
			t.Errorf("tras actualizar falta %q; quedan %v", quiere, titulos)
		}
	}
}

// TestMergeItemConservaLoQueElForgeNoSabeYCambiaLoQueSi: las dos mitades.
//
// Y el `default` importa tanto como los dos `if`: cuando el releído TRAE sección, se usa la
// suya. Es el caso de un retarget, donde la sección nueva es la correcta y conservar la
// vieja dejaría el ítem en el stream que ya no le toca.
func TestMergeItemConservaLoQueElForgeNoSabeYCambiaLoQueSi(t *testing.T) {
	viejo := model.Item{Section: model.SectionReview, ReviewKind: model.ReviewRequested, Title: "viejo"}

	// Sin sección ni kind: se conservan los dos.
	fresco := model.Item{Title: "fresco"}
	merged := mergeItem(viejo, fresco)
	if merged.Section != model.SectionReview || merged.ReviewKind != model.ReviewRequested {
		t.Errorf("no conservo lo que el releido no traia: %+v", merged)
	}
	// El resto viene del releído, no del viejo: es un replace con dos campos rellenados,
	// no un merge de structs.
	if merged.Title != "fresco" {
		t.Errorf("el titulo no es el del releido: %q", merged.Title)
	}

	// Con sección puesta: manda la del releído. Y esto es el retarget.
	fresco = model.Item{Title: "fresco", Section: model.SectionAuthored, ReviewKind: ""}
	merged = mergeItem(viejo, fresco)
	if merged.Section != model.SectionAuthored {
		t.Errorf("con seccion propia se puso la vieja: %q", merged.Section)
	}
	// Y el kind, que el releído no trae, sigue siendo el viejo.
	if merged.ReviewKind != model.ReviewRequested {
		t.Errorf("conservar la seccion impidio conservar el kind: %q", merged.ReviewKind)
	}

	// Y al revés: kind propio con sección vacía.
	fresco = model.Item{Title: "fresco", ReviewKind: model.ReviewAssigned}
	merged = mergeItem(viejo, fresco)
	if merged.Section != model.SectionReview || merged.ReviewKind != model.ReviewAssigned {
		t.Errorf("no mezclo bien: %+v", merged)
	}

	// Y un viejo sin nada: el releído pasa tal cual, sin inventar.
	merged = mergeItem(model.Item{}, model.Item{Title: "fresco"})
	if merged.Section != "" || merged.ReviewKind != "" {
		t.Errorf("con un viejo vacio se invento algo: %+v", merged)
	}
}

// TestSiNoEnElDetalleNoEsUnGuion: la diferencia entre "no" y "no se sabe".
//
// Y el comentario del código la dice y es la razón de que exista la función: un `false`
// aquí es la RESPUESTA del forge, no un dato ausente. Con `orDash` —que devuelve "-" para
// lo vacío— un "no" se pintaría como un guion, que se lee como "el forge no dijo nada". Y la
// diferencia importa porque la columna decide si hace falta una segunda confirmación antes
// de mergear.
func TestSiNoEnElDetalleNoEsUnGuion(t *testing.T) {
	if got := yesNo(true); got != "yes" {
		t.Errorf("yesNo(true) dio %q", got)
	}
	if got := yesNo(false); got != "no" {
		t.Errorf("yesNo(false) dio %q, want \"no\": un false es respuesta, no ausencia", got)
	}
	// Y la diferencia con `orDash` es real: un false NO sale como guion.
	if yesNo(false) == orDash("") {
		t.Error("yesNo(false) sale igual que orDash de una cadena vacia: son cosas distintas")
	}
	// Y el guion sigue siendo lo de lo vacío, que es su función.
	if got := orDash(""); got != "-" {
		t.Errorf("orDash(\"\") dio %q", got)
	}
	if got := orDash("algo"); got != "algo" {
		t.Errorf("orDash con texto dio %q", got)
	}
}

// conItems puebla el stream del item en los streams del modelo, que es el estado que
// `rebuild` convierte en la vista. Es lo que hacen los tests de la TUI para no depender de
// una consulta a un forge.
func conItems(t *testing.T, m *Model, items ...model.Item) {
	t.Helper()
	for _, it := range items {
		k := streamKey{forge: it.Forge, section: it.Section, kind: it.ReviewKind}
		s := m.streams[k]
		if s == nil {
			s = &stream{}
			m.streams[k] = s
		}
		s.items = append(s.items, it)
	}
}

// todosLosItems devuelve los items de todos los streams, que es lo que el rebuild compone
// en el inbox. Recorrer los streams y no el inbox es a proposito: lo que se comprueba aqui
// es el ESTADO del que el render tira, y el render tira de `m.inbox`.
func todosLosItems(m Model) []model.Item {
	var out []model.Item
	for _, s := range m.streams {
		out = append(out, s.items...)
	}
	return out
}
