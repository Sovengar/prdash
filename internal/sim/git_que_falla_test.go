package sim

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/forge/model"
	"prdash/internal/gitcmd"
)

// Los fallos de git del staging, con un `git` de mentira.
//
// Y el motivo del doble está en el campo `Service.Git`: `gitcmd.Runner.Bin` permite apuntar a
// otro binario, así que un `git` que reenvía todo al de verdad menos un subcomando reproduce
// los tres fallos que el staging tiene que avisar —clonar, materializar una rama y hacer el
// checkout— sin tocar la geometría del repo ni esperar a que algo se corrompa.
//
// Y la razón por la que esto no es solo cobertura: los tres errores son `fmt.Errorf` con el
// motivo de git dentro, y ese texto es lo que ve el usuario en el popup de simulación. Sin
// poder provocarlos, esos tres `wrap` eran decoración sin verificar: si `git` devolviera un
// motivo inútil, el wrap lo copiaría tal cual y nadie se enteraría. Aquí se comprueba que el
// subcomando Y el motivo llegan, y que ninguno de los tres se confunde con los otros.

// gitQueFallaEn es un `git` que delega en el de verdad y sale con código 1 —y un mensaje en
// stderr— cuando el primer argumento no esperado es `sub`.
//
// Y el criterio es "el primer argumento no esperado", no "alguno de los argumentos", porque un
// test que rompe `branch` no quiere romper `clone`: el staging llega al fallo DESPUÉS de haber
// clonado, y si el clon fallara el test probaría el error de clon en el sitio del error de
// branch, que es el despiste más fácil de cometer al escribir esto.
//
// Y el script se escribe con comillas simples alrededor del `printf` porque el mensaje lleva
// comillas dobles, y al revés de como está aquí se lo comería el shell.
func gitQueFallaEn(t *testing.T, sub string) *gitcmd.Runner {
	t.Helper()
	dir := t.TempDir()
	guion := filepath.Join(dir, "git")

	// El mensaje sale por stderr porque es de ahí de donde `gitcmd.Runner` lo lee: si fuera
	// por stdout, el aserto vería el texto sin motivo —`exec: ...: exit status 1`— y no
	// probaría nada del `wrap`.
	contenido := "#!/bin/sh\n" +
		"for arg in \"$@\"; do\n" +
		"  if [ \"$arg\" = \"" + sub + "\" ]; then\n" +
		"    printf '%s\\n' 'fatal: el subcomando " + sub + " no se pudo completar' >&2\n" +
		"    exit 1\n" +
		"  fi\n" +
		"done\n" +
		"exec git \"$@\"\n"
	if err := os.WriteFile(guion, []byte(contenido), 0o755); err != nil {
		t.Fatal(err)
	}
	return &gitcmd.Runner{Bin: guion, Timeout: gitcmd.DefaultTimeout}
}

// servicioConGitMalo monta el servicio completo —localizador, caché y git-sim de mentira— con
// el repositorio de siempre, y solo le cambia el runner de git.
func servicioConGitMalo(t *testing.T, repo, rama, sub string) *Service {
	t.Helper()
	loc := locatorFalso{ok: true, place: Place{Repo: repo, Branch: rama}}
	s := newService(t, loc, fakeSim(t, writeJPEG(t)))
	s.Git = gitQueFallaEn(t, sub)
	return s
}

