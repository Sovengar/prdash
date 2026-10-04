package worktree

import (
	"path/filepath"
	"testing"

	"prdash/internal/herdr"
)

// One thing: what wins when the two sources disagree.

func specVacia() Spec {
	return Spec{Repo: "/repos/acme", Branch: "feat/x", Path: "/wt/acme-feat-x", Label: "prdash/acme#1"}
}

func infoVacia() herdr.WorktreeInfo {
	return herdr.WorktreeInfo{}
}

// If Herdr says nothing, the caller's wins.
func TestComposedSinNadaDeHerdrSeQuedaConLoQuePidioElLlamador(t *testing.T) {
	spec := specVacia()
	wt := composed(spec, infoVacia())

	if wt.Path != spec.Path || wt.ID != spec.Path {
		t.Errorf("sin datos de Herdr dio path=%q id=%q, want %q en los dos", wt.Path, wt.ID, spec.Path)
	}
	if wt.Branch != spec.Branch {
		t.Errorf("sin datos de Herdr dio branch=%q, want %q", wt.Branch, spec.Branch)
	}
	if wt.Repo != spec.Repo {
		t.Errorf("sin datos de Herdr dio repo=%q, want %q", wt.Repo, spec.Repo)
	}
	if wt.Label != spec.Label {
		t.Errorf("sin datos de Herdr dio label=%q, want %q", wt.Label, spec.Label)
	}
	if wt.WorkspaceID != "" || wt.RootPaneID != "" {
		t.Errorf("sin datos de Herdr dio workspace=%q pane=%q, want vacios",
			wt.WorkspaceID, wt.RootPaneID)
	}
}

// For path, branch and containers, what Herdr said wins when it said something.
func TestComposedHerdrMandaDondeDigaAlgo(t *testing.T) {
	spec := specVacia()
	info := herdr.WorktreeInfo{
		Path:           "/otro/sitio/movido",
		Branch:         "renombrada-por-herdr",
		WorkspaceID:    "ws-1",
		RootPaneID:     "pane-1",
		WorkspaceLabel: "ws-label",
		Label:          "el-repo",
	}

	wt := composed(spec, info)

	if wt.Path != info.Path {
		t.Errorf("path=%q, want el de Herdr %q: si movió el checkout, el otro no vale",
			wt.Path, info.Path)
	}
	if wt.ID != info.Path {
		t.Errorf("id=%q, want %q: el id es el path, y si el path se movio el id va con el",
			wt.ID, info.Path)
	}
	if wt.Branch != info.Branch {
		t.Errorf("branch=%q, want la de Herdr %q", wt.Branch, info.Branch)
	}
	if wt.WorkspaceID != "ws-1" || wt.RootPaneID != "pane-1" {
		t.Errorf("contenedores=%q/%q, want ws-1/pane-1", wt.WorkspaceID, wt.RootPaneID)
	}
	if wt.Repo != spec.Repo {
		t.Errorf("repo=%q, want %q: Herdr no devuelve el repo, no hay de donde sacarlo",
			wt.Repo, spec.Repo)
	}
}

// The label is the EXCEPTION: it is the caller's.
func TestComposedLaEtiquetaEsDelLlamadorYNoDeHerdr(t *testing.T) {
	// Every Herdr field set at once and distinct from each other, which is the only way the
	// precedence shows: if two matched, either order would pass.
	info := herdr.WorktreeInfo{
		Path:           "/wt/movido",
		Branch:         "rama-de-herdr",
		WorkspaceID:    "ws-1",
		RootPaneID:     "pane-1",
		WorkspaceLabel: "etiqueta-del-workspace",
		Label:          "nombre-del-repo",
	}
	spec := specVacia()

	wt := composed(spec, info)

	if wt.Label != spec.Label {
		t.Errorf("label=%q, want la del llamador %q.\n"+
			"Con la etiqueta de Herdr, el worktree deja de ser reconocible para prdash:\n"+
			"no aparece al listar, no se reutiliza y no se quita.",
			wt.Label, spec.Label)
	}
	for _, par := range [][2]string{
		{"del workspace", info.WorkspaceLabel},
		{"del worktree", info.Label},
	} {
		if par[1] == spec.Label {
			t.Fatalf("la etiqueta %s (%q) es igual a la del llamador (%q): "+
				"el test no distingue nada", par[0], par[1], spec.Label)
		}
	}
	if info.WorkspaceLabel == info.Label {
		t.Fatalf("las dos etiquetas de Herdr coinciden (%q): el test no distingue nada",
			info.Label)
	}
	if wt.Path != info.Path || wt.Branch != info.Branch {
		t.Errorf("la excepcion se ha spilled: path=%q branch=%q, want los de Herdr",
			wt.Path, wt.Branch)
	}
}

func TestComposedSinEtiquetaDelLlamadorRecurreEnEsteOrden(t *testing.T) {
	t.Run("gana la del workspace", func(t *testing.T) {
		spec := specVacia()
		spec.Label = ""
		info := herdr.WorktreeInfo{
			WorkspaceLabel: "etiqueta-del-workspace",
			Label:          "nombre-del-repo",
		}
		if got := composed(spec, info).Label; got != "etiqueta-del-workspace" {
			t.Errorf("label=%q, want la del workspace: es la que se pidió al abrirlo", got)
		}
	})

	t.Run("sin la del workspace, la del repo", func(t *testing.T) {
		spec := specVacia()
		spec.Label = ""
		info := herdr.WorktreeInfo{Label: "nombre-del-repo"}
		if got := composed(spec, info).Label; got != "nombre-del-repo" {
			t.Errorf("label=%q, want la del repo", got)
		}
	})

	t.Run("sin ninguna, el nombre del path", func(t *testing.T) {
		spec := specVacia()
		spec.Label = ""
		if got := composed(spec, infoVacia()).Label; got != filepath.Base(spec.Path) {
			t.Errorf("label=%q, want el nombre del path %q", got, filepath.Base(spec.Path))
		}
	})

	// The third fallback is Herdr's PATH, not the caller's, which is where the recursion shows.
	t.Run("el ultimo recurso sale del path ya resuelto", func(t *testing.T) {
		spec := specVacia()
		spec.Label = ""
		info := herdr.WorktreeInfo{Path: "/otro/movido/prdash-42"}
		got := composed(spec, info)
		if got.Label != "prdash-42" {
			t.Errorf("label=%q, want el nombre del path RESUELTO prdash-42, no %q del llamador",
				got.Label, filepath.Base(spec.Path))
		}
		if filepath.Base(spec.Path) == "prdash-42" {
			t.Fatal("el test no distingue: los dos paths dan el mismo nombre")
		}
	})
}

func TestComposedSinPathNoInventaUnNombreDePath(t *testing.T) {
	spec := specVacia()
	spec.Label = ""
	spec.Path = ""
	wt := composed(spec, infoVacia())

	if wt.Label == "." {
		t.Error(`label="." con path vacío: es el nombre de path de "", que parece un ` +
			"nombre elegido y en realidad son todos el mismo")
	}
	if wt.Label != "" {
		t.Errorf("label=%q con path vacío, want vacía: no hay sitio del que sacar un nombre", wt.Label)
	}
	if wt.ID != "" || wt.Path != "" {
		t.Errorf("con spec sin path dio id=%q path=%q, want vacios", wt.ID, wt.Path)
	}
}
