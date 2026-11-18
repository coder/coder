//go:build !slim

package cli

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

// WrapDirectDatabaseStore is set by the enterprise CLI to wrap the direct
// database connection of commands that write to the database without a
// running server with dbcrypt. AGPL code cannot import enterprise packages,
// so the enterprise CLI injects the implementation here. It is nil in
// AGPL-only builds, where --external-token-encryption-keys is rejected.
var WrapDirectDatabaseStore func(ctx context.Context, db database.Store, keys []string) (database.Store, error)

// requireNoActiveDBCryptKeys returns an error when the database has an active
// dbcrypt key. Commands that write encrypted fields call it when no
// encryption keys were provided.
func requireNoActiveDBCryptKeys(ctx context.Context, db database.Store) error {
	keys, err := db.GetDBCryptKeys(ctx)
	if err != nil {
		return xerrors.Errorf("check database encryption keys: %w", err)
	}
	for _, key := range keys {
		if key.ActiveKeyDigest.Valid && !key.RevokedKeyDigest.Valid {
			return xerrors.New(
				"this database encrypts fields with dbcrypt. " +
					"Provide the deployment's keys with --external-token-encryption-keys or CODER_EXTERNAL_TOKEN_ENCRYPTION_KEYS",
			)
		}
	}
	return nil
}
