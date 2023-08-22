package dbcrypt

import (
	"context"
	"database/sql"
	"encoding/base64"
	"runtime"
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

func (db *dbCrypt) GetUserLinkByLinkedID(ctx context.Context, linkedID string) (database.UserLink, error) {
	link, err := db.Store.GetUserLinkByLinkedID(ctx, linkedID)
	if err != nil {
		return database.UserLink{}, err
	}
	return link, db.decryptFields(func() error {
		return db.Store.DeleteUserLinkByLinkedID(ctx, linkedID)
	}, &link.OAuthAccessToken, &link.OAuthRefreshToken)
}

func (db *dbCrypt) GetUserLinkByUserIDLoginType(ctx context.Context, params database.GetUserLinkByUserIDLoginTypeParams) (database.UserLink, error) {
	link, err := db.Store.GetUserLinkByUserIDLoginType(ctx, params)
	if err != nil {
		return database.UserLink{}, err
	}
	return link, db.decryptFields(func() error {
		return db.Store.DeleteUserLinkByLinkedID(ctx, link.LinkedID)
	}, &link.OAuthAccessToken, &link.OAuthRefreshToken)
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
	return link, db.decryptFields(func() error {
		return db.Store.DeleteGitAuthLink(ctx, database.DeleteGitAuthLinkParams{ // nolint:gosimple
			ProviderID: params.ProviderID,
			UserID:     params.UserID,
		})
	}, &link.OAuthAccessToken, &link.OAuthRefreshToken)
}

func (db *dbCrypt) UpdateGitAuthLink(ctx context.Context, params database.UpdateGitAuthLinkParams) (database.GitAuthLink, error) {
	err := db.encryptFields(&params.OAuthAccessToken, &params.OAuthRefreshToken)
	if err != nil {
		return database.GitAuthLink{}, err
	}
	return db.Store.UpdateGitAuthLink(ctx, params)
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
func (db *dbCrypt) decryptFields(deleteFn func() error, fields ...*string) error {
	doDelete := func(reason string) error {
		err := deleteFn()
		if err != nil {
			return xerrors.Errorf("delete encrypted row: %w", err)
		}
		pc, _, _, ok := runtime.Caller(2)
		details := runtime.FuncForPC(pc)
		if ok && details != nil {
			db.Logger.Debug(context.Background(), "deleted row", slog.F("reason", reason), slog.F("caller", details.Name()))
		}
		return sql.ErrNoRows
	}

	cipherPtr := db.ExternalTokenCipher.Load()
	// If no cipher is loaded, then we don't need to encrypt or decrypt anything!
	if cipherPtr == nil {
		for _, field := range fields {
			if field == nil {
				continue
			}
			if strings.HasPrefix(*field, MagicPrefix) {
				// If we have a magic prefix but encryption is disabled,
				// we should delete the row.
				return doDelete("encryption disabled")
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
			// If it's not base64 with the prefix, we should delete the row.
			return doDelete("stored value was not base64 encoded")
		}
		decrypted, err := cipher.Decrypt(data)
		if err != nil {
			// If the encryption key changed, we should delete the row.
			return doDelete("encryption key changed")
		}
		*field = string(decrypted)
	}
	return nil
}
