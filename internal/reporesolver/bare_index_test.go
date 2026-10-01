package reporesolver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

// TestEnsureBareLimpiaLosRestosDeUnIntentoFallido: un directorio donde debería estar
// el clon pero no es un clon se limpia antes de reintentar.
//
// El clon se publica con un rename, así que un fallo a mitad deja un directorio a
// medias. Si ese directorio se queda, el siguiente intento falla al renombrar
// encima, y el fallo se repite para siempre: el repo queda inservible hasta que
// alguien borre a mano un directorio que no sabe que existe.
//
// El caso que de verdad importa es "directorio con basura dentro", porque uno vacío
// casi lo absorbe todo. Y va con git de verdad, no con un doble: que el clon se
// publique encima de un directorio con contenido es exactamente lo que hay que
// comprobar, y un doble lo comprobaría en la fiction.
func TestEnsureBareLimpiaLosRestosDeUnIntentoFallido(t *testing.T) {
	origin, _ := fixture(t)
	ref := ghRef()
	r := newResolver(t, origin, ref)

	// Se planta un directorio con basura dentro, justo donde va a ir el clon.
	dest := r.barePath(ref)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	basura := filepath.Join(dest, "BASURA")
	if err := os.WriteFile(basura, []byte("restos de un intento fallido"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := r.EnsureBare(t.Context(), ref)
	if err != nil {
		t.Fatalf("EnsureBare con restos previos dio %v, want nil: los restos se limpian antes de reintentar", err)
	}
	if got != dest {
		t.Fatalf("EnsureBare devolvió %q, want %q", got, dest)
	}
	// Y la basura no está: se limpió, no se clonó encima.
	if _, err := os.Stat(basura); err == nil {
		t.Error("la basura del intento anterior sigue ahí: el directorio se usó sin limpiar")
	}
	// Y lo que hay ahora sí es un clon, que es lo que se pidió.
	if !isRepo(dest) {
		t.Error("donde debería estar el clon no hay un clon")
	}
}

// TestEnsureBareConUnRestoVacioTambiénFunciona: un directorio vacío es un resto igual
// que uno con basura.
//
// Va aparte porque el caso contiguo más probable es justo este: el clone falla y deja
// el temporal a medias, y el destino ya existe vacío de un intento anterior.
func TestEnsureBareConUnRestoVacioTambiénFunciona(t *testing.T) {
	origin, _ := fixture(t)
	ref := ghRef()
	r := newResolver(t, origin, ref)

	dest := r.barePath(ref)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := r.EnsureBare(t.Context(), ref); err != nil {
		t.Fatalf("EnsureBare con un directorio vacío dio %v, want nil", err)
	}
	if !isRepo(dest) {
		t.Error("donde debería estar el clon no hay un clon")
	}
}

// TestBuildIndexNoEntraEnDirectoriosOcultosNiSaltaLaRaiz: los ocultos se podan, y la
// raíz NO se poda aunque su nombre empiece por punto.
//
// Son las dos mitades de UNA condición, y por eso van en el mismo test: la regla es
// "oculto Y no soy la raíz", y cada mitad se rompe por un lado distinto.
//
// La primera mitad existe para que el índice no se llene de repos dentro de cachés y
// carpetas de editor. Un clon ahí no es del usuario en el sentido que importa: no lo
// sabe que existe, y resolver un ítem ahi lo dejaría trabajando en un sitio que no ha
// elegido.
//
// La segunda mitad es lo que se pierde con un "==" en vez de un "!=": un root oculto
// es un caso NORMAL —el usuario lo ha configurado, se llama así porque sí— y la regla
// de podar los ocultos existe para no meterse en las carpetas del sistema, no para
// desobedecer lo que se le dijo explícitamente. Un root que el usuario nombró se
// indexa siempre.
func TestBuildIndexNoEntraEnDirectoriosOcultosNiSaltaLaRaiz(t *testing.T) {
	// Dos orígenes distintos para poder distinguir qué repo se indexó: si los dos
	// apuntaran al mismo, el índice tendría una sola entrada y no se sabría cuál.
	origenVisible := filepath.Join(t.TempDir(), "visible.git")
	testutil.InitBare(t, origenVisible)
	origenOculto := filepath.Join(t.TempDir(), "oculto.git")
	testutil.InitBare(t, origenOculto)

	refVisible := ghRef()
	refOculto := glRef("gitlab.example.com", "grupo/escondido")

	// La raíz es un directorio oculto, y dentro hay un repo visible y otro debajo de
	// un directorio oculto. Así el mismo árbol prueba las dos mitades.
	root := filepath.Join(t.TempDir(), ".workspace")

	visible := filepath.Join(root, "proyecto")
	testutil.InitRepo(t, visible)
	testutil.CommitFile(t, visible, "a.txt", "a", "a")
	testutil.SetRemote(t, visible, "origin", origenVisible)

	escondido := filepath.Join(root, ".cache", "escondido")
	testutil.InitRepo(t, escondido)
	testutil.CommitFile(t, escondido, "b.txt", "b", "b")
	testutil.SetRemote(t, escondido, "origin", origenOculto)

	r := New(Options{
		Roots:    []string{root},
		CloneDir: filepath.Join(t.TempDir(), "repos"),
		MemoPath: filepath.Join(t.TempDir(), "memo.json"),
		Hosts:    map[string]string{"github.com": "github", "gitlab.example.com": "gitlab"},
		ParseRemote: func(raw string) (model.RepoRef, bool) {
			// El runner devuelve la salida de git CON su salto final, así que hay que
			// recortar. El ParseRemoteURL de verdad lo hace (por eso funciona con
			// git real); aquí, que es un cierre de test, hay que hacerlo a mano.
			switch strings.TrimSpace(raw) {
			case origenVisible:
				return refVisible, true
			case origenOculto:
				return refOculto, true
			}
			return model.RepoRef{}, false
		},
	})

	// La raíz oculta se indexa: se leفيرó al usuario, así que su voluntad manda.
	if got, ok := r.ResolveLocal(refVisible); !ok || got != visible {
		t.Errorf("ResolveLocal del repo visible = %q, %v; quiero %q: un root se indexa aunque su nombre empiece por punto",
			got, ok, visible)
	}
	// Y el repo que vive dentro de un directorio oculto no se indexa.
	if got, ok := r.ResolveLocal(refOculto); ok {
		t.Errorf("ResolveLocal encontró %q bajo un directorio oculto: un clon dentro de una caché no es un clon del usuario",
			got)
	}
}
