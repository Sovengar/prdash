package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `Load` nunca falla, y esa es la regla del proyecto: un config ausente o malformado
// degrada a defaults con un aviso, nunca aborta. Una TUI que no arranca porque el fichero
// de config tiene una coma de menos es peor que una TUI que arranca con los atajos por
// defecto.
//
// Y "nunca falla" tiene tres mitades distintas que se pueden romper por separado:
//
//	- Sin fichero: defaults SILENCIOSOS. El primer arranque no debe diagnosticar nada.
//	- Fichero ilegible (permisos): defaults CON aviso. Algo está mal y hay que decirlo.
//	- Fichero ilegible por otra causa: defaults CON aviso.
//
// La del medio es la que más se confunde con la primera, y por eso este fichero la fija
// entera: un fichero que existe y no se puede leer no es lo mismo que un fichero que no
// existe, y tratarlos igual hace que un problema de permisos pase desapercibido.

// TestLoadSinFicheroNoDiceNada: el primer arranque es el caso más frecuente de todos.
func TestLoadSinFicheroNoDiceNada(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "no-existe.toml")

	cfg, warn := LoadFrom(path)
	if warn != "" {
		t.Errorf("sin fichero dio aviso %q: el primer arranque tiene que ser silencioso", warn)
	}
	// Y los defaults tienen que ser USABLES: la config que sale de aquí es la que
	// arranca la TUI, así que si le faltan los atajos no hay teclado.
	if len(cfg.Keybindings) == 0 {
		t.Error("sin fichero devolvio una config sin keybindings en vez de los defaults")
	}
}

// TestUnFicheroQueExisteYNoSeLeeAvisa: el caso que se confunde con el anterior.
//
// Y se confunde porque el resultado es el mismo —los defaults— y la tentación es
// devolverlos igual. Pero el aviso es lo único que distingue "no has configurado nada" de
// "no puedo leer tu configuración", y sin él un problema de permisos —un contenedor, un
// fichero con el owner equivocado— es invisible.
func TestUnFicheroQueExisteYNoSeLeeAvisa(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("como root los permisos de lectura no impiden leer")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("x = 1\n"), 0o000); err != nil {
		t.Fatal(err)
	}

	cfg, warn := LoadFrom(path)
	if warn == "" {
		t.Fatal("un fichero que existe y no se puede leer dio aviso vacio: el problema " +
			"pasa desapercibido")
	}
	if !strings.Contains(warn, "config") {
		t.Errorf("el aviso %q no dice de donde viene", warn)
	}
	// Y aun así devuelve defaults, no una config vacía: se degrada, no se rompe.
	if len(cfg.Keybindings) == 0 {
		t.Error("un config ilegible devolvio una config sin keybindings en vez de los defaults")
	}
}

