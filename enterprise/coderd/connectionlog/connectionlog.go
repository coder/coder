package connectionlog

import (
	"context"

	"github.com/hashicorp/go-multierror"

	"cdr.dev/slog"
	agpl "github.com/coder/coder/v2/coderd/connectionlog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	auditbackends "github.com/coder/coder/v2/enterprise/audit/backends"
)

type Backend interface {
	Export(ctx context.Context, clog database.ConnectionLog) error
}

func NewConnectionLogger(backends ...Backend) agpl.ConnectionLogger {
	return &connectionLogger{
		backends: backends,
	}
}

type connectionLogger struct {
	backends []Backend
}

func (c *connectionLogger) Export(ctx context.Context, clog database.ConnectionLog) error {
	var errs error
	for _, backend := range c.backends {
		err := backend.Export(ctx, clog)
		if err != nil {
			errs = multierror.Append(errs, err)
		}
	}
	return errs
}

type dbBackend struct {
	db database.Store
}

func NewDBBackend(db database.Store) Backend {
	return &dbBackend{db: db}
}

func (b *dbBackend) Export(ctx context.Context, clog database.ConnectionLog) error {
	//nolint:gocritic // This is the Connection Logger
	_, err := b.db.InsertConnectionLog(dbauthz.AsConnectionLogger(ctx), database.InsertConnectionLogParams(clog))
	if err != nil {
		return err
	}
	return nil
}

type connectionSlogBackend struct {
	exporter *auditbackends.SlogExporter
}

func NewSlogBackend(logger slog.Logger) Backend {
	return &connectionSlogBackend{
		exporter: auditbackends.NewSlogExporter(logger),
	}
}

func (b *connectionSlogBackend) Export(ctx context.Context, clog database.ConnectionLog) error {
	return b.exporter.ExportStruct(ctx, clog, "connection_log")
}
