package dbcrypt

import (
	"context"
	"database/sql"
	"encoding/base64"

	"github.com/lib/pq"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

// testValue is the value that is stored in dbcrypt_keys.test.
// This is used to determine if the key is valid.
const testValue = "coder"

var (
	ErrNotEnabled = xerrors.New("encryption is not enabled")
	b64encode     = base64.StdEncoding.EncodeToString
	b64decode     = base64.StdEncoding.DecodeString
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
	if len(ciphers) == 0 {
		return nil, xerrors.Errorf("no ciphers configured")
	}
	cm := make(map[string]Cipher)
	for _, c := range ciphers {
		cm[c.HexDigest()] = c
	}
	dbc := &dbCrypt{
		primaryCipherDigest: ciphers[0].HexDigest(),
		ciphers:             cm,
		Store:               db,
	}
	// nolint: gocritic // This is allowed.
	if err := dbc.ensureEncrypted(dbauthz.AsSystemRestricted(ctx)); err != nil {
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

func (db *dbCrypt) InTx(function func(database.Store) error, txOpts *sql.TxOptions) error {
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

func (db *dbCrypt) InsertGitAuthLink(ctx context.Context, params database.InsertGitAuthLinkParams) (database.GitAuthLink, error) {
	if err := db.encryptField(&params.OAuthAccessToken, &params.OAuthAccessTokenKeyID); err != nil {
		return database.GitAuthLink{}, err
	}
	if err := db.encryptField(&params.OAuthRefreshToken, &params.OAuthRefreshTokenKeyID); err != nil {
		return database.GitAuthLink{}, err
	}
	link, err := db.Store.InsertGitAuthLink(ctx, params)
	if err != nil {
		return database.GitAuthLink{}, err
	}
	if err := db.decryptField(&link.OAuthAccessToken, link.OAuthAccessTokenKeyID); err != nil {
		return database.GitAuthLink{}, err
	}
	if err := db.decryptField(&link.OAuthRefreshToken, link.OAuthRefreshTokenKeyID); err != nil {
		return database.GitAuthLink{}, err
	}
	return link, nil
}

func (db *dbCrypt) GetGitAuthLink(ctx context.Context, params database.GetGitAuthLinkParams) (database.GitAuthLink, error) {
	link, err := db.Store.GetGitAuthLink(ctx, params)
	if err != nil {
		return database.GitAuthLink{}, err
	}
	if err := db.decryptField(&link.OAuthAccessToken, link.OAuthAccessTokenKeyID); err != nil {
		return database.GitAuthLink{}, err
	}
	if err := db.decryptField(&link.OAuthRefreshToken, link.OAuthRefreshTokenKeyID); err != nil {
		return database.GitAuthLink{}, err
	}
	return link, nil
}

func (db *dbCrypt) GetGitAuthLinksByUserID(ctx context.Context, userID uuid.UUID) ([]database.GitAuthLink, error) {
	links, err := db.Store.GetGitAuthLinksByUserID(ctx, userID)
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

func (db *dbCrypt) UpdateGitAuthLink(ctx context.Context, params database.UpdateGitAuthLinkParams) (database.GitAuthLink, error) {
	if err := db.encryptField(&params.OAuthAccessToken, &params.OAuthAccessTokenKeyID); err != nil {
		return database.GitAuthLink{}, err
	}
	if err := db.encryptField(&params.OAuthRefreshToken, &params.OAuthRefreshTokenKeyID); err != nil {
		return database.GitAuthLink{}, err
	}
	link, err := db.Store.UpdateGitAuthLink(ctx, params)
	if err != nil {
		return database.GitAuthLink{}, err
	}
	if err := db.decryptField(&link.OAuthAccessToken, link.OAuthAccessTokenKeyID); err != nil {
		return database.GitAuthLink{}, err
	}
	if err := db.decryptField(&link.OAuthRefreshToken, link.OAuthRefreshTokenKeyID); err != nil {
		return database.GitAuthLink{}, err
	}
	return link, nil
}

func (db *dbCrypt) encryptField(field *string, digest *sql.NullString) error {
	// If no cipher is loaded, then we can't encrypt anything!
	if db.ciphers == nil || db.primaryCipherDigest == "" {
		return ErrNotEnabled
	}

	if field == nil {
		return nil
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
	if db.ciphers == nil {
		return ErrNotEnabled
	}

	if !digest.Valid || digest.String == "" {
		// This field is not encrypted.
		return nil
	}

	if field == nil || len(*field) == 0 {
		// We've been asked to decrypt a field that is empty.
		// There is a digest present, so it should have been encrypted,
		// which would have produced a non-empty value.
		// If we return sql.ErrNoRows, then the caller will assume that
		// the value is not present in the database, which is not true.
		// Return a DecryptFailedError to indicate that there is a value
		// present, but it could not be decrypted.
		// Unfortunately the only real way to fix this is to mark this
		// field as not encrypted in the database. We do not want to
		// silently do this, as it would mask a real problem.
		return &DecryptFailedError{
			Inner: xerrors.Errorf("unexpected empty encrypted field with digest %q", digest.String),
		}
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

func (db *dbCrypt) ensureEncrypted(ctx context.Context) error {
	return db.InTx(func(s database.Store) error {
		// Attempt to read the encrypted test fields of the currently active keys.
		ks, err := s.GetDBCryptKeys(ctx)
		if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
			return err
		}

		var highestNumber int32
		for _, k := range ks {
			if k.ActiveKeyDigest.Valid && k.ActiveKeyDigest.String == db.primaryCipherDigest {
				// This is our currently active key. We don't need to do anything further.
				return nil
			}
			if k.Number > highestNumber {
				highestNumber = k.Number
			}
		}

		// If we get here, then we have a new key that we need to insert.
		// If this conflicts with another transaction, we do not need to retry as
		// the other transaction will have inserted the key for us.
		if err := db.InsertDBCryptKey(ctx, database.InsertDBCryptKeyParams{
			Number:          highestNumber + 1,
			ActiveKeyDigest: db.primaryCipherDigest,
			Test:            testValue,
		}); err != nil {
			var pqErr *pq.Error
			if xerrors.As(err, &pqErr) && pqErr.Code == "23505" {
				// Unique constraint violation -> another transaction has inserted the key for us.
				return nil
			}
			return err
		}

		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
}
