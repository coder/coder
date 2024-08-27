package config

import (
	"sync"

	"golang.org/x/xerrors"
)

var EntryNotFound = xerrors.New("entry not found")

type Store interface {
	GetRuntimeSetting(key string) (string, error)
	UpsertRuntimeSetting(key, value string) error
	DeleteRuntimeSetting(key string) error
}

type InMemoryStore struct {
	mu    sync.Mutex
	store map[string]string
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{store: make(map[string]string)}
}

func (s *InMemoryStore) GetRuntimeSetting(key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	val, ok := s.store[key]
	if !ok {
		return "", EntryNotFound
	}

	return val, nil
}

func (s *InMemoryStore) UpsertRuntimeSetting(key, val string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.store[key] = val
	return nil
}

func (s *InMemoryStore) DeleteRuntimeSetting(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.store, key)
	return nil
}
