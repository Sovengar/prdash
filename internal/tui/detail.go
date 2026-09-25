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

// renderDetail pinta el detalle a pantalla completa: identidad, ramas, estado de
// review y de checks, sin salir de la TUI. El ítem se deriva del estado vivo.
func (m *Model) renderDetail() string {
	lines := m.detailLines(m.liveDetail(), true, 0) // 0 = sin límite de alto
	lines = append(lines, "", styleHint.Render("esc/"+m.cfg.KeyFor("detail")+" back to the inbox · cursor is preserved"))
	return strings.Join(lines, "\n")
}

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

// detailLines compone el detalle de un ítem como líneas sueltas, sin el hint de
// cierre: lo comparten la vista a pantalla completa y el panel inferior, que son
// el mismo contenido en distinto hueco de pantalla. `rows` es el alto disponible
// (0 = sin límite). Sin ítem que describir (inbox vacío) lo dice en vez de
// inventar datos.
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
		{"URL", orDash(it.URL)},
	}
	status := []detailField{
		{"State", state.Derive(it).String()},
		{"Review", orDash(reviewLabel(it))},
		{"Checks", checksDetail(it.Checks)},
		{"Diff", diffDetail(it.Diff)},
		{"Updated", relativeTime(it.UpdatedAt)},
		{"Role", roleText(it, viewer)},
	}
	avisos := m.detailWarnings(it)

	// Una columna: los dos bloques apilados, como siempre se ha visto.
	one := []string{title, ""}
	one = appendFields(one, identity)
	one = append(one, "")
	one = appendFields(one, status)
	one = append(one, avisos...)
	if rows <= 0 || len(one) <= rows {
		return one
	}

	// No cabe en una columna: la misma información en rejilla de dos columnas.
	// Se prefiere a recortar campos porque la fila del forge y la del autor solo
	// existen aquí, y en un terminal de 24 líneas se perdían por arriba.
	//
	// El diffstat es lo primero que se cae cuando aún no cabe. Añadirlo hizo que
	// la rejilla pasara de 6 a 7 filas, y con avisos de acción el título empezaba a
	// irse por arriba: un dato que se acaba de perder es peor que uno que nunca se
	// pintó. Es el mismo criterio que la columna DIFF de la lista, que también va
	// la última. El hueco tras el título, en cambio, es decorativo y se cae antes:
	// el título ya destaca por su estilo.
	//
	// Los candidatos van de más a menos preferred y se devuelve el primero que
	// entre; si ninguno entra, se recorta el último (que es el que menos campos
	// sacrifica).
	grids := [][]string{
		detailGrid(append(append([]detailField{}, identity...), status...), m.contentWidth()),
		detailGrid(append(append([]detailField{}, identity...), withoutField(status, "Diff")...), m.contentWidth()),
	}
	layouts := [][]string{
		append(append([]string{title, ""}, grids[0]...), avisos...),
		append(append([]string{title, ""}, grids[1]...), avisos...),
		append(append([]string{title}, grids[1]...), avisos...),
	}
	for _, l := range layouts {
		if len(l) <= rows {
			return l
		}
	}
	return clipTop(append(append([]string{title}, grids[1]...), avisos...), rows)
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
func (m *Model) detailWarnings(it model.Item) []string {
	var out []string
	if reason := m.denied[it.ID()]; reason != "" {
		out = append(out, "", styleWarn.Render("  action disabled: "+reason))
	}
	if reason := m.selfDenied[it.ID()]; reason != "" {
		out = append(out, "", styleWarn.Render("  approve unavailable: "+reason+" · merge still applies"))
	}
	return out
}

// appendFields añade un bloque de campos en una columna.
func appendFields(lines []string, fields []detailField) []string {
	for _, f := range fields {
		lines = append(lines, label(f.key, styleDiffText(f.value)))
	}
	return lines
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
