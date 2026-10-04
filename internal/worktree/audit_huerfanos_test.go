package worktree

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `Audit` es la función que decide qué worktrees son de prdash y cuáles están vivos, y las
// dos decisiones se_basis en leer el fichero `.git` de un directorio.
//
// Y eso tiene un detalle que es la mitad del motivo de este fichero: un worktree ENLAZADO
// tiene un `.git` que es un FICHERO con una línea `gitdir: …`, mientras que un repo normal
// tiene un `.git` que es un DIRECTORIO. Confundirlos es lo que hace que `prdash worktrees list` se
// tragase el repo principal del usuario por un worktree suyo y ofrezca borrarlo.
//
// Y la otra mitad es la detección de huérfanos: un enlace al repo de origen que ya no
// existe. Ese worktree está ahí ocupando disco, ya no se puede actualizar ni borrar por git,
// y solo se limpia si alguien se da cuenta. Marcarlo es lo que permite que `prdash worktrees
// remove --orphans` lo borre.

// TestElFicheroGitDeUnWorktreeDiceDondeEstaElOrigen: `linkedGitDir`, sus cuatro negativas.
//
// Y el caso de la ruta RELATIVA es el que importa y el más fácil de equivocar: git escribe
// `gitdir:` con una ruta absoluta en un worktree normal, pero un `.git` escrito a mano —o de
// una versión antigua— puede tenerla relativa, y esa es relativa AL WORKTREE, no al proceso.
// Sin resolverla, `sourceReachable` miraría un sitio equivocado y cada worktree parecería
// huérfano.
func TestElFicheroGitDeUnWorktreeDiceDondeEstaElOrigen(t *testing.T) {
	base := t.TempDir()

	// El caso bueno: ruta absoluta.
	abs := filepath.Join(base, "abs")
	mkdir(t, abs)
	escribirGit(t, abs, "gitdir: "+filepath.Join(base, "repo", ".git", "worktrees", "wt"))
	if got := linkedGitDir(abs); got != filepath.Join(base, "repo", ".git", "worktrees", "wt") {
		t.Errorf("una ruta absoluta dio %q", got)
	}

	// Relativa: se resuelve contra el worktree, no contra el directorio de trabajo.
	rel := filepath.Join(base, "rel")
	mkdir(t, rel)
	escribirGit(t, rel, "gitdir: ../repo/.git/worktrees/wt")
	want := filepath.Join(base, "repo", ".git", "worktrees", "wt")
	if got := linkedGitDir(rel); got != want {
		t.Errorf("una ruta relativa dio %q, want %q (relativa al worktree)", got, want)
	}

	// Con salto de linea final, que es como git lo escribe, y con espacios alrededor.
	conSalto := filepath.Join(base, "con-salto")
	mkdir(t, conSalto)
	escribirGit(t, conSalto, "gitdir:   "+filepath.Join(base, "otro")+"  \n")
	if got := linkedGitDir(conSalto); got != filepath.Join(base, "otro") {
		t.Errorf("con espacios y salto dio %q", got)
	}

	// Y las cuatro negativas, cada una con su motivo:
	//  - no hay fichero `.git`: no es un worktree.
	//  - no dice `gitdir:`: el fichero tiene otro formato.
	//  - dice `gitdir:` vacío: nada que resolver.
	//  - no se puede leer: sin permisos.
	casos := []struct {
		nombre  string
		valor   string
		ausente bool
	}{
		{"sin prefijo", "/otra/cosa\n", false},
		{"gitdir vacio", "gitdir:\n", false},
		{"gitdir con espacios", "gitdir:    \n", false},
		{"otro prefijo", "worktree: /x\n", false},
		{"fichero ausente", "", true},
	}
	for _, c := range casos {
		dir := filepath.Join(base, "neg-"+strings.ReplaceAll(c.nombre, " ", "-"))
		mkdir(t, dir)
		if !c.ausente {
			escribirGit(t, dir, c.valor)
		}
		if got := linkedGitDir(dir); got != "" {
			t.Errorf("%s: dio %q, want cadena vacia", c.nombre, got)
		}
	}

	// Y un `.git` que es un DIRECTORIO, que es un repo normal y no un worktree enlazado.
	// Leerlo como fichero falla, que es lo que evita que un repo se tome por worktree.
	repo := filepath.Join(base, "repo-normal")
	mkdir(t, filepath.Join(repo, ".git"))
	if got := linkedGitDir(repo); got != "" {
		t.Errorf("un repo normal dio gitdir %q: un .git que es directorio no es un enlace", got)
	}
}

