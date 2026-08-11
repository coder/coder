package dbcrypt

import (
	"context"
	"database/sql"
	"encoding/base64"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
)

// testValue is the value that is stored in dbcrypt_keys.test.
// This is used to determine if the key is valid.
const testValue = "coder"

var (
	b64encode = base64.StdEncoding.EncodeToString
	b64decode = base64.StdEncoding.DecodeString
)

// DecryptFailedError is returned when decryption fails.
type DecryptFailedError struct {
	Inner error
}

func (e *DecryptFailedError) Error() string {
	return xerrors.Errorf("decrypt failed: %w", e.Inner).Error()
}

// New creates a database.Store wrapper that encrypts/decrypts values
// stored at rest in the database.
func New(ctx context.Context, db database.Store, ciphers ...Cipher) (database.Store, error) {
	cm := make(map[string]Cipher)
	for _, c := range ciphers {
		cm[c.HexDigest()] = c
	}
	dbc := &dbCrypt{
		ciphers: cm,
		Store:   db,
	}
	if len(ciphers) > 0 {
		dbc.primaryCipherDigest = ciphers[0].HexDigest()
	}
	// nolint: gocritic // This is allowed.
	authCtx := dbauthz.AsSystemRestricted(ctx)
	if err := dbc.ensureEncryptedWithRetry(authCtx); err != nil {
		return nil, xerrors.Errorf("ensure encrypted database fields: %w", err)
	}
	return dbc, nil
}

type dbCrypt struct {
	// primaryCipherDigest is the digest of the primary cipher used for encrypting data.
	primaryCipherDigest string
	// ciphers is a map of cipher digests to ciphers.
	ciphers map[string]Cipher
	database.Store
}

func (db *dbCrypt) InTx(function func(database.Store) error, txOpts *database.TxOptions) error {
	return db.Store.InTx(func(s database.Store) error {
		return function(&dbCrypt{
			primaryCipherDigest: db.primaryCipherDigest,
			ciphers:             db.ciphers,
			Store:               s,
		})
	}, txOpts)
}

func (db *dbCrypt) GetDBCryptKeys(ctx context.Context) ([]database.DBCryptKey, error) {
	ks, err := db.Store.GetDBCryptKeys(ctx)
	if err != nil {
		return nil, err
	}
	// Decrypt the test field to ensure that the key is valid.
	for i := range ks {
		if !ks[i].ActiveKeyDigest.Valid {
			// Key has been revoked. We can't decrypt the test field, but
			// we need to return it so that the caller knows that the key
			// has been revoked.
			continue
		}
		if err := db.decryptField(&ks[i].Test, ks[i].ActiveKeyDigest); err != nil {
			return nil, err
		}
	}
	return ks, nil
}

// This does not need any special handling as it does not touch any encrypted fields.
// Explicitly defining this here to avoid confusion.
func (db *dbCrypt) RevokeDBCryptKey(ctx context.Context, activeKeyDigest string) error {
	return db.Store.RevokeDBCryptKey(ctx, activeKeyDigest)
}

func (db *dbCrypt) InsertDBCryptKey(ctx context.Context, arg database.InsertDBCryptKeyParams) error {
	// It's nicer to be able to pass a *sql.NullString to encryptField, but we need to pass a string here.
	var digest sql.NullString
	err := db.encryptField(&arg.Test, &digest)
	if err != nil {
		return err
	}
	arg.ActiveKeyDigest = digest.String
	return db.Store.InsertDBCryptKey(ctx, arg)
}

func (db *dbCrypt) GetUserLinkByLinkedID(ctx context.Context, linkedID string) (database.UserLink, error) {
	link, err := db.Store.GetUserLinkByLinkedID(ctx, linkedID)
	if err != nil {
		return database.UserLink{}, err
	}
	if err := db.decryptField(&link.OAuthAccessToken, link.OAuthAccessTokenKeyID); err != nil {
		return database.UserLink{}, err
	}
	if err := db.decryptField(&link.OAuthRefreshToken, link.OAuthRefreshTokenKeyID); err != nil {
		return database.UserLink{}, err
	}
	return link, nil
}

func (db *dbCrypt) GetUserLinksByUserID(ctx context.Context, userID uuid.UUID) ([]database.UserLink, error) {
	links, err := db.Store.GetUserLinksByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	for idx := range links {
		if err := db.decryptField(&links[idx].OAuthAccessToken, links[idx].OAuthAccessTokenKeyID); err != nil {
			return nil, err
		}
		if err := db.decryptField(&links[idx].OAuthRefreshToken, links[idx].OAuthRefreshTokenKeyID); err != nil {
			return nil, err
		}
	}
	return links, nil
}

func (db *dbCrypt) GetUserLinkByUserIDLoginType(ctx context.Context, params database.GetUserLinkByUserIDLoginTypeParams) (database.UserLink, error) {
	link, err := db.Store.GetUserLinkByUserIDLoginType(ctx, params)
	if err != nil {
		return database.UserLink{}, err
	}
	if err := db.decryptField(&link.OAuthAccessToken, link.OAuthAccessTokenKeyID); err != nil {
		return database.UserLink{}, err
	}
	if err := db.decryptField(&link.OAuthRefreshToken, link.OAuthRefreshTokenKeyID); err != nil {
		return database.UserLink{}, err
	}
	return link, nil
}

