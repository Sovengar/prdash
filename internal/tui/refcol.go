// Columna ITEM: qué parte de la ruta de proyecto se ve, y con qué ancho.
//
// Las rutas de proyecto largas (subgrupos de GitLab tipo
// "APPCITTI/vsocial/backend/api-gateway") no caben en una columna de ancho fijo
// y recortarlas por la cabeza se comía justo lo que distingue un ítem de otro:
// el nombre del repo y el "#número". Aquí la ruta se parte en dos: el prefijo
// que comparten los ítems de la sección vive en una línea fija, y la celda solo
// pinta el sufijo. La columna se dimensiona al sufijo más largo de la sección
// pintada, y lo que aun así no quepa se recorta por la cola, nunca por el frente.
//
// Ese reparto es el modo por defecto (common), no el único: la tecla `p` cicla
// entre common, full (referencia completa en la celda) y leaf (solo la hoja más
// el número). Fuera de common no hay línea de prefijo que pintar, y el ancho se
// vuelve a medir contra lo que la celda va a mostrar.
package tui

import (
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"prdash/internal/forge/model"
	"prdash/internal/inbox"
)

// prefixMode es qué parte de la ruta de proyecto ve la columna ITEM.
//
// Vive en la TUI y no en la config porque es estado de vista puro: la config
// lists la barra de atajos pero no sabe qué está painted, y no puede saberlo sin
// importar este paquete —que ya no podría importarla él, porque el ciclo de
// importación va en sentido contrario.
type prefixMode int

const (
	// prefixCommon declara el prefijo común de la sección en su línea y deja la
	// celda con el sufijo. Es el comportamiento heredado de los ADR 0002/0004.
	prefixCommon prefixMode = iota
	// prefixFull pone la referencia entera en la celda y no declara prefijo: no
	// hay un grupo común que valga la pena sacar de la fila.
	prefixFull
	// prefixLeaf pone solo la hoja de la ruta más el número: máxima densidad.
	prefixLeaf
)

// String es el nombre del modo tal y como lo ve el usuario en la barra de
// atajos. next() y String() salen del mismo tipo a propósito: el hint no puede
// describir un modo que el ciclo no alcance.
func (p prefixMode) String() string {
	switch p {
	case prefixFull:
		return "full"
	case prefixLeaf:
		return "leaf"
	default:
		return "common"
	}
}

// next avanza al siguiente modo y da la vuelta al primero. Con tres valores es
// aritmética, no una tabla: no hay forma de que el ciclo se desincronice del
// hint.
func (p prefixMode) next() prefixMode {
	return (p + 1) % 3
}

// refLayout son los anchos de columna de un render más el prefijo de ruta que la
// sección pintada declara en su línea de prefijo.
//
// Se calcula una vez por render y se pasa al header de columnas y a las filas:
// si cada uno midiera por su cuenta, una fila podría recortarse con un ancho y
// la siguiente con otro, y la tabla bailaría al escribir encima. Guarda también
// el modo porque la celda lo necesita para etiquetar cada ítem.
type refLayout struct {
	cols   []tableColumn
	prefix map[model.Section]string
	mode   prefixMode
}

// newRefLayout reparte el ancho de la columna ITEM entre las secciones que se
// pintan: cada una declara el prefijo que comparten sus ítems y solo pinta el
// sufijo, así que la columna se dimensiona al sufijo más largo. El llamador le
// pasa solo la sección activa —que es la única visible— para no gastar ancho en
// sufijos de secciones que no se ven. Se acota a [itemWidthMin, itemWidthCap]
// para que TITLE conserve su sitio.
//
// El ancho se cuenta con el hueco de separación (textWidth): el sufijo más largo
// tiene que CABER, no caber menos un rune. Sin ese +1, el ítem más largo de la
// lista quedaría truncado siempre.
//
// El modo se pide explícitamente y no por defecto: cada llamador tiene que decir
// con qué está pintando, que es justo lo que hace distinto al ancho y a la
// etiqueta. Solo en common se calcula el prefijo común; en los otros dos el
// layout no declara ninguno, y eso basta para que la línea de prefijo no se
// pinte (ver listLines) sin necesitar un caso propio allí.
func newRefLayout(sections []inbox.Section, mode prefixMode) refLayout {
	l := refLayout{mode: mode, prefix: make(map[model.Section]string, len(sections))}
	longest := 0
	for _, sec := range sections {
		if len(sec.Items) == 0 {
			continue
		}
		prefix := ""
		if mode == prefixCommon {
			prefix = sectionPrefix(sec.Items)
		}
		l.prefix[sec.Kind] = prefix
		for _, it := range sec.Items {
			longest = max(longest, utf8.RuneCountInString(refCellText(it, mode, prefix)))
		}
	}
	l.cols = slices.Clone(tableColumns)
	l.cols[colRefIdx].width = min(max(longest+1, itemWidthMin), itemWidthCap)
	return l
}

