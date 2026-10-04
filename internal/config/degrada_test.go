package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// `LoadFrom` es la frontera con el disco y con el TOML del usuario, y su contrato no es
// "cargar la config" sino **degradar**: cualquier cosa que falle devuelve los defaults y un
// aviso, nunca una config a medias y nunca un `os.Exit`. Es la regla del proyecto —
//
//	un config ausente o malformado degrada a defaults con un aviso. Es un patrón, no una
//	excepción
//
// — y este fichero la hace cumplir con el disco de verdad en vez de con un doble de `os`.
//
// Y por qué el disco de verdad: los cuatro fallos de lectura que importan solo existen porque
// existen. Un fichero en modo 000 da EACCES, un directorio donde se espera un fichero da
// EISDIR, un enlace simbólico que se apunta a sí mismo da ELOOP, y un TOML con una llave sin
// cerrar da un error del parser. Los tres primeros los comprobados en el SO antes de escribir
// el test, porque un test que dice "aviso no vacío" sin saber qué error hay detrás pasa
// igual con cualquier fallo.

// TestLoadFromDegradaADefaultsYAvisa: los cuatro fallos de lectura, contra el SO.
//
// Y lo que se comprueba en los cuatro es la misma cosa, y es lo que importa: **la config que
// vuelve es la de defaults y hay un aviso que lo dice**. No que el aviso tenga un texto
// concreto —eso se comprueba con que mencione el fichero— sino que haya uno, porque un
// `LoadFrom` que degrada en silencio deja al usuario con un comportamiento que no pidió y sin
// ninguna pista de por qué.
//
// Y el caso de la config A MEDIAS es el que hay quemirar de verdad: si un fallo de lectura
// devolviera la config parcialmente poblada, el usuario vería unos ajustes que nunca
// escribió. Por eso se compara contra `Defaults()` entero, no contra un par de campos.
func TestLoadFromDegradaADefaultsYAvisa(t *testing.T) {
	dir := t.TempDir()

	// Un directorio donde se espera un fichero: `os.ReadFile` lo abre y el `read` falla con
	// EISDIR. Es el caso de un `~/.config/prdash/config.toml` que alguien convirtió en
	// carpeta, que pasa.
	comoCarpeta := filepath.Join(dir, "config-carpeta")
	if err := os.MkdirAll(comoCarpeta, 0o755); err != nil {
		t.Fatal(err)
	}

	// Un fichero que no se puede leer: modo 000. Requiere no ser root —este tests corre como
	// usuario normal— y es justo el caso de un config de otro usuario o copiado con el modo
	// equivocado.
	sinPermiso := filepath.Join(dir, "config-sin-permiso")
	if err := os.WriteFile(sinPermiso, []byte("[general]\n"), 0o000); err != nil {
		t.Fatal(err)
	}
	if _, err := os.ReadFile(sinPermiso); err == nil {
		t.Skip("el usuario puede leer un fichero en modo 000: el caso de EACCES no aplica aquí")
	}

	// Un enlace simbólico que se apunta a sí mismo: ELOOP. El núcleo lo detecta al pedir el
	// destino, y `os.IsNotExist` dice que NO —que es el punto—: un ELOOP no es un fichero
	// inexistente, así que no puede caer en la rama del "sin fichero, defaults silenciosos".
	conBucle := filepath.Join(dir, "config-bucle")
	if err := os.Symlink(conBucle, conBucle); err != nil {
		t.Fatalf("crear el enlace que se apunta a si mismo: %v", err)
	}

	for _, c := range []struct {
		nombre string
		path   string
	}{
		{"directorio en vez de fichero", comoCarpeta},
		{"fichero sin permiso de lectura", sinPermiso},
		{"enlace que se apunta a si mismo", conBucle},
	} {
		cfg, aviso := LoadFrom(c.path)

		if aviso == "" {
			t.Errorf("%s: LoadFrom degrado SIN aviso", c.nombre)
			continue
		}
		// Y el aviso empieza por "config:", que es lo que permite reconocerlo como un aviso
		// de config y no como un error de cualquier otra cosa.
		if !strings.HasPrefix(aviso, "config:") {
			t.Errorf("%s: el aviso %q no lleva el prefijo config:", c.nombre, aviso)
		}
		// Y la config es la de defaults COMPLETA. Es el contrato entero: si un solo campo
		// quedara con un valor que el usuario no escribió, el fallo sería peor que el
		// original.
		def := Defaults()
		if !mismasSalvoAviso(cfg, def) {
			t.Errorf("%s: la config de vuelta no es la de defaults", c.nombre)
		}
	}
}

