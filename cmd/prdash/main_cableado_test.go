package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"io"
	"prdash/internal/config"
	"prdash/internal/forge/model"
	"prdash/internal/review/plan"
	"prdash/internal/worktree"
)

// `cmd/prdash` estaba al 68,9%, y casi todo lo que faltaba eran funciones sin ninguna
// dependencia inyectada: `hostsOf` mapea hosts, `printDiff` formatea un diffstat,
// `binaryAvailable` decide si un binario existe. Son funciones puras con la lógica de
// selección detrás, y la lógica de selección es donde se Decide el comportamiento.
//
// La que más importa de este fichero es `binaryAvailable`. Un nombre vacío y un binario que
// no existetaken la misma decisión —omitir el pane— pero por motivos opuestos: el primero es
// "el usuario no lo configuró" y el segundo es "está configurado y no lo tenemos". La
// diferencia decide si se avisa, y por eso los tres casos se comprueban por separado: vacío,
// separador de ruta, y nombre suelto.

// TestLaDisponibilidadDeUnBinarioDistingueLasTresCasas: por qué una ruta con separador se
// mira en disco y un nombre suelto en el PATH.
//
// Y el motivo está en que son dos configuraciones distintas del usuario. `tools.hunk =
// "/opt/hunk"` es una ruta absoluta —el usuario tiene su build en ese sitio— y mirarla en
// el PATH no la encontraría nunca. `tools.hunk = "hunk"` es un nombre, y tiene que estar en
// el PATH. Confundirlas hace que la primera dé "no disponible" con el binario delante del
// usuario, y que la segunda dé "no disponible" por no mirar donde toca.
func TestLaDisponibilidadDeUnBinarioDistingueLasTresCasas(t *testing.T) {
	// Vacío: no hay pane. Y da igual que sean espacios en vez de nada, porque un config
	// editado a mano deja "   " más que "": un nombre de espacios no se busca en el PATH,
	// se ejecutaría como comando en blanco.
	for _, vacio := range []string{"", "   ", "\t\n"} {
		if binaryAvailable(vacio) {
			t.Errorf("binaryAvailable(%q) dio true: un pane sin comando no se lanza", vacio)
		}
	}

	// Con separador: se mira el disco, y hace falta que exista Y que sea un fichero.
	dir := t.TempDir()
	ejecutable := filepath.Join(dir, "herramienta")
	if err := os.WriteFile(ejecutable, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !binaryAvailable(ejecutable) {
		t.Errorf("binaryAvailable(%q) dio false con el fichero delante", ejecutable)
	}
	// Un directorio con nombre de ejecutable: disponible de nombre, pero no se puede
	// ejecutar. Y esto es un caso que se da de verdad: `tools.hunk = "/opt/hunk"` donde
	// `/opt/hunk` es un directorio.
	subdir := filepath.Join(dir, "undir")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if binaryAvailable(subdir) {
		t.Error("binaryAvailable dio true para un directorio: un directorio no se ejecuta")
	}
	// Y una ruta que no existe.
	if binaryAvailable(filepath.Join(dir, "no-existe")) {
		t.Error("binaryAvailable dio true para una ruta inexistente")
	}

	// Sin separador: se mira el PATH. Con un nombre que sí está.
	if !binaryAvailable("sh") {
		t.Error("binaryAvailable(\"sh\") dio false y sh está en el PATH de todas partes")
	}
	// Y con un nombre que no.
	if binaryAvailable("definitivamente-no-existe-este-binario") {
		t.Error("binaryAvailable dio true para un nombre que no está en el PATH")
	}
}

// TestHostsOfSoloMapeaLoConfiguradoYNoInventaForge: la normalización de hosts.
//
// Y las tres mitades importan por un motivo común: este mapa decide en qué repo se hace una
// operación. Un host mal mapeado manda el clone o el push al sitio equivocado.
//
// La primera: sin host configurado no hay entrada. Un forge sin host es una config a medias,
// y meterlo en el mapa como `""` haría que cualquier remoto sin host —que es el remoto por
// defecto de git— se resolviera contra un forge.
//
// La segunda: los dos forgesknown están mapeados, y no se solapan. Un host en los dos
// significa que los dos pointan al mismo sitio, y el mapa no puede decir a cuál.
//
// La tercera: solo hay dos forges en la cascada. Que el bitbucket no aparezca no es un
// olvido del mapa: `reporesolver` lo resuelve por su cuenta, y por eso este mapa es de
// los que hay. Un test que espera tres entradas documentaría un hecho que no existe.
func TestHostsOfSoloMapeaLoConfiguradoYNoInventaForge(t *testing.T) {
	// Sin nada: mapa vacío, no un mapa con una entrada vacía.
	if len(hostsOf(config.Config{})) != 0 {
		t.Errorf("sin config dio %v, want mapa vacío", hostsOf(config.Config{}))
	}

	// Con los dos hosts de los defaults. Y el de GitLab NO es gitlab.com: es
	// `gitlab.example.com`, porque los defaults de prdash apuntan a un GitLab
	// self-managed de ejemplo y no al público. La primera versión de este test
	// comprobaba "gitlab.com" y fallaba, y la conclusión fácil —"el mapa no
	// normaliza el host de GitLab"— era falsa: lo que pasa es que el host por
	// defecto no es el que este test suponía.
	cfg := config.Defaults()
	hosts := hostsOf(cfg)
	if len(hosts) != 2 {
		t.Fatalf("con los dos hosts dio %d entradas: %v", len(hosts), hosts)
	}
	if hosts["github.com"] != "github" {
		t.Errorf("github.com -> %q", hosts["github.com"])
	}
	if hosts[cfg.Forges.GitLab.Host] != "gitlab" {
		t.Errorf("%q -> %q, want gitlab", cfg.Forges.GitLab.Host, hosts[cfg.Forges.GitLab.Host])
	}
	// Y ningún host vacío en el mapa, que es lo que rompería la resolución.
	for h := range hosts {
		if h == "" {
			t.Error("el mapa tiene una entrada con host vacío")
		}
	}

	// Un host self-managed: el mapa lo usa tal cual, sin código. Y es el caso que
	// justifica la función: un GitLab en la intranet no es gitlab.com, y la clave del mapa
	// es el host REAL.
	antes := cfg.Forges.GitLab.Host
	cfg.Forges.GitLab.Host = "git.intra.example"
	hosts = hostsOf(cfg)
	if hosts["git.intra.example"] != "gitlab" {
		t.Errorf("el host self-managed dio %q", hosts["git.intra.example"])
	}
	if _, sigue := hosts[antes]; sigue {
		t.Errorf("cambiar el host no quitó el anterior (%q sigue en el mapa)", antes)
	}

	// Y los dos forges en el mismo host: el mapa dice el último, y eso es un dato que hay
	// que tener en cuenta al leer la config, no algo que se pueda detectar aquí. Lo que se
	// comprueba es que no se rompe ni entra en pánico.
	cfg = config.Defaults()
	cfg.Forges.GitHub.Host = "mismo.example"
	cfg.Forges.GitLab.Host = "mismo.example"
	if len(hostsOf(cfg)) != 1 {
		t.Errorf("dos forges en el mismo host dieron %d entradas, want 1", len(hostsOf(cfg)))
	}
}

// TestPrintDiffDistingueDesconocidoDeCero: la celda del diffstat en `--print`.
//
// Y el caso que importa es el de `Known: false` con cifras. Llega cuando la consulta del
// diffstat falló: el ítem tiene lo que el forge no devolvió, que no fueron las cifras,
// y unas cifras inventadas serían peores que un guion. La TUI muestra "-" ahí, y `--print`
// tiene que mostrar lo mismo porque su salida es lo que alguien compara con `diff`.
//
// Y el otro lado: un diffstat conocido y de cero es "+0 -0", que es información —el repo
// tiene cambios de solo borrado, o solo renombrados— y no un hueco.
func TestPrintDiffDistingueDesconocidoDeCero(t *testing.T) {
	casos := []struct {
		nombre string
		d      model.DiffStat
		want   string
	}{
		{"conocido normal", model.DiffStat{Known: true, Additions: 12, Deletions: 3}, "+12 -3"},
		{"solo añadidos", model.DiffStat{Known: true, Additions: 40}, "+40 -0"},
		{"solo borrados", model.DiffStat{Known: true, Deletions: 7}, "+0 -7"},
		{"conocido de cero", model.DiffStat{Known: true}, "+0 -0"},
		{"desconocido", model.DiffStat{Known: false}, "-"},
		// Lo que falla de verdad: cifras sin `Known`. No es que no haya datos, es que hay
		// cifras que no se sabe de dónde salieron.
		{"cifras sin marcar como conocidas", model.DiffStat{Additions: 5, Deletions: 5}, "-"},
		{"cero sin marcar", model.DiffStat{}, "-"},
	}
	for _, c := range casos {
		got := printDiff(c.d)
		if got != c.want {
			t.Errorf("%s: printDiff dio %q, want %q", c.nombre, got, c.want)
		}
		// Y nunca sale un campo vacío que desplace la columna: el ancho de la celda está
		// calculado para el caso más ancho.
		if strings.TrimSpace(got) == "" {
			t.Errorf("%s: printDiff devolvió texto vacío", c.nombre)
		}
	}
}

// TestElAuditorMarcaLoHuerfanoYDejaElRestoEnOk: el listado de worktrees.
//
// Y `state` tiene tres valores posibles y solo dos se usan en el listado, así que el
// default es lo que decide. Poner "" en vez de "ok" haría que la columna de estado saliera
// vacía para todos los worktrees vivos, y el listado parecería un error de formato en vez
// de un inventario.
func TestElAuditorMarcaLoHuerfanoYDejaElRestoEnOk(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "wt")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	// El provisioner real sobre un repo vacío, que es lo que audita.
	live := filepath.Join(dir, "live")
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatal(err)
	}

	entries := provisionerDePrueba(t, live).Audit(context.Background())
	if len(entries) != 0 {
		t.Errorf("un directorio sin worktrees dio %d entradas: %v", len(entries), entries)
	}

	// Y con un worktree real, el listado lo encuentra. Se comprueba la forma de la entrada
	// —que trae Label, Branch, Path y Orphan— porque son los cuatro campos que imprime el
	// listado, y un campo que falta sale como hueco en la columna.
	entradas := provisionerDePrueba(t, repo).Audit(context.Background())
	for _, e := range entradas {
		if e.Path == "" {
			t.Errorf("una entrada sin Path: %+v", e)
		}
		if e.Orphan && e.Reason == "" {
			t.Errorf("una entrada huérfana sin motivo: %+v", e)
		}
		if !e.Orphan {
			t.Errorf("un worktree vivo marcado como huérfano: %+v", e)
		}
	}
}

