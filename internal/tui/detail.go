// Detalle de un ítem del inbox.
package tui

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"prdash/internal/forge/model"
	"prdash/internal/state"
)

// detailPane compone el detalle para el alto del panel inferior.
func (m *Model) detailPane(rows int) []string {
	it, ok := m.selected()
	return m.detailLines(it, ok, rows)
}

// detailField es una línea "etiqueta: valor" del detalle. Se guarda como dato y
// no como texto ya maquetado para que el panel elija la disposición: en una
// columna si cabe, en dos si el hueco es más bajo que el detalle.
type detailField struct{ key, value string }

// labelWidth es el ancho reservado a la etiqueta de un campo del detalle.
const labelWidth = 14

// detailGap separa las dos columnas cuando el detalle va en rejilla.
const detailGap = 4

// detailLines compone el detalle de un ítem como líneas sueltas para el panel
// inferior. `rows` es el alto disponible. Sin ítem que describir (inbox vacío)
// lo dice en vez de inventar datos.
//
// La ficha son tres bloques: los campos cortos en rejilla de dos columnas, el URL
// en una fila a todo el ancho, y los comentarios debajo. Siempre en rejilla, no
// solo cuando no caben en una: en una sola columna la ficha ocupaba 16 de las ~18
// líneas que concede el 40% de un terminal normal y no cabía ni un comentario.
//
// Sacar el URL de la rejilla no cuesta alto, que es lo que hacía sospechar: los 13
// campos ocupan 7 filas en dos columnas, y los 12 que quedan más el URL a ancho
// completo siguen siendo 7. Lo que cambia es que la URL se lee entera, y una URL
// truncada no se puede copiar, que es para lo que está.
//
// La escalera de degradación va de más a menos preferred y se devuelve el primer
// candidato que entre. Si ninguno, se recorta por arriba (ver clipTop).
func (m *Model) detailLines(it model.Item, ok bool, rows int) []string {
	if !ok {
		return []string{styleDim.Render("no selection: move the cursor onto an item")}
	}

	title := styleDetailTitle.Render(it.Title)
	viewer := m.viewerLogin(it.Forge)
	identity := []detailField{
		{"Item", refLabel(it)},
		{"Forge", forgeLabel(it)},
		{"Author", orDash(it.Author)},
		{"Source", orDash(it.SourceBranch)},
		{"Target", orDash(it.TargetBranch)},
		{"Number", fmt.Sprintf("#%d", it.Number)},
	}
	// El orden de los campos de estado no es neutro: la rejilla los empareja por
	// posición, así que el último par es la última fila, que es lo único que sobrevive
	// al recorte en un panel diminuto. Review y Role van al final a propósito, porque
	// son los dos que dicen si la acción procede; si el recorte se los come, la ficha
	// deja de responder a la pregunta para la que está.
	status := []detailField{
		{"State", state.Derive(it).String()},
		{"Checks", checksDetail(it.Checks)},
		{"Diff", diffDetail(it.Diff)},
		{"Updated", relativeTime(it.UpdatedAt)},
		{"Review", orDash(reviewLabel(it))},
		{"Role", roleText(it, viewer)},
	}
	inner := m.contentWidth()
	grid := detailGrid(withField(identity, status), inner)
	noDiff := detailGrid(withField(identity, withoutField(status, "Diff")), inner)
	url := []string{fullWidthField(detailField{"URL", orDash(it.URL)}, inner)}
	avisos := m.detailWarnings(it)

	// El presupuesto de los comentarios es lo que sobra tras la cabecera de la
	// ficha, y se calcula antes de componerlos porque de él depende cuántas filas
	// puede gastar cada uno. Sin esto, un bloque de cinco comentarios de cuatro
	// filas no entraría en un panel de 18 y la escalera lo tiraría entero.
	avail := rows - len(grid) - len(url) - 2 - len(avisos) // 2 = título + hueco
	comments := m.commentLines(it, avail, inner)

	// El orden de lo que se cae sale de dos criterios, y no es el orden en que se
	// enumeran: primero lo que se puede volver a pedir, y después lo más reciente.
	//
	// Los comentarios son lo primero porque son lo único repedible con una tecla (o
	// con el siguiente tick) y lo único que no estaba en la ficha antes de existir
	// esta sección; luego el hueco tras el título, que es decorativo y lo compensa
	// el estilo del propio título; luego la fila del URL, que es una fila entera
	// sacrificada por un dato que se puede volver a leer con `o`; y por último el
	// diffstat, con el criterio que ya tenía la rejilla: un dato que se acaba de
	// perder es peor que uno que nunca se pintó. Es el mismo criterio que la columna
	// DIFF de la lista, que también va la última.
	//
	// El salto del cuarto al quinto candidato se lleva tres cosas a la vez
	// (comentarios, hueco y fila del URL) porque entre ellas no hay ningún tamaño
	// intermedio: quitar solo el URL deja la misma altura, y quitar solo el Diff
	// también, porque la rejilla pasa de 12 a 11 campos y ambas caben en 6 filas.
	withGap := []string{title, ""}
	noGap := []string{title}
	layouts := [][]string{
		stackDetail(withGap, grid, url, comments, avisos),
		stackDetail(withGap, grid, url, avisos),
		stackDetail(noGap, grid, url, comments, avisos),
		stackDetail(noGap, grid, url, avisos),
		stackDetail(noGap, grid, noDiff, avisos),
	}
	for _, l := range layouts {
		if len(l) <= rows {
			return l
		}
	}
	return clipTop(stackDetail(noGap, noDiff, avisos), rows)
}