func (db *dbCrypt) InsertUserLink(ctx context.Context, params database.InsertUserLinkParams) (database.UserLink, error) {
	if err := db.encryptField(&params.OAuthAccessToken, &params.OAuthAccessTokenKeyID); err != nil {
		return database.UserLink{}, err
	}
	if err := db.encryptField(&params.OAuthRefreshToken, &params.OAuthRefreshTokenKeyID); err != nil {
		return database.UserLink{}, err
	}
	link, err := db.Store.InsertUserLink(ctx, params)
	if err != nil {
		return database.UserLink{}, err
	}
	if err := db.decryptField(&link.OAuthAccessToken, link.OAuthAccessTokenKeyID); err != nil {
		return database.UserLink{}, err
	}
	if err := db.decryptField(&link.OAuthRefreshToken, link.OAuthRefreshTokenKeyID); err != nil {
		return database.UserLink{}, err
	}
	return link, nil
}

func (db *dbCrypt) UpdateUserLink(ctx context.Context, params database.UpdateUserLinkParams) (database.UserLink, error) {
	if err := db.encryptField(&params.OAuthAccessToken, &params.OAuthAccessTokenKeyID); err != nil {
		return database.UserLink{}, err
	}
	if err := db.encryptField(&params.OAuthRefreshToken, &params.OAuthRefreshTokenKeyID); err != nil {
		return database.UserLink{}, err
	}
	link, err := db.Store.UpdateUserLink(ctx, params)
	if err != nil {
		return database.UserLink{}, err
	}
	if err := db.decryptField(&link.OAuthAccessToken, link.OAuthAccessTokenKeyID); err != nil {
		return database.UserLink{}, err
	}
	if err := db.decryptField(&link.OAuthRefreshToken, link.OAuthRefreshTokenKeyID); err != nil {
		return database.UserLink{}, err
	}
	return link, nil
}

func (db *dbCrypt) InsertExternalAuthLink(ctx context.Context, params database.InsertExternalAuthLinkParams) (database.ExternalAuthLink, error) {
	if err := db.encryptField(&params.OAuthAccessToken, &params.OAuthAccessTokenKeyID); err != nil {
		return database.ExternalAuthLink{}, err
	}
	if err := db.encryptField(&params.OAuthRefreshToken, &params.OAuthRefreshTokenKeyID); err != nil {
		return database.ExternalAuthLink{}, err
	}
	link, err := db.Store.InsertExternalAuthLink(ctx, params)
	if err != nil {
		return database.ExternalAuthLink{}, err
	}
	if err := db.decryptField(&link.OAuthAccessToken, link.OAuthAccessTokenKeyID); err != nil {
		return database.ExternalAuthLink{}, err
	}
	if err := db.decryptField(&link.OAuthRefreshToken, link.OAuthRefreshTokenKeyID); err != nil {
		return database.ExternalAuthLink{}, err
	}
	return link, nil
}

func (db *dbCrypt) GetExternalAuthLink(ctx context.Context, params database.GetExternalAuthLinkParams) (database.ExternalAuthLink, error) {
	link, err := db.Store.GetExternalAuthLink(ctx, params)
	if err != nil {
		return database.ExternalAuthLink{}, err
	}
	if err := db.decryptField(&link.OAuthAccessToken, link.OAuthAccessTokenKeyID); err != nil {
		return database.ExternalAuthLink{}, err
	}
	if err := db.decryptField(&link.OAuthRefreshToken, link.OAuthRefreshTokenKeyID); err != nil {
		return database.ExternalAuthLink{}, err
	}
	return link, nil
}

func (db *dbCrypt) GetExternalAuthLinksByUserID(ctx context.Context, userID uuid.UUID) ([]database.ExternalAuthLink, error) {
	links, err := db.Store.GetExternalAuthLinksByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	for idx := range links {
		if err := db.decryptField(&links[idx].OAuthAccessToken, links[idx].OAuthAccessTokenKeyID); err != nil {
			return nil, err
		}
		if err := db.decryptField(&links[idx].OAuthRefreshToken, links[idx].OAuthRefreshTokenKeyID); err != nil {
			return nil, err
		}
	}
	return links, nil
}

func (db *dbCrypt) UpdateExternalAuthLink(ctx context.Context, params database.UpdateExternalAuthLinkParams) (database.ExternalAuthLink, error) {
	if err := db.encryptField(&params.OAuthAccessToken, &params.OAuthAccessTokenKeyID); err != nil {
		return database.ExternalAuthLink{}, err
	}
	if err := db.encryptField(&params.OAuthRefreshToken, &params.OAuthRefreshTokenKeyID); err != nil {
		return database.ExternalAuthLink{}, err
	}
	link, err := db.Store.UpdateExternalAuthLink(ctx, params)
	if err != nil {
		return database.ExternalAuthLink{}, err
	}
	if err := db.decryptField(&link.OAuthAccessToken, link.OAuthAccessTokenKeyID); err != nil {
		return database.ExternalAuthLink{}, err
	}
	if err := db.decryptField(&link.OAuthRefreshToken, link.OAuthRefreshTokenKeyID); err != nil {
		return database.ExternalAuthLink{}, err
	}
	return link, nil
}

