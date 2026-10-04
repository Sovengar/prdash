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
	return guardaJSON(path, f)
}

// guardaJSON escribe `v` como JSON indentado en `path`, creando el directorio.
//
// Y hay UNO solo para el snapshot y para la memoria porque los dos hacían la misma secuencia de
// tres pasos —`MkdirAll`, `MarshalIndent`, `WriteFile`— y dos copias de una secuencia divergen:
// el día que uno aprenda a escribir con permiso de grupo, el otro se queda escribiendo a 0644 y
// el síntoma es un fichero que un usuario no puede tocar.
//
// Y el paso de `MarshalIndent` es el único que devuelve un error que NO es de disco, y eso es
// lo que hace que la función tome `any` y no el tipo concreto. Con el tipo concreto, `v` sería
// siempre un struct de cadenas y de slices, y `json.Marshal` de eso no falla nunca —el error solo
// sale con un canal, una función o un NaN—, así que la comprobación sería código muerto y nadie
// podría probarlo. Tomando `any`, el error es real, se puede provocar, y los dos llamadores
// siguen siendo seguros por construcción.
func guardaJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}
