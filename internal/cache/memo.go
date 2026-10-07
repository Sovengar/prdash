package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

const MemoFileName = "memo.json"

const memoVersion = 1

type ReviewRecord struct {
	Repo     string `json:"repo"`
	Worktree string `json:"worktree"`
	Branch   string `json:"branch"`
	Label    string `json:"label"`
}

type Memo struct {
	Version int                     `json:"version"`
	Routes  map[string]string       `json:"routes"`
	Reviews map[string]ReviewRecord `json:"reviews"`
}

func MemoPath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, DirName, MemoFileName), nil
}

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

func SaveMemo(path string, m Memo) error {
	m.Version = memoVersion
	m = normalize(m)
	return saveJSON(path, m)
}

type Store struct {
	path string
	mu   sync.Mutex
	memo Memo
}

func OpenStore(path string) *Store {
	s := &Store{path: path, memo: emptyMemo()}
	if path != "" {
		if m, ok := LoadMemo(path); ok {
			s.memo = m
		}
	}
	return s
}

func (s *Store) Route(key string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.memo.Routes[key]
	return p, ok
}

func (s *Store) SetRoute(key, path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.memo.Routes[key] = path
	s.saveLocked()
}

func (s *Store) Review(key string) (ReviewRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.memo.Reviews[key]
	return rec, ok
}

func (s *Store) SetReview(key string, rec ReviewRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.memo.Reviews[key] = rec
	s.saveLocked()
}

func (s *Store) DeleteReview(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.memo.Reviews, key)
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
