package dbcrypt

import (
	"context"
	"database/sql"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database"
)

// Rotate rotates the database encryption keys by re-encrypting all user tokens
// with the first cipher and revoking all other ciphers.
func Rotate(ctx context.Context, log slog.Logger, sqlDB *sql.DB, ciphers []Cipher) error {
	db := database.New(sqlDB)
	cryptDB, err := New(ctx, db, ciphers...)
	if err != nil {
		return xerrors.Errorf("create cryptdb: %w", err)
	}

	users, err := cryptDB.GetUsers(ctx, database.GetUsersParams{})
	if err != nil {
		return xerrors.Errorf("get users: %w", err)
	}
	log.Info(ctx, "encrypting user tokens", slog.F("user_count", len(users)))
	for idx, usr := range users {
		err := cryptDB.InTx(func(tx database.Store) error {
			userLinks, err := tx.GetUserLinksByUserID(ctx, usr.ID)
			if err != nil {
				return xerrors.Errorf("get user links for user: %w", err)
			}
			for _, userLink := range userLinks {
				if userLink.OAuthAccessTokenKeyID.String == ciphers[0].HexDigest() && userLink.OAuthRefreshTokenKeyID.String == ciphers[0].HexDigest() {
					log.Debug(ctx, "skipping user link", slog.F("user_id", usr.ID), slog.F("current", idx+1), slog.F("cipher", ciphers[0].HexDigest()))
					continue
				}
				if _, err := tx.UpdateUserLink(ctx, database.UpdateUserLinkParams{
					OAuthAccessToken:  userLink.OAuthAccessToken,
					OAuthRefreshToken: userLink.OAuthRefreshToken,
					OAuthExpiry:       userLink.OAuthExpiry,
					UserID:            usr.ID,
					LoginType:         usr.LoginType,
				}); err != nil {
					return xerrors.Errorf("update user link user_id=%s linked_id=%s: %w", userLink.UserID, userLink.LinkedID, err)
				}
			}

			gitAuthLinks, err := tx.GetGitAuthLinksByUserID(ctx, usr.ID)
			if err != nil {
				return xerrors.Errorf("get git auth links for user: %w", err)
			}
			for _, gitAuthLink := range gitAuthLinks {
				if gitAuthLink.OAuthAccessTokenKeyID.String == ciphers[0].HexDigest() && gitAuthLink.OAuthRefreshTokenKeyID.String == ciphers[0].HexDigest() {
					log.Debug(ctx, "skipping git auth link", slog.F("user_id", usr.ID), slog.F("current", idx+1), slog.F("cipher", ciphers[0].HexDigest()))
					continue
				}
				if _, err := tx.UpdateGitAuthLink(ctx, database.UpdateGitAuthLinkParams{
					ProviderID:        gitAuthLink.ProviderID,
					UserID:            usr.ID,
					UpdatedAt:         gitAuthLink.UpdatedAt,
					OAuthAccessToken:  gitAuthLink.OAuthAccessToken,
					OAuthRefreshToken: gitAuthLink.OAuthRefreshToken,
					OAuthExpiry:       gitAuthLink.OAuthExpiry,
				}); err != nil {
					return xerrors.Errorf("update git auth link user_id=%s provider_id=%s: %w", gitAuthLink.UserID, gitAuthLink.ProviderID, err)
				}
			}
			return nil
		}, &sql.TxOptions{
			Isolation: sql.LevelRepeatableRead,
		})
		if err != nil {
			return xerrors.Errorf("update user links: %w", err)
		}
		log.Debug(ctx, "encrypted user tokens", slog.F("user_id", usr.ID), slog.F("current", idx+1), slog.F("cipher", ciphers[0].HexDigest()))
	}

	// Revoke old keys
	for _, c := range ciphers[1:] {
		if err := db.RevokeDBCryptKey(ctx, c.HexDigest()); err != nil {
			return xerrors.Errorf("revoke key: %w", err)
		}
		log.Info(ctx, "revoked unused key", slog.F("digest", c.HexDigest()))
	}

	return nil
}
