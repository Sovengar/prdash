package forge_test

import (
	"context"
	"strings"
	"testing"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/state"
	"prdash/internal/testutil"
)

// Estas dos ramas son el borde de `forge` con la TUI: lo que el paquete decide ANTES de tocar
// el forge, y lo que le pasa al adapter cuando todo va bien.
//
// Y `RunAction` es el sitio donde eso importa más, porque es el último guard antes de una
// acción IRREVERSIBLE en el repo de otra persona. Un guard que deja pasar un caso equivocado
// no da un aviso: da un approve sobre el PR equivocado.

// TestRunActionDelMergePasaElHeadSHAReleidoYNoElDeLaFicha: `runOn` con `ActionMerge`.
//
// Y el `HeadSHA` que viaja al merge es el del ESTADO RELEÍDO, no el de la ficha que el usuario
// vio en pantalla, y esa diferencia es todo el propósito del pin. Entre el refresco y la
// pulsación alguien puede haber subido commits al PR, y si el merge se pidiera con el SHA
// viejo integraría commits que nadie miró.
//
// Y el modo y el borrado de rama vienen del `MergeRequest` del llamador, y se comprueban por
// separado: son la otra mitad de lo que el usuario eligió en el diálogo de confirmación, y
// perderlos es peor que un SHA equivocado porque el borrado de rama es destructivo.
func TestRunActionDelMergePasaElHeadSHAReleidoYNoElDeLaFicha(t *testing.T) {
	// La ficha que el usuario vio: SHA viejo.
	visto := mkItem("github", "github.com", "acme/widget", 3)
	visto.State = "OPEN"
	visto.HeadSHA = "sha-que-el-usuario-vio"

	// El estado releído: SHA nuevo.
	releido := visto
	releido.HeadSHA = "sha-nuevo-del-refresco"

	fake := adapterQuePideElEstado(releido)
	req := forge.MergeRequest{Mode: forge.Squash, DeleteBranch: true}
	out := forge.RunAction(context.Background(), fake, forge.ActionMerge, visto.Ref, 3, req)

	if !fake.seLlamo {
		t.Error("el merge no llegó a Merge: no hay caso bueno contra el que comparar")
	}
	// El `Merge` del doble devuelve un aviso de "no implementado", así que el resultado no sale
	// bien: eso es lo que permite comprobar que la llamada ocurrió y con qué petición.
	if out.OK {
		t.Errorf("el fake no fusiona y el resultado salió bien: %+v", out)
	}
	if out.Mode != forge.Squash || !out.DeleteBranch {
		t.Errorf("la petición salió con modo %q y borrado %v: son lo que el usuario eligió",
			out.Mode, out.DeleteBranch)
	}
	// Y el estado releído es el que acompaña al resultado, que es lo que permite pintar "el
	// PR ya está fusionado" en vez de recargar.
	if !out.HasItem {
		t.Error("el resultado no trae el estado releído: la TUI no puede refrescar la fila")
	}
	if out.Item.HeadSHA != "sha-nuevo-del-refresco" {
		t.Errorf("el estado releído trae SHA %q: se aplicaría sobre una ficha vieja",
			out.Item.HeadSHA)
	}
}