// withField concatena bloques de campos en uno nuevo, sin tocar los de entrada.
func withField(blocks ...[]detailField) []detailField {
	var out []detailField
	for _, b := range blocks {
		out = append(out, b...)
	}
	return out
}

// stackDetail concatena los trozos del detalle. El hueco decorativo va como
// línea vacía explícita y no como trozo: se quita del medio, no de un extremo.
func stackDetail(parts ...[]string) []string {
	var out []string
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// withoutField quita un campo por su etiqueta. Se usa para dropear el diffstat
// cuando el detalle no tiene sitio para todo.
func withoutField(fields []detailField, key string) []detailField {
	out := make([]detailField, 0, len(fields))
	for _, f := range fields {
		if f.key != key {
			out = append(out, f)
		}
	}
	return out
}

// detailWarnings son los avisos de acción deshabilitada del ítem.
//
// Solo el veto que impone el forge, que además es pegajoso: se recuerda por ítem
// hasta que un refresco lo levanta, así que puede seguir ahí después de que la
// cabecera haya dejado de avisar. Por eso merece una fila.
//
// El veto de aprobar lo propio NO se pinta, y es a propósito. Se deriva del ítem y
// del login, así que saldría en todos los renders de todos tus PRs —que son casi
// todos los de "Created by me"— y la ficha acabaría con una línea permanente
// repitiendo lo que el campo Role ya dice. Además es lo único que se puede pedir
// de otro modo: la razón se entrega al pulsar la tecla, en el aviso, que es
// cuando se puede actuar sobre ella. En la ficha solo ocuparía filas.
func (m *Model) detailWarnings(it model.Item) []string {
	if reason := m.denied[it.ID()]; reason != "" {
		return []string{"", styleWarn.Render("  action disabled: " + reason)}
	}
	return nil
}

// fullWidthField compone un campo que ocupa la fila entera en vez de media.
//
// Existe para el URL, y por un motivo concreto: una URL de GitLab self-managed
// con subcarpeta se pasa fácil de 80 caracteres, así que en media columna se leen
// 40 y queda un resto inútil. Y una URL que no se puede copiar entera no sirve
// para nada, que es justo para lo que está en la ficha. El ancho entero lo hace
// legible sin gastar una fila más, porque los 12 campos cortos siguen cabiendo en
// las mismas 6.
func fullWidthField(f detailField, inner int) string {
	return label(f.key, styleDiffText(truncate(f.value, max(1, inner-labelWidth))))
}

// detailGrid reparte los campos en dos columnas de ancho fijo, por filas
// (izquierda, derecha): así se lee como una ficha y no como dos listas.
func detailGrid(fields []detailField, inner int) []string {
	cell := max(24, (inner-detailGap)/2)
	out := make([]string, 0, (len(fields)+1)/2)
	for i := 0; i < len(fields); i += 2 {
		left, leftW := detailCell(fields[i], cell)
		right := ""
		if i+1 < len(fields) {
			right, _ = detailCell(fields[i+1], cell)
		}
		// El hueco mínimo evita que un valor largo llegue a pisar la columna de
		// al lado cuando el campo ocupa la celda entera.
		out = append(out, left+strings.Repeat(" ", max(detailGap, cell-leftW))+right)
	}
	return out
}

// detailCell compone un campo ajustado a un ancho y devuelve su ancho real en
// texto plano, que es lo que necesita el padding de la columna de al lado.
func detailCell(f detailField, width int) (string, int) {
	value := truncate(f.value, max(1, width-labelWidth))
	// El ancho se mide sobre el valor recortado y en plano; el color va encima,
	// ya sin que nadie lo mida (ver styleDiffText).
	return label(f.key, styleDiffText(value)), labelWidth + utf8.RuneCountInString(value)
}

// clipTop recorta por arriba lo que no cabe: el final del detalle (estado,
// review y rol) es lo que dice si la acción procede.
func clipTop(lines []string, rows int) []string {
	if rows <= 0 || len(lines) <= rows {
		return lines
	}
	return lines[len(lines)-rows:]
}

// label compone una línea "etiqueta: valor" del detalle, sin salto de línea: el
// detalle se compone como lista de líneas y quien la pinta decide los saltos.
func label(key, value string) string {
	return styleDetailKey.Render(pad(key+":", labelWidth)) + value
}

// reviewLabel traduce la decisión de review y el tipo de review a texto.
func reviewLabel(it model.Item) string {
	decision := it.ReviewDecision
	switch decision {
	case "APPROVED":
		decision = "approved"
	case "CHANGES_REQUESTED":
		decision = "changes requested"
	case "REVIEW_REQUIRED":
		decision = "review required"
	}
	if decision == "" {
		decision = it.State
	}
	switch it.ReviewKind {
	case model.ReviewRequested:
		return decision + " · review requested"
	case model.ReviewAssigned:
		return decision + " · assigned"
	default:
		return decision
	}
}

// checksDetail describe el estado de los checks para el detalle.
func checksDetail(c model.Checks) string {
	if c.Total == 0 && c.State == model.ChecksUnknown {
		return "no checks"
	}
	base := string(c.State)
	if c.Failing > 0 || c.Pending > 0 {
		base += fmt.Sprintf(" (%d/%d failing, %d pending)", c.Failing, c.Total, c.Pending)
	} else {
		base += fmt.Sprintf(" (%d)", c.Total)
	}
	return base
}

// diffDetail describe el diffstat con los números sin compactar y el recuento de
// ficheros. Aquí sí cabe la cifra exacta: la columna de la lista es la que
// abrevia, y un detalle que dijera "1.2k" cuando la cifra real es 1.234 no
// serviría para nada.
func diffDetail(d model.DiffStat) string {
	if !d.Known {
		return "unknown (forge did not report it)"
	}
	if d.Total() == 0 {
		return "no changes"
	}
	noun := "files"
	if d.Files == 1 {
		noun = "file"
	}
	return fmt.Sprintf("+%d -%d (%d %s)", d.Additions, d.Deletions, d.Files, noun)
}

// relativeTime formatea una marca temporal como "3h", "2d".
func relativeTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