func (db *dbCrypt) UpdateExternalAuthLinkRefreshToken(ctx context.Context, params database.UpdateExternalAuthLinkRefreshTokenParams) error {
	// The SQL query uses an optimistic lock:
	//   WHERE oauth_refresh_token = @old_oauth_refresh_token
	// The caller supplies the plaintext old token (since dbcrypt
	// decrypts on read), but the DB stores the encrypted value.
	// Because AES-GCM is non-deterministic, we cannot simply
	// re-encrypt the old token — the ciphertext would differ.
	// Instead, read the current row from the inner (raw) store
	// and use the actual encrypted value for the WHERE clause.
	if params.OldOauthRefreshToken != "" && db.ciphers != nil && db.primaryCipherDigest != "" {
		raw, err := db.Store.GetExternalAuthLink(ctx, database.GetExternalAuthLinkParams{
			ProviderID: params.ProviderID,
			UserID:     params.UserID,
		})
		if err != nil {
			return err
		}
		// Decrypt the stored token so we can compare with the
		// caller-supplied plaintext.
		decrypted := raw.OAuthRefreshToken
		if err := db.decryptField(&decrypted, raw.OAuthRefreshTokenKeyID); err != nil {
			return err
		}
		if decrypted != params.OldOauthRefreshToken {
			// The token has changed since the caller read it;
			// the optimistic lock should fail (no rows updated).
			// Return nil to match the :exec semantics of the SQL
			// query, which silently updates zero rows.
			return nil
		}
		// Use the raw encrypted value so the WHERE clause matches.
		params.OldOauthRefreshToken = raw.OAuthRefreshToken
	}

	// We would normally use a sql.NullString here, but sqlc does not want to make
	// a params struct with a nullable string.
	var digest sql.NullString
	if params.OAuthRefreshTokenKeyID != "" {
		digest.String = params.OAuthRefreshTokenKeyID
		digest.Valid = true
	}
	if err := db.encryptField(&params.OAuthRefreshToken, &digest); err != nil {
		return err
	}

	return db.Store.UpdateExternalAuthLinkRefreshToken(ctx, params)
}

func (db *dbCrypt) GetCryptoKeys(ctx context.Context) ([]database.CryptoKey, error) {
	keys, err := db.Store.GetCryptoKeys(ctx)
	if err != nil {
		return nil, err
	}
	for i := range keys {
		if err := db.decryptField(&keys[i].Secret.String, keys[i].SecretKeyID); err != nil {
			return nil, err
		}
	}
	return keys, nil
}

func (db *dbCrypt) GetLatestCryptoKeyByFeature(ctx context.Context, feature database.CryptoKeyFeature) (database.CryptoKey, error) {
	key, err := db.Store.GetLatestCryptoKeyByFeature(ctx, feature)
	if err != nil {
		return database.CryptoKey{}, err
	}
	if err := db.decryptField(&key.Secret.String, key.SecretKeyID); err != nil {
		return database.CryptoKey{}, err
	}
	return key, nil
}

func (db *dbCrypt) GetCryptoKeyByFeatureAndSequence(ctx context.Context, params database.GetCryptoKeyByFeatureAndSequenceParams) (database.CryptoKey, error) {
	key, err := db.Store.GetCryptoKeyByFeatureAndSequence(ctx, params)
	if err != nil {
		return database.CryptoKey{}, err
	}
	if err := db.decryptField(&key.Secret.String, key.SecretKeyID); err != nil {
		return database.CryptoKey{}, err
	}
	return key, nil
}

func (db *dbCrypt) InsertCryptoKey(ctx context.Context, params database.InsertCryptoKeyParams) (database.CryptoKey, error) {
	if err := db.encryptField(&params.Secret.String, &params.SecretKeyID); err != nil {
		return database.CryptoKey{}, err
	}
	key, err := db.Store.InsertCryptoKey(ctx, params)
	if err != nil {
		return database.CryptoKey{}, err
	}
	if err := db.decryptField(&key.Secret.String, key.SecretKeyID); err != nil {
		return database.CryptoKey{}, err
	}
	return key, nil
}

func (db *dbCrypt) UpdateCryptoKeyDeletesAt(ctx context.Context, arg database.UpdateCryptoKeyDeletesAtParams) (database.CryptoKey, error) {
	key, err := db.Store.UpdateCryptoKeyDeletesAt(ctx, arg)
	if err != nil {
		return database.CryptoKey{}, err
	}
	if err := db.decryptField(&key.Secret.String, key.SecretKeyID); err != nil {
		return database.CryptoKey{}, err
	}
	return key, nil
}

func (db *dbCrypt) GetCryptoKeysByFeature(ctx context.Context, feature database.CryptoKeyFeature) ([]database.CryptoKey, error) {
	keys, err := db.Store.GetCryptoKeysByFeature(ctx, feature)
	if err != nil {
		return nil, err
	}

	for i := range keys {
		if err := db.decryptField(&keys[i].Secret.String, keys[i].SecretKeyID); err != nil {
			return nil, err
		}
	}

	return keys, nil
}

// decryptAIProvider decrypts the secret fields of an AI provider row.
func (db *dbCrypt) decryptAIProvider(p *database.AIProvider) error {
	if !p.Settings.Valid {
		return nil
	}
	return db.decryptField(&p.Settings.String, p.SettingsKeyID)
}

// decryptAIProviderKey decrypts the api_key field of an AI provider key row.
func (db *dbCrypt) decryptAIProviderKey(k *database.AIProviderKey) error {
	return db.decryptField(&k.APIKey, k.ApiKeyKeyID)
}

// encryptAIProviderSettings encrypts the settings column in place,
// updating settings_key_id as a side effect. A NULL or blank settings
// value clears any associated key reference.
func (db *dbCrypt) encryptAIProviderSettings(settings *sql.NullString, keyID *sql.NullString) error {
	if !settings.Valid || strings.TrimSpace(settings.String) == "" {
		*settings = sql.NullString{}
		*keyID = sql.NullString{}
		return nil
	}
	return db.encryptField(&settings.String, keyID)
}