// TestLosTresFallosDeGitDelStagingSeAvisanConSuSubcomandoYSuMotivo: los tres `wrap`.
//
// Y los tres juntos en un solo test porque el valor está en la comparación: cada uno tiene que
// decir SU subcomando y NINGUNO puede decir el de otro. Un `wrap` que metiera el motivo de
// git sin el subcomando, o que copiara el mensaje del error anterior, daría el mismo texto en
// los tres casos y el usuario vería "check out" cuando lo que falló fue el branch.
//
// Y los tres fallos se provocan en su sitio, con el staging llegando hasta ellos:
//
//   - `clone` falla en el primero de todos, así que el fixture tiene que ser un repo real: si
//     no lo fuera, el clon fallaría antes por otra razón y el test probaría el error equivocado.
//   - `branch` falla en el segundo, con el clon ya hecho y la rama del ítem ya materializada.
//   - `checkout` falla en el tercero, con las dos ramas ya en su sitio.
//
// Y lo que se comprueba en los tres casos, además del texto, es que la simulación NO continúa:
// un error que avisara y siguiera dejaría una imagen de un grafo que no se pudo preparar, y eso
// es peor que no avisar, porque el popup abriría una imagen que no corresponde a nada.
func TestLosTresFallosDeGitDelStagingSeAvisanConSuSubcomandoYSuMotivo(t *testing.T) {
	// Y lo que se comprueba NO es la palabra "clone" suelta, sino la invocación completa que
	// `gitcmd.Error` imprime: `Args` van unidos por espacios detrás del `git -C <dir>`. Y eso
	// es a propósito, porque mi primera versión buscaba la palabra suelta y fallaba con dos
	// errores que no eran del test:
	//
	//   - "clone" aparece en el texto de los OTROS dos errores, dentro de "the simulation
	//     clone", que es el nombre del directorio de trabajo. Un aserto por palabra suelta
	//     habría prohibido el wrap correcto por una palabra que no nombra el subcomando.
	//   - "branch" aparece en "create the branch main-origin", que es el wrap correcto del
	//     fallo de branch y no el de ningún otro.
	//
	// Con la invocación entera las dos colisiones desaparecen: `clone --quiet --shared` solo
	// está en el error del clon, `branch --quiet` solo en el de la rama, y `checkout --quiet`
	// solo en el del checkout. Y así el aserto no depende de cómo se redacte el wrap.
	for _, c := range []struct {
		nombre  string
		sub     string
		quiere  []string
		ausente []string
	}{
		{
			nombre:  "el clon falla",
			sub:     "clone",
			quiere:  []string{"clone --quiet --shared", "clone the local refs of"},
			ausente: []string{"branch --quiet", "checkout --quiet"},
		},
		{
			nombre: "la rama del ítem no se puede materializar",
			sub:    "branch",
			// Y el nombre de la rama del ítem, no el de la base: `materialize` va primero
			// con la del ítem, así que es la que falla. Si el test dijera "main" pasaría
			// también con un fallo de la base, que es otro camino.
			quiere:  []string{"branch --quiet main-origin origin/main-origin", "create the branch main-origin"},
			ausente: []string{"clone --quiet", "checkout --quiet"},
		},
		{
			nombre:  "el checkout de la rama activa falla",
			sub:     "checkout",
			quiere:  []string{"checkout --quiet main", "check out main in the simulation clone"},
			ausente: []string{"clone --quiet", "branch --quiet"},
		},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			repo, _ := simRepoMonta(t)
			it := model.NewItem(model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget"}, 7)
			it.TargetBranch = "main"
			s := servicioConGitMalo(t, repo, "main-origin", c.sub)

			res, err := s.Simulate(context.Background(), it, KindMerge)
			if err == nil {
				t.Fatalf("Simulate con %s roto devolvió nil y una imagen en %s: se publicaría "+
					"un grafo que no se pudo preparar", c.sub, res.Path)
			}
			msg := err.Error()
			for _, q := range c.quiere {
				if !strings.Contains(msg, q) {
					t.Errorf("el aviso %q no dice %q: el wrap no aporta el contexto", msg, q)
				}
			}
			for _, n := range c.ausente {
				if strings.Contains(msg, n) {
					t.Errorf("el aviso %q trae %q, que es la invocación de otro fallo: el "+
						"usuario iría a mirar lo que no falló", msg, n)
				}
			}
			// Y el motivo de git llega dentro del wrap, que es lo que separa "no se pudo
			// completar" de un motivo accionable.
			if !strings.Contains(msg, "no se pudo completar") {
				t.Errorf("el aviso %q no trae el motivo de git", msg)
			}
			// Y no queda imagen: el error aborta antes de `keep`, así que no hay un popup
			// con un fichero que abrir y que no corresponde a nada.
			if res.Path != "" {
				t.Errorf("con %s roto se devolvió la ruta %s", c.sub, res.Path)
			}
		})
	}
}

