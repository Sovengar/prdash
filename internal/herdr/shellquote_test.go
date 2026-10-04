package herdr

import (
	"strings"
	"testing"
)

// `shellQuote` decide si un argumento se pasa literal al shell del pane o entrecomillado, y
// lo que cita son rutas y etiquetas que vienen del forge. Eso la convierte en la frontera
// entre "el forge me dijo algo" y "un shell lo va a interpretar", y en una frontera es donde
// se cuelan las cosas.
//
// Y lo que se cuela sin comillas es la inyección: una etiqueta de review con un `;` o un
// `$(...)` es texto libre hasta que decide `shellSafe`. Con el nombre de una rama —que sale
// de la API y que cualquiera puede crear en su repo— no hace falta ser malicioso para acabar
// en un comando.

// TestLoQueNoEsSeguroLlevaComillasYLoQueLoEsNo: la línea.
//
// Y la lista de lo que NO lleva comillas es corta a propósito: letras, dígitos y un puñado
// de signos. Todo lo demás pasa por comillas, incluidos los espacios y los signos que un
// shell entendería. La regla es "casi todo necesita comillas", no "casi todo puede ir
// suelto" —porque una lista de excepciones es un agujero cada vez que se le añade uno—.
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

// insetList devuelve los textos que no pueden ir sueltos.
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

// TestLaCadenaVaciaLlevaComillasYNoDesaparece: el caso que no se puede tratar como los
// demás.
//
// Y es el que separa "casi todo" de "todo". Un argumento vacío sin comillas desaparece del
// comando: el shell ve un comando con un argumento menos, que es otra cosa. Con `”` el shell
// ve un argumento vacío, que es lo que el caller pidió.
//
// Y es un caso real: el `Cwd` de un pane sale del worktree, y un pane sin directorio de
// trabajo es un pane que arranca en el HOME del usuario —donde está su código y su
// historial— en vez de en el repo del review.
func TestLaCadenaVaciaLlevaComillasYNoDesaparece(t *testing.T) {
	got := shellQuote("")
	if got != "''" {
		t.Errorf("shellQuote de la vacia dio %q, want %q: sin comillas el argumento "+
			"desaparece del comando y el pane arranca en el HOME del usuario", got, "''")
	}
	// Y `shellSafe("")` SÍ da true —el bucle no ve ningún carácter y no hay nada
	// peligroso—, y eso no la deja salir sin comillas: la guarda está ANTES de preguntar,
	// en `shellQuote`. Mi primera versión afirmaba que `shellSafe` tenía que dar false
	// para la vacía, y eso sería poner la protección en el sitio equivocado: si
	// `shellSafe` hiciera el trabajo, un texto SIN caracteresodn it'd estar protegido por
	// la misma razón que los demás, y no lo está.
	if !shellSafe("") {
		t.Error("shellSafe de la vacia dio false: el bucle no ve caracteres y no hay nada " +
			"que marcar. La protege la guarda de shellQuote, no esta")
	}
}