func (db *dbCrypt) GetAIProviderByID(ctx context.Context, id uuid.UUID) (database.AIProvider, error) {
	provider, err := db.Store.GetAIProviderByID(ctx, id)
	if err != nil {
		return database.AIProvider{}, err
	}
	if err := db.decryptAIProvider(&provider); err != nil {
		return database.AIProvider{}, err
	}
	return provider, nil
}

func (db *dbCrypt) GetAIProviderByName(ctx context.Context, name string) (database.AIProvider, error) {
	provider, err := db.Store.GetAIProviderByName(ctx, name)
	if err != nil {
		return database.AIProvider{}, err
	}
	if err := db.decryptAIProvider(&provider); err != nil {
		return database.AIProvider{}, err
	}
	return provider, nil
}

// GetAIProviders returns AI provider rows, with their settings
// decrypted, honoring the include_deleted and include_disabled flags
// from the underlying query.
func (db *dbCrypt) GetAIProviders(ctx context.Context, arg database.GetAIProvidersParams) ([]database.AIProvider, error) {
	providers, err := db.Store.GetAIProviders(ctx, arg)
	if err != nil {
		return nil, err
	}
	for i := range providers {
		if err := db.decryptAIProvider(&providers[i]); err != nil {
			return nil, err
		}
	}
	return providers, nil
}

func (db *dbCrypt) InsertAIProvider(ctx context.Context, params database.InsertAIProviderParams) (database.AIProvider, error) {
	if err := db.encryptAIProviderSettings(&params.Settings, &params.SettingsKeyID); err != nil {
		return database.AIProvider{}, err
	}

	provider, err := db.Store.InsertAIProvider(ctx, params)
	if err != nil {
		return database.AIProvider{}, err
	}
	if err := db.decryptAIProvider(&provider); err != nil {
		return database.AIProvider{}, err
	}
	return provider, nil
}

func (db *dbCrypt) UpdateAIProvider(ctx context.Context, params database.UpdateAIProviderParams) (database.AIProvider, error) {
	if err := db.encryptAIProviderSettings(&params.Settings, &params.SettingsKeyID); err != nil {
		return database.AIProvider{}, err
	}

	provider, err := db.Store.UpdateAIProvider(ctx, params)
	if err != nil {
		return database.AIProvider{}, err
	}
	if err := db.decryptAIProvider(&provider); err != nil {
		return database.AIProvider{}, err
	}
	return provider, nil
}

// UpdateEncryptedAIProviderSettings re-encrypts the settings column
// of a row, regardless of its deleted flag, so that dbcrypt key
// rotation can move every FK reference to a new key digest before
// old keys are revoked.
func (db *dbCrypt) UpdateEncryptedAIProviderSettings(ctx context.Context, params database.UpdateEncryptedAIProviderSettingsParams) (database.AIProvider, error) {
	if err := db.encryptAIProviderSettings(&params.Settings, &params.SettingsKeyID); err != nil {
		return database.AIProvider{}, err
	}

	provider, err := db.Store.UpdateEncryptedAIProviderSettings(ctx, params)
	if err != nil {
		return database.AIProvider{}, err
	}
	if err := db.decryptAIProvider(&provider); err != nil {
		return database.AIProvider{}, err
	}
	return provider, nil
}

func (db *dbCrypt) GetAIProviderKeyByID(ctx context.Context, id uuid.UUID) (database.AIProviderKey, error) {
	key, err := db.Store.GetAIProviderKeyByID(ctx, id)
	if err != nil {
		return database.AIProviderKey{}, err
	}
	if err := db.decryptAIProviderKey(&key); err != nil {
		return database.AIProviderKey{}, err
	}
	return key, nil
}

func (db *dbCrypt) GetAIProviderKeysByProviderID(ctx context.Context, providerID uuid.UUID) ([]database.AIProviderKey, error) {
	keys, err := db.Store.GetAIProviderKeysByProviderID(ctx, providerID)
	if err != nil {
		return nil, err
	}
	for i := range keys {
		if err := db.decryptAIProviderKey(&keys[i]); err != nil {
			return nil, err
		}
	}
	return keys, nil
}

func (db *dbCrypt) GetAIProviderKeysByProviderIDs(ctx context.Context, providerIDs []uuid.UUID) ([]database.AIProviderKey, error) {
	keys, err := db.Store.GetAIProviderKeysByProviderIDs(ctx, providerIDs)
	if err != nil {
		return nil, err
	}
	for i := range keys {
		if err := db.decryptAIProviderKey(&keys[i]); err != nil {
			return nil, err
		}
	}
	return keys, nil
}

func (db *dbCrypt) InsertAIProviderKey(ctx context.Context, params database.InsertAIProviderKeyParams) (database.AIProviderKey, error) {
	if strings.TrimSpace(params.APIKey) == "" {
		params.ApiKeyKeyID = sql.NullString{}
	} else if err := db.encryptField(&params.APIKey, &params.ApiKeyKeyID); err != nil {
		return database.AIProviderKey{}, err
	}

	key, err := db.Store.InsertAIProviderKey(ctx, params)
	if err != nil {
		return database.AIProviderKey{}, err
	}
	if err := db.decryptAIProviderKey(&key); err != nil {
		return database.AIProviderKey{}, err
	}
	return key, nil
}

// GetAIProviderKeys returns AI provider key rows with their api_key
// decrypted. The list handler relies on the default scope (live
// providers only); the dbcrypt key rotation utility calls with
// includeDeleted=TRUE so it can walk every row holding a foreign-key
// reference to dbcrypt_keys before old keys are revoked.
func (db *dbCrypt) GetAIProviderKeys(ctx context.Context, includeDeleted bool) ([]database.AIProviderKey, error) {
	keys, err := db.Store.GetAIProviderKeys(ctx, includeDeleted)
	if err != nil {
		return nil, err
	}
	for i := range keys {
		if err := db.decryptAIProviderKey(&keys[i]); err != nil {
			return nil, err
		}
	}
	return keys, nil
}

