package tui

import (
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

// TestMergeConfirmNombraLosModosQueElRepoAdmite: la Confirmación de merge es el
// aviso que el usuario tiene que leer ANTES de la segunda pulsación, y su
// segunda tecla ES el modo. Por eso no hay estrategia por defecto: todas las que
// se listan tienen que ser las que el repositorio admite, y si el repositorio
// no admite ninguna, la caja lo dice en vez de ofrecer teclas que solo van a
// fallar.
func TestMergeConfirmNombraLosModosQueElRepoAdmite(t *testing.T) {
	base := func(t *testing.T, rules model.MergeRules) Model {
		m2 := newTestModel(t, ghAdapter())
		it := mkItem("github", "github.com", "acme/widget", "Uno", 1, "")
		it.Merge = rules
		m2 = send(t, m2, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{it}, false))
		return m2
	}

	// Los tres modos: las tres teclas.
	m := base(t, model.MergeRulesAll())
	txt := stripANSI(m.mergeConfirmText())
	for _, want := range []string{"press the mode", "esc cancel"} {
		if !strings.Contains(txt, want) {
			t.Errorf("la caja debería decir %q: %q", want, txt)
		}
	}
	// Los tres modos con su tecla, y el separador entre cada uno.
	for _, want := range []string{"r rebase", "m merge commit", "s squash"} {
		if !strings.Contains(txt, want) {
			t.Errorf("con los tres modos debería salir %q: %q", want, txt)
		}
	}
	if n := strings.Count(txt, "·"); n != 3 {
		t.Errorf("con tres modos hay tres separadores, hay %d: %q", n, txt)
	}

	// Un solo modo conocido: SOLO ese. GitHub publica los tres flags en la
	// misma consulta del ítem, así que la lista es exacta: ofrecer squash en un
	// repositorio que lo tiene desactivado gasta una llamada en un rechazo que ya
	// se sabía.
	m = base(t, model.MergeRules{Known: true, MergeCommit: true})
	txt = stripANSI(m.mergeConfirmText())
	if !strings.Contains(txt, "m merge commit") {
		t.Errorf("con un solo modo conocido debería salir ese: %q", txt)
	}
	for _, no := range []string{"rebase", "squash"} {
		if strings.Contains(txt, no) {
			t.Errorf("un repositorio que solo admite merge commit no debería ofrecer %q: %q", no, txt)
		}
	}

	// Reglas desconocidas: se ofrecen las tres, porque no saber no es lo mismo que
	// no permitir (GitLab no expone los flags por GraphQL).
	m = base(t, model.MergeRules{})
	txt = stripANSI(m.mergeConfirmText())
	for _, want := range []string{"rebase", "merge commit", "squash"} {
		if !strings.Contains(txt, want) {
			t.Errorf("sin reglas conocidas deberían ofrecerse las tres: %q", txt)
		}
	}

	// Ninguno: la caja lo dice y NO ofrece teclas de modo. Ofrecerlas sería
	// prometer una acción que va a fallar.
	m = base(t, model.MergeRules{Known: true})
	txt = stripANSI(m.mergeConfirmText())
	if !strings.Contains(txt, "the repository allows no merge strategy") {
		t.Errorf("un repo sin estrategias debería decirlo: %q", txt)
	}
	// Y la caja nombra el ítem y el borrado, que son las otras dos decisiones.
	m = base(t, model.MergeRulesAll())
	txt = stripANSI(m.mergeConfirmText())
	if !strings.Contains(txt, "merge acme") {
		t.Errorf("la caja debería nombrar el ítem: %q", txt)
	}
	if !strings.Contains(txt, "delete") {
		t.Errorf("la caja debería decir si se borra la rama: %q", txt)
	}
}

// TestMergeConfirmConBloqueoBlandoLoDiceAntes: un bloqueo blando (CI inestable,
// por ejemplo) no es un veto: la segunda pulsación sigue valiendo. Pero el aviso
// tiene que estar ANTES de las teclas, porque quien lee la caja completa decide
// si pulsa o no. Y cambia el texto de la caja, no solo lo añade: sin bloqueo dice
// "press the mode" y con bloqueo dice "press the mode anyway".
func TestMergeConfirmConBloqueoBlandoLoDiceAntes(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	it := mkItem("github", "github.com", "acme/widget", "Uno", 1, "")
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{it}, false))

	sinBloqueo := stripANSI(m.mergeConfirmText())
	if strings.Contains(sinBloqueo, "anyway") {
		t.Errorf("sin bloqueo no debería decir \"anyway\": %q", sinBloqueo)
	}

	m.mergeBlockReason = "CI is failing (2 of 5 checks)"
	conBloqueo := stripANSI(m.mergeConfirmText())
	if !strings.Contains(conBloqueo, "CI is failing (2 of 5 checks)") {
		t.Errorf("el aviso del bloqueo debería salir: %q", conBloqueo)
	}
	if !strings.Contains(conBloqueo, "anyway") {
		t.Errorf("con bloqueo blando debería decir que se puede pulsar igual: %q", conBloqueo)
	}
	// Y los modos siguen listados: el bloqueo no quita opciones.
	if !strings.Contains(conBloqueo, "esc cancel") {
		t.Errorf("con bloqueo sigue habiendo salida: %q", conBloqueo)
	}
	// El motivo va antes de las teclas, que es donde está la decisión.
	if strings.Index(conBloqueo, "CI is failing") > strings.Index(conBloqueo, "press the mode") {
		t.Errorf("el aviso debería ir antes de las teclas: %q", conBloqueo)
	}
}

