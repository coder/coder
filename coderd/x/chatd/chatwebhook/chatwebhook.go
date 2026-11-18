// Package chatwebhook verifies that a webhook request for a chat automation
// comes from the sender that holds the automation's secret.
//
// A verifier keeps no state and never logs. Every error from Verify and
// ValidateSecret is built from a fixed string, so it never holds a secret, a
// received token, or a signature.
package chatwebhook

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"time"

	"golang.org/x/xerrors"
)

// Scheme names how a webhook proves its sender. The values equal the
// webhook_auth enum strings.
type Scheme string

const (
	// SchemeBearer compares the Authorization bearer token with the secret.
	SchemeBearer Scheme = "bearer"
	// SchemeGitHub checks the HMAC-SHA256 in X-Hub-Signature-256.
	SchemeGitHub Scheme = "github"
	// SchemeStandardWebhooks checks the Standard Webhooks signature headers.
	SchemeStandardWebhooks Scheme = "standard_webhooks"
)

var (
	// ErrRejected wraps every error that Verify returns. The webhook route
	// answers it with 401, the same as for an unknown automation.
	ErrRejected = xerrors.New("webhook request rejected")
	// ErrInvalidSecret wraps every error that ValidateSecret returns.
	ErrInvalidSecret = xerrors.New("invalid webhook secret")
)

// WebhookVerifier checks that a webhook request comes from the sender
// that holds the automation's secret.
type WebhookVerifier interface {
	// Verify returns nil when the request is authentic. body is the raw
	// request body, already cut to the body limit. now is the server
	// clock, for schemes that check a timestamp.
	Verify(header http.Header, body []byte, secret string, now time.Time) error
	// GenerateSecret returns a new secret when Coder creates the secret
	// for this scheme, and ok=false when the sender creates it.
	GenerateSecret() (secret string, ok bool, err error)
	// ValidateSecret checks a secret that the owner pasted, for schemes
	// where the sender creates the secret.
	ValidateSecret(secret string) error
}

// NewVerifier returns the verifier for scheme.
func NewVerifier(scheme Scheme) (WebhookVerifier, error) {
	switch scheme {
	case SchemeBearer:
		return bearerVerifier{}, nil
	case SchemeGitHub:
		return githubVerifier{}, nil
	case SchemeStandardWebhooks:
		return standardWebhooksVerifier{tolerance: standardWebhooksTolerance}, nil
	default:
		return nil, xerrors.Errorf("unknown webhook scheme %q", string(scheme))
	}
}

// reject returns the error for a request that failed verification. reason
// must be a constant: it must never hold data from the request or the secret.
func reject(reason string) error {
	return xerrors.Errorf("%s: %w", reason, ErrRejected)
}

// invalidSecret returns the error for a pasted secret that failed
// validation. reason must be a constant, for the same reason as in reject.
func invalidSecret(reason string) error {
	return xerrors.Errorf("%s: %w", reason, ErrInvalidSecret)
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

// secretsEqual compares in constant time. Hashing both sides first keeps
// the comparison time from showing the length of the secret.
func secretsEqual(a, b string) bool {
	hashA, hashB := sha256.Sum256([]byte(a)), sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(hashA[:], hashB[:]) == 1
}

// generatedSecretBytes is the amount of randomness in a secret that Coder
// creates.
const generatedSecretBytes = 32

// generateSecret returns a new secret for a scheme where Coder creates it.
// The unpadded base64url form is safe in a header and in the GitHub
// settings page.
func generateSecret() (string, bool, error) {
	buf := make([]byte, generatedSecretBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", false, xerrors.Errorf("read random bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), true, nil
}