// UpdateEncryptedAIProviderKey re-encrypts the api_key column of a
// key row, so that dbcrypt key rotation can move every FK reference
// to a new key digest before old keys are revoked.
func (db *dbCrypt) UpdateEncryptedAIProviderKey(ctx context.Context, params database.UpdateEncryptedAIProviderKeyParams) (database.AIProviderKey, error) {
	if strings.TrimSpace(params.APIKey) == "" {
		params.ApiKeyKeyID = sql.NullString{}
	} else if err := db.encryptField(&params.APIKey, &params.ApiKeyKeyID); err != nil {
		return database.AIProviderKey{}, err
	}

	key, err := db.Store.UpdateEncryptedAIProviderKey(ctx, params)
	if err != nil {
		return database.AIProviderKey{}, err
	}
	if err := db.decryptAIProviderKey(&key); err != nil {
		return database.AIProviderKey{}, err
	}
	return key, nil
}

func (db *dbCrypt) decryptUserAIProviderKey(key *database.UserAIProviderKey) error {
	return db.decryptField(&key.APIKey, key.ApiKeyKeyID)
}

func (db *dbCrypt) GetUserAIProviderKeyByProviderID(ctx context.Context, params database.GetUserAIProviderKeyByProviderIDParams) (database.UserAIProviderKey, error) {
	key, err := db.Store.GetUserAIProviderKeyByProviderID(ctx, params)
	if err != nil {
		return database.UserAIProviderKey{}, err
	}
	if err := db.decryptUserAIProviderKey(&key); err != nil {
		return database.UserAIProviderKey{}, err
	}
	return key, nil
}

func (db *dbCrypt) GetUserAIProviderKeysByUserID(ctx context.Context, userID uuid.UUID) ([]database.UserAIProviderKey, error) {
	keys, err := db.Store.GetUserAIProviderKeysByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	for i := range keys {
		if err := db.decryptUserAIProviderKey(&keys[i]); err != nil {
			return nil, err
		}
	}
	return keys, nil
}

func (db *dbCrypt) GetUserAIProviderKeys(ctx context.Context) ([]database.UserAIProviderKey, error) {
	keys, err := db.Store.GetUserAIProviderKeys(ctx)
	if err != nil {
		return nil, err
	}
	for i := range keys {
		if err := db.decryptUserAIProviderKey(&keys[i]); err != nil {
			return nil, err
		}
	}
	return keys, nil
}

func (db *dbCrypt) UpsertUserAIProviderKey(ctx context.Context, params database.UpsertUserAIProviderKeyParams) (database.UserAIProviderKey, error) {
	if strings.TrimSpace(params.APIKey) == "" {
		params.ApiKeyKeyID = sql.NullString{}
	} else if err := db.encryptField(&params.APIKey, &params.ApiKeyKeyID); err != nil {
		return database.UserAIProviderKey{}, err
	}

	key, err := db.Store.UpsertUserAIProviderKey(ctx, params)
	if err != nil {
		return database.UserAIProviderKey{}, err
	}
	if err := db.decryptUserAIProviderKey(&key); err != nil {
		return database.UserAIProviderKey{}, err
	}
	return key, nil
}

func (db *dbCrypt) UpdateUserAIProviderKey(ctx context.Context, params database.UpdateUserAIProviderKeyParams) (database.UserAIProviderKey, error) {
	if strings.TrimSpace(params.APIKey) == "" {
		params.ApiKeyKeyID = sql.NullString{}
	} else if err := db.encryptField(&params.APIKey, &params.ApiKeyKeyID); err != nil {
		return database.UserAIProviderKey{}, err
	}

	key, err := db.Store.UpdateUserAIProviderKey(ctx, params)
	if err != nil {
		return database.UserAIProviderKey{}, err
	}
	if err := db.decryptUserAIProviderKey(&key); err != nil {
		return database.UserAIProviderKey{}, err
	}
	return key, nil
}

func (db *dbCrypt) UpdateEncryptedUserAIProviderKey(ctx context.Context, params database.UpdateEncryptedUserAIProviderKeyParams) (database.UserAIProviderKey, error) {
	if strings.TrimSpace(params.APIKey) == "" {
		params.ApiKeyKeyID = sql.NullString{}
	} else if err := db.encryptField(&params.APIKey, &params.ApiKeyKeyID); err != nil {
		return database.UserAIProviderKey{}, err
	}

	key, err := db.Store.UpdateEncryptedUserAIProviderKey(ctx, params)
	if err != nil {
		return database.UserAIProviderKey{}, err
	}
	if err := db.decryptUserAIProviderKey(&key); err != nil {
		return database.UserAIProviderKey{}, err
	}
	return key, nil
}

// decryptMCPServerConfig decrypts all encrypted fields on a
// single MCPServerConfig in place.
func (db *dbCrypt) decryptMCPServerConfig(cfg *database.MCPServerConfig) error {
	if err := db.decryptField(&cfg.OAuth2ClientSecret, cfg.OAuth2ClientSecretKeyID); err != nil {
		return err
	}
	if err := db.decryptField(&cfg.APIKeyValue, cfg.APIKeyValueKeyID); err != nil {
		return err
	}
	return db.decryptField(&cfg.CustomHeaders, cfg.CustomHeadersKeyID)
}

