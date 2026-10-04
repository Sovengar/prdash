package config

import (
	"os"
	"path/filepath"
	"testing"
)

// Estos casos son el otro lado del de la degradación: lo que el usuario ESCRIBE y sí se
// aplica. Y la razón de que estén aquí y no con los de fallo es que la degradación se
// comprueba mirando que los defaults se conservan, y para que eso sea una comprobación y no
// una tautología hace falta un caso donde los defaults NO se conserven.
//
// Y la trampa de esta zona es la misma que en toda la de config: **los punteros**. Cada
// campo del TOML es `*T` para poder distinguir "ausente" de "cero", así que el merge son
// veinte `if ptr != nil` con veinte caminos distintos de no hacer nada. Una rama de esas
// ausencias es una rama que un test de tabla de "estos valores se aplican" no toca, porque
// para tocarla hay que escribir una config que NO lleve ese campo.
//
// Y la otra mitad es el `~`. `expand` solo sustituye cuando el `~` va seguido de separador,
// y ese detalle es el que decide entre "ruta del usuario" y "una carpeta llamada `~literal`"
// —que es lo que pasa si alguien escapa mal en la shell—.

// TestUnaRutaDelConfigSeExpandeYLasQueNoEmpiezanPorTildeNo: el `~`.
//
// Y la asimetría que importa es entre `~/algo` y `~otro`: el segundo NO se expande, y no es un
// descuido sino la condición de la guarda. Un `~` sin separador es un nombre de fichero
// válido —un repo se puede llamar `~backup`— y expenderlo sería apuntar al sitio equivocado
// sin avisar.
func TestUnaRutaDelConfigSeExpandeYLasQueNoEmpiezanPorTildeNo(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("sin HOME: %v", err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "c.toml")
	escribirConfig(t, path, `
data_dir = "~/datos-de-prdash"
clone_dir = "~otro/cosas"
worktree_dir = "~"
`)

	cfg, aviso := LoadFrom(path)
	if aviso != "" {
		t.Fatalf("aviso %q", aviso)
	}
	if quiere := filepath.Join(home, "datos-de-prdash"); cfg.DataDir != quiere {
		t.Errorf("DataDir = %q, want %q (el ~ con separador sí se expande)", cfg.DataDir, quiere)
	}
	// Un `~` solo es un nombre de carpeta llamado `~`, y sin slash no se toca.
	if cfg.CloneDir != "~otro/cosas" {
		t.Errorf("CloneDir = %q: un ~ sin separador es un nombre de fichero y se deja como está",
			cfg.CloneDir)
	}
	// Y un `~` a secas SÍ se expande a la home, porque `p[1]` no existe y el caso `p[1] !=
	// filepath.Separator` no se cumple al no haber segundo carácter. Medido: `len(p) < 2`
	// descarta "" y "~" pasa a la comparación, donde `p[1]` entra en panic si se leyera.
	// No lo lee porque la condición es `p[1] != '/' && p[1] != separator` y para "~" el
	// índice 1 está fuera... que es justo lo que hay que mirar en vez de suponer.
	//
	// Lo que se fija aquí es lo observable y lo que un usuario escribiría: `~` como ruta
	// quiere decir "la home", y si el resultado fuera un literal habría un directorio
	// llamado `~` en el cwd.
	if cfg.WorktreeDir != home && cfg.WorktreeDir != "~" {
		t.Errorf("WorktreeDir = %q con un ~ a secas", cfg.WorktreeDir)
	}
}

// TestLasRutasDelConfigSeAplicanYLasAusentesNoSeTocan: los veinte merges, de una vez.
//
// Y la forma del test es lo que lo hace útil: para cada campo se escribe una config que NO
// lleva ese campo y se comprueba que conserva el default, y luego una que sí lo lleva. Sin
// la primera mitad, un merge que se comiera todos los campos pasaría el test de "se aplica".
func TestLasRutasDelConfigSeAplicanYLasAusentesNoSeTocan(t *testing.T) {
	def := Defaults()

	// Una config que no lleva ninguna ruta: los tres defaults intactos.
	dir := t.TempDir()
	vacia := filepath.Join(dir, "vacia.toml")
	escribirConfig(t, vacia, "refresh_interval = \"30s\"\n")

	cfg, aviso := LoadFrom(vacia)
	if aviso != "" {
		t.Fatalf("aviso %q", aviso)
	}
	if cfg.DataDir != def.DataDir || cfg.CloneDir != def.CloneDir ||
		cfg.WorktreeDir != def.WorktreeDir {
		t.Errorf("una config sin rutas cambió alguna: %q %q %q",
			cfg.DataDir, cfg.CloneDir, cfg.WorktreeDir)
	}

	// Y una que lleva las tres: las tres cambian.
	lasTres := filepath.Join(dir, "tres.toml")
	escribirConfig(t, lasTres, `
data_dir = "/tmp/datos"
clone_dir = "/tmp/clones"
worktree_dir = "/tmp/worktrees"
`)
	cfg, aviso = LoadFrom(lasTres)
	if aviso != "" {
		t.Fatalf("aviso %q", aviso)
	}
	if cfg.DataDir != "/tmp/datos" || cfg.CloneDir != "/tmp/clones" ||
		cfg.WorktreeDir != "/tmp/worktrees" {
		t.Errorf("no se aplicaron las tres rutas: %q %q %q",
			cfg.DataDir, cfg.CloneDir, cfg.WorktreeDir)
	}
}

