package sim

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const halfGlifo = '▀'

// The case that names the function: a PNG with the wrong suffix.
func TestLoadAbrePorElContenidoYNoPorLaExtension(t *testing.T) {
	dir := t.TempDir()
	contenido := jpegDePrueba(t, 5, 2, color.RGBA{R: 200, G: 100, B: 0, A: 255})

	mentiroso := filepath.Join(dir, "render.png")
	if err := os.WriteFile(mentiroso, contenido, 0o644); err != nil {
		t.Fatal(err)
	}
	img, err := Load(mentiroso)
	if err != nil {
		t.Fatalf("un JPEG con nombre .png no cargo: %v", err)
	}
	if img.Bounds().Dx() != 5 || img.Bounds().Dy() != 2 {
		t.Errorf("cargo %v, want 5x2", img.Bounds())
	}

	for _, nombre := range []string{"render.jpg", "imagen"} {
		ruta := filepath.Join(dir, nombre)
		if err := os.WriteFile(ruta, contenido, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(ruta); err != nil {
			t.Errorf("%s no cargo: %v", nombre, err)
		}
	}

	pngRuta := filepath.Join(dir, "otro.png")
	if err := os.WriteFile(pngRuta, pngDePrueba(t, 3, 3, color.RGBA{G: 9, A: 255}), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(pngRuta); err != nil {
		t.Errorf("un PNG real no cargo: %v", err)
	}
}

// Two failures and what each says.
func TestLoadDistingueNoExisteDeNoEsImagen(t *testing.T) {
	dir := t.TempDir()

	// Missing: the error has to be distinguishable from the format one, so it leaves the suffix out.
	_, err := Load(filepath.Join(dir, "no-existe.jpg"))
	if err == nil {
		t.Fatal("cargar un fichero inexistente dio nil")
	}
	if strings.Contains(err.Error(), "decode") {
		t.Errorf("un fichero inexistente dio %q, que parece un problema de formato", err)
	}
	// os's error is preserved wrapped, so os.IsNotExist still works through it.
	if !os.IsNotExist(err) {
		t.Errorf("os.IsNotExist dio false con %v: la causa de os.Open no llega", err)
	}

	basura := filepath.Join(dir, "basura.png")
	if err := os.WriteFile(basura, []byte("esto no es una imagen"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = Load(basura)
	if err == nil {
		t.Fatal("cargar basura dio nil")
	}
	if !strings.Contains(err.Error(), "decode") {
		t.Errorf("el error %q no dice que es de decodificación", err)
	}
	if !strings.Contains(err.Error(), "basura.png") {
		t.Errorf("el error %q no nombra el fichero", err)
	}
	vacio := filepath.Join(dir, "vacio.jpg")
	if err := os.WriteFile(vacio, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(vacio); err == nil {
		t.Error("un fichero vacío dio nil")
	}

	if _, err := Load(dir); err == nil {
		t.Error("cargar un directorio dio nil")
	}
}

// Not defensive: average divides by n, and the only way n is zero is an empty block.
func TestElPromedioDeUnBloqueVacioEsElCeroLiteral(t *testing.T) {
	src := solid(2, 2, color.RGBA{R: 200, G: 200, B: 200, A: 255})

	bloques := [][4]int{
		{0, 0, 0, 1},
		{0, 0, 1, 0},
		{0, 0, 0, 0},
		{1, 1, 0, 1},
	}
	for _, b := range bloques {
		got := average(src, b[0], b[1], b[2], b[3])
		if got != (color.RGBA{}) {
			t.Errorf("el bloque vacío %v dio %+v, want el cero literal", b, got)
		}
	}

	if got := average(src, 0, 0, 1, 1); got != (color.RGBA{R: 200, G: 200, B: 200, A: 255}) {
		t.Errorf("un bloque de un pixel dio %+v", got)
	}
	if got := average(src, 0, 0, 2, 2); got != (color.RGBA{R: 200, G: 200, B: 200, A: 255}) {
		t.Errorf("un bloque uniforme dio %+v", got)
	}
}

// The reason slug exists: the project goes into a filename.
func TestElSlugDejaUnNombreDeFicheroYNoUnaRuta(t *testing.T) {
	entradas := []string{
		"grupo/proyecto", "a//b", "mi proyecto", "proyecto2026",
		"mi-proyecto", "mi_proyecto", "proyecto.git",
		"/proyecto/", "///", "", "proyecto+1", "GRUPO/PROYECTO",
		"con:punto:dos", "con\ttabulación", "con\u00f1",
	}
	vistos := map[string]bool{}
	for _, entra := range entradas {
		got := slug(entra)
		// The property: no path separators, no spaces, no tabs, which is what keeps the name usable.
		if strings.ContainsAny(got, "/ \t\\:*?\"<>|") {
			t.Errorf("slug(%q) dio %q, que no es un nombre de fichero", entra, got)
		}
		// And no leading dashes, which is what keeps the cache listing readable.
		if strings.HasPrefix(got, "-") || strings.HasSuffix(got, "-") {
			t.Errorf("slug(%q) dio %q, con guiones en los bordes", entra, got)
		}
		vistos[got] = true
	}

	if got := slug("grupo/proyecto"); got != "grupo-proyecto" {
		t.Errorf("una barra dio %q", got)
	}
	if got := slug("///"); got != "" {
		t.Errorf("solo barras dio %q, want vacío: un nombre de guiones no es un nombre", got)
	}
	for _, entra := range []string{"mi-proyecto", "mi_proyecto", "proyecto.git"} {
		if slug(entra) != entra {
			t.Errorf("slug(%q) lo cambió a %q, y esos caracteres se conservan", entra, slug(entra))
		}
	}
}

// The missing path: Cells assumes RGBA.
func TestUnaImagenPaletizadaSeConvierteAntesDePromediar(t *testing.T) {
	pal := color.Palette{color.RGBA{R: 0, G: 0, B: 0, A: 0}, color.RGBA{R: 77, G: 0, B: 0, A: 255}}
	img := image.NewPaletted(image.Rect(0, 0, 2, 2), pal)
	for y := range 2 {
		for x := range 2 {
			img.SetColorIndex(x, y, 1)
		}
	}

	celdas := Cells(img, 2, 2)
	if len(celdas) != 2 {
		t.Fatalf("salieron %d celdas, want 2", len(celdas))
	}
	for i, c := range celdas {
		if !strings.Contains(c, "38;2;77;0;0") {
			t.Errorf("la celda %d no trae el color del Paletted: %q", i, c)
		}
	}
	if n := strings.Count(celdas[0], string(halfGlifo)); n != 2 {
		t.Errorf("la celda trae %d medios bloques, want 2: %q", n, celdas[0])
	}
}

func jpegDePrueba(t *testing.T, w, h int, c color.RGBA) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, solid(w, h, c), nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func pngDePrueba(t *testing.T, w, h int, c color.RGBA) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, solid(w, h, c)); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