// decryptMCPServerUserToken decrypts all encrypted fields on a
// single MCPServerUserToken in place.
func (db *dbCrypt) decryptMCPServerUserToken(tok *database.MCPServerUserToken) error {
	if err := db.decryptField(&tok.AccessToken, tok.AccessTokenKeyID); err != nil {
		return err
	}
	return db.decryptField(&tok.RefreshToken, tok.RefreshTokenKeyID)
}

func (db *dbCrypt) GetMCPServerConfigByID(ctx context.Context, id uuid.UUID) (database.MCPServerConfig, error) {
	cfg, err := db.Store.GetMCPServerConfigByID(ctx, id)
	if err != nil {
		return database.MCPServerConfig{}, err
	}
	if err := db.decryptMCPServerConfig(&cfg); err != nil {
		return database.MCPServerConfig{}, err
	}
	return cfg, nil
}

func (db *dbCrypt) GetMCPServerConfigBySlug(ctx context.Context, slug string) (database.MCPServerConfig, error) {
	cfg, err := db.Store.GetMCPServerConfigBySlug(ctx, slug)
	if err != nil {
		return database.MCPServerConfig{}, err
	}
	if err := db.decryptMCPServerConfig(&cfg); err != nil {
		return database.MCPServerConfig{}, err
	}
	return cfg, nil
}

func (db *dbCrypt) GetMCPServerConfigs(ctx context.Context) ([]database.MCPServerConfig, error) {
	cfgs, err := db.Store.GetMCPServerConfigs(ctx)
	if err != nil {
		return nil, err
	}
	for i := range cfgs {
		if err := db.decryptMCPServerConfig(&cfgs[i]); err != nil {
			return nil, err
		}
	}
	return cfgs, nil
}

func (db *dbCrypt) GetMCPServerConfigsByIDs(ctx context.Context, ids []uuid.UUID) ([]database.MCPServerConfig, error) {
	cfgs, err := db.Store.GetMCPServerConfigsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range cfgs {
		if err := db.decryptMCPServerConfig(&cfgs[i]); err != nil {
			return nil, err
		}
	}
	return cfgs, nil
}

func (db *dbCrypt) GetEnabledMCPServerConfigs(ctx context.Context) ([]database.MCPServerConfig, error) {
	cfgs, err := db.Store.GetEnabledMCPServerConfigs(ctx)
	if err != nil {
		return nil, err
	}
	for i := range cfgs {
		if err := db.decryptMCPServerConfig(&cfgs[i]); err != nil {
			return nil, err
		}
	}
	return cfgs, nil
}

func (db *dbCrypt) GetForcedMCPServerConfigs(ctx context.Context) ([]database.MCPServerConfig, error) {
	cfgs, err := db.Store.GetForcedMCPServerConfigs(ctx)
	if err != nil {
		return nil, err
	}
	for i := range cfgs {
		if err := db.decryptMCPServerConfig(&cfgs[i]); err != nil {
			return nil, err
		}
	}
	return cfgs, nil
}

func (db *dbCrypt) GetMCPServerUserToken(ctx context.Context, arg database.GetMCPServerUserTokenParams) (database.MCPServerUserToken, error) {
	tok, err := db.Store.GetMCPServerUserToken(ctx, arg)
	if err != nil {
		return database.MCPServerUserToken{}, err
	}
	if err := db.decryptMCPServerUserToken(&tok); err != nil {
		return database.MCPServerUserToken{}, err
	}
	return tok, nil
}

func (db *dbCrypt) MarkMCPServerUserTokenRefreshFailure(ctx context.Context, params database.MarkMCPServerUserTokenRefreshFailureParams) (database.MCPServerUserToken, error) {
	// The query clears the encrypted token fields, so nothing needs
	// encrypting; decrypt the returned row for consistency with the
	// other accessors (a no-op for the cleared fields).
	tok, err := db.Store.MarkMCPServerUserTokenRefreshFailure(ctx, params)
	if err != nil {
		return database.MCPServerUserToken{}, err
	}
	if err := db.decryptMCPServerUserToken(&tok); err != nil {
		return database.MCPServerUserToken{}, err
	}
	return tok, nil
}

func (db *dbCrypt) GetMCPServerUserTokensByUserID(ctx context.Context, userID uuid.UUID) ([]database.MCPServerUserToken, error) {
	toks, err := db.Store.GetMCPServerUserTokensByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	for i := range toks {
		if err := db.decryptMCPServerUserToken(&toks[i]); err != nil {
			return nil, err
		}
	}
	return toks, nil
}

func (db *dbCrypt) InsertMCPServerConfig(ctx context.Context, params database.InsertMCPServerConfigParams) (database.MCPServerConfig, error) {
	if strings.TrimSpace(params.OAuth2ClientSecret) == "" {
		params.OAuth2ClientSecretKeyID = sql.NullString{}
	} else if err := db.encryptField(&params.OAuth2ClientSecret, &params.OAuth2ClientSecretKeyID); err != nil {
		return database.MCPServerConfig{}, err
	}
	if strings.TrimSpace(params.APIKeyValue) == "" {
		params.APIKeyValueKeyID = sql.NullString{}
	} else if err := db.encryptField(&params.APIKeyValue, &params.APIKeyValueKeyID); err != nil {
		return database.MCPServerConfig{}, err
	}
	if strings.TrimSpace(params.CustomHeaders) == "" {
		params.CustomHeadersKeyID = sql.NullString{}
	} else if err := db.encryptField(&params.CustomHeaders, &params.CustomHeadersKeyID); err != nil {
		return database.MCPServerConfig{}, err
	}

	cfg, err := db.Store.InsertMCPServerConfig(ctx, params)
	if err != nil {
		return database.MCPServerConfig{}, err
	}
	if err := db.decryptMCPServerConfig(&cfg); err != nil {
		return database.MCPServerConfig{}, err
	}
	return cfg, nil
}