// TestUnEnlaceRotoEsUnHuerfanoYUnoSanoNo: `sourceReachable`, y lo que depende de ella.
//
// Y la asimetría importa: sin `.git` NO es un worktree, pero con `.git` que no apunta a nada
// SÍ es un worktree huérfano. Tratar los dos como "no es mío" dejaría los huérfanos
// occupying disco para siempre, que es justo el problema que `--orphans` viene a resolver.
func TestUnEnlaceRotoEsUnHuerfanoYUnoSanoNo(t *testing.T) {
	base := t.TempDir()
	origen := filepath.Join(base, "repo", ".git", "worktrees")
	mkdir(t, origen)

	// Enlace sano: el destino EXISTE. Y eso es lo que hay que montar, porque
	// `sourceReachable` hace un `Stat` del gitdir, no del worktree: un destino que no
	// existe da exactamente el mismo resultado que un enlace roto, que es el fallo que
	// un test de "huerfano" monta sin querer al no crear el destino del caso sano.
	sano := filepath.Join(base, "sano")
	mkdir(t, sano)
	escribirFichero(t, filepath.Join(origen, "wt-sano"), "")
	escribirGit(t, sano, "gitdir: "+filepath.Join(origen, "wt-sano"))
	if !sourceReachable(sano) {
		t.Error("un enlace sano dio false")
	}

	// Enlace roto: el destino no existe.
	roto := filepath.Join(base, "roto")
	mkdir(t, roto)
	escribirGit(t, roto, "gitdir: "+filepath.Join(origen, "wt-que-no-existe"))
	if sourceReachable(roto) {
		t.Error("un enlace roto dio true: el worktree se reportaria vivo y ocuparia disco para siempre")
	}

	// Y sin `.git`: no es un worktree, así que no es un huérfano.
	vacio := filepath.Join(base, "vacio")
	mkdir(t, vacio)
	if sourceReachable(vacio) {
		t.Error("un directorio sin .git dio true")
	}
	// Y con un `.git` que no dice nada útil: tampoco.
	invalido := filepath.Join(base, "invalido")
	mkdir(t, invalido)
	escribirGit(t, invalido, "esto no es un enlace")
	if sourceReachable(invalido) {
		t.Error("un .git sin prefijo dio true")
	}
}

