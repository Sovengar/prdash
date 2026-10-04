package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"prdash/internal/worktree"
)

// This file exists because the two functions wrote to a fixed os.Stdout, and without an
// io.Writer in the path the only way to read the output was replacing the process's files.

func TestListarSeparaElResultadoDelAvisoYCadaCosaEnSuCanal(t *testing.T) {
	base, orphans, healthy, foreign := worktreeOrphanFixture(t)
	pr := worktree.NewGitDirect(base)

	var stdout, stderr bytes.Buffer
	if code := listWorktrees(pr, &stdout, &stderr); code != 0 {
		t.Fatalf("código = %d, stderr = %q", code, stderr.String())
	}

	tabla := stdout.String()
	for _, quiere := range []string{"WORKTREE", "RAMA", "ESTADO", "RUTA"} {
		if !strings.Contains(tabla, quiere) {
			t.Errorf("la tabla no tiene la columna %q:\n%s", quiere, tabla)
		}
	}
	for _, e := range append(append([]string{}, orphans...), healthy) {
		if !strings.Contains(tabla, e) {
			t.Errorf("la tabla no lista %s:\n%s", e, tabla)
		}
	}
	if strings.Contains(tabla, foreign) {
		t.Errorf("la tabla menciona el worktree ajeno %s:\n%s", foreign, tabla)
	}
	if strings.Contains(stderr.String(), foreign) {
		t.Errorf("stderr menciona el worktree ajeno %s", foreign)
	}

	avisos := stderr.String()
	for _, o := range orphans {
		if !strings.Contains(avisos, o) {
			t.Errorf("stderr no avisa del huérfano %s:\n%s", o, avisos)
		}
		if !strings.Contains(avisos, "orphaned") && !strings.Contains(avisos, "is orphaned") {
			t.Errorf("el aviso de %s no dice que está huérfano:\n%s", o, avisos)
		}
	}
	if strings.Contains(avisos, healthy) {
		t.Errorf("stderr avisa del worktree sano %s:\n%s", healthy, avisos)
	}
}

// The tabwriter is the only thing between the output and something an eye can read, and it is not
// cosmetic.
func TestListarAlineaLasColumnasParaQueLaTablaSeLea(t *testing.T) {
	base, _, _ := worktreeFixture(t)
	pr := worktree.NewGitDirect(base)

	var stdout, stderr bytes.Buffer
	listWorktrees(pr, &stdout, &stderr)

	lineas := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
	if len(lineas) < 2 {
		t.Fatalf("salida con %d líneas, no hay tabla que alinear:\n%s", len(lineas), stdout.String())
	}

	posicionesCabecera := columnasDe(lineas[0])
	if len(posicionesCabecera) != 4 {
		t.Fatalf("la cabecera tiene %d columnas, want 4: %q", len(posicionesCabecera), lineas[0])
	}
	for _, fila := range lineas[1:] {
		for i, col := range columnasDe(fila) {
			if i >= len(posicionesCabecera) {
				t.Errorf("la fila %q tiene más columnas que la cabecera", fila)
				break
			}
			if col != posicionesCabecera[i] {
				t.Errorf("la columna %d de %q empieza en %d y la de la cabecera en %d: la "+
					"tabla no está alineada", i, fila, col, posicionesCabecera[i])
			}
		}
	}
}

// The tabwriter pads with spaces, so the next column starts exactly where this says.
func columnasDe(fila string) []int {
	out := []int{0}
	i := 0
	for i < len(fila) {
		if fila[i] != ' ' {
			i++
			continue
		}
		inicio := i
		for i < len(fila) && fila[i] == ' ' {
			i++
		}
		if i >= len(fila) {
			break
		}
		out = append(out, inicio+(i-inicio))
	}
	return out
}

