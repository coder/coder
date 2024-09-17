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

	// DefaultRotationInterval is the default interval at which keys are checked for rotation.
	DefaultRotationInterval = time.Minute * 10
	// DefaultKeyDuration is the default duration for which a key is valid. It applies to all features.
	DefaultKeyDuration = time.Hour * 24 * 30
)

// Rotator is responsible for rotating keys in the database.
type Rotator struct {
	db          database.Store
	logger      slog.Logger
	clock       quartz.Clock
	keyDuration time.Duration
	// resultsCh is purely for testing.
	resultsCh chan []database.CryptoKey

	// The following fields are instantiated in Open.
	ticker   *quartz.Ticker
	features []database.CryptoKeyFeature
}

// Open instantiates a new Rotator. It ensures there's at least one
// valid key per feature prior to returning. Close should be called
// to ensure the ticker that is instantiated is not leaked.
func Open(ctx context.Context, db database.Store, logger slog.Logger, clock quartz.Clock, rotateInterval time.Duration, keyDuration time.Duration, results chan []database.CryptoKey) (*Rotator, error) {
	if keyDuration == 0 || rotateInterval == 0 {
		return nil, xerrors.Errorf("key duration and rotate interval must be set")
	}

	kr := &Rotator{
		db:          db,
		keyDuration: keyDuration,
		logger:      logger,
		clock:       clock,
		resultsCh:   results,

		features: database.AllCryptoKeyFeatureValues(),
		ticker:   clock.NewTicker(rotateInterval),
	}
	_, err := kr.rotateKeys(ctx)
	if err != nil {
		kr.Close()
		return nil, xerrors.Errorf("rotate keys: %w", err)
	}

	return kr, nil
}

// Start begins the rotation routine. Callers should invoke this in a goroutine.
func (k *Rotator) Start(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-k.ticker.C:
		}

		modifiedKeys, err := k.rotateKeys(ctx)
		if err != nil {
			k.logger.Error(ctx, "failed to rotate keys", slog.Error(err))
		}

		// This should only be called in test code so we don't
		// bother to select on the push.
		if k.resultsCh != nil {
			k.resultsCh <- modifiedKeys
		}
	}
}

// rotateKeys checks for any keys needing rotation or deletion and
// may insert a new key if it detects that a valid one does
// not exist for a feature.
func (k *Rotator) rotateKeys(ctx context.Context) ([]database.CryptoKey, error) {
	var modifiedKeys []database.CryptoKey
	return modifiedKeys, database.ReadModifyUpdate(k.db, func(tx database.Store) error {
		// Reset the modified keys slice for each iteration.
		modifiedKeys = make([]database.CryptoKey, 0)
		keys, err := tx.GetCryptoKeys(ctx)
		if err != nil {
			return xerrors.Errorf("get keys: %w", err)
		}

		// Groups the keys by feature so that we can
		// ensure we have at least one key for each feature.
		keysByFeature := keysByFeature(keys, k.features)
		now := dbtime.Time(k.clock.Now().UTC())
		for feature, keys := range keysByFeature {
			// We'll use a counter to determine if we should insert a new key. We should always have at least one key for a feature.
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
					k.logger.Debug(ctx, "deleted key",
						slog.F("key", key.Sequence),
						slog.F("feature", key.Feature),
					)
					modifiedKeys = append(modifiedKeys, deletedKey)
				case shouldRotateKey(key, k.keyDuration, now):
					rotatedKeys, err := k.rotateKey(ctx, tx, key)
					if err != nil {
						return xerrors.Errorf("rotate key: %w", err)
					}
					k.logger.Debug(ctx, "rotated key",
						slog.F("key", key.Sequence),
						slog.F("feature", key.Feature),
					)
					validKeys++
					modifiedKeys = append(modifiedKeys, rotatedKeys...)
				default:
					// We only consider keys without a populated deletes_at field as valid.
					// This is because under normal circumstances the deletes_at field
					// is set during rotation (meaning a new key was generated)
					// but it's possible if the database was manually altered to
					// delete the new key we may be in a situation where there
					// isn't a key to replace the one scheduled for deletion.
					if !key.DeletesAt.Valid {
						validKeys++
					}
				}
			}
			if validKeys == 0 {
				k.logger.Info(ctx, "no valid keys detected, inserting new key",
					slog.F("feature", feature),
				)
				latestKey, err := tx.GetLatestCryptoKeyByFeature(ctx, feature)
				if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
					return xerrors.Errorf("get latest key: %w", err)
				}

				newKey, err := k.insertNewKey(ctx, tx, feature, now, latestKey.Sequence+1)
				if err != nil {
					return xerrors.Errorf("insert new key: %w", err)
				}
				modifiedKeys = append(modifiedKeys, newKey)
			}
		}
		return nil
	})
}

func (k *Rotator) insertNewKey(ctx context.Context, tx database.Store, feature database.CryptoKeyFeature, startsAt time.Time, sequence int32) (database.CryptoKey, error) {
	secret, err := generateNewSecret(feature)
	if err != nil {
		return database.CryptoKey{}, xerrors.Errorf("generate new secret: %w", err)
	}

	newKey, err := tx.InsertCryptoKey(ctx, database.InsertCryptoKeyParams{
		Feature:  feature,
		Sequence: sequence,
		Secret: sql.NullString{
			String: secret,
			Valid:  true,
		},
		// Set by dbcrypt if it's required.
		SecretKeyID: sql.NullString{},
		StartsAt:    startsAt.UTC(),
	})
	if err != nil {
		return database.CryptoKey{}, xerrors.Errorf("inserting new key: %w", err)
	}

	k.logger.Info(ctx, "inserted new key for feature", slog.F("feature", feature))
	return newKey, nil
}

func (k *Rotator) rotateKey(ctx context.Context, tx database.Store, key database.CryptoKey) ([]database.CryptoKey, error) {
	// The starts at of the new key is the expiration of the old key. We set the deletes_at of the old key to a little over
	// an hour after its set to expire so there should plenty
	// of time for the new key to enter rotation.
	newStartsAt := key.ExpiresAt(k.keyDuration)

	newKey, err := k.insertNewKey(ctx, tx, key.Feature, newStartsAt, key.Sequence+1)
	if err != nil {
		return nil, xerrors.Errorf("insert new key: %w", err)
	}

	// Set old key's deletes_at to an hour + however long the token
	// for this feature is expected to be valid for. This should
	// allow for sufficient time for the new key to propagate to
	// dependent services (i.e. Workspace Proxies).
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

func (k *Rotator) Close() {
	k.ticker.Stop()
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
	return !now.Add(time.Hour).UTC().Before(expirationTime)
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
