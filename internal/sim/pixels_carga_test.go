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

// `pixels_test.go` ya cubre `Cells` a fondo —el doble de píxeles por celda, el promedio
// frente a la muestra, el upscale, la geometría inválida— y este fichero NO repite nada de
// eso. Solo lo que quedó sin probar: la carga de la imagen, el bloque vacío del promedio, y
// el slug de los nombres de fichero.

const halfGlifo = '▀'

// TestLoadAbrePorElContenidoYNoPorLaExtension: la carga.
//
// Y el caso que da nombre a la función es un PNG con nombre de JPEG. El decodificador va
// por el contenido —eso es lo que dice el comentario del código— así que carga; una
// implementación que eligiera por la extensión fallaría con un error de formato sobre un
// fichero perfectamente válido.
//
// Y no es un caso inventado: `git-sim` produce JPEG y el popup abre la copia con la
// extensión que le puso `keep`, que es `.jpg`. Si alguna vez se cambia ese sufijo sin
// cambiar el contenido, una implementación por extensión se rompe entera y una por
// contenido no se entera.
func TestLoadAbrePorElContenidoYNoPorLaExtension(t *testing.T) {
	dir := t.TempDir()
	contenido := jpegDePrueba(t, 5, 2, color.RGBA{R: 200, G: 100, B: 0, A: 255})

	// Con la extensión que NO corresponde.
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

	// Con la extensión correcta, y sin extensión: los dos casos que de verdad se dan.
	for _, nombre := range []string{"render.jpg", "imagen"} {
		ruta := filepath.Join(dir, nombre)
		if err := os.WriteFile(ruta, contenido, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(ruta); err != nil {
			t.Errorf("%s no cargo: %v", nombre, err)
		}
	}

	// Y un PNG de verdad, que es lo que se produce si se le pide a git-sim otro formato.
	pngRuta := filepath.Join(dir, "otro.png")
	if err := os.WriteFile(pngRuta, pngDePrueba(t, 3, 3, color.RGBA{G: 9, A: 255}), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(pngRuta); err != nil {
		t.Errorf("un PNG real no cargo: %v", err)
	}
}

// TestLoadDistingueNoExisteDeNoEsImagen: los dos fallos, y lo que dice cada uno.
//
// Y son fallos distintos para quien depura, y esa es la razón de separarlos. "No existe"
// dice que el render no llegó a dejar fichero; "no es una imagen" dice que lo dejó corrupto
// o que se escribió a medias. Con un único mensaje de error, quien mira el popup no sabe
// cuál de las dos cosas pasó.
//
// Y el del formato tiene que NOMBRAR el fichero, porque la caché guarda veinte imágenes y
// sin el nombre no hay forma de saber cuál está corrupta.
func TestLoadDistingueNoExisteDeNoEsImagen(t *testing.T) {
	dir := t.TempDir()

	// Inexistente: el error tiene que ser distinguible del de formato, así que sale sin
	// la palabra "decode".
	_, err := Load(filepath.Join(dir, "no-existe.jpg"))
	if err == nil {
		t.Fatal("cargar un fichero inexistente dio nil")
	}
	if strings.Contains(err.Error(), "decode") {
		t.Errorf("un fichero inexistente dio %q, que parece un problema de formato", err)
	}
	// Y el error de os se preserva envuelto, para que `os.IsNotExist` siga funcionando
	// a través de la capa. Es lo que permite que un llamador pueda distinguirlo sin leer
	// el texto.
	if !os.IsNotExist(err) {
		t.Errorf("os.IsNotExist dio false con %v: la causa de os.Open no llega", err)
	}

	// Basura: el error dice "decode" y nombra el fichero.
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
	// Y un fichero vacío, que es lo que deja una copia interrumpida.
	vacio := filepath.Join(dir, "vacio.jpg")
	if err := os.WriteFile(vacio, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(vacio); err == nil {
		t.Error("un fichero vacío dio nil")
	}

	// Y un directorio: falla sin panic. `image.Decode` lee y se queda sin bytes.
	if _, err := Load(dir); err == nil {
		t.Error("cargar un directorio dio nil")
	}
}

// TestElPromedioDeUnBloqueVacioEsElCeroLiteral: la rama `n == 0`.
//
// Y no es defensiva. `average` divide `rs/n` y `n` sale de contar los píxeles del bucle; con
// un bloque de ancho o alto cero el bucle no entra nunca y la división es por cero, o sea un
// panic. El `if n == 0` es lo que evita el panic, y sin un test que lo recorra no hay forma
// de saber que está.
//
// Y el bloque vacío llega de verdad cuando la imagen es más estrecha que el destino —un
// popup muy ancho con una simulación pequeña—, no solo de una llamada sintética.
func TestElPromedioDeUnBloqueVacioEsElCeroLiteral(t *testing.T) {
	src := solid(2, 2, color.RGBA{R: 200, G: 200, B: 200, A: 255})

	// Las tres formas de bloque vacío: ancho cero, alto cero, y ambos.
	bloques := [][4]int{
		{0, 0, 0, 1},
		{0, 0, 1, 0},
		{0, 0, 0, 0},
		// Y uno invertido, que es lo que pasa si el cálculo de la fracción se sale del
		// destino por un redondeo.
		{1, 1, 0, 1},
	}
	for _, b := range bloques {
		got := average(src, b[0], b[1], b[2], b[3])
		if got != (color.RGBA{}) {
			t.Errorf("el bloque vacío %v dio %+v, want el cero literal", b, got)
		}
	}

	// Y el caso bueno al lado, para que la comprobación no pueda pasar por "todo da cero".
	if got := average(src, 0, 0, 1, 1); got != (color.RGBA{R: 200, G: 200, B: 200, A: 255}) {
		t.Errorf("un bloque de un pixel dio %+v", got)
	}
	// Y un bloque de dos píxeles idénticos no cambia nada: promediar lo mismo da lo mismo.
	if got := average(src, 0, 0, 2, 2); got != (color.RGBA{R: 200, G: 200, B: 200, A: 255}) {
		t.Errorf("un bloque uniforme dio %+v", got)
	}
}

// TestElSlugDejaUnNombreDeFicheroYNoUnaRuta: `slug`.
//
// Y la razón de que exista es que el proyecto entra en un nombre de fichero, y una barra
// en un nombre es un directorio. Sin `slug`, "grupo/proyecto" produciría una ruta dentro
// del caché en vez de un fichero, y `keep` acabaría escribiendo en un directorio que no
// existe.
//
// Y lo que se fija no es el texto exacto de cada caso, que es un dato, sino la PROPIEDAD
// que hace falta: lo que sale siempre es un nombre de fichero válido. Un aserto de igualdad
// por caso documenta el comportamiento; un aserto de propiedad lo protege.
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
		// La propiedad: sin separadores de ruta, sin espacios y sin tabuladores. Es lo que
		// evita que `keep` escriba fuera del caché.
		if strings.ContainsAny(got, "/ \t\\:*?\"<>|") {
			t.Errorf("slug(%q) dio %q, que no es un nombre de fichero", entra, got)
		}
		// Y sin guiones en los bordes, que es lo que hace legible la lista del caché: un
		// "-proyecto-" se lee como un prefijo que no existe.
		if strings.HasPrefix(got, "-") || strings.HasSuffix(got, "-") {
			t.Errorf("slug(%q) dio %q, con guiones en los bordes", entra, got)
		}
		vistos[got] = true
	}

	// Y los tres casos que merecen aserto propio, porque son los que se confunden entre
	// sí y no se deducen de la propiedad anterior.
	if got := slug("grupo/proyecto"); got != "grupo-proyecto" {
		t.Errorf("una barra dio %q", got)
	}
	if got := slug("///"); got != "" {
		t.Errorf("solo barras dio %q, want vacío: un nombre de guiones no es un nombre", got)
	}
	// Los caracteres que se conservan a propósito: quitarlos haría que dos proyectos
	// distintos—"mi-proyecto" y "mi.proyecto"— se leyeran igual en el caché, y sus
	// imágenes se pisarían en la poda.
	for _, entra := range []string{"mi-proyecto", "mi_proyecto", "proyecto.git"} {
		if slug(entra) != entra {
			t.Errorf("slug(%q) lo cambió a %q, y esos caracteres se conservan", entra, slug(entra))
		}
	}
}

// TestUnaImagenPaletizadaSeConvierteAntesDePromediar: el camino de conversión.
//
// Y es el que faltaba: `Cells` convierte a RGBA antes de reducir, así que una imagen que no
// es RGBA —un Paletted, que es lo que dan muchos decodificadores para imágenes con pocos
// colores— no falla y no se calcula con el índice de la paleta por color.
//
// Y el resultado tiene que ser el color del Paletted, no el índice. Con la paleta de un solo
// color, un error aquí daría el índice 0 —que suele ser un color cualquiera— en vez del
// color, y una imagen monocroma de un terminal saldría con colores inventados.
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
	// Y el glifo es de medio bloque, que es lo que compra la resolución vertical.
	if n := strings.Count(celdas[0], string(halfGlifo)); n != 2 {
		t.Errorf("la celda trae %d medios bloques, want 2: %q", n, celdas[0])
	}
}

// jpegDePrueba serializa una imagen en JPEG, que es lo que produce git-sim.
func jpegDePrueba(t *testing.T, w, h int, c color.RGBA) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, solid(w, h, c), nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// pngDePrueba serializa una imagen en PNG.
func pngDePrueba(t *testing.T, w, h int, c color.RGBA) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, solid(w, h, c)); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
