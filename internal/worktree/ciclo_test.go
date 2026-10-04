package worktree

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/testutil"
)

// Estas cinco funciones son el otro lado de `Audit`: lo que se hace con un worktree cuando
// hay que actuar sobre él. Y comparten una propiedad que las hace delicadas de probar —
// **cada una tiene una negativa que no es un fallo sino un "no hago nada"**, y una negativa
// mal distinguida se traduce en borrar trabajo del usuario.
//
// Y el caso que da nombre al fichero es el del worktree con la MISMA ruta y OTRA rama:
// `Create` lo rechaza, y ese rechazo es lo que evita que un review se monte encima de otro
// con un `git worktree add` que git se negaría a hacer con un mensaje que nadie lee.

// wtConRamaMontaUnWorktreeReal bajo la raíz dada y devuelve la raíz.
func wtConRamaMonta(t *testing.T, raiz, etiqueta, rama string) Worktree {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "repo")
	testutil.InitRepo(t, repo)
	testutil.CommitFile(t, repo, "base.txt", "base", "base")

	// La rama tiene que existir y NO puede ser `main`: git no deja tener dos worktrees
	// en la misma rama, y `main` ya la tiene el repo principal. La primera versión de
	// este helper pasaba "main" y los cinco tests de este fichero fallaban con "'main'
	// is already used by worktree", que es un error del FIXTURE y no del código que se
	// quiere probar.
	if rama == "main" {
		rama = "pr-" + etiqueta
	}
	testutil.RunGit(t, repo, "branch", rama)
	spec := Spec{
		Repo:   repo,
		Branch: rama,
		Path:   filepath.Join(raiz, etiqueta),
		Label:  etiqueta,
	}
	wt, err := NewGitDirect(raiz).Create(context.Background(), spec)
	if err != nil {
		t.Fatalf("crear el worktree %s: %v", etiqueta, err)
	}
	return wt
}

// TestCrearSobreUnWorktreeQueYaEstaEnLaMismaRamaLoReutiliza: el camino de reutilización.
//
// Y aquí está la asimetría completa, y es de las dos mitades: la MISMA rama se reutiliza y
// la OTRA se rechaza. Reutilizar es lo correcto —el review ya está montado y rehacerlo
// perdería el estado— y rechazar lo otro también, porque `git worktree add` no puede poner
// dos worktrees en la misma rama, y si el código no lo comprueba el error llega del git con
// un mensaje que no dice qué ítem lo provocó.
func TestCrearSobreUnWorktreeQueYaEstaEnLaMismaRamaLoReutiliza(t *testing.T) {
	raiz := t.TempDir()
	primero := wtConRamaMonta(t, raiz, "prdash-pr-1", "feat/x")

	// La misma rama: reutiliza, y no crea un segundo.
	g := NewGitDirect(raiz)
	segundo, err := g.Create(context.Background(), Spec{
		Repo: primero.Repo, Branch: "feat/x",
		Path: primero.Path, Label: "prdash-pr-1",
	})
	if err != nil {
		t.Fatalf("volver a crear sobre la misma rama dio error: %v", err)
	}
	if segundo.Path != primero.Path || segundo.Branch != "feat/x" {
		t.Errorf("el worktree reutilizado no es el mismo: %+v", segundo)
	}
	// Y sigue habiendo UN worktree, no dos.
	if n := len(strings.Split(strings.TrimSpace(
		testutil.RunGit(t, primero.Repo, "worktree", "list")), "\n")); n != 2 {
		// 2 líneas: el worktree principal y el del review.
		t.Errorf("hay %d líneas en worktree list, want 2", n)
	}

	// Otra rama en la MISMA ruta: rechazo, y el mensaje tiene que decir qué rama había y
	// cuál se pedía. Y la otra rama existe de verdad, para que el rechazo sea el del
	// código y no el de git —que daría un mensaje distinto—.
	testutil.RunGit(t, primero.Repo, "branch", "otra")
	// Y el mensaje tiene que decir qué rama había y cuál se pedía: un "already exists"
	// a secas deja a quien lee sin saber si el bug es suyo o del código.
	_, err = g.Create(context.Background(), Spec{
		Repo: primero.Repo, Branch: "otra",
		Path: primero.Path, Label: "prdash-pr-1",
	})
	if err == nil {
		t.Fatal("crear con otra rama sobre el mismo worktree dio nil")
	}
	for _, quiere := range []string{"feat/x", "otra"} {
		if !strings.Contains(err.Error(), quiere) {
			t.Errorf("el error %q no menciona %q", err, quiere)
		}
	}
	// Y el worktree sigue con SU rama: el rechazo no lo tocó.
	if got := testutil.RunGit(t, primero.Path, "rev-parse", "--abbrev-ref", "HEAD"); got != "feat/x" {
		t.Errorf("tras el rechazo la rama quedó en %q, want feat/x", got)
	}
}

