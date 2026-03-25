package automations_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/automations"
)

func TestVerifySignature(t *testing.T) {
	t.Parallel()

	secret := "test-secret"
	payload := []byte(`{"action":"opened"}`)

	// Compute valid signature.
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	validSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	tests := []struct {
		name   string
		secret string
		sig    string
		want   bool
	}{
		{"valid signature", secret, validSig, true},
		{"wrong secret", "wrong-secret", validSig, false},
		{"empty signature header", secret, "", false},
		{"empty secret", "", validSig, false},
		{"missing sha256 prefix", secret, hex.EncodeToString(mac.Sum(nil)), false},
		{"wrong prefix", secret, "sha512=" + hex.EncodeToString(mac.Sum(nil)), false},
		{"invalid hex", secret, "sha256=zzzz", false},
		{"tampered payload signature", secret, "sha256=" + hex.EncodeToString([]byte("wrong")), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := automations.VerifySignature(payload, tt.secret, tt.sig)
			require.Equal(t, tt.want, got)
		})
	}
}
