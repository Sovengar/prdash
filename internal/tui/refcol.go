// Columna ITEM: prefijo de ruta común por sección y ancho por contenido.
//
// Las rutas de proyecto largas (subgrupos de GitLab tipo
// "APPCITTI/vsocial/backend/api-gateway") no caben en una columna de ancho fijo
// y recortarlas por la cabeza se comía justo lo que distingue un ítem de otro:
// el nombre del repo y el "#número". Aquí la ruta se parte en dos: el prefijo
// que comparten los ítems de la sección vive en su cabecera, y la celda solo
// pinta el sufijo. La columna se dimensiona al sufijo más largo de todo el
// inbox, y lo que aun así no quepa se recorta por la cola, nunca por el frente.
package tui

import (
	"slices"
	"strings"
	"unicode/utf8"

	"prdash/internal/forge/model"
	"prdash/internal/inbox"
)

// refLayout son los anchos de columna de un render más el prefijo de ruta que
// cada sección declara en su cabecera.
//
// Se calcula una vez por render y se pasa a la cabecera y a las filas: si cada
// uno midiera por su cuenta, una fila podría recortarse con un ancho y la
// siguiente con otro, y la tabla bailaría al escribir encima.
type refLayout struct {
	cols   []tableColumn
	prefix map[model.Section]string
}

// newRefLayout reparte el ancho de la columna ITEM entre las secciones: cada una
// declara el prefijo que comparten sus ítems y solo pinta el sufijo, así que la
// columna se dimensiona al sufijo más largo de todas ellas. Se acota a
// [itemWidthMin, itemWidthCap] para que TITLE conserve su sitio.
func newRefLayout(sections []inbox.Section) refLayout {
	l := refLayout{prefix: make(map[model.Section]string, len(sections))}
	longest := 0
	for _, sec := range sections {
		if len(sec.Items) == 0 {
			continue
		}
		prefix := sectionPrefix(sec.Items)
		l.prefix[sec.Kind] = prefix
		for _, it := range sec.Items {
			longest = max(longest, utf8.RuneCountInString(refSuffix(it, prefix)))
		}
	}
	l.cols = slices.Clone(tableColumns)
	l.cols[colRefIdx].width = min(max(longest, itemWidthMin), itemWidthCap)
	return l
}

// prefixOf devuelve el prefijo de ruta que la cabecera de la sección declara. Sin
// prefijo, la celda pinta la ruta completa.
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

// refSuffix es la etiqueta de la celda: la referencia del ítem sin el prefijo que
// la cabecera de la sección ya declara. Con prefijo vacío, o si por lo que sea
// no encaja, devuelve la referencia completa.
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
// que identifica el ítem. El frente de la ruta ya está en la cabecera.
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