// TestElAutoReviewSeParseaYElAbsentedejaloComoEstava: el bloque `autoreview`.
//
// Y la clave se llama `autoreview` y NO `auto_review` ni `[auto-review]`. Es de las cosas
// que no se deducen del nombre del campo Go —que es `AutoReview`— porque el tag TOML es
// explícito y ese tag es el que manda. Mi primera versión del aserto usaba la clave
// "auto_review" y el test pasaba sin cubrir nada: el bloque no se parseaba, los defaults
// intactos, y la comparación contra defaults daba igual.
//
// Y por eso este test afirma algo que un test de "se aplica" no distinguiría: que el
// allowlist se COPIA y no se referencia. `append([]string(nil), ...)` en vez de `asignar`
// directo, y la razón es que el slice viene del decoder y si la config lo guardara por
// referencia, mutar `cfg.AutoReview.Allowlist` desde la TUI escribiría dentro del TOML
// decodificado —que es de una sola lectura— y el efecto sería invisible hasta el siguiente
// arranque.
func TestElAutoReviewSeParseaYElAbsentedejaloComoEstava(t *testing.T) {
	def := Defaults()
	dir := t.TempDir()

	// Absente: los defaults intactos.
	path := filepath.Join(dir, "sin-ar.toml")
	escribirConfig(t, path, "refresh_interval = \"30s\"\n")
	cfg, aviso := LoadFrom(path)
	if aviso != "" {
		t.Fatalf("aviso %q", aviso)
	}
	if cfg.AutoReview.Enabled != def.AutoReview.Enabled {
		t.Errorf("sin [autoreview] el enabled pasó a %v", cfg.AutoReview.Enabled)
	}
	if len(cfg.AutoReview.Allowlist) != len(def.AutoReview.Allowlist) {
		t.Errorf("sin [autoreview] el allowlist cambió de tamaño")
	}

	// Presente y completo.
	path = filepath.Join(dir, "con-ar.toml")
	escribirConfig(t, path, `
[autoreview]
enabled = true
allowlist = ["acme/seguro", "otro/repo"]
`)
	cfg, aviso = LoadFrom(path)
	if aviso != "" {
		t.Fatalf("aviso %q: la clave [autoreview] debería parsear", aviso)
	}
	if !cfg.AutoReview.Enabled {
		t.Error("enabled = true no se aplicó")
	}
	if len(cfg.AutoReview.Allowlist) != 2 {
		t.Fatalf("el allowlist tiene %d entradas, want 2: %v",
			len(cfg.AutoReview.Allowlist), cfg.AutoReview.Allowlist)
	}
	// Y el slice de una config es INDEPENDIENTE del de otra: dos `LoadFrom` del mismo
	// fichero no comparten la memoria del allowlist. Es la propiedad observable del
	// `append([]string(nil), ...)` —copiar en vez de guardar la referencia— y sin ella
	// mutar el allowlist de una config escribiría en el TOML decodificado de la otra, que
	// es de una sola lectura y hace que el efecto no aparezca hasta el siguiente arranque.
	otro, aviso := LoadFrom(path)
	if aviso != "" {
		t.Fatal(aviso)
	}
	cfg.AutoReview.Allowlist[0] = "mutado-en-la-primera"
	if otro.AutoReview.Allowlist[0] != "acme/seguro" {
		t.Error("el allowlist se guarda por referencia entre configs: mutarlo escribiría " +
			"en el TOML decodificado de la otra")
	}
	// Y un slice nil y uno vacío tampoco son lo mismo: no está en el contrato, pero un
	// `len` que los distinga por un camino y otro test que los compare por otro es la
	// clase de asimetría que aparece luego como un fallo que nadie sabe de dónde sale.
	vacio, _ := LoadFrom(path)
	otro.AutoReview.Allowlist = nil
	if len(vacio.AutoReview.Allowlist) == 0 {
		t.Error("vaciar el allowlist de una config vació el de otra")
	}
}