// TestLoadPorLaRutaDeXDG: `Load` es `LoadFrom(Path())`, y `Path` depende de
// `$XDG_CONFIG_HOME`.
//
// Y el caso de no tener ninguna variable puesta es el de un entorno mínimo —un contenedor,
// un `docker run` sin `-e`—, donde `$HOME` tampoco está. Ahí `os.UserConfigDir` falla, y la
// consecuencia de que `Load` lo ignore en vez de abortar es que la TUI arranca con los
// atajos por defecto en vez de no arrancar.
func TestLoadPorLaRutaDeXDG(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	path, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	// Y la ruta es la que corresponde: subdirectorio de prdash dentro de XDG. No basta
	// con que sea "algún sitio bajo XDG", porque un `prdash` en la raíz se mezcla con lo
	// que otro programa haya puesto ahí.
	if filepath.Dir(path) != filepath.Join(dir, DirName) {
		t.Errorf("Path dio %q, want dentro de %q", path, filepath.Join(dir, DirName))
	}
	if filepath.Base(path) != FileName {
		t.Errorf("Path dio el fichero %q, want %q", filepath.Base(path), FileName)
	}

	// Con un config escrito ahí, `Load` lo encuentra sin que nadie le pase la ruta.
	if err := os.MkdirAll(filepath.Join(dir, DirName), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("[keybindings]\nquit = \"Q\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, warn := Load()
	if warn != "" {
		t.Errorf("Load aviso %q con un config valido", warn)
	}
	if got := cfg.KeyFor("quit"); got != "Q" {
		t.Errorf("Load no leyo el override de quit: %q", got)
	}

	// Y sin XDG ni HOME: `Path` falla, y `Load` degrada a defaults en vez de abortar.
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "")
	cfg, _ = Load()
	if len(cfg.Keybindings) == 0 {
		t.Error("sin HOME ni XDG_CONFIG_HOME, Load devolvio una config sin keybindings")
	}
}

// TestKeyForCaeAlDefaultYNoAlVacio: una acción sin tecla puesta usa la de por defecto.
//
// Y el detalle que importa es que el fallback es un default REAL, no la cadena vacía. Un
// "" como tecla asignada no hace nada en la TUI, y sin este fallback cualquier acción
// nueva que se añada al mapa de acciones se quedaría sin tecla sin que nadie se entere.
func TestKeyForCaeAlDefaultYNoAlVacio(t *testing.T) {
	cfg := Defaults()

	// Las de por defecto existen y no están vacías.
	for action := range DefaultKeybindings() {
		if got := cfg.KeyFor(action); got == "" {
			t.Errorf("la accion %q tiene tecla vacia con la config por defecto", action)
		}
	}
	// Con override, manda el override.
	cfg.Keybindings["quit"] = "Z"
	if got := cfg.KeyFor("quit"); got != "Z" {
		t.Errorf("con override, KeyFor dio %q, want Z", got)
	}
	// Y una acción que no está en el mapa devuelve vacío, porque no hay default que
	// devolver: inventar una tecla sería peor que no tenerla.
	if got := cfg.KeyFor("accion-inventada"); got != "" {
		t.Errorf("una accion desconocida dio tecla %q, want vacio", got)
	}
}

// TestActionForKeyInvierteElMapaYEsEstable: la dirección opuesta, con la ambigüedad
// resuelta de forma determinista.
//
// Y el caso que importa es el de dos acciones con la MISMA tecla. La TUI consulta
// `ActionForKey` con la tecla que ha leído, así que si dos acciones comparten tecla la
// consulta tiene que devolver una sola —si no, la que llegue antes sería la que se
// ejecutase—, y el código lo resuelve ordenando las acciones. Lo que se fija aquí es que
// devuelve SIEMPRE la misma, porque una resolución que depende del orden de recorrido de
// un mapa hace que la misma tecla ejecute acciones distintas en dos sesiones.
func TestActionForKeyInvierteElMapaYEsEstable(t *testing.T) {
	cfg := Defaults()

	// La tecla de una acción devuelve esa acción.
	if got := cfg.ActionForKey(cfg.KeyFor("quit")); got != "quit" {
		t.Errorf("ActionForKey dio %q, want quit", got)
	}
	// Tecla vacía: sin acción. Y es el caso de una tecla que no está asignada: una
	// cadena vacía se traduce a un nombre de acción vacío que no dispara nada.
	if got := cfg.ActionForKey(""); got != "" {
		t.Errorf("la tecla vacia dio accion %q, want vacio", got)
	}
	// Tecla no asignada.
	if got := cfg.ActionForKey("Ctrl+Alt+Imposible"); got != "" {
		t.Errorf("una tecla sin asignar dio accion %q, want vacio", got)
	}

	// Y la ambigüedad es estable: se ejecuta cien veces y sale lo mismo.
	cfg.Keybindings["a"] = "X"
	cfg.Keybindings["b"] = "X"
	primera := cfg.ActionForKey("X")
	for i := 0; i < 100; i++ {
		if got := cfg.ActionForKey("X"); got != primera {
			t.Fatalf("ActionForKey dio %q en la llamada %d y %q en la primera", got, i, primera)
		}
	}
	// Y esa respuesta tiene que ser una de las dos que comparten la tecla, no una
	// tercera cosa.
	if primera != "a" && primera != "b" {
		t.Errorf("ActionForKey dio %q, que no es ninguna de las dos acciones", primera)
	}
}

// TestCmdArgsParteElComandoPorEspacios: el argv base sale de `strings.Fields`, no de un
// `Split(" ")`.
//
// Y la diferencia es un caso que se da en un fichero de config escrito a mano: un comando
// con espacios dobles o con espacios alrededor. Con `Split(" ")` salen argumentos vacíos,
// que git y hunk interpretan como el directorio actual —que es el repo equivocado— en vez
// de como un error.
func TestCmdArgsParteElComandoPorEspacios(t *testing.T) {
	cfg := Defaults()

	// Con espacios raros: los campos salen igual de bien formados.
	cfg.Commands["agente"] = "  tuicr   review  "
	got := cfg.CmdArgs("agente")
	want := []string{"tuicr", "review"}
	if len(got) != len(want) {
		t.Fatalf("con espacios raros dio %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("argv[%d] = %q, want %q (completo %q)", i, got[i], want[i], got)
		}
	}
	// Ningún argumento vacío, que es lo que el `Split(" ")` habría producido.
	for _, a := range got {
		if a == "" {
			t.Errorf("salió un argumento vacío en %q", got)
		}
	}

	// Sin override ni default conocido: argv vacío. Y se fija que sea vacío, porque un
	// argv vacío aquí es la señal de "no hay comando para esta acción", y quien lo reciba
	// omite el pane. Un fallback con el nombre de la acción —como el que tiene
	// `ToolArgs`— sería inventar un binario.
	if got := cfg.CmdArgs("accion-que-no-existe"); len(got) != 0 {
		t.Errorf("una accion sin comando dio %q, want argv vacio", got)
	}
	// Y un comando que son solo espacios.
	cfg.Commands["vacio"] = "   "
	if got := cfg.CmdArgs("vacio"); len(got) != 0 {
		t.Errorf("un comando de solo espacios dio %q", got)
	}
}

// TestElOverrideDelPaneNoEsUnValorVacio: `[commands]` con un valor en blanco NO es un
// override.
//
// Y esa es toda la función: la diferencia entre "el usuario no ha configurado este pane" y
// "el usuario ha configurado este pane con nada". Lo segundo casi siempre es un fichero
// editado a mano que se quedó a medias, y tomarlo como override lanzaría un pane con un
// argv vacío, que es un proceso que arranca y no hace nada.
func TestElOverrideDelPaneNoEsUnValorVacio(t *testing.T) {
	cfg := Defaults()

	// Sin nada: no hay override.
	if _, ok := cfg.PaneOverride("hunk"); ok {
		t.Error("sin configurar dio override")
	}
	// Con un override real: sale partido y confirmado.
	cfg.Commands["hunk"] = "hunk --model o3"
	argv, ok := cfg.PaneOverride("hunk")
	if !ok {
		t.Fatal("un comando configurado dio override=false")
	}
	if len(argv) != 3 || argv[0] != "hunk" || argv[2] != "o3" {
		t.Errorf("el override dio %q", argv)
	}
	// Con valores que NO son override: en blanco y solo espacios. Y se comprueban los
	// dos porque un fichero escrito a mano produce cualquiera de los dos.
	for _, vacio := range []string{"", "   ", "\t\n "} {
		cfg.Commands["hunk"] = vacio
		if _, ok := cfg.PaneOverride("hunk"); ok {
			t.Errorf("un comando de %q dio override=true", vacio)
		}
	}
}

// TestToolArgsPriorizaElOverrideYLuegoElBinario: el orden de la cascada.
//
// Y el orden está en el comentario de la función y es una decisión: `commands.<name>` gana
// sobre `tools.<name>` porque el primero es el argv COMPLETO y el segundo es un binario con
// sus flags de serie. Al revés, un `hunk --model o3` en `[commands]` se quedaría sin el
// flag.
//
// Y el último peldaño es el nombre del propio binario, que es lo que hace que funcione out
// of the box sin configurar nada. Los tres peldaños se fijan porque el orden es invisible
// en el resultado.
func TestToolArgsPriorizaElOverrideYLuegoElBinario(t *testing.T) {
	cfg := Defaults()

	// Sin nada: el default de cada herramienta. Y aquí los defaults NO son el nombre de la
	// herramienta: `agent` sale como `opencode` y `editor` como `vi`. La primera versión de
	// este aserto suponía que el último peldaño de la cascada era el nombre propio, y es
	// un nombre real —el que ejecuta el editor del sistema—, lo que hace que la cascada
	// sirva para algo sin configurar nada.
	porDefecto := map[string]string{
		"tuicr": "tuicr", "hunk": "hunk", "agent": "opencode", "editor": "vi",
	}
	for name, want := range porDefecto {
		got := cfg.ToolArgs(name)
		if len(got) == 0 {
			t.Errorf("%q sin configurar dio argv vacio", name)
			continue
		}
		if got[0] != want {
			t.Errorf("%q sin configurar dio %q, want que empiece por %q", name, got, want)
		}
	}

	// Con `tools.<name>`: el binario con sus flags, y el nombre ya no es el peldaño final.
	cfg.Tools.Tuicr = "/opt/tuicr --dark"
	got := cfg.ToolArgs("tuicr")
	if got[0] != "/opt/tuicr" || len(got) != 2 {
		t.Errorf("con tools.tuicr dio %q", got)
	}

	// Con `commands.<name>` encima: el argv completo gana, flags incluidos.
	// Y `--model o3` son DOS campos, no uno con un espacio dentro. Que es justo lo que
	// un argv necesita: un argumento con un espacio dentro llegaría al proceso como uno
	// solo y el flag no se entendería.
	cfg.Commands["tuicr"] = "tuicr review --model o3"
	got = cfg.ToolArgs("tuicr")
	want := []string{"tuicr", "review", "--model", "o3"}
	if len(got) != len(want) {
		t.Fatalf("con commands.tuicr dio %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("argv[%d] = %q, want %q (completo %q)", i, got[i], want[i], got)
		}
	}

	// Y un `commands` en blanco NO tapa el `tools`: el override vacío no es override.
	// Es la misma regla del pane, y si no se respetara aquí, poner la clave en el config
	// para desactivarla sin querer lo desactivaría de verdad.
	cfg.Commands["tuicr"] = "  "
	got = cfg.ToolArgs("tuicr")
	if got[0] != "/opt/tuicr" {
		t.Errorf("un commands.tuicr en blanco dio %q, y deberia seguir mandando tools", got)
	}
}

// TestFieldsOrUsaElFallbackSoloSiEstaVacio: el peldaño final de la cascada.
//
// Y el caso que separa las dos ramas de un `== ""` ingenuo es el de los espacios: un
// `tools.hunk = "   "` en el config es un valor presente y vacío a la vez. Con la
// comparación ingenua, el fallback se carga `"   "` y el argv sale vacío —un pane sin
// comando—, que es justo el fallo que el fallback existe para evitar.
func TestFieldsOrUsaElFallbackSoloSiEstaVacio(t *testing.T) {
	casos := []struct {
		raw      string
		fallback string
		want     string
	}{
		{"", "hunk", "hunk"},
		{"   ", "hunk", "hunk"},
		{"\t\n", "hunk", "hunk"},
		{"/opt/hunk", "hunk", "/opt/hunk"},
		{"/opt/hunk --dark", "hunk", "/opt/hunk"},
		{"con  espacios   dentro", "hunk", "con"},
	}
	for _, c := range casos {
		got := fieldsOr(c.raw, c.fallback)
		if len(got) == 0 {
			t.Errorf("fieldsOr(%q, %q) dio argv vacio", c.raw, c.fallback)
			continue
		}
		if got[0] != c.want {
			t.Errorf("fieldsOr(%q, %q) dio %q, want que empiece por %q", c.raw, c.fallback, got, c.want)
		}
	}
}
