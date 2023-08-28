// Package dbcrypt provides a database.Store wrapper that encrypts/decrypts
// values stored at rest in the database.
//
// Encryption is done using a Cipher. The Cipher is stored in an atomic pointer
// so that it can be rotated as required.
//
// The Cipher is currently used to encrypt/decrypt the following fields:
// - database.UserLink.OAuthAccessToken
// - database.UserLink.OAuthRefreshToken
// - database.GitAuthLink.OAuthAccessToken
// - database.GitAuthLink.OAuthRefreshToken
// - database.DBCryptSentinelValue
//
// Encrypted fields are stored in the following format:
// "dbcrypt-<first 7 characters of cipher's SHA256 digest>-<base64-encoded encrypted value>"
//
// The first 7 characters of the cipher's SHA256 digest are used to identify the cipher
// used to encrypt the value.
//
// Two ciphers can be provided to support key rotation. The primary cipher is used to encrypt
// and decrypt all values. We only use the secondary cipher to decrypt values if decryption
// with the primary cipher fails.
package dbcrypt

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"github.com/google/uuid"
	"strings"
	"sync/atomic"

	"github.com/hashicorp/go-multierror"

	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
)

// MagicPrefix is prepended to all encrypted values in the database.
// This is used to determine if a value is encrypted or not.
// If it is encrypted but a key is not provided, an error is returned.
// MagicPrefix will be followed by the first 7 characters of the cipher's
// SHA256 digest, followed by a dash, followed by the base64-encoded
// encrypted value.
const MagicPrefix = "dbcrypt-"

// MagicPrefixLength is the length of the entire prefix used to identify
// encrypted values.
const MagicPrefixLength = len(MagicPrefix) + 8

// sentinelValue is the value that is stored in the database to indicate
// whether encryption is enabled. If not enabled, the value either not
// present, or is the raw string "coder".
// Otherwise, the value must be the encrypted value of the string "coder"
// using the current cipher.
const sentinelValue = "coder"

var (
	ErrNotEnabled = xerrors.New("encryption is not enabled")
	b64encode     = base64.StdEncoding.EncodeToString
	b64decode     = base64.StdEncoding.DecodeString
)

// DecryptFailedError is returned when decryption fails.
// It unwraps to sql.ErrNoRows.
type DecryptFailedError struct {
	Inner error
}

func (e *DecryptFailedError) Error() string {
	return xerrors.Errorf("decrypt failed: %w", e.Inner).Error()
}

func (*DecryptFailedError) Unwrap() error {
	return sql.ErrNoRows
}

func IsDecryptFailedError(err error) bool {
	var e *DecryptFailedError
	return errors.As(err, &e)
}

type Options struct {
	// PrimaryCipher is an optional cipher that is used
	// to encrypt/decrypt user link and git auth link tokens. If this is nil,
	// then no encryption/decryption will be performed.
	PrimaryCipher *atomic.Pointer[Cipher]
	// SecondaryCipher is an optional cipher that is only used
	// to decrypt user link and git auth link tokens.
	// This should only be used when rotating the primary cipher.
	SecondaryCipher *atomic.Pointer[Cipher]
	Logger          slog.Logger
}

// New creates a database.Store wrapper that encrypts/decrypts values
// stored at rest in the database.
func New(ctx context.Context, db database.Store, options *Options) (database.Store, error) {
	if options.PrimaryCipher.Load() == nil {
		return nil, xerrors.Errorf("at least one cipher is required")
	}
	dbc := &dbCrypt{
		Options: options,
		Store:   db,
	}
	if err := ensureEncrypted(dbauthz.AsSystemRestricted(ctx), dbc); err != nil {
		return nil, xerrors.Errorf("ensure encrypted database fields: %w", err)
	}
	return dbc, nil
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
		return "", err
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

func (db *dbCrypt) GetUserLinksByUserID(ctx context.Context, userID uuid.UUID) ([]database.UserLink, error) {
	links, err := db.Store.GetUserLinksByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	for _, link := range links {
		if err := db.decryptFields(&link.OAuthAccessToken, &link.OAuthRefreshToken); err != nil {
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

func (db *dbCrypt) GetGitAuthLinksByUserID(ctx context.Context, userID uuid.UUID) ([]database.GitAuthLink, error) {
	links, err := db.Store.GetGitAuthLinksByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	for _, link := range links {
		if err := db.decryptFields(&link.OAuthAccessToken, &link.OAuthRefreshToken); err != nil {
			return nil, err
		}
	}
	return links, nil
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
	// Encryption ALWAYS happens with the primary cipher.
	cipherPtr := db.PrimaryCipher.Load()
	// If no cipher is loaded, then we can't encrypt anything!
	if cipherPtr == nil {
		return ErrNotEnabled
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
		*field = MagicPrefix + cipher.HexDigest()[:7] + "-" + b64encode(encrypted)
	}
	return nil
}

// decryptFields decrypts the given fields in place.
// If the value fails to decrypt, sql.ErrNoRows will be returned.
func (db *dbCrypt) decryptFields(fields ...*string) error {
	var merr *multierror.Error

	// We try to decrypt with both the primary and secondary cipher.
	primaryCipherPtr := db.PrimaryCipher.Load()
	if err := decryptWithCipher(primaryCipherPtr, fields...); err == nil {
		return nil
	} else {
		merr = multierror.Append(merr, err)
	}
	secondaryCipherPtr := db.SecondaryCipher.Load()
	if err := decryptWithCipher(secondaryCipherPtr, fields...); err == nil {
		return nil
	} else {
		merr = multierror.Append(merr, err)
	}
	return merr
}

func decryptWithCipher(cipherPtr *Cipher, fields ...*string) error {
	// If no cipher is loaded, then we can't decrypt anything!
	if cipherPtr == nil {
		return ErrNotEnabled
	}

	cipher := *cipherPtr
	for _, field := range fields {
		if field == nil {
			continue
		}

		if len(*field) < 16 || !strings.HasPrefix(*field, MagicPrefix) {
			// We do not force decryption of unencrypted rows. This could be damaging
			// to the deployment, and admins can always manually purge data.
			continue
		}

		// The first 7 characters of the digest are used to identify the cipher.
		// If the cipher changes, we should complain loudly.
		encPrefix := cipher.HexDigest()[:7]
		if !strings.HasPrefix((*field)[8:15], encPrefix) {
			return &DecryptFailedError{
				Inner: xerrors.Errorf("cipher mismatch: expected %q, got %q", encPrefix, (*field)[8:15]),
			}
		}
		data, err := b64decode((*field)[16:])
		if err != nil {
			// If it's not base64 with the prefix, we should complain loudly.
			return &DecryptFailedError{
				Inner: xerrors.Errorf("malformed encrypted field %q: %w", *field, err),
			}
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

func ensureEncrypted(ctx context.Context, dbc *dbCrypt) error {
	return dbc.InTx(func(s database.Store) error {
		val, err := s.GetDBCryptSentinelValue(ctx)
		if err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
		}

		if val != "" && val != sentinelValue {
			return xerrors.Errorf("database is already encrypted with a different key")
		}

		// Mark the database as officially having been touched by the new cipher.
		if err := s.SetDBCryptSentinelValue(ctx, sentinelValue); err != nil {
			return xerrors.Errorf("mark database as encrypted: %w", err)
		}

		return nil
	}, nil)
}
