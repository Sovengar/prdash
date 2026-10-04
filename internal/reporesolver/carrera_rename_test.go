package reporesolver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/forge/model"
	"prdash/internal/gitcmd"
	"prdash/internal/testutil"
)

// El `rename` que publica el clon.
//
// Y el caso que lo hace fallar no es una carrera, que es lo que parece al leerlo. `EnsureBare`
// lo llama `Executor.resolveRepo`, que solo llama `TUI.update`, o sea que las monturas salen
// del único goroutine del `update` de Bubbletea y están serializadas por construcción. La
// carrera entre dos clones del mismo repo no se puede dar.
//
// Y entonces ¿qué es lo que falla? `rename` falla cuando `dest` ya existe y es un directorio
// con contenido, y `dest` acaba de comprobar el código línea a línea: `isRepo` dijo que no es un
// repo y el `os.Stat` que sigue lo limpió. Para que vuelva a estar ocupado alguien tiene que
// haber publicado entre medias, y eso ya no es una carrera sino otra cosa —un `mount` usb, un
// rsync, un script de arranque— que conviene avisar en vez de asumir.
//
// Y lo que se comprueba son las TRES propiedades que importan de ese `if`, y las tres son de
// limpieza:
//
//   - El temporal PROPIO se borra. Un `.tmp-` pesa lo que pesa el repo.
//   - El clon ajeno NO se toca. `rename` no pisa un directorio con contenido, y el código no
//     hace un `RemoveAll(dest)` en este camino a propósito: si el ocupante es de otro
//     programa, borrarlo sería peor que avisar.
//   - Y el siguiente intento funciona. El clon ajeno es un repo válido, así que `isRepo` lo
//     acepta y `EnsureBare` devuelve sin volver a clonar. El fallo deja el árbol en un estado
//     del que se puede reintentar sin limpiar a mano nada.

// gitQuePublicaElClonAntesDeDevolver es un `git` que clona de verdad y luego deja en `dest` un
// segundo clon, que es lo que hace el ocupante inesperado del que habla el test.
//
// Y el guion saca `dest` del propio temporal que le pasaron, quitándole el sufijo `.tmp-<nano>`,
// en vez de recibirlo por entorno: es igual de fiable y no depende de que el entorno del test
// llegue al subproceso, que `gitcmd.Env()` filtra por si acaso.
//
// Y hace el segundo clon DESPUÉS del primero, no antes. Si lo hiciera antes, el clon propio
// fallaría al encontrar el destino ocupado y estaríamos probando el error de clonar en el
// sitio del error de publicar, que es el despiste más fácil de cometer al escribir este guion.
func gitQuePublicaElClonAntesDeDevolver(t *testing.T, fuente string) *gitcmd.Runner {
	t.Helper()
	dir := t.TempDir()
	guion := filepath.Join(dir, "git")

	contenido := "#!/bin/sh\n" +
		"if [ \"$1\" = \"clone\" ]; then\n" +
		"  git \"$@\" || exit $?\n" +
		"  for ultimo; do :; done\n" +
		"  destino=\"${ultimo%%.tmp-*}\"\n" +
		"  git clone --bare --quiet -- " + fuente + " \"$destino\" >/dev/null 2>&1\n" +
		"  exit 0\n" +
		"fi\n" +
		"exec git \"$@\"\n"
	if err := os.WriteFile(guion, []byte(contenido), 0o755); err != nil {
		t.Fatal(err)
	}
	return &gitcmd.Runner{Bin: guion, Timeout: gitcmd.DefaultTimeout}
}