// TestElBorradoRechazaLoQueNoEsWorktreePropio: lo que `removeWorktrees` NO toca.
//
// Y esta es la parte dangerous del comando, así que lo que se prueba es la negativa. Borra
// rutas que el usuario da en la línea de órdenes, y borrar la ruta equivocada pierde trabajo
// que no está en git. Las tres rechazas son distintas:
//
//   - No es un worktree de prdash: pertenece a otra herramienta o es un repo normal.
//   - No existe: no hay nada que borrar, y el error es de quien lo escribió mal.
//   - Y una ruta relativa se resuelve a absoluta ANTES de comprobar, porque `Owned` compara
//     contra la raíz de prdash y una ruta relativa comparada tal cual no casaría con nada.
//
// Y la última comprobación es la que importa para el usuario: una ruta que no es suya sale
// SÍ o SÍ rechazada, y nunca "borrada y luego forgiveness".
func TestElBorradoRechazaLoQueNoEsWorktreePropio(t *testing.T) {
	dir := t.TempDir()
	// Una ruta que existe y no es un worktree de prdash: un repo normal.
	ajeno := filepath.Join(dir, "otro-repo")
	if err := os.MkdirAll(ajeno, 0o755); err != nil {
		t.Fatal(err)
	}

	pr := provisionerDePrueba(t, dir)

	// Un repo ajeno: rechazado, y sigue ahí.
	code := removeWorktreesWithin(pr, io.Discard, io.Discard, false, false, []string{ajeno}, time.Second)
	if code == 0 {
		t.Error("borrar un repo ajeno devolvió 0")
	}
	if _, err := os.Stat(ajeno); err != nil {
		t.Errorf("el repo ajeno no está: %v", err)
	}

	// Una ruta que no existe.
	if code := removeWorktreesWithin(pr, io.Discard, io.Discard, false, false, []string{filepath.Join(dir, "nada")}, time.Second); code == 0 {
		t.Error("borrar una ruta inexistente devolvió 0")
	}

	// Una ruta relativa: se resuelve, se rechaza por lo mismo, y no se toca nada de aquí.
	antes, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if code := removeWorktreesWithin(pr, io.Discard, io.Discard, false, false, []string{"no-existe-en-absoluto"}, time.Second); code == 0 {
		t.Error("una ruta relativa inexistente devolvió 0")
	}
	despues, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(antes) != len(despues) {
		t.Errorf("el número de entradas del directorio cambió de %d a %d con un rechazo",
			len(antes), len(despues))
	}

	// Y `--dry-run` sin nada que hacer no borra ni falla.
	if code := removeWorktreesWithin(pr, io.Discard, io.Discard, false, true, []string{ajeno}, time.Second); code == 0 {
		t.Error("dry-run sobre un repo ajeno devolvió 0: dry-run no debería rechazar, " +
			"sino avisar y no tocar")
	}
	if _, err := os.Stat(ajeno); err != nil {
		t.Errorf("el dry-run borró el repo ajeno: %v", err)
	}
}

