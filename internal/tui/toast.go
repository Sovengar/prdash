// Toasts: avisos transitorios que se dibujan encima de la vista, abajo a la
// derecha, y se autodestruyen pasado su TTL.
//
// Mismo modelo que el ToastManager de dbx (pila de avisos con caducidad, tick
// propio y overlay en la esquina), con dos diferencias: el reloj es inyectable
// para poder testear la expiración sin dormir, y el ancho se recorta al ancho
// útil de la vista para no desbordar nunca laterminal.
package tui

import (
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	// toastDuration es el TTL por defecto de un aviso.
	toastDuration = 4 * time.Second
	// toastTickInterval es la resolución con la que caducan los avisos.
	toastTickInterval = 500 * time.Millisecond
	// toastMinWidth/toastMaxWidth acotan el ancho de la caja.
	toastMinWidth = 24
	toastMaxWidth = 60
	// toastFrame es lo que el marco y el padding suman al ancho exterior: los dos
	// bordes verticales y una columna de padding por lado. En lipgloss Width() es el
	// ancho del contenido, así que esto es lo que hay que restar para saber cuánto
	// texto cabe.
	toastFrame = 4
	// toastIconGap es lo que el icono y su espacio empujan delante del texto. Solo
	// en la primera línea, que es la razón de que el texto se parta al ancho útil
	// menos esto y no al ancho útil.
	//
	// Es el ancho del icono (1) más su espacio (1), y por eso el ancho del icono se
	// mide y no se supone: si algún día se añade un icono de dos columnas, el hueco
	// crece con él y el texto sigue entrando.
	toastIconGap = 2
	// toastIconSpace es el espacio que va entre el icono y el texto. Va con nombre
	// aparte y no dentro de toastIconGap porque son dos cosas distintas: una es lo
	// que mide el icono (que depende del nivel) y otra el hueco fijo. Sumarlos en un
	// solo 2 haría que un icono de dos columnas no empujara nada, que es justo el
	// error que haría que el texto se saliera del marco.
	toastIconSpace = 1
	// toastMinInner es el suelo del ancho de TEXTO. Por debajo de 8 columnas no hay
	// nada legible, y un aviso ilegible no informa de nada: mejor uno estrecho y
	// partido en tres líneas que uno ancho con media palabra por línea.
	toastMinInner = 8
	// toastMinAvailable es el hueco mínimo para intentar encajar la caja. Por
	// debajo la superposición la recortaría por la derecha, así que es más honesto
	// no fingir que cabe: se usa el ancho acotado y se deja que el recorte la
	// recorte.
	toastMinAvailable = 8
)

// toastLevel es la gravedad de un aviso y decide icono y color.
type toastLevel int

const (
	toastSuccess toastLevel = iota
	toastError
	toastInfo
	toastWarning
)

// toast es un aviso con su caducidad.
type toast struct {
	message  string
	level    toastLevel
	created  time.Time
	duration time.Duration
}

// toastManager es la pila de avisos vivos.
type toastManager struct {
	toasts []toast
	// now es el reloj: inyectable para que los tests no dependan del tiempo.
	now func() time.Time
}

// newToastManager construye la pila con el reloj real.
func newToastManager() *toastManager {
	return &toastManager{now: time.Now}
}

// show añade un aviso con el TTL por defecto.
func (t *toastManager) show(message string, level toastLevel) {
	t.showFor(message, level, toastDuration)
}

// showFor añade un aviso con TTL propio.
func (t *toastManager) showFor(message string, level toastLevel, d time.Duration) {
	if message == "" {
		return
	}
	t.toasts = append(t.toasts, toast{message: message, level: level, created: t.now(), duration: d})
}

// replace sustituye el aviso vigente cuyo texto es prev por uno nuevo, con su TTL
// reiniciado. Si no lo encuentra —ya caducó o lo sustituyó otro— apila el nuevo.
// Es lo que permite componer sobre un aviso sin duplicar su texto.
func (t *toastManager) replace(prev, message string, level toastLevel) {
	if message == "" {
		return
	}
	for i := len(t.toasts) - 1; i >= 0; i-- {
		if t.toasts[i].message == prev {
			t.toasts[i] = toast{message: message, level: level, created: t.now(), duration: toastDuration}
			return
		}
	}
	t.show(message, level)
}