// TestAuditSoloDevuelveLoQueEsDePrdashYMarcaLoRoto: el recorrido entero.
//
// Y las tres filtraciones importan por lo que dejan pasar, no por lo que paran:
//
//   - Un repo normal del usuario no sale, porque no es un worktree enlazado ni tiene
//     ownership de prdash. Que saliera sería lo peor: `prdash worktrees remove` lo
//     borraría.
//   - Un worktree de OTRA herramienta no sale, por el ownership. Que saliera significaría
//     que prdash puede borrar el worktree de la herramienta de al lado.
//   - Un `.git` que es un directorio se salta de golpe, para no bajar a recorrer el
//     contenido entero de un repo.
//
// Y el huerfano sale MARCADO, no ausente: es el caso para el que existe la columna de
// estado del listado y el aviso a stderr.
func TestAuditSoloDevuelveLoQueEsDePrdashYMarcaLoRoto(t *testing.T) {
	base := t.TempDir()
	raiz := filepath.Join(base, "worktrees")
	mkdir(t, raiz)

	// El repo de origen, para que los worktrees tengan a qué apuntar.
	origen := filepath.Join(base, "repo", ".git", "worktrees")
	mkdir(t, origen)
	// Los destinos de los enlaces sanos tienen que existir en disco, por lo mismo que en
	// el caso anterior: `sourceReachable` hace `Stat` del gitdir.
	escribirFichero(t, filepath.Join(origen, "wt-1"), "")
	escribirFichero(t, filepath.Join(origen, "wt-ajeno"), "")

	// 1. Un repo normal del usuario: no es un worktree.
	repoUsuario := filepath.Join(base, "repo-del-usuario")
	mkdir(t, filepath.Join(repoUsuario, ".git"))

	// 2. Un worktree de prdash con el enlace sano.
	vivo := filepath.Join(raiz, "prdash-pr-1")
	mkdir(t, vivo)
	escribirGit(t, vivo, "gitdir: "+filepath.Join(origen, "wt-1"))

	// 3. Un worktree de prdash con el enlace roto: huérfano.
	roto := filepath.Join(raiz, "prdash-pr-2")
	mkdir(t, roto)
	escribirGit(t, roto, "gitdir: "+filepath.Join(origen, "wt-que-borre"))

	// 4. Un worktree de otra herramienta: no es de prdash.
	ajeno := filepath.Join(raiz, "vroom-pr-9")
	mkdir(t, ajeno)
	escribirGit(t, ajeno, "gitdir: "+filepath.Join(origen, "wt-ajeno"))

	// 5. Y un directorio normal sin nada.
	mkdir(t, filepath.Join(raiz, "una-carpeta"))

	entradas := NewGitDirect(raiz).Audit(context.Background())

	vistos := map[string]Entry{}
	for _, e := range entradas {
		vistos[filepath.Base(e.Path)] = e
	}

	if len(entradas) != 2 {
		t.Errorf("Audit devolvio %d entradas, want 2 (solo las de prdash): %+v", len(entradas), entradas)
	}
	if _, ok := vistos["repo-del-usuario"]; ok {
		t.Error("un repo normal del usuario salio en el listado: prdash worktrees remove podria borrarlo")
	}
	if _, ok := vistos["vroom-pr-9"]; ok {
		t.Error("un worktree de otra herramienta salio: el ownership no filtro")
	}

	// El vivo: con su rama, que se lee del propio worktree.
	v, ok := vistos["prdash-pr-1"]
	if !ok {
		t.Fatal("el worktree vivo no salio")
	}
	if v.Orphan {
		t.Errorf("el worktree vivo salio huerfano: %+v", v)
	}
	// Y el huérfano: marcado Y con motivo, porque un aviso sin motivo es "algo está mal"
	// sin decir qué arreglar.
	r, ok := vistos["prdash-pr-2"]
	if !ok {
		t.Fatal("el worktree huerfano no salio: deberia salir para que se pueda limpiar")
	}
	if !r.Orphan {
		t.Errorf("el worktree con el enlace roto no salio huerfano: %+v", r)
	}
	if strings.TrimSpace(r.Reason) == "" {
		t.Error("el huerfano salio sin motivo: un aviso sin motivo no dice que arreglar")
	}
	if !strings.Contains(r.Reason, "no longer reachable") {
		t.Errorf("el motivo %q no dice que el repo de origen ya no esta", r.Reason)
	}

	// Y la ETIQUETA manda sobre el directorio, que es lo que hace que el listado diga
	// "prdash-pr-1" y no el nombre completo de la ruta.
	if v.Label != "prdash-pr-1" {
		t.Errorf("Label = %q, want prdash-pr-1", v.Label)
	}

	// Y el orden es por ruta, que es lo que hace estable la salida entre ejecuciones.
	for i := 1; i < len(entradas); i++ {
		if entradas[i-1].Path > entradas[i].Path {
			t.Errorf("el listado no viene ordenado por ruta: %q antes que %q",
				entradas[i-1].Path, entradas[i].Path)
		}
	}
}

// TestAuditSinRaizNoDevuelveNada: la negativa de la entrada.
//
// Y una raiz vacia es el caso de un `WorktreeDir` sin configurar, que es lo que pasa con una
// config mínima en `--print`. Devolver `nil` y no un panic es lo que evita que la lista se
// vacíe con un fallo.
func TestAuditSinRaizNoDevuelveNada(t *testing.T) {
	if got := (&GitDirect{}).Audit(context.Background()); got != nil {
		t.Errorf("sin Base devolvio %+v, want nil", got)
	}
	// Y una raiz que no existe: `WalkDir` falla y el error se traga, que es lo correcto
	// porque el directorio de worktrees se crea en el primer montaje.
	inexistente := filepath.Join(t.TempDir(), "no-existe")
	if got := NewGitDirect(inexistente).Audit(context.Background()); len(got) != 0 {
		t.Errorf("una raiz inexistente devolvio %+v", got)
	}
}

// mkdir crea un directorio.
func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

// escribirFichero crea un fichero con su contenido, y crea los padres.
func escribirFichero(t *testing.T, path, contenido string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contenido), 0o644); err != nil {
		t.Fatal(err)
	}
}

// escribirGit pone el fichero `.git` de un worktree enlazado.
func escribirGit(t *testing.T, path, contenido string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(path, ".git"), []byte(contenido), 0o644); err != nil {
		t.Fatal(err)
	}
}
