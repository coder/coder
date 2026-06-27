package intercept

import (
	"context"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/utils"
)

// CredentialKind identifies how a request was authenticated.
// Keep in sync with the credential_kind enum in coderd's database.
type CredentialKind string

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

// Hint placeholders for credentials with no static key value to mask: a pool
// before failover selects a key, and a key resolved dynamically at request time.
const (
	hintFailoverKey     = "<failover key>"
	hintBedrockChainKey = "<aws chain>"
)

// Credential is the per-request upstream authentication for an interception:
//   - BYOK: a user-supplied secret.
//   - Bedrock: AWS Bedrock credentials, used to sign requests.
//   - CentralizedPool: a provider-managed key pool with failover.
type Credential interface {
	Kind() CredentialKind
	// AuthHeader is the header carrying this request's credential, or empty when
	// the credential is not carried in a header.
	AuthHeader() string
	// Hint is a masked, identifiable fragment of the credential.
	Hint() string
	// Length is the length of the credential value.
	Length() int
}

// BYOK authenticates with a single user-supplied secret.
type BYOK struct {
	Secret string
	Header string
}

func (BYOK) Kind() CredentialKind { return CredentialKindBYOK }
func (b BYOK) AuthHeader() string { return b.Header }
func (b BYOK) Hint() string       { return utils.MaskSecret(b.Secret) }
func (b BYOK) Length() int        { return len(b.Secret) }

// Bedrock authenticates with AWS Bedrock: requests are signed (so there is no
// auth header) using either static credentials (when an access key is set) or
// the AWS default credential chain. There is no key pool or failover.
type Bedrock struct {
	AccessKey string
}

func (Bedrock) Kind() CredentialKind { return CredentialKindCentralized }
func (Bedrock) AuthHeader() string   { return "" }
func (b Bedrock) Length() int        { return len(b.AccessKey) }

func (b Bedrock) Hint() string {
	if b.AccessKey == "" {
		return hintBedrockChainKey
	}
	return utils.MaskSecret(b.AccessKey)
}

// ClaudePlatformAWS authenticates against Claude Platform for AWS. In SigV4
// mode requests are signed (so there is no static auth header) using static
// credentials, the AWS default credential chain, or an assumed role. In
// API-key mode the workspace key is sent as x-api-key. There is no key pool or
// failover.
type ClaudePlatformAWS struct {
	// APIKey is set when authenticating with a workspace API key (x-api-key
	// mode). When empty, requests are SigV4-signed and carry no static auth
	// header.
	APIKey string
	// AccessKey is the static AWS access key, if configured. Used only for the
	// masked hint.
	AccessKey string
}

func (ClaudePlatformAWS) Kind() CredentialKind { return CredentialKindCentralized }

// AuthHeader reports the header carrying this request's credential. In API-key
// mode the SDK sets x-api-key and it must be preserved when rebuilding the
// upstream headers; in SigV4 mode the credential lives in the signed
// Authorization header injected after header rebuilding, so there is no static
// header to preserve.
func (c ClaudePlatformAWS) AuthHeader() string {
	if c.APIKey != "" {
		return AuthHeaderXAPIKey
	}
	return ""
}

func (c ClaudePlatformAWS) Length() int {
	if c.APIKey != "" {
		return len(c.APIKey)
	}
	return len(c.AccessKey)
}

func (c ClaudePlatformAWS) Hint() string {
	if c.APIKey != "" {
		return utils.MaskSecret(c.APIKey)
	}
	if c.AccessKey == "" {
		return hintBedrockChainKey
	}
	return utils.MaskSecret(c.AccessKey)
}

// CentralizedPool authenticates with a provider-managed key pool and fails over
// across keys.
type CentralizedPool struct {
	Pool   *keypool.Pool
	Header string
	// currentKey is the key most recently handed out by NextKey, nil until the first call.
	currentKey *keypool.Key
}

func (*CentralizedPool) Kind() CredentialKind { return CredentialKindCentralized }
func (c *CentralizedPool) AuthHeader() string { return c.Header }

func (c *CentralizedPool) Hint() string {
	if c.currentKey != nil {
		return c.currentKey.Hint()
	}
	return hintFailoverKey
}

func (c *CentralizedPool) Length() int {
	if c.currentKey != nil {
		return c.currentKey.Length()
	}
	return 0
}

// NextKey advances the failover walker and records the selected key as the one
// in use.
func (c *CentralizedPool) NextKey(w *keypool.Walker) (*keypool.Key, *keypool.Error) {
	key, err := w.Next()
	if err != nil {
		return nil, err
	}
	c.currentKey = key
	return key, nil
}

var (
	_ Credential = BYOK{}
	_ Credential = Bedrock{}
	_ Credential = ClaudePlatformAWS{}
	_ Credential = &CentralizedPool{}
)

// AsBYOK reports whether c is a BYOK credential and returns it if so.
func AsBYOK(c Credential) (BYOK, bool) {
	b, ok := c.(BYOK)
	return b, ok
}

// AsCentralizedPool reports whether c is a key-pool credential that fails over,
// and returns it if so.
func AsCentralizedPool(c Credential) (*CentralizedPool, bool) {
	pool, ok := c.(*CentralizedPool)
	return pool, ok
}

// WithCredentialInfo returns a context carrying the credential hint and length.
func WithCredentialInfo(ctx context.Context, cred Credential) context.Context {
	return slog.With(ctx,
		slog.F("credential_hint", cred.Hint()),
		slog.F("credential_length", cred.Length()),
	)
}