// update poda los avisos caducados. Es lo que llama el tick.
func (t *toastManager) update() {
	now := t.now()
	alive := t.toasts[:0]
	for _, x := range t.toasts {
		if now.Sub(x.created) < x.duration {
			alive = append(alive, x)
		}
	}
	t.toasts = alive
}

// texts devuelve los mensajes vivos, para los tests.
func (t *toastManager) texts() []string {
	out := make([]string, 0, len(t.toasts))
	for _, x := range t.toasts {
		out = append(out, x.message)
	}
	return out
}

// last devuelve el mensaje del último aviso, o "" si no hay ninguno.
func (t *toastManager) last() string {
	if len(t.toasts) == 0 {
		return ""
	}
	return t.toasts[len(t.toasts)-1].message
}

// blocks dibuja cada aviso vivo como una caja; es lo que se superpone.
func (t *toastManager) blocks(available int) []string {
	out := make([]string, 0, len(t.toasts))
	for _, x := range t.toasts {
		out = append(out, t.render(x, available))
	}
	return out
}

// render compone la caja de un aviso: icono, texto envuelto y borde.
func (t *toastManager) render(x toast, available int) string {
	icon := toastIcon(x.level)
	// Los tres números de la caja salen de toastGeometry, y el texto se envuelve al
	// que devuelve. La geometría está en su propia función por la razón de siempre:
	// dentro del pintado, un ancho mal calculado no se ve como un ancho mal
	// calculado, se ve como "el aviso ocupa más filas" y el recorte de la
	// superposición se come la diferencia.
	width, wrapAt := toastGeometry(x.message, icon, available)
	lines := wrapText(x.message, wrapAt)

	var b strings.Builder
	for i, line := range lines {
		if i == 0 {
			b.WriteString(styleToast(x.level).Render(icon+" "+line) + "\n")
			continue
		}
		b.WriteString(styleToast(x.level).Render("  "+line) + "\n")
	}
	// lipgloss añade el borde y el padding; el ancho se fija para que todas las
	// líneas del bloque midan lo mismo y el overlay no desalinee la vista.
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(toastBorderColor(x.level))).
		Padding(0, 1).
		Width(width).
		Render(strings.TrimRight(b.String(), "\n"))
}

// toastGeometry son los dos números con los que se compone la caja de un aviso: el
// ancho exterior que se le pasa a lipgloss, y el ancho al que se parte el texto.
//
// Está en su propia función por la razón de siempre: dentro del pintado, un ancho
// mal calculado no se ve como un ancho mal calculado, se ve como "el aviso ocupa
// más filas". Y como la superposición recorta la caja al ancho de la vista, una
// columna de más o de menos se la come el recorte. Siendo aritmética pura se
// comprueba directo, sin pintar nada.
//
// El ancho sale de una ESTIMACIÓN y no de una medida exacta, y eso es deliberado:
// la primera estimación es "el texto más el icono más el marco", acotada por los
// dos límites y por el espacio libre. El que garante que el texto quepa de verdad
// es el envoltorio de abajo, que parte cada línea al ancho útil. Si la estimación
// se pasa, el texto sale partido en más líneas, y eso es mejor que un texto
// recortado a media palabra por el marco.
//
// O sea: la aritmética de aquí decide DÓNDE EMPIEZA el ancho útil, y de ahí solo
// se deduce lo que cabe. El ancho útil es el exterior menos el marco, y el texto se
// parte al ancho útil menos el hueco del icono, porque la primera línea lo lleva
// delante y las siguientes no. De ahí los dos suelos: por debajo de 8 de ancho
// útil no hay texto legible, y por debajo de 4 de partición un aviso corto se
// parte en una palabra por línea, que no informa de nada.
func toastGeometry(message, icon string, available int) (width, wrapAt int) {
	// Ancho total estimado: el texto, el icono, su espacio y el hueco de
	// borde+padding. En lipgloss Width() es el ancho del CONTENIDO, sin borde ni
	// padding, así que el marco va sumando y luego se resta.
	//
	// Cada sumando es una pieza con nombre, y la suma es el contrato: si un término
	// falta o sobra, el aviso se sale o se estrecha de más, y desde el texto
	// superpuesto no se distingue de que la superposición lo recortara.
	width = ansi.StringWidth(message) + ansi.StringWidth(icon) + toastIconSpace + toastFrame
	width = min(max(width, toastMinWidth), toastMaxWidth)

	// Por debajo de este hueco no se intenta encajar: una caja más ancha que la
	// columna donde va a caer la superposición la recorta por la derecha, y una
	// columna tan estrecha no da para un aviso de ancho mínimo.
	if available > toastMinAvailable {
		width = min(width, available)
	}

	// Ancho útil: el exterior menos el marco, con suelo. Y el de partición: el útil
	// menos el hueco del icono, SIN suelo propio.
	//
	// Lo de no ponerle suelo al de partición es una conclusión, no un descuido:
	// inner >= toastMinInner = 8, así que inner - toastIconGap >= 6, y un suelo de
	// 4 o de 5 nunca tocaría. Un suelo que no puede llegar es ruido que además
	// invita a escribir tests que pasan por el suelo y no por la aritmética. El
	// suelo que SÍ importa es el de inner, y ese está arriba.
	inner := max(width-toastFrame, toastMinInner)
	return width, inner - toastIconGap
}