// TestUnPRYaFusionadoOCerradoBloqueaElMergeSinLlamarAlForge: los dos estados que sí lo bloquean.
//
// Y son los dos únicos, y conviene decirlo porque la primera versión del test metía también
// `DIRTY` y `BLOCKED` —ramas en conflicto y CI en rojo— esperando que bloquearan. No lo hacen:
// `RunAction` solo mira el ESTADO DEL PR, con `state.Actionable`, y un PR con las ramas en
// conflicto sigue siendo integrable en el forge.
//
// Y eso no es un agujero: el bloqueo de un CI en rojo es una política, y lo aplica
// `state.MergeBlock` en la TUI, con su confirmación. Ponerlo en `RunAction` convertiría la
// herramienta en un muro —nadie podría fusionar con el CI en rojo ni aunque lo hubiera
// arreglado— y separaría el "¿se puede?" del "¿debe?".
//
// Y lo que sí se comprueba para los dos que bloquean es que el guard corta ANTES de llamar al
// forge, que es lo que distingue "rechazado" de "falló después": un merge que se lanza sobre un
// PR ya fusionado puede integrar medio sin que nadie lo sepa.
func TestUnPRYaFusionadoOCerradoBloqueaElMergeSinLlamarAlForge(t *testing.T) {
	for _, c := range []struct {
		nombre string
		estado string
	}{
		{"ya fusionado", "MERGED"},
		{"ya cerrado", "CLOSED"},
	} {
		it := mkItem("github", "github.com", "acme/widget", 4)
		it.State = c.estado
		it.HeadSHA = "abc123"
		fake := adapterQuePideElEstado(it)

		out := forge.RunAction(context.Background(), fake, forge.ActionMerge, it.Ref, 4,
			forge.MergeRequest{Mode: forge.MergeCommit})

		if out.OK {
			t.Errorf("%s: el merge salió bien sobre un PR %q", c.nombre, c.estado)
		}
		if fake.seLlamo {
			t.Errorf("%s: se llamó a Merge: un merge lanzado sobre un PR %q puede "+
				"integrar medio sin que nadie lo sepa", c.nombre, c.estado)
		}
		if !out.Conflict {
			t.Errorf("%s: no se marcó como conflicto (%+v): la TUI no ofrecería refrescar",
				c.nombre, out)
		}
		// Y el motivo dice el motivo: "already merged", no un "algo falló" genérico. La
		// diferencia entre "vuelve a refrescar" y "no vuelvas a pulsar esto" la sabe el
		// estado del PR, y el motivo es lo que se lo cuenta al usuario.
		if !strings.Contains(out.Msg, "already") {
			t.Errorf("%s: el motivo %q no dice que el PR ya está %s", c.nombre, out.Msg, c.estado)
		}
		// Y el ítem releído viene, que es lo que permite pintar por qué sin recargar.
		if !out.HasItem || out.Item.Number != 4 {
			t.Errorf("%s: el resultado no trae el ítem re-leído", c.nombre)
		}
	}

	// Y el caso que de verdad NO bloquea, para que lo anterior no sea "nunca fusiona": un PR
	// con las ramas en conflicto llega al forge, porque resolverlo es trabajo del usuario y el
	// forge es quien dice si se puede.
	it := mkItem("github", "github.com", "acme/widget", 40)
	it.State = "DIRTY"
	it.HeadSHA = "abc123"
	fake := adapterQuePideElEstado(it)
	if out := forge.RunAction(context.Background(), fake, forge.ActionMerge, it.Ref, 40,
		forge.MergeRequest{Mode: forge.MergeCommit}); !fake.seLlamo || out.OK {
		t.Errorf("un PR con las ramas en conflicto no llegó a Merge, o salió bien: "+
			"llamado=%v out=%+v. El bloqueo blando lo aplica MergeBlock, no RunAction",
			fake.seLlamo, out)
	}
}

