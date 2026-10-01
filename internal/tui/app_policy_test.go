package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"prdash/internal/cache"
	"prdash/internal/config"
	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

// TestElTickSoloSeArmaConAutoRefresh: el auto-refresh es lo que arma el primer tick.
//
// Con el intervalo en cero no hay nada que consultar, así que no se arma. Lo que se
// afirma es el estado interno, porque "no armar el tick" no se ve en un render: se ve
// como un spinner que gira cuando no se ha refrescado nada, que es un falso "estoy
// trabajando" sobre datos viejos.
//
// Y el borde es el intervalo en cero, que es el valor por defecto y el de "apagado".
func TestElTickSoloSeArmaConAutoRefresh(t *testing.T) {
	// Con intervalo, el tick queda pendiente: hay que consultar.
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	conIntervalo := config.Defaults()
	conIntervalo.RefreshInterval = 2 * time.Second
	m := New(conIntervalo, nil)
	if !m.tickPending {
		t.Error("con intervalo de refresco no quedó ningun tick pendiente: no se va a consultar nada")
	}

	// Sin intervalo, no. Y cero es el valor por defecto de "apagado", asi que es el
	// caso que hay que mirar: un "> 0" comprobado como "no es cero" daria lo mismo,
	// pero un ">=-0" en el otro sentido armaria un tick con intervalo cero, que es
	// un bucle que consulta sin parar.
	sinIntervalo := config.Defaults()
	sinIntervalo.RefreshInterval = 0
	m = New(sinIntervalo, nil)
	if m.tickPending {
		t.Error("sin intervalo de refresco quedó un tick pendiente: un bucle que consulta sin parar")
	}
}

// TestSelectedNoSaleDelRango: el ítem bajo el cursor tiene que EXISTIR.
//
// El borde es el cursor una posición más allá del final. La lista no es circular —
// el cursor se queda en el último ítem al llegar al final, no vuelve al primero—, así
// que un cursor en `len(rows)` es un estado que se puede alcanzar por otras vías: un
// refresco que devuelve menos ítems, o un cambio de sección. Y en ese estado,
// indexar sin comprobar es un PANIC, no un fallo.
//
// Por eso la comprobación es `>=` y no `>`, y por eso el caso del cursor exactamente
// en el final está aquí y no en un test de navegación.
func TestSelectedNoSaleDelRango(t *testing.T) {
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	m.applySnapshot(snapshotCon(mkItem("github", "github.com", "acme/widget", "uno", 1, "")))
	m.rebuild()

	if got, ok := m.selected(); !ok || got.Number != 1 {
		t.Fatalf("con el cursor en 0, selected = %d, %v; quiero 1, true", got.Number, ok)
	}

	// El cursor en la posición DESPUÉS del último no devuelve un ítem. Con un ">" en
	// vez de un ">=", intentaría indexar fuera del slice.
	for _, cursor := range []int{1, 2, 100} {
		m.cursor = cursor
		if got, ok := m.selected(); ok {
			t.Errorf("con el cursor en %d devolvió el ítem %d: está fuera de la lista de %d",
				cursor, got.Number, len(m.rows()))
		}
		got, _ := m.selected()
		if got != (model.Item{}) {
			t.Errorf("con el cursor fuera de rango devolvió %+v, want el ítem cero: "+
				"un item a medias se parece a uno bueno", got)
		}
	}

	// Y con la lista vacía tampoco, que es el caso de un forge que falla: sin ítems
	// no hay nada bajo el cursor.
	m.applySnapshot(snapshotCon())
	m.rebuild()
	m.cursor = 0
	if _, ok := m.selected(); ok {
		t.Error("con la lista vacía devolvió un ítem")
	}
}

