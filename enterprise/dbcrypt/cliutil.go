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
				extraCurrent := !extraHasContent(externalAuthLink.OAuthExtra) || externalAuthLink.OAuthExtraKeyID.String == ciphers[0].HexDigest()
				if externalAuthLink.OAuthAccessTokenKeyID.String == ciphers[0].HexDigest() && externalAuthLink.OAuthRefreshTokenKeyID.String == ciphers[0].HexDigest() && extraCurrent {
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
					OAuthExtraKeyID:           sql.NullString{}, // dbcrypt will update as required
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

			chatMCPServers, err := cryptTx.GetChatMCPServersByChatOwnerID(ctx, uid)
			if err != nil {
				return xerrors.Errorf("get chat MCP servers for user %s: %w", uid, err)
			}
			for _, server := range chatMCPServers {
				trimmed := strings.TrimSpace(server.Headers)
				switch {
				case trimmed == "" || trimmed == "{}":
					log.Debug(ctx, "skipping empty chat MCP server headers", slog.F("user_id", uid), slog.F("chat_id", server.ChatID), slog.F("slug", server.Slug), slog.F("current", idx+1))
				case server.HeadersKeyID.Valid && server.HeadersKeyID.String == ciphers[0].HexDigest():
					log.Debug(ctx, "skipping chat MCP server headers", slog.F("user_id", uid), slog.F("chat_id", server.ChatID), slog.F("slug", server.Slug), slog.F("current", idx+1), slog.F("cipher", ciphers[0].HexDigest()))
				default:
					if err := cryptTx.UpdateEncryptedChatMCPServerHeaders(ctx, database.UpdateEncryptedChatMCPServerHeadersParams{
						ID:           server.ID,
						Headers:      server.Headers,
						HeadersKeyID: sql.NullString{}, // dbcrypt will re-encrypt
					}); err != nil {
						return xerrors.Errorf("rotate chat MCP server headers chat_id=%s slug=%s: %w", server.ChatID, server.Slug, err)
					}
					log.Debug(ctx, "rotated chat MCP server headers", slog.F("user_id", uid), slog.F("chat_id", server.ChatID), slog.F("slug", server.Slug), slog.F("current", idx+1), slog.F("cipher", ciphers[0].HexDigest()))
				}
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

	log.Info(ctx, "encrypting workspace secrets")
	err = forEachWorkspaceSecret(ctx, cryptDB, func(secret database.WorkspaceSecret) error {
		if secret.ValueKeyID.Valid && secret.ValueKeyID.String == ciphers[0].HexDigest() {
			log.Debug(ctx, "skipping workspace secret", slog.F("workspace_id", secret.WorkspaceID), slog.F("secret_name", secret.Name), slog.F("cipher", ciphers[0].HexDigest()))
			return nil
		}
		if _, err := cryptDB.UpdateEncryptedWorkspaceSecretValue(ctx, database.UpdateEncryptedWorkspaceSecretValueParams{
			ID:         secret.ID,
			Value:      secret.Value,
			ValueKeyID: sql.NullString{}, // dbcrypt will re-encrypt
		}); err != nil {
			if xerrors.Is(err, sql.ErrNoRows) {
				// A build cleared the row after it was listed; there is no
				// value left to rotate.
				log.Debug(ctx, "skipping cleared workspace secret", slog.F("workspace_id", secret.WorkspaceID), slog.F("secret_name", secret.Name))
				return nil
			}
			return xerrors.Errorf("rotate workspace secret workspace_id=%s name=%s: %w", secret.WorkspaceID, secret.Name, err)
		}
		log.Debug(ctx, "rotated workspace secret", slog.F("workspace_id", secret.WorkspaceID), slog.F("secret_name", secret.Name), slog.F("cipher", ciphers[0].HexDigest()))
		return nil
	})
	if err != nil {
		return err
	}

	log.Info(ctx, "encrypting workspace build provisioner state")
	if err := rewriteWorkspaceBuildStates(ctx, log, cryptDB, ciphers[0].HexDigest()); err != nil {
		return err
	}

	log.Info(ctx, "encrypting cached template version terraform plans")
	if err := rewriteCachedPlans(ctx, log, cryptDB, ciphers[0].HexDigest()); err != nil {
		return err
	}

	log.Info(ctx, "encrypting sensitive template version variables")
	if err := rewriteTemplateVersionVariables(ctx, log, cryptDB, ciphers[0].HexDigest()); err != nil {
		return err
	}

	log.Info(ctx, "encrypting sensitive workspace build parameters")
	if err := rewriteWorkspaceBuildParameters(ctx, log, cryptDB, ciphers[0].HexDigest()); err != nil {
		return err
	}

	log.Info(ctx, "encrypting sensitive workspace resource metadata")
	if err := rewriteWorkspaceResourceMetadata(ctx, log, cryptDB, ciphers[0].HexDigest()); err != nil {
		return err
	}

	log.Info(ctx, "encrypting workspace agent environment variables")
	if err := rewriteWorkspaceAgentEnvironmentVariables(ctx, log, cryptDB, ciphers[0].HexDigest()); err != nil {
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
				if !externalAuthLink.OAuthAccessTokenKeyID.Valid && !externalAuthLink.OAuthRefreshTokenKeyID.Valid && !externalAuthLink.OAuthExtraKeyID.Valid {
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
					OAuthExtraKeyID:           sql.NullString{}, // we explicitly want to clear the key id
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

			chatMCPServers, err := tx.GetChatMCPServersByChatOwnerID(ctx, uid)
			if err != nil {
				return xerrors.Errorf("get chat MCP servers for user %s: %w", uid, err)
			}
			for _, server := range chatMCPServers {
				if !server.HeadersKeyID.Valid {
					log.Debug(ctx, "skipping chat MCP server headers", slog.F("user_id", uid), slog.F("chat_id", server.ChatID), slog.F("slug", server.Slug), slog.F("current", idx+1))
					continue
				}
				if err := tx.UpdateEncryptedChatMCPServerHeaders(ctx, database.UpdateEncryptedChatMCPServerHeadersParams{
					ID:           server.ID,
					Headers:      server.Headers,
					HeadersKeyID: sql.NullString{}, // clear the key ID
				}); err != nil {
					return xerrors.Errorf("decrypt chat MCP server headers chat_id=%s slug=%s: %w", server.ChatID, server.Slug, err)
				}
				log.Debug(ctx, "decrypted chat MCP server headers", slog.F("user_id", uid), slog.F("chat_id", server.ChatID), slog.F("slug", server.Slug), slog.F("current", idx+1))
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

	log.Info(ctx, "decrypting workspace secrets")
	err = forEachWorkspaceSecret(ctx, cryptDB, func(secret database.WorkspaceSecret) error {
		if !secret.ValueKeyID.Valid {
			log.Debug(ctx, "skipping workspace secret", slog.F("workspace_id", secret.WorkspaceID), slog.F("secret_name", secret.Name))
			return nil
		}
		if _, err := cryptDB.UpdateEncryptedWorkspaceSecretValue(ctx, database.UpdateEncryptedWorkspaceSecretValueParams{
			ID:         secret.ID,
			Value:      secret.Value,
			ValueKeyID: sql.NullString{}, // primaryCipherDigest is empty, so dbcrypt writes this value as plaintext.
		}); err != nil {
			if xerrors.Is(err, sql.ErrNoRows) {
				// A build cleared the row after it was listed; there is no
				// value left to decrypt.
				log.Debug(ctx, "skipping cleared workspace secret", slog.F("workspace_id", secret.WorkspaceID), slog.F("secret_name", secret.Name))
				return nil
			}
			return xerrors.Errorf("decrypt workspace secret workspace_id=%s name=%s: %w", secret.WorkspaceID, secret.Name, err)
		}
		log.Debug(ctx, "decrypted workspace secret", slog.F("workspace_id", secret.WorkspaceID), slog.F("secret_name", secret.Name))
		return nil
	})
	if err != nil {
		return err
	}

	log.Info(ctx, "decrypting workspace build provisioner state")
	if err := rewriteWorkspaceBuildStates(ctx, log, cryptDB, ""); err != nil {
		return err
	}

	log.Info(ctx, "decrypting cached template version terraform plans")
	if err := rewriteCachedPlans(ctx, log, cryptDB, ""); err != nil {
		return err
	}

	log.Info(ctx, "decrypting sensitive template version variables")
	if err := rewriteTemplateVersionVariables(ctx, log, cryptDB, ""); err != nil {
		return err
	}

	log.Info(ctx, "decrypting sensitive workspace build parameters")
	if err := rewriteWorkspaceBuildParameters(ctx, log, cryptDB, ""); err != nil {
		return err
	}

	log.Info(ctx, "decrypting sensitive workspace resource metadata")
	if err := rewriteWorkspaceResourceMetadata(ctx, log, cryptDB, ""); err != nil {
		return err
	}

	log.Info(ctx, "decrypting workspace agent environment variables")
	if err := rewriteWorkspaceAgentEnvironmentVariables(ctx, log, cryptDB, ""); err != nil {
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

// workspaceSecretsPageSize bounds how many workspace secrets Rotate and
// Decrypt hold in memory at once.
const workspaceSecretsPageSize = 1000

// keyIDPageSize bounds how many rows Rotate and Decrypt list at once when
// they look up the key each value is stored with.
const keyIDPageSize = 100

// rewriteWorkspaceBuildStates saves the provisioner state of every workspace
// build again through cryptDB, which stores it with wantKeyID. Builds whose
// state is already stored with wantKeyID are skipped. Rotate passes the digest
// of the primary cipher. Decrypt passes "", because it stores values
// unencrypted.
func rewriteWorkspaceBuildStates(ctx context.Context, log slog.Logger, cryptDB database.Store, wantKeyID string) error {
	afterID := uuid.Nil
	for {
		page, err := cryptDB.GetWorkspaceBuildProvisionerStateKeyIDs(ctx, database.GetWorkspaceBuildProvisionerStateKeyIDsParams{
			AfterID:    afterID,
			LimitCount: keyIDPageSize,
		})
		if err != nil {
			return xerrors.Errorf("list workspace builds with provisioner state after id=%s: %w", afterID, err)
		}
		for _, build := range page {
			if build.ProvisionerStateKeyID.String == wantKeyID {
				log.Debug(ctx, "skipping workspace build provisioner state", slog.F("build_id", build.ID), slog.F("key_id", wantKeyID))
				continue
			}
			if err := rewriteWorkspaceBuildState(ctx, cryptDB, build.ID, wantKeyID); err != nil {
				return xerrors.Errorf("rewrite provisioner state for build %s: %w", build.ID, err)
			}
			log.Debug(ctx, "rewrote workspace build provisioner state", slog.F("build_id", build.ID), slog.F("key_id", wantKeyID))
		}
		if len(page) < keyIDPageSize {
			return nil
		}
		afterID = page[len(page)-1].ID
	}
}

// buildStateRewriteAttempts is how many times rewriteWorkspaceBuildState
// tries a build whose state keeps changing before it gives up.
const buildStateRewriteAttempts = 3

// rewriteWorkspaceBuildState saves the provisioner state of one workspace
// build again. The control plane can store new state for the build while the
// sweep runs, so the state is read and written in one repeatable read
// transaction. If the row changes after the read, Postgres fails the write
// instead of replacing the new state, and the build is read again.
func rewriteWorkspaceBuildState(ctx context.Context, cryptDB database.Store, buildID uuid.UUID, wantKeyID string) error {
	var err error
	for attempt := 0; attempt < buildStateRewriteAttempts; attempt++ {
		err = cryptDB.InTx(func(tx database.Store) error {
			return rewriteWorkspaceBuildStateInTx(ctx, tx, buildID, wantKeyID)
		}, &database.TxOptions{
			Isolation: sql.LevelRepeatableRead,
		})
		if !database.IsSerializedError(err) {
			return err
		}
	}
	return err
}

// rewriteWorkspaceBuildStateInTx is the body of the transaction that
// rewriteWorkspaceBuildState runs.
func rewriteWorkspaceBuildStateInTx(ctx context.Context, tx database.Store, buildID uuid.UUID, wantKeyID string) error {
	state, err := tx.GetWorkspaceBuildProvisionerStateByID(ctx, buildID)
	if err != nil {
		return xerrors.Errorf("get provisioner state: %w", err)
	}
	if state.ProvisionerStateKeyID.String == wantKeyID {
		// The state changed since it was listed and is already stored with
		// wantKeyID.
		return nil
	}
	return tx.UpdateEncryptedWorkspaceBuildProvisionerState(ctx, database.UpdateEncryptedWorkspaceBuildProvisionerStateParams{
		ID:                    buildID,
		ProvisionerState:      state.ProvisionerState,
		ProvisionerStateKeyID: sql.NullString{}, // Set by cryptDB.
	})
}

// rewriteCachedPlans saves the cached plan of every template version again
// through cryptDB, which stores it with wantKeyID. Plans already stored with
// wantKeyID are skipped. See rewriteWorkspaceBuildStates for the values of
// wantKeyID.
func rewriteCachedPlans(ctx context.Context, log slog.Logger, cryptDB database.Store, wantKeyID string) error {
	afterID := uuid.Nil
	for {
		page, err := cryptDB.GetTemplateVersionTerraformValuesCachedPlanKeyIDs(ctx, database.GetTemplateVersionTerraformValuesCachedPlanKeyIDsParams{
			AfterID:    afterID,
			LimitCount: keyIDPageSize,
		})
		if err != nil {
			return xerrors.Errorf("list template versions with a cached plan after id=%s: %w", afterID, err)
		}
		for _, version := range page {
			if version.CachedPlanKeyID.String == wantKeyID {
				log.Debug(ctx, "skipping cached plan", slog.F("template_version_id", version.TemplateVersionID), slog.F("key_id", wantKeyID))
				continue
			}
			values, err := cryptDB.GetTemplateVersionTerraformValues(ctx, version.TemplateVersionID)
			if err != nil {
				return xerrors.Errorf("get cached plan for template version %s: %w", version.TemplateVersionID, err)
			}
			err = cryptDB.UpdateTemplateVersionTerraformValuesCachedPlan(ctx, database.UpdateTemplateVersionTerraformValuesCachedPlanParams{
				TemplateVersionID: version.TemplateVersionID,
				CachedPlan:        values.CachedPlan,
				CachedPlanKeyID:   sql.NullString{}, // Set by cryptDB.
				UpdatedAt:         values.UpdatedAt,
			})
			if err != nil {
				return xerrors.Errorf("rewrite cached plan for template version %s: %w", version.TemplateVersionID, err)
			}
			log.Debug(ctx, "rewrote cached plan", slog.F("template_version_id", version.TemplateVersionID), slog.F("key_id", wantKeyID))
		}
		if len(page) < keyIDPageSize {
			return nil
		}
		afterID = page[len(page)-1].TemplateVersionID
	}
}

// rewriteTemplateVersionVariables saves the value of every sensitive template
// version variable again through cryptDB, which stores it with wantKeyID.
// Values already stored with wantKeyID are skipped. See
// rewriteWorkspaceBuildStates for the values of wantKeyID.
func rewriteTemplateVersionVariables(ctx context.Context, log slog.Logger, cryptDB database.Store, wantKeyID string) error {
	afterID := uuid.Nil
	for {
		page, err := cryptDB.GetTemplateVersionIDsWithSensitiveVariables(ctx, database.GetTemplateVersionIDsWithSensitiveVariablesParams{
			AfterID:    afterID,
			LimitCount: keyIDPageSize,
		})
		if err != nil {
			return xerrors.Errorf("list template versions with sensitive variables after id=%s: %w", afterID, err)
		}
		for _, versionID := range page {
			variables, err := cryptDB.GetTemplateVersionVariables(ctx, versionID)
			if err != nil {
				return xerrors.Errorf("get variables for template version %s: %w", versionID, err)
			}
			for _, variable := range variables {
				if !variable.Sensitive || variable.Value == "" || variable.ValueKeyID.String == wantKeyID {
					continue
				}
				err = cryptDB.UpdateTemplateVersionVariableValue(ctx, database.UpdateTemplateVersionVariableValueParams{
					TemplateVersionID: versionID,
					Name:              variable.Name,
					Value:             variable.Value,
					ValueKeyID:        sql.NullString{}, // Set by cryptDB.
				})
				if err != nil {
					return xerrors.Errorf("rewrite variable %s of template version %s: %w", variable.Name, versionID, err)
				}
				log.Debug(ctx, "rewrote template version variable", slog.F("template_version_id", versionID), slog.F("name", variable.Name), slog.F("key_id", wantKeyID))
			}
		}
		if len(page) < keyIDPageSize {
			return nil
		}
		afterID = page[len(page)-1]
	}
}

// rewriteWorkspaceBuildParameters saves every sensitive workspace build
// parameter value again through cryptDB, which stores it with wantKeyID.
// Values already stored with wantKeyID are skipped. The listing includes the
// values of ephemeral parameters that were written before the sensitive
// column existed, and rewriting one marks it sensitive. See
// rewriteWorkspaceBuildStates for the values of wantKeyID.
func rewriteWorkspaceBuildParameters(ctx context.Context, log slog.Logger, cryptDB database.Store, wantKeyID string) error {
	afterBuildID := uuid.Nil
	afterName := ""
	for {
		page, err := cryptDB.GetSensitiveWorkspaceBuildParameters(ctx, database.GetSensitiveWorkspaceBuildParametersParams{
			AfterWorkspaceBuildID: afterBuildID,
			AfterName:             afterName,
			LimitCount:            keyIDPageSize,
		})
		if err != nil {
			return xerrors.Errorf("list sensitive workspace build parameters after build=%s name=%q: %w", afterBuildID, afterName, err)
		}
		for _, parameter := range page {
			if parameter.ValueKeyID.String == wantKeyID {
				continue
			}
			err = cryptDB.UpdateWorkspaceBuildParameterValue(ctx, database.UpdateWorkspaceBuildParameterValueParams{
				WorkspaceBuildID: parameter.WorkspaceBuildID,
				Name:             parameter.Name,
				Value:            parameter.Value,
				ValueKeyID:       sql.NullString{}, // Set by cryptDB.
			})
			if err != nil {
				return xerrors.Errorf("rewrite parameter %s of workspace build %s: %w", parameter.Name, parameter.WorkspaceBuildID, err)
			}
			log.Debug(ctx, "rewrote workspace build parameter", slog.F("workspace_build_id", parameter.WorkspaceBuildID), slog.F("name", parameter.Name), slog.F("key_id", wantKeyID))
		}
		if len(page) < keyIDPageSize {
			return nil
		}
		last := page[len(page)-1]
		afterBuildID, afterName = last.WorkspaceBuildID, last.Name
	}
}

// rewriteWorkspaceResourceMetadata saves the value of every sensitive
// workspace resource metadatum again through cryptDB, which stores it with
// wantKeyID. Values already stored with wantKeyID are skipped. See
// rewriteWorkspaceBuildStates for the values of wantKeyID.
func rewriteWorkspaceResourceMetadata(ctx context.Context, log slog.Logger, cryptDB database.Store, wantKeyID string) error {
	var afterID int64
	for {
		page, err := cryptDB.GetSensitiveWorkspaceResourceMetadata(ctx, database.GetSensitiveWorkspaceResourceMetadataParams{
			AfterID:    afterID,
			LimitCount: keyIDPageSize,
		})
		if err != nil {
			return xerrors.Errorf("list sensitive workspace resource metadata after id=%d: %w", afterID, err)
		}
		for _, metadatum := range page {
			if metadatum.ValueKeyID.String == wantKeyID {
				continue
			}
			err = cryptDB.UpdateWorkspaceResourceMetadatumValue(ctx, database.UpdateWorkspaceResourceMetadatumValueParams{
				ID:         metadatum.ID,
				Value:      metadatum.Value,
				ValueKeyID: sql.NullString{}, // Set by cryptDB.
			})
			if err != nil {
				return xerrors.Errorf("rewrite metadatum %s of workspace resource %s: %w", metadatum.Key, metadatum.WorkspaceResourceID, err)
			}
			log.Debug(ctx, "rewrote workspace resource metadatum", slog.F("workspace_resource_id", metadatum.WorkspaceResourceID), slog.F("key", metadatum.Key), slog.F("key_id", wantKeyID))
		}
		if len(page) < keyIDPageSize {
			return nil
		}
		afterID = page[len(page)-1].ID
	}
}

// rewriteWorkspaceAgentEnvironmentVariables saves the environment variables
// of every workspace agent again through cryptDB, which stores their values
// with wantKeyID. Agents whose values are already stored with wantKeyID are
// skipped. See rewriteWorkspaceBuildStates for the values of wantKeyID.
func rewriteWorkspaceAgentEnvironmentVariables(ctx context.Context, log slog.Logger, cryptDB database.Store, wantKeyID string) error {
	afterID := uuid.Nil
	for {
		page, err := cryptDB.GetWorkspaceAgentEnvironmentVariables(ctx, database.GetWorkspaceAgentEnvironmentVariablesParams{
			AfterID:    afterID,
			LimitCount: keyIDPageSize,
		})
		if err != nil {
			return xerrors.Errorf("list workspace agents with environment variables after id=%s: %w", afterID, err)
		}
		for _, agent := range page {
			if agent.EnvironmentVariablesKeyID.String == wantKeyID {
				continue
			}
			err = cryptDB.UpdateWorkspaceAgentEnvironmentVariablesByID(ctx, database.UpdateWorkspaceAgentEnvironmentVariablesByIDParams{
				ID:                        agent.ID,
				EnvironmentVariables:      agent.EnvironmentVariables,
				EnvironmentVariablesKeyID: sql.NullString{}, // Set by cryptDB.
			})
			if err != nil {
				return xerrors.Errorf("rewrite environment variables of workspace agent %s: %w", agent.ID, err)
			}
			log.Debug(ctx, "rewrote workspace agent environment variables", slog.F("workspace_agent_id", agent.ID), slog.F("key_id", wantKeyID))
		}
		if len(page) < keyIDPageSize {
			return nil
		}
		afterID = page[len(page)-1].ID
	}
}

// forEachWorkspaceSecret calls fn for every workspace secret that still holds
// a value, one page at a time.
func forEachWorkspaceSecret(ctx context.Context, store database.Store, fn func(database.WorkspaceSecret) error) error {
	afterID := uuid.Nil
	for {
		page, err := store.GetWorkspaceSecrets(ctx, database.GetWorkspaceSecretsParams{
			AfterID:    afterID,
			LimitCount: workspaceSecretsPageSize,
		})
		if err != nil {
			return xerrors.Errorf("get workspace secrets after id=%s: %w", afterID, err)
		}
		for _, secret := range page {
			if err := fn(secret); err != nil {
				return err
			}
		}
		if len(page) < workspaceSecretsPageSize {
			return nil
		}
		afterID = page[len(page)-1].ID
	}
}

// nolint: gosec
const sqlDeleteEncryptedUserTokens = `
BEGIN;
DELETE FROM user_links
  WHERE oauth_access_token_key_id IS NOT NULL
	OR oauth_refresh_token_key_id IS NOT NULL;
DELETE FROM external_auth_links
	WHERE oauth_access_token_key_id IS NOT NULL
	OR oauth_refresh_token_key_id IS NOT NULL
	OR oauth_extra_key_id IS NOT NULL;
DELETE FROM user_ai_provider_keys
	WHERE api_key_key_id IS NOT NULL;
DELETE FROM user_secrets
	WHERE value_key_id IS NOT NULL;
-- Keep workspace_secrets rows as build history.
UPDATE workspace_secrets
	SET value = NULL, value_key_id = NULL, cleared_at = CURRENT_TIMESTAMP
	WHERE value_key_id IS NOT NULL;
DELETE FROM chat_mcp_servers
	WHERE headers_key_id IS NOT NULL;
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
-- Terraform state cannot be recovered without the encryption keys. Clear it
-- rather than leaving unreadable ciphertext; affected workspaces must be
-- rebuilt and may leave orphaned cloud resources behind.
UPDATE workspace_builds
	SET provisioner_state = NULL,
		provisioner_state_key_id = NULL
	WHERE provisioner_state_key_id IS NOT NULL;
-- Cached Terraform plans are regenerated on the next template version import
-- or dynamic-parameter render; callers already handle a missing row.
DELETE FROM template_version_terraform_values
	WHERE cached_plan_key_id IS NOT NULL;
-- Encrypted template variable values cannot be recovered without the keys.
-- Clear them; affected templates need a new version that provides the values
-- again.
UPDATE template_version_variables
	SET value = '',
		value_key_id = NULL
	WHERE value_key_id IS NOT NULL;
-- Encrypted workspace build parameter values cannot be recovered without the
-- keys. Clear them.
UPDATE workspace_build_parameters
	SET value = '',
		value_key_id = NULL
	WHERE value_key_id IS NOT NULL;
-- Encrypted workspace resource metadata values cannot be recovered without
-- the keys. Clear them; they are written again on the next workspace build.
UPDATE workspace_resource_metadata
	SET value = NULL,
		value_key_id = NULL
	WHERE value_key_id IS NOT NULL;
-- Encrypted workspace agent environment variables cannot be recovered without
-- the keys. Clear them; they are written again on the next workspace build.
UPDATE workspace_agents
	SET environment_variables = NULL,
		environment_variables_key_id = NULL
	WHERE environment_variables_key_id IS NOT NULL;
DELETE FROM ai_provider_keys
	WHERE api_key_key_id IS NOT NULL;
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