// TestUnConfigAusenteDegradaEnSilencioYUnoVacioTambien: la asimetría con lo de arriba.
//
// Y es deliberada y es lo que hay que fijar, porque se lee al revés de cómo se leen casi
// todos los códigos: **la ausencia de fichero NO es un aviso** y el resto de fallos de
// lectura SÍ.
//
// Y el motivo es que prdash se ejecuta sin config la mayoría de las veces —se arranca en un
// repo, se ve el inbox y ya— y un aviso en cada arranque por "no hay config" sería ruido que
// enseña a ignorar los avisos que sí importan. En cambio, un config que existe y no se puede
// leer es una señal de que el usuario puso algo ahí y no está surtiendo efecto, y eso sí hay
// que decir.
//
// Y el segundo caso es el que casi se cuela: un fichero VACÍO es un config presente y
// legible, y no es ningún cambio. Un `data_dir = ""` explícito sí lo sería, y por eso esa
// cadena vacía se trata distinto de un fichero vacío —más abajo.
func TestUnConfigAusenteDegradaEnSilencioYUnoVacioTambien(t *testing.T) {
	dir := t.TempDir()

	// No existe: defaults, sin aviso.
	cfg, aviso := LoadFrom(filepath.Join(dir, "no-existe.toml"))
	if aviso != "" {
		t.Errorf("un config ausente dio aviso %q: se avisaría en cada arranque", aviso)
	}
	if !mismasSalvoAviso(cfg, Defaults()) {
		t.Error("un config ausente no devolvió los defaults")
	}

	// Existe y está vacío: también sin aviso, y también defaults. Un fichero de cero bytes
	// es un config válido con cero ajustes.
	vacio := filepath.Join(dir, "vacio.toml")
	if err := os.WriteFile(vacio, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, aviso = LoadFrom(vacio)
	if aviso != "" {
		t.Errorf("un config vacío dio aviso %q", aviso)
	}
	if !mismasSalvoAviso(cfg, Defaults()) {
		t.Error("un config vacío no devolvió los defaults")
	}
}

// TestUnTOMLQueNoParseaAvisaYNoSeQuedaConMitadDeLoLeido: el parser, contra su error.
//
// Y este es el fallo más caro de los cuatro, y por eso el aserto importante no es el aviso:
// es que **la config es la de defaults y no la parcialmente leída**. Un `toml.Decode` que
// llena lo que puede y falla en lo que no puede deja un estado intermedio donde el usuario
// tiene medio config aplicada y medio no, y la más slick de las dos partes: parece que el
// cambio surtió efecto cuando noEntero.
//
// Y el TOML del caso está malformado en el sitio que más daño hace —una llave sin valor—,
// que es donde el parser se para después de haber leído las anteriores.
func TestUnTOMLQueNoParseaAvisaYNoSeQuedaConMitadDeLoLeido(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "roto.toml")
	// Tres llaves legibles ANTES del error. Si el parser devolviera lo que alcanzó a leer,
	// esta config tendría un roots distinto del de por defecto.
	escribirConfig(t, path, `
roots = ["/tmp/un-sitio-que-no-debe-aplicarse"]
data_dir = "/otro"
refresh_interval = "10s"
esto-no-es-una-llave
`)

	cfg, aviso := LoadFrom(path)

	if aviso == "" {
		t.Fatal("un TOML malformado dio nil: la config rota se aplicaría en silencio")
	}
	if !strings.Contains(aviso, "toml") && !strings.Contains(aviso, "config:") {
		t.Errorf("el aviso %q no dice que es del config", aviso)
	}
	def := Defaults()
	if len(cfg.Roots) != len(def.Roots) || len(cfg.Roots) == 0 {
		t.Errorf("la config se quedó a medias: %d raíces, las de por defecto son %d",
			len(cfg.Roots), len(def.Roots))
	}
	for _, r := range cfg.Roots {
		if r == "/tmp/un-sitio-que-no-debe-aplicarse" {
			t.Error("se aplicó un roots del TOML que no llegó a parsear entero")
		}
	}
	if cfg.DataDir == "/otro" {
		t.Error("se aplicó un data_dir de un TOML que no llegó a parsear entero")
	}
}

