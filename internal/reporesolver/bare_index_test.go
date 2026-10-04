package reporesolver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

func TestEnsureBareLimpiaLosRestosDeUnIntentoFallido(t *testing.T) {
	origin, _ := fixture(t)
	ref := ghRef()
	r := newResolver(t, origin, ref)

	dest := r.barePath(ref)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	basura := filepath.Join(dest, "BASURA")
	if err := os.WriteFile(basura, []byte("restos de un intento fallido"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := r.EnsureBare(t.Context(), ref)
	if err != nil {
		t.Fatalf("EnsureBare con restos previos dio %v, want nil: los restos se limpian antes de reintentar", err)
	}
	if got != dest {
		t.Fatalf("EnsureBare devolvió %q, want %q", got, dest)
	}
	if _, err := os.Stat(basura); err == nil {
		t.Error("la basura del intento anterior sigue ahí: el directorio se usó sin limpiar")
	}
	if !isRepo(dest) {
		t.Error("donde debería estar el clon no hay un clon")
	}
}

// An empty directory is a leftover just as much as one with rubbish in it, and it is its own case.
func TestEnsureBareConUnRestoVacioTambiénFunciona(t *testing.T) {
	origin, _ := fixture(t)
	ref := ghRef()
	r := newResolver(t, origin, ref)

	dest := r.barePath(ref)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := r.EnsureBare(t.Context(), ref); err != nil {
		t.Fatalf("EnsureBare con un directorio vacío dio %v, want nil", err)
	}
	if !isRepo(dest) {
		t.Error("donde debería estar el clon no hay un clon")
	}
}

// The ROOT is not pruned even if its name starts with a dot.
func TestBuildIndexNoEntraEnDirectoriosOcultosNiSaltaLaRaiz(t *testing.T) {
	// Two different origins so the indexed repo can be told apart.
	origenVisible := filepath.Join(t.TempDir(), "visible.git")
	testutil.InitBare(t, origenVisible)
	origenOculto := filepath.Join(t.TempDir(), "oculto.git")
	testutil.InitBare(t, origenOculto)

	refVisible := ghRef()
	refOculto := glRef("gitlab.example.com", "grupo/escondido")

	root := filepath.Join(t.TempDir(), ".workspace")

	visible := filepath.Join(root, "proyecto")
	testutil.InitRepo(t, visible)
	testutil.CommitFile(t, visible, "a.txt", "a", "a")
	testutil.SetRemote(t, visible, "origin", origenVisible)

	escondido := filepath.Join(root, ".cache", "escondido")
	testutil.InitRepo(t, escondido)
	testutil.CommitFile(t, escondido, "b.txt", "b", "b")
	testutil.SetRemote(t, escondido, "origin", origenOculto)

	r := New(Options{
		Roots:    []string{root},
		CloneDir: filepath.Join(t.TempDir(), "repos"),
		MemoPath: filepath.Join(t.TempDir(), "memo.json"),
		Hosts:    map[string]string{"github.com": "github", "gitlab.example.com": "gitlab"},
		ParseRemote: func(raw string) (model.RepoRef, bool) {
			// The runner returns git's output WITH its trailing newline, so it has to be trimmed.
			switch strings.TrimSpace(raw) {
			case origenVisible:
				return refVisible, true
			case origenOculto:
				return refOculto, true
			}
			return model.RepoRef{}, false
		},
	})

	// The hidden root is indexed: it was configured, so the user's will wins.
	if got, ok := r.ResolveLocal(refVisible); !ok || got != visible {
		t.Errorf("ResolveLocal del repo visible = %q, %v; quiero %q: un root se indexa aunque su nombre empiece por punto",
			got, ok, visible)
	}
	if got, ok := r.ResolveLocal(refOculto); ok {
		t.Errorf("ResolveLocal encontró %q bajo un directorio oculto: un clon dentro de una caché no es un clon del usuario",
			got)
	}
}