// TestSectionPrefixNoSeComeLaUltimaParteDeLaRuta: el prefijo de la columna ITEM
// es el directorio que comparten TODOS los ítems de la sección, y nunca el
// último segmento. Comérselo dejaría la celda ITEM con "#42" y sin decir de qué
// repo es, que es justo el dato que hace falta para distinguir dos ítems del
// mismo grupo.
//
// El mínimo de segmentos se lleva el más corto: con "grp/proy" y "grp/sub/proy"
// el común es "grp", y quedarse con el de la primeraRoutes daría "grp/proy",
// que no es común.
func TestSectionPrefixNoSeComeLaUltimaParteDeLaRuta(t *testing.T) {
	cases := []struct {
		name  string
		projs []string
		want  string
	}{
		{"mismo repo", []string{"acme/widget", "acme/widget"}, "acme"},
		{"grupo comun", []string{"grp/proj", "grp/otro"}, "grp"},
		{"grupo comun de tres", []string{"a/b", "a/c", "a/d"}, "a"},
		{"subgrupos distintos", []string{"grp/sub/proj", "grp/otro/proj"}, "grp"},
		{"grupos distintos", []string{"acme/widget", "otro/widget"}, ""},
		{"uno sin barra", []string{"widget", "acme/widget"}, ""},
		{"solo uno", []string{"acme/widget"}, ""},
		{"nada en comun con tres", []string{"a/x", "b/y", "c/z"}, ""},
		{"solo la hoja igual", []string{"x/widget", "y/widget"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			items := make([]model.Item, 0, len(c.projs))
			for i, proj := range c.projs {
				items = append(items, model.NewItem(model.RepoRef{Forge: "github", Host: "github.com", Project: proj}, i+1))
			}
			if got := sectionPrefix(items); got != c.want {
				t.Errorf("sectionPrefix(%v) = %q, want %q", c.projs, got, c.want)
			}
		})
	}
	// Y con un solo proyecto, o con ninguno, no hay prefijo: no hay nada común
	// que destacar.
	if got := sectionPrefix(nil); got != "" {
		t.Errorf("sectionPrefix(nil) = %q, want \"\"", got)
	}
	if got := sectionPrefix([]model.Item{}); got != "" {
		t.Errorf("sectionPrefix([]) = %q, want \"\"", got)
	}
}

// TestWrapHintAcotaElNumeroDeLineas: los hints son la ayuda y no pueden comerse la
// pantalla. Se acotan al ancho Y al número máximo de líneas, y el recorte tira
// por el final, que es donde está lo que config.Hints() puso al final a propósito.
func TestWrapHintAcotaElNumeroDeLineas(t *testing.T) {
	plano := func(s string) string { return stripANSI(s) }

	// Con un solo hint: una línea, pintada.
	got := wrapHint("uno", 80, plano)
	if len(got) != 1 || got[0] != "uno" {
		t.Errorf("un hint dio %q, want [\"uno\"]", got)
	}

	// Con el ancho justo: no se parte de más.
	for _, w := range []int{3, 4, 10} {
		got = wrapHint("uno dos", w, plano)
		for _, l := range got {
			if len(l) > w {
				t.Errorf("ancho %d: la línea %q se pasa", w, l)
			}
		}
	}

	// El número de líneas está acotado aunque el texto quepa en una sola muy
	// ancha: maxHintLines es el tope, y lo que se pasa se descarta.
	largo := strings.TrimSpace(strings.Repeat("palabra ", 200))
	got = wrapHint(largo, 400, plano)
	if len(got) > maxHintLines {
		t.Errorf("un hint enorme dio %d líneas, want <= %d", len(got), maxHintLines)
	}
	// Y el recorte es por el final: lo que se conserva es la cabeza.
	if len(got) > 0 && !strings.HasPrefix(largo, got[0]) {
		t.Errorf("el recorte debería tirar por el final, la primera línea es %q", got[0])
	}

	// Y el pintado se aplica a cada línea, que es justo lo que hace que el color no
	// dependa de dónde caiga el corte.
	got = wrapHint("uno dos tres", 4, func(s string) string { return "[" + s + "]" })
	for _, l := range got {
		if !strings.HasPrefix(l, "[") || !strings.HasSuffix(l, "]") {
			t.Errorf("la línea no está vestida: %q", l)
		}
	}

	// Texto vacío: una línea vacía, no una lista vacía (que pintaría una caja sin
	// contenido y parecería un bug).
	if got = wrapHint("", 20, plano); len(got) != 1 {
		t.Errorf("texto vacío dio %q, want una línea", got)
	}
}