// TestElRenameQueFallaNoDejaTemporalNiBorraElClonAjenoYPermiteReintentar: `EnsureBare`.
//
// Y el caso es el error de publicación del clon, que era la última rama sin poder provocar del
// resolutor. Se provoca con un `git` que deja un segundo clon en el destino justo antes de que
// el código publique el suyo.
//
// Y el aserto que manda es el segundo: el clon ajeno sigue ahí. Es lo que distingue esta
// escritura de "no me deja nada" y de "me borra lo del otro", que es la forma en que un
// `RemoveAll(dest)` de limpieza a lo bruto acabaría con el trabajo de otro programa. Y
// el código NO lo hace en este camino, que es lo correcto: si lo que ocupa `dest` es de otro,
// lo propio es avisar y dejar que el otro decida.
//
// Y el tercero es el que hace que el error sea recuperable sin intervención: el clon ajeno es un
// repo válido, así que el siguiente `EnsureBare` lo ve con `isRepo` y devuelve sin clonar otra
// vez. Un error que obliga a borrar el árbol de clones a mano para reintentar es un error
// distinto del que se avisa.
func TestElRenameQueFallaNoDejaTemporalNiBorraElClonAjenoYPermiteReintentar(t *testing.T) {
	base := t.TempDir()

	// El remoto: un repo normal con un commit, porque un bare no tiene working tree y
	// `CommitFile` necesita uno. `EnsureBare` lo clona como bare, que es lo que hace el
	// ejecutor de verdad.
	origen := filepath.Join(base, "fuente")
	testutil.InitRepo(t, origen)
	testutil.CommitFile(t, origen, "f.txt", "base", "base")

	cloneDir := filepath.Join(base, "clones")
	nuevoResolver := func() *Resolver {
		return New(Options{
			Roots:    []string{base},
			CloneDir: cloneDir,
			MemoPath: filepath.Join(base, "memo.json"),
			Hosts:    map[string]string{"github.com": "github"},
			CloneURL: func(model.RepoRef) string { return origen },
			Git:      gitQuePublicaElClonAntesDeDevolver(t, origen),
		})
	}
	ref := model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/proyecto"}
	dest := nuevoResolver().barePath(ref)

	destino, err := nuevoResolver().EnsureBare(context.Background(), ref)
	if err == nil {
		t.Fatalf("EnsureBare devolvió nil y %q: el clon se publicó sobre un destino ocupado sin "+
			"avisar, y quien lo ocupa puede haber perdido su trabajo", destino)
	}

	// El aviso dice qué pasó y dónde, que es lo que separa "no se pudo publicar" de "el clon
	// está mal". Y el motivo del sistema va dentro: `ENOTEMPTY` y `EXDEV` son arreglos
	// distintos —el primero es un ocupante, el segundo es que el temporal cayó en otro
	// dispositivo— y sin él el diagnóstico es el mismo para los dos.
	if !strings.Contains(err.Error(), "publish the bare clone") {
		t.Errorf("el aviso %q no dice que falló la publicación", err)
	}
	if !strings.Contains(err.Error(), dest) {
		t.Errorf("el aviso %q no nombra el destino %s", err, dest)
	}
	if !strings.Contains(err.Error(), ".tmp-") {
		t.Errorf("el aviso %q no trae el motivo del sistema, que es lo que dice si el destino "+
			"está ocupado o si el temporal cayó en otro dispositivo", err)
	}
	// Y no devuelve una ruta con el error: el ejecutor creería que tiene repo local y montaría
	// un worktree sobre un temporal que ya no existe.
	if destino != "" {
		t.Errorf("EnsureBare devolvió %q con el error", destino)
	}

	// El temporal propio se fue, y este es el aserto de tamaño: cada `.tmp-` es un clon del
	// repo entero, y uno que sobrevive al intento se suma al siguiente.
	temporales, errGlob := filepath.Glob(filepath.Join(cloneDir, "**", "*.tmp-*"))
	if errGlob != nil {
		t.Fatal(errGlob)
	}
	if len(temporales) != 0 {
		t.Errorf("quedaron %d temporales tras una publicación fallida: %v", len(temporales), temporales)
	}

	// Y el clon ajeno sigue entero y utilizable. Que sea utilizable es lo que hace que el
	// siguiente intento sea gratis.
	if !isRepo(dest) {
		t.Errorf("EnsureBare se llevó por delante el clon que ya estaba en %s: un ocupante no "+
			"es necesariamente basura", dest)
	}
	r := nuevoResolver()
	p, err := r.EnsureBare(context.Background(), ref)
	if err != nil {
		t.Fatalf("el reintento falló con %v: el error anterior dejó el árbol en un estado del "+
			"que no se puede reintentar sin limpiar a mano", err)
	}
	if p != dest {
		t.Errorf("el reintento devolvió %q, want %q", p, dest)
	}
	if out := testutil.RunGit(t, p, "rev-parse", "--verify", "--quiet", "main"); out == "" {
		t.Error("el clon que se quedó no tiene la rama main: no sirve para montar")
	}
}
