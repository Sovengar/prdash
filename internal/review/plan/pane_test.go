package plan

import (
	"strconv"
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

// The pane env is the contract with the review tools.
func TestPaneEnvProvidesWhatItHasAndWhatItDoesNot(t *testing.T) {
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
			t.Errorf("%s is missing, and it is identity so it is always provided", k)
		}
	}
	for _, k := range []string{"PRDASH_BRANCH", "PRDASH_BASE", "PRDASH_URL"} {
		if v, ok := env[k]; ok {
			t.Errorf("%s = %q, want absent: the data does not exist and is not invented", k, v)
		}
	}
	for k, v := range env {
		if strings.HasSuffix(v, "=") {
			t.Errorf("%s travels empty: a variable without a value is a value, not an absence", k)
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

func TestToolEffectiveFallsBackToTheDefaultBinary(t *testing.T) {
	cases := []struct {
		name  string
		tool  Tool
		extra []string
		want  string
	}{
		{"empty argv uses the default", Tool{}, []string{"pr", "7"}, "default-bin pr 7"},
		{"own argv without extra", Tool{Argv: []string{"tuicr", "pr"}}, nil, "tuicr pr"},
		{"own argv with extra", Tool{Argv: []string{"hunk"}}, []string{"diff", "main...HEAD"}, "hunk diff main...HEAD"},
		{"override is verbatim", Tool{Override: true, Argv: []string{"my-script", "--yes", "complete"}}, []string{"must", "not", "appear"}, "my-script --yes complete"},
		{"empty override does not invent the default", Tool{Override: true}, []string{"nor-this"}, ""},
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
	got[0] = "other"
	if tool.Argv[0] != "tuicr" {
		t.Errorf("effective() returned an aliased slice: %v", tool.Argv)
	}
}

// An override with no argv has no executable, which means "omit the pane".
func TestToolBinaryDistinguishesAnEmptyOverrideFromAnEmptyBase(t *testing.T) {
	cases := []struct {
		name string
		tool Tool
		want string
	}{
		{"own argv", Tool{Argv: []string{"tuicr", "pr"}}, "tuicr"},
		{"empty base falls back to default", Tool{}, "default-bin"},
		{"empty override has no binary", Tool{Override: true}, ""},
		{"override with argv", Tool{Override: true, Argv: []string{"my-script", "--x"}}, "my-script"},
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
			t.Errorf("env variable without '=': %q", kv)
			continue
		}
		out[k] = v
	}
	return out
}
