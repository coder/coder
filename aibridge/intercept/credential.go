package intercept

import (
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/utils"
)

// CredentialKind identifies how a request was authenticated.
// Keep in sync with the credential_kind enum in coderd's database.
type CredentialKind string

// Credential kind constants for interception recording.
const (
	CredentialKindCentralized CredentialKind = "centralized"
	CredentialKindBYOK        CredentialKind = "byok"
)

// Auth header names shared by providers (which set them on resolved
// credentials) and interceptors (which present credentials under them).
const (
	AuthHeaderXAPIKey       = "X-Api-Key" //nolint:gosec // G101 false positive: HTTP header name, not a credential.
	AuthHeaderAuthorization = "Authorization"
)

// Credential is the per-request upstream authentication for an interception.
// It is one of:
//   - Centralized: provider-managed keys with automatic failover.
//   - BYOK: a single user-supplied secret, no failover.
//
// An interception authenticates with exactly one of these, never both: the
// credential is a single value of one concrete type, resolved from the incoming
// headers in the provider's CreateInterceptor.
type Credential interface {
	// Kind reports how the request authenticates.
	Kind() CredentialKind
	// AuthHeader is the upstream header that carries this request's credential
	// (e.g. "X-Api-Key" or "Authorization"), so the client-header middleware
	// preserves it when rebuilding the outgoing request.
	AuthHeader() string
	// Hint is the masked view of the key currently in use, for recording.
	Hint() string
	// Length is the length of the key currently in use, for recording.
	Length() int
}

// BYOK authenticates with a single user-supplied secret and performs no
// failover. Its key is immutable for the lifetime of the interception.
type BYOK struct {
	Secret string
	// Header is the header the user authenticated with.
	Header string
}

func (BYOK) Kind() CredentialKind { return CredentialKindBYOK }
func (b BYOK) AuthHeader() string { return b.Header }
func (b BYOK) Hint() string       { return utils.MaskSecret(b.Secret) }
func (b BYOK) Length() int        { return len(b.Secret) }

// Centralized authenticates via a provider-managed key pool with automatic
// failover across keys.
type Centralized struct {
	Pool *keypool.Pool
	// Header is the provider's canonical auth header.
	Header string
	// current is the pool key most recently selected by the failover loop.
	current string
}

func (*Centralized) Kind() CredentialKind { return CredentialKindCentralized }
func (c *Centralized) AuthHeader() string { return c.Header }
func (c *Centralized) Hint() string       { return utils.MaskSecret(c.current) }
func (c *Centralized) Length() int        { return len(c.current) }

// SetKey records the centralized key currently in use, so the upstream request
// and the recorded hint reflect the key the failover loop selected.
func (c *Centralized) SetKey(value string) { c.current = value }

var (
	_ Credential = BYOK{}
	_ Credential = &Centralized{}
)

// AsBYOK reports whether c is a BYOK credential and returns it if so.
func AsBYOK(c Credential) (BYOK, bool) {
	b, ok := c.(BYOK)
	return b, ok
}

// AsCentralized reports whether c is a centralized credential backed by a key
// pool and returns it if so. It returns false for BYOK and for a pool-less
// centralized credential, such as Bedrock, which signs with AWS. Only
// pool-backed centralized requests fail over across keys.
func AsCentralized(c Credential) (*Centralized, bool) {
	centralized, ok := c.(*Centralized)
	if !ok || centralized.Pool == nil {
		return nil, false
	}
	return centralized, true
}
