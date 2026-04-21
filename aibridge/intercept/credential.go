package intercept

import "github.com/coder/coder/v2/aibridge/utils"

// CredentialKind identifies how a request was authenticated.
// Keep in sync with the credential_kind enum in coderd's database.
type CredentialKind string

// Credential kind constants for interception recording.
const (
	CredentialKindCentralized CredentialKind = "centralized"
	CredentialKindBYOK        CredentialKind = "byok"
)

// CredentialInfo holds credential metadata for an interception.
type CredentialInfo struct {
	Kind   CredentialKind
	Hint   string
	Length int
}

// NewCredentialInfo creates a CredentialInfo from a raw credential.
// The credential is automatically masked before storage so that the
// original secret is never retained.
func NewCredentialInfo(kind CredentialKind, credential string) CredentialInfo {
	return CredentialInfo{
		Kind:   kind,
		Hint:   utils.MaskSecret(credential),
		Length: len(credential),
	}
}