// A case with content, not a silent return.
func TestSinWorktreesElListadoDiceQueNoHayYNoEsUnError(t *testing.T) {
	pr := worktree.NewGitDirect(t.TempDir())

	var stdout, stderr bytes.Buffer
	if code := listWorktrees(pr, &stdout, &stderr); code != 0 {
		t.Errorf("código = %d sin worktrees, want 0", code)
	}
	if !strings.Contains(stdout.String(), "no prdash review worktrees") {
		t.Errorf("no dice que no hay worktrees: %q", stdout.String())
	}
	if stderr.String() != "" {
		t.Errorf("stderr = %q sin worktrees, want vacío", stderr.String())
	}
}

// The property to pin is touching nothing, and it is checked by the tree being byte-identical.
func TestBorrarEnSecoDiceQueIriaABorrarYTocanNada(t *testing.T) {
	base, orphans, healthy, foreign := worktreeOrphanFixture(t)
	pr := worktree.NewGitDirect(base)

	var stdout, stderr bytes.Buffer
	code := removeWorkphans(pr, &stdout, &stderr, true)
	if code != 0 {
		t.Fatalf("código = %d, stderr = %q", code, stderr.String())
	}
	out := stdout.String()
	for _, o := range orphans {
		if !strings.Contains(out, "would remove: "+o) {
			t.Errorf("no dice que borraría %s:\n%s", o, out)
		}
		if strings.Contains(out, "worktree removed: "+o) {
			t.Errorf("una prueba seca dice que lo BORRÓ: %s", o)
		}
		if !worktree.Exists(o) {
			t.Errorf("--dry-run borró %s", o)
		}
	}
	if !worktree.Exists(healthy) || !worktree.Exists(foreign) {
		t.Error("--dry-run tocó el worktree sano o el ajeno")
	}
}

// The "writes nothing to stderr" assertion is what makes it a test: an empty batch is the happy
// path and must be quiet.
func TestSinHuérfanosElBorradoEnLoteDiceQueNoHayYNoEscribeEnStderr(t *testing.T) {
	base, _, _ := worktreeFixture(t) // sano + ajeno, sin huérfanos
	pr := worktree.NewGitDirect(base)

	for _, dryRun := range []bool{false, true} {
		var stdout, stderr bytes.Buffer
		if code := removeOrphans(pr, &stdout, &stderr, dryRun, worktreeTimeout); code != 0 {
			t.Errorf("dryRun=%v: código = %d, cero huérfanos es el caso feliz", dryRun, code)
		}
		if !strings.Contains(stdout.String(), "no orphaned prdash worktrees") {
			t.Errorf("dryRun=%v: no dice que no hay huérfanos: %q", dryRun, stdout.String())
		}
		if stderr.String() != "" {
			t.Errorf("dryRun=%v: stderr = %q con cero huérfanos, want vacío",
				dryRun, stderr.String())
		}
	}
}

