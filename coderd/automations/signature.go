package automations

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// VerifySignature checks an HMAC-SHA256 signature in the format used
// by GitHub and many other webhook providers: "sha256=<hex-digest>".
// The comparison uses constant-time equality to prevent timing attacks.
func VerifySignature(payload []byte, secret string, signatureHeader string) bool {
	if secret == "" || signatureHeader == "" {
		return false
	}

	parts := strings.SplitN(signatureHeader, "=", 2)
	if len(parts) != 2 || parts[0] != "sha256" {
		return false
	}

	expectedMAC, err := hex.DecodeString(parts[1])
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	actualMAC := mac.Sum(nil)

	return hmac.Equal(actualMAC, expectedMAC)
}