// toastIcon es el glifo del nivel.
func toastIcon(level toastLevel) string {
	switch level {
	case toastSuccess:
		return "✓"
	case toastError:
		return "✗"
	case toastInfo:
		return "ℹ"
	case toastWarning:
		return "⚠"
	default:
		return "•"
	}
}

// toastBorderColor es el color del borde según el nivel.
func toastBorderColor(level toastLevel) string {
	switch level {
	case toastSuccess:
		return "46"
	case toastError:
		return "196"
	case toastInfo:
		return "39"
	case toastWarning:
		return "208"
	default:
		return "244"
	}
}

// styleToast elige el estilo del texto del aviso.
func styleToast(level toastLevel) lipglossStyle {
	switch level {
	case toastSuccess:
		return styleOK
	case toastError:
		return styleError
	case toastInfo:
		return styleInfo
	case toastWarning:
		return styleWarn
	default:
		return styleDim
	}
}

// wrapText parte el texto en líneas de como mucho max columnas, por palabras.
func wrapText(text string, max int) []string {
	if max <= 0 || ansi.StringWidth(text) <= max {
		return []string{text}
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{text}
	}
	lines := []string{words[0]}
	for _, w := range words[1:] {
		last := len(lines) - 1
		if ansi.StringWidth(lines[last])+1+ansi.StringWidth(w) <= max {
			lines[last] += " " + w
			continue
		}
		lines = append(lines, w)
	}
	return lines
}

// overlayToasts superpone los avisos abajo a la derecha de `content`. Se
// recortan por la derecha con ansi.Truncate/TruncateLeft para conservar los
// códigos de color de la línea base: al revés que un simple replace, el texto de
// debajo no se ensucia ni se desalinea.
//
// Solo pinta sobre las filas marcadas en `rows`: el interior de las cajas. Un
// aviso nunca cae sobre un borde, así que ningún marco se rompe por tener un
// aviso encima —que es lo que pasaba con el borde inferior del detalle—. Cada
// aviso busca el hueco más bajo que le quepa, y los siguientes se apilan por
// encima del anterior; si ya no queda interior libre, los que sobren no se
// pintan, porque un aviso ilegible no informa de nada.
func overlayToasts(content string, boxes []string, width int, rows []bool) string {
	if len(boxes) == 0 {
		return content
	}
	lines := strings.Split(content, "\n")
	anchor := len(lines) - 1
	for _, b := range boxes {
		block := strings.Split(b, "\n")
		bw := ansi.StringWidth(block[0])
		bh := toastBlockHeight(len(block), anchor)
		base, ok := landRow(rows, anchor, bh)
		if !ok {
			break
		}
		x := toastColumn(width, bw)
		for j := range bh {
			i := base - bh + 1 + j
			line := lines[i]
			lines[i] = ansi.Truncate(line, x, "") + block[j] + ansi.TruncateLeft(line, x+bw, "")
		}
		anchor = base - bh
	}
	return strings.Join(lines, "\n")
}

