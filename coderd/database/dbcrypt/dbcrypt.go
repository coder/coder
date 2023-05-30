package dbcrypt

import (
	"context"
	"database/sql"
	"strings"
	"sync/atomic"

	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/cryptorand"
)

// MagicPrefix is prepended to all encrypted values in the database.
// This is used to determine if a value is encrypted or not.
// If it is encrypted but a key is not provided, an error is returned.
const MagicPrefix = "dbcrypt-"

// ErrInvalidCipher is returned when an invalid cipher is provided
// for the encrypted data.
var ErrInvalidCipher = xerrors.New("an invalid encryption cipher was provided for the encrypted data")

type Options struct {
	// ExternalTokenCipher is an optional cipher that is used
	// to encrypt/decrypt user link and git auth link tokens. If this is nil,
	// then no encryption/decryption will be performed.
	ExternalTokenCipher *atomic.Pointer[cryptorand.Cipher]
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
		*field = MagicPrefix + string(encrypted)
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
				return ErrInvalidCipher
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
			continue
		}

		decrypted, err := cipher.Decrypt([]byte((*field)[len(MagicPrefix):]))
		if err != nil {
			return xerrors.Errorf("%w: %s", ErrInvalidCipher, err)
		}
		*field = string(decrypted)
	}
	return nil
}
