package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// State tracks the last-polled timestamp per source key.
// Safe for concurrent use.
type State struct {
	mu   sync.RWMutex
	data map[string]time.Time
}

func newState() *State {
	return &State{data: make(map[string]time.Time)}
}

func loadState(path string) (*State, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var raw map[string]time.Time
	if err := json.NewDecoder(f).Decode(&raw); err != nil {
		return nil, err
	}
	return &State{data: raw}, nil
}

func (s *State) Get(key string) time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data[key]
}

func (s *State) Set(key string, t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = t
}

// Save atomically writes state to path via a temp file + rename.
func (s *State) Save(path string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".state-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if err := json.NewEncoder(f).Encode(s.data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}
