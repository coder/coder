package keyrotate

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/quartz"
)

const (
	WorkspaceAppsTokenDuration = time.Minute
	OIDCConvertTokenDuration   = time.Minute * 5
	TailnetResumeTokenDuration = time.Hour * 24
)

type KeyRotator struct {
	DB           database.Store
	KeyDuration  time.Duration
	Clock        quartz.Clock
	Logger       slog.Logger
	ScanInterval time.Duration
	ResultsCh    chan []database.CryptoKey
	features     []database.CryptoKeyFeature
}

func (k *KeyRotator) Start(ctx context.Context) {
	ticker := k.Clock.NewTicker(k.ScanInterval)
	defer ticker.Stop()

	if len(k.features) == 0 {
		k.features = database.AllCryptoKeyFeatureValues()
	}

	for {
		modifiedKeys, err := k.rotateKeys(ctx)
		if err != nil {
			k.Logger.Error(ctx, "failed to rotate keys", slog.Error(err))
		}

		// This should only be called in test code so we don't
		// both to select on the push.
		if k.ResultsCh != nil {
			k.ResultsCh <- modifiedKeys
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// rotateKeys checks for keys nearing expiration and rotates them if necessary.
func (k *KeyRotator) rotateKeys(ctx context.Context) ([]database.CryptoKey, error) {
	var modifiedKeys []database.CryptoKey
	return modifiedKeys, database.ReadModifyUpdate(k.DB, func(tx database.Store) error {
		// Reset the modified keys slice for each iteration.
		modifiedKeys = make([]database.CryptoKey, 0)
		keys, err := tx.GetCryptoKeys(ctx)
		if err != nil {
			return xerrors.Errorf("get keys: %w", err)
		}

		// Groups the keys by feature so that we can
		// ensure we have at least one key for each feature.
		keysByFeature := keysByFeature(keys, k.features)
		now := dbtime.Time(k.Clock.Now().UTC())
		for feature, keys := range keysByFeature {
			// We'll use this to determine if we should insert a new key. We should always have at least one key for a feature.
			var validKeys int
			for _, key := range keys {
				switch {
				case shouldDeleteKey(key, now):
					deletedKey, err := tx.DeleteCryptoKey(ctx, database.DeleteCryptoKeyParams{
						Feature:  key.Feature,
						Sequence: key.Sequence,
					})
					if err != nil {
						return xerrors.Errorf("delete key: %w", err)
					}
					k.Logger.Debug(ctx, "deleted key",
						slog.F("key", key.Sequence),
						slog.F("feature", key.Feature),
					)
					modifiedKeys = append(modifiedKeys, deletedKey)
				case shouldRotateKey(key, k.KeyDuration, now):
					rotatedKeys, err := k.rotateKey(ctx, tx, key)
					if err != nil {
						return xerrors.Errorf("rotate key: %w", err)
					}
					validKeys++
					modifiedKeys = append(modifiedKeys, rotatedKeys...)
				default:
					// Even though the key is valid for signing we don't consider it valid for the purpose of determining if we should generate a new key. Under normal circumstances the deletes_at field is set during rotation (meaning a new key was generated) but it's possible if the database was manually altered to delete the new key we may be in a situation where there isn't a key to replace the one scheduled for deletion.
					if !key.DeletesAt.Valid {
						validKeys++
					}
				}
			}
			if validKeys == 0 {
				k.Logger.Info(ctx, "no valid keys detected, inserting new key",
					slog.F("feature", feature),
				)
				newKey, err := k.insertNewKey(ctx, tx, feature, now)
				if err != nil {
					return xerrors.Errorf("insert new key: %w", err)
				}
				modifiedKeys = append(modifiedKeys, newKey)
			}
		}
		return nil
	})
}

func (k *KeyRotator) insertNewKey(ctx context.Context, tx database.Store, feature database.CryptoKeyFeature, now time.Time) (database.CryptoKey, error) {
	secret, err := generateNewSecret(feature)
	if err != nil {
		return database.CryptoKey{}, xerrors.Errorf("generate new secret: %w", err)
	}

	latestKey, err := tx.GetLatestCryptoKeyByFeature(ctx, feature)
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		return database.CryptoKey{}, xerrors.Errorf("get latest key: %w", err)
	}

	newKey, err := tx.InsertCryptoKey(ctx, database.InsertCryptoKeyParams{
		Feature: feature,
		// We'll assume that the first key we insert is 1.
		Sequence: latestKey.Sequence + 1,
		Secret: sql.NullString{
			String: secret,
			Valid:  true,
		},
		// Set by dbcrypt if it's required.
		SecretKeyID: sql.NullString{},
		StartsAt:    now.UTC(),
	})
	if err != nil {
		return database.CryptoKey{}, xerrors.Errorf("inserting new key: %w", err)
	}

	k.Logger.Info(ctx, "inserted new key for feature", slog.F("feature", feature))
	return newKey, nil
}

func (k *KeyRotator) rotateKey(ctx context.Context, tx database.Store, key database.CryptoKey) ([]database.CryptoKey, error) {
	// The starts at of the new key is the expiration of the old key.
	newStartsAt := key.ExpiresAt(k.KeyDuration)

	secret, err := generateNewSecret(key.Feature)
	if err != nil {
		return nil, xerrors.Errorf("generate new secret: %w", err)
	}

	// Insert new key
	newKey, err := tx.InsertCryptoKey(ctx, database.InsertCryptoKeyParams{
		Feature:  key.Feature,
		Sequence: key.Sequence + 1,
		Secret: sql.NullString{
			String: secret,
			Valid:  true,
		},
		// Set by dbcrypt if it's required.
		SecretKeyID: sql.NullString{},
		StartsAt:    newStartsAt.UTC(),
	})
	if err != nil {
		return nil, xerrors.Errorf("inserting new key: %w", err)
	}

	// Set old key's deletes_at
	deletesAt := newStartsAt.Add(time.Hour).Add(tokenDuration(key.Feature))

	updatedKey, err := tx.UpdateCryptoKeyDeletesAt(ctx, database.UpdateCryptoKeyDeletesAtParams{
		Feature:  key.Feature,
		Sequence: key.Sequence,
		DeletesAt: sql.NullTime{
			Time:  deletesAt.UTC(),
			Valid: true,
		},
	})
	if err != nil {
		return nil, xerrors.Errorf("update old key's deletes_at: %w", err)
	}

	return []database.CryptoKey{updatedKey, newKey}, nil
}

func generateNewSecret(feature database.CryptoKeyFeature) (string, error) {
	switch feature {
	case database.CryptoKeyFeatureWorkspaceApps:
		return generateKey(96)
	case database.CryptoKeyFeatureOidcConvert:
		return generateKey(32)
	case database.CryptoKeyFeatureTailnetResume:
		return generateKey(64)
	}
	return "", xerrors.Errorf("unknown feature: %s", feature)
}

func generateKey(length int) (string, error) {
	b := make([]byte, length)
	_, err := rand.Read(b)
	if err != nil {
		return "", xerrors.Errorf("rand read: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func tokenDuration(feature database.CryptoKeyFeature) time.Duration {
	switch feature {
	case database.CryptoKeyFeatureWorkspaceApps:
		return WorkspaceAppsTokenDuration
	case database.CryptoKeyFeatureOidcConvert:
		return OIDCConvertTokenDuration
	case database.CryptoKeyFeatureTailnetResume:
		return TailnetResumeTokenDuration
	default:
		return 0
	}
}

func shouldDeleteKey(key database.CryptoKey, now time.Time) bool {
	return key.DeletesAt.Valid && !now.Before(key.DeletesAt.Time.UTC())
}

func shouldRotateKey(key database.CryptoKey, keyDuration time.Duration, now time.Time) bool {
	// If deletes_at is set, we've already inserted a key.
	if key.DeletesAt.Valid {
		return false
	}
	expirationTime := key.ExpiresAt(keyDuration)
	return !now.Add(time.Hour).UTC().Before(expirationTime.UTC())
}

func keysByFeature(keys []database.CryptoKey, features []database.CryptoKeyFeature) map[database.CryptoKeyFeature][]database.CryptoKey {
	m := map[database.CryptoKeyFeature][]database.CryptoKey{}
	for _, feature := range features {
		m[feature] = []database.CryptoKey{}
	}
	for _, key := range keys {
		m[key.Feature] = append(m[key.Feature], key)
	}
	return m
}
