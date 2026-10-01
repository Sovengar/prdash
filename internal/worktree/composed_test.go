package worktree

import (
	"path/filepath"
	"testing"

	"prdash/internal/herdr"
)

// Los tests de `composed` comprueban UNA COSA: qué gana cuando las dos fuentes
// discrepan, campo por campo. Es una función de precedencia, y una precedencia se
// demuestra enumerando los conflictos, no describiéndola.
//
// El caso de partida es el de "Herdr no dice nada", que es lo que devuelve un mock
// recién construido y por eso no dice NADA de lo que importa.

func specVacia() Spec {
	return Spec{Repo: "/repos/acme", Branch: "feat/x", Path: "/wt/acme-feat-x", Label: "prdash/acme#1"}
}

func infoVacia() herdr.WorktreeInfo {
	return herdr.WorktreeInfo{}
}

// TestComposedSinNadaDeHerdrSeQuedaConLoQuePidioElLlamador: si Herdr no dice nada,
// el worktree es el que pidió el llamador, punto.
//
// Es el caso base, y el que más se parece a un trabajo real: el mock recién hecho
// devuelve la estructura vacía, así que cualquier campo que se lea de ahí sale
// "". Si eso escribiera en el Worktree, un worktree recien creado tendría path "",
// rama "" y etiqueta "", y eso no se ve en un tests que no mire esos campos.
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
	// Y la etiqueta es la del llamador, que es la de ownership.
	if wt.Label != spec.Label {
		t.Errorf("sin datos de Herdr dio label=%q, want %q", wt.Label, spec.Label)
	}
	// Y lo que el workspace contribute va tal cual, sin inventar.
	if wt.WorkspaceID != "" || wt.RootPaneID != "" {
		t.Errorf("sin datos de Herdr dio workspace=%q pane=%q, want vacios",
			wt.WorkspaceID, wt.RootPaneID)
	}
}

// TestComposedHerdrMandaDondeDigaAlgo: para path, rama y contenedores, lo que dice
// Herdr gana SIEMPRE, incluso cuando el llamador dijo otra cosa.
//
// Y la razón es que no es negociable: si Herdr movió el checkout, el Worktree que
// dice el llamador apunta a un sitio donde no hay nada. El fallo no es un error
// visible, es un review montado en el vacío: los dos worktrees existen y solo uno
// tiene el checkout.
//
// Se prueba con los dos en COLISIÓN a propósito, porque si coincidieran la
// precedencia no se estaría probando.
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

	// El path movido manda, y el ID va con el porque el ID ES el path.
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
	// Y los contenedores se copian tal cual, que es lo unico que se sabe de ellos.
	if wt.WorkspaceID != "ws-1" || wt.RootPaneID != "pane-1" {
		t.Errorf("contenedores=%q/%q, want ws-1/pane-1", wt.WorkspaceID, wt.RootPaneID)
	}
	// El repo NO se toca: Herdr no lo contesta nunca, y el caller es quien sabe
	// de dónde se sacó la rama.
	if wt.Repo != spec.Repo {
		t.Errorf("repo=%q, want %q: Herdr no devuelve el repo, no hay de donde sacarlo",
			wt.Repo, spec.Repo)
	}
}

// TestComposedLaEtiquetaEsDelLlamadorYNoDeHerdr: la etiqueta es la EXCEPCION.
//
// Aquí es donde la precedencia está al revés, y es lo único que está al revés, así
// que es lo único que hay que probar con todos los datos de Herdr puestos.
//
// `spec.Label` es la etiqueta de OWNERSHIP: lo que pidió el llamador para reconocer
// el worktree. `info.Label` es el nombre del repo que reporta el nativo, y
// `info.WorkspaceLabel` es el `--label` con el que se abrió el workspace. Ninguno de
// los dos es lo que pidió el llamador, y si uno ganara, el worktree se renombraría
// solo al nombre del repo y prdash dejaría de reconocerlo: no lo encontraría al
// listar, ni al reutilizar, ni al quitar.
func TestComposedLaEtiquetaEsDelLlamadorYNoDeHerdr(t *testing.T) {
	// Con TODOS los campos de Herdr puestos a la vez, y distintos entre si, que es
	// la unica forma de que la precedencia se vea: si coincidieran, cualquier
	// orden daria lo mismo y el test no probaria nada.
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
	// Y se comprueba que los tres candidatos eran REALMENTE distintos entre sí,
	// porque si dos coincidieran el test passaría por la razón equivocada: no
	// estaría probando la precedencia sino la casualidad.
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
	// Y los otros campos SI los tomó de Herdr, para que se vea que la excepcion es
	// de la etiqueta y no una precedencia invertida entera.
	if wt.Path != info.Path || wt.Branch != info.Branch {
		t.Errorf("la excepcion se ha spilled: path=%q branch=%q, want los de Herdr",
			wt.Path, wt.Branch)
	}
}