// toastBlockHeight es de cuántas filas se pinta un bloque que tiene bh filas,
// cuando solo quedan `anchor` filas por debajo de donde se busca.
//
// El tope es anchor+1 y no anchor: la última fila (índice anchor) es la más baja
// que existe, así que desde ella caben anchor+1 filas contando las de arriba. Sin
// ese +1 una caja de 3 filas en una ventana de 3 intentaría ocupar de la 0 a la 2
// con la base en la 1, y landRow no encontraría sitio y el aviso no se pintaría
// nunca justo en la ventana que está hecha a su medida.
//
// NO lleva suelo a cero, y es a propósito: anchor viene de len(lines)-1 sobre un
// strings.Split, que siempre devuelve al menos una línea, así que anchor >= 0 y el
// suelo no podría tocar. Un suelo que no llega es ruido que además esconde el +1
// de verdad, que es el que hay que comprobar.
//
// Esta función solo decide la cuenta; que haya sitio es cosa de landRow.
func toastBlockHeight(bh, anchor int) int { return min(bh, anchor+1) }

// toastColumn es la columna por la que empieza una caja de bw columnas en una
// vista de width.
//
// Va pegada a la derecha, con UNA columna de aire entre la caja y el borde: un
// aviso que llega al último borde se lee como parte del marco, y no como algo
// encima. El suelo a 0 es para el caso de que la caja sea más ancha que la vista:
// entonces no hay ningún sitio donde no se salga, y empezar en la columna 0 es lo
// menos malo, porque el recorte de la superposición ya lo está tapando por la
// derecha.
//
// En su propia función porque es geometría comprobable: desde el texto
// superpuesto, "la caja se salió una columna" no se distingue de "la caja se salió
// y el recorte lo tapó", que es justo lo que pasa.
func toastColumn(width, bw int) int { return max(width-bw-1, 0) }

// landRow devuelve la fila más baja, sin pasar de anchor, en la que cabe un
// bloque de bh filas que admiten avisos. Devuelve false si no cabe en ninguna.
//
// El bucle arranca en anchor SIN acotarlo a len(rows)-1, y es a propósito. Antes se
// acotaba con un min(anchor, len(rows)-1) que no podía hacer nada: el único llamante
// pasa anchor = len(lines)-1 sobre un rows que la pila construye fila a fila a la
// par que las líneas, así que len(rows) == len(lines) y anchor == len(rows)-1
// siempre. Y tras cada aviso el ancla baja (anchor = base - bh, con bh >= 1 porque
// el bh <= 0 de arriba ya salió), así que nunca vuelve a subir. El clamp era ruido
// que además tapaba lo que sí hay que mirar, que es el paso hacia arriba.
//
// Que un anchor disparado no localice filas es cosa de admitenAviso, que acota cada
// índice contra len(rows) y devuelve false. Por eso quitar el clamp es seguro: el
// índice que se sale de rows no se lee, se rechaza.
func landRow(rows []bool, anchor, bh int) (int, bool) {
	if bh <= 0 {
		return 0, false
	}
	for base := anchor; base >= bh-1; base-- {
		if admitenAviso(rows, base-bh+1, bh) {
			return base, true
		}
	}
	return 0, false
}

// admitenAviso indica si las n filas que empiezan en from son todas interior de
// alguna caja. Una fila que no existe cuenta como que no: es preferible no
// pintar a pintar de más.
func admitenAviso(rows []bool, from, n int) bool {
	for i := from; i < from+n; i++ {
		if i < 0 || i >= len(rows) || !rows[i] {
			return false
		}
	}
	return true
}
