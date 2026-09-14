package aibridged

import (
	"context"

	"golang.org/x/xerrors"
)

// SetPoolForTest closes the unused interception pool and installs a replacement.
// Call it after interception is selected, before requests, reloads, or shutdown.
// The server owns the replacement pool and shuts it down with the server.
func (s *Server) SetPoolForTest(ctx context.Context, pool Pooler) error {
	s.backendMu.Lock()
	defer s.backendMu.Unlock()
	if pool == nil {
		return xerrors.New("nil request pool")
	}
	backend := s.backend.Load()
	if backend == nil || backend.pool == nil {
		return xerrors.New("interception mode not selected")
	}
	if err := backend.pool.Shutdown(ctx); err != nil {
		return xerrors.Errorf("shutdown unused request pool: %w", err)
	}
	s.backend.Store(&requestHandler{pool: pool, keyPools: pool.KeyPools})
	return nil
}
