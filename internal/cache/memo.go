package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// MemoFileName es el fichero de memoria de rutas resueltas y reviews activos,
// dentro del dir XDG de cache.
const MemoFileName = "memo.json"

// memoVersion del formato; un fichero de otra versión se ignora.
const memoVersion = 1

// ReviewRecord es el worktree que un ítem tiene montado como review activo.
type ReviewRecord struct {
	Repo     string `json:"repo"`
	Worktree string `json:"worktree"`
	Branch   string `json:"branch"`
	Label    string `json:"label"`
}

// Memo es la memoria de rutas: clones locales ya resueltos (clave
// "forge/host/proyecto") y el review activo de cada ítem (clave "…/proyecto#N").
type Memo struct {
	Version int                     `json:"version"`
	Routes  map[string]string       `json:"routes"`
	Reviews map[string]ReviewRecord `json:"reviews"`
}

// MemoPath devuelve la ruta del fichero de memoria ($XDG_CACHE_HOME/prdash).
func MemoPath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, DirName, MemoFileName), nil
}

// LoadMemo lee la memoria. Fichero ausente, corrupto o de versión desconocida =
// (Memo vacío, false), sin error.
func LoadMemo(path string) (Memo, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return emptyMemo(), false
	}
	var m Memo
	if err := json.Unmarshal(raw, &m); err != nil || m.Version != memoVersion {
		return emptyMemo(), false
	}
	return normalize(m), true
}

// SaveMemo persiste la memoria (best-effort: el llamador puede ignorar el error).
func SaveMemo(path string, m Memo) error {
	m.Version = memoVersion
	m = normalize(m)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

// Store es un acceso seguro en proceso a la memoria persistida. Serializa los
// read-modify-write y mantiene el fichero coherente entre resolutor y executor.
type Store struct {
	path string
	mu   sync.Mutex
	memo Memo
}

// OpenStore carga (o crea vacía) la memoria en path. path vacío = solo memoria
// en proceso, sin persistencia.
func OpenStore(path string) *Store {
	s := &Store{path: path, memo: emptyMemo()}
	if path != "" {
		if m, ok := LoadMemo(path); ok {
			s.memo = m
		}
	}
	return s
}

// Route devuelve la ruta local recordada para una clave de repo.
func (s *Store) Route(key string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.memo.Routes[key]
	return p, ok
}

// SetRoute recuerda la ruta local de una clave de repo.
func (s *Store) SetRoute(key, path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.memo.Routes[key] = path
	s.saveLocked()
}

// Review devuelve el review activo recordado para una clave de ítem.
func (s *Store) Review(key string) (ReviewRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.memo.Reviews[key]
	return rec, ok
}

// SetReview recuerda el review activo de una clave de ítem.
func (s *Store) SetReview(key string, rec ReviewRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.memo.Reviews[key] = rec
	s.saveLocked()
}

func (s *Store) saveLocked() {
	if s.path == "" {
		return
	}
	_ = SaveMemo(s.path, s.memo)
}

func emptyMemo() Memo {
	return Memo{
		Version: memoVersion,
		Routes:  map[string]string{},
		Reviews: map[string]ReviewRecord{},
	}
}

func normalize(m Memo) Memo {
	if m.Routes == nil {
		m.Routes = map[string]string{}
	}
	if m.Reviews == nil {
		m.Reviews = map[string]ReviewRecord{}
	}
	return m
}
