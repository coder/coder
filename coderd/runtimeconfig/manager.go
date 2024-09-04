package runtimeconfig

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

type NoopManager struct{}

func NewNoopManager() *NoopManager {
	return &NoopManager{}
}

func (NoopManager) GetRuntimeSetting(context.Context, string) (string, error) {
	return "", EntryNotFound
}

func (NoopManager) UpsertRuntimeSetting(context.Context, string, string) error {
	return EntryNotFound
}

func (NoopManager) DeleteRuntimeSetting(context.Context, string) error {
	return EntryNotFound
}

func (n NoopManager) Scoped(string) Manager {
	return n
}

type StoreManager struct {
	Store

	ns string
}

func NewStoreManager(store Store) *StoreManager {
	if store == nil {
		panic("developer error: store must not be nil")
	}
	return &StoreManager{Store: store}
}

func (m StoreManager) GetRuntimeSetting(ctx context.Context, key string) (string, error) {
	key = m.namespacedKey(key)
	val, err := m.Store.GetRuntimeConfig(ctx, key)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", xerrors.Errorf("%q: %w", key, EntryNotFound)
		}
		return "", xerrors.Errorf("fetch %q: %w", key, err)
	}

	return val, nil
}

func (m StoreManager) UpsertRuntimeSetting(ctx context.Context, key, val string) error {
	err := m.Store.UpsertRuntimeConfig(ctx, database.UpsertRuntimeConfigParams{Key: m.namespacedKey(key), Value: val})
	if err != nil {
		return xerrors.Errorf("update %q: %w", key, err)
	}
	return nil
}

func (m StoreManager) DeleteRuntimeSetting(ctx context.Context, key string) error {
	return m.Store.DeleteRuntimeConfig(ctx, m.namespacedKey(key))
}

func (m StoreManager) Scoped(ns string) Manager {
	return &StoreManager{Store: m.Store, ns: ns}
}

func (m StoreManager) namespacedKey(k string) string {
	return fmt.Sprintf("%s:%s", m.ns, k)
}
