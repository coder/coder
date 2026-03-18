package autochat

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	"golang.org/x/xerrors"
)

// VerifyWebhookSignature validates the HMAC-SHA256 signature of a webhook
// payload. It checks X-Hub-Signature-256 (GitHub) and X-Coder-Signature
// headers. The signature must be hex-encoded, optionally prefixed with
// "sha256=".
func VerifyWebhookSignature(body []byte, header http.Header, secret string) error {
	if secret == "" {
		return xerrors.New("webhook secret must not be empty")
	}

	sig := header.Get("X-Hub-Signature-256")
	if sig == "" {
		sig = header.Get("X-Coder-Signature")
	}
	if sig == "" {
		return xerrors.New("missing webhook signature header (expected X-Hub-Signature-256 or X-Coder-Signature)")
	}
	sig = strings.TrimPrefix(sig, "sha256=")
	sig = strings.ToLower(sig)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))

	// Use hmac.Equal for constant-time comparison to prevent timing
	// attacks.
	if !hmac.Equal([]byte(sig), []byte(expected)) {
		return xerrors.New("webhook signature mismatch")
	}
	return nil
}

// GenerateWebhookSecret generates a cryptographically random hex-encoded
// secret suitable for HMAC-SHA256 webhook verification.
func GenerateWebhookSecret() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		return "", xerrors.Errorf("generate webhook secret: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// MaskSecret returns a masked version of a webhook secret for display.
// Shows the first 8 characters followed by "****".
func MaskSecret(secret string) string {
	if len(secret) <= 8 {
		return "****"
	}
	return secret[:8] + "****"
}