// TestComposedSinEtiquetaDelLlamadorRecurreEnEsteOrden: si el llamador NO dio
// etiqueta, entonces si, y en este orden:
//
//  1. la del workspace, que es el `--label` que se pasó al abrirlo;
//  2. la del worktree, que es el nombre del repo;
//  3. el nombre del path.
//
// Y el orden NO es arbitrario, que es lo que hay que demostrar: los tres se
// distinguen en un test que los pone a los tres a la vez y va quitando uno.
//
// El 3 es el último porque un path puede ser cualquier cosa —`/tmp/x`, un checkout
// temporal— y un worktree cuya etiqueta es `/tmp/x` no lo distingue de otro. Un
// nombre de repo, en cambio, sí lo distingue, así que va antes.
func TestComposedSinEtiquetaDelLlamadorRecurreEnEsteOrden(t *testing.T) {
	// 1. La del workspace gana sobre la del repo: es la que pidió el llamador en su
	// momento, y la del repo es un nombre de repo, no un identificador.
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

	// 2. Sin la del workspace, la del repo. Que no es ideal —es un nombre de repo—,
	// pero es mejor que un path.
	t.Run("sin la del workspace, la del repo", func(t *testing.T) {
		spec := specVacia()
		spec.Label = ""
		info := herdr.WorktreeInfo{Label: "nombre-del-repo"}
		if got := composed(spec, info).Label; got != "nombre-del-repo" {
			t.Errorf("label=%q, want la del repo", got)
		}
	})

	// 3. Y sin ninguna de las dos, el nombre del path. Que es lo unico que queda.
	t.Run("sin ninguna, el nombre del path", func(t *testing.T) {
		spec := specVacia()
		spec.Label = ""
		if got := composed(spec, infoVacia()).Label; got != filepath.Base(spec.Path) {
			t.Errorf("label=%q, want el nombre del path %q", got, filepath.Base(spec.Path))
		}
	})

	// Y el 3 con el PATH de Herdr, no con el del llamador, que es donde se ve si el
	// recurso se calcula después de resolver la precedencia o antes. Aquí importa:
	// si se calculara con el path del llamador, un worktree movido por Herdr
	// acabaría con etiqueta del sitio viejo.
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

// TestComposedSinPathNoInventaUnNombreDePath: un `filepath.Base("")` es ".", y un
// worktree con etiqueta "." no lo distingue de nada.
//
// Por qué "." es PEOR que nada, que es lo que hace el aserto: una etiqueta vacía se
// ve que falta, y "." parece un nombre elegido a propósito. Dos worktrees sin path
// acabarían los dos llamándose ".", que es una colisión silenciosa: el `id` cae
// sobre el mismo valor y el segundo parece duplicado del primero.
//
// Cuando puede pasar: cuando Herdr devuelve un path VACÍO y el llamador no dio
// ninguno, o sea un spec incompleto. `Create` lo rechaza antes de llegar aquí, pero
// `composed` es una función pura y se llama con lo que le den: si algún día otro
// llamador la usa, el nombre del path tiene que ser el de un sitio, o nada.
func TestComposedSinPathNoInventaUnNombreDePath(t *testing.T) {
	spec := specVacia()
	spec.Label = ""
	spec.Path = ""
	wt := composed(spec, infoVacia())

	if wt.Label == "." {
		t.Error(`label="." con path vacío: es el nombre de path de "", que parece un ` +
			"nombre elegido y en realidad son todos el mismo")
	}
	// Y lo que se quiere de verdad: sin path, la etiqueta se queda vacía. Que es
	// visible.
	if wt.Label != "" {
		t.Errorf("label=%q con path vacío, want vacía: no hay sitio del que sacar un nombre", wt.Label)
	}
	if wt.ID != "" || wt.Path != "" {
		t.Errorf("con spec sin path dio id=%q path=%q, want vacios", wt.ID, wt.Path)
	}
}
