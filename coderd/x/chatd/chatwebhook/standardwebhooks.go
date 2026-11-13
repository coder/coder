package chatwebhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	// standardWebhooksTolerance is how far webhook-timestamp may differ
	// from the server clock, in either direction.
	standardWebhooksTolerance = 5 * time.Minute

	standardWebhooksSecretPrefix = "whsec_"
)

// standardWebhooksVerifier checks a Standard Webhooks request:
// https://github.com/standard-webhooks/standard-webhooks/blob/main/spec/standard-webhooks.md
// The sender signs "webhook-id.webhook-timestamp.body" with HMAC-SHA256 and
// sends one or more "v1,<base64>" entries in webhook-signature.
type standardWebhooksVerifier struct {
	tolerance time.Duration
}

func (v standardWebhooksVerifier) Verify(header http.Header, body []byte, secret string, now time.Time) error {
	key, err := parseStandardWebhooksSecret(secret)
	if err != nil {
		return reject("secret is not usable")
	}

	// The spec asks senders to keep "." out of the id, because a "." would
	// make the signed string ambiguous.
	id, ok := singleHeader(header, "webhook-id")
	if !ok || id == "" || strings.Contains(id, ".") {
		return reject("webhook-id header missing, repeated or malformed")
	}
	timestamp, ok := singleHeader(header, "webhook-timestamp")
	if !ok {
		return reject("webhook-timestamp header missing or repeated")
	}
	// Digits only: no sign, space or fraction. 62 bits keeps time.Unix from
	// overflowing.
	seconds, err := strconv.ParseUint(timestamp, 10, 62)
	if err != nil {
		return reject("webhook-timestamp is not a unix timestamp")
	}
	age := now.Sub(time.Unix(int64(seconds), 0))
	if age > v.tolerance || age < -v.tolerance {
		return reject("webhook-timestamp outside the allowed window")
	}

	signatures := header.Values("webhook-signature")
	if len(signatures) == 0 {
		return reject("webhook-signature header missing")
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(id + "." + timestamp + "."))
	_, _ = mac.Write(body)
	expected := mac.Sum(nil)

	// A sender can list several signatures, for example during secret
	// rotation. One valid v1 entry is enough. Entries with another version
	// or a malformed entry are skipped.
	for _, line := range signatures {
		for entry := range strings.SplitSeq(line, " ") {
			version, encoded, ok := strings.Cut(entry, ",")
			if !ok || version != "v1" {
				continue
			}
			received, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				continue
			}
			if hmac.Equal(received, expected) {
				return nil
			}
		}
	}
	return reject("no signature matches")
}

// GenerateSecret reports ok=false: the sender creates the secret.
func (standardWebhooksVerifier) GenerateSecret() (string, bool, error) {
	return "", false, nil
}

// ValidateSecret accepts "whsec_" followed by standard base64 that decodes
// to at least one byte. The spec describes secrets of 24 to 64 bytes, but
// this check does not enforce a length: it would lock out a sender that
// deviates and only protects the owner from the sender's own weak key.
func (standardWebhooksVerifier) ValidateSecret(secret string) error {
	_, err := parseStandardWebhooksSecret(secret)
	return err
}

// parseStandardWebhooksSecret returns the HMAC key inside a "whsec_" secret.
func parseStandardWebhooksSecret(secret string) ([]byte, error) {
	encoded, ok := strings.CutPrefix(secret, standardWebhooksSecretPrefix)
	if !ok {
		return nil, invalidSecret("secret has no whsec_ prefix")
	}
	// The decoder skips newlines, so a pasted secret with one would pass.
	if strings.ContainsAny(encoded, "\r\n") {
		return nil, invalidSecret("secret contains a line break")
	}
	key, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil {
		return nil, invalidSecret("secret is not base64")
	}
	if len(key) == 0 {
		return nil, invalidSecret("secret is empty")
	}
	return key, nil
}
