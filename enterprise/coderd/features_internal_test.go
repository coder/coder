package coderd

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/databasefake"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/testutil"
)

func TestFeaturesService_EntitlementsAPI(t *testing.T) {
	t.Parallel()
	logger := slogtest.Make(t, nil)

	// Note that these are not actually used because we don't run the syncEntitlements
	// routine in this test.
	pubsub := database.NewPubsubInMemory()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	keyID := "testing"

	t.Run("NoLicense", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		uut := &featuresService{
			logger:      logger,
			database:    db,
			pubsub:      pubsub,
			keys:        map[string]ed25519.PublicKey{keyID: pub},
			enablements: Enablements{AuditLogs: true},
			entitlements: entitlements{
				hasLicense: false,
				activeUsers: numericalEntitlement{
					entitlement{notEntitled},
					entitlementLimit{
						unlimited: true,
					},
				},
				auditLogs: entitlement{notEntitled},
			},
		}
		result := requestEntitlements(t, uut)
		assert.False(t, result.HasLicense)
		assert.Empty(t, result.Warnings)
		assert.Equal(t, codersdk.EntitlementNotEntitled, result.Features[codersdk.FeatureUserLimit].Entitlement)
		assert.Equal(t, codersdk.EntitlementNotEntitled, result.Features[codersdk.FeatureAuditLog].Entitlement)
	})

	t.Run("FullLicense", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()
		db := databasefake.New()
		uut := &featuresService{
			logger:      logger,
			database:    db,
			pubsub:      pubsub,
			keys:        map[string]ed25519.PublicKey{keyID: pub},
			enablements: Enablements{AuditLogs: true},
			entitlements: entitlements{
				hasLicense: true,
				activeUsers: numericalEntitlement{
					entitlement{entitled},
					entitlementLimit{
						unlimited: false,
						limit:     100,
					},
				},
				auditLogs: entitlement{entitled},
			},
		}
		_, err = db.InsertUser(ctx, database.InsertUserParams{
			ID:             uuid.UUID{},
			Email:          "",
			Username:       "",
			HashedPassword: nil,
			CreatedAt:      time.Time{},
			UpdatedAt:      time.Time{},
			RBACRoles:      nil,
			LoginType:      "",
		})
		require.NoError(t, err)
		result := requestEntitlements(t, uut)
		assert.True(t, result.HasLicense)
		ul := result.Features[codersdk.FeatureUserLimit]
		assert.Equal(t, codersdk.EntitlementEntitled, ul.Entitlement)
		assert.Equal(t, int64(100), *ul.Limit)
		assert.Equal(t, int64(1), *ul.Actual)
		assert.True(t, ul.Enabled)
		al := result.Features[codersdk.FeatureAuditLog]
		assert.Equal(t, codersdk.EntitlementEntitled, al.Entitlement)
		assert.True(t, al.Enabled)
		assert.Nil(t, al.Limit)
		assert.Nil(t, al.Actual)
		assert.Empty(t, result.Warnings)
	})

	t.Run("Warnings", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()
		db := databasefake.New()
		uut := &featuresService{
			logger:      logger,
			database:    db,
			pubsub:      pubsub,
			keys:        map[string]ed25519.PublicKey{keyID: pub},
			enablements: Enablements{AuditLogs: true},
			entitlements: entitlements{
				hasLicense: true,
				activeUsers: numericalEntitlement{
					entitlement{gracePeriod},
					entitlementLimit{
						unlimited: false,
						limit:     4,
					},
				},
				auditLogs: entitlement{gracePeriod},
			},
		}
		for i := byte(0); i < 5; i++ {
			_, err = db.InsertUser(ctx, database.InsertUserParams{
				ID:             uuid.UUID{i},
				Email:          "",
				Username:       "",
				HashedPassword: nil,
				CreatedAt:      time.Time{},
				UpdatedAt:      time.Time{},
				RBACRoles:      nil,
				LoginType:      "",
			})
			require.NoError(t, err)
		}
		result := requestEntitlements(t, uut)
		assert.True(t, result.HasLicense)
		ul := result.Features[codersdk.FeatureUserLimit]
		assert.Equal(t, codersdk.EntitlementGracePeriod, ul.Entitlement)
		assert.Equal(t, int64(4), *ul.Limit)
		assert.Equal(t, int64(5), *ul.Actual)
		assert.True(t, ul.Enabled)
		al := result.Features[codersdk.FeatureAuditLog]
		assert.Equal(t, codersdk.EntitlementGracePeriod, al.Entitlement)
		assert.True(t, al.Enabled)
		assert.Nil(t, al.Limit)
		assert.Nil(t, al.Actual)
		assert.Len(t, result.Warnings, 2)
		assert.Contains(t, result.Warnings,
			"Your deployment has 5 active users but is only licensed for 4")
		assert.Contains(t, result.Warnings,
			"Audit logging is enabled but your license for this feature is expired")
	})
}