func (db *dbCrypt) UpdateMCPServerConfig(ctx context.Context, params database.UpdateMCPServerConfigParams) (database.MCPServerConfig, error) {
	if strings.TrimSpace(params.OAuth2ClientSecret) == "" {
		params.OAuth2ClientSecretKeyID = sql.NullString{}
	} else if err := db.encryptField(&params.OAuth2ClientSecret, &params.OAuth2ClientSecretKeyID); err != nil {
		return database.MCPServerConfig{}, err
	}
	if strings.TrimSpace(params.APIKeyValue) == "" {
		params.APIKeyValueKeyID = sql.NullString{}
	} else if err := db.encryptField(&params.APIKeyValue, &params.APIKeyValueKeyID); err != nil {
		return database.MCPServerConfig{}, err
	}
	if strings.TrimSpace(params.CustomHeaders) == "" {
		params.CustomHeadersKeyID = sql.NullString{}
	} else if err := db.encryptField(&params.CustomHeaders, &params.CustomHeadersKeyID); err != nil {
		return database.MCPServerConfig{}, err
	}

	cfg, err := db.Store.UpdateMCPServerConfig(ctx, params)
	if err != nil {
		return database.MCPServerConfig{}, err
	}
	if err := db.decryptMCPServerConfig(&cfg); err != nil {
		return database.MCPServerConfig{}, err
	}
	return cfg, nil
}

func (db *dbCrypt) UpsertMCPServerUserToken(ctx context.Context, params database.UpsertMCPServerUserTokenParams) (database.MCPServerUserToken, error) {
	if strings.TrimSpace(params.AccessToken) == "" {
		params.AccessTokenKeyID = sql.NullString{}
	} else if err := db.encryptField(&params.AccessToken, &params.AccessTokenKeyID); err != nil {
		return database.MCPServerUserToken{}, err
	}
	if strings.TrimSpace(params.RefreshToken) == "" {
		params.RefreshTokenKeyID = sql.NullString{}
	} else if err := db.encryptField(&params.RefreshToken, &params.RefreshTokenKeyID); err != nil {
		return database.MCPServerUserToken{}, err
	}

	tok, err := db.Store.UpsertMCPServerUserToken(ctx, params)
	if err != nil {
		return database.MCPServerUserToken{}, err
	}
	if err := db.decryptMCPServerUserToken(&tok); err != nil {
		return database.MCPServerUserToken{}, err
	}
	return tok, nil
}

func (db *dbCrypt) UpdateMCPServerUserTokenFromRefresh(ctx context.Context, params database.UpdateMCPServerUserTokenFromRefreshParams) (database.MCPServerUserToken, error) {
	if strings.TrimSpace(params.AccessToken) == "" {
		params.AccessTokenKeyID = sql.NullString{}
	} else if err := db.encryptField(&params.AccessToken, &params.AccessTokenKeyID); err != nil {
		return database.MCPServerUserToken{}, err
	}
	if strings.TrimSpace(params.RefreshToken) == "" {
		params.RefreshTokenKeyID = sql.NullString{}
	} else if err := db.encryptField(&params.RefreshToken, &params.RefreshTokenKeyID); err != nil {
		return database.MCPServerUserToken{}, err
	}

	tok, err := db.Store.UpdateMCPServerUserTokenFromRefresh(ctx, params)
	if err != nil {
		return database.MCPServerUserToken{}, err
	}
	if err := db.decryptMCPServerUserToken(&tok); err != nil {
		return database.MCPServerUserToken{}, err
	}
	return tok, nil
}

func (db *dbCrypt) CreateUserSecret(ctx context.Context, params database.CreateUserSecretParams) (database.UserSecret, error) {
	if err := db.encryptField(&params.Value, &params.ValueKeyID); err != nil {
		return database.UserSecret{}, err
	}
	secret, err := db.Store.CreateUserSecret(ctx, params)
	if err != nil {
		return database.UserSecret{}, err
	}
	if err := db.decryptField(&secret.Value, secret.ValueKeyID); err != nil {
		return database.UserSecret{}, err
	}
	return secret, nil
}

func (db *dbCrypt) GetUserSecretByUserIDAndName(ctx context.Context, arg database.GetUserSecretByUserIDAndNameParams) (database.UserSecret, error) {
	secret, err := db.Store.GetUserSecretByUserIDAndName(ctx, arg)
	if err != nil {
		return database.UserSecret{}, err
	}
	if err := db.decryptField(&secret.Value, secret.ValueKeyID); err != nil {
		return database.UserSecret{}, err
	}
	return secret, nil
}

func (db *dbCrypt) ListUserSecretsWithValues(ctx context.Context, userID uuid.UUID) ([]database.UserSecret, error) {
	secrets, err := db.Store.ListUserSecretsWithValues(ctx, userID)
	if err != nil {
		return nil, err
	}
	for i := range secrets {
		if err := db.decryptField(&secrets[i].Value, secrets[i].ValueKeyID); err != nil {
			return nil, err
		}
	}
	return secrets, nil
}

