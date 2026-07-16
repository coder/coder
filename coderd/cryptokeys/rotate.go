package cryptokeys

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"slices"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/quartz"
)

const (
	WorkspaceAppsTokenDuration = time.Minute
	OIDCConvertTokenDuration   = time.Minute * 5
	TailnetResumeTokenDuration = time.Hour * 24
	// NATSCAOverlap is how long a NATS cluster CA certificate stays valid past
	// the end of its active-signing window (startsAt + keyDuration). The next CA
	// becomes the active signer at the window's end, but replicas keep minting
	// leaves with the old CA until their key cache refreshes onto the new one.
	// This overlap keeps the old CA valid through that transition, so it must
	// exceed the cache refresh interval (plus a small leaf clamp buffer). Leaf
	// lifetime imposes nothing here: leaves are clamped to just before their
	// signing CA's NotAfter (see coderd/x/nats mintLeaf).
	NATSCAOverlap = time.Minute * 30

	// defaultRotationInterval is the default interval at which keys are checked for rotation.
	defaultRotationInterval = time.Minute * 10
	// DefaultKeyDuration is the default duration for which a key is valid. It applies to all features.
	DefaultKeyDuration = time.Hour * 24 * 30
)

// defaultRotatedFeatures are the crypto key features the rotator manages. It
// intentionally excludes features that are gated behind an experiment or
// deployment flag so that a dormant feature's enum value does not cause the
// rotator to mint keys it has no generator for. Gated features are opted in by
// the caller that owns their generator.
var defaultRotatedFeatures = []database.CryptoKeyFeature{
	database.CryptoKeyFeatureWorkspaceAppsToken,
	database.CryptoKeyFeatureWorkspaceAppsAPIKey,
	database.CryptoKeyFeatureOIDCConvert,
	database.CryptoKeyFeatureTailnetResume,
}

// DefaultRotatedFeatures returns the crypto key features the rotator manages by
// default. It excludes experiment-gated features such as the NATS CA.
func DefaultRotatedFeatures() []database.CryptoKeyFeature {
	return slices.Clone(defaultRotatedFeatures)
}

// rotator is responsible for rotating keys in the database.
type rotator struct {
	db          database.Store
	logger      slog.Logger
	clock       quartz.Clock
	keyDuration time.Duration

	features []database.CryptoKeyFeature
}

type RotatorOption func(*rotator)

func WithClock(clock quartz.Clock) RotatorOption {
	return func(r *rotator) {
		r.clock = clock
	}
}

func WithKeyDuration(keyDuration time.Duration) RotatorOption {
	return func(r *rotator) {
		r.keyDuration = keyDuration
	}
}

// WithFeatures sets the crypto key features the rotator manages, replacing the
// default set. Use this to opt experiment- or deployment-gated features (such
// as the NATS cluster CA) into rotation only when their owner is active.
func WithFeatures(features []database.CryptoKeyFeature) RotatorOption {
	return func(r *rotator) {
		r.features = slices.Clone(features)
	}
}

// StartRotator starts a background process that rotates keys in the database.
// It ensures there's at least one valid key per feature prior to returning.
// Canceling the provided context will stop the background process.
func StartRotator(ctx context.Context, logger slog.Logger, db database.Store, opts ...RotatorOption) {
	//nolint:gocritic // KeyRotator can only rotate crypto keys.
	ctx = dbauthz.AsKeyRotator(ctx)
	kr := &rotator{
		db:          db,
		logger:      logger.Named("keyrotator"),
		clock:       quartz.NewReal(),
		keyDuration: DefaultKeyDuration,
		features:    defaultRotatedFeatures,
	}

	for _, opt := range opts {
		opt(kr)
	}

	err := kr.rotateKeys(ctx)
	if err != nil {
		kr.logger.Critical(ctx, "failed to rotate keys", slog.Error(err))
	}

	go kr.start(ctx)
}

// start begins the process of rotating keys.
// Canceling the context will stop the rotation process.
func (k *rotator) start(ctx context.Context) {
	w := k.clock.TickerFunc(ctx, defaultRotationInterval, func() error {
		err := k.rotateKeys(ctx)
		if err != nil {
			k.logger.Error(ctx, "failed to rotate keys", slog.Error(err))
		}
		return nil
	})
	err := w.Wait()
	k.logger.Debug(ctx, "stopping key rotation", slog.Error(err))
}

