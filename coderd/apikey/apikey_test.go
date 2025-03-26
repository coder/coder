package apikey_test

import (
	"crypto/sha256"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/apikey"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
)

func TestGenerate(t *testing.T) {
	t.Parallel()

	type testcase struct {
		name   string
		params apikey.CreateParams
		fail   bool
	}

	cases := []testcase{
		{
			name: "OK",
			params: apikey.CreateParams{
				UserID:          uuid.New(),
				LoginType:       database.LoginTypeOIDC,
				DefaultLifetime: time.Duration(0),
				ExpiresAt:       time.Now().Add(time.Hour),
				LifetimeSeconds: int64(time.Hour.Seconds()),
				TokenName:       "hello",
				RemoteAddr:      "1.2.3.4",
				Scope:           database.APIKeyScopeApplicationConnect,
			},
		},
		{
			name: "InvalidScope",
			params: apikey.CreateParams{
				UserID:          uuid.New(),
				LoginType:       database.LoginTypeOIDC,
				DefaultLifetime: time.Duration(0),
				ExpiresAt:       time.Now().Add(time.Hour),
				LifetimeSeconds: int64(time.Hour.Seconds()),
				TokenName:       "hello",
				RemoteAddr:      "1.2.3.4",
				Scope:           database.APIKeyScope("test"),
			},
			fail: true,
		},
		{
			name: "DeploymentSessionDuration",
			params: apikey.CreateParams{
				UserID:          uuid.New(),
				LoginType:       database.LoginTypeOIDC,
				DefaultLifetime: time.Hour,
				LifetimeSeconds: 0,
				ExpiresAt:       time.Time{},
				TokenName:       "hello",
				RemoteAddr:      "1.2.3.4",
				Scope:           database.APIKeyScopeApplicationConnect,
			},
		},
		{
			name: "LifetimeSeconds",
			params: apikey.CreateParams{
				UserID:          uuid.New(),
				LoginType:       database.LoginTypeOIDC,
				DefaultLifetime: time.Duration(0),
				LifetimeSeconds: int64(time.Hour.Seconds()),
				ExpiresAt:       time.Time{},
				TokenName:       "hello",
				RemoteAddr:      "1.2.3.4",
				Scope:           database.APIKeyScopeApplicationConnect,
			},
		},
		{
			name: "DefaultIP",
			params: apikey.CreateParams{
				UserID:          uuid.New(),
				LoginType:       database.LoginTypeOIDC,
				DefaultLifetime: time.Duration(0),
				ExpiresAt:       time.Now().Add(time.Hour),
				LifetimeSeconds: int64(time.Hour.Seconds()),
				TokenName:       "hello",
				RemoteAddr:      "",
				Scope:           database.APIKeyScopeApplicationConnect,
			},
		},
		{
			name: "DefaultScope",
			params: apikey.CreateParams{
				UserID:          uuid.New(),
				LoginType:       database.LoginTypeOIDC,
				DefaultLifetime: time.Duration(0),
				ExpiresAt:       time.Now().Add(time.Hour),
				LifetimeSeconds: int64(time.Hour.Seconds()),
				TokenName:       "hello",
				RemoteAddr:      "1.2.3.4",
				Scope:           "",
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			key, keystr, err := apikey.Generate(tc.params)
			if tc.fail {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotEmpty(t, keystr)
			require.NotEmpty(t, key.ID)
			require.NotEmpty(t, key.HashedSecret)

			// Assert the string secret is formatted correctly
			keytokens := strings.Split(keystr, "-")
			require.Len(t, keytokens, 2)
			require.Equal(t, key.ID, keytokens[0])

			// Assert that the hashed secret is correct.
			hashed := sha256.Sum256([]byte(keytokens[1]))
			assert.ElementsMatch(t, hashed, key.HashedSecret)

			assert.Equal(t, tc.params.UserID, key.UserID)
			assert.WithinDuration(t, dbtime.Now(), key.CreatedAt, time.Second*5)
			assert.WithinDuration(t, dbtime.Now(), key.UpdatedAt, time.Second*5)

			switch {
			case tc.params.LifetimeSeconds > 0:
				assert.Equal(t, tc.params.LifetimeSeconds, key.LifetimeSeconds)
			case !tc.params.ExpiresAt.IsZero():
				// Should not be a delta greater than 5 seconds.
				assert.InDelta(t, time.Until(tc.params.ExpiresAt).Seconds(), key.LifetimeSeconds, 5)
			default:
				assert.Equal(t, int64(tc.params.DefaultLifetime.Seconds()), key.LifetimeSeconds)
			}

			switch {
			case !tc.params.ExpiresAt.IsZero():
				assert.Equal(t, tc.params.ExpiresAt.UTC(), key.ExpiresAt)
			case tc.params.LifetimeSeconds > 0:
				assert.WithinDuration(t, dbtime.Now().Add(time.Duration(tc.params.LifetimeSeconds)*time.Second), key.ExpiresAt, time.Second*5)
			default:
				assert.WithinDuration(t, dbtime.Now().Add(tc.params.DefaultLifetime), key.ExpiresAt, time.Second*5)
			}

			if tc.params.RemoteAddr != "" {
				assert.Equal(t, tc.params.RemoteAddr, key.IPAddress.IPNet.IP.String())
			} else {
				assert.Equal(t, "0.0.0.0", key.IPAddress.IPNet.IP.String())
			}

			if tc.params.Scope != "" {
				assert.Equal(t, tc.params.Scope, key.Scope)
			} else {
				assert.Equal(t, database.APIKeyScopeAll, key.Scope)
			}

			if tc.params.TokenName != "" {
				assert.Equal(t, tc.params.TokenName, key.TokenName)
			}
			if tc.params.LoginType != "" {
				assert.Equal(t, tc.params.LoginType, key.LoginType)
			}
		})
	}
}