// TestLaComillaSimpleSeEscapaYElRestoNo: el escape, que es el caso que se cuela.
//
// Y el escape es el clásico `'\”`: cerrar la comilla, poner una comilla literal escapada,
// reabrir. Y lo que hay que comprobar es que el shell resuelve el resultado al texto
// original, porque un escape mal puesto deja el comando SIN cerrar y el shell se come lo que
// viene detrás —que en un pane es el siguiente comando—.
//
// Y lo que no hay que hacer es escapar con barra invertida dentro de comillas simples: las
// comillas simples no interpretan escapes, así que `\'` dentro de ellas son dos caracteres,
// no una comilla.
func TestLaComillaSimpleSeEscapaYElRestoNo(t *testing.T) {
	got := shellQuote("it's")
	want := `'it'\''s'`
	if got != want {
		t.Errorf("shellQuote dio %q, want %q", got, want)
	}
	if resuelto := desEscapar(got); resuelto != "it's" {
		t.Errorf("el shell leería %q, want %q", resuelto, "it's")
	}

	// Varias comillas seguidas, que es donde el escape se descuadra si se hace por
	// sustitución de un solo caso.
	varias := shellQuote("'a'b'c'")
	if r := desEscapar(varias); r != "'a'b'c'" {
		t.Errorf("varias comillas: el shell leería %q, want %q", r, "'a'b'c'")
	}

	// Y una comilla suelta.
	if r := desEscapar(shellQuote("'")); r != "'" {
		t.Errorf("una comilla suelta dio %q", r)
	}
	// Y una cadena que ya viene entre comillas simples: se cita entera y las comillas de
	// dentro se escapan. Es el caso de una etiqueta que el forge ya entrecomilló, que pasa
	// por las dos capas y acaba con comillas duplicadas.
	yaCitada := "'prefijo'"
	if r := desEscapar(shellQuote(yaCitada)); r != yaCitada {
		t.Errorf("una etiqueta ya citada dio %q, want %q", r, yaCitada)
	}
}

// desEscapar es el shell mínimo que resuelve las comillas simples con el escape `'\”`, que
// es lo que hace falta para comprobar que el resultado de `shellQuote` es lo que el shell
// leería.
//
// Y está aquí y no en producción a propósito: es la REFERENCIA contra la que se compara. Si
// compartiera implementación con `shellQuote` el test no probaría nada —comparar la función
// consigo misma es el peor aserto posible—.
func desEscapar(citado string) string {
	if len(citado) < 2 || !strings.HasPrefix(citado, "'") || !strings.HasSuffix(citado, "'") {
		return citado
	}
	interior := citado[1 : len(citado)-1]
	// Dentro de comillas simples toda comilla tiene que ser parte de un `'\''`, así que
	// sustituir esa secuencia es resolver el caso sin ambigüedad.
	return strings.ReplaceAll(interior, `'\''`, "'")
}

// TestUnArgumentoConTodoLoPeligrosoJuntosNoSeRompe: la mezcla, que es el caso real.
//
// Y es el que de verdad aparece: una etiqueta de branch de GitHub puede tener espacios,
// comillas y signos a la vez —`fix "the thing"; rm -rf /` es un nombre de rama
// perfectamente válido—. Y lo que se comprueba es la propiedad de composición, no la forma:
// el shell tiene que devolver EXACTAMENTE el texto original.
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
		// Y la propiedad mecánica: el número de comillas simples es PAR. Una cadena citada
		// tiene exactamente la comilla de apertura y la de cierre, más un escape por
		// cada comilla simple interior —y `\'\''` aporta cuatro—.
		//
		// La primera versión de este aserto pedía un número IMPAR, que es lo contrario
		// de lo correcto: `echo '#123'` tiene dos comillas y funciona. La propiedad que
		// importa ya está arriba: el shell resuelve al original.
		if n := strings.Count(citado, "'"); n%2 != 0 {
			t.Errorf("%q se citó como %q, con un número impar de comillas", original, citado)
		}
	}
}

// TestUnTextoQueNoEsSeguroNuncaSaleSinComillas: la propiedad, para todo el dominio.
//
// Y es un aserto de PROPIEDAD y no de casos, que es lo que protege el futuro: un carácter
// nuevo en `shellSafe` no rompe una tabla de casos porque nadie lo piensa, pero rompe este
// barrido, que incluye todo lo imprimible en ASCII.
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
	// Y fuera de ASCII no se sabe: el barrido solo cubre lo imprimible, y dejarlo escrito
	// evita que alguien dé por hecho que los emojis o el chino pasan sin comillas.
	for _, s := range []string{"ñ", "日", "🙂"} {
		if shellSafe(s) {
			t.Errorf("%q sale sin comillas; los no ASCII no están en la lista de seguros", s)
		}
	}
}
