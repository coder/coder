// Package chatwebhook verifies Standard Webhooks requests:
// https://github.com/standard-webhooks/standard-webhooks/blob/main/spec/standard-webhooks.md
//
// Verify keeps no state and never logs. Every error it returns is built from
// a fixed string, so it never holds a secret, a received signature, or an
// expected one.
package chatwebhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/xerrors"
)

const (
	// tolerance is how far webhook-timestamp may differ from the server
	// clock, in either direction.
	tolerance = 5 * time.Minute

	secretPrefix = "whsec_"
)

// Verify returns nil when the request is signed with secret, a "whsec_"
// secret. body is the raw request body. now is the server clock. The sender
// signs "webhook-id.webhook-timestamp.body" with HMAC-SHA256 and sends one or
// more "v1,<base64>" entries in webhook-signature.
func Verify(header http.Header, body []byte, secret string, now time.Time) error {
	key, ok := parseSecret(secret)
	if !ok {
		return xerrors.New("secret is not usable")
	}

	// The spec asks senders to keep "." out of the id, because a "." would
	// make the signed string ambiguous.
	id, ok := singleHeader(header, "webhook-id")
	if !ok || id == "" || strings.Contains(id, ".") {
		return xerrors.New("webhook-id header missing, repeated or malformed")
	}
	timestamp, ok := singleHeader(header, "webhook-timestamp")
	if !ok {
		return xerrors.New("webhook-timestamp header missing or repeated")
	}
	// Digits only: no sign, space or fraction. 62 bits keeps time.Unix from
	// overflowing.
	seconds, err := strconv.ParseUint(timestamp, 10, 62)
	if err != nil {
		return xerrors.New("webhook-timestamp is not a unix timestamp")
	}
	age := now.Sub(time.Unix(int64(seconds), 0))
	if age > tolerance || age < -tolerance {
		return xerrors.New("webhook-timestamp outside the allowed window")
	}

	signatures := header.Values("webhook-signature")
	if len(signatures) == 0 {
		return xerrors.New("webhook-signature header missing")
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
	return xerrors.New("no signature matches")
}

// parseSecret returns the HMAC key inside a "whsec_" secret: standard base64
// that decodes to at least one byte. The spec describes keys of 24 to 64
// bytes, but no length is enforced: that would lock out a sender that
// deviates and only protects against the sender's own weak key.
func parseSecret(secret string) ([]byte, bool) {
	encoded, ok := strings.CutPrefix(secret, secretPrefix)
	if !ok {
		return nil, false
	}
	// The decoder skips newlines, so a pasted secret with one would pass.
	if strings.ContainsAny(encoded, "\r\n") {
		return nil, false
	}
	key, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil || len(key) == 0 {
		return nil, false
	}
	return key, true
}

// singleHeader returns the value of the named header when the request has
// exactly one such line. A repeated header is ambiguous, so it counts as
// missing.
func singleHeader(h http.Header, name string) (string, bool) {
	values := h.Values(name)
	if len(values) != 1 {
		return "", false
	}
	return values[0], true
}
