//go:build !slim

package cli

import (
	"bytes"
	"context"
	"encoding/base64"

	"cdr.dev/slog"

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
		Use:   "dbcrypt-rotate --postgres-url <postgres_url> --external-token-encryption-keys <new-key>,<old-key>",
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
			logger, closeLogger, err := cli.BuildLogger(inv, vals)
			if err != nil {
				return xerrors.Errorf("set up logging: %w", err)
			}
			defer closeLogger()

			if vals.PostgresURL == "" {
				return xerrors.Errorf("no database configured")
			}

			if vals.ExternalTokenEncryptionKeys == nil || len(vals.ExternalTokenEncryptionKeys) != 2 {
				return xerrors.Errorf("dbcrypt-rotate requires exactly two external token encryption keys")
			}

			newKey, err := base64.StdEncoding.DecodeString(vals.ExternalTokenEncryptionKeys[0])
			if err != nil {
				return xerrors.Errorf("new key must be base64-encoded")
			}
			oldKey, err := base64.StdEncoding.DecodeString(vals.ExternalTokenEncryptionKeys[1])
			if err != nil {
				return xerrors.Errorf("old key must be base64-encoded")
			}
			if bytes.Equal(newKey, oldKey) {
				return xerrors.Errorf("old and new keys must be different")
			}

			primaryCipher, err := dbcrypt.CipherAES256(newKey)
			if err != nil {
				return xerrors.Errorf("create primary cipher: %w", err)
			}
			secondaryCipher, err := dbcrypt.CipherAES256(oldKey)
			if err != nil {
				return xerrors.Errorf("create secondary cipher: %w", err)
			}
			ciphers := dbcrypt.NewCiphers(primaryCipher, secondaryCipher)

			sqlDB, err := cli.ConnectToPostgres(inv.Context(), logger, "postgres", vals.PostgresURL.Value())
			if err != nil {
				return xerrors.Errorf("connect to postgres: %w", err)
			}
			defer func() {
				_ = sqlDB.Close()
			}()
			logger.Info(ctx, "connected to postgres")

			db := database.New(sqlDB)

			cryptDB, err := dbcrypt.New(ctx, db, ciphers)
			if err != nil {
				return xerrors.Errorf("create cryptdb: %w", err)
			}

			users, err := cryptDB.GetUsers(ctx, database.GetUsersParams{})
			if err != nil {
				return xerrors.Errorf("get users: %w", err)
			}
			for idx, usr := range users {
				userLinks, err := cryptDB.GetUserLinksByUserID(ctx, usr.ID)
				if err != nil {
					return xerrors.Errorf("get user links for user: %w", err)
				}
				for _, userLink := range userLinks {
					if _, err := cryptDB.UpdateUserLink(ctx, database.UpdateUserLinkParams{
						OAuthAccessToken:  userLink.OAuthAccessToken,
						OAuthRefreshToken: userLink.OAuthRefreshToken,
						OAuthExpiry:       userLink.OAuthExpiry,
						UserID:            usr.ID,
						LoginType:         usr.LoginType,
					}); err != nil {
						return xerrors.Errorf("update user link: %w", err)
					}
				}
				gitAuthLinks, err := cryptDB.GetGitAuthLinksByUserID(ctx, usr.ID)
				if err != nil {
					return xerrors.Errorf("get git auth links for user: %w", err)
				}
				for _, gitAuthLink := range gitAuthLinks {
					if _, err := cryptDB.UpdateGitAuthLink(ctx, database.UpdateGitAuthLinkParams{
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
				logger.Info(ctx, "encrypted user tokens", slog.F("current", idx+1), slog.F("of", len(users)))
			}
			logger.Info(ctx, "operation completed successfully")
			return nil
		},
	}
	return cmd
}
