package plan

import (
	"strconv"
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

// The pane env is the contract with the review tools.
func TestPaneEnvAportaLoQueSeYLoQueNo(t *testing.T) {
	base := func() (model.Item, Worktree) {
		it := model.NewItem(model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget", Owner: "acme", Name: "widget"}, 7)
		it.SourceBranch = "feat/x"
		it.TargetBranch = "develop"
		it.URL = "https://github.com/acme/widget/pull/7"
		return it, Worktree{Path: "/wt/pr-7", Branch: "prdash/pr-7"}
	}

	it, wt := base()
	env := envMap(t, paneEnv(it, wt))
	for k, want := range map[string]string{
		"PRDASH_REPO":     "acme/widget",
		"PRDASH_NUMBER":   "7",
		"PRDASH_WORKTREE": "/wt/pr-7",
		"PRDASH_BRANCH":   "prdash/pr-7",
		"PRDASH_BASE":     "develop",
		"PRDASH_URL":      "https://github.com/acme/widget/pull/7",
	} {
		if env[k] != want {
			t.Errorf("%s = %q, want %q", k, env[k], want)
		}
	}
	if len(env) != 6 {
		t.Errorf("env = %d variables, want 6: %v", len(env), env)
	}

	bare := model.NewItem(model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget"}, 3)
	env = envMap(t, paneEnv(bare, Worktree{Path: "/wt/pr-3"}))
	for _, k := range []string{"PRDASH_REPO", "PRDASH_NUMBER", "PRDASH_WORKTREE"} {
		if _, ok := env[k]; !ok {
			t.Errorf("falta %s, que es identidad y siempre se aporta", k)
		}
	}
	for _, k := range []string{"PRDASH_BRANCH", "PRDASH_BASE", "PRDASH_URL"} {
		if v, ok := env[k]; ok {
			t.Errorf("%s = %q, want ausente: el dato no existe y no se inventa", k, v)
		}
	}
	for k, v := range env {
		if strings.HasSuffix(v, "=") {
			t.Errorf("%s viaja vacía: una variable sin valor es un valor, no una ausencia", k)
		}
	}
	if len(env) != 3 {
		t.Errorf("env = %d variables, want 3: %v", len(env), env)
	}

	env = envMap(t, paneEnv(bare, Worktree{Path: "/wt"}))
	if env["PRDASH_NUMBER"] != strconv.Itoa(bare.Number) {
		t.Errorf("PRDASH_NUMBER = %q, want %q", env["PRDASH_NUMBER"], strconv.Itoa(bare.Number))
	}
}

func TestToolEffectiveCaeAlBinarioPorDefecto(t *testing.T) {
	cases := []struct {
		name  string
		tool  Tool
		extra []string
		want  string
	}{
		{"argv vacío usa el default", Tool{}, []string{"pr", "7"}, "default-bin pr 7"},
		{"argv propio sin extra", Tool{Argv: []string{"tuicr", "pr"}}, nil, "tuicr pr"},
		{"argv propio con extra", Tool{Argv: []string{"hunk"}}, []string{"diff", "main...HEAD"}, "hunk diff main...HEAD"},
		{"override es verbatim", Tool{Override: true, Argv: []string{"mi-script", "--ya", "completo"}}, []string{"NO", "SE", "ANADE"}, "mi-script --ya completo"},
		{"override vacío no inventa el default", Tool{Override: true}, []string{"tampoco"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := strings.Join(c.tool.effective("default-bin", c.extra...), " "); got != c.want {
				t.Errorf("effective() = %q, want %q", got, c.want)
			}
		})
	}

	// The returned argv is not aliased to the Tool's: if the receiver mutates it, the change cannot
	// leak back.
	tool := Tool{Argv: []string{"tuicr"}}
	got := tool.effective("default-bin", "extra")
	got[0] = "otro"
	if tool.Argv[0] != "tuicr" {
		t.Errorf("effective() devolvió un slice aliasado: %v", tool.Argv)
	}
}

// An override with no argv has no executable, which means "omit the pane".
func TestToolBinaryDistingueOverrideVacioDeBaseVacio(t *testing.T) {
	cases := []struct {
		name string
		tool Tool
		want string
	}{
		{"argv propio", Tool{Argv: []string{"tuicr", "pr"}}, "tuicr"},
		{"base vacío al default", Tool{}, "default-bin"},
		{"override vacío no tiene binario", Tool{Override: true}, ""},
		{"override con argv", Tool{Override: true, Argv: []string{"mi-script", "--x"}}, "mi-script"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.tool.binary("default-bin"); got != c.want {
				t.Errorf("binary() = %q, want %q", got, c.want)
			}
		})
	}
}

func envMap(t *testing.T, env []string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, kv := range env {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			t.Errorf("variable de env sin '=': %q", kv)
			continue
		}
		out[k] = v
	}
	return out
}
