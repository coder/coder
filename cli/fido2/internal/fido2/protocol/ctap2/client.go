package ctap2

import (
	"io"
	"iter"

	"github.com/coder/coder/v2/cli/fido2/internal/fido2/protocol/webauthn"
	"github.com/ldclabs/cose/key"
)

// Client is the interface for CTAP2 client operations.
// It defines the methods that an authenticator must support to be compliant with CTAP2.
type Client interface {
	io.Closer

	// MakeCredential creates a new credential on the authenticator.
	MakeCredential(pinUvAuthProtocolType PinUvAuthProtocolType, pinUvAuthToken []byte, clientData []byte,
		rp webauthn.PublicKeyCredentialRpEntity, user webauthn.PublicKeyCredentialUserEntity,
		pubKeyCredParams []webauthn.PublicKeyCredentialParameters,
		excludeList []webauthn.PublicKeyCredentialDescriptor,
		extensions *CreateExtensionInputs, options map[Option]bool,
		enterpriseAttestation uint,
		attestationFormatsPreference []webauthn.AttestationStatementFormatIdentifier,
	) (*AuthenticatorMakeCredentialResponse, error)

	// GetAssertion retrieves an assertion from the authenticator.
	GetAssertion(pinUvAuthProtocolType PinUvAuthProtocolType, pinUvAuthToken []byte,
		rpID string, clientData []byte, allowList []webauthn.PublicKeyCredentialDescriptor,
		extensions *GetExtensionInputs, options map[Option]bool,
	) iter.Seq2[*AuthenticatorGetAssertionResponse, error]

	// GetInfo retrieves the authenticator's information.
	GetInfo() (*AuthenticatorGetInfoResponse, error)

	// GetKeyAgreement retrieves the key agreement key for the specified PIN/UV auth protocol.
	GetKeyAgreement(pinUvAuthProtocolType PinUvAuthProtocolType) (key.Key, error)

	// GetPinToken retrieves the PIN token from the authenticator.
	// This method is used for backward compatibility.
	GetPinToken(pinUvAuthProtocolType PinUvAuthProtocolType, keyAgreement key.Key, pin string) ([]byte, error)

	// GetPinUvAuthTokenUsingPinWithPermissions retrieves the PIN/UV auth token using PIN with permissions.
	GetPinUvAuthTokenUsingPinWithPermissions(
		pinUvAuthProtocolType PinUvAuthProtocolType,
		keyAgreement key.Key,
		pin string,
		permissions Permission,
		rpID string,
	) ([]byte, error)
}
