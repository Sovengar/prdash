package sim

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestArgsPutGlobalsBeforeTheSubcommand: git-sim declara las opciones globales en
// el grupo raíz y las del subcomando no las aceptan, así que el orden del argv no
// es cosmético: es lo que decide si arranca o si aborta con el usage.
func TestArgsPutGlobalsBeforeTheSubcommand(t *testing.T) {
	r := NewRunner()
	got := r.args(Spec{Kind: KindMerge, Ref: "prdash/pr-7"}, "/tmp/media")

	want := []string{
		"--output-only-path",
		"--no-animate",
		"--media-dir", "/tmp/media",
		"merge", "prdash/pr-7",
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("args = %v, want %v", got, want)
	}
}

// TestArgsNeverAskForQuietAndThePath: git-sim imprime la ruta de la imagen solo
// cuando no está en silencio. Pedir las dos cosas a la vez —que es lo que parece
// lo razonable— deja la salida vacía y la simulación falla sin explicación. Este
// test lo fija para que nadie lo vuelva a "optimizar".
func TestArgsNeverAskForQuietAndThePath(t *testing.T) {
	for _, kind := range []Kind{KindMerge, KindRebase} {
		argv := strings.Join(NewRunner().args(Spec{Kind: kind, Ref: "x"}, "/tmp/m"), " ")
		if strings.Contains(argv, "--quiet") {
			t.Errorf("%s: el argv pide --quiet, que anula --output-only-path: %s", kind, argv)
		}
		if !strings.Contains(argv, "--output-only-path") {
			t.Errorf("%s: el argv no pide la ruta de la imagen: %s", kind, argv)
		}
	}
}

// TestArgsCarryTheRefOfEachKind: el ref no es la misma rama en merge que en
// rebase, y es quien lo pide quien lo decide.
func TestArgsCarryTheRefOfEachKind(t *testing.T) {
	r := NewRunner()
	if got := r.args(Spec{Kind: KindRebase, Ref: "main"}, "/tmp/m"); got[len(got)-2] != "rebase" {
		t.Errorf("subcomando = %q, want rebase", got[len(got)-2])
	}
	if got := r.args(Spec{Kind: KindRebase, Ref: "main"}, "/tmp/m"); got[len(got)-1] != "main" {
		t.Errorf("ref = %q, want main", got[len(got)-1])
	}
}

// TestEnvForcesNoAutoOpen: git-sim termina entregando la imagen al visor del
// escritorio y, sin display, esa llamada no vuelve. Como el flag no tiene forma
// negativa en el CLI, se apaga por entorno; y como Settings lee el primer valor
// que encuentra, hay que quitar el del usuario en vez de añadir otro detrás.
func TestEnvForcesNoAutoOpen(t *testing.T) {
	t.Setenv("git_sim_auto_open", "true")
	t.Setenv("git_sim_img_format", "png")

	var seen int
	for _, kv := range env() {
		switch kv {
		case "git_sim_auto_open=false":
			seen++
		case "git_sim_img_format=png":
			t.Error("env conserva un git_sim_* del usuario; el primero gana y anula el forzado")
		}
	}
	if seen != 1 {
		t.Errorf("git_sim_auto_open=false aparece %d veces, want 1", seen)
	}
}

// TestImagePathTakesTheLastNonEmptyLine: con --output-only-path la ruta es la
// única salida, pero un manim que se queja por stdout no debe desplazar el
// resultado.
func TestImagePathTakesTheLastNonEmptyLine(t *testing.T) {
	if got := imagePath("ruido\n/tmp/x.jpg\n\n"); got != "/tmp/x.jpg" {
		t.Errorf("imagePath = %q, want /tmp/x.jpg", got)
	}
	if got := imagePath("   \n"); got != "" {
		t.Errorf("imagePath = %q, want vacío", got)
	}
}

// TestRenderReturnsTheImagePath: el runner no interpreta la salida, solo la
// devuelve; lo que haga con ella es del servicio.
func TestRenderReturnsTheImagePath(t *testing.T) {
	dir := t.TempDir()
	bin := writeScript(t, dir, "git-sim", "#!/bin/sh\necho /tmp/media/img.jpg\n")

	r := &Runner{Bin: bin}
	got, err := r.Render(context.Background(), dir, "/tmp/media", Spec{Kind: KindMerge, Ref: "feat"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got != "/tmp/media/img.jpg" {
		t.Errorf("imagen = %q", got)
	}
}

// TestRenderFailsWithTheStderrOfGitSim: el mensaje que explica un fallo es el de
// git-sim, no el genérico del runtime, porque es el único que nombra la causa
// (una base que no está en el clon, un ref inválido).
func TestRenderFailsWithTheStderrOfGitSim(t *testing.T) {
	dir := t.TempDir()
	bin := writeScript(t, dir, "git-sim", "#!/bin/sh\necho \"'x' is not a valid Git ref\" >&2\nexit 1\n")

	r := &Runner{Bin: bin}
	_, err := r.Render(context.Background(), dir, "/tmp/media", Spec{Kind: KindMerge, Ref: "x"})
	if err == nil {
		t.Fatal("Render no falló")
	}
	var serr *Error
	if !asError(err, &serr) {
		t.Fatalf("err = %T, want *sim.Error", err)
	}
	if !strings.Contains(serr.Msg, "not a valid Git ref") {
		t.Errorf("Msg = %q, want el stderr de git-sim", serr.Msg)
	}
	if serr.ExitCode != 1 {
		t.Errorf("ExitCode = %d, want 1", serr.ExitCode)
	}
}

// TestRenderRejectsAnUnknownKind: un kind que no existe no puede convertirse en
// argv, y sin esta comprobación se leería como un binario.
func TestRenderRejectsAnUnknownKind(t *testing.T) {
	r := NewRunner()
	if _, err := r.Render(context.Background(), t.TempDir(), "/tmp/m", Spec{Kind: "cherry-pick", Ref: "x"}); err == nil {
		t.Fatal("Render aceptó un kind desconocido")
	}
	if _, err := r.Render(context.Background(), t.TempDir(), "/tmp/m", Spec{Kind: KindMerge}); err == nil {
		t.Fatal("Render aceptó un spec sin ref")
	}
}

// TestAvailableFollowsTheBinary: sin git-sim la acción tiene que poder avisar
// antes de abrir el popup, no fallar a mitad.
func TestAvailableFollowsTheBinary(t *testing.T) {
	missing := &Runner{Bin: "git-sim-que-no-existe", lookPath: func(string) (string, error) {
		return "", os.ErrNotExist
	}}
	if missing.Available() {
		t.Error("Available = true con un binario ausente")
	}

	here := &Runner{Bin: "git-sim", lookPath: func(name string) (string, error) { return "/usr/bin/" + name, nil }}
	if !here.Available() {
		t.Error("Available = false con un binario presente")
	}
}

// writeScript crea un ejecutable con cuerpo de shell y devuelve su ruta.
func writeScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}
