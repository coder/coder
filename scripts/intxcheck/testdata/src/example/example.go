package example

import "context"

type TxOptions struct{}

type Store interface {
	InTx(func(Store) error, *TxOptions) error
	GetUser(context.Context) (string, error)
	GetConfig(context.Context) (string, error)
}

type Server struct {
	db Store
}

func helper(context.Context, Store) {}

func helperWithDB(ctx context.Context, db Store) {
	_, _ = db.GetUser(ctx)
}

func shadowingOK(ctx context.Context, db Store) error {
	return db.InTx(func(db Store) error {
		_, _ = db.GetUser(ctx)
		return nil
	}, nil)
}

func pkgFuncOK(ctx context.Context, db Store) error {
	return db.InTx(func(tx Store) error {
		helperWithDB(ctx, tx)
		return nil
	}, nil)
}

func (s *Server) directMisuse(ctx context.Context) error {
	return s.db.InTx(func(tx Store) error {
		_, _ = s.db.GetUser(ctx) // want "outer store 's.db' used inside InTx; use transaction store 'tx' instead"
		return nil
	}, nil)
}

func (s *Server) passThroughMisuse(ctx context.Context) error {
	return s.db.InTx(func(tx Store) error {
		helper(ctx, s.db) // want "outer store 's.db' passed as argument inside InTx; use transaction store 'tx' instead"
		return nil
	}, nil)
}

func (s *Server) indirectMisuse(ctx context.Context) error {
	return s.db.InTx(func(tx Store) error {
		s.getConfig(ctx) // want "call to 's.getConfig' inside InTx uses outer store 's.db'; pass 'tx' through the helper or hoist the call"
		return nil
	}, nil)
}

func (s *Server) suppressedCase(ctx context.Context) error {
	return s.db.InTx(func(tx Store) error {
		_, _ = s.db.GetUser(ctx) //intxcheck:ignore
		return nil
	}, nil)
}

func (srv *Server) getConfig(ctx context.Context) string {
	value, _ := srv.db.GetConfig(ctx)
	return value
}

func (s *Server) correctUsage(ctx context.Context) error {
	return s.db.InTx(func(tx Store) error {
		_, _ = tx.GetUser(ctx)
		return nil
	}, nil)
}

func (s *Server) safeHelper(ctx context.Context) error {
	return s.db.InTx(func(tx Store) error {
		s.formatName("test")
		return nil
	}, nil)
}

func (s *Server) formatName(name string) string {
	return name
}

func (s *Server) outsideInTx(ctx context.Context) error {
	_, _ = s.db.GetUser(ctx)
	return nil
}