// TestSaveSnapshotSinRutaNoEscribeNada: sin ruta de cache no se persiste nada.
//
// Y tiene que ser así de silencioso. Un "escribir igualmente" dejaría un fichero de
// snapshot en un sitio que el usuario no eligió —el directorio de trabajo, o la raíz
// del repo—, y eso no es un detalle: un fichero que aparece donde no se lo pidió es un
// fichero que nadie limpia.
//
// Se comprueba por lo que hay en disco, que es el único sitio donde se ve.
func TestSaveSnapshotSinRutaNoEscribeNada(t *testing.T) {
	dir := t.TempDir()
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	m.applySnapshot(snapshotCon(mkItem("github", "github.com", "acme/widget", "uno", 1, "")))
	m.rebuild()

	// Sin ruta: no escribe. Y no falla.
	m.cachePath = ""
	antes := entradasEn(t, dir)
	m.saveSnapshot()
	if despues := entradasEn(t, dir); len(despues) != len(antes) {
		t.Errorf("sin ruta de cache aparecieron ficheros: %v", despues)
	}

	// Con ruta: escribe, y escribe EN ESA RUTA. Ese es el otro lado del borde.
	//
	// Y hay que esperar a que aparezca, porque el guardado va en una goroutine a
	// propósito: bloquear la UI por escribir un fichero de estado sería hacer que un
	// disco lento se notara en el teclado. Que sea asíncrono es el diseño, así que el
	// test lo espera con un techo en vez de mirar una vez y rendirse.
	m.cachePath = filepath.Join(dir, "snapshot.json")
	m.saveSnapshot()
	if !esperaFichero(t, m.cachePath) {
		t.Fatalf("con ruta de cache no escribió el snapshot en %s", m.cachePath)
	}
	// Y el fichero que escribió es JSON válido, que es lo único que lo hace bueno: un
	// snapshot corrupto es un snapshot que se pierde, y se pierde en silencio porque
	// escribir no falla.
	raw, err := os.ReadFile(m.cachePath)
	if err != nil {
		t.Fatalf("no se pudo leer el snapshot escrito: %v", err)
	}
	if len(raw) == 0 {
		t.Error("el snapshot escrito está vacío")
	}
	var f cache.File
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Errorf("el snapshot escrito no es JSON valido: %v", err)
	}
	if len(f.Streams) == 0 {
		t.Error("el snapshot escrito no tiene ningun stream: no guarda lo que hay en pantalla")
	}
}

// snapshotCon arma una instantánea con los ítems dados en la sección de review, que
// es por donde se entra a la lista. Se usa el camino real de la entrada, no un atajo
// al campo interno, porque lo que se afirma es lo que hace el programa.
func snapshotCon(items ...model.Item) cache.File {
	return cache.File{Streams: []cache.Stream{{
		Forge:   "github",
		Host:    "github.com",
		Section: model.SectionReview,
		Kind:    model.ReviewRequested,
		Items:   items,
	}}}
}

// TestUnStreamDeUnForgeNoConfiguradoNoRevienta: un stream puede venir de un forge
// que ya no está en la configuración.
//
// Es un caso real, no unPhantom: la instantánea se persiste, y entre dos ejecuciones
// el usuario puede quitar un forge del TOML. Al abrir, se leen sus streams, y al
// refrescar llegan con la clave de ese forge. Si el estado del forge no existe, tocar
// su `updatedAt` es un PANIC, y un panic en un mensaje de red tumba la TUI entera.
//
// Lo que se afirma es que el mensaje se acepta y se ignora el estado que no está.
func TestUnStreamDeUnForgeNoConfiguradoNoRevienta(t *testing.T) {
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})

	// El stream es de "gitlab", que no está en la configuración.
	m = send(t, m, pageMsg{
		cycle:     1,
		key:       streamKey{forge: "gitlab", section: model.SectionReview},
		items:     []model.Item{mkItem("gitlab", "gitlab.com", "acme/widget", "uno", 1, "")},
		unchanged: true,
	})

	// No hay estado para ese forge, y aun así no ha petado: la TUI sigue viva.
	if _, ok := m.statuses["gitlab"]; ok {
		t.Error("apareció un estado para un forge que no está en la configuración")
	}
	// Y el stream existe y es consultable: los ítems de un forge retirado se pueden
	// seguir viendo, que es distinto de romperse.
	if m.streams[streamKey{forge: "gitlab", section: model.SectionReview}] == nil {
		t.Error("el stream de un forge no configurado no se guardó")
	}
}

