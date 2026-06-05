package intercept_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/quartz"
)

// TestCredential covers the public surface of the Credential interface and
// its two implementations (BYOK, Centralized), plus the AsBYOK/AsCentralized
// helpers used by interceptors to branch on credential kind. The Bedrock-style
// pool-less Centralized case is the most important: AsCentralized must return
// false there so the failover loop does not try to walk a nil pool.
func TestCredential(t *testing.T) {
	t.Parallel()

	pool, err := keypool.New(config.ProviderAnthropic, []string{"k0-pool-key"}, quartz.NewMock(t), nil)
	require.NoError(t, err)

	tests := []struct {
		name                string
		newCred             func() intercept.Credential
		expectKind          intercept.CredentialKind
		expectAuthHeader    string
		expectHint          string
		expectLength        int
		expectAsBYOK        bool
		expectAsCentralized bool
	}{
		{
			name: "byok_authorization",
			newCred: func() intercept.Credential {
				return intercept.BYOK{Secret: "user-bearer-token", Header: intercept.AuthHeaderAuthorization}
			},
			expectKind:          intercept.CredentialKindBYOK,
			expectAuthHeader:    intercept.AuthHeaderAuthorization,
			expectHint:          "us...en",
			expectLength:        len("user-bearer-token"),
			expectAsBYOK:        true,
			expectAsCentralized: false,
		},
		{
			name: "byok_xapikey",
			newCred: func() intercept.Credential {
				return intercept.BYOK{Secret: "user-api-key", Header: intercept.AuthHeaderXAPIKey}
			},
			expectKind:          intercept.CredentialKindBYOK,
			expectAuthHeader:    intercept.AuthHeaderXAPIKey,
			expectHint:          "us...ey",
			expectLength:        len("user-api-key"),
			expectAsBYOK:        true,
			expectAsCentralized: false,
		},
		{
			// Centralized hint is empty until the failover loop calls
			// SetKey: the credential is constructed before a key is
			// chosen.
			name: "centralized_with_pool_no_key_selected",
			newCred: func() intercept.Credential {
				return &intercept.Centralized{Pool: pool, Header: intercept.AuthHeaderXAPIKey}
			},
			expectKind:          intercept.CredentialKindCentralized,
			expectAuthHeader:    intercept.AuthHeaderXAPIKey,
			expectHint:          "",
			expectLength:        0,
			expectAsBYOK:        false,
			expectAsCentralized: true,
		},
		{
			name: "centralized_after_set_key",
			newCred: func() intercept.Credential {
				c := &intercept.Centralized{Pool: pool, Header: intercept.AuthHeaderXAPIKey}
				c.SetKey("k0-pool-key")
				return c
			},
			expectKind:          intercept.CredentialKindCentralized,
			expectAuthHeader:    intercept.AuthHeaderXAPIKey,
			expectHint:          "k0...ey",
			expectLength:        len("k0-pool-key"),
			expectAsBYOK:        false,
			expectAsCentralized: true,
		},
		{
			// Bedrock-style: centralized but pool-less (AWS signs the
			// request, no pool to walk). AsCentralized must return
			// false so the failover loop short-circuits to a single
			// attempt instead of walking a nil pool.
			name: "centralized_pool_less_bedrock",
			newCred: func() intercept.Credential {
				return &intercept.Centralized{Pool: nil, Header: intercept.AuthHeaderXAPIKey}
			},
			expectKind:          intercept.CredentialKindCentralized,
			expectAuthHeader:    intercept.AuthHeaderXAPIKey,
			expectHint:          "",
			expectLength:        0,
			expectAsBYOK:        false,
			expectAsCentralized: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cred := tc.newCred()

			assert.Equal(t, tc.expectKind, cred.Kind(), "Kind")
			assert.Equal(t, tc.expectAuthHeader, cred.AuthHeader(), "AuthHeader")
			assert.Equal(t, tc.expectHint, cred.Hint(), "Hint")
			assert.Equal(t, tc.expectLength, cred.Length(), "Length")

			byok, byokOK := intercept.AsBYOK(cred)
			assert.Equal(t, tc.expectAsBYOK, byokOK, "AsBYOK ok")
			if tc.expectAsBYOK {
				assert.Equal(t, cred, byok, "AsBYOK returns the credential")
			}

			centralized, centralizedOK := intercept.AsCentralized(cred)
			assert.Equal(t, tc.expectAsCentralized, centralizedOK, "AsCentralized ok")
			if tc.expectAsCentralized {
				assert.Same(t, cred, centralized, "AsCentralized returns the same pointer")
			} else {
				assert.Nil(t, centralized, "AsCentralized returns nil when not centralized")
			}
		})
	}
}
