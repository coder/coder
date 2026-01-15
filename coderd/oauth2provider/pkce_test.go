package oauth2provider_test

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/oauth2provider"
	"github.com/coder/coder/v2/codersdk"
)

func TestVerifyPKCE(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		verifier    string
		challenge   string
		expectValid bool
	}{
		{
			name:        "ValidPKCE",
			verifier:    "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk",
			challenge:   "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM",
			expectValid: true,
		},
		{
			name:        "InvalidPKCE",
			verifier:    "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk",
			challenge:   "wrong_challenge",
			expectValid: false,
		},
		{
			name:        "EmptyChallenge",
			verifier:    "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk",
			challenge:   "",
			expectValid: false,
		},
		{
			name:        "EmptyVerifier",
			verifier:    "",
			challenge:   "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM",
			expectValid: false,
		},
		{
			name:        "BothEmpty",
			verifier:    "",
			challenge:   "",
			expectValid: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := oauth2provider.VerifyPKCE(tt.challenge, tt.verifier)
			require.Equal(t, tt.expectValid, result)
		})
	}
}

func TestPKCES256Generation(t *testing.T) {
	t.Parallel()

	// Test that we can generate a valid S256 challenge from a verifier
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	expectedChallenge := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"

	// Generate challenge using S256 method
	h := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(h[:])

	require.Equal(t, expectedChallenge, challenge)
	require.True(t, oauth2provider.VerifyPKCE(challenge, verifier))
}

func TestValidatePKCECodeChallengeMethod(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		method        string
		expectError   bool
		errorContains string
	}{
		{
			name:        "EmptyIsValid",
			method:      "",
			expectError: false,
		},
		{
			name:        "S256IsValid",
			method:      string(codersdk.OAuth2PKCECodeChallengeMethodS256),
			expectError: false,
		},
		{
			name:          "PlainIsRejected",
			method:        string(codersdk.OAuth2PKCECodeChallengeMethodPlain),
			expectError:   true,
			errorContains: "plain",
		},
		{
			name:          "UnknownIsRejected",
			method:        "unknown_method",
			expectError:   true,
			errorContains: "unsupported",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := codersdk.ValidatePKCECodeChallengeMethod(tt.method)
			if tt.expectError {
				require.Error(t, err)
				if tt.errorContains != "" {
					require.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}
