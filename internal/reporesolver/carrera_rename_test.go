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

// The case that fails the rename is NOT a race, which is what it looks like: the mounts come from
//Bubbletea's single update goroutine and are serialised by construction.

// The script derives dest from the temporary it was handed, rather than through the environment.
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

// The reason it warns is that `dest` being occupied means something unexpected, which deserves a
// warning rather than an assumption.
func TestElRenameQueFallaNoDejaTemporalNiBorraElClonAjenoYPermiteReintentar(t *testing.T) {
	base := t.TempDir()

	// A normal remote with a commit, because a bare has no working tree.
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

	// ENOTEMPTY and EXDEV are different repairs (an occupant and a different device) and without
	//the system's reason the diagnosis is the same for both.
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
	// And it does NOT return a path with the error: the executor would believe it has a local repo.
	if destino != "" {
		t.Errorf("EnsureBare devolvió %q con el error", destino)
	}

	temporales, errGlob := filepath.Glob(filepath.Join(cloneDir, "**", "*.tmp-*"))
	if errGlob != nil {
		t.Fatal(errGlob)
	}
	if len(temporales) != 0 {
		t.Errorf("quedaron %d temporales tras una publicación fallida: %v", len(temporales), temporales)
	}

	// The foreign clone stays whole and usable, which is what makes the warning the right call.
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
