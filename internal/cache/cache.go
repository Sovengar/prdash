// Package cache persiste un snapshot del inbox para pintarlo al instante al
// arrancar mientras el refresco corre en segundo plano. Un fichero corrupto o
// ausente se ignora en silencio: el cache nunca es fuente de verdad.
package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"prdash/internal/forge/model"
)

// FileName es el fichero de cache dentro del dir XDG de cache.
const FileName = "inbox.json"

// DirName es el subdirectorio de prdash bajo $XDG_CACHE_HOME.
const DirName = "prdash"

// version del formato; un fichero de otra versión se ignora.
const version = 1

// Stream es una lista del inbox cacheada, con su cursor para reanudar la
// paginación de forma incremental.
type Stream struct {
	Forge   string           `json:"forge"`
	Host    string           `json:"host"`
	Section model.Section    `json:"section"`
	Kind    model.ReviewKind `json:"kind,omitempty"`
	Cursor  string           `json:"cursor,omitempty"`
	Items   []model.Item     `json:"items"`
}

// File es el documento JSON completo.
type File struct {
	Version int       `json:"version"`
	SavedAt time.Time `json:"saved_at"`
	Streams []Stream  `json:"streams"`
}

// Path devuelve la ruta del fichero de cache ($XDG_CACHE_HOME/prdash).
func Path() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, DirName, FileName), nil
}

// Load lee el snapshot. Cache corrupto, versión desconocida o ausente =
// (File{}, false) sin error.
func Load(path string) (File, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return File{}, false
	}
	var f File
	if err := json.Unmarshal(raw, &f); err != nil || f.Version != version {
		return File{}, false
	}
	return f, true
}

// Save persiste el snapshot (best-effort: el llamador puede ignorar el error).
func Save(path string, f File) error {
	f.Version = version
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}