// TestUnaAccionQueNoEsNiApproveNiMergeNoSeDespachaYNoSeLeeElItem: el corte de `RunAction`.
//
// Y este test es el que documenta un bug. Antes, el `default` del dispatch devolvía nil, y nil es
// lo que `classifyAction` lee como "no hubo ningún warning" —su forma de decir que la acción
// salió bien—. Así que `RunAction` con una acción desconocida devolvía `OK: true` sin haber
// hecho nada, y el aviso de la cabecera, que se compone de `OK`, decía lo contrario de lo que
// pasó.
//
// Y no se ve en la TUI porque `canActionOn` filtra por approve y merge, así que es una
// contención: y una contención que informa de éxito no contiene. El día que un Kind nuevo se
// enrute por aquí sin añadir su rama, el usuario ve "hecho" sobre una acción que no ocurrió.
//
// Y lo que se comprueba son las TRES mitades del arreglo:
//
//   - No sale bien, y el motivo dice qué pasa y cuál es la acción, para que quien depura no
//     tenga que adivinar qué Kind se coló.
//   - No se despacha al adapter: ni approve ni merge.
//   - Y NO se relee el ítem. Ese es el detalle que justifica que el corte esté antes de
//     `runOn` y no dentro del `exec`: `runOn` empieza releyendo el estado del forge, así que
//     despachar una acción imposible costaría un viaje de ida y vuelta a GitHub para nada.
func TestUnaAccionQueNoEsNiApproveNiMergeNoSeDespachaYNoSeLeeElItem(t *testing.T) {
	for _, kind := range []forge.ActionKind{
		forge.ActionRetarget, forge.ActionKind("inventado"), forge.ActionKind(""),
	} {
		it := mkItem("github", "github.com", "acme/widget", 8)
		it.State = "OPEN"
		it.HeadSHA = "abc123"
		fake := adapterQuePideElEstado(it)
		fake.leeStates = 0

		out := forge.RunAction(context.Background(), fake, kind, it.Ref, 8,
			forge.MergeRequest{Mode: forge.MergeCommit})

		if out.OK {
			t.Errorf("la acción %q salió bien sin hacerse: el aviso de la cabecera diría "+
				"que la operación funcionó", kind)
		}
		if fake.seLlamo {
			t.Errorf("la acción %q llegó a Merge", kind)
		}
		if fake.leeStates != 0 {
			t.Errorf("la acción %q releyó el ítem %d veces: una acción imposible no debe "+
				"gastar un viaje al forge", kind, fake.leeStates)
		}
		// Y el motivo es canónico y NOMBRA la acción: sin ella, quien depura tiene que
		// adivinar qué Kind se coló por una firma que no dice cuál.
		if !strings.Contains(out.Msg, "does not implement") {
			t.Errorf("la acción %q dio el motivo %q, que no dice que no está implementada", kind, out.Msg)
		}
		if !strings.Contains(out.Msg, string(kind)) {
			t.Errorf("la acción %q dio el motivo %q, que no la nombra", kind, out.Msg)
		}
		// Y no es conflicto ni permiso. Conflicto es "el ítem cambió, refresca" —el ítem no
		// ha cambiado— y permiso es "esta acción está deshabilitada para este ítem", que en
		// la TUI lo registra como denegado para siempre. Un fallo de quién llamó no es
		// ninguna de las dos cosas.
		if out.Conflict || out.Perm || out.Unmergeable {
			t.Errorf("la acción %q se clasificó como %+v: no es un conflicto ni un permiso, "+
				"es un fallo de quién llamó", kind, out)
		}
		// Y el Kind y el ID viajan, para que el aviso se atribuya a la acción y al ítem que
		// se intentó tocar.
		if out.Kind != kind {
			t.Errorf("el Kind del resultado es %q, want %q", out.Kind, kind)
		}
		if out.ID.Project != "acme/widget" || out.ID.Number != 8 {
			t.Errorf("el ID del resultado es %+v: el aviso no se podría atribuir al ítem", out.ID)
		}
	}

	// Y el control: con approve sí se relee el ítem, que es lo que hace que lo anterior sea
	// "corta antes de releer" y no "nunca relee".
	it := mkItem("github", "github.com", "acme/widget", 9)
	it.State = "OPEN"
	bueno := adapterQuePideElEstado(it)
	bueno.leeStates = 0
	forge.RunAction(context.Background(), bueno, forge.ActionApprove, it.Ref, 9,
		forge.MergeRequest{})
	if bueno.leeStates == 0 {
		t.Error("con approve no se releyó el ítem: el guard no es lo que cortó, y el estado " +
			"del inbox se quedaría sin refrescar")
	}
}

