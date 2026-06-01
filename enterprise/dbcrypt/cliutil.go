package dbcrypt

import (
	"context"
	"database/sql"
	"strings"

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
				}); err != nil {
					return xerrors.Errorf("rotate user secret user_id=%s name=%s: %w", uid, secret.Name, err)
				}
				log.Debug(ctx, "rotated user secret", slog.F("user_id", uid), slog.F("secret_name", secret.Name), slog.F("current", idx+1), slog.F("cipher", ciphers[0].HexDigest()))
			}

			mcpUserTokens, err := cryptTx.GetMCPServerUserTokensByUserID(ctx, uid)
			if err != nil {
				return xerrors.Errorf("get mcp server user tokens for user %s: %w", uid, err)
			}
			for _, tok := range mcpUserTokens {
				if tok.AccessTokenKeyID.Valid && tok.AccessTokenKeyID.String == ciphers[0].HexDigest() &&
					tok.RefreshTokenKeyID.Valid && tok.RefreshTokenKeyID.String == ciphers[0].HexDigest() {
					log.Debug(ctx, "skipping mcp server user token", slog.F("user_id", uid), slog.F("mcp_server_config_id", tok.MCPServerConfigID), slog.F("current", idx+1), slog.F("cipher", ciphers[0].HexDigest()))
					continue
				}
				if _, err := cryptTx.UpsertMCPServerUserToken(ctx, database.UpsertMCPServerUserTokenParams{
					MCPServerConfigID: tok.MCPServerConfigID,
					UserID:            uid,
					AccessToken:       tok.AccessToken,
					AccessTokenKeyID:  sql.NullString{}, // dbcrypt will re-encrypt
					RefreshToken:      tok.RefreshToken,
					RefreshTokenKeyID: sql.NullString{}, // dbcrypt will re-encrypt
					TokenType:         tok.TokenType,
					Expiry:            tok.Expiry,
				}); err != nil {
					return xerrors.Errorf("rotate mcp server user token user_id=%s mcp_server_config_id=%s: %w", uid, tok.MCPServerConfigID, err)
				}
				log.Debug(ctx, "rotated mcp server user token", slog.F("user_id", uid), slog.F("mcp_server_config_id", tok.MCPServerConfigID), slog.F("current", idx+1), slog.F("cipher", ciphers[0].HexDigest()))
			}

			mcpHeaderRows, err := cryptTx.GetMCPServerUserHeaderValuesByUserID(ctx, uid)
			if err != nil {
				return xerrors.Errorf("get mcp server user header values for user %s: %w", uid, err)
			}
			for _, row := range mcpHeaderRows {
				if row.HeaderValuesKeyID.Valid && row.HeaderValuesKeyID.String == ciphers[0].HexDigest() {
					log.Debug(ctx, "skipping mcp server user header values", slog.F("user_id", uid), slog.F("mcp_server_config_id", row.MCPServerConfigID), slog.F("current", idx+1), slog.F("cipher", ciphers[0].HexDigest()))
					continue
				}
				if _, err := cryptTx.UpsertMCPServerUserHeaderValues(ctx, database.UpsertMCPServerUserHeaderValuesParams{
					MCPServerConfigID: row.MCPServerConfigID,
					UserID:            uid,
					HeaderValues:      row.HeaderValues,
					HeaderValuesKeyID: sql.NullString{}, // dbcrypt will re-encrypt
				}); err != nil {
					return xerrors.Errorf("rotate mcp server user header values user_id=%s mcp_server_config_id=%s: %w", uid, row.MCPServerConfigID, err)
				}
				log.Debug(ctx, "rotated mcp server user header values", slog.F("user_id", uid), slog.F("mcp_server_config_id", row.MCPServerConfigID), slog.F("current", idx+1), slog.F("cipher", ciphers[0].HexDigest()))
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

	mcpConfigs, err := cryptDB.GetMCPServerConfigs(ctx)
	if err != nil {
		return xerrors.Errorf("get mcp server configs: %w", err)
	}
	log.Info(ctx, "encrypting mcp server config secrets", slog.F("config_count", len(mcpConfigs)))
	for idx, cfg := range mcpConfigs {
		oauth2Current := cfg.OAuth2ClientSecretKeyID.Valid && cfg.OAuth2ClientSecretKeyID.String == ciphers[0].HexDigest()
		apiKeyCurrent := cfg.APIKeyValueKeyID.Valid && cfg.APIKeyValueKeyID.String == ciphers[0].HexDigest()
		customHeadersCurrent := cfg.CustomHeadersKeyID.Valid && cfg.CustomHeadersKeyID.String == ciphers[0].HexDigest()
		if oauth2Current && apiKeyCurrent && customHeadersCurrent {
			log.Debug(ctx, "skipping mcp server config", slog.F("mcp_server_config_id", cfg.ID), slog.F("slug", cfg.Slug), slog.F("current", idx+1), slog.F("cipher", ciphers[0].HexDigest()))
			continue
		}
		if _, err := cryptDB.UpdateEncryptedMCPServerConfig(ctx, database.UpdateEncryptedMCPServerConfigParams{
			ID:                      cfg.ID,
			OAuth2ClientSecret:      cfg.OAuth2ClientSecret,
			OAuth2ClientSecretKeyID: sql.NullString{}, // dbcrypt will update as required
			APIKeyValue:             cfg.APIKeyValue,
			APIKeyValueKeyID:        sql.NullString{}, // dbcrypt will update as required
			CustomHeaders:           cfg.CustomHeaders,
			CustomHeadersKeyID:      sql.NullString{}, // dbcrypt will update as required
		}); err != nil {
			return xerrors.Errorf("update mcp server config id=%s slug=%s: %w", cfg.ID, cfg.Slug, err)
		}
		log.Debug(ctx, "encrypted mcp server config", slog.F("mcp_server_config_id", cfg.ID), slog.F("slug", cfg.Slug), slog.F("current", idx+1), slog.F("cipher", ciphers[0].HexDigest()))
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
				}); err != nil {
					return xerrors.Errorf("decrypt user secret user_id=%s name=%s: %w", uid, secret.Name, err)
				}
				log.Debug(ctx, "decrypted user secret", slog.F("user_id", uid), slog.F("secret_name", secret.Name), slog.F("current", idx+1))
			}

			mcpUserTokens, err := tx.GetMCPServerUserTokensByUserID(ctx, uid)
			if err != nil {
				return xerrors.Errorf("get mcp server user tokens for user %s: %w", uid, err)
			}
			for _, tok := range mcpUserTokens {
				if !tok.AccessTokenKeyID.Valid && !tok.RefreshTokenKeyID.Valid {
					log.Debug(ctx, "skipping mcp server user token", slog.F("user_id", uid), slog.F("mcp_server_config_id", tok.MCPServerConfigID), slog.F("current", idx+1))
					continue
				}
				if _, err := tx.UpsertMCPServerUserToken(ctx, database.UpsertMCPServerUserTokenParams{
					MCPServerConfigID: tok.MCPServerConfigID,
					UserID:            uid,
					AccessToken:       tok.AccessToken,
					AccessTokenKeyID:  sql.NullString{}, // clear the key ID
					RefreshToken:      tok.RefreshToken,
					RefreshTokenKeyID: sql.NullString{}, // clear the key ID
					TokenType:         tok.TokenType,
					Expiry:            tok.Expiry,
				}); err != nil {
					return xerrors.Errorf("decrypt mcp server user token user_id=%s mcp_server_config_id=%s: %w", uid, tok.MCPServerConfigID, err)
				}
				log.Debug(ctx, "decrypted mcp server user token", slog.F("user_id", uid), slog.F("mcp_server_config_id", tok.MCPServerConfigID), slog.F("current", idx+1))
			}

			mcpHeaderRows, err := tx.GetMCPServerUserHeaderValuesByUserID(ctx, uid)
			if err != nil {
				return xerrors.Errorf("get mcp server user header values for user %s: %w", uid, err)
			}
			for _, row := range mcpHeaderRows {
				if !row.HeaderValuesKeyID.Valid {
					log.Debug(ctx, "skipping mcp server user header values", slog.F("user_id", uid), slog.F("mcp_server_config_id", row.MCPServerConfigID), slog.F("current", idx+1))
					continue
				}
				if _, err := tx.UpsertMCPServerUserHeaderValues(ctx, database.UpsertMCPServerUserHeaderValuesParams{
					MCPServerConfigID: row.MCPServerConfigID,
					UserID:            uid,
					HeaderValues:      row.HeaderValues,
					HeaderValuesKeyID: sql.NullString{}, // clear the key ID
				}); err != nil {
					return xerrors.Errorf("decrypt mcp server user header values user_id=%s mcp_server_config_id=%s: %w", uid, row.MCPServerConfigID, err)
				}
				log.Debug(ctx, "decrypted mcp server user header values", slog.F("user_id", uid), slog.F("mcp_server_config_id", row.MCPServerConfigID), slog.F("current", idx+1))
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

	mcpConfigs, err := cryptDB.GetMCPServerConfigs(ctx)
	if err != nil {
		return xerrors.Errorf("get mcp server configs: %w", err)
	}
	log.Info(ctx, "decrypting mcp server config secrets", slog.F("config_count", len(mcpConfigs)))
	for idx, cfg := range mcpConfigs {
		if !cfg.OAuth2ClientSecretKeyID.Valid && !cfg.APIKeyValueKeyID.Valid && !cfg.CustomHeadersKeyID.Valid {
			log.Debug(ctx, "skipping mcp server config", slog.F("mcp_server_config_id", cfg.ID), slog.F("slug", cfg.Slug), slog.F("current", idx+1))
			continue
		}
		if _, err := cryptDB.UpdateEncryptedMCPServerConfig(ctx, database.UpdateEncryptedMCPServerConfigParams{
			ID:                      cfg.ID,
			OAuth2ClientSecret:      cfg.OAuth2ClientSecret,
			OAuth2ClientSecretKeyID: sql.NullString{}, // explicitly clear the key id
			APIKeyValue:             cfg.APIKeyValue,
			APIKeyValueKeyID:        sql.NullString{}, // explicitly clear the key id
			CustomHeaders:           cfg.CustomHeaders,
			CustomHeadersKeyID:      sql.NullString{}, // explicitly clear the key id
		}); err != nil {
			return xerrors.Errorf("decrypt mcp server config id=%s slug=%s: %w", cfg.ID, cfg.Slug, err)
		}
		log.Debug(ctx, "decrypted mcp server config", slog.F("mcp_server_config_id", cfg.ID), slog.F("slug", cfg.Slug), slog.F("current", idx+1))
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
UPDATE ai_providers
	SET settings = NULL,
		settings_key_id = NULL
	WHERE settings_key_id IS NOT NULL;
DELETE FROM ai_provider_keys
	WHERE api_key_key_id IS NOT NULL;
DELETE FROM mcp_server_user_tokens
	WHERE access_token_key_id IS NOT NULL
	OR refresh_token_key_id IS NOT NULL;
DELETE FROM mcp_server_user_header_values
	WHERE header_values_key_id IS NOT NULL;
UPDATE mcp_server_configs
	SET oauth2_client_secret = '',
		oauth2_client_secret_key_id = NULL,
		api_key_value = '',
		api_key_value_key_id = NULL,
		custom_headers = '{}',
		custom_headers_key_id = NULL
	WHERE oauth2_client_secret_key_id IS NOT NULL
	OR api_key_value_key_id IS NOT NULL
	OR custom_headers_key_id IS NOT NULL;
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
