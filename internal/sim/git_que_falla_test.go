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

// It fails by SUBCOMMAND and not by argument, because the staging reaches the failure AFTER
// cloning: breaking clone too would test the clone error where the branch error belongs.
func gitQueFallaEn(t *testing.T, sub string) *gitcmd.Runner {
	t.Helper()
	dir := t.TempDir()
	guion := filepath.Join(dir, "git")

	// The message goes through stderr because that is where gitcmd.Runner reads it.
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

func servicioConGitMalo(t *testing.T, repo, rama, sub string) *Service {
	t.Helper()
	loc := locatorFalso{ok: true, place: Place{Repo: repo, Branch: rama}}
	s := newService(t, loc, fakeSim(t, writeJPEG(t)))
	s.Git = gitQueFallaEn(t, sub)
	return s
}

// All three in one test, because the value is in the comparison: each names its own subcommand and none
// may name another's.
func TestLosTresFallosDeGitDelStagingSeAvisanConSuSubcomandoYSuMotivo(t *testing.T) {
	// Not the loose word "clone" but the full invocation gitcmd.Error prints. The loose word collides:
	//"clone" appears inside "the simulation clone", and "branch" inside "create the branch".
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
			// The item's branch name, not the base's: materialize goes first with it.
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
			// And git's reason is inside the wrap, which is what tells "could not complete" from a generic
			// failure.
			if !strings.Contains(msg, "no se pudo completar") {
				t.Errorf("el aviso %q no trae el motivo de git", msg)
			}
			if res.Path != "" {
				t.Errorf("con %s roto se devolvió la ruta %s", c.sub, res.Path)
			}
		})
	}
}

// Which branch appears in the warning is what this decides, and it is what the user needs to fix it.
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

// Called directly rather than through Simulate: Simulate asks for the cache dir AFTER cloning and
// materialising, so the whole fixture would be built to never arrive.
func TestKeepSinDirectorioDeCacheSeQuejaEnVezDeAdivinarDondeGuardar(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")

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
	// The wrap says what was being looked for, so the warning is not someone else's `exec: ...`.
	if !strings.Contains(err.Error(), "locate the simulation cache") {
		t.Errorf("el aviso %q no dice que es el caché de simulaciones", err)
	}
	// And UserCacheDir's cause stays inside, because "no cache" and "the cache is in an unreadable
	// place" are different.
	if !strings.Contains(err.Error(), "HOME") {
		t.Errorf("el aviso %q no trae la causa de os.UserCacheDir, que es lo que nombra la "+
			"variable que falta", err)
	}

	s.CacheDir = t.TempDir()
	if _, err := s.keep("no-existe.jpg", it, KindMerge); err == nil ||
		!strings.Contains(err.Error(), "keep the simulation image") {
		t.Errorf("con CacheDir puesto el aviso %q es el del caché y no el de la imagen ausente",
			err)
	}
}
