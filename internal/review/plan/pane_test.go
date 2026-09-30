package plan

import (
	"strconv"
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

// TestPaneEnvAportaLoQueSeYLoQueNo: el env del pane es el contrato con las
// herramientas de review (tuicr, hunk, el agente). Son datos del ítem y del
// worktree, y un pane que los recibe a medias —una variable que falta o una que
// viaja vacía— hace que la herramienta revise la cosa equivocada sin avisar.
//
// Las tres variables de identidad (repo, número y worktree) siempre se aportan:
// un pane sin ellas no sabe sobre qué está trabajando. Las de contexto (rama,
// base y URL) solo si el dato existe, y vacías no: `PRDASH_BRANCH=` se lee
// como "la rama es la cadena vacía", que es un valor, no una ausencia.
func TestPaneEnvAportaLoQueSeYLoQueNo(t *testing.T) {
	base := func() (model.Item, Worktree) {
		it := model.NewItem(model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget", Owner: "acme", Name: "widget"}, 7)
		it.SourceBranch = "feat/x"
		it.TargetBranch = "develop"
		it.URL = "https://github.com/acme/widget/pull/7"
		return it, Worktree{Path: "/wt/pr-7", Branch: "prdash/pr-7"}
	}

	// Con todo: las seis.
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

	// Sin rama, sin base y sin URL: solo las de identidad, y ninguna vacía
	// colgada (una variable presente y vacía se lee como un valor, no como una
	// ausencia).
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

	// Y el número va como número, no como el número de la struct formateado.
	env = envMap(t, paneEnv(bare, Worktree{Path: "/wt"}))
	if env["PRDASH_NUMBER"] != strconv.Itoa(bare.Number) {
		t.Errorf("PRDASH_NUMBER = %q, want %q", env["PRDASH_NUMBER"], strconv.Itoa(bare.Number))
	}
}

// TestToolEffectiveCaeAlBinarioPorDefecto: un Tool con argv vacío y sin override
// usa el binario por defecto, que es lo que hace que la config vacía funcione sin
// tocar nada. Y con override, el argv va verbatim: no se le añade nada, porque
// la config dice que ese argv es el completo.
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

	// Y el argv devuelto no se aliasa al del Tool: si quien lo recibe lo muta, no
	// puede cambiar el Tool de origen.
	tool := Tool{Argv: []string{"tuicr"}}
	got := tool.effective("default-bin", "extra")
	got[0] = "otro"
	if tool.Argv[0] != "tuicr" {
		t.Errorf("effective() devolvió un slice aliasado: %v", tool.Argv)
	}
}

// TestToolBinaryDistingueOverrideVacioDeBaseVacio: un override sin argv no tiene
// ejecutable, y eso significa "omite el pane". Un base sin argv sí tiene:
// significa "usa el default". Confundirlos convertiría "configurado como vacío"
// en "usa tuicr", que es lo contrario de lo que el usuario escribió.
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

// envMap convierte el env del pane en un mapa, y falla si algo no es "K=V".
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