// TestReviewKindSoloSeEstampaEnLaSeccionDeReview: el tipo de review es de la sección
// de review, y en ninguna otra.
//
// La regla protege contra un estado mal formado: en la sección de asignados, un ítem
// con ReviewKind es un ítem que dice "esto es un review solicitado" sin estar en la
// lista de reviews. La lista de asignados se pinta con los mismos ítems, así que ese
// campo se acaba leyendo.
//
// Y va al revés de lo que parece: lo que se comprueba es la SECCIÓN del stream, no la
// del ítem, porque es el stream el que sabe de dónde viene la página.
func TestReviewKindSoloSeEstampaEnLaSeccionDeReview(t *testing.T) {
	for _, seccion := range []model.Section{model.SectionReview, model.SectionAuthored, model.SectionMentions} {
		m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
		item := mkItem("github", "github.com", "acme/widget", "uno", 1, "")

		m = send(t, m, pageMsg{
			cycle: 1,
			key:   streamKey{forge: "github", section: seccion, kind: model.ReviewRequested},
			items: []model.Item{item},
			first: true,
		})

		got := m.streams[streamKey{forge: "github", section: seccion, kind: model.ReviewRequested}]
		if got == nil {
			t.Fatalf("sección %v: el stream no se guardó", seccion)
		}
		tiene := got.items[0].ReviewKind != ""
		quiere := seccion == model.SectionReview
		if tiene != quiere {
			t.Errorf("sección %v: ReviewKind %q presente=%v, quiere %v. "+
				"Un ReviewKind fuera de la sección de review dice que el ítem es un review en una lista donde no lo es",
				seccion, got.items[0].ReviewKind, tiene, quiere)
		}
	}
}

// esperaFichero espera a que aparezca un fichero, con techo. El guardado del
// snapshot va en una goroutine, así que "no ha salido" no se sabe mirando una vez.
func esperaFichero(t *testing.T, path string) bool {
	t.Helper()
	for range 200 {
		if _, err := os.Stat(path); err == nil {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

func entradasEn(t *testing.T, dir string) []string {
	t.Helper()
	got, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var nombres []string
	for _, e := range got {
		nombres = append(nombres, e.Name())
	}
	return nombres
}

// TestElEstadoDelForgeSoloSeSellaConHost: el host de un forge se guarda cuando la
// instantánea lo trae, y no cuando viene vacío.
//
// Un host vacío NO es "sé el host": es "no lo sé". Si se sellara con un vacío, el
// estado del forge quedaría con un host vacío, y todo lo que se pinte con él —la
// referencia de cada ítem— saldría sin host, que es como se ven los repos en la lista:
//
//	acme/widget  en vez de  github.com/acme/widget
//
// Y no es un caso raro: un forge recién añadido no tiene host hasta que responde.
func TestElEstadoDelForgeSoloSeSellaConHost(t *testing.T) {
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})

	// Con host: se guarda.
	m.applySnapshot(cache.File{Streams: []cache.Stream{{
		Forge: "github", Host: "github.com", Section: model.SectionReview,
		Items: []model.Item{mkItem("github", "github.com", "acme/widget", "uno", 1, "")},
	}}})
	if got := m.statuses["github"].host; got != "github.com" {
		t.Errorf("con host dio %q, want github.com", got)
	}

	// Sin host: NO se toca el que ya había. Un host vacío no borra un host conocido.
	m.applySnapshot(cache.File{Streams: []cache.Stream{{Forge: "github", Host: ""}}})
	if got := m.statuses["github"].host; got != "github.com" {
		t.Errorf("un snapshot sin host borró el host conocido: quedó en %q. "+
			"Un host vacío es no saberlo, no saber que no lo hay", got)
	}

	// Y un forge que no está en la lista no revienta: la instantánea puede traer uno
	// que se añadió después de abrir.
	m.applySnapshot(cache.File{Streams: []cache.Stream{{Forge: "gitlab", Host: "gitlab.com"}}})
}