// The most important case in the command, because it decides whether a user's work survives a
// partly-failing batch.
func TestRutasRechazadasYTocadasVanAStderrYElCodigoDeSaltaPeroElRestoSeBorra(t *testing.T) {
	base, orphans, healthy, foreign := worktreeOrphanFixture(t)
	pr := worktree.NewGitDirect(base)
	// User work that is NOT prdash's but sits under the root with the prefix's name: the case an
	//`rm -rf` would take with it.
	ajeno := filepath.Join(base, "prdash-mio-del-usuario")
	if err := os.MkdirAll(ajeno, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ajeno, "trabajo.txt"),
		[]byte("no me borres"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := removeWorktreesWithin(pr, &stdout, &stderr, false, false,
		[]string{orphans[0], foreign, "/no/existe/en/absoluto", orphans[1]}, 10*time.Second)

	if code != 1 {
		t.Errorf("código = %d con dos rutas rechazadas, want 1", code)
	}
	errOut := stderr.String()
	for _, rechazada := range []string{foreign, "/no/existe/en/absoluto"} {
		if !strings.Contains(errOut, rechazada) {
			t.Errorf("stderr no nombra la ruta rechazada %s:\n%s", rechazada, errOut)
		}
		if !strings.Contains(errOut, "leaving it alone") {
			t.Errorf("el rechazo de %s no dice que se la deja intacta:\n%s", rechazada, errOut)
		}
	}
	out := stdout.String()
	for _, o := range orphans {
		if !strings.Contains(out, "worktree removed: "+o) {
			t.Errorf("stdout no dice que se borró %s:\n%s", o, out)
		}
		if worktree.Exists(o) {
			t.Errorf("%s sigue ahí después de borrarlo", o)
		}
	}
	// The "looks like prdash" name is the case Owned has to recognise by the label and not by the
	//directory name.
	for _, intacto := range []string{foreign, healthy} {
		if !worktree.Exists(intacto) {
			t.Errorf("%s se borró, y no era suyo", intacto)
		}
	}
	if _, err := os.Stat(filepath.Join(ajeno, "trabajo.txt")); err != nil {
		t.Errorf("borró el trabajo del usuario que solo se parecia a un worktree: %v", err)
	}
}

func TestElDespachoEscribeCadaErrorEnSuCanal(t *testing.T) {
	base, _, healthy, foreign := worktreeOrphanFixture(t)
	pr := worktree.NewGitDirect(base)

	for _, c := range []struct {
		nombre  string
		args    []string
		want    int
		aStderr string
		aStdout string
	}{
		{"subcomando desconocido", []string{"inventado"}, 2, "unknown subcommand", ""},
		{"list sin provisioner de nada", []string{"list"}, 0, "", "WORKTREE"},
		{"remove sin rutas", []string{"remove"}, 2, "missing at least one path", ""},
		{"flag desconocido", []string{"remove", "--inventado"}, 2, "unknown flag", ""},
		{"typo de orphans", []string{"remove", "--orphan"}, 2, "unknown flag", ""},
		{"dry-run sin orphans", []string{"remove", "--dry-run"}, 2, "requires --orphans", ""},
		{"los dos modos mezclados", []string{"remove", "--orphans", "x"}, 2, "cannot be mixed", ""},
		{"ruta con guion inicial", []string{"remove", "-prdash-1"}, 2, "unknown flag", ""},
		{"subcomando vacío", nil, 0, "", "WORKTREE"},
	} {
		var stdout, stderr bytes.Buffer
		code := runWorktrees(pr, &stdout, &stderr, c.args)
		if code != c.want {
			t.Errorf("%s: código = %d, want %d (stderr: %s)", c.nombre, code, c.want, stderr.String())
		}
		if c.aStderr != "" && !strings.Contains(stderr.String(), c.aStderr) {
			t.Errorf("%s: stderr = %q, want que contenga %q", c.nombre, stderr.String(), c.aStderr)
		}
		if c.aStdout != "" && !strings.Contains(stdout.String(), c.aStdout) {
			t.Errorf("%s: stdout = %q, want que contenga %q", c.nombre, stdout.String(), c.aStdout)
		}
		if code == 2 && stdout.String() != "" {
			t.Errorf("%s: un error de uso escribió en stdout: %q", c.nombre, stdout.String())
		}
	}

	var stdout, stderr bytes.Buffer
	if code := runWorktrees(pr, &stdout, &stderr, []string{"remove", "--orphans", "--dry-run"}); code != 0 {
		t.Errorf("un lote en seco bien formado dio código %d", code)
	}
	if !worktree.Exists(healthy) || !worktree.Exists(foreign) {
		t.Error("el lote en seco tocó algo: Exists en false significa que ya no está")
	}
}

// Not worktreeTimeout (30s), because a failing test would then hang for 30s.
func removeWorkphans(pr worktree.Provisioner, stdout, stderr io.Writer, dryRun bool) int {
	return removeOrphans(pr, stdout, stderr, dryRun, 10*time.Second)
}
