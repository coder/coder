package dbcrypt

import (
	"context"
	"database/sql"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
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

	userIDs, err := db.AllUserIDs(ctx, false)
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
					Claims:                 userLink.Claims,
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
					ProviderID:                externalAuthLink.ProviderID,
					UserID:                    uid,
					UpdatedAt:                 externalAuthLink.UpdatedAt,
					OAuthAccessToken:          externalAuthLink.OAuthAccessToken,
					OAuthAccessTokenKeyID:     sql.NullString{}, // dbcrypt will update as required
					OAuthRefreshToken:         externalAuthLink.OAuthRefreshToken,
					OAuthRefreshTokenKeyID:    sql.NullString{}, // dbcrypt will update as required
					OAuthExpiry:               externalAuthLink.OAuthExpiry,
					OAuthExtra:                externalAuthLink.OAuthExtra,
					OauthRefreshFailureReason: "",
					RefreshLeaseExpiresAt:     sql.NullTime{},
				}); err != nil {
					return xerrors.Errorf("update external auth link user_id=%s provider_id=%s: %w", externalAuthLink.UserID, externalAuthLink.ProviderID, err)
				}
			}

			userSecrets, err := cryptTx.ListUserSecretsWithValues(ctx, uid)
			if err != nil {
				return xerrors.Errorf("get user secrets for user %s: %w", uid, err)
			}
			for _, secret := range userSecrets {
				if secret.ValueKeyID.Valid && secret.ValueKeyID.String == ciphers[0].HexDigest() {
					log.Debug(ctx, "skipping user secret", slog.F("user_id", uid), slog.F("secret_name", secret.Name), slog.F("current", idx+1), slog.F("cipher", ciphers[0].HexDigest()))
					continue
				}
				if _, err := cryptTx.UpdateUserSecretByUserIDAndName(ctx, database.UpdateUserSecretByUserIDAndNameParams{
					UserID:            uid,
					Name:              secret.Name,
					UpdateValue:       true,
					Value:             secret.Value,
					ValueKeyID:        sql.NullString{}, // dbcrypt will re-encrypt
					UpdateDescription: false,
					Description:       "",
					UpdateEnvName:     false,
					EnvName:           "",
					UpdateFilePath:    false,
					FilePath:          "",
					UpdateEnabled:     false,
					Enabled:           false,
				}); err != nil {
					return xerrors.Errorf("rotate user secret user_id=%s name=%s: %w", uid, secret.Name, err)
				}
				log.Debug(ctx, "rotated user secret", slog.F("user_id", uid), slog.F("secret_name", secret.Name), slog.F("current", idx+1), slog.F("cipher", ciphers[0].HexDigest()))
			}

			sshKey, err := cryptTx.GetGitSSHKey(ctx, uid)
			if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
				return xerrors.Errorf("get gitsshkey for user %s: %w", uid, err)
			}
			if err == nil {
				switch {
				case sshKey.PrivateKey == "":
					// Post-Delete wipes the private_key and key_id; nothing to encrypt.
					log.Debug(ctx, "skipping empty gitsshkey", slog.F("user_id", uid), slog.F("current", idx+1))
				case sshKey.PrivateKeyKeyID.Valid && sshKey.PrivateKeyKeyID.String == ciphers[0].HexDigest():
					log.Debug(ctx, "skipping gitsshkey", slog.F("user_id", uid), slog.F("current", idx+1), slog.F("cipher", ciphers[0].HexDigest()))
				default:
					if _, err := cryptTx.UpdateGitSSHKey(ctx, database.UpdateGitSSHKeyParams{
						UserID:          uid,
						UpdatedAt:       sshKey.UpdatedAt,
						PrivateKey:      sshKey.PrivateKey,
						PrivateKeyKeyID: sql.NullString{}, // dbcrypt will re-encrypt
						PublicKey:       sshKey.PublicKey,
					}); err != nil {
						return xerrors.Errorf("rotate gitsshkey user_id=%s: %w", uid, err)
					}
					log.Debug(ctx, "rotated gitsshkey", slog.F("user_id", uid), slog.F("current", idx+1), slog.F("cipher", ciphers[0].HexDigest()))
				}
			}

			if err := rewriteMCPServerUserTokens(ctx, log, cryptTx, uid, ciphers[0].HexDigest()); err != nil {
				return err
			}

			return nil
		}, &database.TxOptions{
			Isolation: sql.LevelRepeatableRead,
		})
		if err != nil {
			return xerrors.Errorf("update user tokens and chat provider keys: %w", err)
		}
		log.Debug(ctx, "encrypted user tokens", slog.F("user_id", uid), slog.F("current", idx+1), slog.F("cipher", ciphers[0].HexDigest()))
	}

	aiProviders, err := cryptDB.GetAIProviders(ctx, database.GetAIProvidersParams{IncludeDeleted: true, IncludeDisabled: true})
	if err != nil {
		return xerrors.Errorf("get ai providers: %w", err)
	}
	log.Info(ctx, "encrypting ai provider settings", slog.F("provider_count", len(aiProviders)))
	for idx, ap := range aiProviders {
		if !ap.Settings.Valid || strings.TrimSpace(ap.Settings.String) == "" {
			continue
		}
		if ap.SettingsKeyID.Valid && ap.SettingsKeyID.String == ciphers[0].HexDigest() {
			log.Debug(ctx, "skipping ai provider", slog.F("ai_provider_id", ap.ID), slog.F("name", ap.Name), slog.F("current", idx+1), slog.F("cipher", ciphers[0].HexDigest()))
			continue
		}
		if _, err := cryptDB.UpdateEncryptedAIProviderSettings(ctx, database.UpdateEncryptedAIProviderSettingsParams{
			ID:            ap.ID,
			Settings:      ap.Settings,
			SettingsKeyID: sql.NullString{}, // dbcrypt will update as required
		}); err != nil {
			return xerrors.Errorf("update ai provider id=%s name=%s: %w", ap.ID, ap.Name, err)
		}
		log.Debug(ctx, "encrypted ai provider settings", slog.F("ai_provider_id", ap.ID), slog.F("name", ap.Name), slog.F("current", idx+1), slog.F("cipher", ciphers[0].HexDigest()))
	}

	aiProviderKeys, err := cryptDB.GetAIProviderKeys(ctx, true)
	if err != nil {
		return xerrors.Errorf("get ai provider keys: %w", err)
	}
	log.Info(ctx, "encrypting ai provider keys", slog.F("key_count", len(aiProviderKeys)))
	for idx, apk := range aiProviderKeys {
		if strings.TrimSpace(apk.APIKey) == "" {
			continue
		}
		if apk.ApiKeyKeyID.Valid && apk.ApiKeyKeyID.String == ciphers[0].HexDigest() {
			log.Debug(ctx, "skipping ai provider key", slog.F("ai_provider_key_id", apk.ID), slog.F("provider_id", apk.ProviderID), slog.F("current", idx+1), slog.F("cipher", ciphers[0].HexDigest()))
			continue
		}
		if _, err := cryptDB.UpdateEncryptedAIProviderKey(ctx, database.UpdateEncryptedAIProviderKeyParams{
			ID:          apk.ID,
			APIKey:      apk.APIKey,
			ApiKeyKeyID: sql.NullString{}, // dbcrypt will update as required
		}); err != nil {
			return xerrors.Errorf("update ai provider key id=%s provider_id=%s: %w", apk.ID, apk.ProviderID, err)
		}
		log.Debug(ctx, "encrypted ai provider key", slog.F("ai_provider_key_id", apk.ID), slog.F("provider_id", apk.ProviderID), slog.F("current", idx+1), slog.F("cipher", ciphers[0].HexDigest()))
	}

	userAIProviderKeys, err := cryptDB.GetUserAIProviderKeys(ctx)
	if err != nil {
		return xerrors.Errorf("get user ai provider keys: %w", err)
	}
	log.Info(ctx, "encrypting user ai provider keys", slog.F("key_count", len(userAIProviderKeys)))
	for idx, key := range userAIProviderKeys {
		if strings.TrimSpace(key.APIKey) == "" {
			continue
		}
		if key.ApiKeyKeyID.Valid && key.ApiKeyKeyID.String == ciphers[0].HexDigest() {
			log.Debug(ctx, "skipping user ai provider key", slog.F("user_ai_provider_key_id", key.ID), slog.F("ai_provider_id", key.AIProviderID), slog.F("user_id", key.UserID), slog.F("current", idx+1), slog.F("cipher", ciphers[0].HexDigest()))
			continue
		}
		if _, err := cryptDB.UpdateEncryptedUserAIProviderKey(ctx, database.UpdateEncryptedUserAIProviderKeyParams{
			ID:          key.ID,
			APIKey:      key.APIKey,
			ApiKeyKeyID: sql.NullString{}, // dbcrypt will update as required
		}); err != nil {
			return xerrors.Errorf("update user ai provider key id=%s ai_provider_id=%s user_id=%s: %w", key.ID, key.AIProviderID, key.UserID, err)
		}
		log.Debug(ctx, "encrypted user ai provider key", slog.F("user_ai_provider_key_id", key.ID), slog.F("ai_provider_id", key.AIProviderID), slog.F("user_id", key.UserID), slog.F("current", idx+1), slog.F("cipher", ciphers[0].HexDigest()))
	}

	if err := rewriteCryptoKeys(ctx, log, cryptDB, sqlDB, ciphers[0].HexDigest()); err != nil {
		return err
	}
	if err := rewriteMCPServerConfigs(ctx, log, cryptDB, ciphers[0].HexDigest()); err != nil {
		return err
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

	userIDs, err := db.AllUserIDs(ctx, false)
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
					Claims:                 userLink.Claims,
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
					ProviderID:                externalAuthLink.ProviderID,
					UserID:                    uid,
					UpdatedAt:                 externalAuthLink.UpdatedAt,
					OAuthAccessToken:          externalAuthLink.OAuthAccessToken,
					OAuthAccessTokenKeyID:     sql.NullString{}, // we explicitly want to clear the key id
					OAuthRefreshToken:         externalAuthLink.OAuthRefreshToken,
					OAuthRefreshTokenKeyID:    sql.NullString{}, // we explicitly want to clear the key id
					OAuthExpiry:               externalAuthLink.OAuthExpiry,
					OAuthExtra:                externalAuthLink.OAuthExtra,
					OauthRefreshFailureReason: "",
					RefreshLeaseExpiresAt:     sql.NullTime{},
				}); err != nil {
					return xerrors.Errorf("update external auth link user_id=%s provider_id=%s: %w", externalAuthLink.UserID, externalAuthLink.ProviderID, err)
				}
			}

			userSecrets, err := tx.ListUserSecretsWithValues(ctx, uid)
			if err != nil {
				return xerrors.Errorf("get user secrets for user %s: %w", uid, err)
			}
			for _, secret := range userSecrets {
				if !secret.ValueKeyID.Valid {
					log.Debug(ctx, "skipping user secret", slog.F("user_id", uid), slog.F("secret_name", secret.Name), slog.F("current", idx+1))
					continue
				}
				if _, err := tx.UpdateUserSecretByUserIDAndName(ctx, database.UpdateUserSecretByUserIDAndNameParams{
					UserID:            uid,
					Name:              secret.Name,
					UpdateValue:       true,
					Value:             secret.Value,
					ValueKeyID:        sql.NullString{}, // clear the key ID
					UpdateDescription: false,
					Description:       "",
					UpdateEnvName:     false,
					EnvName:           "",
					UpdateFilePath:    false,
					FilePath:          "",
					UpdateEnabled:     false,
					Enabled:           false,
				}); err != nil {
					return xerrors.Errorf("decrypt user secret user_id=%s name=%s: %w", uid, secret.Name, err)
				}
				log.Debug(ctx, "decrypted user secret", slog.F("user_id", uid), slog.F("secret_name", secret.Name), slog.F("current", idx+1))
			}

			sshKey, err := tx.GetGitSSHKey(ctx, uid)
			if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
				return xerrors.Errorf("get gitsshkey for user %s: %w", uid, err)
			}
			if err == nil && sshKey.PrivateKeyKeyID.Valid {
				if _, err := tx.UpdateGitSSHKey(ctx, database.UpdateGitSSHKeyParams{
					UserID:          uid,
					UpdatedAt:       sshKey.UpdatedAt,
					PrivateKey:      sshKey.PrivateKey,
					PrivateKeyKeyID: sql.NullString{}, // clear the key ID
					PublicKey:       sshKey.PublicKey,
				}); err != nil {
					return xerrors.Errorf("decrypt gitsshkey user_id=%s: %w", uid, err)
				}
				log.Debug(ctx, "decrypted gitsshkey", slog.F("user_id", uid), slog.F("current", idx+1))
			}

			if err := rewriteMCPServerUserTokens(ctx, log, tx, uid, ""); err != nil {
				return err
			}

			return nil
		}, &database.TxOptions{
			Isolation: sql.LevelRepeatableRead,
		})
		if err != nil {
			return xerrors.Errorf("update user tokens and chat provider keys: %w", err)
		}
		log.Debug(ctx, "decrypted user tokens", slog.F("user_id", uid), slog.F("current", idx+1), slog.F("cipher", ciphers[0].HexDigest()))
	}

	aiProviders, err := cryptDB.GetAIProviders(ctx, database.GetAIProvidersParams{IncludeDeleted: true, IncludeDisabled: true})
	if err != nil {
		return xerrors.Errorf("get ai providers: %w", err)
	}
	log.Info(ctx, "decrypting ai provider settings", slog.F("provider_count", len(aiProviders)))
	for idx, ap := range aiProviders {
		if !ap.SettingsKeyID.Valid {
			log.Debug(ctx, "skipping ai provider", slog.F("ai_provider_id", ap.ID), slog.F("name", ap.Name), slog.F("current", idx+1))
			continue
		}
		if _, err := cryptDB.UpdateEncryptedAIProviderSettings(ctx, database.UpdateEncryptedAIProviderSettingsParams{
			ID:            ap.ID,
			Settings:      ap.Settings,
			SettingsKeyID: sql.NullString{}, // explicitly clear the key id
		}); err != nil {
			return xerrors.Errorf("decrypt ai provider id=%s name=%s: %w", ap.ID, ap.Name, err)
		}
		log.Debug(ctx, "decrypted ai provider", slog.F("ai_provider_id", ap.ID), slog.F("name", ap.Name), slog.F("current", idx+1))
	}

	aiProviderKeys, err := cryptDB.GetAIProviderKeys(ctx, true)
	if err != nil {
		return xerrors.Errorf("get ai provider keys: %w", err)
	}
	log.Info(ctx, "decrypting ai provider keys", slog.F("key_count", len(aiProviderKeys)))
	for idx, apk := range aiProviderKeys {
		if !apk.ApiKeyKeyID.Valid {
			log.Debug(ctx, "skipping ai provider key", slog.F("ai_provider_key_id", apk.ID), slog.F("provider_id", apk.ProviderID), slog.F("current", idx+1))
			continue
		}
		if _, err := cryptDB.UpdateEncryptedAIProviderKey(ctx, database.UpdateEncryptedAIProviderKeyParams{
			ID:          apk.ID,
			APIKey:      apk.APIKey,
			ApiKeyKeyID: sql.NullString{}, // explicitly clear the key id
		}); err != nil {
			return xerrors.Errorf("decrypt ai provider key id=%s provider_id=%s: %w", apk.ID, apk.ProviderID, err)
		}
		log.Debug(ctx, "decrypted ai provider key", slog.F("ai_provider_key_id", apk.ID), slog.F("provider_id", apk.ProviderID), slog.F("current", idx+1))
	}

	userAIProviderKeys, err := cryptDB.GetUserAIProviderKeys(ctx)
	if err != nil {
		return xerrors.Errorf("get user ai provider keys: %w", err)
	}
	log.Info(ctx, "decrypting user ai provider keys", slog.F("key_count", len(userAIProviderKeys)))
	for idx, key := range userAIProviderKeys {
		if !key.ApiKeyKeyID.Valid {
			log.Debug(ctx, "skipping user ai provider key", slog.F("user_ai_provider_key_id", key.ID), slog.F("ai_provider_id", key.AIProviderID), slog.F("user_id", key.UserID), slog.F("current", idx+1))
			continue
		}
		if _, err := cryptDB.UpdateEncryptedUserAIProviderKey(ctx, database.UpdateEncryptedUserAIProviderKeyParams{
			ID:          key.ID,
			APIKey:      key.APIKey,
			ApiKeyKeyID: sql.NullString{}, // explicitly clear the key id
		}); err != nil {
			return xerrors.Errorf("decrypt user ai provider key id=%s ai_provider_id=%s user_id=%s: %w", key.ID, key.AIProviderID, key.UserID, err)
		}
		log.Debug(ctx, "decrypted user ai provider key", slog.F("user_ai_provider_key_id", key.ID), slog.F("ai_provider_id", key.AIProviderID), slog.F("user_id", key.UserID), slog.F("current", idx+1))
	}

	if err := rewriteCryptoKeys(ctx, log, cryptDB, sqlDB, ""); err != nil {
		return err
	}
	if err := rewriteMCPServerConfigs(ctx, log, cryptDB, ""); err != nil {
		return err
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
DELETE FROM user_ai_provider_keys
	WHERE api_key_key_id IS NOT NULL;
DELETE FROM user_secrets
	WHERE value_key_id IS NOT NULL;
-- gitsshkeys has no delete path in product code: rows are inserted on
-- user creation and only ever mutated by regenerate. dbcrypt's 'delete'
-- command is the one operation that needs to wipe encrypted content,
-- and it does so by clearing the value rather than deleting the row,
-- so users can regenerate via the UI.
UPDATE gitsshkeys
	SET private_key = '',
		private_key_key_id = NULL
	WHERE private_key_key_id IS NOT NULL;
UPDATE ai_providers
	SET settings = NULL,
		settings_key_id = NULL
	WHERE settings_key_id IS NOT NULL;
DELETE FROM ai_provider_keys
	WHERE api_key_key_id IS NOT NULL;
UPDATE crypto_keys
	SET secret = NULL,
		secret_key_id = NULL
	WHERE secret_key_id IS NOT NULL;
UPDATE mcp_server_configs
	SET oauth2_client_secret = '',
		oauth2_client_secret_key_id = NULL,
		api_key_value = '',
		api_key_value_key_id = NULL,
		custom_headers = '',
		custom_headers_key_id = NULL
	WHERE oauth2_client_secret_key_id IS NOT NULL
		OR api_key_value_key_id IS NOT NULL
		OR custom_headers_key_id IS NOT NULL;
DELETE FROM mcp_server_user_tokens
	WHERE access_token_key_id IS NOT NULL
		OR refresh_token_key_id IS NOT NULL;
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
		return xerrors.Errorf("delete encrypted tokens and AI provider keys: %w", err)
	}
	log.Info(ctx, "deleted encrypted user tokens and AI provider API keys")

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