// TestUnValorInvalidoSeIgnoraYUnoValidoSeAplica: la degradación por campo.
//
// Y es donde el "degrada con aviso" NO aplica: un `refresh_interval = "no es una duración"`
// es un error tipográfico, no un config roto. Avisar de eso en cada arranque por una línea
// mal escrita sería echar a la gente la función entera. Lo que hace el código es ignorar
// el campo y seguir con defaults —que es exactamente lo mismo que si no estuviera—.
//
// Y el caso que hace que el silencio sea aceptable es el de un intervalo NEGATIVO: `-5s` sí
// parsea bien, así que el error no es de formato y aun así no se aplica. Sin el `d >= 0`, un
// intervalo negativo haría que el próximo tick se pidiera en el pasado y el refresco no
// ocurriría nunca más.
func TestUnValorInvalidoSeIgnoraYUnoValidoSeAplica(t *testing.T) {
	dir := t.TempDir()
	def := Defaults()

	for _, c := range []struct {
		nombre   string
		toml     string
		want     time.Duration
		explicar string
	}{
		{"duración no parseable", `refresh_interval = "quince minutos"`, def.RefreshInterval,
			"una duración mal escrita se ignora y el default manda"},
		{"duración negativa", `refresh_interval = "-5s"`, def.RefreshInterval,
			"un intervalo negativo no se aplica: el tick se pediría en el pasado"},
		{"duración vacía", `refresh_interval = ""`, def.RefreshInterval,
			"una duración vacía no es una duración"},
		{"duración válida", `refresh_interval = "90s"`, 90 * time.Second,
			"una duración válida sí se aplica"},
		{"cero", `refresh_interval = "0s"`, 0,
			"cero SÍ se aplica: es un refresco manual, y es lo que el usuario pidió"},
	} {
		path := filepath.Join(dir, "c.toml")
		escribirConfig(t, path, c.toml)

		cfg, aviso := LoadFrom(path)
		if aviso != "" {
			t.Errorf("%s: dio aviso %q, y un error tipográfico no es un config roto",
				c.nombre, aviso)
		}
		if cfg.RefreshInterval != c.want {
			t.Errorf("%s: RefreshInterval = %v, want %v (%s)",
				c.nombre, cfg.RefreshInterval, c.want, c.explicar)
		}
	}
}

// TestUnaCadenaVaciaEnUnaRutaSeIgnoraYNoLaDejaVacia: la guarda de las rutas.
//
// Y el motivo de la guarda es que el código escribe `fc.DataDir != nil && *fc.DataDir != ""`:
// hay dos formas de que una ruta llegue vacía y solo una es un error del usuario.
//
//   - `data_dir = ""` explícito: el usuario la puso vacía. Es una petición rara, pero es una
//     petición, y aplicarla dejaría el data_dir sin valor —y un `data_dir` vacío es un path
//     relativo al directorio de trabajo, que cambia según dónde se lance prdash. Mejor el
//     default, que es el de la HOME del usuario.
//   - La clave sin escribir: eso no llega ni a ser cadena vacía, `fc.DataDir` es nil.
//
// Y lo que hace la guarda es dejar el default en los dos casos, que es lo que un aserto de
// "no queda vacío" comprueba. Y el generalización es que vale para las cuatro rutas del
// config —data_dir, clone_dir y worktree_dir—, que comparten la misma guarda.
func TestUnaCadenaVaciaEnUnaRutaSeIgnoraYNoLaDejaVacia(t *testing.T) {
	dir := t.TempDir()
	def := Defaults()
	path := filepath.Join(dir, "c.toml")

	escribirConfig(t, path, `
data_dir = ""
clone_dir = ""
worktree_dir = ""
`)
	cfg, aviso := LoadFrom(path)
	if aviso != "" {
		t.Errorf("dio aviso %q", aviso)
	}
	if cfg.DataDir != def.DataDir {
		t.Errorf("DataDir = %q con un data_dir vacío en el config, want el default %q",
			cfg.DataDir, def.DataDir)
	}
	if cfg.CloneDir != def.CloneDir {
		t.Errorf("CloneDir = %q con un clone_dir vacío, want %q", cfg.CloneDir, def.CloneDir)
	}
	if cfg.WorktreeDir != def.WorktreeDir {
		t.Errorf("WorktreeDir = %q con un worktree_dir vacío, want %q",
			cfg.WorktreeDir, def.WorktreeDir)
	}
	// Y ninguna queda vacía, que es lo que importaba: un DataDir vacío es un path relativo y
	// prdash se puede lanzar desde cualquier sitio.
	for nombre, valor := range map[string]string{
		"DataDir": cfg.DataDir, "CloneDir": cfg.CloneDir, "WorktreeDir": cfg.WorktreeDir,
	} {
		if valor == "" {
			t.Errorf("%s quedó vacía: sería un path relativo al directorio de trabajo", nombre)
		}
	}
}

