// Package cache persists an inbox snapshot so the UI paints instantly at startup.
// A corrupt or missing file is ignored silently: the cache is never a source of truth.
package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"prdash/internal/forge/model"
)

const FileName = "inbox.json"

const DirName = "prdash"

// A snapshot from another version is ignored rather than migrated: the format is cheap to rebuild.
const version = 1

type Stream struct {
	Forge   string           `json:"forge"`
	Host    string           `json:"host"`
	Section model.Section    `json:"section"`
	Kind    model.ReviewKind `json:"kind,omitempty"`
	Cursor  string           `json:"cursor,omitempty"`
	Items   []model.Item     `json:"items"`
}

type File struct {
	Version int       `json:"version"`
	SavedAt time.Time `json:"saved_at"`
	Streams []Stream  `json:"streams"`
}

func Path() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, DirName, FileName), nil
}

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

// Best-effort: losing the snapshot costs a refetch, not correctness.
func Save(path string, f File) error {
	f.Version = version
	return guardaJSON(path, f)
}

// One helper for both the snapshot and the memo: they ran the same three-step sequence and two
// copies of a sequence drift. The `any` is what makes the error branch real — marshalling a struct
// of strings and slices never fails, so with a concrete type the check would be dead code.
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
