//go:build !slim

package cli

import (
	"context"
	"encoding/base64"

	"golang.org/x/xerrors"

	agplcli "github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/enterprise/dbcrypt"
)

// init wires dbcrypt into AGPL commands that write to the database directly.
// The AGPL CLI cannot import enterprise packages, so it exposes a hook
// instead. This runs in init to avoid data races between parallel command
// constructions.
func init() {
	agplcli.WrapDirectDatabaseStore = wrapDirectDatabaseStore
}

func wrapDirectDatabaseStore(ctx context.Context, db database.Store, keys []string) (database.Store, error) {
	decodedKeys := make([][]byte, 0, len(keys))
	for i, key := range keys {
		decodedKey, err := base64.StdEncoding.DecodeString(key)
		if err != nil {
			return nil, xerrors.Errorf("decode external token encryption key at index %d: %w", i, err)
		}
		decodedKeys = append(decodedKeys, decodedKey)
	}
	ciphers, err := dbcrypt.NewCiphers(decodedKeys...)
	if err != nil {
		return nil, xerrors.Errorf("create ciphers: %w", err)
	}
	cryptDB, err := dbcrypt.New(ctx, db, ciphers...)
	if err != nil {
		return nil, xerrors.Errorf("enable database encryption: %w", err)
	}
	return cryptDB, nil
}