// rewriteCryptoKeys re-encrypts every crypto_keys.secret under the primary
// cipher, or writes plaintext when digest is empty (Decrypt). secret_key_id
// is a FK to dbcrypt_keys.active_key_digest, so leftover values block revoke.
func rewriteCryptoKeys(ctx context.Context, log slog.Logger, cryptDB database.Store, sqlDB *sql.DB, digest string) error {
	keys, err := cryptDB.GetCryptoKeys(ctx)
	if err != nil {
		return xerrors.Errorf("get crypto keys: %w", err)
	}
	action := "encrypting"
	if digest == "" {
		action = "decrypting"
	}
	log.Info(ctx, action+" crypto keys", slog.F("key_count", len(keys)))
	for idx, key := range keys {
		if digest != "" && key.SecretKeyID.Valid && key.SecretKeyID.String == digest {
			log.Debug(ctx, "skipping crypto key", slog.F("feature", key.Feature), slog.F("sequence", key.Sequence), slog.F("current", idx+1), slog.F("cipher", digest))
			continue
		}
		if digest == "" && !key.SecretKeyID.Valid {
			log.Debug(ctx, "skipping crypto key", slog.F("feature", key.Feature), slog.F("sequence", key.Sequence), slog.F("current", idx+1))
			continue
		}
		if err := updateCryptoKeySecret(ctx, cryptDB, sqlDB, key); err != nil {
			return err
		}
		log.Debug(ctx, "updated crypto key", slog.F("feature", key.Feature), slog.F("sequence", key.Sequence), slog.F("current", idx+1))
	}
	return nil
}

