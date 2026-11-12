package chatwebhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"time"
)

// githubVerifier checks the HMAC-SHA256 of the raw body that GitHub sends
// in X-Hub-Signature-256.
type githubVerifier struct{}

func (githubVerifier) Verify(header http.Header, body []byte, secret string, _ time.Time) error {
	// HMAC accepts an empty key, so a sender could sign with one.
	if secret == "" {
		return reject("secret is empty")
	}
	value, ok := singleHeader(header, "X-Hub-Signature-256")
	if !ok {
		return reject("signature header missing or repeated")
	}
	digest, ok := strings.CutPrefix(value, "sha256=")
	if !ok {
		return reject("signature header has no sha256= prefix")
	}
	received, err := hex.DecodeString(digest)
	if err != nil {
		return reject("signature is not hex")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	if !hmac.Equal(received, mac.Sum(nil)) {
		return reject("signature does not match")
	}
	return nil
}

func (githubVerifier) GenerateSecret() (string, bool, error) {
	return generateSecret()
}

func (githubVerifier) ValidateSecret(string) error {
	return invalidSecret("coder creates the secret for this scheme")
}
