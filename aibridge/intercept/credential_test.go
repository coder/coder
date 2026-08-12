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

// TestCredential covers the public surface of the Credential interface and its
// three implementations (BYOK, Bedrock, CentralizedPool), plus the
// AsBYOK/AsCentralizedPool helpers interceptors use to route. Only
// CentralizedPool fails over, so AsCentralizedPool must be true only for it.
func TestCredential(t *testing.T) {
	t.Parallel()

	// Matches the VARCHAR(15) DB constraint.
	const maxCredentialHintLength = 15

	tests := []struct {
		name                    string
		newCred                 func(t *testing.T) intercept.Credential
		expectKind              intercept.CredentialKind
		expectAuthHeader        string
		expectHint              string
		expectLength            int
		expectAsBYOK            bool
		expectAsCentralizedPool bool
	}{
		{
			name: "byok_authorization",
			newCred: func(*testing.T) intercept.Credential {
				return intercept.BYOK{Secret: "user-bearer-token", Header: intercept.AuthHeaderAuthorization}
			},
			expectKind:       intercept.CredentialKindBYOK,
			expectAuthHeader: intercept.AuthHeaderAuthorization,
			expectHint:       "us...en",
			expectLength:     len("user-bearer-token"),
			expectAsBYOK:     true,
		},
		{
			name: "byok_xapikey",
			newCred: func(*testing.T) intercept.Credential {
				return intercept.BYOK{Secret: "user-api-key", Header: intercept.AuthHeaderXAPIKey}
			},
			expectKind:       intercept.CredentialKindBYOK,
			expectAuthHeader: intercept.AuthHeaderXAPIKey,
			expectHint:       "us...ey",
			expectLength:     len("user-api-key"),
			expectAsBYOK:     true,
		},
		{
			// Bedrock with static AWS credentials: the access key ID is
			// masked. AWS signs the request, so there is no auth header.
			name: "centralized_bedrock_static",
			newCred: func(*testing.T) intercept.Credential {
				return intercept.Bedrock{AccessKey: "AKIAIOSFODNN7EXAMPLE"}
			},
			expectKind:       intercept.CredentialKindCentralized,
			expectAuthHeader: "",
			expectHint:       "AKIA...MPLE",
			expectLength:     len("AKIAIOSFODNN7EXAMPLE"),
		},
		{
			// Bedrock with dynamic credentials (AWS default credential chain):
			// no static key to mask, so the hint is a descriptive placeholder.
			name: "centralized_bedrock_dynamic",
			newCred: func(*testing.T) intercept.Credential {
				return intercept.Bedrock{AccessKey: ""}
			},
			expectKind:       intercept.CredentialKindCentralized,
			expectAuthHeader: "",
			expectHint:       "<aws chain>",
			expectLength:     0,
		},
		{
			// Pool before failover selects a key: the hint is a placeholder
			// until NextKey hands one out.
			name: "centralized_pool_before_key",
			newCred: func(t *testing.T) intercept.Credential {
				pool, err := keypool.New(config.ProviderAnthropic, []string{"k0-pool-key"}, quartz.NewMock(t), nil)
				require.NoError(t, err)
				return &intercept.CentralizedPool{Pool: pool, Header: intercept.AuthHeaderXAPIKey}
			},
			expectKind:              intercept.CredentialKindCentralized,
			expectAuthHeader:        intercept.AuthHeaderXAPIKey,
			expectHint:              "<failover key>",
			expectLength:            0,
			expectAsCentralizedPool: true,
		},
		{
			// Pool after NextKey: Hint/Length reflect the selected key.
			name: "centralized_pool_after_next_key",
			newCred: func(t *testing.T) intercept.Credential {
				pool, err := keypool.New(config.ProviderAnthropic, []string{"k0-pool-key"}, quartz.NewMock(t), nil)
				require.NoError(t, err)
				cp := &intercept.CentralizedPool{Pool: pool, Header: intercept.AuthHeaderXAPIKey}
				_, keyErr := cp.NextKey(cp.Pool.Walker())
				require.Nil(t, keyErr)
				return cp
			},
			expectKind:              intercept.CredentialKindCentralized,
			expectAuthHeader:        intercept.AuthHeaderXAPIKey,
			expectHint:              "k0...ey",
			expectLength:            len("k0-pool-key"),
			expectAsCentralizedPool: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cred := tc.newCred(t)

			assert.Equal(t, tc.expectKind, cred.Kind(), "Kind")
			assert.Equal(t, tc.expectAuthHeader, cred.AuthHeader(), "AuthHeader")
			assert.Equal(t, tc.expectHint, cred.Hint(), "Hint")
			assert.LessOrEqual(t, len(cred.Hint()), maxCredentialHintLength,
				"Hint must fit the credential_hint column")
			assert.Equal(t, tc.expectLength, cred.Length(), "Length")

			credBYOK, credBYOKOK := intercept.AsBYOK(cred)
			assert.Equal(t, tc.expectAsBYOK, credBYOKOK, "AsBYOK ok")
			if tc.expectAsBYOK {
				assert.Equal(t, cred, credBYOK, "AsBYOK returns the credential")
			}

			credPool, credPoolOK := intercept.AsCentralizedPool(cred)
			assert.Equal(t, tc.expectAsCentralizedPool, credPoolOK, "AsCentralizedPool ok")
			if tc.expectAsCentralizedPool {
				assert.Same(t, cred, credPool, "AsCentralizedPool returns the same pointer")
			} else {
				assert.Nil(t, credPool, "AsCentralizedPool returns nil when not a pool")
			}
		})
	}
}