// TestElCheckoutUsaLaRamaQueDiceElModoYElMensajeLaNombra: el nombre que aparece en el aviso.
//
// Y esto es lo que `stage` decide —base activa para integrar, rama del ítem para rebasar— y es
// la clase de detalle que un `wrap` puede perder: si el error dijera "check out" a secas, el
// usuario no sabría si falló su rama o la base, que es justo lo que necesita saber para
// arreglarlo.
//
// Y los dos modos se comprueban con el mismo repo, así que la rama que falla la decide el
// `kind` y no el fixture.
func TestElCheckoutUsaLaRamaQueDiceElModoYElMensajeLaNombra(t *testing.T) {
	for _, c := range []struct {
		kind Kind
		base string
	}{
		{KindMerge, "main"},
		{KindRebase, "main-origin"},
	} {
		repo, _ := simRepoMonta(t)
		it := model.NewItem(model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget"}, 7)
		it.TargetBranch = "main"
		s := servicioConGitMalo(t, repo, "main-origin", "checkout")

		_, err := s.Simulate(context.Background(), it, c.kind)
		if err == nil {
			t.Fatalf("%s con el checkout roto devolvió nil", c.kind)
		}
		if !strings.Contains(err.Error(), "check out "+c.base) {
			t.Errorf("%s: el aviso %q no nombra la rama activa (%s)", c.kind, err, c.base)
		}
	}
}

// TestKeepSinDirectorioDeCacheSeQuejaEnVezDeAdivinarDondeGuardar: el `cacheDir` que falla.
//
// Y `cacheDir` llama a `os.UserCacheDir`, que devuelve error cuando NO hay `$XDG_CACHE_HOME` y
// NO hay `$HOME` —o `$HOME` está vacío— y es el único `os.UserCacheDir` del proyecto que se
// puede poner en esa situación sin権限: no necesita ser root ni un contenedor.
//
// Y el caso de por qué se llega a él llamando a `keep` DIRECTO y no a `Simulate` es que
// `Simulate` llama a `cacheDir` DESPUÉS de clonar y de materializar, o sea que el fixture
// entero —un repo real, un clon, dos ramas, un git-sim que renderiza un JPEG— se monta para no
// llegar nunca. `keep` es una función con su contrato propio, con su error propio, y se puede
// probar sola.
//
// Y la consecuencia de no tener caché no es "guardar en el sitio de antes": es no guardar. Un
// `keep` que ante un `UserCacheDir` caído cayera a `/tmp` llenaría el temporal del sistema de
// ficheros del usuario con imágenes de simulaciones que nadie va a abrir, y los borraría al
// reiniciar. Avisar es lo correcto.
func TestKeepSinDirectorioDeCacheSeQuejaEnVezDeAdivinarDondeGuardar(t *testing.T) {
	// Sin `HOME` ni `XDG_CACHE_HOME`: lo que hace que `os.UserCacheDir` no tenga de dónde
	// tirar. Y vacíos, no solo ausentes, porque un `HOME=""` exportado es lo que pasa en un
	// contenedor mal configurado y `os.UserCacheDir` lo trata como no puesto.
	t.Setenv("HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")

	// Y `CacheDir` vacío a propósito, que es lo que hace que el servicio NO sepa ya el
	// directorio y tenga que preguntarle al sistema. Con `CacheDir` puesto, este guard no se
	// toca nunca —ni siquiera con el entorno roto—.
	s := &Service{}
	it := model.NewItem(model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget"}, 7)

	dst, err := s.keep("no-existe.jpg", it, KindMerge)
	if err == nil {
		t.Fatalf("keep sin caché devolvió nil y la ruta %q: alguien guardaría la imagen en un "+
			"lugar que no es el suyo", dst)
	}
	if dst != "" {
		t.Errorf("keep devolvió la ruta %q junto con el error: el llamador avisaría de un "+
			"guardado que no ocurrió", dst)
	}
	// Y el wrap dice qué se buscaba, para que el aviso no sea el `exec: ...` de otro sitio.
	if !strings.Contains(err.Error(), "locate the simulation cache") {
		t.Errorf("el aviso %q no dice que es el caché de simulaciones", err)
	}
	// Y la causa de `UserCacheDir` sigue dentro, porque "no hay caché" y "el caché está en un
	// sitio que no se puede escribir" son el mismo texto y arreglos distintos.
	if !strings.Contains(err.Error(), "HOME") {
		t.Errorf("el aviso %q no trae la causa de os.UserCacheDir, que es lo que nombra la "+
			"variable que falta", err)
	}

	// Y el lado bueno del mismo servicio, para que se vea que el fallo es del entorno y no de
	// `keep`: con `CacheDir` puesto, el mismo ítem y la misma imagen ausente fallan por la
	// imagen, no por el caché.
	s.CacheDir = t.TempDir()
	if _, err := s.keep("no-existe.jpg", it, KindMerge); err == nil ||
		!strings.Contains(err.Error(), "keep the simulation image") {
		t.Errorf("con CacheDir puesto el aviso %q es el del caché y no el de la imagen ausente",
			err)
	}
}