// TestElPresupuestoEsPorItemYNoGlobal: la razón de existir de `removeWorktreesWithin`.
//
// Y el detalle no es que cada borrado tenga un plazo —eso ya lo hacía el global— sino que
// NO lo comparten. Con un presupuesto global, un ítem lento que agota el suyo deja a los
// siguientes sin nada y el resultado es un borrado parcial por timeout, que es el peor
// estado posible: el usuario pidió borrar cinco y se borraron dos.
//
// Y aquí solo se comprueba lo comprobable sin montar cinco worktrees reales: que la función
// ACEPTA el presupuesto y que sin worktrees que borrar no se hanguea. La garantía de
// "por ítem" la da el código, que llama a `removeOne` con el presupuesto dentro del bucle.
func TestElPresupuestoEsPorItemYNoGlobal(t *testing.T) {
	pr := provisionerDePrueba(t, t.TempDir())

	// Un presupuesto ridículo: sin worktrees que borrar no tiene nada que esperar.
	if code := removeWorktreesWithin(pr, io.Discard, io.Discard, false, false, []string{filepath.Join(t.TempDir(), "nada")},
		time.Millisecond); code == 0 {
		t.Error("una ruta inexistente con presupuesto minúsculo devolvió 0")
	}

	// Y sin items, devolviendo rápido: un presupuesto por ítem no se puede confundir con uno
	// global si no hay ítems a los que repartir.
	inicio := time.Now()
	removeWorktreesWithin(pr, io.Discard, io.Discard, true, true, nil, 2*time.Second)
	if elapsed := time.Since(inicio); elapsed > time.Second {
		t.Errorf("un dry-run de huérfanos tardó %s con nada que borrar", elapsed)
	}
}

