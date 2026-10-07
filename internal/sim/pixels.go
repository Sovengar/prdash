package sim

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"strings"
)

const halfBlock = "▀"

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
			fmt.Fprintf(&b, "\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm%s\x1b[0m",
				tr>>8, tg>>8, tb>>8, br>>8, bg>>8, bb>>8, halfBlock)
		}
		lines = append(lines, b.String())
	}
	return lines
}

func Fit(img image.Image, maxCols, maxRows int) (cols, rows int) {
	return FitCells(img, 1, 2, maxCols, maxRows)
}

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

	perRow := float64(b.Dx()) / float64(b.Dy()) * float64(cellH) / float64(cellW)

	if float64(maxRows)*perRow <= float64(maxCols) {
		return max(int(float64(maxRows)*perRow), 1), maxRows
	}
	return maxCols, max(int(float64(maxCols)/perRow), 1)
}

func Resize(img image.Image, w, h int) *image.RGBA {
	if w <= 0 || h <= 0 {
		return nil
	}
	src := rgba(img)
	if src == nil {
		return nil
	}
	return shrink(src, w, h)
}

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
