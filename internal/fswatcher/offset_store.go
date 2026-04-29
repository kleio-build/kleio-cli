package fswatcher

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// OffsetStore persists file read offsets to disk for incremental processing.
type OffsetStore struct {
	mu      sync.Mutex
	path    string
	offsets map[string]int64
}

func NewOffsetStore(path string) *OffsetStore {
	return &OffsetStore{
		path:    path,
		offsets: make(map[string]int64),
	}
}

func (s *OffsetStore) Get(filePath string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.offsets[filePath]
}

func (s *OffsetStore) Set(filePath string, offset int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.offsets[filePath] = offset
}

func (s *OffsetStore) Save() error {
	s.mu.Lock()
	data, err := json.MarshalIndent(s.offsets, "", "  ")
	s.mu.Unlock()
	if err != nil {
		return err
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0644)
}

func (s *OffsetStore) Load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return json.Unmarshal(data, &s.offsets)
}
