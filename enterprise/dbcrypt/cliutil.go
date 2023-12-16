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

	userIDs, err := db.AllUserIDs(ctx)
	if err != nil {
		return xerrors.Errorf("get users: %w", err)
	}
	log.Info(ctx, "encrypting user tokens", slog.F("user_count", len(userIDs)))
	for idx, uid := range userIDs {
		err := cryptDB.InTx(func(cryptTx database.Store) error {
			userLinks, err := cryptTx.GetUserLinksByUserID(ctx, uid)
			if err != nil {
				return xerrors.Errorf("get user links for user: %w", err)
			}
			for _, userLink := range userLinks {
				if userLink.OAuthAccessTokenKeyID.String == ciphers[0].HexDigest() && userLink.OAuthRefreshTokenKeyID.String == ciphers[0].HexDigest() {
					log.Debug(ctx, "skipping user link", slog.F("user_id", uid), slog.F("current", idx+1), slog.F("cipher", ciphers[0].HexDigest()))
					continue
				}
				if _, err := cryptTx.UpdateUserLink(ctx, database.UpdateUserLinkParams{
					OAuthAccessToken:       userLink.OAuthAccessToken,
					OAuthAccessTokenKeyID:  sql.NullString{}, // dbcrypt will update as required
					OAuthRefreshToken:      userLink.OAuthRefreshToken,
					OAuthRefreshTokenKeyID: sql.NullString{}, // dbcrypt will update as required
					OAuthExpiry:            userLink.OAuthExpiry,
					UserID:                 uid,
					LoginType:              userLink.LoginType,
					DebugContext:           userLink.DebugContext,
				}); err != nil {
					return xerrors.Errorf("update user link user_id=%s linked_id=%s: %w", userLink.UserID, userLink.LinkedID, err)
				}
			}

			externalAuthLinks, err := cryptTx.GetExternalAuthLinksByUserID(ctx, uid)
			if err != nil {
				return xerrors.Errorf("get git auth links for user: %w", err)
			}
			for _, externalAuthLink := range externalAuthLinks {
				if externalAuthLink.OAuthAccessTokenKeyID.String == ciphers[0].HexDigest() && externalAuthLink.OAuthRefreshTokenKeyID.String == ciphers[0].HexDigest() {
					log.Debug(ctx, "skipping external auth link", slog.F("user_id", uid), slog.F("current", idx+1), slog.F("cipher", ciphers[0].HexDigest()))
					continue
				}
				if _, err := cryptTx.UpdateExternalAuthLink(ctx, database.UpdateExternalAuthLinkParams{
					ProviderID:             externalAuthLink.ProviderID,
					UserID:                 uid,
					UpdatedAt:              externalAuthLink.UpdatedAt,
					OAuthAccessToken:       externalAuthLink.OAuthAccessToken,
					OAuthAccessTokenKeyID:  sql.NullString{}, // dbcrypt will update as required
					OAuthRefreshToken:      externalAuthLink.OAuthRefreshToken,
					OAuthRefreshTokenKeyID: sql.NullString{}, // dbcrypt will update as required
					OAuthExpiry:            externalAuthLink.OAuthExpiry,
					OAuthExtra:             externalAuthLink.OAuthExtra,
				}); err != nil {
					return xerrors.Errorf("update external auth link user_id=%s provider_id=%s: %w", externalAuthLink.UserID, externalAuthLink.ProviderID, err)
				}
			}
			return nil
		}, &sql.TxOptions{
			Isolation: sql.LevelRepeatableRead,
		})
		if err != nil {
			return xerrors.Errorf("update user links: %w", err)
		}
		log.Debug(ctx, "encrypted user tokens", slog.F("user_id", uid), slog.F("current", idx+1), slog.F("cipher", ciphers[0].HexDigest()))
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

// Decrypt decrypts all user tokens and revokes all ciphers.
func Decrypt(ctx context.Context, log slog.Logger, sqlDB *sql.DB, ciphers []Cipher) error {
	db := database.New(sqlDB)
	cdb, err := New(ctx, db, ciphers...)
	if err != nil {
		return xerrors.Errorf("create cryptdb: %w", err)
	}

	// HACK: instead of adding logic to configure the primary cipher, we just
	// set it to the empty string so that it will not encrypt anything.
	cryptDB, ok := cdb.(*dbCrypt)
	if !ok {
		return xerrors.Errorf("developer error: dbcrypt.New did not return *dbCrypt")
	}
	cryptDB.primaryCipherDigest = ""

	userIDs, err := db.AllUserIDs(ctx)
	if err != nil {
		return xerrors.Errorf("get users: %w", err)
	}
	log.Info(ctx, "decrypting user tokens", slog.F("user_count", len(userIDs)))
	for idx, uid := range userIDs {
		err := cryptDB.InTx(func(tx database.Store) error {
			userLinks, err := tx.GetUserLinksByUserID(ctx, uid)
			if err != nil {
				return xerrors.Errorf("get user links for user: %w", err)
			}
			for _, userLink := range userLinks {
				if !userLink.OAuthAccessTokenKeyID.Valid && !userLink.OAuthRefreshTokenKeyID.Valid {
					log.Debug(ctx, "skipping user link", slog.F("user_id", uid), slog.F("current", idx+1))
					continue
				}
				if _, err := tx.UpdateUserLink(ctx, database.UpdateUserLinkParams{
					OAuthAccessToken:       userLink.OAuthAccessToken,
					OAuthAccessTokenKeyID:  sql.NullString{}, // we explicitly want to clear the key id
					OAuthRefreshToken:      userLink.OAuthRefreshToken,
					OAuthRefreshTokenKeyID: sql.NullString{}, // we explicitly want to clear the key id
					OAuthExpiry:            userLink.OAuthExpiry,
					UserID:                 uid,
					LoginType:              userLink.LoginType,
					DebugContext:           userLink.DebugContext,
				}); err != nil {
					return xerrors.Errorf("update user link user_id=%s linked_id=%s: %w", userLink.UserID, userLink.LinkedID, err)
				}
			}

			externalAuthLinks, err := tx.GetExternalAuthLinksByUserID(ctx, uid)
			if err != nil {
				return xerrors.Errorf("get git auth links for user: %w", err)
			}
			for _, externalAuthLink := range externalAuthLinks {
				if !externalAuthLink.OAuthAccessTokenKeyID.Valid && !externalAuthLink.OAuthRefreshTokenKeyID.Valid {
					log.Debug(ctx, "skipping external auth link", slog.F("user_id", uid), slog.F("current", idx+1))
					continue
				}
				if _, err := tx.UpdateExternalAuthLink(ctx, database.UpdateExternalAuthLinkParams{
					ProviderID:             externalAuthLink.ProviderID,
					UserID:                 uid,
					UpdatedAt:              externalAuthLink.UpdatedAt,
					OAuthAccessToken:       externalAuthLink.OAuthAccessToken,
					OAuthAccessTokenKeyID:  sql.NullString{}, // we explicitly want to clear the key id
					OAuthRefreshToken:      externalAuthLink.OAuthRefreshToken,
					OAuthRefreshTokenKeyID: sql.NullString{}, // we explicitly want to clear the key id
					OAuthExpiry:            externalAuthLink.OAuthExpiry,
					OAuthExtra:             externalAuthLink.OAuthExtra,
				}); err != nil {
					return xerrors.Errorf("update external auth link user_id=%s provider_id=%s: %w", externalAuthLink.UserID, externalAuthLink.ProviderID, err)
				}
			}
			return nil
		}, &sql.TxOptions{
			Isolation: sql.LevelRepeatableRead,
		})
		if err != nil {
			return xerrors.Errorf("update user links: %w", err)
		}
		log.Debug(ctx, "decrypted user tokens", slog.F("user_id", uid), slog.F("current", idx+1), slog.F("cipher", ciphers[0].HexDigest()))
	}

	// Revoke _all_ keys
	for _, c := range ciphers {
		if err := db.RevokeDBCryptKey(ctx, c.HexDigest()); err != nil {
			return xerrors.Errorf("revoke key: %w", err)
		}
		log.Info(ctx, "revoked unused key", slog.F("digest", c.HexDigest()))
	}

	return nil
}

// nolint: gosec
const sqlDeleteEncryptedUserTokens = `
BEGIN;
DELETE FROM user_links
  WHERE oauth_access_token_key_id IS NOT NULL
	OR oauth_refresh_token_key_id IS NOT NULL;
DELETE FROM external_auth_links
	WHERE oauth_access_token_key_id IS NOT NULL
	OR oauth_refresh_token_key_id IS NOT NULL;
COMMIT;
`

// Delete deletes all user tokens and revokes all ciphers.
// This is a destructive operation and should only be used
// as a last resort, for example, if the database encryption key has been
// lost.
func Delete(ctx context.Context, log slog.Logger, sqlDB *sql.DB) error {
	store := database.New(sqlDB)
	_, err := sqlDB.ExecContext(ctx, sqlDeleteEncryptedUserTokens)
	if err != nil {
		return xerrors.Errorf("delete user links: %w", err)
	}
	log.Info(ctx, "deleted encrypted user tokens")

	log.Info(ctx, "revoking all active keys")
	keys, err := store.GetDBCryptKeys(ctx)
	if err != nil {
		return xerrors.Errorf("get db crypt keys: %w", err)
	}
	for _, k := range keys {
		if !k.ActiveKeyDigest.Valid {
			continue
		}
		if err := store.RevokeDBCryptKey(ctx, k.ActiveKeyDigest.String); err != nil {
			return xerrors.Errorf("revoke key: %w", err)
		}
		log.Info(ctx, "revoked unused key", slog.F("digest", k.ActiveKeyDigest.String))
	}

	return nil
}