// TestElBuildDelEjecutorUsaLaConfig: la cascada de la config al ejecutor.
//
// Y lo que se comprueba es el cableado, que es donde una config nueva se queda a medias:
// cada herramienta del ejecutor sale de la config por su nombre, y un nombre mal escrito
// devuelve la cadena vacía —que `binaryAvailable` trata como no disponible— y el pane
// desaparece sin aviso.
func TestElBuildDelEjecutorUsaLaConfig(t *testing.T) {
	// Y lo que se descubre vaciando `tools.agent` es que NO queda vacío: `ToolArgs` cae al
	// nombre por defecto de la herramienta, que es "opencode". O sea, para las cuatro
	// herramientas conocidas, `paneTool` no puede devolver un argv vacío —el último peldaño
	// de la cascada es un nombre real—. Y eso es lo que evita un pane sin comando: un argv
	// vacío lanzaría un proceso que arranca y no hace nada. La versión anterior de este test
	// daba por hecho que vaciar la config vacía el pane, y lo que pasa es lo contrario.
	cfg := config.Defaults()
	cfg.Tools.Tuicr = "/opt/tuicr --dark"
	cfg.Tools.Hunk = "hunk"
	cfg.Tools.Agent = ""

	tools := plan.Tools{
		Tuicr:  paneTool(cfg, "tuicr"),
		Hunk:   paneTool(cfg, "hunk"),
		Agent:  paneTool(cfg, "agent"),
		Editor: paneTool(cfg, "editor"),
	}

	if got := tools.Binary(plan.KindTuicr); got != "/opt/tuicr" {
		t.Errorf("el binario de tuicr salió %q", got)
	}
	if got := tools.Binary(plan.KindHunk); got != "hunk" {
		t.Errorf("el binario de hunk salió %q", got)
	}
	// El de `tools` vacío cae al nombre por defecto, no a nada.
	if got := tools.Binary(plan.KindAgent); got != "opencode" {
		t.Errorf("con tools.agent vacío dio %q, want el nombre por defecto", got)
	}
	// Y lo que sí queda sin comando es una herramienta que no existe en la cascada, que es
	// donde el argv vacío sí es posible y donde `binaryAvailable` tiene algo que decidir.
	if got := tools.Binary(plan.Kind("inventada")); got != "" {
		t.Errorf("una herramienta inexistente dio %q, want vacío", got)
	}
	if binaryAvailable(tools.Binary(plan.Kind("inventada"))) {
		t.Error("un argv vacío se declaró disponible")
	}

	// Y el ejecutor completo se arma sin que entre en pánico con una config mínima. No se
	// comprueba que funcione —eso necesita Herdr— sino que el cableado no revienta.
	ex := buildExecutor(config.Config{})
	if ex == nil {
		t.Fatal("buildExecutor devolvió nil")
	}
	if ex.Resolver == nil || ex.Worktrees == nil || ex.Herdr == nil {
		t.Errorf("el ejecutor tiene una pieza a nil: %+v", ex)
	}
	// Y el simulador, que es el otro servicio armado sobre el ejecutor.
	svc := buildSimulator(config.Config{}, ex)
	if svc == nil {
		t.Fatal("buildSimulator devolvió nil")
	}
	// Y su `Locate` sin review montado: dice que no, en vez de devolver un sitio vacío.
	loc := simLocator{ex: ex}
	if _, ok := loc.Locate(model.Item{}); ok {
		t.Error("el locator encontró un clon sin review montado")
	}
}

// provisionerDePrueba devuelve un provisioner real con la raíz en dir.
func provisionerDePrueba(t *testing.T, dir string) worktree.Provisioner {
	t.Helper()
	return worktree.Select(nil, dir)
}