// TestAprobarLoPropioSeRechazaConMotivo: el veto de la autoaprobación.
//
// Y es un veto que no depende del forge: nadie puede aprobar su propio PR, y el forge lo
// rechazaría con un error que no explica la regla. Avisar aquí convierte un error opaco en un
// motivo que el usuario entiende, y evita el viaje de ida y vuelta.
//
// Y el motivo tiene que decir que es SU PR, no "no se puede aprobar": sin el sujeto, el usuario
// busca el problema en sus permisos en vez de en la regla.
func TestAprobarLoPropioSeRechazaConMotivo(t *testing.T) {
	it := mkItem("github", "github.com", "acme/widget", 7)
	it.State = "OPEN"
	it.HeadSHA = "abc123"
	it.Author = "yo-mismo"
	// Y el adapter devuelve el aviso de clase `selfreview`, que es como lo devuelven los dos
	// reales: el veto NO está en `forge`, está en `Adapter.Approve`. Mi primera versión lo
	// buscaba en `RunAction` y aprobaba su propio PR sin quejarse.
	fake := adapterQuePideElEstado(it)
	fake.approveVeto = true

	out := forge.RunAction(context.Background(), fake, forge.ActionApprove, it.Ref, 7,
		forge.MergeRequest{})
	if out.OK {
		t.Fatal("se aprobó el propio PR")
	}
	if strings.TrimSpace(out.Msg) == "" {
		t.Fatal("sin motivo: el usuario ve que la tecla no hace nada")
	}
	// Y el motivo es el CANÓNICO, no el texto de la CLI. El adapter puede decir lo que quiera
	// —cada CLI tiene su frase— y lo que se le enseña al usuario es uno solo, en el idioma del
	// producto. Por eso se sustituye y no se copia.
	if out.Msg != state.SelfReviewReason {
		t.Errorf("el motivo es %q, want el canónico %q", out.Msg, state.SelfReviewReason)
	}
	// Y se clasifica como PERMISO y no como conflicto, que es lo que evita que la TUI lo
	// reintente: es una denegación permanente de ese ítem, y re-leer el estado no lo cambia.
	if !out.Perm {
		t.Errorf("el veto salió como %+v: sin la marca de permiso la TUI lo reintentaría "+
			"en cada refresco", out)
	}
	if out.Conflict {
		t.Error("el veto salió además como conflicto: dos clases a la vez y la TUI no sabe " +
			"cuál ofrecer")
	}
}

// adapterQueCuestaSiSeMergeo embebe el `FakeAdapter` del proyecto y solo anota si le pidieron
// fusionar.
//
// Y embebecer en vez de implementar la interfaz entera es lo que hace falta aquí, y no por
// comodidad: `runOn` RE-LEE el ítem antes de actuar, así que un adapter que devuelve el valor
// cero hace que todos los tests midieran lo mismo —"could not re-read the item"— y el guard que
// se quería comprobar no llegaba a ejecutarse.
//
// Y la única diferencia con el `FakeAdapter` es que `Merge` anota la llamada, que es lo que
// convierte "el guard paró" en un hecho en vez de una impresión.
type adapterQueCuestaSiSeMergeo struct {
	testutil.FakeAdapter
	seLlamo bool
	// leeStates cuenta las relecturas de `ItemState`, que es lo que hace observable que el
	// corte de una acción imposible esté ANTES de `runOn` y no dentro del dispatch. Sin el
	// contador, "no llamó a Merge" y "no costó un viaje al forge" serían la misma prueba.
	leeStates int
	// approveVeto hace que `Approve` devuelva el aviso de clase `selfreview` que devuelven
	// los dos adapters reales cuando el PR es del propio usuario. Es un flag y no una
	// sustitución de método porque los métodos no se pueden asignar en un valor embebido:
	// `fake.Approve = ...` no compila, y la vía era un doble con un campo de configuración.
	approveVeto bool
}

func (a *adapterQueCuestaSiSeMergeo) Approve(
	_ context.Context, _ model.RepoRef, _ int,
) []model.Warning {
	if a.approveVeto {
		return []model.Warning{{Forge: a.ForgeName, Kind: "selfreview", Msg: "you cannot approve"}}
	}
	return a.FakeAdapter.Approve(context.Background(), model.RepoRef{}, 0)
}

func (a *adapterQueCuestaSiSeMergeo) ItemState(
	ctx context.Context, ref model.RepoRef, number int,
) (model.Item, []model.Warning) {
	a.leeStates++
	return a.FakeAdapter.ItemState(ctx, ref, number)
}

func (a *adapterQueCuestaSiSeMergeo) Merge(
	_ context.Context, _ model.RepoRef, _ int, _ forge.MergeRequest,
) []model.Warning {
	a.seLlamo = true
	return []model.Warning{{Forge: a.ForgeName, Kind: "denegado", Msg: "el fake no fusiona"}}
}

// adapterQuePideElEstado es el doble con el `ItemState` bien poblado, que es el mínimo para
// que `runOn` pase del re-lectura a los guards que se quieren probar.
func adapterQuePideElEstado(it model.Item) *adapterQueCuestaSiSeMergeo {
	return &adapterQueCuestaSiSeMergeo{
		FakeAdapter: testutil.FakeAdapter{
			ForgeName:  it.Forge,
			HostName:   it.Host,
			ItemStates: map[string]model.Item{testutil.ItemKey(it.Ref.Project, it.Number): it},
		},
	}
}