// prefixOf devuelve el prefijo de ruta que la línea de prefijo de la sección
// declara. Sin prefijo, la celda pinta la ruta completa. Fuera del modo common
// no hay prefijo que declarar, así que sale vacío y la línea no se pinta.
func (l refLayout) prefixOf(sec model.Section) string {
	return l.prefix[sec]
}

// sectionPrefix devuelve el prefijo de ruta que comparten todos los ítems de una
// sección, alineado en fronteras "/" y sin comerse nunca el segmento final: la
// celda siempre conserva el nombre del proyecto y su número.
//
// Con menos de dos ítems, o sin nada en común, devuelve "" y cada celda pinta la
// ruta entera.
func sectionPrefix(items []model.Item) string {
	if len(items) < 2 {
		return ""
	}
	segs := make([][]string, 0, len(items))
	minSegs := -1
	for _, it := range items {
		s := strings.Split(it.Ref.Project, "/")
		if minSegs < 0 || len(s) < minSegs {
			minSegs = len(s)
		}
		segs = append(segs, s)
	}
	// El prefijo tiene que ser un directorio estricto para TODOS: nunca el
	// último segmento, o una sección de un solo repo se quedaría sin celda.
	common := 0
	for i := range minSegs - 1 {
		seg := segs[0][i]
		for _, other := range segs[1:] {
			if other[i] != seg {
				return strings.Join(segs[0][:common], "/")
			}
		}
		common++
	}
	return strings.Join(segs[0][:common], "/")
}

// refCellText es la etiqueta de la celda ITEM en el modo dado: lo que el ancho
// de la columna mide y lo que la fila pinta.
//
// common y full comparten etiqueta porque con el prefijo vacío (que es lo que
// pasa en full, y también en una sección sin prefijo común) refSuffix ya devuelve
// la referencia completa. Esa es toda la degradación de common a full: no hay un
// caso especial, es el comportamiento que ya tenía.
func refCellText(it model.Item, mode prefixMode, prefix string) string {
	if mode == prefixLeaf {
		return refLeaf(it)
	}
	return refSuffix(it, prefix)
}

// refLeaf es la referencia de máxima densidad: el último segmento del proyecto
// más el número. Lo que identify al ítem y nada del camino que lleva hasta él.
//
// Corta por el último separador, no por el primero, porque el identificador está
// al final de la ruta. Un proyecto vacío da "#n": no es un caso raro —un forge
// puede devolver la referencia sin parsear— y aun así deja la celda con el
// número, que es lo único que sigue identificando el ítem.
func refLeaf(it model.Item) string {
	project := it.Ref.Project
	if i := strings.LastIndex(project, "/"); i >= 0 {
		project = project[i+1:]
	}
	return project + "#" + strconv.Itoa(it.Number)
}

// refSuffix es la etiqueta de la celda: la referencia del ítem sin el prefijo que
// la línea de prefijo de la sección ya declara. Con prefijo vacío, o si por lo
// que sea no encaja, devuelve la referencia completa.
func refSuffix(it model.Item, prefix string) string {
	label := refLabel(it)
	if prefix == "" {
		return label
	}
	rest, ok := strings.CutPrefix(label, prefix+"/")
	if !ok {
		return label
	}
	return rest
}

// truncateTail recorta por la izquierda añadiendo "…" delante, de modo que en
// una ruta larga sobreviva la cola (hoja del proyecto y "#número"), que es lo
// que identifica el ítem. El frente de la ruta ya está en la línea de prefijo.
func truncateTail(s string, w int) string {
	if w <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	return "…" + string(runes[len(runes)-(w-1):])
}
