package gitcmd

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Three pieces, not one, because each answers a different question about a git failure.
func TestElMensajeDeErrorTraeLoQueHaceFaltaParaArreglarlo(t *testing.T) {
	casos := []struct {
		nombre string
		e      *Error
		want   string
	}{
		{
			nombre: "con directorio y codigo",
			e:      &Error{Args: []string{"rev-parse", "HEAD"}, Dir: "/repos/proy", Msg: "fatal: not a git repository", ExitCode: 128},
			want:   "git -C /repos/proy rev-parse HEAD: fatal: not a git repository (exit 128)",
		},
		{
			nombre: "sin directorio",
			e:      &Error{Args: []string{"status"}, Msg: "no changes", ExitCode: 1},
			want:   "git status: no changes (exit 1)",
		},
		{
			nombre: "error sin codigo",
			e:      &Error{Args: []string{"log"}, Msg: "interrumpido"},
			want:   "git log: interrumpido",
		},
		{
			nombre: "sin argumentos",
			e:      &Error{Msg: "algo"},
			want:   "git : algo",
		},
		{
			nombre: "varios argumentos",
			e:      &Error{Args: []string{"worktree", "add", "-b", "b", "/r"}, Msg: "ya existe"},
			want:   "git worktree add -b b /r: ya existe",
		},
	}
	for _, c := range casos {
		if got := c.e.Error(); got != c.want {
			t.Errorf("%s: Error() dio %q, want %q", c.nombre, got, c.want)
		}
	}
}

// Unwrap is what lets us say "this is not git's error": errors.As(err, &exitError) needs the
// chain mounted.
func TestElErrorDesenvuelveLaCausa(t *testing.T) {
	sinCausa := &Error{Args: []string{"x"}, Msg: "m"}
	if sinCausa.Unwrap() != nil {
		t.Error("Unwrap devolvio algo en un Error sin causa")
	}
	if errors.Unwrap(sinCausa) != nil {
		t.Error("errors.Unwrap devolvio algo en un Error sin causa")
	}

	cmd := exec.Command("sh", "-c", "exit 42")
	err := cmd.Run()
	if err == nil {
		t.Fatal("esperaba que sh -c 'exit 42' fallara")
	}
	conCausa := &Error{Args: []string{"x"}, Msg: "m", Err: err}

	if !errors.Is(conCausa, err) {
		t.Error("errors.Is no llega a la causa")
	}
	var exit *exec.ExitError
	if !errors.As(conCausa, &exit) {
		t.Fatal("errors.As no llega al *exec.ExitError: sin esto ExitCode es 0 siempre")
	}
	if exit.ExitCode() != 42 {
		t.Errorf("ExitCode = %d, want 42", exit.ExitCode())
	}
	if !strings.Contains(conCausa.Error(), "m") {
		t.Errorf("el mensaje perdió el texto propio: %q", conCausa.Error())
	}
}

func TestRunTraeElErrorRealDeGit(t *testing.T) {
	dir := repoVacio(t)

	_, err := New().Run(context.Background(), dir, "cat-file", "-p", "no-existe")
	if err == nil {
		t.Fatal("git cat-file sobre un objeto inexistente dio nil")
	}
	gerr, ok := err.(*Error)
	if !ok {
		t.Fatalf("Run devolvió %T, want *Error", err)
	}
	if gerr.ExitCode == 0 {
		t.Error("ExitCode = 0 en un fallo de git: el código de salida no se leyó")
	}
	if strings.HasPrefix(gerr.Msg, "exit status") {
		t.Errorf("el mensaje es %q: se cogió el error del proceso en vez de su stderr", gerr.Msg)
	}
	if strings.TrimSpace(gerr.Msg) == "" {
		t.Error("el mensaje quedó vacío")
	}
	if strings.Contains(gerr.Msg, "\n") {
		t.Errorf("el mensaje tiene saltos de línea: %q", gerr.Msg)
	}
	if !strings.Contains(gerr.Error(), "cat-file") {
		t.Errorf("Error() no nombra el comando: %q", gerr.Error())
	}
	if !strings.Contains(gerr.Error(), dir) {
		t.Errorf("Error() no nombra el directorio: %q", gerr.Error())
	}
}

