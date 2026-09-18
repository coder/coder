package aibridged

import (
	"context"
	"testing"

	"golang.org/x/xerrors"
)

// SetPoolForTest closes the unused interception pool and installs a replacement.
// Call it after interception is selected, before requests, reloads, or shutdown.
// The server owns the replacement pool and shuts it down with the server.
func (s *Server) SetPoolForTest(ctx context.Context, t testing.TB, pool Pooler) error {
	t.Helper()
	s.backendMu.Lock()
	defer s.backendMu.Unlock()
	if pool == nil {
		return xerrors.New("nil request pool")
	}
	current := s.backend.Load()
	if current == nil || current.pool == nil {
		return xerrors.New("interception mode not selected")
	}
	if err := current.pool.Shutdown(ctx); err != nil {
		return xerrors.Errorf("shutdown unused request pool: %w", err)
	}
	s.backend.Store(&backend{pool: pool, keyPools: pool.KeyPools})
	return nil
}