// updateCryptoKeySecret writes one crypto_keys row's secret and
// secret_key_id. encryptField is a no-op when the primary cipher digest is
// empty, so the same helper both re-encrypts (Rotate) and stores plaintext
// (Decrypt). There is no store update for these columns.
func updateCryptoKeySecret(ctx context.Context, crypt database.Store, sqlDB *sql.DB, key database.CryptoKey) error {
	dbc, ok := crypt.(*dbCrypt)
	if !ok {
		return xerrors.Errorf("developer error: dbcrypt store is not *dbCrypt")
	}
	if !key.Secret.Valid {
		return nil
	}
	secret := key.Secret.String
	var digest sql.NullString
	if err := dbc.encryptField(&secret, &digest); err != nil {
		return xerrors.Errorf("encrypt crypto key feature=%s sequence=%d: %w", key.Feature, key.Sequence, err)
	}
	var digestArg any
	if digest.Valid {
		digestArg = digest.String
	}
	_, err := sqlDB.ExecContext(ctx, `
		UPDATE crypto_keys
		SET secret = $1, secret_key_id = $2
		WHERE feature = $3 AND sequence = $4
	`, secret, digestArg, key.Feature, key.Sequence)
	if err != nil {
		return xerrors.Errorf("update crypto key feature=%s sequence=%d: %w", key.Feature, key.Sequence, err)
	}
	return nil
}

