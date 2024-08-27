package runtimeconfig

import (
	"context"
	"sync"

	"golang.org/x/xerrors"
)

var EntryNotFound = xerrors.New("entry not found")

type Store interface {
	GetRuntimeSetting(ctx context.Context, key string) (string, error)
	UpsertRuntimeSetting(ctx context.Context, key, value string) error
	DeleteRuntimeSetting(ctx context.Context, key string) error
}

type InMemoryStore struct {
	mu    sync.Mutex
	store map[string]string
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{store: make(map[string]string)}
}

func (s *InMemoryStore) GetRuntimeSetting(_ context.Context, key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	val, ok := s.store[key]
	if !ok {
		return "", EntryNotFound
	}

	return val, nil
}

func (s *InMemoryStore) UpsertRuntimeSetting(_ context.Context, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.store[key] = value
	return nil
}

func (s *InMemoryStore) DeleteRuntimeSetting(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.store, key)
	return nil
}