func TestFeaturesServiceSyncEntitlements(t *testing.T) {
	t.Parallel()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	keyID := "testing"

	// This tests that pubsub updates work by setting the resync interval very long
	t.Run("PubSub", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()
		logger := slogtest.Make(t, nil)
		pubsub := database.NewPubsubInMemory()
		db := databasefake.New()
		uut := &featuresService{
			logger:         logger,
			database:       db,
			pubsub:         pubsub,
			keys:           map[string]ed25519.PublicKey{keyID: pub},
			enablements:    Enablements{AuditLogs: true},
			resyncInterval: time.Hour, // no resyncs during test
			entitlements:   entitlements{},
		}

		_, invalidKey, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		// Start of day, 3 licenses, one expired, one invalid
		_ = putLicense(ctx, t, db, priv, keyID, 1000, -2*time.Hour, -1*time.Hour)
		_ = putLicense(ctx, t, db, invalidKey, "invalid", 900, time.Hour, 2*time.Hour)
		l0 := putLicense(ctx, t, db, priv, keyID, 300, time.Hour, 2*time.Hour)

		go uut.syncEntitlements(ctx)

		testutil.Eventually(ctx, t, userLimitIs(uut, 300), testutil.IntervalFast)

		// New license
		l1 := putLicense(ctx, t, db, priv, keyID, 305, time.Hour, 2*time.Hour)
		err = pubsub.Publish(PubSubEventLicenses, []byte("add"))
		require.NoError(t, err)

		// User limit goes up, because 305 > 300
		testutil.Eventually(ctx, t, userLimitIs(uut, 305), testutil.IntervalFast)

		// New license with lower limit
		_ = putLicense(ctx, t, db, priv, keyID, 295, time.Hour, 2*time.Hour)
		err = pubsub.Publish(PubSubEventLicenses, []byte("add"))
		require.NoError(t, err)

		// Need to delete the others before the limit lowers
		_, err = db.DeleteLicense(ctx, l1.ID)
		require.NoError(t, err)
		err = pubsub.Publish(PubSubEventLicenses, []byte("delete"))
		require.NoError(t, err)
		testutil.Eventually(ctx, t, userLimitIs(uut, 300), testutil.IntervalFast)

		_, err = db.DeleteLicense(ctx, l0.ID)
		require.NoError(t, err)
		err = pubsub.Publish(PubSubEventLicenses, []byte("delete"))
		require.NoError(t, err)
		testutil.Eventually(ctx, t, userLimitIs(uut, 295), testutil.IntervalFast)
	})

	// This tests that periodic resyncs work by setting the resync interval very fast and
	// not sending any pubsub updates.
	t.Run("Resyncs", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()
		logger := slogtest.Make(t, nil)
		pubsub := database.NewPubsubInMemory()
		db := databasefake.New()
		uut := &featuresService{
			logger:         logger,
			database:       db,
			pubsub:         pubsub,
			keys:           map[string]ed25519.PublicKey{keyID: pub},
			enablements:    Enablements{AuditLogs: true},
			resyncInterval: 10 * time.Millisecond,
			entitlements:   entitlements{},
		}

		_, invalidKey, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		// Start of day, 3 licenses, one expired, one invalid
		_ = putLicense(ctx, t, db, priv, keyID, 1000, -2*time.Hour, -1*time.Hour)
		_ = putLicense(ctx, t, db, invalidKey, "invalid", 900, time.Hour, 2*time.Hour)
		l0 := putLicense(ctx, t, db, priv, keyID, 300, time.Hour, 2*time.Hour)

		go uut.syncEntitlements(ctx)

		testutil.Eventually(ctx, t, userLimitIs(uut, 300), testutil.IntervalFast)

		// New license
		l1 := putLicense(ctx, t, db, priv, keyID, 305, time.Hour, 2*time.Hour)

		// User limit goes up, because 305 > 300
		testutil.Eventually(ctx, t, userLimitIs(uut, 305), testutil.IntervalFast)

		// New license with lower limit
		_ = putLicense(ctx, t, db, priv, keyID, 295, time.Hour, 2*time.Hour)

		// Need to delete the others before the limit lowers
		_, err = db.DeleteLicense(ctx, l1.ID)
		require.NoError(t, err)
		testutil.Eventually(ctx, t, userLimitIs(uut, 300), testutil.IntervalFast)

		_, err = db.DeleteLicense(ctx, l0.ID)
		require.NoError(t, err)
		testutil.Eventually(ctx, t, userLimitIs(uut, 295), testutil.IntervalFast)
	})
}

func requestEntitlements(t *testing.T, uut coderd.FeaturesService) codersdk.Entitlements {
	t.Helper()
	r := httptest.NewRequest("GET", "https://example.com/api/v2/entitlements", nil)
	rw := httptest.NewRecorder()
	uut.EntitlementsAPI(rw, r)
	resp := rw.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	dec := json.NewDecoder(resp.Body)
	var result codersdk.Entitlements
	err := dec.Decode(&result)
	require.NoError(t, err)
	return result
}

func putLicense(
	ctx context.Context, t *testing.T, db database.Store,
	k ed25519.PrivateKey, keyID string, userLimit int64,
	timeToGrace, timeToExpire time.Duration,
) database.License {
	t.Helper()
	c := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "test@testing.test",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(timeToExpire)),
			NotBefore: jwt.NewNumericDate(time.Now().Add(-time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-time.Minute)),
		},
		LicenseExpires: jwt.NewNumericDate(time.Now().Add(timeToGrace)),
		Version:        CurrentVersion,
		Features: Features{
			UserLimit: userLimit,
			AuditLog:  1,
		},
	}
	j, err := makeLicense(c, k, keyID)
	require.NoError(t, err)
	l, err := db.InsertLicense(ctx, database.InsertLicenseParams{
		UploadedAt: c.IssuedAt.Time,
		JWT:        j,
		Exp:        c.ExpiresAt.Time,
	})
	require.NoError(t, err)
	return l
}

func userLimitIs(fs *featuresService, limit int64) func(context.Context) bool {
	return func(_ context.Context) bool {
		fs.mu.RLock()
		defer fs.mu.RUnlock()
		return fs.entitlements.activeUsers.limit == limit
	}
}