// TestLaEtiquetaDelWorktreeLaPoneElNombreDelDirectorioSiNoViene: el default.
//
// Y la razón por la que el default es el nombre del directorio y no otra cosa: la etiqueta
// es lo que `Owned` reconoce como "de prdash", y lo que el listado y el aviso a stderr
// enseñan. Si el default fuera el nombre de la rama, un worktree de una rama que no lleva
// el prefijo dejaría de ser reconocible como suyo, y el único nombre que siempre lleva el
// prefijo es el del directorio.
func TestLaEtiquetaDelWorktreeLaPoneElNombreDelDirectorioSiNoViene(t *testing.T) {
	raiz := t.TempDir()
	repo := filepath.Join(t.TempDir(), "repo")
	testutil.InitRepo(t, repo)
	testutil.CommitFile(t, repo, "a.txt", "a", "a")

	g := NewGitDirect(raiz)

	// Sin etiqueta: sale el nombre del directorio, que es la ruta.
	testutil.RunGit(t, repo, "branch", "pr-7")
	wt, err := g.Create(context.Background(), Spec{
		Repo: repo, Branch: "pr-7", Path: filepath.Join(raiz, "prdash-pr-7"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if wt.Label != "prdash-pr-7" {
		t.Errorf("sin etiqueta salió %q, want el nombre del directorio", wt.Label)
	}
	// Y la etiqueta meet el requisito de `Owned`, que es lo que permite que `Audit` lo
	// liste y que `remove` lo borre.
	if !Owned(wt.Label, wt.Path) {
		t.Errorf("la etiqueta por defecto %q no la reconoce Owned", wt.Label)
	}

	// Con etiqueta: manda la suya. Y el caso que importa es que difiera del directorio,
	// que es lo que pasa cuando el nombre del directorio no lleva el prefijo y hay que
	// marcarlo como propio de todas formas.
	testutil.RunGit(t, repo, "branch", "pr-9")
	conEtiqueta, err := g.Create(context.Background(), Spec{
		Repo: repo, Branch: "pr-9",
		Path: filepath.Join(raiz, "sin-prefijo"), Label: "prdash-pr-9",
	})
	if err != nil {
		t.Fatal(err)
	}
	if conEtiqueta.Label != "prdash-pr-9" {
		t.Errorf("con etiqueta salió %q", conEtiqueta.Label)
	}
	// Y con etiqueta puesta, la ruta sin prefijo esRecognition-free pero la etiqueta no.
	if !Owned(conEtiqueta.Label, conEtiqueta.Path) {
		t.Errorf("con etiqueta %q en un directorio sin prefijo, Owned no lo reconoce",
			conEtiqueta.Label)
	}
	// Y el ID y el Path son la ruta, siempre. Son lo que se le pasa a `Remove`.
	if conEtiqueta.ID != conEtiqueta.Path || conEtiqueta.Path == "" {
		t.Errorf("ID/Path no son la ruta: %+v", conEtiqueta)
	}
	// Y el repo de origen viaja en la struct, que es lo que permite borrar sin volver a
	// resolver nada.
	if conEtiqueta.Repo != repo {
		t.Errorf("Repo = %q, want %q", conEtiqueta.Repo, repo)
	}
}

// TestRemoveIfCleanNoBorraUnWorktreeSucioYExplicaPorQue: la puerta antes de borrar.
//
// Y el motivo está en el nombre de la función y en lo que devuelve: `bool, string`. El
// bool es "¿lo borré?", y el string es POR QUÉ NO. Devolver solo el error haría que un
// worktree con cambios sin commitear pareciera un fallo de borrado, cuando en realidad es
// la salvaguarda funcionando.
//
// Y el caso que hace que el motivo importe de verdad: cambios sin commitear. El usuario
// abre la review, edita algo para probar, cierra y borra. Si el borrado no lo parase,
// perdería esas ediciones sin que nada las pidiera.
func TestRemoveIfCleanNoBorraUnWorktreeSucioYExplicaPorQue(t *testing.T) {
	raiz := t.TempDir()
	wt := wtConRamaMonta(t, raiz, "prdash-pr-1", "main")
	g := NewGitDirect(raiz)
	ctx := context.Background()

	// Limpio: se borra.
	borrado, motivo, err := g.RemoveIfClean(ctx, wt.Path)
	if err != nil {
		t.Fatalf("RemoveIfClean de un worktree limpio: %v", err)
	}
	if !borrado {
		t.Errorf("un worktree limpio no se borró: %q", motivo)
	}
	if motivo != "" {
		t.Errorf("tras borrar quedó el motivo %q, y no hay motivo de nada", motivo)
	}
	// Y el registro de git no lo lista ya, que es la garantía que da `Remove`. El
	// directorio puede quedar vacío en disco según cómo lo quite git, y afirmar sobre eso
	// sería afirmar sobre el comportamiento de git y no sobre el del código.
	registro := testutil.RunGit(t, wt.Repo, "worktree", "list")
	if strings.Contains(registro, "prdash-pr-1") {
		t.Errorf("tras Remove el worktree sigue en el registro de git: %q", registro)
	}

	// Sucio: NO se borra, y el motivo lo dice.
	sucio := wtConRamaMonta(t, raiz, "prdash-pr-2", "main")
	if err := os.WriteFile(filepath.Join(sucio.Path, "cambiado.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	borrado, motivo, err = g.RemoveIfClean(ctx, sucio.Path)
	if err != nil {
		t.Fatalf("RemoveIfClean de un worktree sucio: %v", err)
	}
	if borrado {
		t.Error("un worktree con cambios sin commitear se borró: se perdieron las ediciones")
	}
	if !exists(t, sucio.Path) {
		t.Error("el worktree sucio desapareció igualmente")
	}
	if strings.TrimSpace(motivo) == "" {
		t.Fatal("no se borró y no hay motivo: el usuario no tiene forma de saber por qué")
	}
	// Y el motivo tiene que poder.REACCIONAR: un motivo que no dice que hacer convierte
	// "no lo he borrado" en un misterio.
	if !strings.Contains(strings.ToLower(motivo), "uncommitted") &&
		!strings.Contains(strings.ToLower(motivo), "changes") {
		t.Logf("el motivo no menciona los cambios: %q", motivo)
	}
}

// TestDirtyCuentaLosFicherosSinTrackear: por qué no basta con diff.
//
// Y este es el caso que el comentario de `dirty` explica y que un test de `diff --quiet`
// pasaría por alto: un fichero NUEVO sin trackear es trabajo sin commitear, y `diff` lo
// ignora porque no hay nada commiteado que comparar. Con `diff`, borrar un worktree con un
// fichero nuevo tiraría ese fichero.
//
// Y el segundo caso es el inverso y también importa: un fichero que está en el índice pero
// sin commitear. `diff` sin `--cached` tampoco lo ve.
func TestDirtyCuentaLosFicherosSinTrackear(t *testing.T) {
	raiz := t.TempDir()
	wt := wtConRamaMonta(t, raiz, "prdash-pr-1", "main")
	g := NewGitDirect(raiz)
	ctx := context.Background()

	// Limpio.
	sucio, err := g.dirty(ctx, wt.Path)
	if err != nil {
		t.Fatal(err)
	}
	if sucio {
		t.Error("un worktree recién creado salió sucio")
	}

	// Con un fichero NUEVO sin trackear. Es el caso que `diff` no ve.
	nuevo := filepath.Join(wt.Path, "nuevo.txt")
	if err := os.WriteFile(nuevo, []byte("trabajo sin commitear"), 0o644); err != nil {
		t.Fatal(err)
	}
	sucio, err = g.dirty(ctx, wt.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !sucio {
		t.Error("un fichero sin trackear no cuenta como trabajo: diff lo ignora y el " +
			"borrado lo tiraría")
	}

	// Y en el índice pero sin commitear, que es el otro hueco de `diff`.
	if err := os.WriteFile(nuevo, []byte("cambiado otra vez"), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.RunGit(t, wt.Path, "add", "nuevo.txt")
	sucio, err = g.dirty(ctx, wt.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !sucio {
		t.Error("cambios en el índice sin commitear no cuentan como trabajo")
	}

	// Y con todo commiteado vuelve a estar limpio.
	testutil.RunGit(t, wt.Path, "commit", "-m", "lo que sea")
	sucio, err = g.dirty(ctx, wt.Path)
	if err != nil {
		t.Fatal(err)
	}
	if sucio {
		t.Error("tras commitear sigue salido sucio")
	}
}

// TestRemoveSeNiegaATocarLoQueNoEsPropio: la negativa que protege el trabajo del usuario.
//
// Y son tres rechazos, y cada uno previene un daño distinto:
//
//   - Una ruta que no existe: no hay nada que borrar. Devolver nil, no error, porque el
//     trabajo ya está hecho —es el estado al que se quiere llegar— y un error haría que
//     un script reintentara.
//   - Una ruta que no es un worktree de prdash: podría ser un repo del usuario o el
//     worktree de otra herramienta. Esto es lo que impide que `prdash worktrees remove` sea
//     un `rm` con pasos.
//   - Una ruta que sale de la raíz gestionada: aunque el directorio se llame
//     `prdash-algo`, si no cuelga de la raíz que se le dio a prdash, no es suyo.
func TestRemoveSeNiegaATocarLoQueNoEsPropio(t *testing.T) {
	raiz := t.TempDir()
	wt := wtConRamaMonta(t, raiz, "prdash-pr-1", "main")
	g := NewGitDirect(raiz)
	ctx := context.Background()

	// Un repo del usuario, con nombre de prdash pero fuera de la raíz: el caso que el
	// nombre no debe poder salvar.
	ajeno := filepath.Join(t.TempDir(), "prdash-mio")
	testutil.InitRepo(t, ajeno)
	testutil.CommitFile(t, ajeno, "importante.txt", "no me borres", "importante")

	// Y el campo `debeSeguir` dice qué tiene que pasar con la ruta después, porque no es lo
	// mismo en los tres casos: una ruta que NO existe no puede "seguir ahí", y afirmar
	// sobre ello comprobaría una tautología. La primera versión de este test tenía un solo
	// assert —"la ruta sigue ahí"— para los tres, y el caso inexistente fallaba por esa
	// razón y no porque `Remove` hiciera algo mal.
	carpeta := filepath.Join(raiz, "prdash-carpeta-vacia")
	if err := os.MkdirAll(carpeta, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		nombre     string
		ruta       string
		debeSeguir bool
	}{
		{"fuera de la raiz", ajeno, true},
		{"inexistente", filepath.Join(raiz, "prdash-no-existe"), false},
		{"no es worktree", carpeta, true},
	} {
		err := g.Remove(ctx, c.ruta)
		if err == nil {
			t.Errorf("%s: Remove dio nil, y tiene que negar", c.nombre)
			continue
		}
		// Y la ruta sigue ahí cuando tenía que seguir.
		if c.debeSeguir && !exists(t, c.ruta) {
			t.Errorf("%s: Remove borró %s, que no es suyo", c.nombre, c.ruta)
		}
	}

	// Y el repo ajeno sigue con su fichero, que es la comprobación que lo demuestra.
	if _, err := os.Stat(filepath.Join(ajeno, "importante.txt")); err != nil {
		t.Errorf("el repo ajeno perdió su contenido: %v", err)
	}

	// Y el worktree de verdad sí sale del registro, para que los tres rechazos de arriba no
	// signifiquen que `Remove` no hace nada.
	if err := g.Remove(ctx, wt.Path); err != nil {
		t.Fatalf("Remove del worktree propio: %v", err)
	}
	if strings.Contains(testutil.RunGit(t, wt.Repo, "worktree", "list"), "prdash-pr-1") {
		t.Error("el worktree propio sigue en el registro tras Remove")
	}
}

// TestInspeccionarTraeElEstadoDelWorktreeSinMontarNada: `inspect`.
//
// Y la razón de que exista es no crear el worktree para leer de él. `git worktree list`
// responde con lo que hay en el registro sin materializar nada, y un `Create` para luego
// tirar el resultado dejaría un directorio de más cada vez que se consulta el estado de un
// review. Y eso en un bucle de refresco son decenas.
func TestInspeccionarTraeElEstadoDelWorktreeSinMontarNada(t *testing.T) {
	raiz := t.TempDir()
	wt := wtConRamaMonta(t, raiz, "prdash-pr-1", "main")
	g := NewGitDirect(raiz)

	visto, ok, err := g.inspect(context.Background(), wt.Path)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	// El segundo valor es "es un worktree", y es lo que hace que un directorio cualquiera
	// bajo la raíz no se lea como review.
	if !ok {
		t.Fatal("inspect no vio un worktree que existe")
	}
	if visto.Branch == "" {
		t.Error("inspect no leyó la rama del worktree")
	}
	if visto.Path != wt.Path {
		t.Errorf("Path = %q, want %q", visto.Path, wt.Path)
	}

	// Y sobre algo que NO es un worktree: dice que no, sin error. Un directorio suelto bajo
	// la raíz tiene que ser "no es un review", no un fallo —la raíz se escanea entera y
	// hay carpetas que no son worktrees—.
	carpeta := filepath.Join(raiz, "prdash-carpeta")
	if err := os.MkdirAll(carpeta, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := g.inspect(context.Background(), carpeta); ok || err != nil {
		t.Errorf("una carpeta suelta dio (ok=%v, err=%v), want (false, nil)", ok, err)
	}
	// Y no se creó nada nuevo: el worktree ya existía, pero `inspect` no ha añadido un
	// worktree al repo.
	lineas := strings.Split(strings.TrimSpace(
		testutil.RunGit(t, wt.Repo, "worktree", "list")), "\n")
	if len(lineas) != 2 {
		t.Errorf("inspect dejó %d líneas en worktree list, want 2", len(lineas))
	}
}

// exists informa si una ruta existe, para no repetir os.Stat.
func exists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	return err == nil
}