// rewriteMCPServerConfigs re-encrypts or decrypts every MCP server config's
// secret columns. oauth2_client_secret_key_id, api_key_value_key_id, and
// custom_headers_key_id all FK to dbcrypt_keys.active_key_digest.
func rewriteMCPServerConfigs(ctx context.Context, log slog.Logger, cryptDB database.Store, digest string) error {
	cfgs, err := allMCPServerConfigs(ctx, cryptDB)
	if err != nil {
		return err
	}
	action := "encrypting"
	if digest == "" {
		action = "decrypting"
	}
	log.Info(ctx, action+" mcp server configs", slog.F("config_count", len(cfgs)))
	for idx, cfg := range cfgs {
		if digest != "" && mcpServerConfigEncryptedWith(cfg, digest) {
			log.Debug(ctx, "skipping mcp server config", slog.F("mcp_server_config_id", cfg.ID), slog.F("current", idx+1), slog.F("cipher", digest))
			continue
		}
		if digest == "" && !mcpServerConfigHasKeyID(cfg) {
			log.Debug(ctx, "skipping mcp server config", slog.F("mcp_server_config_id", cfg.ID), slog.F("current", idx+1))
			continue
		}
		if _, err := cryptDB.UpdateMCPServerConfig(ctx, mcpServerConfigUpdateParams(cfg)); err != nil {
			return xerrors.Errorf("update mcp server config id=%s: %w", cfg.ID, err)
		}
		log.Debug(ctx, "updated mcp server config", slog.F("mcp_server_config_id", cfg.ID), slog.F("current", idx+1))
	}
	return nil
}

