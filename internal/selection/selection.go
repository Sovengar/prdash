// Package selection persiste el ítem seleccionado en la TUI para que los
// subcomandos del plugin invocados por teclado (sin `clicked_url`) puedan
// operar sobre él. Es estado efímero de UI: si falta, está corrupto o es
// obsoleto, el llamador lo trata como "sin selección" y da un error claro.
package selection

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"prdash/internal/forge/model"
)

// FileName es el fichero de selección dentro del dir XDG de estado.
const FileName = "selection.json"

// DirName es el subdirectorio de prdash bajo $XDG_STATE_HOME.
const DirName = "prdash"

// MaxAge es la antigüedad máxima de una selección: pasado ese tiempo se
// considera obsoleta, porque la TUI que la mantenía ya no está detrás.
const MaxAge = 24 * time.Hour

// Selected es la identidad persistida del ítem seleccionado. Guarda la
// identidad completa (forge, host, proyecto, número, tipo, sección y URL) para
// poder reconstruir el ítem sin volver a consultar el inbox.
type Selected struct {
	Forge   string           `json:"forge"`
	Host    string           `json:"host"`
	Project string           `json:"project"`
	Owner   string           `json:"owner,omitempty"`
	Name    string           `json:"name,omitempty"`
	Number  int              `json:"number"`
	Kind    model.ReviewKind `json:"kind,omitempty"`
	Section model.Section    `json:"section,omitempty"`
	URL     string           `json:"url,omitempty"`
	SavedAt time.Time        `json:"saved_at"`
}

// FromItem construye la selección persistible de un ítem.
func FromItem(it model.Item, now time.Time) Selected {
	return Selected{
		Forge:   it.Forge,
		Host:    it.Host,
		Project: it.Ref.Project,
		Owner:   it.Ref.Owner,
		Name:    it.Ref.Name,
		Number:  it.Number,
		Kind:    it.ReviewKind,
		Section: it.Section,
		URL:     it.URL,
		SavedAt: now,
	}
}

// Item reconstruye el ítem a partir de la selección. Devuelve false si la
// identidad está incompleta o es inválida.
func (s Selected) Item() (model.Item, bool) {
	if s.Forge == "" || s.Host == "" || s.Project == "" || s.Number <= 0 {
		return model.Item{}, false
	}
	ref := model.RepoRef{Forge: s.Forge, Host: s.Host, Project: s.Project, Owner: s.Owner, Name: s.Name}
	it := model.NewItem(ref, s.Number)
	it.ReviewKind = s.Kind
	it.Section = s.Section
	it.URL = s.URL
	return it, true
}

// Fresh informa si la selección es lo bastante reciente. Una selección sin
// fecha se considera obsoleta.
func (s Selected) Fresh(now time.Time, maxAge time.Duration) bool {
	if s.SavedAt.IsZero() {
		return false
	}
	return now.Sub(s.SavedAt) <= maxAge
}

// Path devuelve la ruta del fichero de selección ($XDG_STATE_HOME/prdash). Sin
// XDG_STATE_HOME usa ~/.local/state.
func Path() (string, error) {
	dir := os.Getenv("XDG_STATE_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(dir, DirName, FileName), nil
}

// Save persiste la selección de forma atómica (tmp+rename), de modo que una
// lectura concurrente nunca vea un fichero a medias.
func Save(path string, s Selected) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Load lee la selección. Fichero ausente, corrupto o con JSON inválido =
// (Selected{}, false), sin error.
func Load(path string) (Selected, bool) {
	if path == "" {
		return Selected{}, false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return Selected{}, false
	}
	var s Selected
	if err := json.Unmarshal(raw, &s); err != nil {
		return Selected{}, false
	}
	return s, true
}
