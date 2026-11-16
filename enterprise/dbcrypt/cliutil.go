package dbcrypt

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
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
				extraCurrent := !extraHoldsSecret(externalAuthLink.OAuthExtra) || externalAuthLink.OAuthExtraKeyID.String == ciphers[0].HexDigest()
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
	err = forEachSecretRow(ctx, sqlDB, workspaceBuildStateScan, func(buildID uuid.UUID, keyID sql.NullString) error {
		if keyID.Valid && keyID.String == ciphers[0].HexDigest() {
			log.Debug(ctx, "skipping workspace build provisioner state", slog.F("build_id", buildID), slog.F("cipher", ciphers[0].HexDigest()))
			return nil
		}
		row, err := cryptDB.GetWorkspaceBuildProvisionerStateByID(ctx, buildID)
		if err != nil {
			return xerrors.Errorf("get provisioner state for build %s: %w", buildID, err)
		}
		if err := cryptDB.UpdateWorkspaceBuildProvisionerStateByID(ctx, database.UpdateWorkspaceBuildProvisionerStateByIDParams{
			ID:                    buildID,
			UpdatedAt:             dbtime.Now(),
			ProvisionerState:      row.ProvisionerState,
			ProvisionerStateKeyID: sql.NullString{}, // dbcrypt will re-encrypt
		}); err != nil {
			return xerrors.Errorf("rotate provisioner state for build %s: %w", buildID, err)
		}
		log.Debug(ctx, "rotated workspace build provisioner state", slog.F("build_id", buildID), slog.F("cipher", ciphers[0].HexDigest()))
		return nil
	})
	if err != nil {
		return err
	}

	log.Info(ctx, "encrypting cached template version terraform plans")
	err = forEachSecretRow(ctx, sqlDB, terraformValuesCachedPlanScan, func(templateVersionID uuid.UUID, keyID sql.NullString) error {
		if keyID.Valid && keyID.String == ciphers[0].HexDigest() {
			log.Debug(ctx, "skipping cached plan", slog.F("template_version_id", templateVersionID), slog.F("cipher", ciphers[0].HexDigest()))
			return nil
		}
		row, err := cryptDB.GetTemplateVersionTerraformValues(ctx, templateVersionID)
		if err != nil {
			return xerrors.Errorf("get cached plan for template version %s: %w", templateVersionID, err)
		}
		if err := cryptDB.UpdateTemplateVersionTerraformValuesCachedPlan(ctx, database.UpdateTemplateVersionTerraformValuesCachedPlanParams{
			TemplateVersionID: templateVersionID,
			CachedPlan:        row.CachedPlan,
			CachedPlanKeyID:   sql.NullString{}, // dbcrypt will re-encrypt
			UpdatedAt:         row.UpdatedAt,
		}); err != nil {
			return xerrors.Errorf("rotate cached plan for template version %s: %w", templateVersionID, err)
		}
		log.Debug(ctx, "rotated cached plan", slog.F("template_version_id", templateVersionID), slog.F("cipher", ciphers[0].HexDigest()))
		return nil
	})
	if err != nil {
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
	err = forEachSecretRow(ctx, sqlDB, workspaceBuildStateScan, func(buildID uuid.UUID, keyID sql.NullString) error {
		if !keyID.Valid {
			log.Debug(ctx, "skipping workspace build provisioner state", slog.F("build_id", buildID))
			return nil
		}
		row, err := cryptDB.GetWorkspaceBuildProvisionerStateByID(ctx, buildID)
		if err != nil {
			return xerrors.Errorf("get provisioner state for build %s: %w", buildID, err)
		}
		if err := cryptDB.UpdateWorkspaceBuildProvisionerStateByID(ctx, database.UpdateWorkspaceBuildProvisionerStateByIDParams{
			ID:                    buildID,
			UpdatedAt:             dbtime.Now(),
			ProvisionerState:      row.ProvisionerState,
			ProvisionerStateKeyID: sql.NullString{}, // primaryCipherDigest is empty, so dbcrypt writes this value as plaintext.
		}); err != nil {
			return xerrors.Errorf("decrypt provisioner state for build %s: %w", buildID, err)
		}
		log.Debug(ctx, "decrypted workspace build provisioner state", slog.F("build_id", buildID))
		return nil
	})
	if err != nil {
		return err
	}

	log.Info(ctx, "decrypting cached template version terraform plans")
	err = forEachSecretRow(ctx, sqlDB, terraformValuesCachedPlanScan, func(templateVersionID uuid.UUID, keyID sql.NullString) error {
		if !keyID.Valid {
			log.Debug(ctx, "skipping cached plan", slog.F("template_version_id", templateVersionID))
			return nil
		}
		row, err := cryptDB.GetTemplateVersionTerraformValues(ctx, templateVersionID)
		if err != nil {
			return xerrors.Errorf("get cached plan for template version %s: %w", templateVersionID, err)
		}
		if err := cryptDB.UpdateTemplateVersionTerraformValuesCachedPlan(ctx, database.UpdateTemplateVersionTerraformValuesCachedPlanParams{
			TemplateVersionID: templateVersionID,
			CachedPlan:        row.CachedPlan,
			CachedPlanKeyID:   sql.NullString{}, // primaryCipherDigest is empty, so dbcrypt writes this value as plaintext.
			UpdatedAt:         row.UpdatedAt,
		}); err != nil {
			return xerrors.Errorf("decrypt cached plan for template version %s: %w", templateVersionID, err)
		}
		log.Debug(ctx, "decrypted cached plan", slog.F("template_version_id", templateVersionID))
		return nil
	})
	if err != nil {
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

// secretRowPageSize is the page size used when iterating rows of tables whose
// secret columns are excluded from the store's hot paths.
const secretRowPageSize = 100

// secretRowScan describes a table whose secret-bearing rows are listed
// through raw SQL for `coder server dbcrypt` maintenance sweeps. The actual
// secret reads and writes still go through the dbcrypt-wrapped store.
type secretRowScan struct {
	table             string
	idColumn          string
	keyIDColumn       string
	nonEmptyPredicate string
}

var workspaceBuildStateScan = secretRowScan{
	table:             "workspace_builds",
	idColumn:          "id",
	keyIDColumn:       "provisioner_state_key_id",
	nonEmptyPredicate: "provisioner_state IS NOT NULL AND provisioner_state != ''::bytea",
}

var terraformValuesCachedPlanScan = secretRowScan{
	table:             "template_version_terraform_values",
	idColumn:          "template_version_id",
	keyIDColumn:       "cached_plan_key_id",
	nonEmptyPredicate: "cached_plan IS NOT NULL AND cached_plan != 'null'::jsonb AND cached_plan != '{}'::jsonb",
}

// forEachSecretRow calls fn for every row of the scan's table that holds a
// non-empty secret value, one ID-ordered page at a time.
func forEachSecretRow(ctx context.Context, sqlDB *sql.DB, scan secretRowScan, fn func(id uuid.UUID, keyID sql.NullString) error) error {
	type rowRef struct {
		id    uuid.UUID
		keyID sql.NullString
	}
	//nolint:gosec // Identifiers and predicates are compile-time constants; only values are parameterized.
	query := fmt.Sprintf(`
		SELECT %s, %s
		FROM %s
		WHERE %s
			AND %s > $1
		ORDER BY %s
		LIMIT $2`,
		scan.idColumn, scan.keyIDColumn, scan.table, scan.nonEmptyPredicate, scan.idColumn, scan.idColumn,
	)
	afterID := uuid.Nil
	for {
		rows, err := sqlDB.QueryContext(ctx, query, afterID, secretRowPageSize)
		if err != nil {
			return xerrors.Errorf("list %s rows after id=%s: %w", scan.table, afterID, err)
		}
		// Collect the page before calling fn so the connection is free for
		// fn's own queries.
		var page []rowRef
		for rows.Next() {
			var ref rowRef
			if err := rows.Scan(&ref.id, &ref.keyID); err != nil {
				_ = rows.Close()
				return xerrors.Errorf("scan %s row: %w", scan.table, err)
			}
			page = append(page, ref)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return xerrors.Errorf("iterate %s rows: %w", scan.table, err)
		}
		_ = rows.Close()

		for _, ref := range page {
			if err := fn(ref.id, ref.keyID); err != nil {
				return err
			}
		}
		if len(page) < secretRowPageSize {
			return nil
		}
		afterID = page[len(page)-1].id
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

// AuditResult summarizes one dbcrypt-managed column and the number of rows
// whose value is stored in plaintext (non-empty value with a NULL key ID).
type AuditResult struct {
	Table            string
	Column           string
	UnencryptedCount int64
}

// auditTarget describes a column that dbcrypt encrypts at rest. emptyValues
// are plaintext values that carry no secret (and are therefore never
// encrypted), such as the empty string. nonEmptyPredicate, when set, replaces
// the default text-column emptiness check for columns of other types.
type auditTarget struct {
	table             string
	column            string
	keyIDColumn       string
	emptyValues       []string
	nonEmptyPredicate string
}

// auditTargets lists every column dbcrypt encrypts. Extend this list whenever
// a column gains a *_key_id companion so `coder server dbcrypt audit` keeps
// flagging rows written by code paths that bypass encryption (SEC-1491).
var auditTargets = []auditTarget{
	{table: "user_links", column: "oauth_access_token", keyIDColumn: "oauth_access_token_key_id"},
	{table: "user_links", column: "oauth_refresh_token", keyIDColumn: "oauth_refresh_token_key_id"},
	{table: "external_auth_links", column: "oauth_access_token", keyIDColumn: "oauth_access_token_key_id"},
	{table: "external_auth_links", column: "oauth_refresh_token", keyIDColumn: "oauth_refresh_token_key_id"},
	{table: "external_auth_links", column: "oauth_extra", keyIDColumn: "oauth_extra_key_id", nonEmptyPredicate: "oauth_extra IS NOT NULL AND oauth_extra != 'null'::jsonb AND oauth_extra != '{}'::jsonb"},
	{table: "gitsshkeys", column: "private_key", keyIDColumn: "private_key_key_id"},
	{table: "workspace_builds", column: "provisioner_state", keyIDColumn: "provisioner_state_key_id", nonEmptyPredicate: "provisioner_state IS NOT NULL AND provisioner_state != ''::bytea"},
	{table: "template_version_terraform_values", column: "cached_plan", keyIDColumn: "cached_plan_key_id", nonEmptyPredicate: "cached_plan IS NOT NULL AND cached_plan != 'null'::jsonb AND cached_plan != '{}'::jsonb"},
	{table: "user_secrets", column: "value", keyIDColumn: "value_key_id"},
	{table: "workspace_secrets", column: "value", keyIDColumn: "value_key_id"},
	{table: "crypto_keys", column: "secret", keyIDColumn: "secret_key_id"},
	{table: "ai_providers", column: "settings", keyIDColumn: "settings_key_id"},
	{table: "ai_provider_keys", column: "api_key", keyIDColumn: "api_key_key_id"},
	{table: "user_ai_provider_keys", column: "api_key", keyIDColumn: "api_key_key_id"},
	{table: "mcp_server_configs", column: "oauth2_client_secret", keyIDColumn: "oauth2_client_secret_key_id"},
	{table: "mcp_server_configs", column: "api_key_value", keyIDColumn: "api_key_value_key_id"},
	{table: "mcp_server_configs", column: "custom_headers", keyIDColumn: "custom_headers_key_id", emptyValues: []string{"{}"}},
	{table: "mcp_server_configs", column: "signing_secret", keyIDColumn: "signing_secret_key_id"},
	{table: "mcp_server_user_tokens", column: "access_token", keyIDColumn: "access_token_key_id"},
	{table: "mcp_server_user_tokens", column: "refresh_token", keyIDColumn: "refresh_token_key_id"},
	{table: "chat_mcp_servers", column: "headers", keyIDColumn: "headers_key_id", emptyValues: []string{"{}"}},
}

// Audit reports, for every dbcrypt-managed column, how many rows hold a
// non-empty value with a NULL key ID, meaning the value is stored in
// plaintext. This catches rows written by code paths that bypass dbcrypt as
// well as rows that predate enabling encryption. It performs reads only and
// needs no encryption keys.
func Audit(ctx context.Context, sqlDB *sql.DB) ([]AuditResult, error) {
	results := make([]AuditResult, 0, len(auditTargets))
	for _, target := range auditTargets {
		var (
			predicate string
			args      []any
		)
		if target.nonEmptyPredicate != "" {
			predicate = target.nonEmptyPredicate
		} else {
			empties := append([]string{""}, target.emptyValues...)
			placeholders := make([]string, 0, len(empties))
			for i, v := range empties {
				placeholders = append(placeholders, fmt.Sprintf("$%d", i+1))
				args = append(args, v)
			}
			predicate = fmt.Sprintf(
				"%s IS NOT NULL AND %s NOT IN (%s)",
				target.column, target.column, strings.Join(placeholders, ", "),
			)
		}
		//nolint:gosec // The table and column names and predicates are compile-time constants from auditTargets; only the empty values are parameterized.
		query := fmt.Sprintf(
			"SELECT COUNT(*) FROM %s WHERE %s IS NULL AND %s",
			target.table, target.keyIDColumn, predicate,
		)
		var count int64
		if err := sqlDB.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
			return nil, xerrors.Errorf("count unencrypted rows in %s.%s: %w", target.table, target.column, err)
		}
		results = append(results, AuditResult{
			Table:            target.table,
			Column:           target.column,
			UnencryptedCount: count,
		})
	}
	return results, nil
}