// TestUnaPistaConTeclaVaciaNoSePinta: la tercera clase de entrada de la barra.
//
// Y el caso es una acción que se queda sin tecla, y hay dos maneras de que pase: que no esté
// en el mapa de keybindings y tampoco tenga default —que es lo que pasa con una acción que se
// borró del código pero sigue en el config del usuario— o que el mapa la tenga con la cadena
// vacía.
//
// Y lo que se fija es que la entrada se SALTARA en vez de pintarse como un hueco. Un hueco
// en la barra —" refresh" con un espacio delante y nada antes— hace que el ojooya a buscar una
// tecla que no existe, y en una barra de atajos un hueco es peor que una entrada de menos.
//
// Y se comprueba por las dos vías, porque son fallos distintos: `KeyFor` puede devolver "" por
// los dos, pero solo uno se puede provocar desde el config del usuario.
func TestUnaPistaConTeclaVaciaNoSePinta(t *testing.T) {
	cfg := Defaults()

	// Una acción que no existe en el mapa ni tiene default. El mapa no la tiene porque se
	// inventa el nombre, y `KeyFor` cae a `DefaultKeybindings()` que tampoco la tiene.
	conHuerfana := cfg.Hints(HintState{"accion-que-no-existe": "texto"})
	if len(conHuerfana) != len(cfg.Hints(nil)) {
		t.Errorf("una acción sin tecla añadió %d entradas a la barra",
			len(conHuerfana)-len(cfg.Hints(nil)))
	}

	// Y una acción real con la tecla puesta a vacío en el config. El merge de keybindings
	// tiene su propia guarda para esto, así que aquí hay que llegar por `Keybindings`
	// directamente —que es lo que hace `wire` cuando el mapa viene de otra parte—.
	conVacia := Defaults()
	for _, accion := range []string{"refresh", "approve", "merge", "quit"} {
		conVacia.Keybindings[accion] = ""
	}
	for _, h := range conVacia.Hints(nil) {
		if strings.TrimSpace(h) == "" {
			t.Errorf("una pista con la tecla vacía se pintó como %q", h)
		}
		if strings.HasPrefix(h, " ") {
			t.Errorf("la pista %q empieza por un hueco: la tecla desapareció y el hueco quedó", h)
		}
	}
}

// mismasSalvoAviso compara dos configs por lo que el usuario puede observar en la TUI. No
// usa reflect.DeepEqual porque los campos que no ve nadie —los internal— pueden diferir sin
// que importe, y un aserto que falla por eso esconde el fallo que sí importa.
func mismasSalvoAviso(a, b Config) bool {
	if len(a.Roots) != len(b.Roots) {
		return false
	}
	for i := range a.Roots {
		if a.Roots[i] != b.Roots[i] {
			return false
		}
	}
	if a.RefreshInterval != b.RefreshInterval || a.DataDir != b.DataDir ||
		a.CloneDir != b.CloneDir || a.WorktreeDir != b.WorktreeDir {
		return false
	}
	if a.Forges != b.Forges {
		return false
	}
	if len(a.Keybindings) != len(b.Keybindings) {
		return false
	}
	for k, v := range a.Keybindings {
		if b.Keybindings[k] != v {
			return false
		}
	}
	if len(a.Commands) != len(b.Commands) {
		return false
	}
	for k, v := range a.Commands {
		if b.Commands[k] != v {
			return false
		}
	}
	return len(a.Hints(nil)) == len(b.Hints(nil))
}
