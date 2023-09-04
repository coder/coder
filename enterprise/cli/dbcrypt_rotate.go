//go:build !slim

package cli

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/dbcrypt"

	"golang.org/x/xerrors"
)

func (*RootCmd) dbcryptRotate() *clibase.Cmd {
	var (
		vals = new(codersdk.DeploymentValues)
		opts = vals.Options()
	)
	cmd := &clibase.Cmd{
		Use:   "dbcrypt-rotate --postgres-url <postgres_url> --external-token-encryption-keys <new-key>,<old-keys>",
		Short: "Rotate database encryption keys",
		Options: clibase.OptionSet{
			*opts.ByName("Postgres Connection URL"),
			*opts.ByName("External Token Encryption Keys"),
		},
		Middleware: clibase.Chain(
			clibase.RequireNArgs(0),
		),
		Handler: func(inv *clibase.Invocation) error {
			ctx, cancel := context.WithCancel(inv.Context())
			defer cancel()
			logger := slog.Make(sloghuman.Sink(inv.Stdout))
			if ok, _ := inv.ParsedFlags().GetBool("verbose"); ok {
				logger = logger.Leveled(slog.LevelDebug)
			}

			if vals.PostgresURL == "" {
				return xerrors.Errorf("no database configured")
			}

			switch len(vals.ExternalTokenEncryptionKeys) {
			case 0:
				return xerrors.Errorf("no external token encryption keys provided")
			case 1:
				logger.Info(ctx, "only one key provided, data will be re-encrypted with the same key")
			}

			keys := make([][]byte, 0, len(vals.ExternalTokenEncryptionKeys))
			var newKey []byte
			for idx, ek := range vals.ExternalTokenEncryptionKeys {
				dk, err := base64.StdEncoding.DecodeString(ek)
				if err != nil {
					return xerrors.Errorf("key must be base64-encoded")
				}
				if idx == 0 {
					newKey = dk
				} else if bytes.Equal(dk, newKey) {
					return xerrors.Errorf("old key at index %d is the same as the new key", idx)
				}
				keys = append(keys, dk)
			}

			ciphers, err := dbcrypt.NewCiphers(keys...)
			if err != nil {
				return xerrors.Errorf("create ciphers: %w", err)
			}

			sqlDB, err := cli.ConnectToPostgres(inv.Context(), logger, "postgres", vals.PostgresURL.Value())
			if err != nil {
				return xerrors.Errorf("connect to postgres: %w", err)
			}
			defer func() {
				_ = sqlDB.Close()
			}()
			logger.Info(ctx, "connected to postgres")

			db := database.New(sqlDB)

			cryptDB, err := dbcrypt.New(ctx, db, ciphers...)
			if err != nil {
				return xerrors.Errorf("create cryptdb: %w", err)
			}

			users, err := cryptDB.GetUsers(ctx, database.GetUsersParams{})
			if err != nil {
				return xerrors.Errorf("get users: %w", err)
			}
			logger.Info(ctx, "encrypting user tokens", slog.F("count", len(users)))
			for idx, usr := range users {
				err := cryptDB.InTx(func(tx database.Store) error {
					userLinks, err := tx.GetUserLinksByUserID(ctx, usr.ID)
					if err != nil {
						return xerrors.Errorf("get user links for user: %w", err)
					}
					for _, userLink := range userLinks {
						if _, err := tx.UpdateUserLink(ctx, database.UpdateUserLinkParams{
							OAuthAccessToken:  userLink.OAuthAccessToken,
							OAuthRefreshToken: userLink.OAuthRefreshToken,
							OAuthExpiry:       userLink.OAuthExpiry,
							UserID:            usr.ID,
							LoginType:         usr.LoginType,
						}); err != nil {
							return xerrors.Errorf("update user link: %w", err)
						}
					}
					gitAuthLinks, err := tx.GetGitAuthLinksByUserID(ctx, usr.ID)
					if err != nil {
						return xerrors.Errorf("get git auth links for user: %w", err)
					}
					for _, gitAuthLink := range gitAuthLinks {
						if _, err := tx.UpdateGitAuthLink(ctx, database.UpdateGitAuthLinkParams{
							ProviderID:        gitAuthLink.ProviderID,
							UserID:            usr.ID,
							UpdatedAt:         gitAuthLink.UpdatedAt,
							OAuthAccessToken:  gitAuthLink.OAuthAccessToken,
							OAuthRefreshToken: gitAuthLink.OAuthRefreshToken,
							OAuthExpiry:       gitAuthLink.OAuthExpiry,
						}); err != nil {
							return xerrors.Errorf("update git auth link: %w", err)
						}
					}
					return nil
				}, &sql.TxOptions{
					Isolation: sql.LevelRepeatableRead,
				})
				if err != nil {
					return xerrors.Errorf("update user links: %w", err)
				}
				logger.Debug(ctx, "encrypted user tokens", slog.F("current", idx+1), slog.F("cipher", ciphers[0].HexDigest()))
			}
			logger.Info(ctx, "operation completed successfully")

			// Revoke old keys
			for _, c := range ciphers[1:] {
				if err := db.RevokeDBCryptKey(ctx, c.HexDigest()); err != nil {
					return xerrors.Errorf("revoke key: %w", err)
				}
				logger.Info(ctx, "revoked unused key", slog.F("digest", c.HexDigest()))
			}
			return nil
		},
	}
	return cmd
}