func allMCPServerConfigs(ctx context.Context, store database.Store) ([]database.MCPServerConfig, error) {
	var all []database.MCPServerConfig
	for _, deleted := range []bool{false, true} {
		orgs, err := store.GetOrganizations(ctx, database.GetOrganizationsParams{Deleted: deleted})
		if err != nil {
			return nil, xerrors.Errorf("get organizations deleted=%t: %w", deleted, err)
		}
		for _, org := range orgs {
			cfgs, err := store.GetMCPServerConfigsByOrganization(ctx, org.ID)
			if err != nil {
				return nil, xerrors.Errorf("get mcp server configs org=%s: %w", org.ID, err)
			}
			all = append(all, cfgs...)
		}
	}
	return all, nil
}

func mcpServerConfigHasKeyID(cfg database.MCPServerConfig) bool {
	return cfg.OAuth2ClientSecretKeyID.Valid || cfg.APIKeyValueKeyID.Valid || cfg.CustomHeadersKeyID.Valid
}

func mcpServerConfigEncryptedWith(cfg database.MCPServerConfig, digest string) bool {
	return (!cfg.OAuth2ClientSecretKeyID.Valid || cfg.OAuth2ClientSecretKeyID.String == digest) &&
		(!cfg.APIKeyValueKeyID.Valid || cfg.APIKeyValueKeyID.String == digest) &&
		(!cfg.CustomHeadersKeyID.Valid || cfg.CustomHeadersKeyID.String == digest)
}

