package migrations

import (
	"context"
	"database/sql"

	"golang.org/x/xerrors"
)

// EnsureTailnetTablesLogged converts the tailnet coordination tables from
// UNLOGGED to LOGGED and reports whether it changed anything.
//
// Migration 000412 made the tables UNLOGGED. UNLOGGED tables are not
// replicated to standbys and are emptied on promotion or crash recovery,
// and pgcoord never rewrites its rows afterwards, so a Postgres failover
// left replicas with no coordination state. Release branches cannot take
// a new migration without breaking a later upgrade, so the conversion
// runs here at startup instead. On a database where the tables are
// already LOGGED it only reads pg_class.
func EnsureTailnetTablesLogged(ctx context.Context, db *sql.DB) (bool, error) {
	var unlogged int
	err := db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM pg_class
		WHERE oid = ANY(ARRAY[
			'tailnet_coordinators'::regclass,
			'tailnet_peers'::regclass,
			'tailnet_tunnels'::regclass
		])
		AND relpersistence = 'u'`).Scan(&unlogged)
	if err != nil {
		return false, xerrors.Errorf("check tailnet table persistence: %w", err)
	}
	if unlogged == 0 {
		return false, nil
	}

	// Parent first: a LOGGED child cannot reference an UNLOGGED parent.
	for _, stmt := range []string{
		`ALTER TABLE tailnet_coordinators SET LOGGED`,
		`ALTER TABLE tailnet_peers SET LOGGED`,
		`ALTER TABLE tailnet_tunnels SET LOGGED`,
	} {
		_, err = db.ExecContext(ctx, stmt)
		if err != nil {
			return false, xerrors.Errorf("%s: %w", stmt, err)
		}
	}
	return true, nil
}