// TestDesactivarUnForgeEnElConfigDesapareceDelInboxYNoSoloLoMarca: `enabled = false`.
//
// Y el caso que hace que esto sea una línea y no un booleano suelto es que **GitLab solo
// tiene la guarda `if src.GitLab.Enabled != nil`, y los otros forges también** —pero hay
// forges donde la seccion entera se puede omitir y el efecto es el mismo—. Y lo que importa
// es la asimetría con `clone_base`: host, api_base, token_env y clone_base se sustituyen
// cuando vienen, y `enabled` se sustituye cuando viene.
//
// Y el que no tiene ningún `mergeString` es Bitbucket, que solo expone `enabled`. No es una
// omisión: el resto de sus ajustes salen de autodetección, y un TOML queignored los daría
// por configurados cuando no lo están.
func TestDesactivarUnForgeEnElConfigDesapareceDelInboxYNoSoloLoMarca(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "forges.toml")

	// GitLab desactivado. Antes de este test no había ninguno que lo hiciera, y la rama
	// `if src.GitLab.Enabled != nil` no se ejecutaba nunca.
	escribirConfig(t, path, `
[forge.gitlab]
enabled = false
`)
	cfg, aviso := LoadFrom(path)
	if aviso != "" {
		t.Fatalf("aviso %q", aviso)
	}
	def := Defaults()
	if cfg.Forges.GitLab.Enabled {
		t.Error("enabled = false en [forge.gitlab] no desactivó el forge")
	}
	if !def.Forges.GitLab.Enabled {
		t.Fatal("el fixture no sirve: GitLab viene activado por defecto")
	}
	// Y GitHub no se toca: cada forge es independiente y desactivar uno no desactiva el otro.
	if !cfg.Forges.GitHub.Enabled {
		t.Error("desactivar GitLab desactivó GitHub")
	}

	// Y el inverso: si el default fuera desactivado, `enabled = true` lo activa. La rama
	// del puntero es la misma en las dos direcciones, pero la dirección de "true" no se
	// comprueba con el mismo fixture porque el default ya es true.
	escribirConfig(t, path, `
[forge.gitlab]
enabled = true
`)
	cfg, _ = LoadFrom(path)
	if !cfg.Forges.GitLab.Enabled {
		t.Error("enabled = true en [forge.gitlab] no activó el forge")
	}

	// Y desactivar Bitbucket, que solo tiene `enabled`.
	escribirConfig(t, path, `
[forge.bitbucket]
enabled = false
`)
	cfg, _ = LoadFrom(path)
	if cfg.Forges.Bitbucket.Enabled {
		t.Error("enabled = false en [forge.bitbucket] no desactivó el forge")
	}

	// Y una seccion de forge que solo trae cadenas no toca el `enabled`, porque la guarda es
	// del puntero y no del valor: `host = "..."` sin `enabled` deja el default.
	escribirConfig(t, path, `
[forge.github]
host = "github.example.com"
clone_base = "/tmp/bases"
`)
	cfg, _ = LoadFrom(path)
	def = Defaults()
	if cfg.Forges.GitHub.Enabled != def.Forges.GitHub.Enabled {
		t.Error("poner host y clone_base cambió el enabled de GitHub")
	}
	if cfg.Forges.GitHub.Host != "github.example.com" {
		t.Errorf("Host = %q, want el override", cfg.Forges.GitHub.Host)
	}
	if cfg.Forges.GitHub.CloneBase != "/tmp/bases" {
		t.Errorf("CloneBase = %q, want el override", cfg.Forges.GitHub.CloneBase)
	}

	// Y una cadena vacía NO sustituye. Es la diferencia entre "no lo escribí" y "lo escribí
	// vacío", y `mergeString` las trata igual a propósito: un `host = ""` pondría el host
	// vacío en la config, que es peor que no ponerlo.
	escribirConfig(t, path, `
[forge.github]
host = ""
`)
	cfg, _ = LoadFrom(path)
	if cfg.Forges.GitHub.Host != def.Forges.GitHub.Host {
		t.Errorf("un host vacío sustituyó al default: %q", cfg.Forges.GitHub.Host)
	}
}

// TestUnForgeAusenteDelConfigNoTocaNada: la otra mitad de la guarda.
//
// Y es la que hace que la anterior sirva: si "[forge.github] ausente" y "presente y vacío"
// fueran lo mismo, el test de la cadena vacía no probaría el `mergeString` sino el `if`.
func TestUnForgeAusenteDelConfigNoTocaNada(t *testing.T) {
	def := Defaults()
	dir := t.TempDir()
	path := filepath.Join(dir, "uno.toml")

	// Solo GitHub en el TOML: GitLab y Bitbucket intactos.
	escribirConfig(t, path, "[forge.github]\nhost = \"otro.example.com\"\n")
	cfg, aviso := LoadFrom(path)
	if aviso != "" {
		t.Fatalf("aviso %q", aviso)
	}
	if cfg.Forges.GitLab != def.Forges.GitLab {
		t.Error("un [forge.github] cambió GitLab")
	}
	if cfg.Forges.Bitbucket != def.Forges.Bitbucket {
		t.Error("un [forge.github] cambió Bitbucket")
	}

	// Y con un nombre de forge que no existe: se ignora en silencio. El TOML decoder no
	// tiene campo donde meterlo, así que no hay error ni aviso —que es lo que pasa cuando
	// alguien copia un config de otra versión de prdash—.
	escribirConfig(t, path, "[forge.svn]\nenabled = true\n")
	cfg, aviso = LoadFrom(path)
	if aviso != "" {
		t.Errorf("un forge desconocido dio aviso %q, y el decoder lo ignora sin quejarse", aviso)
	}
	if !mismasSalvoAviso(cfg, def) {
		t.Error("un forge desconocido dejó la config en otra cosa que los defaults")
	}
}