func (db *dbCrypt) UpdateUserSecretByUserIDAndName(ctx context.Context, arg database.UpdateUserSecretByUserIDAndNameParams) (database.UserSecret, error) {
	if arg.UpdateValue {
		if err := db.encryptField(&arg.Value, &arg.ValueKeyID); err != nil {
			return database.UserSecret{}, err
		}
	}
	secret, err := db.Store.UpdateUserSecretByUserIDAndName(ctx, arg)
	if err != nil {
		return database.UserSecret{}, err
	}
	if err := db.decryptField(&secret.Value, secret.ValueKeyID); err != nil {
		return database.UserSecret{}, err
	}
	return secret, nil
}

func (db *dbCrypt) InsertGitSSHKey(ctx context.Context, params database.InsertGitSSHKeyParams) (database.GitSSHKey, error) {
	if err := db.encryptField(&params.PrivateKey, &params.PrivateKeyKeyID); err != nil {
		return database.GitSSHKey{}, err
	}
	key, err := db.Store.InsertGitSSHKey(ctx, params)
	if err != nil {
		return database.GitSSHKey{}, err
	}
	if err := db.decryptField(&key.PrivateKey, key.PrivateKeyKeyID); err != nil {
		return database.GitSSHKey{}, err
	}
	return key, nil
}

func (db *dbCrypt) GetGitSSHKey(ctx context.Context, userID uuid.UUID) (database.GitSSHKey, error) {
	key, err := db.Store.GetGitSSHKey(ctx, userID)
	if err != nil {
		return database.GitSSHKey{}, err
	}
	if err := db.decryptField(&key.PrivateKey, key.PrivateKeyKeyID); err != nil {
		return database.GitSSHKey{}, err
	}
	return key, nil
}

func (db *dbCrypt) UpdateGitSSHKey(ctx context.Context, params database.UpdateGitSSHKeyParams) (database.GitSSHKey, error) {
	if err := db.encryptField(&params.PrivateKey, &params.PrivateKeyKeyID); err != nil {
		return database.GitSSHKey{}, err
	}
	key, err := db.Store.UpdateGitSSHKey(ctx, params)
	if err != nil {
		return database.GitSSHKey{}, err
	}
	if err := db.decryptField(&key.PrivateKey, key.PrivateKeyKeyID); err != nil {
		return database.GitSSHKey{}, err
	}
	return key, nil
}

func (db *dbCrypt) encryptField(field *string, digest *sql.NullString) error {
	// If no cipher is loaded, then we can't encrypt anything!
	if db.ciphers == nil || db.primaryCipherDigest == "" {
		return nil
	}

	if field == nil {
		return xerrors.Errorf("developer error: encryptField called with nil field")
	}
	if digest == nil {
		return xerrors.Errorf("developer error: encryptField called with nil digest")
	}

	encrypted, err := db.ciphers[db.primaryCipherDigest].Encrypt([]byte(*field))
	if err != nil {
		return err
	}
	// Base64 is used to support UTF-8 encoding in PostgreSQL.
	*field = b64encode(encrypted)
	*digest = sql.NullString{String: db.primaryCipherDigest, Valid: true}
	return nil
}

// decryptFields decrypts the given field using the key with the given digest.
// If the value fails to decrypt, sql.ErrNoRows will be returned.
func (db *dbCrypt) decryptField(field *string, digest sql.NullString) error {
	if field == nil {
		return xerrors.Errorf("developer error: decryptField called with nil field")
	}

	if !digest.Valid || digest.String == "" {
		// This field is not encrypted.
		return nil
	}

	key, ok := db.ciphers[digest.String]
	if !ok {
		return &DecryptFailedError{
			Inner: xerrors.Errorf("no cipher with digest %q", digest.String),
		}
	}

	data, err := b64decode(*field)
	if err != nil {
		// If it's not valid base64, we should complain loudly.
		return &DecryptFailedError{
			Inner: xerrors.Errorf("malformed encrypted field %q: %w", *field, err),
		}
	}
	decrypted, err := key.Decrypt(data)
	if err != nil {
		return &DecryptFailedError{Inner: err}
	}
	*field = string(decrypted)
	return nil
}

func (db *dbCrypt) ensureEncryptedWithRetry(ctx context.Context) error {
	var err error
	for i := 0; i < 3; i++ {
		err = db.ensureEncrypted(ctx)
		if err == nil {
			return nil
		}
		// If we get a serialization error, then we need to retry.
		if !database.IsSerializedError(err) {
			return err
		}
		// otherwise, retry
	}
	// If we get here, then we ran out of retries
	return err
}

func (db *dbCrypt) ensureEncrypted(ctx context.Context) error {
	return db.InTx(func(s database.Store) error {
		// Attempt to read the encrypted test fields of the currently active keys.
		ks, err := s.GetDBCryptKeys(ctx)
		if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
			return err
		}

		var highestNumber int32
		var activeCipherFound bool
		for _, k := range ks {
			// If our primary key has been revoked, then we can't do anything.
			if k.RevokedKeyDigest.Valid && k.RevokedKeyDigest.String == db.primaryCipherDigest {
				return xerrors.Errorf("primary encryption key %q has been revoked", db.primaryCipherDigest)
			}

			if k.ActiveKeyDigest.Valid && k.ActiveKeyDigest.String == db.primaryCipherDigest {
				activeCipherFound = true
			}

			if k.Number > highestNumber {
				highestNumber = k.Number
			}
		}

		if activeCipherFound || len(db.ciphers) == 0 {
			return nil
		}

		// If we get here, then we have a new key that we need to insert.
		return s.InsertDBCryptKey(ctx, database.InsertDBCryptKeyParams{
			Number:          highestNumber + 1,
			ActiveKeyDigest: db.primaryCipherDigest,
			Test:            testValue,
		})
	}, &database.TxOptions{Isolation: sql.LevelRepeatableRead})
}
