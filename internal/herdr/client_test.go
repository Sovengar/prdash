package herdr

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// fakeCLI graba las invocaciones y responde según una función, de modo que los
// tests no tocan el Herdr real.
type fakeCLI struct {
	env     map[string]string
	respond func(args []string) ([]byte, []byte, error)
	calls   [][]string
}

func (f *fakeCLI) client() *Client {
	return &Client{
		Bin:    "herdr",
		getenv: func(k string) string { return f.env[k] },
		execFn: func(_ context.Context, args ...string) ([]byte, []byte, error) {
			f.calls = append(f.calls, append([]string(nil), args...))
			return f.respond(args)
		},
	}
}

func (f *fakeCLI) called(prefix ...string) bool {
	for _, c := range f.calls {
		if len(c) >= len(prefix) {
			match := true
			for i := range prefix {
				if c[i] != prefix[i] {
					match = false
					break
				}
			}
			if match {
				return true
			}
		}
	}
	return false
}

func versionOK(_ []string) ([]byte, []byte, error) {
	return []byte("herdr 0.9.1-preview.2026-09-21-0f\n"), nil, nil
}

func TestAvailableRequiresHerdrEnv(t *testing.T) {
	f := &fakeCLI{env: map[string]string{}, respond: versionOK}
	if f.client().Available() {
		t.Fatal("sin HERDR_ENV no debería estar disponible")
	}
}

func TestAvailableRequiresMinVersion(t *testing.T) {
	f := &fakeCLI{env: map[string]string{"HERDR_ENV": "1"}, respond: func(args []string) ([]byte, []byte, error) {
		return []byte("herdr 0.8.2\n"), nil, nil
	}}
	if f.client().Available() {
		t.Fatal("0.8.2 no alcanza el mínimo 0.9.0")
	}
}

func TestAvailableWithSupportedVersion(t *testing.T) {
	f := &fakeCLI{env: map[string]string{"HERDR_ENV": "1"}, respond: versionOK}
	c := f.client()
	if !c.Available() {
		t.Fatal("dentro de Herdr con 0.9.1 debería estar disponible")
	}
	if v, ok := c.Version(); !ok || v.String() != "0.9.1" {
		t.Fatalf("version = %v ok=%v", v, ok)
	}
}

func TestAvailableToleratesUnknownVersion(t *testing.T) {
	f := &fakeCLI{env: map[string]string{"HERDR_ENV": "1"}, respond: func(args []string) ([]byte, []byte, error) {
		return []byte("herdr dev\n"), nil, nil
	}}
	if !f.client().Available() {
		t.Fatal("una versión ilegible no debería bloquear por drift")
	}
}

func TestWorktreeCreateBuildsArgsAndParses(t *testing.T) {
	f := &fakeCLI{env: map[string]string{"HERDR_ENV": "1"}, respond: func(args []string) ([]byte, []byte, error) {
		if len(args) > 0 && args[0] == "--version" {
			return []byte("herdr 0.9.1\n"), nil, nil
		}
		return []byte(fixtureWorktreeCreated), nil, nil
	}}
	info, err := f.client().WorktreeCreate(context.Background(), WorktreeSpec{
		Cwd: "/repo", Branch: "prdash/pr-7", Path: "/wt/prdash-pr-7", Label: "prdash-pr-7", NoFocus: true,
	})
	if err != nil {
		t.Fatalf("WorktreeCreate: %v", err)
	}
	if info.Path != "/home/u/.herdr/worktrees/prdash/feat-x" || info.WorkspaceID != "w18" {
		t.Fatalf("info = %+v", info)
	}
	var got []string
	for _, c := range f.calls {
		if c[0] == "worktree" {
			got = c
		}
	}
	for _, want := range []string{"--cwd", "/repo", "--branch", "prdash/pr-7", "--path", "/wt/prdash-pr-7", "--label", "prdash-pr-7", "--no-focus"} {
		if !contains(got, want) {
			t.Fatalf("faltó %q en %v", want, got)
		}
	}
}

func TestErrorIsTypedFromStderrJSON(t *testing.T) {
	f := &fakeCLI{env: map[string]string{"HERDR_ENV": "1"}, respond: func(args []string) ([]byte, []byte, error) {
		if len(args) > 0 && args[0] == "--version" {
			return []byte("herdr 0.9.1\n"), nil, nil
		}
		return nil, []byte(`{"error":{"code":"worktree_create_failed","message":"branch already checked out"}}`), errors.New("exit status 1")
	}}
	_, err := f.client().WorktreeCreate(context.Background(), WorktreeSpec{Cwd: "/repo", Branch: "x"})
	if err == nil {
		t.Fatal("esperaba error")
	}
	var herr *Error
	if !errors.As(err, &herr) {
		t.Fatalf("error no tipado: %T", err)
	}
	if herr.Code != "worktree_create_failed" || !strings.Contains(herr.Msg, "already checked out") {
		t.Fatalf("error = %+v", herr)
	}
}

func TestPaneRunJoinsArgv(t *testing.T) {
	f := &fakeCLI{env: map[string]string{"HERDR_ENV": "1"}, respond: func(args []string) ([]byte, []byte, error) {
		if len(args) > 0 && args[0] == "--version" {
			return []byte("herdr 0.9.1\n"), nil, nil
		}
		return []byte(`{"id":"cli:pane:run","result":{"type":"pane_run"}}`), nil, nil
	}}
	if err := f.client().PaneRun(context.Background(), "w18:p1", []string{"tuicr", "pr", "https://github.com/o/r/pull/7"}); err != nil {
		t.Fatalf("PaneRun: %v", err)
	}
	want := []string{"pane", "run", "w18:p1", "tuicr pr https://github.com/o/r/pull/7"}
	if len(f.calls) == 0 || !equalSlices(f.calls[len(f.calls)-1], want) {
		t.Fatalf("llamada = %v, want %v", f.calls, want)
	}
}

func TestNotifyBuildsArgs(t *testing.T) {
	f := &fakeCLI{env: map[string]string{"HERDR_ENV": "1"}, respond: func(args []string) ([]byte, []byte, error) {
		if len(args) > 0 && args[0] == "--version" {
			return []byte("herdr 0.9.1\n"), nil, nil
		}
		return []byte(fixtureNotification), nil, nil
	}}
	if err := f.client().Notify(context.Background(), "review listo", NotifyOptions{Sound: "done"}); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	last := f.calls[len(f.calls)-1]
	if !contains(last, "--sound") || !contains(last, "done") {
		t.Fatalf("notify args = %v", last)
	}
}

func TestPaneWaitOutputPassesTimeoutMillis(t *testing.T) {
	f := &fakeCLI{env: map[string]string{"HERDR_ENV": "1"}, respond: func(args []string) ([]byte, []byte, error) {
		if len(args) > 0 && args[0] == "--version" {
			return []byte("herdr 0.9.1\n"), nil, nil
		}
		return []byte(`{"id":"cli:pane:wait-output","result":{"type":"pane_output_matched"}}`), nil, nil
	}}
	if err := f.client().PaneWaitOutput(context.Background(), "w18:p1", "ready", 2*time.Second); err != nil {
		t.Fatalf("PaneWaitOutput: %v", err)
	}
	last := f.calls[len(f.calls)-1]
	if !contains(last, "--match") || !contains(last, "2000") {
		t.Fatalf("wait-output args = %v", last)
	}
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
