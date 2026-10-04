package model

import (
	"testing"
)

// Este fichero cubre las funciones del modelo que no tenían test y que comparten una
// propiedad: son funciones de PRESENTACIÓN y de DEFAULT, no de lógica. No calculan nada
// que se pueda equivocar por un algoritmo; se equivocan cuando devuelven el texto
// equivocado, y un texto equivocado en un enum se ve en pantalla como si fuera un bug de
// la forge.
//
// Y ese es el motivo de fijarlas: son texto que forma parte del contrato. `String()` lo
// usa `--print`, así que su salida está en lo que alguien compara con `diff`. Y la
// diferencia entre `String()` y `Legend()` —"Review / assigned" contra "Assigned"— está
// documentada en el código como deliberada, y sin un test que la fije la próxima
// refactorización los iguala "para no duplicar" y nadie se entera hasta que la línea del
// borde se sale de ancho.

// TestLaSeccionTieneDosNombresYSoloUnoEsDeAPI: el nombre largo y la leyenda corta.
//
// Y los tres casos validados juntos, porque son tres ramas del switch y una tabla que solo
// comprobara el camino feliz dejaría el `default` sin cubrir. Y el `default` es el que
// importa: una sección desconocida tiene que caer al valor crudo en vez de a un título
// inventado, porque un título inventado se lee como una sección que existe.
func TestLaSeccionTieneDosNombresYSoloUnoEsDeAPI(t *testing.T) {
	casos := []struct {
		seccion   Section
		wantStr   string
		wantLendg string
	}{
		{SectionAuthored, "Created by me", "Mine"},
		{SectionReview, "Review / assigned", "Assigned"},
		{SectionMentions, "Mentions", "Mentioned"},
		// Desconocida: cae al valor crudo, y sale el MISMO en los dos sitios. No hay
		// título de sobra para una sección que no existe.
		{Section("otra"), "otra", "otra"},
		{Section(""), "", ""},
	}
	for _, c := range casos {
		if got := c.seccion.String(); got != c.wantStr {
			t.Errorf("Section(%q).String() dio %q, want %q", c.seccion, got, c.wantStr)
		}
		if got := c.seccion.Legend(); got != c.wantLendg {
			t.Errorf("Section(%q).Legend() dio %q, want %q", c.seccion, got, c.wantLendg)
		}
	}
}

// TestLosDosNombresNoSeConfunden: la razón de que `Legend` exista aparte.
//
// Y es el test que salva la refactorización equivocada. La tentación con dos métodos que
// devuelven texto sobre el mismo tipo es borrar el segundo y usar el primero en todas
// partes, que es lo correcto por DRY y lo rompe en pantalla: "Created by me" mide 13
// caracteres y "Mine" mide 4, y la leyenda de conteos va en el borde del inbox, que es lo
// más estrecho que hay. Igualarlos no falla, se desborda.
func TestLosDosNombresNoSeConfunden(t *testing.T) {
	// Lo que se fija es que NUNCA coinciden. Si coincidieran, alguien puede haber
	// borrado `Legend` y delegado en `String`, y la línea del borde se ensancha sin que
	// nada falle.
	//
	// Y lo que NO se fija es que la leyenda siempre sea más corta, que fue lo primero que
	// se escribió aquí y es falso: "Mentioned" son 9 caracteres y "Mentions" son 8. La
	// razón por la que existen dos nombres es que la leyenda quepa en la línea del borde,
	// y esa es una condición sobre la línea, no sobre la longitud de cada texto. Un
	// aserto de longitud habría creado la impresión de una garantía que el código no da.
	for _, s := range []Section{SectionAuthored, SectionReview, SectionMentions} {
		if s.Legend() == s.String() {
			t.Errorf("%q: la leyenda y el titulo son el mismo texto (%q). La leyenda va "+
				"en el borde del inbox y no cabe con los nombres largos", s, s.String())
		}
	}
	// Y la leyenda de las tres secciones que existen está dentro de un ancho que el borde
	// aguanta. El número sale de ahí, no de un gusto: es el presupuesto de la línea.
	for _, s := range []Section{SectionAuthored, SectionReview, SectionMentions} {
		if len(s.Legend()) > 9 {
			t.Errorf("%q: la leyenda %q son %d caracteres y el borde no tiene ese ancho",
				s, s.Legend(), len(s.Legend()))
		}
	}
}

// TestElTotalDeLineasSumaLasDosCaras: `Total` es la magnitud que ordena el trabajo, así
// que el orden importa.
//
// Y el caso del diffstat desconocido —cero y cero— devuelve cero, que es lo que un
// "sin datos" tiene que parecer en la columna. Lo que NO puede hacer es un total que no
// cuadre con las dos caras, porque eso es una cifra que se ordena mal.
func TestElTotalDeLineasSumaLasDosCaras(t *testing.T) {
	casos := []struct {
		d    DiffStat
		want int
	}{
		{DiffStat{Additions: 3, Deletions: 4}, 7},
		{DiffStat{Additions: 100, Deletions: 0}, 100},
		{DiffStat{Additions: 0, Deletions: 100}, 100},
		{DiffStat{}, 0},
		// Diffstat desconocido: cero, no menos.
		{DiffStat{Known: false, Additions: 5, Deletions: 5}, 10},
	}
	for _, c := range casos {
		if got := c.d.Total(); got != c.want {
			t.Errorf("DiffStat%+v.Total() dio %d, want %d", c.d, got, c.want)
		}
	}
}

// TestLasReglasDeMergePermisivasLoDicenConSuBit: `MergeRulesAll` devuelve las tres
// estrategias, pero con `Known` a true.
//
// Y ese bit es la parte que importa, y es la que un test que solo mirara los tres
// booleanos pasaría por alto. `Known=false` significa "no lo sabemos", que el comentario
// dice que NO restringe. Un default permisivo con `Known=false` sería indistinguible de un
// repositorio cuyas reglas no se han leído, y la consecuencia sería la que el comentario
// describe: ofrecer un modo que el forge va a rechazar.
//
// O sea: las tres estrategias a true y `Known` a true son afirmações sobre el mismo dato.
// Poner `Known` a false no es un default más conservador, es decir menos.
func TestLasReglasDeMergePermisivasLoDicenConSuBit(t *testing.T) {
	r := MergeRulesAll()
	if !r.Known {
		t.Error("MergeRulesAll dio Known=false: entonces no se distingue de unas reglas " +
			"que nadie ha leido del forge")
	}
	if !r.MergeCommit || !r.Rebase || !r.Squash {
		t.Errorf("MergeRulesAll dio %+v: el default tiene que admitir las tres", r)
	}
	// Y la struct vacía es lo contrario en el bit, y eso también es una aserción: si
	// alguien invierte el sentido de `Known`, los dos extremos se intercambian.
	if (MergeRules{}).Known {
		t.Error("las reglas vacias dio Known=true: Known debe decir si se leyeron, no " +
			"si son restrictivas")
	}
}