// rotateKeys checks for any keys needing rotation or deletion and
// may insert a new key if it detects that a valid one does
// not exist for a feature.
func (k *rotator) rotateKeys(ctx context.Context) error {
	return k.db.InTx(
		func(tx database.Store) error {
			err := tx.AcquireLock(ctx, database.LockIDCryptoKeyRotation)
			if err != nil {
				return xerrors.Errorf("acquire lock: %w", err)
			}

			cryptokeys, err := tx.GetCryptoKeys(ctx)
			if err != nil {
				return xerrors.Errorf("get keys: %w", err)
			}

			featureKeys := keysByFeature(cryptokeys, k.features)

			now := dbtime.Time(k.clock.Now().UTC())
			for feature, keys := range featureKeys {
				// We'll use a counter to determine if we should insert a new key. We should always have at least one key for a feature.
				var validKeys int
				for _, key := range keys {
					switch {
					case shouldDeleteKey(key, now):
						_, err := tx.DeleteCryptoKey(ctx, database.DeleteCryptoKeyParams{
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
					case shouldRotateKey(key, k.keyDuration, now):
						_, err := k.rotateKey(ctx, tx, key, now)
						if err != nil {
							return xerrors.Errorf("rotate key: %w", err)
						}
						k.logger.Debug(ctx, "rotated key",
							slog.F("key", key.Sequence),
							slog.F("feature", key.Feature),
						)
						validKeys++
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
					k.logger.Debug(ctx, "no valid keys detected, inserting new key",
						slog.F("feature", feature),
					)
					_, err := k.insertNewKey(ctx, tx, feature, now)
					if err != nil {
						return xerrors.Errorf("insert new key: %w", err)
					}
				}
			}
			return nil
		}, &database.TxOptions{
			Isolation:    sql.LevelRepeatableRead,
			TxIdentifier: "rotate_keys",
		})
}

func (k *rotator) insertNewKey(ctx context.Context, tx database.Store, feature database.CryptoKeyFeature, startsAt time.Time) (database.CryptoKey, error) {
	secret, err := generateNewSecret(feature, startsAt, k.keyDuration)
	if err != nil {
		return database.CryptoKey{}, xerrors.Errorf("generate new secret: %w", err)
	}

	latestKey, err := tx.GetLatestCryptoKeyByFeature(ctx, feature)
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		return database.CryptoKey{}, xerrors.Errorf("get latest key: %w", err)
	}

	newKey, err := tx.InsertCryptoKey(ctx, database.InsertCryptoKeyParams{
		Feature:  feature,
		Sequence: latestKey.Sequence + 1,
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

	k.logger.Debug(ctx, "inserted new key for feature", slog.F("feature", feature), slog.F("sequence", newKey.Sequence))
	return newKey, nil
}

func (k *rotator) rotateKey(ctx context.Context, tx database.Store, key database.CryptoKey, now time.Time) ([]database.CryptoKey, error) {
	startsAt := minStartsAt(key, now, k.keyDuration)
	newKey, err := k.insertNewKey(ctx, tx, key.Feature, startsAt)
	if err != nil {
		return nil, xerrors.Errorf("insert new key: %w", err)
	}

	// Set old key's deletes_at to an hour + however long the token
	// for this feature is expected to be valid for. This should
	// allow for sufficient time for the new key to propagate to
	// dependent services (i.e. Workspace Proxies).
	deletesAt := startsAt.Add(time.Hour).Add(tokenDuration(key.Feature))

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

// generateNewSecret generates the secret for a new key of the given feature.
// keyDuration is the rotator's key duration; it is only used by features whose
// secret encodes its own validity window (currently only the NATS CA, whose
// certificate must outlive the key row's active-signer period).
func generateNewSecret(feature database.CryptoKeyFeature, startsAt time.Time, keyDuration time.Duration) (string, error) {
	switch feature {
	case database.CryptoKeyFeatureWorkspaceAppsAPIKey:
		return generateKey(32)
	case database.CryptoKeyFeatureWorkspaceAppsToken:
		return generateKey(64)
	case database.CryptoKeyFeatureOIDCConvert:
		return generateKey(64)
	case database.CryptoKeyFeatureTailnetResume:
		return generateKey(64)
	case database.CryptoKeyFeatureNATSCA:
		return generateCASecret(startsAt, keyDuration)
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
	case database.CryptoKeyFeatureWorkspaceAppsAPIKey:
		return WorkspaceAppsTokenDuration
	case database.CryptoKeyFeatureWorkspaceAppsToken:
		return WorkspaceAppsTokenDuration
	case database.CryptoKeyFeatureOIDCConvert:
		return OIDCConvertTokenDuration
	case database.CryptoKeyFeatureTailnetResume:
		return TailnetResumeTokenDuration
	case database.CryptoKeyFeatureNATSCA:
		// The old CA row only needs to outlive its own certificate, which stays
		// valid for NATSCAOverlap past the active-signing window. Keeping the
		// row (and thus its trust-root status) beyond cert expiry is pointless.
		return NATSCAOverlap
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

// keysByFeature groups keys by feature, restricted to the managed feature set.
// GetCryptoKeys returns rows for every feature, but the rotator only manages a
// subset (features can be gated, e.g. nats_ca behind an experiment). Keys for
// features outside the managed set belong to features this rotator is not
// responsible for and are skipped, so their presence (for example nats_ca rows
// left over from a prior experiment-on run) does not abort rotation of the
// managed features.
func keysByFeature(keys []database.CryptoKey, features []database.CryptoKeyFeature) map[database.CryptoKeyFeature][]database.CryptoKey {
	m := map[database.CryptoKeyFeature][]database.CryptoKey{}
	for _, feature := range features {
		m[feature] = []database.CryptoKey{}
	}
	for _, key := range keys {
		if _, ok := m[key.Feature]; !ok {
			continue
		}
		m[key.Feature] = append(m[key.Feature], key)
	}
	return m
}

// minStartsAt ensures the minimum starts_at time we use for a new
// key is no less than 3*the default rotation interval.
func minStartsAt(key database.CryptoKey, now time.Time, keyDuration time.Duration) time.Time {
	expiresAt := key.ExpiresAt(keyDuration)
	minStartsAt := now.Add(3 * defaultRotationInterval)
	if expiresAt.Before(minStartsAt) {
		return minStartsAt
	}
	return expiresAt
}