func mcpServerConfigUpdateParams(cfg database.MCPServerConfig) database.UpdateMCPServerConfigParams {
	updatedBy := cfg.UpdatedBy.UUID
	if !cfg.UpdatedBy.Valid {
		updatedBy = cfg.CreatedBy.UUID
	}
	return database.UpdateMCPServerConfigParams{
		DisplayName:             cfg.DisplayName,
		Slug:                    cfg.Slug,
		Description:             cfg.Description,
		IconURL:                 cfg.IconURL,
		Transport:               cfg.Transport,
		Url:                     cfg.Url,
		AuthType:                cfg.AuthType,
		OAuth2ClientID:          cfg.OAuth2ClientID,
		OAuth2ClientSecret:      cfg.OAuth2ClientSecret,
		OAuth2ClientSecretKeyID: sql.NullString{},
		OAuth2AuthURL:           cfg.OAuth2AuthURL,
		OAuth2TokenURL:          cfg.OAuth2TokenURL,
		OAuth2RevocationURL:     cfg.OAuth2RevocationURL,
		OAuth2Scopes:            cfg.OAuth2Scopes,
		APIKeyHeader:            cfg.APIKeyHeader,
		APIKeyValue:             cfg.APIKeyValue,
		APIKeyValueKeyID:        sql.NullString{},
		CustomHeaders:           cfg.CustomHeaders,
		CustomHeadersKeyID:      sql.NullString{},
		ToolAllowList:           cfg.ToolAllowList,
		ToolDenyList:            cfg.ToolDenyList,
		Availability:            cfg.Availability,
		Enabled:                 cfg.Enabled,
		ModelIntent:             cfg.ModelIntent,
		AllowInPlanMode:         cfg.AllowInPlanMode,
		ForwardCoderHeaders:     cfg.ForwardCoderHeaders,
		UpdatedBy:               updatedBy,
		ID:                      cfg.ID,
	}
}

