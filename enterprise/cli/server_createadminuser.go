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

// init wires dbcrypt into `coder server create-admin-user`. The AGPL CLI
// cannot import enterprise packages, so it exposes a hook instead. This runs
// in init to avoid data races between parallel command constructions.
func init() {
	agplcli.WrapCreateAdminUserStore = func(ctx context.Context, db database.Store, keys []string) (database.Store, error) {
		ks := make([][]byte, 0, len(keys))
		for i, key := range keys {
			dk, err := base64.StdEncoding.DecodeString(key)
			if err != nil {
				return nil, xerrors.Errorf("decode external token encryption key at index %d: %w", i, err)
			}
			ks = append(ks, dk)
		}
		ciphers, err := dbcrypt.NewCiphers(ks...)
		if err != nil {
			return nil, xerrors.Errorf("create ciphers: %w", err)
		}
		cryptDB, err := dbcrypt.New(ctx, db, ciphers...)
		if err != nil {
			return nil, xerrors.Errorf("enable at-rest encryption: %w", err)
		}
		return cryptDB, nil
	}
}
