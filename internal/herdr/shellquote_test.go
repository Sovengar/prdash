package herdr

import (
	"strings"
	"testing"
)

// The safe list is short on purpose.
func TestLoQueNoEsSeguroLlevaComillasYLoQueLoEsNo(t *testing.T) {
	seguros := []string{
		"abc", "ABC", "a1", "0",
		"pr-123", "feature_x", "main-2", "v1.2.3",
		"a-b_c.d", "-", "_", ".",
	}
	for _, s := range seguros {
		if !shellSafe(s) {
			t.Errorf("shellSafe(%q) dio false: un nombre sin signos raros no necesita comillas", s)
		}
		if got := shellQuote(s); got != s {
			t.Errorf("shellQuote(%q) dio %q, want el texto tal cual", s, got)
		}
	}

	for _, s := range insetList() {
		if shellSafe(s) {
			t.Errorf("shellSafe(%q) dio true: un shell interpretaría ese texto", s)
		}
		got := shellQuote(s)
		if !strings.HasPrefix(got, "'") || !strings.HasSuffix(got, "'") {
			t.Errorf("shellQuote(%q) dio %q, y tiene que ir entre comillas simples", s, got)
		}
	}
}

func insetList() []string {
	return []string{
		"con espacio",
		"punto y coma",
		"$(comando)",
		"`comando`",
		"a|b", "a&b", "a>b", "a<b", "a;b",
		"a\nb", "a\tb",
		"comilla'simple",
		"dolar$",
		"tilde~",
		"asterisco*",
		"interrogacion?",
		"corchete[]",
		"llave{}",
		"parentheses()",
		"comilla\"doble",
		`\`,
		"#comentario",
		"#123",
	}
}

// The case that cannot be treated like the others.
func TestLaCadenaVaciaLlevaComillasYNoDesaparece(t *testing.T) {
	got := shellQuote("")
	if got != "''" {
		t.Errorf("shellQuote de la vacia dio %q, want %q: sin comillas el argumento "+
			"desaparece del comando y el pane arranca en el HOME del usuario", got, "''")
	}
	// shellSafe("") IS true —the loop sees no character and nothing dangerous— and that does not
	//let it out unquoted.
	if !shellSafe("") {
		t.Error("shellSafe de la vacia dio false: el bucle no ve caracteres y no hay nada " +
			"que marcar. La protege la guarda de shellQuote, no esta")
	}
}

// The classic `'\”` escape: closing the quote, escaping, reopening.
func TestLaComillaSimpleSeEscapaYElRestoNo(t *testing.T) {
	got := shellQuote("it's")
	want := `'it'\''s'`
	if got != want {
		t.Errorf("shellQuote dio %q, want %q", got, want)
	}
	if resuelto := desEscapar(got); resuelto != "it's" {
		t.Errorf("el shell leería %q, want %q", resuelto, "it's")
	}

	varias := shellQuote("'a'b'c'")
	if r := desEscapar(varias); r != "'a'b'c'" {
		t.Errorf("varias comillas: el shell leería %q, want %q", r, "'a'b'c'")
	}

	if r := desEscapar(shellQuote("'")); r != "'" {
		t.Errorf("una comilla suelta dio %q", r)
	}
	// A string that already comes in single quotes is quoted whole and its inner quotes are left
	// alone.
	yaCitada := "'prefijo'"
	if r := desEscapar(shellQuote(yaCitada)); r != yaCitada {
		t.Errorf("una etiqueta ya citada dio %q, want %q", r, yaCitada)
	}
}

// The smallest shell that resolves single quotes with that escape, which is all it takes to check it.
func desEscapar(citado string) string {
	if len(citado) < 2 || !strings.HasPrefix(citado, "'") || !strings.HasSuffix(citado, "'") {
		return citado
	}
	interior := citado[1 : len(citado)-1]
	// Inside single quotes every quote has to be part of a `'\''`, so a plain replacement breaks.
	return strings.ReplaceAll(interior, `'\''`, "'")
}

// The mix is the real case: a label with spaces, quotes and slashes.
func TestUnArgumentoConTodoLoPeligrosoJuntosNoSeRompe(t *testing.T) {
	casos := []string{
		`fix "the thing"; rm -rf /`,
		`rama con 'comilla' y espacio`,
		`$(whoami)`,
		"nueva\nlínea",
		"tab\tdentro",
		"acentos-y-ñ",
		"%s %d %v\n",
		"*",
		`a'b'c"d`,
		"\\\\",
		"  con espacios alrededor  ",
	}
	for _, original := range casos {
		citado := shellQuote(original)
		if resuelto := desEscapar(citado); resuelto != original {
			t.Errorf("el shell leería %q de %q, y debía leer %q", resuelto, citado, original)
		}
		// The mechanical property: a quoted string has an EVEN number of single quotes.
		if n := strings.Count(citado, "'"); n%2 != 0 {
			t.Errorf("%q se citó como %q, con un número impar de comillas", original, citado)
		}
	}
}

// A PROPERTY assertion over the whole domain, not a table of cases.
func TestUnTextoQueNoEsSeguroNuncaSaleSinComillas(t *testing.T) {
	for r := rune(32); r < rune(127); r++ {
		s := string(r)
		sinComillas := shellQuote(s) == s
		if sinComillas != shellSafe(s) {
			t.Errorf("el rune %q sale %s y shellSafe lo marca %s: las dos cosas "+
				"tienen que coincidir",
				r,
				map[bool]string{true: "sin comillas", false: "citado"}[sinComillas],
				map[bool]string{true: "seguro", false: "inseguro"}[shellSafe(s)])
		}
	}
	// Outside ASCII it is unknown: the sweep only covers printable ASCII, and writing that down
	// avoids guessing.
	for _, s := range []string{"ñ", "日", "🙂"} {
		if shellSafe(s) {
			t.Errorf("%q sale sin comillas; los no ASCII no están en la lista de seguros", s)
		}
	}
}
