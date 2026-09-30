package sim

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg" // el registro de jpeg es lo que permite decodificar la imagen
	_ "image/png"  // git-sim acepta también PNG y el registro no cuesta nada
	"os"
	"strings"
)

// halfBlock dibuja la mitad superior de una celda: con el foreground y el
// background truecolor se Gets dos píxeles por celda, el doble de resolución
// vertical que un bloque entero, que es lo que hace legible un grafo de commits.
const halfBlock = "▀"

// Load abre y decodifica una imagen. Acepta lo que git-sim produce (JPEG por
// defecto, PNG si se le pidió): el decodificador va por el contenido, no por la
// extensión, así que un nombre con el sufijo equivocado no lo rompe.
func Load(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	return img, nil
}

// Cells dibuja la imagen en w columnas y h líneas de terminal, dos píxeles
// verticales por celda.
//
// Reduce por promedio de caja, no por vecino más cercano: una muestra puntual
// deja los trazos finos de un grafo rotos en un mullón, que es justo el detalle
// que se viene a mirar. Cada celda es autocontenida (fija sus dos colores y
// reinicia al final), así que el texto de alrededor no se ensucia.
func Cells(img image.Image, w, h int) []string {
	if img == nil || w <= 0 || h <= 0 {
		return nil
	}
	src := rgba(img)
	if src == nil {
		return nil
	}
	small := shrink(src, w, 2*h)

	lines := make([]string, 0, h)
	for y := range h {
		var b strings.Builder
		for x := range w {
			tr, tg, tb, _ := small.At(x, 2*y).RGBA()
			br, bg, bb, _ := small.At(x, 2*y+1).RGBA()
			// El rango de At es 0..65535 y el de las celdas 0..255.
			fmt.Fprintf(&b, "\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm%s\x1b[0m",
				tr>>8, tg>>8, tb>>8, br>>8, bg>>8, bb>>8, halfBlock)
		}
		lines = append(lines, b.String())
	}
	return lines
}

// Fit calcula de cuántas columnas y líneas se puede dibujar la imagen sin
// deformarla dentro de un área de maxCols × maxRows celdas, y devuelve el mayor
// tamaño que cabe. Asume celdas 1×2; para el ratio real del terminal está
// FitCells.
func Fit(img image.Image, maxCols, maxRows int) (cols, rows int) {
	return FitCells(img, 1, 2, maxCols, maxRows)
}

// FitCells es Fit con la relación de aspecto real de la celda, que es lo que
// decide cuántas columnas por línea necesita la imagen. No es 2:1 sino lo que
// mida el terminal —en kitty con la fuente por defecto son 9×19 px— y suponerlo
// introduce un error de un 5% en el tamaño, que es justo el tipo de error que hace
// que algo "casi cuadre".
//
// El alto se paga a double: una imagen 16:9 con celdas 1×2 necesita 3,56 columnas
// por línea, no 1,78. Sin el factor, un grafo se estiraría a lo ancho y los dos
// commits de una fila se verían como una tira de elipses en vez de dos círculos.
//
// Se usa el mayor tamaño que cabe en lugar de rellenar el área: una celda vacía a
// un lado de la imagen es del borde del popup, que es donde se lee que la imagen
// termina.
func FitCells(img image.Image, cellW, cellH, maxCols, maxRows int) (cols, rows int) {
	if img == nil || maxCols <= 0 || maxRows <= 0 {
		return max(maxCols, 0), max(maxRows, 0)
	}
	b := img.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 {
		return maxCols, maxRows
	}
	if cellW <= 0 {
		cellW = 1
	}
	if cellH <= 0 {
		cellH = 2
	}

	// cols por línea: la imagen es imgW/imgH de ancha, y una celda es cellH/cellW
	// de alta, así que cada fila de celdas "consume" imgW/imgH * cellH/cellW
	// columnas.
	perRow := float64(b.Dx()) / float64(b.Dy()) * float64(cellH) / float64(cellW)
	if perRow <= 0 {
		return maxCols, maxRows
	}

	if float64(maxRows)*perRow <= float64(maxCols) {
		// El alto manda: se usan todas las líneas disponibles.
		return max(int(float64(maxRows)*perRow), 1), maxRows
	}
	// La anchura manda: se llena de alto lo que la imagen permita.
	return maxCols, max(int(float64(maxCols)/perRow), 1)
}

// Resize devuelve la imagen ajustada a w × h píxeles por promedio de caja, en
// RGBA. Es el mismo reescalado que usan las celdas del half-block, expuesto para
// el camino de la capa de gráficos: mandar la imagen a Herdr sin ajustarla sería
// mandar 1920×1080 para pintar un rectángulo de 800 px y wasting 5× el ancho de
// banda base64.
func Resize(img image.Image, w, h int) *image.RGBA {
	if w <= 0 || h <= 0 {
		return nil
	}
	// El nil y la imagen de tamaño cero se descartan dentro de rgba, no con un
	// `img == nil` propio: pedirse los bounds de un nil es un panic, así que un
	// comprobante aquí, DESPUÉS de la llamada, no cubría nada.
	src := rgba(img)
	if src == nil {
		return nil
	}
	return shrink(src, w, h)
}

// rgba normaliza a *image.RGBA para poder leer píxeles por índice. Devuelve nil
// para lo que no se puede promediar: una imagen inexistente (pedirle los bounds
// sería un panic) y una vacía (bounds de tamaño cero), que no tiene nada que
// dibujar.
func rgba(img image.Image) *image.RGBA {
	if img == nil {
		return nil
	}
	b := img.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 {
		return nil
	}
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Bounds(), img, b.Min, draw.Src)
	return out
}

// shrink promedia el rectángulo de origen de cada píxel destino. Las fracciones
// enteras pueden quedar vacías en destino muy grande respecto del original, y
// ahí se copia el píxel de la esquina en vez de dejar el negro: un píxel de
// ruido en una imagen diminuta se lee como una mota que el render sí tenía.
//
// `src` siempre viene de rgba, que deja los bounds en el origen: por eso los
// índices no suman el Min. Sumarlo no costaba nada, pero era aritmética que ya no
// podía cambiar el resultado, y es exactamente la clase de valor que se puede
// mutar sin que ninguna prueba se entere.
func shrink(src *image.RGBA, w, h int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	sb := src.Bounds()
	for y := range h {
		y0 := y * sb.Dy() / h
		y1 := (y + 1) * sb.Dy() / h
		if y1 <= y0 {
			y1 = min(y0+1, sb.Max.Y)
		}
		for x := range w {
			x0 := x * sb.Dx() / w
			x1 := (x + 1) * sb.Dx() / w
			if x1 <= x0 {
				x1 = min(x0+1, sb.Max.X)
			}
			dst.SetRGBA(x, y, average(src, x0, y0, x1, y1))
		}
	}
	return dst
}

// average promedia el bloque [x0,x1) × [y0,y1) en un solo píxel.
func average(src *image.RGBA, x0, y0, x1, y1 int) color.RGBA {
	var rs, gs, bs, as, n uint64
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			c := src.RGBAAt(x, y)
			rs += uint64(c.R)
			gs += uint64(c.G)
			bs += uint64(c.B)
			as += uint64(c.A)
			n++
		}
	}
	if n == 0 {
		return color.RGBA{}
	}
	return color.RGBA{
		R: uint8(rs / n),
		G: uint8(gs / n),
		B: uint8(bs / n),
		A: uint8(as / n),
	}
}