// Returning what was written before the failure is the right behaviour for what prdash does with
// git.
func TestRunDevuelveLaSalidaParcialCuandoFalla(t *testing.T) {
	dir := t.TempDir()
	parcial := filepath.Join(dir, "git-parcial")
	if err := os.WriteFile(parcial, []byte("#!/bin/sh\necho 'linea buena'\necho 'fatal: se rompio' >&2\nexit 7\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	out, err := (&Runner{Bin: parcial}).Run(context.Background(), dir, "log")
	if err == nil {
		t.Fatal("un binario que sale con 7 dio nil")
	}
	if !strings.Contains(out, "linea buena") {
		t.Errorf("la salida anterior al fallo se perdió: %q", out)
	}
	gerr, ok := err.(*Error)
	if !ok {
		t.Fatalf("Run devolvió %T, want *Error", err)
	}
	if gerr.ExitCode != 7 {
		t.Errorf("ExitCode = %d, want 7", gerr.ExitCode)
	}
	if !strings.Contains(gerr.Msg, "se rompio") {
		t.Errorf("el mensaje no trae el stderr: %q", gerr.Msg)
	}

	if err := os.WriteFile(parcial, []byte("#!/bin/sh\nexit 7\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, err = (&Runner{Bin: parcial}).Run(context.Background(), dir, "log")
	if err == nil {
		t.Fatal("un binario que sale con 7 dio nil")
	}
	if out != "" {
		t.Errorf("un binario que no escribió devolvió %q", out)
	}
	if strings.TrimSpace(gerr.Msg) == "" {
		t.Error("el mensaje del primer caso quedó vacío")
	}
}

func TestElCaminoFelizTraeLaSalidaEntera(t *testing.T) {
	dir := repoVacio(t)
	r := New()

	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hola\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), dir, "add", "a.txt"); err != nil {
		t.Fatalf("git add: %v", err)
	}
	out, err := r.Run(context.Background(), dir, "diff", "--cached", "--stat")
	if err != nil {
		t.Fatalf("git diff --cached: %v", err)
	}
	if !strings.Contains(out, "a.txt") {
		t.Errorf("la salida de git no llegó intacta: %q", out)
	}
}

// The order matters: the width trim first, then the first-line cut.
func TestFirstLineCortaLoQueNoCabeEnElToast(t *testing.T) {
	casos := []struct {
		entrada string
		want    string
	}{
		{"primera\nsegunda\ntercera", "primera"},
		{"  con espacios  \nsegunda", "  con espacios  "},
		{"una sola linea", "una sola linea"},
		{"", ""},
		{"\nempieza con salto", ""},
		{"con\n", "con"},
		{"trailing  \n", "trailing  "},
	}
	for _, c := range casos {
		if got := firstLine(c.entrada); got != c.want {
			t.Errorf("firstLine(%q) dio %q, want %q", c.entrada, got, c.want)
		}
		if strings.ContainsAny(firstLine(c.entrada), "\n") {
			t.Errorf("firstLine(%q) devolvio texto con salto", c.entrada)
		}
	}
}

// The most important thing Env does.
func TestElEntornoDeGitNoHeredaelContextoDelShell(t *testing.T) {
	hostiles := map[string]string{
		"GIT_DIR":                          "/otro/repo/.git",
		"GIT_WORK_TREE":                    "/otro/repo",
		"GIT_INDEX_FILE":                   "/otro/index",
		"GIT_COMMON_DIR":                   "/otro/common",
		"GIT_OBJECT_DIRECTORY":             "/otro/objs",
		"GIT_ALTERNATE_OBJECT_DIRECTORIES": "/otro/alt",
		"GIT_NAMESPACE":                    "otro",
		"GIT_CEILING_DIRECTORIES":          "/otro/techo",
		"GIT_PREFIX":                       "src/",
		"LC_ALL":                           "es_ES.UTF-8",
		"LANG":                             "es_ES.UTF-8",
		"LANGUAGE":                         "es",
		"LC_MESSAGES":                      "es_ES.UTF-8",
	}
	for k, v := range hostiles {
		t.Setenv(k, v)
	}

	env := Env()
	vistos := map[string]string{}
	for _, kv := range env {
		if i := strings.IndexByte(kv, '='); i > 0 {
			vistos[kv[:i]] = kv[i+1:]
		}
	}

	for k := range hostiles {
		if _, sigue := vistos[k]; sigue {
			if k == "LC_ALL" || k == "LANG" {
				continue
			}
			t.Errorf("%s sigue en el entorno con el valor %q: git operaría en ese "+
				"contexto en vez del que se le pide", k, vistos[k])
		}
	}

	for k, want := range map[string]string{
		"LC_ALL":              "C",
		"GIT_TERMINAL_PROMPT": "0",
		"GIT_PAGER":           "cat",
		"NO_COLOR":            "1",
	} {
		if vistos[k] != want {
			t.Errorf("%s = %q, want %q", k, vistos[k], want)
		}
	}
	if _, sigue := vistos["LANG"]; sigue {
		t.Error("LANG sigue presente: compite con el LC_ALL que se pone")
	}

	t.Setenv("PRDASH_TEST_QUE_SI_SE_CONSERVA", "valor")
	vistos = map[string]string{}
	for _, kv := range Env() {
		if i := strings.IndexByte(kv, '='); i > 0 {
			vistos[kv[:i]] = kv[i+1:]
		}
	}
	if vistos["PRDASH_TEST_QUE_SI_SE_CONSERVA"] != "valor" {
		t.Error("Env tiró una variable que no debía: el entorno se filtra de más")
	}
}

func TestElTimeoutDeRunCortaDeVerdad(t *testing.T) {
	dir := t.TempDir()
	colgado := filepath.Join(dir, "git-colgado")
	script := "#!/bin/sh\nsh -c 'sleep 5' &\nsleep 5\n"
	if err := os.WriteFile(colgado, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	r := &Runner{Bin: colgado, Timeout: 50 * time.Millisecond}
	inicio := time.Now()
	_, err := r.Run(context.Background(), dir, "status")
	elapsed := time.Since(inicio)

	if err == nil {
		t.Error("un git colgado dio nil")
	}
	if elapsed > 2*time.Second {
		t.Errorf("Run tardó %s con un timeout de 50ms: el contexto no corta la llamada, "+
			"solo mata al proceso", elapsed)
	}

	corto := filepath.Join(dir, "git-corto")
	if err := os.WriteFile(corto, []byte("#!/bin/sh\nsleep 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	r = &Runner{Bin: corto}
	inicio = time.Now()
	if _, err := r.Run(context.Background(), dir, "status"); err != nil {
		t.Errorf("con el suelo por defecto y un binario sano dio error: %v", err)
	}
	if elapsed := time.Since(inicio); elapsed < 900*time.Millisecond {
		t.Errorf("con Timeout vacío tardó %s: se aplicó un timeout corto en vez del suelo",
			elapsed)
	}
}

// A name and not a resolved path, so the Runner normalises it.
func TestElBinarioPorDefectoEsGitYNoElCampoVacio(t *testing.T) {
	dir := repoVacio(t)
	if _, err := (&Runner{}).Run(context.Background(), dir, "rev-parse", "--git-dir"); err != nil {
		t.Fatalf("sin Bin, Run falló: %v", err)
	}
	if got := New().Bin; got != "git" {
		t.Errorf("New().Bin = %q, want git", got)
	}
	if got := New().Timeout; got != DefaultTimeout {
		t.Errorf("New().Timeout = %v, want DefaultTimeout", got)
	}
}

func repoVacio(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	r := New()
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "t@example.com"},
		{"config", "user.name", "Test"},
	} {
		if _, err := r.Run(context.Background(), dir, args...); err != nil {
			t.Fatalf("git %s: %v", strings.Join(args, " "), err)
		}
	}
	return dir
}