// rewriteMCPServerUserTokens re-encrypts or decrypts one user's MCP OAuth
// tokens. access_token_key_id and refresh_token_key_id FK to
// dbcrypt_keys.active_key_digest.
func rewriteMCPServerUserTokens(ctx context.Context, log slog.Logger, store database.Store, uid uuid.UUID, digest string) error {
	tokens, err := store.GetMCPServerUserTokensByUserID(ctx, uid)
	if err != nil {
		return xerrors.Errorf("get mcp server user tokens for user %s: %w", uid, err)
	}
	for _, tok := range tokens {
		if digest != "" && mcpServerUserTokenEncryptedWith(tok, digest) {
			log.Debug(ctx, "skipping mcp server user token", slog.F("user_id", uid), slog.F("mcp_server_config_id", tok.MCPServerConfigID), slog.F("cipher", digest))
			continue
		}
		if digest == "" && !tok.AccessTokenKeyID.Valid && !tok.RefreshTokenKeyID.Valid {
			log.Debug(ctx, "skipping mcp server user token", slog.F("user_id", uid), slog.F("mcp_server_config_id", tok.MCPServerConfigID))
			continue
		}
		if _, err := store.UpsertMCPServerUserToken(ctx, database.UpsertMCPServerUserTokenParams{
			MCPServerConfigID: tok.MCPServerConfigID,
			UserID:            tok.UserID,
			AccessToken:       tok.AccessToken,
			AccessTokenKeyID:  sql.NullString{},
			RefreshToken:      tok.RefreshToken,
			RefreshTokenKeyID: sql.NullString{},
			TokenType:         tok.TokenType,
			Expiry:            tok.Expiry,
		}); err != nil {
			return xerrors.Errorf("update mcp server user token user_id=%s config_id=%s: %w", tok.UserID, tok.MCPServerConfigID, err)
		}
		log.Debug(ctx, "updated mcp server user token", slog.F("user_id", uid), slog.F("mcp_server_config_id", tok.MCPServerConfigID))
	}
	return nil
}

func mcpServerUserTokenEncryptedWith(tok database.MCPServerUserToken, digest string) bool {
	return (!tok.AccessTokenKeyID.Valid || tok.AccessTokenKeyID.String == digest) &&
		(!tok.RefreshTokenKeyID.Valid || tok.RefreshTokenKeyID.String == digest)
}
