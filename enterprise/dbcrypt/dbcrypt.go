package dbcrypt

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"strings"
	"sync/atomic"

	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/database"
)

// MagicPrefix is prepended to all encrypted values in the database.
// This is used to determine if a value is encrypted or not.
// If it is encrypted but a key is not provided, an error is returned.
const MagicPrefix = "dbcrypt-"

// sentinelValue is the value that is stored in the database to indicate
// whether encryption is enabled. If not enabled, the raw value is "coder".
// Otherwise, the value is encrypted.
const sentinelValue = "coder"

var ErrNotEncrypted = xerrors.New("database is not encrypted")

type Options struct {
	// ExternalTokenCipher is an optional cipher that is used
	// to encrypt/decrypt user link and git auth link tokens. If this is nil,
	// then no encryption/decryption will be performed.
	ExternalTokenCipher *atomic.Pointer[Cipher]
	Logger              slog.Logger
}

// New creates a database.Store wrapper that encrypts/decrypts values
// stored at rest in the database.
func New(db database.Store, options *Options) database.Store {
	return &dbCrypt{
		Options: options,
		Store:   db,
	}
}

type dbCrypt struct {
	*Options
	database.Store
}

func (db *dbCrypt) InTx(function func(database.Store) error, txOpts *sql.TxOptions) error {
	return db.Store.InTx(func(s database.Store) error {
		return function(&dbCrypt{
			Options: db.Options,
			Store:   s,
		})
	}, txOpts)
}

func (db *dbCrypt) GetDBCryptSentinelValue(ctx context.Context) (string, error) {
	rawValue, err := db.Store.GetDBCryptSentinelValue(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotEncrypted
		}
		return "", err
	}
	if rawValue == sentinelValue {
		return "", ErrNotEncrypted
	}
	return rawValue, db.decryptFields(&rawValue)
}

func (db *dbCrypt) GetUserLinkByLinkedID(ctx context.Context, linkedID string) (database.UserLink, error) {
	link, err := db.Store.GetUserLinkByLinkedID(ctx, linkedID)
	if err != nil {
		return database.UserLink{}, err
	}
	return link, db.decryptFields(&link.OAuthAccessToken, &link.OAuthRefreshToken)
}

func (db *dbCrypt) GetUserLinkByUserIDLoginType(ctx context.Context, params database.GetUserLinkByUserIDLoginTypeParams) (database.UserLink, error) {
	link, err := db.Store.GetUserLinkByUserIDLoginType(ctx, params)
	if err != nil {
		return database.UserLink{}, err
	}
	return link, db.decryptFields(&link.OAuthAccessToken, &link.OAuthRefreshToken)
}

func (db *dbCrypt) InsertUserLink(ctx context.Context, params database.InsertUserLinkParams) (database.UserLink, error) {
	err := db.encryptFields(&params.OAuthAccessToken, &params.OAuthRefreshToken)
	if err != nil {
		return database.UserLink{}, err
	}
	return db.Store.InsertUserLink(ctx, params)
}

func (db *dbCrypt) UpdateUserLink(ctx context.Context, params database.UpdateUserLinkParams) (database.UserLink, error) {
	err := db.encryptFields(&params.OAuthAccessToken, &params.OAuthRefreshToken)
	if err != nil {
		return database.UserLink{}, err
	}
	return db.Store.UpdateUserLink(ctx, params)
}

func (db *dbCrypt) InsertGitAuthLink(ctx context.Context, params database.InsertGitAuthLinkParams) (database.GitAuthLink, error) {
	err := db.encryptFields(&params.OAuthAccessToken, &params.OAuthRefreshToken)
	if err != nil {
		return database.GitAuthLink{}, err
	}
	return db.Store.InsertGitAuthLink(ctx, params)
}

func (db *dbCrypt) GetGitAuthLink(ctx context.Context, params database.GetGitAuthLinkParams) (database.GitAuthLink, error) {
	link, err := db.Store.GetGitAuthLink(ctx, params)
	if err != nil {
		return database.GitAuthLink{}, err
	}
	return link, db.decryptFields(&link.OAuthAccessToken, &link.OAuthRefreshToken)
}

func (db *dbCrypt) UpdateGitAuthLink(ctx context.Context, params database.UpdateGitAuthLinkParams) (database.GitAuthLink, error) {
	err := db.encryptFields(&params.OAuthAccessToken, &params.OAuthRefreshToken)
	if err != nil {
		return database.GitAuthLink{}, err
	}
	return db.Store.UpdateGitAuthLink(ctx, params)
}

func (db *dbCrypt) SetDBCryptSentinelValue(ctx context.Context, value string) error {
	err := db.encryptFields(&value)
	if err != nil {
		return err
	}
	return db.Store.SetDBCryptSentinelValue(ctx, value)
}

func (db *dbCrypt) encryptFields(fields ...*string) error {
	cipherPtr := db.ExternalTokenCipher.Load()
	// If no cipher is loaded, then we don't need to encrypt or decrypt anything!
	if cipherPtr == nil {
		return nil
	}
	cipher := *cipherPtr
	for _, field := range fields {
		if field == nil {
			continue
		}

		encrypted, err := cipher.Encrypt([]byte(*field))
		if err != nil {
			return err
		}
		// Base64 is used to support UTF-8 encoding in PostgreSQL.
		*field = MagicPrefix + base64.StdEncoding.EncodeToString(encrypted)
	}
	return nil
}

// decryptFields decrypts the given fields in place.
// If the value fails to decrypt, sql.ErrNoRows will be returned.
func (db *dbCrypt) decryptFields(fields ...*string) error {
	cipherPtr := db.ExternalTokenCipher.Load()
	// If no cipher is loaded, then we don't need to encrypt or decrypt anything!
	if cipherPtr == nil {
		for _, field := range fields {
			if field == nil {
				continue
			}
			if strings.HasPrefix(*field, MagicPrefix) {
				// If we have a magic prefix but encryption is disabled,
				// complain loudly.
				return xerrors.Errorf("failed to decrypt field %q: encryption is disabled", *field)
			}
		}
		return nil
	}

	cipher := *cipherPtr
	for _, field := range fields {
		if field == nil {
			continue
		}
		if len(*field) < len(MagicPrefix) || !strings.HasPrefix(*field, MagicPrefix) {
			// We do not force encryption of unencrypted rows. This could be damaging
			// to the deployment, and admins can always manually purge data.
			continue
		}
		data, err := base64.StdEncoding.DecodeString((*field)[len(MagicPrefix):])
		if err != nil {
			// If it's not base64 with the prefix, we should complain loudly.
			return xerrors.Errorf("malformed encrypted field %q: %w", *field, err)
		}
		decrypted, err := cipher.Decrypt(data)
		if err != nil {
			// If the encryption key changed, return our special error that unwraps to sql.ErrNoRows.
			return &DecryptFailedError{Inner: err}
		}
		*field = string(decrypted)
	}
	return nil
}
